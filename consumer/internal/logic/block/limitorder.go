package block

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/market/marketclient"
	"richcode.cc/dex/model/limitordermodel"
	limitorderpkg "richcode.cc/dex/pkg/limitorder"
	"richcode.cc/dex/pkg/types"
)

// Limit Order instruction discriminators (from IDL)
var (
	// place_order discriminator: [51, 194, 155, 175, 109, 130, 96, 106]
	DiscriminatorPlaceOrder = []byte{51, 194, 155, 175, 109, 130, 96, 106}

	// cancel_order discriminator: [95, 129, 237, 240, 8, 49, 223, 132]
	DiscriminatorCancelOrder = []byte{95, 129, 237, 240, 8, 49, 223, 132}

	// deposit discriminator: [242, 35, 198, 137, 82, 225, 242, 182]
	DiscriminatorDeposit = []byte{242, 35, 198, 137, 82, 225, 242, 182}

	// withdraw discriminator: [183, 18, 70, 156, 148, 109, 161, 34]
	DiscriminatorWithdraw = []byte{183, 18, 70, 156, 148, 109, 161, 34}

	// match_orders discriminator: [17, 1, 201, 93, 7, 51, 251, 134]
	DiscriminatorMatchOrders = []byte{17, 1, 201, 93, 7, 51, 251, 134}

	// create_margin discriminator: [141, 121, 208, 99, 171, 176, 254, 118]
	DiscriminatorCreateMargin = []byte{141, 121, 208, 99, 171, 176, 254, 118}

	// emergency_close_order discriminator: [206, 184, 70, 54, 173, 21, 252, 137]
	DiscriminatorEmergencyClose = []byte{206, 184, 70, 54, 173, 21, 252, 137}

	// sweep_expired discriminator: [10, 72, 70, 57, 62, 128, 19, 22]
	DiscriminatorSweepExpired = []byte{10, 72, 70, 57, 62, 128, 19, 22}

	// close_order discriminator: [123, 134, 81, 0, 49, 68, 98, 145]
	DiscriminatorCloseOrder = []byte{123, 134, 81, 0, 49, 68, 98, 145}

	// update_custom_price discriminator: [172, 30, 208, 193, 197, 146, 17, 148]
	DiscriminatorUpdateCustomPrice = []byte{172, 30, 208, 193, 197, 146, 17, 148}
)

type LimitOrderDecoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction
	innerInstruction    *client.InnerInstruction
}

func DecodeLimitOrderInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, innerInstructions *client.InnerInstruction) (trade *types.TradeWithPair, err error) {
	decoder := &LimitOrderDecoder{
		ctx:                 ctx,
		svcCtx:              sc,
		dtx:                 dtx,
		compiledInstruction: instruction,
		innerInstruction:    innerInstructions,
	}

	return decoder.Decode()
}

func (d *LimitOrderDecoder) Decode() (*types.TradeWithPair, error) {
	data := d.compiledInstruction.Data
	if len(data) < 8 {
		return nil, fmt.Errorf("instruction data too short")
	}

	discriminator := data[:8]

	logx.Infof("[LimitOrder] Decoding instruction with discriminator: %v", discriminator)

	switch {
	case bytesEqual(discriminator, DiscriminatorPlaceOrder):
		return d.handlePlaceOrder()
	case bytesEqual(discriminator, DiscriminatorCancelOrder):
		return d.handleCancelOrder()
	case bytesEqual(discriminator, DiscriminatorDeposit):
		return d.handleDeposit()
	case bytesEqual(discriminator, DiscriminatorWithdraw):
		return d.handleWithdraw()
	case bytesEqual(discriminator, DiscriminatorMatchOrders):
		return d.handleMatchOrders()
	case bytesEqual(discriminator, DiscriminatorCreateMargin):
		return d.handleCreateMargin()
	case bytesEqual(discriminator, DiscriminatorEmergencyClose):
		return d.handleEmergencyClose()
	case bytesEqual(discriminator, DiscriminatorSweepExpired):
		return d.handleSweepExpired()
	case bytesEqual(discriminator, DiscriminatorCloseOrder):
		return d.handleCloseOrder()
	case bytesEqual(discriminator, DiscriminatorUpdateCustomPrice):
		return d.handleUpdateCustomPrice()
	default:
		logx.Infof("[LimitOrder] Unknown instruction discriminator: %v", discriminator)
		// Unknown instruction, ignore
		return nil, nil
	}
}

