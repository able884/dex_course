package solmodel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/duke-git/lancet/v2/slice"
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/threading"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"richcode.cc/dex/pkg/transfer"
)

// avoid unused err
var _ = InitField
var _ SolTokenAccountModel = (*customSolTokenAccountModel)(nil)
var solTokenAccountModelLock sync.Mutex

type (
	// SolTokenAccountModel is an interface to be customized, add more methods here,
	// and implement the added methods in customSolTokenAccountModel.
	SolTokenAccountModel interface {
		solTokenAccountModel
		customSolTokenAccountLogicModel
	}

	customSolTokenAccountLogicModel interface {
		WithSession(tx *gorm.DB) SolTokenAccountModel
		BatchInsertTokenAccounts(ctx context.Context, tokenAccounts []*SolTokenAccount) error
		BatchUpsertTokenAccounts(ctx context.Context, tokenAccounts []*SolTokenAccount) error
		CountByTokenAddressWithTime(ctx context.Context, chainId int64, tokenAddress string, createdAt time.Time) (int64, error)
	}

	customSolTokenAccountModel struct {
		*defaultSolTokenAccountModel
	}
)

func (c customSolTokenAccountModel) WithSession(tx *gorm.DB) SolTokenAccountModel {
	newModel := *c.defaultSolTokenAccountModel
	c.defaultSolTokenAccountModel = &newModel
	c.conn = tx
	return c
}

// NewSolTokenAccountModel returns a model for the database table.
func NewSolTokenAccountModel(conn *gorm.DB) SolTokenAccountModel {
	return &customSolTokenAccountModel{
		defaultSolTokenAccountModel: newSolTokenAccountModel(conn),
	}
}

// 动态计算表名
func (m *defaultSolTokenAccountModel) getTableName(createdAt time.Time) string {
	// 获取当前时间所在周的开始时间（按周一为一周的开始）
	startOfWeek := getStartOfWeek(createdAt)

	// 格式化时间为YYYYMMDD
	timePart := startOfWeek.Format("20060102")

	// 拼接最终表名
	return fmt.Sprintf("sol_token_account_%s", timePart)
}

// 获取给定时间所在周的开始时间（按周一为一周的开始）
func getStartOfWeek(t time.Time) time.Time {
	year, month, day := t.Date()
	weekday := t.Weekday()
	offset := (int(weekday) + 6) % 7 // 让周一成为一周的第一天

	// 将日期调整为当前周的周一
	startOfWeek := time.Date(year, month, day-offset, 0, 0, 0, 0, time.UTC)
	return startOfWeek
}

