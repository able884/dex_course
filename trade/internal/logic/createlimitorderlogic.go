package logic

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/limitorder"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type CreateLimitOrderLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateLimitOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLimitOrderLogic {
	return &CreateLimitOrderLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateLimitOrderLogic) CreateLimitOrder(in *trade.CreateLimitOrderRequest) (*trade.CreateLimitOrderResponse, error) {
	// 1. 验证参数
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. 构建交易
	txBase64, orderPda, orderId, err := l.buildCreateOrderTx(in)
	if err != nil {
		return nil, err
	}

	return &trade.CreateLimitOrderResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
		OrderPda:  orderPda,
		OrderId:   orderId,
	}, nil
}

func (l *CreateLimitOrderLogic) validate(in *trade.CreateLimitOrderRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if in.ChainId == 0 {
		in.ChainId = constants.SolChainIdInt
	}
	if strings.TrimSpace(in.MarketPda) == "" {
		return errors.New("market_pda is required")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address is required")
	}
	if in.Side != 1 && in.Side != 2 {
		return errors.New("side must be 1 (bid/buy) or 2 (ask/sell)")
	}

	// 验证价格
	priceDecimal, err := decimal.NewFromString(strings.TrimSpace(in.Price))
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}
	if priceDecimal.Cmp(decimal.Zero) <= 0 {
		return errors.New("price must be greater than zero")
	}

	// 验证数量
	qtyDecimal, err := decimal.NewFromString(strings.TrimSpace(in.Quantity))
	if err != nil {
		return fmt.Errorf("invalid quantity: %w", err)
	}
	if qtyDecimal.Cmp(decimal.Zero) <= 0 {
		return errors.New("quantity must be greater than zero")
	}

	// 验证 self_trade_behavior
	if in.SelfTradeBehavior != 1 && in.SelfTradeBehavior != 2 {
		in.SelfTradeBehavior = 1 // Default to DecrementTake
	}

	return nil
}