// 解析下单指令，保存订单到数据库，并发布订单簿更新和用户订单更新到 Redis 频道
func (d *LimitOrderDecoder) handlePlaceOrder() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// Log for debugging
	logx.Infof("[LimitOrder] place_order accounts count: %d, accounts: %v", len(accounts), accounts)

	// 根据 IDL，place_order 有 6 个账户：
	// 0: config, 1: market, 2: margin, 3: order, 4: owner, 5: system_program
	if len(accounts) < 6 {
		logx.Errorf("[LimitOrder] place_order: insufficient accounts, got %d, need 6", len(accounts))
		logx.Errorf("[LimitOrder] place_order: tx=%s, accounts=%v", d.dtx.TxHash, accounts)
		return nil, fmt.Errorf("place_order: insufficient accounts, got %d", len(accounts))
	}

	// Parse accounts based on IDL order
	configPda := tx.AccountKeys[accounts[0]].String()
	marketPda := tx.AccountKeys[accounts[1]].String()
	marginPda := tx.AccountKeys[accounts[2]].String()
	orderPda := tx.AccountKeys[accounts[3]].String()
	owner := tx.AccountKeys[accounts[4]].String()
	systemProgram := tx.AccountKeys[accounts[5]].String()

	logx.Infof("[LimitOrder] place_order parsed accounts: config=%s, market=%s, margin=%s, order=%s, owner=%s, system=%s",
		configPda, marketPda, marginPda, orderPda, owner, systemProgram)

	// Parse instruction data: discriminator(8) + PlaceOrderParams + whitelist_proof
	// PlaceOrderParams: side(1) + price_lots(8) + qty_lots(8) + expiry_slot(8) + min_fill_bps(option<u16>) + self_trade_behavior(1)
	data := d.compiledInstruction.Data[8:] // Skip discriminator
	if len(data) < 26 {
		return nil, fmt.Errorf("place_order: insufficient data, got %d bytes", len(data))
	}

	// Parse PlaceOrderParams
	side := data[0] // Side enum: 0=Bid, 1=Ask
	priceLots := binary.LittleEndian.Uint64(data[1:9])
	qtyLots := binary.LittleEndian.Uint64(data[9:17])
	expirySlot := binary.LittleEndian.Uint64(data[17:25])

	// min_fill_bps is Option<u16>: 1 byte discriminator + 2 bytes value if Some
	var minFillBps *uint16
	offset := 25
	if len(data) > offset {
		hasMinFill := data[offset]
		offset++
		if hasMinFill == 1 && len(data) >= offset+2 {
			val := binary.LittleEndian.Uint16(data[offset : offset+2])
			minFillBps = &val
			offset += 2
		}
	}

	// self_trade_behavior: 1 byte enum (0=DecrementTake, 1=CancelNew)
	selfTradeBehavior := uint8(0) // Default to DecrementTake
	if len(data) > offset {
		selfTradeBehavior = data[offset]
	}

	// 获取市场信息以计算显示价格和数量
	marketResp, err := d.svcCtx.MarketService.GetLimitOrderMarkets(d.ctx, &marketclient.GetLimitOrderMarketsRequest{
		ChainId:  100000,
		PageNo:   1,
		PageSize: 1000,
	})
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market info: %v", err)
		return nil, err
	}

	var marketItem *marketclient.MarketItem
	for _, m := range marketResp.Markets {
		if m.MarketPda == marketPda {
			marketItem = m
			break
		}
	}

	if marketItem == nil {
		logx.Errorf("[LimitOrder] Market not found: %s", marketPda)
		return nil, fmt.Errorf("market not found: %s", marketPda)
	}

	// 根据市场 PDA 从数据库获取市场信息，以获取市场 ID 和最小下单量等参数
	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}

	// 计算显示价格和数量，考虑代币精度
	baseMultiplier := float64(pow10LimitOrder(int32(marketDb.BaseDecimals)))
	quoteMultiplier := float64(pow10LimitOrder(int32(marketDb.QuoteDecimals)))
	priceDisplay := float64(priceLots) * baseMultiplier / quoteMultiplier
	qtyDisplay := float64(qtyLots) / baseMultiplier
	totalValueDisplay := priceDisplay * qtyDisplay

	// Side枚举定义：0=Bid(买方),1=Ask(卖方)
	// lockedSide规则：Bid(0)锁定报价币(Quote=2)、Ask(1)锁定基准币(Base=1)
	lockedSide := int64(2) // Quote
	if side == 1 {         // Ask
		lockedSide = 1 // Base
	}

	// Convert side to database format: Bid(0)→1, Ask(1)→2
	dbSide := int64(side) + 1

	logx.Infof("[LimitOrder] place_order side conversion: chain_side=%d, db_side=%d, locked_side=%d", side, dbSide, lockedSide)

	// 检查订单是否已存在（可能是重复处理或链上状态不一致）
	existingOrder, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, orderPda)
	if err == nil && existingOrder != nil {
		logx.Infof("[LimitOrder] Order already exists: order_pda=%s, status=%d, skipping insert", orderPda, existingOrder.Status)
		// 如果订单已存在且状态不是已取消/已过期，则跳过
		if existingOrder.Status == 1 || existingOrder.Status == 2 {
			// 订单仍然活跃或已成交，跳过
			return nil, nil
		}
		// 如果订单已取消或已过期，说明链上可能重用了 PDA（不应该发生）
		logx.Errorf("[LimitOrder] Order PDA reused after cancellation/expiry: order_pda=%s, old_status=%d", orderPda, existingOrder.Status)
		return nil, fmt.Errorf("order PDA reused: %s", orderPda)
	}

	// Save to database
	order := &limitordermodel.LimitOrder{
		OrderPda:          orderPda,
		MarketId:          marketDb.Id,
		MarketPda:         marketPda,
		Owner:             owner,
		MarginPda:         marginPda,
		OrderId:           int64(marketDb.SeqNum + 1), // seq_num will be incremented on-chain
		Side:              dbSide,
		PriceLots:         int64(priceLots),
		QtyLots:           int64(qtyLots),
		RemainingLots:     int64(qtyLots), // Initially all remaining
		LockedSide:        lockedSide,
		ExpirySlot:        int64(expirySlot),
		SelfTradeBehavior: int64(selfTradeBehavior) + 1, // Chain: 0/1 → DB: 1/2
		PriceDisplay:      priceDisplay,
		QtyDisplay:        qtyDisplay,
		RemainingDisplay:  qtyDisplay,
		TotalValueDisplay: totalValueDisplay,
		Status:            1, // Active
		FilledPercent:     0.0,
		CreateTxHash:      sql.NullString{String: d.dtx.TxHash, Valid: true},
	}

	if minFillBps != nil {
		order.MinFillBps = sql.NullInt64{Int64: int64(*minFillBps), Valid: true}
	}

	err = d.svcCtx.LimitOrderModel.Insert(d.ctx, order)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to insert order: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] place_order saved: tx=%s, order=%s, market=%s, owner=%s, side=%d, price=%.6f, qty=%.6f",
		d.dtx.TxHash, orderPda, marketPda, owner, side, priceDisplay, qtyDisplay)

	if err := d.updateMarginAfterPlaceOrder(marginPda, marketDb, side, priceLots, qtyLots); err != nil {
		logx.Errorf("[LimitOrder] Failed to update margin after place_order: %v", err)
		return nil, err
	}

	// 发布订单簿更新到 Redis 频道 "limit_order_orderbook"
	d.publishOrderBookUpdate(marketPda, marketDb.Id)

	// 发布用户订单更新到 Redis 频道 "user_orders_{owner}"
	d.publishUserOrderUpdate(order, "created")

	return nil, nil
}

