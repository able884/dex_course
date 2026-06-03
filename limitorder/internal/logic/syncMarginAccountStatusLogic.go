package logic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncMarginAccountStatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSyncMarginAccountStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMarginAccountStatusLogic {
	return &SyncMarginAccountStatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 同步保证金账户状态（检查链上是否存在，更新数据库状态）
func (l *SyncMarginAccountStatusLogic) SyncMarginAccountStatus(in *limitorder.SyncMarginAccountStatusRequest) (*limitorder.SyncMarginAccountStatusResponse, error) {
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

	logx.Infof("Syncing margin account status: market=%s, owner=%s, margin_pda=%s",
		in.MarketPda, in.UserWalletAddress, marginPDA.String())

	// 查询市场信息
	market, err := l.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(l.ctx, in.MarketPda)
	if err != nil {
		return nil, fmt.Errorf("failed to find market: %w", err)
	}

	// 查询链上账户是否存在
	accountInfo, err := l.svcCtx.SolanaClient.GetAccountInfo(l.ctx, marginPDA)
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	accountExists := accountInfo != nil && accountInfo.Value != nil

	// 查询数据库记录
	dbMargin, err := l.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(l.ctx, marginPDA.String())

	if accountExists {
		// 链上账户存在，同步数据并更新状态为"正常"
		logx.Infof("Margin account exists on chain, syncing data...")

		// 使用 SyncMarginAccount logic 同步数据（传递 TxHash 参数）
		syncLogic := NewSyncMarginAccountLogic(l.ctx, l.svcCtx)
		syncResp, err := syncLogic.SyncMarginAccount(&limitorder.SyncMarginAccountRequest{
			ChainId:           in.ChainId,
			UserWalletAddress: in.UserWalletAddress,
			MarketPda:         in.MarketPda,
			TxHash:            in.TxHash, // 传递交易哈希
		})
		if err != nil {
			return nil, fmt.Errorf("failed to sync margin account: %w", err)
		}

		// SyncMarginAccount 内部已经更新了 status 和 init_tx_hash，无需额外更新
		logx.Infof("Margin account synced: status=%d, last_sync_slot=%d", syncResp.Margin.Status, syncResp.Margin.LastSyncSlot)

		return &limitorder.SyncMarginAccountStatusResponse{
			Success:   true,
			Message:   "Margin account exists on chain and synced successfully",
			Status:    1, // 正常
			MarginPda: marginPDA.String(),
			TxBase64:  "",
			Margin:    syncResp.Margin,
		}, nil
	}

	// 链上账户不存在
	logx.Infof("Margin account does not exist on chain")

	// 如果数据库中存在记录，更新或保持状态为"创建中"
	if dbMargin == nil {
		// 数据库中也不存在，创建记录并设置状态为"创建中"
		newMargin := &limitordermodel.LimitOrderMargin{
			MarginPda:   marginPDA.String(),
			MarketId:    market.Id,
			MarketPda:   in.MarketPda,
			Owner:       in.UserWalletAddress,
			Status:      0, // 0=创建中
			InitTxHash: sql.NullString{
				String: in.TxHash,
				Valid:  in.TxHash != "",
			},
			BaseFree:           0,
			BaseLocked:         0,
			QuoteFree:          0,
			QuoteLocked:        0,
			BaseFreeDisplay:    0,
			BaseLockedDisplay:  0,
			QuoteFreeDisplay:   0,
			QuoteLockedDisplay: 0,
			LastSyncSlot:       0,
			CreatedAt:          time.Now(),
		}
		if err := l.svcCtx.LimitOrderMarginModel.Insert(l.ctx, newMargin); err != nil {
			logx.Errorf("Failed to insert margin record: %v", err)
		} else {
			logx.Infof("Created margin record with status=0 (creating)")
		}
	} else {
		// 数据库记录存在，确保状态为"创建中"
		if dbMargin.Status != 0 {
			dbMargin.Status = 0
			if err := l.svcCtx.LimitOrderMarginModel.Update(l.ctx, dbMargin); err != nil {
				logx.Errorf("Failed to update margin status: %v", err)
			} else {
				logx.Infof("Updated margin record status to 0 (creating)")
			}
		}
	}

	// 重新构建初始化交易
	txBase64, marginPDAResult, err := l.svcCtx.SolanaClient.BuildInitializeMarginTransaction(
		l.ctx,
		marketPubkey,
		ownerPubkey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build initialize transaction: %w", err)
	}

	return &limitorder.SyncMarginAccountStatusResponse{
		Success:   true,
		Message:   "Margin account does not exist on chain. Transaction rebuilt. Please sign and send the transaction.",
		Status:    0, // 创建中
		MarginPda: marginPDAResult.String(),
		TxBase64:  txBase64,
		Margin:    nil,
	}, nil
}
