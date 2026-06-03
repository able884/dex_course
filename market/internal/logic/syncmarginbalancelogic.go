package logic

import (
	"context"
	"errors"
	"fmt"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/pkg/limitorder"
)

type SyncMarginBalanceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSyncMarginBalanceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMarginBalanceLogic {
	return &SyncMarginBalanceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SyncMarginBalanceLogic) SyncMarginBalance(in *market.SyncMarginBalanceRequest) (*market.SyncMarginBalanceResponse, error) {
	// Validate parameters
	if err := l.validate(in); err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Parse addresses
	marketPda, err := ag_solanago.PublicKeyFromBase58(in.MarketPda)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("invalid market_pda: %v", err),
		}, nil
	}

	userWallet, err := ag_solanago.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("invalid user_wallet_address: %v", err),
		}, nil
	}

	// Get market info from database
	marketModel := limitordermodel.NewLimitOrderMarketModel(l.svcCtx.DB)
	marketInfo, err := marketModel.FindOneByMarketPda(l.ctx, in.MarketPda)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("market not found: %v", err),
		}, nil
	}

	// Parse mints
	baseMint, err := ag_solanago.PublicKeyFromBase58(marketInfo.BaseMint)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("invalid base_mint: %v", err),
		}, nil
	}

	quoteMint, err := ag_solanago.PublicKeyFromBase58(marketInfo.QuoteMint)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("invalid quote_mint: %v", err),
		}, nil
	}

	// Derive margin PDA
	_, _, marginPda, _, err := limitorder.DerivePDAs(baseMint, quoteMint, userWallet, 0)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("failed to derive margin PDA: %v", err),
		}, nil
	}

	// Fetch margin account from blockchain
	if l.svcCtx.SolCli == nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: "solana rpc client not configured",
		}, nil
	}

	rpcClient := l.svcCtx.SolCli
	accountInfo, err := rpcClient.GetAccountInfo(l.ctx, marginPda)
	if err != nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: fmt.Sprintf("failed to fetch margin account: %v", err),
		}, nil
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: "margin account not found on chain",
		}, nil
	}

	// Decode margin account data
	// The account data structure (from Anchor):
	// - 8 bytes: discriminator
	// - 32 bytes: owner (Pubkey)
	// - 32 bytes: market (Pubkey)
	// - 8 bytes: base_free (u64)
	// - 8 bytes: quote_free (u64)
	// - 8 bytes: base_locked (u64)
	// - 8 bytes: quote_locked (u64)
	// - 1 byte: bump
	data := accountInfo.Value.Data.GetBinary()
	if len(data) < 105 { // 8 + 32 + 32 + 8 + 8 + 8 + 8 + 1
		return &market.SyncMarginBalanceResponse{
			Success: false,
			Message: "invalid margin account data",
		}, nil
	}

	// Skip discriminator (8 bytes) and owner/market (64 bytes)
	offset := 72

	// Read balances (little-endian u64)
	baseFree := readU64LE(data[offset : offset+8])
	quoteFree := readU64LE(data[offset+8 : offset+16])
	baseLocked := readU64LE(data[offset+16 : offset+24])
	quoteLocked := readU64LE(data[offset+24 : offset+32])

	// Calculate totals
	baseTotal := baseFree + baseLocked
	quoteTotal := quoteFree + quoteLocked

	// Convert to display format
	baseFreeDisplay := float64(baseFree) / float64(pow10(int32(marketInfo.BaseDecimals)))
	quoteFreeDisplay := float64(quoteFree) / float64(pow10(int32(marketInfo.QuoteDecimals)))
	baseLockedDisplay := float64(baseLocked) / float64(pow10(int32(marketInfo.BaseDecimals)))
	quoteLockedDisplay := float64(quoteLocked) / float64(pow10(int32(marketInfo.QuoteDecimals)))
	baseTotalDisplay := float64(baseTotal) / float64(pow10(int32(marketInfo.BaseDecimals)))
	quoteTotalDisplay := float64(quoteTotal) / float64(pow10(int32(marketInfo.QuoteDecimals)))

	// Get current slot
	slot, err := rpcClient.GetSlot(l.ctx, ag_rpc.CommitmentConfirmed)
	if err != nil {
		slot = 0 // Use 0 if failed to get slot
	}

	// Update database
	marginModel := limitordermodel.NewLimitOrderMarginModel(l.svcCtx.DB)
	existingMargin, err := marginModel.FindOneByMarginPda(l.ctx, marginPda.String())

	if err != nil || existingMargin == nil {
		// Create new margin record
		newMargin := &limitordermodel.LimitOrderMargin{
			MarginPda:          marginPda.String(),
			MarketPda:          marketPda.String(),
			Owner:              userWallet.String(),
			BaseFree:           int64(baseFree),
			QuoteFree:          int64(quoteFree),
			BaseLocked:         int64(baseLocked),
			QuoteLocked:        int64(quoteLocked),
			BaseFreeDisplay:    baseFreeDisplay,
			QuoteFreeDisplay:   quoteFreeDisplay,
			BaseLockedDisplay:  baseLockedDisplay,
			QuoteLockedDisplay: quoteLockedDisplay,
			Status:             1, // Active
			LastSyncSlot:       int64(slot),
		}

		err = marginModel.Insert(l.ctx, newMargin)
		if err != nil {
			return &market.SyncMarginBalanceResponse{
				Success: false,
				Message: fmt.Sprintf("failed to insert margin: %v", err),
			}, nil
		}
	} else {
		// Update existing margin record
		existingMargin.BaseFree = int64(baseFree)
		existingMargin.QuoteFree = int64(quoteFree)
		existingMargin.BaseLocked = int64(baseLocked)
		existingMargin.QuoteLocked = int64(quoteLocked)
		existingMargin.BaseFreeDisplay = baseFreeDisplay
		existingMargin.QuoteFreeDisplay = quoteFreeDisplay
		existingMargin.BaseLockedDisplay = baseLockedDisplay
		existingMargin.QuoteLockedDisplay = quoteLockedDisplay
		existingMargin.Status = 1 // Active
		existingMargin.LastSyncSlot = int64(slot)

		err = marginModel.Update(l.ctx, existingMargin)
		if err != nil {
			return &market.SyncMarginBalanceResponse{
				Success: false,
				Message: fmt.Sprintf("failed to update margin: %v", err),
			}, nil
		}
	}

	// Return success with updated margin data
	return &market.SyncMarginBalanceResponse{
		Success: true,
		Message: "margin balance synced successfully",
		Margin: &market.MarginAccountItem{
			MarginPda:    marginPda.String(),
			MarketPda:    marketPda.String(),
			BaseFree:     fmt.Sprintf("%.9f", baseFreeDisplay),
			QuoteFree:    fmt.Sprintf("%.9f", quoteFreeDisplay),
			BaseLocked:   fmt.Sprintf("%.9f", baseLockedDisplay),
			QuoteLocked:  fmt.Sprintf("%.9f", quoteLockedDisplay),
			BaseTotal:    fmt.Sprintf("%.9f", baseTotalDisplay),
			QuoteTotal:   fmt.Sprintf("%.9f", quoteTotalDisplay),
			BaseSymbol:   marketInfo.BaseSymbol,
			QuoteSymbol:  marketInfo.QuoteSymbol,
			Status:       1,
			LastSyncSlot: int64(slot),
		},
	}, nil
}

func (l *SyncMarginBalanceLogic) validate(in *market.SyncMarginBalanceRequest) error {
	if in.ChainId != 100000 {
		return errors.New("invalid chain_id, must be 100000 for Solana")
	}

	if in.MarketPda == "" {
		return errors.New("market_pda is required")
	}

	if in.UserWalletAddress == "" {
		return errors.New("user_wallet_address is required")
	}

	return nil
}

func readU64LE(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func pow10(n int32) uint64 {
	result := uint64(1)
	for i := int32(0); i < n; i++ {
		result *= 10
	}
	return result
}