func (d *LimitOrderDecoder) handleCancelOrder() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// Log for debugging
	logx.Infof("[LimitOrder] cancel_order accounts count: %d, accounts: %v", len(accounts), accounts)

	// According to IDL, cancel_order has 5 accounts:
	// 0: config, 1: market, 2: margin, 3: order, 4: owner
	if len(accounts) < 5 {
		logx.Errorf("[LimitOrder] cancel_order: insufficient accounts, got %d, need 5", len(accounts))
		logx.Errorf("[LimitOrder] cancel_order: tx=%s, accounts=%v", d.dtx.TxHash, accounts)
		return nil, fmt.Errorf("cancel_order: insufficient accounts, got %d", len(accounts))
	}

	// Parse accounts based on IDL order
	configPda := tx.AccountKeys[accounts[0]].String()
	marketPda := tx.AccountKeys[accounts[1]].String()
	marginPda := tx.AccountKeys[accounts[2]].String()
	orderPda := tx.AccountKeys[accounts[3]].String()
	owner := tx.AccountKeys[accounts[4]].String()

	logx.Infof("[LimitOrder] cancel_order parsed accounts: config=%s, market=%s, margin=%s, order=%s, owner=%s",
		configPda, marketPda, marginPda, orderPda, owner)

	// Find and update order in database
	order, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, orderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to find order %s: %v", orderPda, err)
		return nil, err
	}

	// Update order status to Cancelled
	now := time.Now()
	order.Status = 3 // Cancelled
	order.CancelTxHash = sql.NullString{String: d.dtx.TxHash, Valid: true}
	order.CancelledAt = sql.NullTime{Time: now, Valid: true}

	err = d.svcCtx.LimitOrderModel.Update(d.ctx, order)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update order: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] cancel_order saved: tx=%s, order=%s, market=%s, owner=%s",
		d.dtx.TxHash, orderPda, marketPda, owner)

	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}
	if err := d.unlockMarginForOrder(marginPda, marketDb, order); err != nil {
		logx.Errorf("[LimitOrder] Failed to unlock margin after cancel_order: %v", err)
		return nil, err
	}

	// Publish to Redis for WebSocket broadcast
	d.publishOrderBookUpdate(marketPda, order.MarketId)
	d.publishUserOrderUpdate(order, "cancelled")

	return nil, nil
}

// 解析充值指令，更新保证金账户余额，并发布更新到 Redis 频道 "limit_order_margin"
func (d *LimitOrderDecoder) handleDeposit() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to IDL, deposit has 11 accounts:
	// 0: config, 1: market, 2: margin, 3: owner, 4: base_mint, 5: quote_mint,
	// 6: user_base_token, 7: user_quote_token, 8: base_vault, 9: quote_vault, 10: token_program
	if len(accounts) < 11 {
		logx.Errorf("[LimitOrder] deposit: insufficient accounts, got %d, need 11", len(accounts))
		logx.Errorf("[LimitOrder] deposit: tx=%s, accounts=%v", d.dtx.TxHash, accounts)
		return nil, fmt.Errorf("deposit: insufficient accounts, got %d", len(accounts))
	}

	marginPda := tx.AccountKeys[accounts[2]].String()
	marketPda := tx.AccountKeys[accounts[1]].String()
	owner := tx.AccountKeys[accounts[3]].String()

	logx.Infof("[LimitOrder] deposit parsed accounts: config=%s, market=%s, margin=%s, owner=%s",
		tx.AccountKeys[accounts[0]].String(), marketPda, marginPda, owner)

	// Parse instruction data
	data := d.compiledInstruction.Data[8:] // Skip discriminator
	if len(data) < 9 {
		return nil, fmt.Errorf("deposit: insufficient data")
	}

	// 解析充值数量和方向
	amount := binary.LittleEndian.Uint64(data[0:8])
	side := data[8] // 0=Base, 1=Quote

	// Get market info
	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}

	marketResp, err := d.svcCtx.MarketService.GetLimitOrderMarkets(d.ctx, &marketclient.GetLimitOrderMarketsRequest{
		ChainId:  100000,
		PageNo:   1,
		PageSize: 1000,
	})
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market info: %v", err)
		return nil, err
	}

	var marketItem *marketclient.MarketItem
	for _, m := range marketResp.Markets {
		if m.MarketPda == marketPda {
			marketItem = m
			break
		}
	}

	if marketItem == nil {
		return nil, fmt.Errorf("market not found: %s", marketPda)
	}

	// 从数据库查找保证金账户，如果不存在则创建新账户
	margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, marginPda)
	if err != nil {
		// 创建新保证金账户
		margin = &limitordermodel.LimitOrderMargin{
			MarginPda: marginPda,
			MarketId:  marketDb.Id,
			MarketPda: marketPda,
			Owner:     owner,
			Status:    1, // Normal
		}
		err = d.svcCtx.LimitOrderMarginModel.Insert(d.ctx, margin)
		if err != nil {
			logx.Errorf("[LimitOrder] Failed to create margin account: %v", err)
			return nil, err
		}
	}

	// 根据充值的方向，更新保证金账户余额
	if side == 0 { // Base
		// 充值基础代币，增加保证金账户的基础可用余额
		margin.BaseFree += int64(amount)
		decimals := marketItem.BaseDecimals
		margin.BaseFreeDisplay = float64(margin.BaseFree) / float64(pow10LimitOrder(decimals))
	} else { // Quote
		// 充值报价代币，增加保证金账户的报价可用余额
		margin.QuoteFree += int64(amount)
		decimals := marketItem.QuoteDecimals
		margin.QuoteFreeDisplay = float64(margin.QuoteFree) / float64(pow10LimitOrder(decimals))
	}

	err = d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update margin: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] deposit saved: tx=%s, margin=%s, market=%s, owner=%s, amount=%d, side=%d",
		d.dtx.TxHash, marginPda, marketPda, owner, amount, side)

	// 发布保证金账户更新到 Redis 频道 "limit_order_margin"
	d.publishMarginUpdate(margin)

	return nil, nil
}

