package logic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncMarketStatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSyncMarketStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMarketStatusLogic {
	return &SyncMarketStatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 同步市场状态
func (l *SyncMarketStatusLogic) SyncMarketStatus(in *limitorder.SyncMarketStatusRequest) (*limitorder.SyncMarketStatusResponse, error) {
	// 1. Validate parameters
	if in.MarketPda == "" {
		return nil, fmt.Errorf("market_pda is required")
	}

	// Check Solana client
	if l.svcCtx.SolanaClient == nil {
		return nil, fmt.Errorf("solana client not initialized")
	}

	// Log all request fields for debugging
	logx.Infof("SyncMarketStatus called: chain_id=%d, market_pda=%s, tx_hash=%s, admin='%s' (length=%d)",
		in.GetChainId(), in.GetMarketPda(), in.GetTxHash(), in.GetAdmin(), len(in.GetAdmin()))

	// 2. Parse market PDA
	marketPubkey, err := solana.PublicKeyFromBase58(in.MarketPda)
	if err != nil {
		return nil, fmt.Errorf("invalid market_pda: %w", err)
	}

	// 3. Check if market exists on-chain
	existsOnChain, err := l.svcCtx.SolanaClient.GetMarketAccount(l.ctx, marketPubkey)
	if err != nil {
		// If error contains "not found", treat it as market doesn't exist on-chain (normal case)
		// Only return error for other types of errors (RPC errors, connection issues, etc.)
		errMsg := err.Error()
		if !containsNotFound(errMsg) {
			logx.Errorf("Failed to check market on-chain (unexpected error): %v", err)
			return nil, fmt.Errorf("failed to check on-chain status: %w", err)
		}
		// Market not found on-chain - this is expected for markets being created
		logx.Infof("Market not found on-chain (expected): market_pda=%s", in.MarketPda)
		existsOnChain = false
	}

	// 4. Check if market exists in database
	dbMarket, err := l.svcCtx.LimitOrderMarketModel.FindOneByMarketPda(l.ctx, in.MarketPda)
	dbExists := err == nil && dbMarket != nil

	logx.Infof("Market status check: market_pda=%s, on_chain=%v, in_db=%v", in.MarketPda, existsOnChain, dbExists)

	// 5. Handle different scenarios
	if existsOnChain {
		// Market exists on-chain
		if dbExists {
			// Scenario 1: Exists both on-chain and in DB - Update status to 1 (normal)
			if dbMarket.Status != 1 {
				dbMarket.Status = 1 // Set to normal
				if in.TxHash != "" {
					dbMarket.InitTxHash = sql.NullString{String: in.TxHash, Valid: true}
				}
				if err := l.svcCtx.LimitOrderMarketModel.Update(l.ctx, dbMarket); err != nil {
					logx.Errorf("Failed to update market status: %v", err)
					return nil, fmt.Errorf("failed to update market status: %w", err)
				}
				logx.Infof("Updated market status to 1 (normal): market_pda=%s", in.MarketPda)
			}

			// Return market info
			return &limitorder.SyncMarketStatusResponse{
				Success:   true,
				Message:   "Market synced successfully. Market is active on-chain.",
				Status:    1,
				MarketPda: in.MarketPda,
				Market: &limitorder.MarketInfo{
					Id:            dbMarket.Id,
					MarketPda:     dbMarket.MarketPda,
					BaseMint:      dbMarket.BaseMint,
					QuoteMint:     dbMarket.QuoteMint,
					BaseSymbol:    dbMarket.BaseSymbol,
					QuoteSymbol:   dbMarket.QuoteSymbol,
					BaseDecimals:  int32(dbMarket.BaseDecimals),
					QuoteDecimals: int32(dbMarket.QuoteDecimals),
					BaseVault:     dbMarket.BaseVault,
					QuoteVault:    dbMarket.QuoteVault,
					TickSize:      dbMarket.TickSize,
					MinBaseLot:    dbMarket.MinBaseLot,
					MinQuoteLot:   dbMarket.MinQuoteLot,
					MakerFeeBps:   int32(dbMarket.MakerFeeBps),
					TakerFeeBps:   int32(dbMarket.TakerFeeBps),
					Status:        int32(dbMarket.Status),
					Paused:        dbMarket.Paused != 0,
				},
			}, nil
		} else {
			// Scenario 2: Exists on-chain but not in DB
			logx.Infof("Market exists on-chain but not in database: market_pda=%s", in.MarketPda)
			return &limitorder.SyncMarketStatusResponse{
				Success:   false,
				Message:   "Market exists on-chain but not found in database. Please contact administrator.",
				Status:    1,
				MarketPda: in.MarketPda,
			}, nil
		}
	} else {
		// Market doesn't exist on-chain
		if dbExists {
			// Scenario 3: Exists in DB but not on-chain - rebuild transaction
			logx.Infof("Market not found on-chain, rebuilding transaction: market_pda=%s, status=%d", in.MarketPda, dbMarket.Status)

			// Parse addresses from database
			baseMintPubkey, err := solana.PublicKeyFromBase58(dbMarket.BaseMint)
			if err != nil {
				return nil, fmt.Errorf("invalid base_mint in database: %w", err)
			}

			quoteMintPubkey, err := solana.PublicKeyFromBase58(dbMarket.QuoteMint)
			if err != nil {
				return nil, fmt.Errorf("invalid quote_mint in database: %w", err)
			}

			var adminPubkey solana.PublicKey
			if in.Admin != "" {
				var err error
				adminPubkey, err = solana.PublicKeyFromBase58(in.Admin)
				if err != nil {
					logx.Errorf("Failed to parse admin address '%s': %v", in.Admin, err)
					return nil, fmt.Errorf("invalid admin address: %w", err)
				}
				logx.Infof("Successfully parsed admin pubkey: %s", adminPubkey.String())
			} else {
				// If admin not provided, we cannot rebuild the transaction
				logx.Errorf("Admin address is empty. Request fields: chain_id=%d, market_pda=%s, tx_hash=%s",
					in.GetChainId(), in.GetMarketPda(), in.GetTxHash())
				return nil, fmt.Errorf("admin wallet address is required to rebuild the transaction")
			}

			// Parse Pyth price feed if available
			var pythPriceFeed solana.PublicKey
			if dbMarket.PythPriceFeed.Valid && dbMarket.PythPriceFeed.String != "" {
				pythPriceFeed, err = solana.PublicKeyFromBase58(dbMarket.PythPriceFeed.String)
				if err != nil {
					pythPriceFeed = solana.SystemProgramID
				}
			} else {
				pythPriceFeed = solana.SystemProgramID
			}

			// Build initialize market transaction
			txBase64, _, err := l.svcCtx.SolanaClient.BuildInitializeMarketTransaction(
				l.ctx,
				adminPubkey,
				baseMintPubkey,
				quoteMintPubkey,
				uint64(dbMarket.TickSize),
				uint64(dbMarket.MinBaseLot),
				uint64(dbMarket.MinQuoteLot),
				uint16(dbMarket.MakerFeeBps),
				uint16(dbMarket.TakerFeeBps),
				pythPriceFeed,
			)
			if err != nil {
				logx.Errorf("Failed to rebuild market transaction: %v", err)
				return nil, fmt.Errorf("failed to rebuild transaction: %w", err)
			}

			logx.Infof("Successfully rebuilt market transaction: market_pda=%s", in.MarketPda)

			return &limitorder.SyncMarketStatusResponse{
				Success:   true,
				Message:   "Market not found on-chain. Transaction rebuilt successfully. Please sign and send the transaction.",
				Status:    0,
				MarketPda: in.MarketPda,
				TxBase64:  txBase64,
				Market: &limitorder.MarketInfo{
					Id:            dbMarket.Id,
					MarketPda:     dbMarket.MarketPda,
					BaseMint:      dbMarket.BaseMint,
					QuoteMint:     dbMarket.QuoteMint,
					BaseSymbol:    dbMarket.BaseSymbol,
					QuoteSymbol:   dbMarket.QuoteSymbol,
					BaseDecimals:  int32(dbMarket.BaseDecimals),
					QuoteDecimals: int32(dbMarket.QuoteDecimals),
					BaseVault:     dbMarket.BaseVault,
					QuoteVault:    dbMarket.QuoteVault,
					TickSize:      dbMarket.TickSize,
					MinBaseLot:    dbMarket.MinBaseLot,
					MinQuoteLot:   dbMarket.MinQuoteLot,
					MakerFeeBps:   int32(dbMarket.MakerFeeBps),
					TakerFeeBps:   int32(dbMarket.TakerFeeBps),
					Status:        int32(dbMarket.Status),
					Paused:        dbMarket.Paused != 0,
				},
			}, nil
		} else {
			// Scenario 4: Doesn't exist anywhere
			return &limitorder.SyncMarketStatusResponse{
				Success:   false,
				Message:   "Market not found on-chain or in database. Please create the market first.",
				Status:    0,
				MarketPda: in.MarketPda,
			}, nil
		}
	}
}

// containsNotFound checks if error message contains "not found" indicating account doesn't exist
func containsNotFound(errMsg string) bool {
	errMsgLower := strings.ToLower(errMsg)
	return strings.Contains(errMsgLower, "not found") ||
		strings.Contains(errMsgLower, "notfound") ||
		strings.Contains(errMsgLower, "account does not exist") ||
		strings.Contains(errMsgLower, "invalid account data")
}
