package trademodel

import (
	"context"
	"time"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TradeOrderLogModel = (*customTradeOrderLogModel)(nil)

type (
	// TradeOrderLogModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTradeOrderLogModel.
	TradeOrderLogModel interface {
		tradeOrderLogModel
		customTradeOrderLogLogicModel
	}

	customTradeOrderLogLogicModel interface {
		WithSession(tx *gorm.DB) TradeOrderLogModel
		InsertWithOrder(ctx context.Context, order *TradeOrder) error
	}

	customTradeOrderLogModel struct {
		*defaultTradeOrderLogModel
	}
)

func (c customTradeOrderLogModel) WithSession(tx *gorm.DB) TradeOrderLogModel {
	newModel := *c.defaultTradeOrderLogModel
	c.defaultTradeOrderLogModel = &newModel
	c.conn = tx
	return c
}

// NewTradeOrderLogModel returns a model for the database table.
func NewTradeOrderLogModel(conn *gorm.DB) TradeOrderLogModel {
	return &customTradeOrderLogModel{
		defaultTradeOrderLogModel: newTradeOrderLogModel(conn),
	}
}

func (m *defaultTradeOrderLogModel) customCacheKeys(data *TradeOrderLog) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (c customTradeOrderLogModel) InsertWithOrder(ctx context.Context, o *TradeOrder) error {
	// Use raw SQL to handle duplicate key constraint
	// If the order_id + status combination already exists, update the existing record
	query := `
		INSERT INTO trade_order_log (
			order_id, uid, trade_type, chain_id, token_ca, swap_type, wallet_index, wallet_address,
			is_auto_slippage, slippage, is_anti_mev, gas_type, status, fail_reason, double_out,
			order_cap, order_amount, order_price_base, order_value_base, order_base_price,
			final_cap, final_amount, final_price_base, final_value_base, final_base_price,
			gas_fee, priority_fee, dex_fee, server_fee, jito_fee, tx_hash, dex_name, pair_ca,
			created_at, drawdown_price, trailing_percent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			uid = VALUES(uid),
			trade_type = VALUES(trade_type),
			chain_id = VALUES(chain_id),
			token_ca = VALUES(token_ca),
			swap_type = VALUES(swap_type),
			wallet_index = VALUES(wallet_index),
			wallet_address = VALUES(wallet_address),
			is_auto_slippage = VALUES(is_auto_slippage),
			slippage = VALUES(slippage),
			is_anti_mev = VALUES(is_anti_mev),
			gas_type = VALUES(gas_type),
			fail_reason = VALUES(fail_reason),
			double_out = VALUES(double_out),
			order_cap = VALUES(order_cap),
			order_amount = VALUES(order_amount),
			order_price_base = VALUES(order_price_base),
			order_value_base = VALUES(order_value_base),
			order_base_price = VALUES(order_base_price),
			final_cap = VALUES(final_cap),
			final_amount = VALUES(final_amount),
			final_price_base = VALUES(final_price_base),
			final_value_base = VALUES(final_value_base),
			final_base_price = VALUES(final_base_price),
			gas_fee = VALUES(gas_fee),
			priority_fee = VALUES(priority_fee),
			dex_fee = VALUES(dex_fee),
			server_fee = VALUES(server_fee),
			jito_fee = VALUES(jito_fee),
			tx_hash = VALUES(tx_hash),
			dex_name = VALUES(dex_name),
			pair_ca = VALUES(pair_ca),
			drawdown_price = VALUES(drawdown_price),
			trailing_percent = VALUES(trailing_percent)
	`

	now := time.Now().UTC()
	err := c.conn.WithContext(ctx).Exec(query,
		o.Id, o.Uid, o.TradeType, o.ChainId, o.TokenCa, o.SwapType, o.WalletIndex, o.WalletAddress,
		o.IsAutoSlippage, o.Slippage, o.IsAntiMev, o.GasType, o.Status, o.FailReason, o.DoubleOut,
		o.OrderCap, o.OrderAmount, o.OrderPriceBase, o.OrderValueBase, o.OrderBasePrice,
		o.FinalCap, o.FinalAmount, o.FinalPriceBase, o.FinalValueBase, o.FinalBasePrice,
		o.GasFee, o.PriorityFee, o.DexFee, o.ServerFee, o.JitoFee, o.TxHash, o.DexName, o.PairCa,
		now, o.DrawdownPrice, o.TrailingPercent,
	).Error

	return err
}