// 解析提现指令，更新保证金账户余额，并发布更新到 Redis 频道 "limit_order_margin"
func (d *LimitOrderDecoder) handleWithdraw() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to IDL, withdraw has 9 accounts:
	// 0: config, 1: market, 2: margin, 3: owner, 4: user_base_token, 5: user_quote_token,
	// 6: base_vault, 7: quote_vault, 8: token_program
	if len(accounts) < 9 {
		logx.Errorf("[LimitOrder] withdraw: insufficient accounts, got %d, need 9", len(accounts))
		logx.Errorf("[LimitOrder] withdraw: tx=%s, accounts=%v", d.dtx.TxHash, accounts)
		return nil, fmt.Errorf("withdraw: insufficient accounts, got %d", len(accounts))
	}

	marginPda := tx.AccountKeys[accounts[2]].String()
	marketPda := tx.AccountKeys[accounts[1]].String()
	owner := tx.AccountKeys[accounts[3]].String()

	logx.Infof("[LimitOrder] withdraw parsed accounts: config=%s, market=%s, margin=%s, owner=%s",
		tx.AccountKeys[accounts[0]].String(), marketPda, marginPda, owner)

	// Parse instruction data
	data := d.compiledInstruction.Data[8:] // Skip discriminator
	if len(data) < 9 {
		return nil, fmt.Errorf("withdraw: insufficient data")
	}

	amount := binary.LittleEndian.Uint64(data[0:8])
	side := data[8] // 0=Base, 1=Quote

	// Get market info
	marketResp, err := d.svcCtx.MarketService.GetLimitOrderMarkets(d.ctx, &marketclient.GetLimitOrderMarketsRequest{
		ChainId:  100000,
		PageNo:   1,
		PageSize: 1000,
	})
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market info: %v", err)
		return nil, err
	}

	var marketItem *marketclient.MarketItem
	for _, m := range marketResp.Markets {
		if m.MarketPda == marketPda {
			marketItem = m
			break
		}
	}

	if marketItem == nil {
		return nil, fmt.Errorf("market not found: %s", marketPda)
	}

	// Find margin account
	margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, marginPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Margin account not found: %s", marginPda)
		return nil, err
	}

	// Update balance based on side
	if side == 0 { // Base
		margin.BaseFree -= int64(amount)
		if margin.BaseFree < 0 {
			margin.BaseFree = 0
		}
		decimals := marketItem.BaseDecimals
		margin.BaseFreeDisplay = float64(margin.BaseFree) / float64(pow10LimitOrder(decimals))
	} else { // Quote
		margin.QuoteFree -= int64(amount)
		if margin.QuoteFree < 0 {
			margin.QuoteFree = 0
		}
		decimals := marketItem.QuoteDecimals
		margin.QuoteFreeDisplay = float64(margin.QuoteFree) / float64(pow10LimitOrder(decimals))
	}

	err = d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update margin: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] withdraw saved: tx=%s, margin=%s, market=%s, owner=%s, amount=%d, side=%d",
		d.dtx.TxHash, marginPda, marketPda, owner, amount, side)

	// Publish to Redis
	d.publishMarginUpdate(margin)

	return nil, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pow10LimitOrder(n int32) uint64 {
	result := uint64(1)
	for i := int32(0); i < n; i++ {
		result *= 10
	}
	return result
}

// 每次订单创建、取消、成交后调用，查询该市场前20档买卖挂单聚合数据，构建订单簿消息并发布到 Redis 频道 "limit_order_orderbook"
func (d *LimitOrderDecoder) publishOrderBookUpdate(marketPda string, marketId int64) {
	if d.svcCtx.Redis == nil {
		return
	}

	// 查询该交易市场的未成交挂单，用于拼接构建订单簿
	type OrderBookAgg struct {
		Price      float64
		Qty        float64
		Total      float64
		OrderCount int32
	}

	// 查询买方挂单（交易方向=1，按价格从高到低降序排列）
	var bidAggs []OrderBookAgg
	err := d.svcCtx.DB.WithContext(d.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("price_display as price, SUM(remaining_display) as qty, SUM(remaining_display * price_display) as total, COUNT(*) as order_count").
		Where("market_id = ? AND status = ? AND side = ?", marketId, 1, 1).
		Group("price_display").
		Order("price_display DESC").
		Limit(20).
		Scan(&bidAggs).Error
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to query bid orders: %v", err)
		bidAggs = []OrderBookAgg{}
	}

	// 查询卖方挂单（交易方向=2，按价格从低到高升序排列）
	var askAggs []OrderBookAgg
	err = d.svcCtx.DB.WithContext(d.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("price_display as price, SUM(remaining_display) as qty, SUM(remaining_display * price_display) as total, COUNT(*) as order_count").
		Where("market_id = ? AND status = ? AND side = ?", marketId, 1, 2).
		Group("price_display").
		Order("price_display ASC").
		Limit(20).
		Scan(&askAggs).Error
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to query ask orders: %v", err)
		askAggs = []OrderBookAgg{}
	}

	// 构建买单数组
	bids := make([]map[string]interface{}, 0, len(bidAggs))
	for _, agg := range bidAggs {
		bids = append(bids, map[string]interface{}{
			"price":    fmt.Sprintf("%.6f", agg.Price),
			"quantity": fmt.Sprintf("%.2f", agg.Qty),
			"total":    fmt.Sprintf("%.6f", agg.Total),
		})
	}

	// 构建卖单数组
	asks := make([]map[string]interface{}, 0, len(askAggs))
	for _, agg := range askAggs {
		asks = append(asks, map[string]interface{}{
			"price":    fmt.Sprintf("%.6f", agg.Price),
			"quantity": fmt.Sprintf("%.2f", agg.Qty),
			"total":    fmt.Sprintf("%.6f", agg.Total),
		})
	}

	message := map[string]interface{}{
		"chain_id":   100000,
		"market_pda": marketPda,
		"timestamp":  time.Now().Unix(),
		"bids":       bids,
		"asks":       asks,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to marshal orderbook update: %v", err)
		return
	}

	// 发布到 Redis 频道 "limit_order_orderbook"
	_, err = d.svcCtx.Redis.Publish("limit_order_orderbook", string(data))
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to publish orderbook update: %v", err)
	} else {
		logx.Infof("[LimitOrder] Published orderbook update: market=%s, bids=%d, asks=%d", marketPda, len(bids), len(asks))
	}
}

// 发布用户订单更新到 Redis 频道 "limit_order_user_order"，
// 消息包含订单详情和更新类型（创建、取消、成交等），供 WebSocket 服务器推送给用户
func (d *LimitOrderDecoder) publishUserOrderUpdate(order *limitordermodel.LimitOrder, action string) {
	if d.svcCtx.Redis == nil {
		return
	}

	message := map[string]interface{}{
		"chain_id":            100000,
		"market_pda":          order.MarketPda,
		"market_id":           order.MarketId,
		"order_pda":           order.OrderPda,
		"user_wallet_address": order.Owner,
		"action":              action, // "created", "cancelled", "filled"
		"order": map[string]interface{}{
			"order_id":       order.OrderId,
			"side":           order.Side,
			"price_display":  order.PriceDisplay,
			"qty_display":    order.QtyDisplay,
			"remaining":      order.RemainingDisplay,
			"status":         order.Status,
			"filled_percent": order.FilledPercent,
			"expiry_slot":    order.ExpirySlot,
		},
		"timestamp": time.Now().Unix(),
	}

	data, err := json.Marshal(message)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to marshal user order update: %v", err)
		return
	}

	_, err = d.svcCtx.Redis.Publish("limit_order_user_order", string(data))
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to publish user order update: %v", err)
	} else {
		logx.Infof("[LimitOrder] Published user order update: order=%s, action=%s", order.OrderPda, action)
	}
}

