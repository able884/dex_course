package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/limitorder"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type CancelLimitOrderLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCancelLimitOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelLimitOrderLogic {
	return &CancelLimitOrderLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CancelLimitOrderLogic) CancelLimitOrder(in *trade.CancelLimitOrderRequest) (*trade.CancelLimitOrderResponse, error) {
	// 1. 验证参数
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. 构建取消订单交易
	txBase64, err := l.buildCancelOrderTx(in)
	if err != nil {
		return nil, err
	}

	return &trade.CancelLimitOrderResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *CancelLimitOrderLogic) validate(in *trade.CancelLimitOrderRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if in.ChainId == 0 {
		in.ChainId = constants.SolChainIdInt
	}
	if strings.TrimSpace(in.OrderPda) == "" {
		return errors.New("order_pda is required")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address is required")
	}
	return nil
}

func (l *CancelLimitOrderLogic) buildCancelOrderTx(in *trade.CancelLimitOrderRequest) (string, error) {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	// 解析地址
	orderPda, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.OrderPda))
	if err != nil {
		return "", fmt.Errorf("invalid order_pda: %w", err)
	}

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("invalid user_wallet_address: %w", err)
	}

	// 查询订单信息
	orderInfo, err := l.getOrderInfo(in.OrderPda)
	if err != nil {
		return "", fmt.Errorf("failed to get order info: %w", err)
	}

	// 验证订单所有者
	if orderInfo.Owner != in.UserWalletAddress {
		return "", errors.New("unauthorized: user is not the order owner")
	}

	// 检查订单状态
	if orderInfo.Status != 1 {
		return "", errors.New("order is not active (already filled, cancelled, or expired)")
	}

	// 查询市场信息
	marketInfo, err := l.getMarketInfo(orderInfo.MarketPda)
	if err != nil {
		return "", fmt.Errorf("failed to get market info: %w", err)
	}

	// 解析 market PDA
	marketPda, err := aSDK.PublicKeyFromBase58(orderInfo.MarketPda)
	if err != nil {
		return "", fmt.Errorf("invalid market_pda in order: %w", err)
	}

	// 解析 margin PDA
	marginPda, err := aSDK.PublicKeyFromBase58(orderInfo.MarginPda)
	if err != nil {
		return "", fmt.Errorf("invalid margin_pda in order: %w", err)
	}

	// 解析 config PDA
	baseMint, err := aSDK.PublicKeyFromBase58(marketInfo.BaseMint)
	if err != nil {
		return "", fmt.Errorf("invalid base_mint in market: %w", err)
	}

	quoteMint, err := aSDK.PublicKeyFromBase58(marketInfo.QuoteMint)
	if err != nil {
		return "", fmt.Errorf("invalid quote_mint in market: %w", err)
	}

	config, _, _, _, err := limitorder.DerivePDAs(baseMint, quoteMint, userWallet, 0)
	if err != nil {
		return "", fmt.Errorf("failed to derive config PDA: %w", err)
	}

	// 创建 cancel_order 指令
	accounts := limitorder.CancelOrderAccounts{
		Config: config,
		Market: marketPda,
		Margin: marginPda,
		Order:  orderPda,
		Owner:  userWallet,
	}

	cancelOrderIx, err := limitorder.NewCancelOrderInstruction(accounts)
	if err != nil {
		return "", fmt.Errorf("failed to create cancel_order instruction: %w", err)
	}

	// 获取最新区块哈希
	recent, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// 构建交易
	tx, err := aSDK.NewTransaction(
		[]aSDK.Instruction{cancelOrderIx},
		recent.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	// 序列化为 base64
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}
	txBase64 := base64.StdEncoding.EncodeToString(txBytes)

	// 预先更新订单状态为取消中（cancelling）
	// Consumer 监听到链上事件后会更新为 cancelled=3
	go l.updateOrderStatusToCancelling(in.OrderPda)

	return txBase64, nil
}

func (l *CancelLimitOrderLogic) getOrderInfo(orderPda string) (*limitordermodel.LimitOrder, error) {
	// 从数据库查询订单信息
	orderModel := limitordermodel.NewLimitOrderModel(l.svcCtx.DB)
	order, err := orderModel.FindOneByOrderPda(l.ctx, orderPda)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	return order, nil
}

func (l *CancelLimitOrderLogic) getMarketInfo(marketPda string) (*limitordermodel.LimitOrderMarket, error) {
	// 从数据库查询市场信息
	marketModel := limitordermodel.NewLimitOrderMarketModel(l.svcCtx.DB)
	market, err := marketModel.FindOneByMarketPda(l.ctx, marketPda)
	if err != nil {
		return nil, fmt.Errorf("market not found: %w", err)
	}

	return market, nil
}

// updateOrderStatusToCancelling 更新订单状态为取消中（异步执行）
func (l *CancelLimitOrderLogic) updateOrderStatusToCancelling(orderPda string) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("[CancelLimitOrder] Panic in updateOrderStatusToCancelling: %v", r)
		}
	}()

	orderModel := limitordermodel.NewLimitOrderModel(l.svcCtx.DB)

	// 查询订单
	order, err := orderModel.FindOneByOrderPda(l.ctx, orderPda)
	if err != nil {
		logx.Errorf("[CancelLimitOrder] Failed to find order for status update: %v", err)
		return
	}

	// 更新状态为取消中（使用 status=5 表示 cancelling）
	// Consumer 监听到链上 CancelOrder 事件后会更新为 cancelled=3
	order.Status = 5 // 5=cancelling
	order.UpdatedAt = time.Now()

	if err := orderModel.Update(l.ctx, order); err != nil {
		logx.Errorf("[CancelLimitOrder] Failed to update order status to cancelling: %v", err)
	} else {
		logx.Infof("[CancelLimitOrder] Order status updated to cancelling: order_pda=%s", orderPda)
	}
}
