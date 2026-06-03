package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/pkg/limitorder"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type DepositMarginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDepositMarginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DepositMarginLogic {
	return &DepositMarginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DepositMarginLogic) DepositMargin(in *trade.DepositMarginRequest) (*trade.DepositMarginResponse, error) {
	// Validate parameters
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// Build transaction
	txBase64, err := l.buildDepositTx(in)
	if err != nil {
		return nil, err
	}

	return &trade.DepositMarginResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *DepositMarginLogic) validate(in *trade.DepositMarginRequest) error {
	if in.ChainId != 100000 {
		return errors.New("invalid chain_id, must be 100000 for Solana")
	}

	if in.MarketPda == "" {
		return errors.New("market_pda is required")
	}

	if in.UserWalletAddress == "" {
		return errors.New("user_wallet_address is required")
	}

	if in.Side != 1 && in.Side != 2 {
		return errors.New("invalid side, must be 1 (Base) or 2 (Quote)")
	}

	if in.Amount == "" {
		return errors.New("amount is required")
	}

	amount, err := strconv.ParseFloat(in.Amount, 64)
	if err != nil || amount <= 0 {
		return errors.New("invalid amount")
	}

	return nil
}

func (l *DepositMarginLogic) buildDepositTx(in *trade.DepositMarginRequest) (string, error) {
	// Parse addresses
	marketPda, err := ag_solanago.PublicKeyFromBase58(in.MarketPda)
	if err != nil {
		return "", fmt.Errorf("invalid market_pda: %w", err)
	}

	userWallet, err := ag_solanago.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		return "", fmt.Errorf("invalid user_wallet_address: %w", err)
	}

	// Query market info from database
	marketInfo, err := l.getMarketInfo(in.MarketPda)
	if err != nil {
		return "", fmt.Errorf("failed to get market info: %w", err)
	}

	// Parse mints
	baseMint, err := ag_solanago.PublicKeyFromBase58(marketInfo.BaseMint)
	if err != nil {
		return "", fmt.Errorf("invalid base_mint: %w", err)
	}

	quoteMint, err := ag_solanago.PublicKeyFromBase58(marketInfo.QuoteMint)
	if err != nil {
		return "", fmt.Errorf("invalid quote_mint: %w", err)
	}

	// Derive PDAs
	config, _, margin, _, err := limitorder.DerivePDAs(baseMint, quoteMint, userWallet, 0)
	if err != nil {
		return "", fmt.Errorf("failed to derive PDAs: %w", err)
	}

	// Derive vaults using correct seeds: ["MarketVault", market, mint]
	baseVault, quoteVault, err := limitorder.DeriveVaultPDAs(marketPda, baseMint, quoteMint)
	if err != nil {
		return "", fmt.Errorf("failed to derive vaults: %w", err)
	}

	// Get user token accounts (Associated Token Accounts)
	userBaseToken, _, err := ag_solanago.FindAssociatedTokenAddress(userWallet, baseMint)
	if err != nil {
		return "", fmt.Errorf("failed to get user base token account: %w", err)
	}

	userQuoteToken, _, err := ag_solanago.FindAssociatedTokenAddress(userWallet, quoteMint)
	if err != nil {
		return "", fmt.Errorf("failed to get user quote token account: %w", err)
	}

	// Get RPC client for checking account existence
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	// Check if user token accounts exist and create them if needed
	var instructions []ag_solanago.Instruction

	// Check and create base token account if needed
	if in.Side == 1 { // Only check base token if depositing base
		baseAccountInfo, err := rpcClient.GetAccountInfo(l.ctx, userBaseToken)
		if err != nil || baseAccountInfo == nil || baseAccountInfo.Value == nil {
			// Account doesn't exist, create it using associated-token-account program
			createBaseATAIx := ag_solanago.NewInstruction(
				ag_solanago.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"), // Associated Token Program
				[]*ag_solanago.AccountMeta{
					{PublicKey: userWallet, IsWritable: true, IsSigner: true},                    // payer
					{PublicKey: userBaseToken, IsWritable: true, IsSigner: false},                // associated token account
					{PublicKey: userWallet, IsWritable: false, IsSigner: false},                  // wallet
					{PublicKey: baseMint, IsWritable: false, IsSigner: false},                    // mint
					{PublicKey: ag_solanago.SystemProgramID, IsWritable: false, IsSigner: false}, // system program
					{PublicKey: ag_solanago.TokenProgramID, IsWritable: false, IsSigner: false},  // token program
				},
				[]byte{}, // No data needed for create instruction
			)
			instructions = append(instructions, createBaseATAIx)
		}
	}

	// Check and create quote token account if needed
	if in.Side == 2 { // Only check quote token if depositing quote
		quoteAccountInfo, err := rpcClient.GetAccountInfo(l.ctx, userQuoteToken)
		if err != nil || quoteAccountInfo == nil || quoteAccountInfo.Value == nil {
			// Account doesn't exist, create it using associated-token-account program
			createQuoteATAIx := ag_solanago.NewInstruction(
				ag_solanago.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"), // Associated Token Program
				[]*ag_solanago.AccountMeta{
					{PublicKey: userWallet, IsWritable: true, IsSigner: true},                    // payer
					{PublicKey: userQuoteToken, IsWritable: true, IsSigner: false},               // associated token account
					{PublicKey: userWallet, IsWritable: false, IsSigner: false},                  // wallet
					{PublicKey: quoteMint, IsWritable: false, IsSigner: false},                   // mint
					{PublicKey: ag_solanago.SystemProgramID, IsWritable: false, IsSigner: false}, // system program
					{PublicKey: ag_solanago.TokenProgramID, IsWritable: false, IsSigner: false},  // token program
				},
				[]byte{}, // No data needed for create instruction
			)
			instructions = append(instructions, createQuoteATAIx)
		}
	}

	// Convert amount to raw units
	amountFloat, _ := strconv.ParseFloat(in.Amount, 64)
	var decimals int64
	if in.Side == 1 { // Base
		decimals = marketInfo.BaseDecimals
	} else { // Quote
		decimals = marketInfo.QuoteDecimals
	}
	amountRaw := uint64(amountFloat * float64(pow10(int32(decimals))))

	// Convert side (1=Base, 2=Quote in API -> 0=Base, 1=Quote in program)
	var tokenSide limitorder.TokenSide
	if in.Side == 1 {
		tokenSide = limitorder.TokenSideBase // 0
	} else {
		tokenSide = limitorder.TokenSideQuote // 1
	}

	// Create deposit instruction
	depositIx, err := limitorder.NewDepositInstruction(
		limitorder.DepositAccounts{
			Config:         config,
			Market:         marketPda,
			Margin:         margin,
			Owner:          userWallet,
			BaseMint:       baseMint,
			QuoteMint:      quoteMint,
			UserBaseToken:  userBaseToken,
			UserQuoteToken: userQuoteToken,
			BaseVault:      baseVault,
			QuoteVault:     quoteVault,
			TokenProgram:   ag_solanago.TokenProgramID,
		},
		amountRaw,
		tokenSide,
		[][32]byte{}, // Empty whitelist proof for now
	)
	if err != nil {
		return "", fmt.Errorf("failed to create deposit instruction: %w", err)
	}

	// Add deposit instruction to the list
	instructions = append(instructions, depositIx)

	// Get recent blockhash
	recent, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Build unsigned transaction
	tx, err := ag_solanago.NewTransaction(
		instructions, // Use the instructions array (may include ATA creation + deposit)
		recent.Value.Blockhash,
		ag_solanago.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Serialize to base64
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(txBytes), nil
}

func (l *DepositMarginLogic) getMarketInfo(marketPda string) (*limitordermodel.LimitOrderMarket, error) {
	// Query from database
	marketModel := limitordermodel.NewLimitOrderMarketModel(l.svcCtx.DB)
	market, err := marketModel.FindOneByMarketPda(l.ctx, marketPda)
	if err != nil {
		return nil, fmt.Errorf("market not found: %w", err)
	}

	return market, nil
}

func pow10(n int32) uint64 {
	result := uint64(1)
	for i := int32(0); i < n; i++ {
		result *= 10
	}
	return result
}