// 发布保证金账户更新到 Redis 频道 "limit_order_margin"
func (d *LimitOrderDecoder) publishMarginUpdate(margin *limitordermodel.LimitOrderMargin) {
	if d.svcCtx.Redis == nil {
		return
	}

	var baseSymbol string
	var quoteSymbol string
	market, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, margin.MarketPda)
	if err == nil && market != nil {
		baseSymbol = market.BaseSymbol
		quoteSymbol = market.QuoteSymbol
	}

	baseTotal := margin.BaseFreeDisplay + margin.BaseLockedDisplay
	quoteTotal := margin.QuoteFreeDisplay + margin.QuoteLockedDisplay

	message := limitorderpkg.MarginUpdate{
		ChainId:           100000,
		MarketPda:         margin.MarketPda,
		UserWalletAddress: margin.Owner,
		MarginPda:         margin.MarginPda,
		Status:            margin.Status,
		BaseFree:          fmt.Sprintf("%.8f", margin.BaseFreeDisplay),
		BaseLocked:        fmt.Sprintf("%.8f", margin.BaseLockedDisplay),
		BaseTotal:         fmt.Sprintf("%.8f", baseTotal),
		QuoteFree:         fmt.Sprintf("%.8f", margin.QuoteFreeDisplay),
		QuoteLocked:       fmt.Sprintf("%.8f", margin.QuoteLockedDisplay),
		QuoteTotal:        fmt.Sprintf("%.8f", quoteTotal),
		BaseSymbol:        baseSymbol,
		QuoteSymbol:       quoteSymbol,
		LastSyncSlot:      margin.LastSyncSlot,
		Timestamp:         time.Now().Unix(),
	}

	data, err := json.Marshal(message)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to marshal margin update: %v", err)
		return
	}

	_, err = d.svcCtx.Redis.Publish(limitorderpkg.ChannelMargin, string(data))
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to publish margin update: %v", err)
	} else {
		logx.Infof("[LimitOrder] Published margin update: margin=%s", margin.MarginPda)
	}
}

func (d *LimitOrderDecoder) updateMarginAfterPlaceOrder(marginPda string, marketDb *limitordermodel.LimitOrderMarket, side uint8, priceLots uint64, qtyLots uint64) error {
	margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, marginPda)
	if err != nil {
		return fmt.Errorf("margin not found: %s", marginPda)
	}

	priceLots64, err := safeUint64ToInt64(priceLots)
	if err != nil {
		return err
	}
	qtyLots64, err := safeUint64ToInt64(qtyLots)
	if err != nil {
		return err
	}

	lockedSide := int64(2) // Quote
	if side == 1 {
		lockedSide = 1 // Base
	}

	if err := applyMarginLock(margin, lockedSide, priceLots64, qtyLots64); err != nil {
		return err
	}

	updateMarginDisplay(margin, marketDb.BaseDecimals, marketDb.QuoteDecimals)

	if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin); err != nil {
		return err
	}

	d.publishMarginUpdate(margin)
	return nil
}

func (d *LimitOrderDecoder) unlockMarginForOrder(marginPda string, marketDb *limitordermodel.LimitOrderMarket, order *limitordermodel.LimitOrder) error {
	margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, marginPda)
	if err != nil {
		return fmt.Errorf("margin not found: %s", marginPda)
	}

	if err := applyMarginUnlock(margin, order.LockedSide, order.PriceLots, order.RemainingLots); err != nil {
		return err
	}

	updateMarginDisplay(margin, marketDb.BaseDecimals, marketDb.QuoteDecimals)

	if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin); err != nil {
		return err
	}

	d.publishMarginUpdate(margin)
	return nil
}

func updateMarginDisplay(margin *limitordermodel.LimitOrderMargin, baseDecimals, quoteDecimals int64) {
	baseDiv := float64(pow10LimitOrder(int32(baseDecimals)))
	quoteDiv := float64(pow10LimitOrder(int32(quoteDecimals)))

	if baseDiv > 0 {
		margin.BaseFreeDisplay = float64(margin.BaseFree) / baseDiv
		margin.BaseLockedDisplay = float64(margin.BaseLocked) / baseDiv
	}
	if quoteDiv > 0 {
		margin.QuoteFreeDisplay = float64(margin.QuoteFree) / quoteDiv
		margin.QuoteLockedDisplay = float64(margin.QuoteLocked) / quoteDiv
	}
}

func applyMarginLock(margin *limitordermodel.LimitOrderMargin, lockedSide int64, priceLots, qtyLots int64) error {
	if qtyLots < 0 || priceLots < 0 {
		return fmt.Errorf("invalid lots: price=%d qty=%d", priceLots, qtyLots)
	}

	if lockedSide == 1 {
		margin.BaseFree -= qtyLots
		if margin.BaseFree < 0 {
			margin.BaseFree = 0
		}
		margin.BaseLocked += qtyLots
		return nil
	}

	quoteAmount, err := calcQuoteAmount(priceLots, qtyLots)
	if err != nil {
		return err
	}
	margin.QuoteFree -= quoteAmount
	if margin.QuoteFree < 0 {
		margin.QuoteFree = 0
	}
	margin.QuoteLocked += quoteAmount
	return nil
}

func applyMarginUnlock(margin *limitordermodel.LimitOrderMargin, lockedSide int64, priceLots, qtyLots int64) error {
	if qtyLots < 0 || priceLots < 0 {
		return fmt.Errorf("invalid lots: price=%d qty=%d", priceLots, qtyLots)
	}

	if lockedSide == 1 {
		margin.BaseLocked -= qtyLots
		if margin.BaseLocked < 0 {
			margin.BaseLocked = 0
		}
		margin.BaseFree += qtyLots
		return nil
	}

	quoteAmount, err := calcQuoteAmount(priceLots, qtyLots)
	if err != nil {
		return err
	}
	margin.QuoteLocked -= quoteAmount
	if margin.QuoteLocked < 0 {
		margin.QuoteLocked = 0
	}
	margin.QuoteFree += quoteAmount
	return nil
}

func calcQuoteAmount(priceLots, qtyLots int64) (int64, error) {
	if priceLots == 0 || qtyLots == 0 {
		return 0, nil
	}
	if priceLots > math.MaxInt64/qtyLots {
		return 0, fmt.Errorf("quote amount overflow: price=%d qty=%d", priceLots, qtyLots)
	}
	return priceLots * qtyLots, nil
}

