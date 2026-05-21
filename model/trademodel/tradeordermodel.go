package trademodel

import (
	"context"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeOrderModel = (*customTradeOrderModel)(nil)

type (
	// TradeOrderModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTradeOrderModel.
	TradeOrderModel interface {
		tradeOrderModel
		customTradeOrderLogicModel
	}

	customTradeOrderLogicModel interface {
		WithSession(tx *gorm.DB) TradeOrderModel
		UpdateOrderBySelect(ctx context.Context, order *TradeOrder, selectStr ...string) error
		InsertWithLog(ctx context.Context, data *TradeOrder) error
		UpdateOrderStatus(ctx context.Context, order *TradeOrder, oldStatus, newStatus int64) (int64, error)
	}

	customTradeOrderModel struct {
		*defaultTradeOrderModel
	}
)

func (c customTradeOrderModel) WithSession(tx *gorm.DB) TradeOrderModel {
	newModel := *c.defaultTradeOrderModel
	c.defaultTradeOrderModel = &newModel
	c.conn = tx
	return c
}

// NewTradeOrderModel returns a model for the database table.
func NewTradeOrderModel(conn *gorm.DB) TradeOrderModel {
	return &customTradeOrderModel{
		defaultTradeOrderModel: newTradeOrderModel(conn),
	}
}

func (m *defaultTradeOrderModel) customCacheKeys(data *TradeOrder) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (c customTradeOrderModel) UpdateOrderBySelect(ctx context.Context, order *TradeOrder, selectStr ...string) error {
	return c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&order).Select(selectStr).Updates(&order).Error
		if err != nil {
			logx.WithContext(ctx).Errorf("UpdateOrderBySelect err %s", err.Error())
			return err
		}
		err = NewTradeOrderLogModel(tx).InsertWithOrder(ctx, order)
		return err
	})
}

func (c customTradeOrderModel) InsertWithLog(ctx context.Context, order *TradeOrder) error {
	return c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Save(&order).Error
		if err != nil {
			return err
		}
		return NewTradeOrderLogModel(tx).InsertWithOrder(ctx, order)
	})
}

func (c customTradeOrderModel) UpdateOrderStatus(ctx context.Context, order *TradeOrder, oldStatus, newStatus int64) (int64, error) {
	var rowsAffected int64
	err := c.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx = tx.Model(&order).Where("status = ?", oldStatus).Update("status", newStatus)
		err := tx.Error
		if err != nil {
			return err
		}
		rowsAffected = tx.RowsAffected
		if rowsAffected == 0 {
			return nil
		}
		// todo: error
		// return NewTradeOrderLogModel(tx).InsertWithOrder(ctx, order)
		return nil
	})

	return rowsAffected, err
}