func (m *defaultSolTokenAccountModel) BatchInsertTokenAccounts(ctx context.Context, tokenAccounts []*SolTokenAccount) error {

	if len(tokenAccounts) == 0 {
		return nil
	}
	tracer := otel.Tracer("defaultSolTokenAccountModel BatchInsertTokenAccounts")
	ctx, span := tracer.Start(context.Background(), "BatchInsertTokenAccounts")
	defer span.End()

	startTime := time.Now()
	defer func() {
		logc.Infof(ctx, "BatchInsertTokenAccounts cost: %v", time.Since(startTime))
	}()

	// 按照分表规则对 tokenAccounts 分组
	tableGroups := make(map[string][]*SolTokenAccount)
	for _, account := range tokenAccounts {
		tableName := m.getTableName(account.CreatedAt)
		tableGroups[tableName] = append(tableGroups[tableName], account)
	}

	times := 20 * 10
	getLock := false
	for {
		time.Sleep(time.Millisecond * 50)
		times--
		if getLock {
			break
		}

		if times <= 0 {
			return errors.New("get lock time out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lock := solTokenAccountModelLock.TryLock()
			if lock {
				getLock = true
				break
			}
		}

	}

	defer solTokenAccountModelLock.Unlock()

	group := threading.NewRoutineGroup()

	for tableName, accounts := range tableGroups {

		group.RunSafe(func(tableName string, accounts []*SolTokenAccount) func() {
			return func() {

				slice.Reverse(accounts)
				accounts = slice.UniqueByComparator[*SolTokenAccount](accounts, func(item *SolTokenAccount, other *SolTokenAccount) bool {
					if item.OwnerAddress == other.OwnerAddress && item.TokenAccountAddress == other.TokenAccountAddress {
						logc.Errorf(ctx, "BatchInsertTokenAccounts:UniqueByComparator dup token address: %v, account1: %v, account2: %v", item.TokenAddress, item.Balance, other.Balance)
						return true
					}
					return false
				})
				slice.Reverse(accounts)

				_ = setLockWaitTimeout(m.conn, 5)

				_, _ = m.createTableIfNotExists(tableName)

				now := time.Now()

				tx := m.conn.Table(tableName).WithContext(ctx).Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "owner_address"}, {Name: "token_account_address"}}, // 冲突字段
					DoUpdates: clause.Assignments(map[string]interface{}{
						"balance":    gorm.Expr("CASE WHEN VALUES(slot) > slot THEN VALUES(balance) ELSE balance END"),
						"slot":       gorm.Expr("CASE WHEN VALUES(slot) > slot THEN VALUES(slot) ELSE slot END"),
						"updated_at": gorm.Expr("CASE WHEN VALUES(slot) > slot THEN VALUES(updated_at) ELSE updated_at END"),
					}),
				}).CreateInBatches(accounts, 1024)

				err := tx.Error
				affected := tx.RowsAffected

				if time.Since(now) >= time.Second*5 {
					logc.Errorf(ctx, "BatchInsertTokenAccounts duration exceed 5s: %v size: %v, cost: %v, table name: %v, affected: %v,sql: %v", err, len(accounts), time.Since(now), tableName, affected, tx.Statement.SQL.String())
				}

				if err != nil {
					accountsStr, _ := transfer.Struct2String(accounts)
					logc.Errorf(ctx, "BatchInsertTokenAccounts err: %v size: %v, cost: %v, table name: %v, data: %v", err, len(accounts), time.Since(now), tableName, accountsStr)
					return
				}

				logc.Infof(ctx, "BatchInsertTokenAccounts success size: %v, cost: %v, table name: %v, affected: %v", len(accounts), time.Since(now), tableName, affected)

			}
		}(tableName, accounts))

	}

	group.Wait()

	return nil
}