func safeUint64ToInt64(val uint64) (int64, error) {
	if val > math.MaxInt64 {
		return 0, fmt.Errorf("value overflows int64: %d", val)
	}
	return int64(val), nil
}

func (d *LimitOrderDecoder) updateMarginsAfterMatch(marketDb *limitordermodel.LimitOrderMarket, makerOrder, takerOrder *limitordermodel.LimitOrder, matchQtyLots int64, makerMarginPda, takerMarginPda string) error {
	if matchQtyLots <= 0 {
		return nil
	}
	if makerMarginPda == "" || takerMarginPda == "" {
		return fmt.Errorf("missing margin pda")
	}
	if makerMarginPda == takerMarginPda {
		return fmt.Errorf("maker and taker margin are the same")
	}

	quoteAmount, err := calcQuoteAmount(makerOrder.PriceLots, matchQtyLots)
	if err != nil {
		return err
	}
	fee, err := calcTakerFee(quoteAmount, marketDb.TakerFeeBps)
	if err != nil {
		return err
	}
	quoteNet := quoteAmount - fee
	if quoteNet < 0 {
		quoteNet = 0
	}

	makerMargin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, makerMarginPda)
	if err != nil {
		return fmt.Errorf("maker margin not found: %s", makerMarginPda)
	}
	takerMargin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, takerMarginPda)
	if err != nil {
		return fmt.Errorf("taker margin not found: %s", takerMarginPda)
	}

	if takerOrder.Side == 1 {
		makerMargin.BaseLocked -= matchQtyLots
		if makerMargin.BaseLocked < 0 {
			makerMargin.BaseLocked = 0
		}
		takerMargin.QuoteLocked -= quoteAmount
		if takerMargin.QuoteLocked < 0 {
			takerMargin.QuoteLocked = 0
		}
		takerMargin.BaseFree += matchQtyLots
		makerMargin.QuoteFree += quoteNet
	} else {
		takerMargin.BaseLocked -= matchQtyLots
		if takerMargin.BaseLocked < 0 {
			takerMargin.BaseLocked = 0
		}
		makerMargin.QuoteLocked -= quoteAmount
		if makerMargin.QuoteLocked < 0 {
			makerMargin.QuoteLocked = 0
		}
		makerMargin.BaseFree += matchQtyLots
		takerMargin.QuoteFree += quoteNet
	}

	updateMarginDisplay(makerMargin, marketDb.BaseDecimals, marketDb.QuoteDecimals)
	updateMarginDisplay(takerMargin, marketDb.BaseDecimals, marketDb.QuoteDecimals)

	if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, makerMargin); err != nil {
		return err
	}
	if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, takerMargin); err != nil {
		return err
	}

	d.publishMarginUpdate(makerMargin)
	d.publishMarginUpdate(takerMargin)
	return nil
}

func calcTakerFee(quoteAmount int64, takerFeeBps int64) (int64, error) {
	if quoteAmount <= 0 || takerFeeBps <= 0 {
		return 0, nil
	}
	if quoteAmount > math.MaxInt64/takerFeeBps {
		return 0, fmt.Errorf("fee overflow: quote=%d bps=%d", quoteAmount, takerFeeBps)
	}
	numerator := quoteAmount * takerFeeBps
	fee := (numerator + 10000 - 1) / 10000
	return fee, nil
}

