package logic

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncMarginAccountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSyncMarginAccountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMarginAccountLogic {
	return &SyncMarginAccountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 同步保证金账户数据（从链上同步到数据库）
func (l *SyncMarginAccountLogic) SyncMarginAccount(in *limitorder.SyncMarginAccountRequest) (*limitorder.SyncMarginAccountResponse, error) {
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

	logx.Infof("Syncing margin account: market=%s, owner=%s, margin_pda=%s",
		in.MarketPda, in.UserWalletAddress, marginPDA.String())

	// 查询链上账户数据
	marginData, slot, err := l.fetchMarginAccountFromChain(marginPDA)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch margin account from chain: %w", err)
	}

	// 验证账户数据
	if marginData.Owner != in.UserWalletAddress {
		return nil, fmt.Errorf("margin account owner mismatch: expected %s, got %s",
			in.UserWalletAddress, marginData.Owner)
	}
	if marginData.Market != in.MarketPda {
		return nil, fmt.Errorf("margin account market mismatch: expected %s, got %s",
			in.MarketPda, marginData.Market)
	}

	// 查询市场信息（获取 decimals）
	market, err := l.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(l.ctx, in.MarketPda)
	if err != nil {
		return nil, fmt.Errorf("failed to find market: %w", err)
	}

	// 计算显示值
	baseDecimals := market.BaseDecimals
	quoteDecimals := market.QuoteDecimals

	baseFreeDisplay := convertLotsToDisplay(marginData.BaseFree, baseDecimals)
	baseLockedDisplay := convertLotsToDisplay(marginData.BaseLocked, baseDecimals)
	quoteFreeDisplay := convertLotsToDisplay(marginData.QuoteFree, quoteDecimals)
	quoteLockedDisplay := convertLotsToDisplay(marginData.QuoteLocked, quoteDecimals)

	// 保存或更新到数据库
	dbMargin := &limitordermodel.LimitOrderMargin{
		MarginPda:          marginPDA.String(),
		MarketId:           market.Id,
		MarketPda:          in.MarketPda,
		Owner:              in.UserWalletAddress,
		BaseFree:           marginData.BaseFree,
		BaseLocked:         marginData.BaseLocked,
		QuoteFree:          marginData.QuoteFree,
		QuoteLocked:        marginData.QuoteLocked,
		BaseFreeDisplay:    baseFreeDisplay,
		BaseLockedDisplay:  baseLockedDisplay,
		QuoteFreeDisplay:   quoteFreeDisplay,
		QuoteLockedDisplay: quoteLockedDisplay,
		LastSyncSlot:       slot,
		UpdatedAt:          time.Now(),
	}

	// 尝试查找现有记录
	existingMargin, err := l.svcCtx.LimitOrderMarginModel.FindOneByMarginPda(l.ctx, marginPDA.String())
	if err != nil {
		// 记录不存在，创建新记录
		dbMargin.CreatedAt = time.Now()
		dbMargin.Status = 1 // 链上账户已存在，状态设为正常
		// 如果提供了交易哈希，保存
		if in.TxHash != "" {
			dbMargin.InitTxHash = sql.NullString{
				String: in.TxHash,
				Valid:  true,
			}
		}
		if err := l.svcCtx.LimitOrderMarginModel.Insert(l.ctx, dbMargin); err != nil {
			return nil, fmt.Errorf("failed to insert margin: %w", err)
		}
		logx.Infof("Created new margin record: margin_pda=%s, status=1 (normal)", marginPDA.String())
	} else {
		// 记录已存在，更新
		dbMargin.Id = existingMargin.Id
		dbMargin.CreatedAt = existingMargin.CreatedAt
		dbMargin.Status = 1 // 链上账户已存在，状态更新为正常

		// 如果提供了交易哈希，更新；否则保留现有值
		if in.TxHash != "" {
			dbMargin.InitTxHash = sql.NullString{
				String: in.TxHash,
				Valid:  true,
			}
		} else {
			dbMargin.InitTxHash = existingMargin.InitTxHash
		}

		if err := l.svcCtx.LimitOrderMarginModel.Update(l.ctx, dbMargin); err != nil {
			return nil, fmt.Errorf("failed to update margin: %w", err)
		}
		logx.Infof("Updated existing margin record: id=%d, margin_pda=%s, status=1 (normal)", dbMargin.Id, marginPDA.String())
	}

	// 返回响应
	return &limitorder.SyncMarginAccountResponse{
		Success: true,
		Message: "Margin account synced successfully",
		Margin: &limitorder.MarginAccount{
			Id:           dbMargin.Id,
			MarginPda:    dbMargin.MarginPda,
			MarketPda:    dbMargin.MarketPda,
			Owner:        dbMargin.Owner,
			BaseFree:     fmt.Sprintf("%.8f", dbMargin.BaseFreeDisplay),
			BaseLocked:   fmt.Sprintf("%.8f", dbMargin.BaseLockedDisplay),
			QuoteFree:    fmt.Sprintf("%.8f", dbMargin.QuoteFreeDisplay),
			QuoteLocked:  fmt.Sprintf("%.8f", dbMargin.QuoteLockedDisplay),
			Status:       int32(dbMargin.Status),
			InitTxHash:   dbMargin.InitTxHash.String,
			LastSyncSlot: dbMargin.LastSyncSlot,
		},
	}, nil
}