// BatchUpsertTokenAccounts 批量插入或更新TokenAccount，若owner_address已存在则更新
func (m *defaultSolTokenAccountModel) BatchUpsertTokenAccounts(ctx context.Context, tokenAccounts []*SolTokenAccount) error {
	if len(tokenAccounts) == 0 {
		return nil
	}

	tracer := otel.Tracer("defaultSolTokenAccountModel BatchUpsertTokenAccounts")
	ctx, span := tracer.Start(context.Background(), "BatchUpsertTokenAccounts")
	defer span.End()

	startTime := time.Now()
	defer func() {
		logc.Infof(ctx, "BatchUpsertTokenAccounts cost: %v", time.Since(startTime))
	}()

	// 按照分表规则对 tokenAccounts 分组
	tableGroups := make(map[string][]*SolTokenAccount)
	for _, account := range tokenAccounts {
		tableName := m.getTableName(account.CreatedAt)
		tableGroups[tableName] = append(tableGroups[tableName], account)
	}

	times := 20 * 10
	getLock := false
UpsertGetLockLoop:
	for {
		time.Sleep(time.Millisecond * 50)
		times--
		if getLock {
			break
		}

		if times <= 0 {
			return errors.New("get lock time out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lock := solTokenAccountModelLock.TryLock()
			if lock {
				getLock = true
				break UpsertGetLockLoop
			}
		}
	}

	defer solTokenAccountModelLock.Unlock()

	group := threading.NewRoutineGroup()

	for tableName, accounts := range tableGroups {
		group.RunSafe(func(tableName string, accounts []*SolTokenAccount) func() {
			return func() {
				// 去重处理
				slice.Reverse(accounts)
				accounts = slice.UniqueByComparator[*SolTokenAccount](accounts, func(item *SolTokenAccount, other *SolTokenAccount) bool {
					if item.OwnerAddress == other.OwnerAddress && item.TokenAccountAddress == other.TokenAccountAddress {
						logc.Errorf(ctx, "BatchUpsertTokenAccounts:UniqueByComparator dup token address: %v, account1: %v, account2: %v", item.TokenAddress, item.Balance, other.Balance)
						return true
					}
					return false
				})
				slice.Reverse(accounts)

				_ = setLockWaitTimeout(m.conn, 5)

				if _, err := m.createTableIfNotExists(tableName); err != nil {
					logc.Errorf(ctx, "BatchUpsertTokenAccounts create table %s err:%v", tableName, err)
					return
				}

				now := time.Now()

				// 使用 OnConflict 实现 upsert 逻辑
				// 当 owner_address 和 token_account_address 冲突时，更新所有字段
				tx := m.conn.Table(tableName).WithContext(ctx).Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "owner_address"}, {Name: "token_account_address"}}, // 冲突字段
					DoUpdates: clause.Assignments(map[string]interface{}{
						"status":        gorm.Expr("VALUES(status)"),
						"chain_id":      gorm.Expr("VALUES(chain_id)"),
						"token_address": gorm.Expr("VALUES(token_address)"),
						"token_decimal": gorm.Expr("VALUES(token_decimal)"),
						"balance":       gorm.Expr("CASE WHEN VALUES(slot) > slot THEN VALUES(balance) ELSE balance END"),
						"slot":          gorm.Expr("CASE WHEN VALUES(slot) > slot THEN VALUES(slot) ELSE slot END"),
						"updated_at":    gorm.Expr("VALUES(updated_at)"),
					}),
				}).CreateInBatches(accounts, 1024)

				err := tx.Error
				affected := tx.RowsAffected

				if time.Since(now) >= time.Second*5 {
					logc.Errorf(ctx, "BatchUpsertTokenAccounts duration exceed 5s: %v size: %v, cost: %v, table name: %v, affected: %v", err, len(accounts), time.Since(now), tableName, affected)
				}

				if err != nil {
					accountsStr, _ := transfer.Struct2String(accounts)
					logc.Errorf(ctx, "BatchUpsertTokenAccounts err: %v size: %v, cost: %v, table name: %v, data: %v", err, len(accounts), time.Since(now), tableName, accountsStr)
					return
				}

				logc.Infof(ctx, "BatchUpsertTokenAccounts success size: %v, cost: %v, table name: %v, affected: %v", len(accounts), time.Since(now), tableName, affected)
			}
		}(tableName, accounts))
	}

	group.Wait()

	return nil
}

func (m *defaultSolTokenAccountModel) customCacheKeys(data *SolTokenAccount) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func setLockWaitTimeout(db *gorm.DB, timeout int) error {
	return db.Exec(fmt.Sprintf("SET SESSION innodb_lock_wait_timeout = %d", timeout)).Error
}

// 动态创建表（每7天一个表）
func (m *defaultSolTokenAccountModel) createTableIfNotExists(tableName string) (string, error) {
	// 自动创建表
	exists := m.conn.Migrator().HasTable(tableName)
	if !exists {
		_ = m.conn.Table(tableName).AutoMigrate(&SolTokenAccount{})
	}

	return tableName, nil
}

// CountByTokenAddressWithTime 统计指定时间之后某个代币的持有者数量
// 优化：使用动态表名，支持分表查询
func (m *defaultSolTokenAccountModel) CountByTokenAddressWithTime(ctx context.Context, chainId int64, tokenAddress string, createdAt time.Time) (int64, error) {
	tableName := m.getTableName(createdAt)

	var count int64
	err := m.conn.WithContext(ctx).
		Table(tableName).
		Where("chain_id = ? AND token_address = ? AND balance > 0 AND created_at >= ?", chainId, tokenAddress, createdAt).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}