// handleMatchOrders processes order matching (trade execution)
func (d *LimitOrderDecoder) handleMatchOrders() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to IDL, match_orders has 8 accounts:
	// 0: config, 1: market, 2: maker_margin, 3: taker_margin, 4: maker_order, 5: taker_order, 6: pyth_price_feed, 7: caller
	if len(accounts) < 8 {
		return nil, fmt.Errorf("match_orders: insufficient accounts, got %d", len(accounts))
	}

	marketPda := tx.AccountKeys[accounts[1]].String()
	makerMarginPda := tx.AccountKeys[accounts[2]].String()
	takerMarginPda := tx.AccountKeys[accounts[3]].String()
	makerOrderPda := tx.AccountKeys[accounts[4]].String()
	takerOrderPda := tx.AccountKeys[accounts[5]].String()

	// Parse instruction data: discriminator(8) + MatchParams
	// MatchParams: match_qty_lots(8)
	data := d.compiledInstruction.Data[8:]
	if len(data) < 8 {
		return nil, fmt.Errorf("match_orders: insufficient data")
	}

	matchQtyLots := binary.LittleEndian.Uint64(data[0:8])

	// Update maker order
	makerOrder, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, makerOrderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to find maker order %s: %v", makerOrderPda, err)
		return nil, err
	}

	// Update taker order
	takerOrder, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, takerOrderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to find taker order %s: %v", takerOrderPda, err)
		return nil, err
	}

	// Get market info for calculations
	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}

	baseMultiplier := float64(pow10LimitOrder(int32(marketDb.BaseDecimals)))
	matchQtyDisplay := float64(matchQtyLots) / baseMultiplier
	matchQty64, err := safeUint64ToInt64(matchQtyLots)
	if err != nil {
		return nil, err
	}

	if makerOrder.Owner == takerOrder.Owner {
		if matchQty64 > takerOrder.RemainingLots {
			matchQty64 = takerOrder.RemainingLots
		}

		switch takerOrder.SelfTradeBehavior {
		case 1: // DecrementTake
			takerOrder.RemainingLots -= matchQty64
			if takerOrder.RemainingLots < 0 {
				takerOrder.RemainingLots = 0
			}
			takerOrder.RemainingDisplay = float64(takerOrder.RemainingLots) / baseMultiplier
			if takerOrder.QtyLots > 0 {
				takerOrder.FilledPercent = float64(takerOrder.QtyLots-takerOrder.RemainingLots) / float64(takerOrder.QtyLots) * 100
			}
			if takerOrder.RemainingLots == 0 {
				takerOrder.Status = 2 // Filled
				now := time.Now()
				takerOrder.FilledAt = sql.NullTime{Time: now, Valid: true}
			}

			if err := d.svcCtx.LimitOrderModel.Update(d.ctx, takerOrder); err != nil {
				logx.Errorf("[LimitOrder] Failed to update taker order: %v", err)
				return nil, err
			}

			margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, takerMarginPda)
			if err != nil {
				logx.Errorf("[LimitOrder] Failed to find taker margin %s: %v", takerMarginPda, err)
				return nil, err
			}
			if err := applyMarginUnlock(margin, takerOrder.LockedSide, takerOrder.PriceLots, matchQty64); err != nil {
				logx.Errorf("[LimitOrder] Failed to unlock margin for self-trade: %v", err)
				return nil, err
			}
			updateMarginDisplay(margin, marketDb.BaseDecimals, marketDb.QuoteDecimals)
			if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin); err != nil {
				logx.Errorf("[LimitOrder] Failed to update margin: %v", err)
				return nil, err
			}
			d.publishMarginUpdate(margin)

			logx.Infof("[LimitOrder] match_orders self-trade decrement: tx=%s, market=%s, order=%s, qty=%.6f",
				d.dtx.TxHash, marketPda, takerOrderPda, matchQtyDisplay)

			d.publishOrderBookUpdate(marketPda, marketDb.Id)
			d.publishUserOrderUpdate(takerOrder, "filled")
			return nil, nil

		case 2: // CancelNew
			now := time.Now()
			takerOrder.Status = 3 // Cancelled
			takerOrder.CancelTxHash = sql.NullString{String: d.dtx.TxHash, Valid: true}
			takerOrder.CancelledAt = sql.NullTime{Time: now, Valid: true}
			if err := d.svcCtx.LimitOrderModel.Update(d.ctx, takerOrder); err != nil {
				logx.Errorf("[LimitOrder] Failed to update taker order: %v", err)
				return nil, err
			}

			margin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, takerMarginPda)
			if err != nil {
				logx.Errorf("[LimitOrder] Failed to find taker margin %s: %v", takerMarginPda, err)
				return nil, err
			}
			if err := applyMarginUnlock(margin, takerOrder.LockedSide, takerOrder.PriceLots, takerOrder.RemainingLots); err != nil {
				logx.Errorf("[LimitOrder] Failed to unlock margin for self-trade cancel: %v", err)
				return nil, err
			}
			updateMarginDisplay(margin, marketDb.BaseDecimals, marketDb.QuoteDecimals)
			if err := d.svcCtx.LimitOrderMarginModel.Update(d.ctx, margin); err != nil {
				logx.Errorf("[LimitOrder] Failed to update margin: %v", err)
				return nil, err
			}
			d.publishMarginUpdate(margin)

			logx.Infof("[LimitOrder] match_orders self-trade cancel: tx=%s, market=%s, order=%s",
				d.dtx.TxHash, marketPda, takerOrderPda)

			d.publishOrderBookUpdate(marketPda, marketDb.Id)
			d.publishUserOrderUpdate(takerOrder, "cancelled")
			return nil, nil
		}
	}

	// Update remaining lots for both orders
	makerOrder.RemainingLots -= matchQty64
	if makerOrder.RemainingLots < 0 {
		makerOrder.RemainingLots = 0
	}
	makerOrder.RemainingDisplay = float64(makerOrder.RemainingLots) / baseMultiplier
	if makerOrder.QtyLots > 0 {
		makerOrder.FilledPercent = float64(makerOrder.QtyLots-makerOrder.RemainingLots) / float64(makerOrder.QtyLots) * 100
	}

	if makerOrder.RemainingLots == 0 {
		makerOrder.Status = 2 // Filled
		now := time.Now()
		makerOrder.FilledAt = sql.NullTime{Time: now, Valid: true}
	}

	takerOrder.RemainingLots -= matchQty64
	if takerOrder.RemainingLots < 0 {
		takerOrder.RemainingLots = 0
	}
	takerOrder.RemainingDisplay = float64(takerOrder.RemainingLots) / baseMultiplier
	if takerOrder.QtyLots > 0 {
		takerOrder.FilledPercent = float64(takerOrder.QtyLots-takerOrder.RemainingLots) / float64(takerOrder.QtyLots) * 100
	}

	if takerOrder.RemainingLots == 0 {
		takerOrder.Status = 2 // Filled
		now := time.Now()
		takerOrder.FilledAt = sql.NullTime{Time: now, Valid: true}
	}

	// Update both orders in database
	err = d.svcCtx.LimitOrderModel.Update(d.ctx, makerOrder)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update maker order: %v", err)
		return nil, err
	}

	err = d.svcCtx.LimitOrderModel.Update(d.ctx, takerOrder)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update taker order: %v", err)
		return nil, err
	}

	if err := d.updateMarginsAfterMatch(marketDb, makerOrder, takerOrder, matchQty64, makerMarginPda, takerMarginPda); err != nil {
		logx.Errorf("[LimitOrder] Failed to update margins after match_orders: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] match_orders saved: tx=%s, market=%s, maker=%s, taker=%s, qty=%.6f",
		d.dtx.TxHash, marketPda, makerOrderPda, takerOrderPda, matchQtyDisplay)

	// Publish updates
	d.publishOrderBookUpdate(marketPda, marketDb.Id)
	d.publishUserOrderUpdate(makerOrder, "filled")
	d.publishUserOrderUpdate(takerOrder, "filled")

	return nil, nil
}

// 解析创建保证金账户指令
func (d *LimitOrderDecoder) handleCreateMargin() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to updated IDL, create_margin now has 11 accounts:
	// 0: config, 1: market, 2: margin, 3: owner, 4: base_mint, 5: quote_mint,
	// 6: user_base_token, 7: user_quote_token, 8: token_program, 9: associated_token_program, 10: system_program
	if len(accounts) < 11 {
		// Try old format (5 accounts) for backward compatibility
		if len(accounts) >= 5 {
			logx.Infof("[LimitOrder] create_margin: using old format with %d accounts", len(accounts))
			marketPda := tx.AccountKeys[accounts[1]].String()
			marginPda := tx.AccountKeys[accounts[2]].String()
			owner := tx.AccountKeys[accounts[3]].String()

			return d.createMarginAccount(marketPda, marginPda, owner)
		}
		return nil, fmt.Errorf("create_margin: insufficient accounts, got %d", len(accounts))
	}

	// New format with ATA creation
	marketPda := tx.AccountKeys[accounts[1]].String()
	marginPda := tx.AccountKeys[accounts[2]].String()
	owner := tx.AccountKeys[accounts[3]].String()

	logx.Infof("[LimitOrder] create_margin (new format): market=%s, margin=%s, owner=%s", marketPda, marginPda, owner)

	return d.createMarginAccount(marketPda, marginPda, owner)
}

