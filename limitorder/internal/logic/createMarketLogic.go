package logic

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMarketLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateMarketLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMarketLogic {
	return &CreateMarketLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 创建限价单市场
func (l *CreateMarketLogic) CreateMarket(in *limitorder.CreateMarketRequest) (*limitorder.CreateMarketResponse, error) {
	// 1. Validate input parameters
	if in.BaseMint == "" {
		return nil, fmt.Errorf("base_mint is required")
	}
	if in.QuoteMint == "" {
		return nil, fmt.Errorf("quote_mint is required")
	}
	if in.Admin == "" {
		return nil, fmt.Errorf("admin wallet address is required")
	}
	if in.BaseDecimals < 0 || in.BaseDecimals > 18 {
		return nil, fmt.Errorf("invalid base_decimals: must be 0-18")
	}
	if in.QuoteDecimals < 0 || in.QuoteDecimals > 18 {
		return nil, fmt.Errorf("invalid quote_decimals: must be 0-18")
	}
	if in.TickSize <= 0 {
		return nil, fmt.Errorf("tick_size must be positive")
	}
	if in.MinBaseLot <= 0 {
		return nil, fmt.Errorf("min_base_lot must be positive")
	}
	if in.MakerFeeBps < 0 || in.MakerFeeBps > 5000 {
		return nil, fmt.Errorf("maker_fee_bps must be 0-5000 (0-50%%)")
	}
	if in.TakerFeeBps < 0 || in.TakerFeeBps > 5000 {
		return nil, fmt.Errorf("taker_fee_bps must be 0-5000 (0-50%%)")
	}

	// Check Solana client
	if l.svcCtx.SolanaClient == nil {
		return nil, fmt.Errorf("solana client not initialized")
	}

	logx.Infof("CreateMarket called: base=%s/%s, quote=%s/%s, admin=%s, tick=%f, minLot=%d, fees=%d/%d",
		in.BaseSymbol, in.BaseMint, in.QuoteSymbol, in.QuoteMint, in.Admin,
		in.TickSize, in.MinBaseLot, in.MakerFeeBps, in.TakerFeeBps)

	// 2. Parse addresses
	adminPubkey, err := solana.PublicKeyFromBase58(in.Admin)
	if err != nil {
		return nil, fmt.Errorf("invalid admin address: %w", err)
	}

	baseMintPubkey, err := solana.PublicKeyFromBase58(in.BaseMint)
	if err != nil {
		return nil, fmt.Errorf("invalid base_mint address: %w", err)
	}

	quoteMintPubkey, err := solana.PublicKeyFromBase58(in.QuoteMint)
	if err != nil {
		return nil, fmt.Errorf("invalid quote_mint address: %w", err)
	}

	// 3. Calculate market PDA
	marketPDA, _, err := l.svcCtx.SolanaClient.FindMarketPDA(baseMintPubkey, quoteMintPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to find market PDA: %w", err)
	}

	// 4. Calculate vault PDAs
	baseVaultPDA, _, err := l.svcCtx.SolanaClient.FindVaultPDA(marketPDA, baseMintPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to find base vault PDA: %w", err)
	}

	quoteVaultPDA, _, err := l.svcCtx.SolanaClient.FindVaultPDA(marketPDA, quoteMintPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to find quote vault PDA: %w", err)
	}

	// 5. Check if market already exists in database
	existingMarket, err := l.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(l.ctx, marketPDA.String())
	if err == nil && existingMarket != nil {
		logx.Infof("Market already exists: market_pda=%s", marketPDA.String())
		return &limitorder.CreateMarketResponse{
			Success:   false,
			Message:   "Market already exists for this token pair",
			MarketPda: marketPDA.String(),
		}, nil
	}

	// 6. Parse or use default Pyth price feed
	var pythPriceFeed solana.PublicKey
	if in.PythPriceFeed != "" {
		pythPriceFeed, err = solana.PublicKeyFromBase58(in.PythPriceFeed)
		if err != nil {
			return nil, fmt.Errorf("invalid pyth_price_feed: %w", err)
		}
	} else {
		// Use system program as default (means no price feed)
		pythPriceFeed = solana.SystemProgramID
	}

	// 7. Convert display values to lots (on-chain atomic units)
	quoteMultiplier := decimal.NewFromInt(pow10(int32(in.QuoteDecimals)))
	baseMultiplier := decimal.NewFromInt(pow10(int32(in.BaseDecimals)))

	// Convert tick_size from display value to lots
	// tick_size_lots = tick_size_display * 10^quote_decimals / 10^base_decimals
	tickSizeLotsDec := decimal.NewFromFloat(in.TickSize).Mul(quoteMultiplier).Div(baseMultiplier)
	tickSizeLotsFloat, _ := tickSizeLotsDec.Float64()
	tickSizeLots := uint64(math.Round(tickSizeLotsFloat))
	if tickSizeLots == 0 {
		return nil, fmt.Errorf("tick_size too small after conversion, try increasing tick_size or adjusting decimals")
	}

	// Convert min_base_lot from display value to lots
	// min_base_lot_lots = min_base_lot_display * 10^base_decimals
	minBaseLotDec := decimal.NewFromFloat(in.MinBaseLot).Mul(baseMultiplier)
	minBaseLotFloat, _ := minBaseLotDec.Float64()
	minBaseLotLots := uint64(math.Round(minBaseLotFloat))
	if minBaseLotLots == 0 {
		return nil, fmt.Errorf("min_base_lot too small after conversion, must be at least %v", 1.0/baseMultiplier.InexactFloat64())
	}

	// Calculate min_quote_lot based on min_base_lot and tick_size
	// min_quote_lot = min_base_lot_lots * tick_size_lots
	minQuoteLot := minBaseLotLots * tickSizeLots

	logx.Infof("[CreateMarket] Conversion: tick_size=%.6f -> %d lots, min_base_lot=%.6f -> %d lots, min_quote_lot=%d lots",
		in.TickSize, tickSizeLots, in.MinBaseLot, minBaseLotLots, minQuoteLot)

	// 8. Build initialize market transaction (includes init_market AND init_vaults instructions)
	txBase64, marketPDAResult, err := l.svcCtx.SolanaClient.BuildInitializeMarketTransaction(
		l.ctx,
		adminPubkey,
		baseMintPubkey,
		quoteMintPubkey,
		tickSizeLots,
		minBaseLotLots,
		minQuoteLot,
		uint16(in.MakerFeeBps),
		uint16(in.TakerFeeBps),
		pythPriceFeed,
	)
	if err != nil {
		logx.Errorf("Failed to build initialize market transaction: %v", err)
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// 9. Save market configuration to database with status=0 (creating)
	newMarket := &limitordermodel.LimitOrderMarket{
		MarketPda:      marketPDAResult.String(),
		BaseMint:       in.BaseMint,
		QuoteMint:      in.QuoteMint,
		BaseSymbol:     in.BaseSymbol,
		QuoteSymbol:    in.QuoteSymbol,
		BaseDecimals:   int64(in.BaseDecimals),
		QuoteDecimals:  int64(in.QuoteDecimals),
		BaseVault:      baseVaultPDA.String(),
		QuoteVault:     quoteVaultPDA.String(),
		TickSize:       int64(tickSizeLots),
		MinBaseLot:     int64(minBaseLotLots),
		MinQuoteLot:    int64(minQuoteLot),
		MakerFeeBps:    int64(in.MakerFeeBps),
		TakerFeeBps:    int64(in.TakerFeeBps),
		FeeAccumulator: 0,
		SeqNum:         0,
		Paused:         0,
		Status:         0,        // 0=创建中
		Network:        "devnet", // TODO: determine from chain_id
	}

	if in.PythPriceFeed != "" {
		newMarket.PythPriceFeed = sql.NullString{String: in.PythPriceFeed, Valid: true}
	}

	if err := l.svcCtx.LimitOrderMarketModel.Insert(l.ctx, newMarket); err != nil {
		logx.Errorf("Failed to insert market record: %v", err)
		// Don't block the flow, transaction can still be sent
	} else {
		logx.Infof("Created market record in database: market_pda=%s", marketPDAResult.String())
	}

	logx.Infof("CreateMarket success: market_pda=%s, base=%s/%s, quote=%s/%s",
		marketPDAResult.String(), in.BaseSymbol, in.BaseMint, in.QuoteSymbol, in.QuoteMint)

	return &limitorder.CreateMarketResponse{
		Success:   true,
		Message:   "Market and vaults creation transaction built successfully. This transaction includes both init_market and init_vaults instructions.",
		MarketPda: marketPDAResult.String(),
		TxBase64:  txBase64,
		ExpiresAt: 0, // TODO: calculate expiration time
	}, nil
}

// Helper function to calculate power of 10
func pow10(n int32) int64 {
	result := int64(1)
	for i := int32(0); i < n; i++ {
		result *= 10
	}
	return result
}