func (l *CreateLimitOrderLogic) buildCreateOrderTx(in *trade.CreateLimitOrderRequest) (string, string, int64, error) {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", "", 0, errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	// 解析地址
	marketPda, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.MarketPda))
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid market_pda: %w", err)
	}

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid user_wallet_address: %w", err)
	}

	// 查询市场信息
	marketInfo, err := l.getMarketInfo(in.MarketPda)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get market info: %w", err)
	}

	// 解析 mint 地址
	baseMint, err := aSDK.PublicKeyFromBase58(marketInfo.BaseMint)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid base_mint in market: %w", err)
	}

	quoteMint, err := aSDK.PublicKeyFromBase58(marketInfo.QuoteMint)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid quote_mint in market: %w", err)
	}

	// 计算 lots（使用 decimal 避免浮点误差）
	priceDec, err := decimal.NewFromString(strings.TrimSpace(in.Price))
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid price: %w", err)
	}
	qtyDec, err := decimal.NewFromString(strings.TrimSpace(in.Quantity))
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid quantity: %w", err)
	}

	// 正确的 lots 计算：
	// price_lots 表示每个 min_base_lot 需要多少 quote token 的最小单位
	// qty_lots 表示有多少个 min_base_lot
	// quote_needed = price_lots * qty_lots 应该等于总共需要的 quote token 最小单位数

	// 计算 base 和 quote 的 multiplier
	baseMultiplier := decimal.NewFromInt(int64(pow10(int32(marketInfo.BaseDecimals))))
	quoteMultiplier := decimal.NewFromInt(int64(pow10(int32(marketInfo.QuoteDecimals))))
	minBaseLot := decimal.NewFromInt(marketInfo.MinBaseLot)

	// qty_lots = qty_display * base_multiplier, then round down to min_base_lot
	qtyLotsDec := qtyDec.Mul(baseMultiplier).Div(minBaseLot).Floor().Mul(minBaseLot)
	if qtyLotsDec.LessThan(minBaseLot) {
		return "", "", 0, fmt.Errorf("quantity too small for min_base_lot")
	}
	qtyLots := uint64(qtyLotsDec.IntPart())

	// price_lots = price_display * quote_multiplier / base_multiplier
	priceLotsDec := priceDec.Mul(quoteMultiplier).Div(baseMultiplier)

	// Align price_lots to tick_size to avoid InvalidTick on-chain
	tickSizeDec := decimal.NewFromInt(marketInfo.TickSize)
	if marketInfo.TickSize > 0 {
		quotient := priceLotsDec.Div(tickSizeDec)
		if in.Side == 1 {
			// Buy: round down to avoid exceeding limit price
			quotient = quotient.Floor()
		} else {
			// Sell: round up to avoid undercutting limit price
			quotient = quotient.Ceil()
		}
		priceLotsDec = quotient.Mul(tickSizeDec)
	}
	if priceLotsDec.LessThanOrEqual(decimal.Zero) {
		return "", "", 0, fmt.Errorf("price too small for tick_size")
	}
	priceLots := uint64(priceLotsDec.IntPart())

	// 计算需要的 quote 金额（用于买单）
	quoteNeeded := priceLots * qtyLots

	priceFloat, _ := priceDec.Float64()
	qtyFloat, _ := qtyDec.Float64()
	quoteMultiplierFloat, _ := quoteMultiplier.Float64()

	logx.Infof("[CreateLimitOrder] Market config - base_decimals: %d, quote_decimals: %d, tick_size: %d, min_base_lot: %d",
		marketInfo.BaseDecimals, marketInfo.QuoteDecimals, marketInfo.TickSize, marketInfo.MinBaseLot)
	logx.Infof("[CreateLimitOrder] Order calculation - price: %.6f, qty: %.6f, base_multiplier: %s, quote_multiplier: %s",
		priceFloat, qtyFloat, baseMultiplier.String(), quoteMultiplier.String())
	logx.Infof("[CreateLimitOrder] Lots calculation - price_lots: %d, qty_lots: %d, quote_needed: %d (%.6f quote tokens)",
		priceLots, qtyLots, quoteNeeded, float64(quoteNeeded)/quoteMultiplierFloat)

	currentSlot, err := rpcClient.GetSlot(l.ctx, ag_rpc.CommitmentConfirmed)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get current slot: %w", err)
	}

	// 处理过期时间：
	// - 如果 ExpirySlot <= 0，表示永不过期，设置为 0（链上和撮合引擎都会识别为永不过期）
	// - 如果 ExpirySlot > 0，必须大于当前 slot
	expirySlot := uint64(0) // 默认永不过期
	if in.ExpirySlot > 0 {
		expirySlot = uint64(in.ExpirySlot)
		if expirySlot <= currentSlot {
			return "", "", 0, fmt.Errorf("expiry_slot must be greater than current slot: %d <= %d", expirySlot, currentSlot)
		}
	}

	// Read market account from chain to get the latest seq_num.
	// Use confirmed commitment to avoid stale data.
	marketAccountInfo, err := rpcClient.GetAccountInfoWithOpts(
		l.ctx,
		marketPda,
		&ag_rpc.GetAccountInfoOpts{
			Commitment: ag_rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get market account from chain: %w", err)
	}
	if marketAccountInfo == nil || marketAccountInfo.Value == nil {
		return "", "", 0, fmt.Errorf("market account not found on chain")
	}

	// 解析 market 账户数据以获取 seq_num
	// Market 账户结构: discriminator(8) + base_mint(32) + quote_mint(32) + base_vault(32) + quote_vault(32) +
	//                  tick_size(8) + min_base_lot(8) + min_quote_lot(8) + maker_fee_bps(2) + taker_fee_bps(2) +
	//                  fee_accumulator(8) + seq_num(8) + ...
	marketData := marketAccountInfo.Value.Data.GetBinary()
	if len(marketData) < 8+32+32+32+32+8+8+8+2+2+8+8 {
		return "", "", 0, fmt.Errorf("invalid market account data length: %d", len(marketData))
	}

	// seq_num 位于偏移量: 8 + 32*4 + 8*3 + 2*2 + 8 = 8 + 128 + 24 + 4 + 8 = 172
	seqNumOffset := 8 + 32*4 + 8*3 + 2*2 + 8
	seqNumBytes := marketData[seqNumOffset : seqNumOffset+8]
	chainSeqNum := binary.LittleEndian.Uint64(seqNumBytes)

	logx.Infof("[CreateLimitOrder] Market seq_num from chain: %d, from DB: %d", chainSeqNum, marketInfo.SeqNum)

	// 派生 PDAs（使用链上 seq_num + 1 作为 order_id）
	orderSeqNum := chainSeqNum + 1
	config, _, margin, order, err := limitorder.DerivePDAs(
		baseMint,
		quoteMint,
		userWallet,
		orderSeqNum,
	)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to derive PDAs: %w", err)
	}

	logx.Infof("[CreateLimitOrder] Derived PDAs - config: %s, market: %s, margin: %s, order: %s, orderSeqNum: %d",
		config.String(), marketPda.String(), margin.String(), order.String(), orderSeqNum)

	// 构建 PlaceOrderParams
	var minFillBps *uint16
	if in.MinFillBps > 0 {
		minFillBps = &[]uint16{uint16(in.MinFillBps)}[0]
	}

	// 转换 Side: API 使用 1=Buy/2=Sell, Solana 使用 0=Bid/1=Ask
	var side limitorder.Side
	if in.Side == 1 {
		side = limitorder.SideBid // 0
	} else {
		side = limitorder.SideAsk // 1
	}

	// 转换 SelfTradeBehavior: API 使用 1/2, Solana 使用 0/1
	var selfTradeBehavior limitorder.SelfTradeBehavior
	if in.SelfTradeBehavior == 1 {
		selfTradeBehavior = limitorder.SelfTradeDecrementTake // 0
	} else {
		selfTradeBehavior = limitorder.SelfTradeCancelNew // 1
	}

	params := limitorder.PlaceOrderParams{
		Side:              side,
		PriceLots:         priceLots,
		QtyLots:           qtyLots,
		ExpirySlot:        expirySlot,
		MinFillBps:        minFillBps,
		SelfTradeBehavior: selfTradeBehavior,
	}

	// Whitelist proof（空数组表示不需要白名单）
	whitelistProof := make([][32]byte, 0)

	// 创建 place_order 指令
	accounts := limitorder.PlaceOrderAccounts{
		Config:        config,
		Market:        marketPda,
		Margin:        margin,
		Order:         order,
		Owner:         userWallet,
		SystemProgram: aSDK.SystemProgramID,
	}

	placeOrderIx, err := limitorder.NewPlaceOrderInstruction(accounts, params, whitelistProof)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create place_order instruction: %w", err)
	}

	// 获取最新区块哈希
	recent, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// 构建交易
	tx, err := aSDK.NewTransaction(
		[]aSDK.Instruction{placeOrderIx},
		recent.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create transaction: %w", err)
	}

	// 序列化为 base64
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to serialize transaction: %w", err)
	}
	txBase64 := base64.StdEncoding.EncodeToString(txBytes)

	// 预先保存订单到数据库（状态为待确认 pending=0）
	// Consumer 监听到链上事件后会更新为 active=1
	go l.saveOrderToDatabase(order.String(), in, marketInfo, priceLots, qtyLots, expirySlot, orderSeqNum, margin.String())

	return txBase64, order.String(), int64(orderSeqNum), nil
}

