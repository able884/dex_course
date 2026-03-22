package solmodel

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeModel = (*customTradeModel)(nil)

var tradeModelLock sync.Mutex
var tableLock sync.Mutex

type (
	// TradeModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTradeModel.
	TradeModel interface {
		tradeModel
		customTradeLogicModel
	}

	customTradeLogicModel interface {
		WithSession(tx *gorm.DB) TradeModel
		BatchInsertTrades(ctx context.Context, trades []*Trade) error
	}

	customTradeModel struct {
		*defaultTradeModel
	}
)

func (c customTradeModel) WithSession(tx *gorm.DB) TradeModel {
	newModel := *c.defaultTradeModel
	c.defaultTradeModel = &newModel
	c.conn = tx
	return c
}

// NewTradeModel returns a model for the database table.
func NewTradeModel(conn *gorm.DB) TradeModel {
	return &customTradeModel{
		defaultTradeModel: newTradeModel(conn),
	}
}

func (m *defaultTradeModel) customCacheKeys(data *Trade) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

// createShardTableIfNotExists 自动创建表（如果不存在）
func (m *defaultTradeModel) createShardTableIfNotExists(table string) error {

	exists := m.conn.Migrator().HasTable(table)
	if !exists {
		tableLock.Lock()
		defer tableLock.Unlock()
		exists := m.conn.Migrator().HasTable(table)
		if !exists {
			logx.Infof("createShardTableIfNotExists will create table: %v", table)
			err := m.conn.Table(table).Migrator().CreateTable(&Trade{})
			if err != nil {
				return err
			}
			logx.Infof("createShardTableIfNotExists:table success: %v", table)
		}
	}
	return nil
}

// getShardTableNameByTime generates a table name based on the given timestamp (30-minute interval)
func getShardTableNameByTime(baseTable string, t time.Time) string {
	// Format the table name as: baseTable_<year>_<month>_<day>
	return fmt.Sprintf("%s_%d_%02d_%02d",
		baseTable,
		t.Year(),  // Year
		t.Month(), // Month
		t.Day())   // Day
}

func (m *defaultTradeModel) BatchInsertTrades(ctx context.Context, trades []*Trade) error {
	if len(trades) == 0 {
		return nil
	}

	// Group data by table name
	tableGroups := make(map[string][]*Trade)
	for _, trade := range trades {
		table := getShardTableNameByTime("trade", trade.BlockTime)
		tableGroups[table] = append(tableGroups[table], trade)
	}

	// tradeModelLock.Lock()
	// defer tradeModelLock.Unlock()
	// Insert data into each table
	for table, group := range tableGroups {
		// Ensure the table exists
		if err := m.createShardTableIfNotExists(table); err != nil {
			return fmt.Errorf("failed to create table %s: %w", table, err)
		}

		// Batch insert data into the table
		// err := m.conn.WithContext(ctx).Table(table).Clauses(clause.OnConflict{
		// 	Columns:   []clause.Column{{Name: "hash_id_index"}}, // Update on conflict using unique index
		// 	UpdateAll: true,
		// }).CreateInBatches(group, 1024).Error
		// now := time.Now()
		err := m.conn.WithContext(ctx).Table(table).CreateInBatches(group, 1024).Error
		// logx.Infof("BatchInsertTrades size: %v, cost: %v", len(group), time.Since(now))
		if err != nil && strings.Contains(err.Error(), "Deadlock found when trying to get lock") {
			tradeModelLock.Lock()
			err := m.conn.WithContext(ctx).Table(table).CreateInBatches(group, 1024).Error
			tradeModelLock.Unlock()
			return errors.Wrap(err, fmt.Sprintf("BatchInsertTrades error after get lock: %v", len(group)))
		}

		if err != nil {
			return fmt.Errorf("failed to batch insert into table %s: %w", table, err)
		}
	}

	return nil
}
