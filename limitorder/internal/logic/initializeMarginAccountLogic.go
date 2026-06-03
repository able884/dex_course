package logic

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type InitializeMarginAccountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewInitializeMarginAccountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *InitializeMarginAccountLogic {
	return &InitializeMarginAccountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 初始化保证金账户
func (l *InitializeMarginAccountLogic) InitializeMarginAccount(in *limitorder.InitializeMarginAccountRequest) (*limitorder.InitializeMarginAccountResponse, error) {
	// 参数验证
	if in.MarketPda == "" {
		return nil, fmt.Errorf("market_pda is required")
	}
	if in.UserWalletAddress == "" {
		return nil, fmt.Errorf("user_wallet_address is required")
	}

	// 检查 Solana 客户端是否可用
	if l.svcCtx.SolanaClient == nil {
		return nil, fmt.Errorf("solana client not initialized")
	}

	// 解析地址
	marketPubkey, err := solana.PublicKeyFromBase58(in.MarketPda)
	if err != nil {
		return nil, fmt.Errorf("invalid market_pda: %w", err)
	}

	ownerPubkey, err := solana.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid user_wallet_address: %w", err)
	}

	// 计算 margin PDA
	marginPDA, _, err := l.svcCtx.SolanaClient.FindMarginPDA(marketPubkey, ownerPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to find margin PDA: %w", err)
	}

	// 查询市场信息
	market, err := l.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(l.ctx, in.MarketPda)
	if err != nil {
		return nil, fmt.Errorf("failed to find market: %w", err)
	}

	// 检查数据库中是否已存在该 margin 账户
	existingMargin, err := l.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(l.ctx, marginPDA.String())
	if err == nil && existingMargin != nil {
		// 账户已存在，检查状态
		if existingMargin.Status == 1 {
			// 状态正常，说明已经初始化完成
			return nil, fmt.Errorf("margin account already initialized and active")
		}
		// 状态为创建中，允许重新构建交易
		logx.Infof("Margin account exists in database with status=%d, rebuilding transaction", existingMargin.Status)
	}

	// 构建初始化保证金账户的交易
	txBase64, marginPDAResult, err := l.svcCtx.SolanaClient.BuildInitializeMarginTransaction(
		l.ctx,
		marketPubkey,
		ownerPubkey,
	)
	if err != nil {
		logx.Errorf("Failed to build initialize margin transaction: %v", err)
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// 保存或更新到数据库，状态设置为"创建中"
	if existingMargin == nil {
		// 创建新记录
		newMargin := &limitordermodel.LimitOrderMargin{
			MarginPda:          marginPDAResult.String(),
			MarketId:           market.Id,
			MarketPda:          in.MarketPda,
			Owner:              in.UserWalletAddress,
			Status:             0, // 0=创建中
			BaseFree:           0,
			BaseLocked:         0,
			QuoteFree:          0,
			QuoteLocked:        0,
			BaseFreeDisplay:    0,
			BaseLockedDisplay:  0,
			QuoteFreeDisplay:   0,
			QuoteLockedDisplay: 0,
			LastSyncSlot:       0,
		}
		if err := l.svcCtx.LimitOrderMarginModel.Insert(l.ctx, newMargin); err != nil {
			logx.Errorf("Failed to insert margin record: %v", err)
			// 不阻断流程，继续返回交易
		} else {
			logx.Infof("Created margin record with status=0 (creating): margin_pda=%s", marginPDAResult.String())
		}
	} else {
		// 更新现有记录，重置状态为"创建中"
		existingMargin.Status = 0 // 重置为创建中
		if err := l.svcCtx.LimitOrderMarginModel.Update(l.ctx, existingMargin); err != nil {
			logx.Errorf("Failed to update margin status: %v", err)
		} else {
			logx.Infof("Updated margin record status to 0 (creating): margin_pda=%s", marginPDAResult.String())
		}
	}

	logx.Infof("InitializeMarginAccount success: chain_id=%d, market_pda=%s, user=%s, margin_pda=%s",
		in.ChainId, in.MarketPda, in.UserWalletAddress, marginPDAResult.String())

	return &limitorder.InitializeMarginAccountResponse{
		Success:   true,
		Message:   "Transaction built successfully. Database record created with status=0 (creating). Please sign and send the transaction on the client side.",
		MarginPda: marginPDAResult.String(),
		TxHash:    txBase64, // 注意：这里返回的是 base64 编码的未签名交易，前端需要签名后发送
	}, nil
}