func (l *CreateLimitOrderLogic) getMarketInfo(marketPda string) (*limitordermodel.LimitOrderMarket, error) {
	// 从数据库查询市场信息
	marketModel := limitordermodel.NewLimitOrderMarketModel(l.svcCtx.DB)
	market, err := marketModel.FindOneByMarketPda(l.ctx, marketPda)
	if err != nil {
		return nil, fmt.Errorf("market not found: %w", err)
	}

	// 检查市场状态
	if market.Status != 1 {
		return nil, errors.New("market is not active")
	}
	if market.Paused != 0 {
		return nil, errors.New("market is paused")
	}

	return market, nil
}

// saveOrderToDatabase 保存订单到数据库（异步执行）
func (l *CreateLimitOrderLogic) saveOrderToDatabase(
	orderPda string,
	in *trade.CreateLimitOrderRequest,
	marketInfo *limitordermodel.LimitOrderMarket,
	priceLots uint64,
	qtyLots uint64,
	expirySlot uint64,
	orderSeqNum uint64,
	marginPda string,
) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("[CreateLimitOrder] Panic in saveOrderToDatabase: %v", r)
		}
	}()

	orderModel := limitordermodel.NewLimitOrderModel(l.svcCtx.DB)

	// 计算显示价格和数量
	baseMultiplier := decimal.NewFromInt(int64(pow10(int32(marketInfo.BaseDecimals))))
	quoteMultiplier := decimal.NewFromInt(int64(pow10(int32(marketInfo.QuoteDecimals))))

	displayPrice := decimal.NewFromInt(int64(priceLots)).Mul(baseMultiplier).Div(quoteMultiplier)
	displayQty := decimal.NewFromInt(int64(qtyLots)).Div(baseMultiplier)

	order := &limitordermodel.LimitOrder{
		OrderPda:          orderPda,
		MarketPda:         in.MarketPda,
		MarginPda:         marginPda,
		Owner:             in.UserWalletAddress,
		Side:              int64(in.Side),
		PriceLots:         int64(priceLots),
		QtyLots:           int64(qtyLots),
		RemainingLots:     int64(qtyLots),
		PriceDisplay:      displayPrice.InexactFloat64(),
		QtyDisplay:        displayQty.InexactFloat64(),
		RemainingDisplay:  displayQty.InexactFloat64(),
		TotalValueDisplay: displayPrice.Mul(displayQty).InexactFloat64(),
		ExpirySlot:        int64(expirySlot),
		Status:            0, // 0=pending, 1=active, 2=filled, 3=cancelled, 4=expired
		OrderId:           int64(orderSeqNum),
		SelfTradeBehavior: int64(in.SelfTradeBehavior),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := orderModel.Insert(l.ctx, order); err != nil {
		logx.Errorf("[CreateLimitOrder] Failed to save order to database: %v", err)
	} else {
		logx.Infof("[CreateLimitOrder] Order saved to database: order_pda=%s, order_id=%d, status=pending", orderPda, orderSeqNum)
	}
}