// createMarginAccount is a helper function to create margin account
func (d *LimitOrderDecoder) createMarginAccount(marketPda, marginPda, owner string) (*types.TradeWithPair, error) {
	// Get market from database
	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}

	// Check if margin account already exists
	existingMargin, err := d.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(d.ctx, marginPda)
	if err == nil && existingMargin != nil {
		logx.Infof("[LimitOrder] Margin account already exists: %s, skipping", marginPda)
		return nil, nil
	}

	// Create margin account
	margin := &limitordermodel.LimitOrderMargin{
		MarginPda:  marginPda,
		MarketId:   marketDb.Id,
		MarketPda:  marketPda,
		Owner:      owner,
		Status:     1, // Normal
		InitTxHash: sql.NullString{String: d.dtx.TxHash, Valid: true},
	}

	err = d.svcCtx.LimitOrderMarginModel.Insert(d.ctx, margin)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to create margin account: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] create_margin saved: tx=%s, margin=%s, market=%s, owner=%s",
		d.dtx.TxHash, marginPda, marketPda, owner)

	// Publish margin update
	d.publishMarginUpdate(margin)

	return nil, nil
}

// handleEmergencyClose processes emergency order closure
func (d *LimitOrderDecoder) handleEmergencyClose() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	if len(accounts) < 5 {
		return nil, fmt.Errorf("emergency_close: insufficient accounts")
	}

	marketPda := tx.AccountKeys[accounts[1]].String()
	marginPda := tx.AccountKeys[accounts[2]].String()
	orderPda := tx.AccountKeys[accounts[3]].String()
	owner := tx.AccountKeys[accounts[4]].String()

	// Find and update order
	order, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, orderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to find order %s: %v", orderPda, err)
		return nil, err
	}

	// Update order status to Cancelled (emergency)
	now := time.Now()
	order.Status = 3 // Cancelled
	order.CancelTxHash = sql.NullString{String: d.dtx.TxHash, Valid: true}
	order.CancelledAt = sql.NullTime{Time: now, Valid: true}

	err = d.svcCtx.LimitOrderModel.Update(d.ctx, order)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update order: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] emergency_close saved: tx=%s, order=%s, owner=%s", d.dtx.TxHash, orderPda, owner)

	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}
	if err := d.unlockMarginForOrder(marginPda, marketDb, order); err != nil {
		logx.Errorf("[LimitOrder] Failed to unlock margin after emergency_close: %v", err)
		return nil, err
	}

	// Publish updates
	d.publishOrderBookUpdate(order.MarketPda, order.MarketId)
	d.publishUserOrderUpdate(order, "cancelled")

	return nil, nil
}

// handleSweepExpired processes expired order cleanup
func (d *LimitOrderDecoder) handleSweepExpired() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	if len(accounts) < 4 {
		return nil, fmt.Errorf("sweep_expired: insufficient accounts")
	}

	marketPda := tx.AccountKeys[accounts[1]].String()
	marginPda := tx.AccountKeys[accounts[2]].String()
	orderPda := tx.AccountKeys[accounts[3]].String()

	// Find and update order
	order, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, orderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to find order %s: %v", orderPda, err)
		return nil, err
	}

	// Update order status to Expired
	now := time.Now()
	order.Status = 4 // Expired
	order.ExpiredAt = sql.NullTime{Time: now, Valid: true}

	err = d.svcCtx.LimitOrderModel.Update(d.ctx, order)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to update order: %v", err)
		return nil, err
	}

	logx.Infof("[LimitOrder] sweep_expired saved: tx=%s, order=%s", d.dtx.TxHash, orderPda)

	marketDb, err := d.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(d.ctx, marketPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Failed to get market from DB: %v", err)
		return nil, err
	}
	if err := d.unlockMarginForOrder(marginPda, marketDb, order); err != nil {
		logx.Errorf("[LimitOrder] Failed to unlock margin after sweep_expired: %v", err)
		return nil, err
	}

	// Publish updates
	d.publishOrderBookUpdate(order.MarketPda, order.MarketId)
	d.publishUserOrderUpdate(order, "expired")

	return nil, nil
}

// handleCloseOrder processes order closure (rent reclaim)
func (d *LimitOrderDecoder) handleCloseOrder() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to IDL, close_order has 2 accounts:
	// 0: order, 1: owner
	if len(accounts) < 2 {
		return nil, fmt.Errorf("close_order: insufficient accounts, got %d", len(accounts))
	}

	orderPda := tx.AccountKeys[accounts[0]].String()
	owner := tx.AccountKeys[accounts[1]].String()

	logx.Infof("[LimitOrder] close_order: order=%s, owner=%s", orderPda, owner)

	// Note: The order account is closed on-chain, so we don't need to update database
	// The order should already be in a non-active state (Filled/Cancelled/Expired)
	// We can optionally mark it as "closed" in database if needed

	order, err := d.svcCtx.LimitOrderModel.FindOneByOrderPda(d.ctx, orderPda)
	if err != nil {
		logx.Errorf("[LimitOrder] Order not found for close: %s", orderPda)
		// Not an error - order might have been cleaned up already
		return nil, nil
	}

	logx.Infof("[LimitOrder] close_order processed: tx=%s, order=%s, status=%d",
		d.dtx.TxHash, orderPda, order.Status)

	return nil, nil
}

// handleUpdateCustomPrice processes custom price updates
func (d *LimitOrderDecoder) handleUpdateCustomPrice() (*types.TradeWithPair, error) {
	tx := d.dtx.Tx
	accounts := d.compiledInstruction.Accounts

	// According to IDL, update_custom_price has 3 accounts:
	// 0: config, 1: admin, 2: market
	if len(accounts) < 3 {
		return nil, fmt.Errorf("update_custom_price: insufficient accounts, got %d", len(accounts))
	}

	marketPda := tx.AccountKeys[accounts[2]].String()

	// Parse instruction data: discriminator(8) + price(i64) + exponent(i32) + conf(u64)
	data := d.compiledInstruction.Data[8:]
	if len(data) < 20 {
		return nil, fmt.Errorf("update_custom_price: insufficient data, got %d bytes", len(data))
	}

	price := int64(binary.LittleEndian.Uint64(data[0:8]))
	exponent := int32(binary.LittleEndian.Uint32(data[8:12]))
	conf := binary.LittleEndian.Uint64(data[12:20])

	logx.Infof("[LimitOrder] update_custom_price: market=%s, price=%d, exponent=%d, conf=%d",
		marketPda, price, exponent, conf)

	// Note: Custom price is stored on-chain in the Market account
	// We don't need to store it separately in our database
	// The market service will fetch it from on-chain when needed

	return nil, nil
}