// MarginAccountData 链上保证金账户数据结构
type MarginAccountData struct {
	Owner       string
	Market      string
	BaseFree    int64
	QuoteFree   int64
	BaseLocked  int64
	QuoteLocked int64
	Bump        uint8
}

// fetchMarginAccountFromChain 从链上获取保证金账户数据
func (l *SyncMarginAccountLogic) fetchMarginAccountFromChain(marginPDA solana.PublicKey) (*MarginAccountData, int64, error) {
	// 获取账户信息
	accountInfo, err := l.svcCtx.SolanaClient.GetAccountInfo(l.ctx, marginPDA)
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			return nil, 0, fmt.Errorf("margin account does not exist (not initialized)")
		}
		return nil, 0, fmt.Errorf("failed to get account info: %w", err)
	}

	if accountInfo.Value == nil {
		return nil, 0, fmt.Errorf("margin account does not exist (nil value)")
	}

	data := accountInfo.Value.Data.GetBinary()
	slot := int64(accountInfo.Context.Slot)

	// 反序列化账户数据
	// Anchor 账户布局: 8 字节 discriminator + 账户数据
	// UserMargin 结构:
	// - owner: 32 bytes (Pubkey)
	// - market: 32 bytes (Pubkey)
	// - base_free: 8 bytes (u64)
	// - quote_free: 8 bytes (u64)
	// - base_locked: 8 bytes (u64)
	// - quote_locked: 8 bytes (u64)
	// - bump: 1 byte (u8)

	if len(data) < 8+32+32+8+8+8+8+1 {
		return nil, 0, fmt.Errorf("invalid account data length: expected at least %d, got %d",
			8+32+32+8+8+8+8+1, len(data))
	}

	// 跳过 8 字节 discriminator
	offset := 8

	// 读取 owner (32 bytes)
	owner := solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	// 读取 market (32 bytes)
	market := solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	// 读取 base_free (8 bytes, little-endian u64)
	baseFree := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 读取 quote_free (8 bytes, little-endian u64)
	quoteFree := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 读取 base_locked (8 bytes, little-endian u64)
	baseLocked := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 读取 quote_locked (8 bytes, little-endian u64)
	quoteLocked := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 读取 bump (1 byte)
	bump := data[offset]

	logx.Infof("Fetched margin account data: owner=%s, market=%s, base_free=%d, quote_free=%d, base_locked=%d, quote_locked=%d, bump=%d",
		owner.String(), market.String(), baseFree, quoteFree, baseLocked, quoteLocked, bump)

	return &MarginAccountData{
		Owner:       owner.String(),
		Market:      market.String(),
		BaseFree:    baseFree,
		QuoteFree:   quoteFree,
		BaseLocked:  baseLocked,
		QuoteLocked: quoteLocked,
		Bump:        bump,
	}, slot, nil
}

// convertLotsToDisplay 将 lots 转换为显示值
func convertLotsToDisplay(lots int64, decimals int64) float64 {
	if lots == 0 {
		return 0
	}
	divisor := decimal.NewFromFloat(math.Pow10(int(decimals)))
	value := decimal.NewFromInt(lots).Div(divisor)
	result, _ := value.Float64()
	return result
}
