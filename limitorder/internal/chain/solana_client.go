package chain

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// Program ID for the limit order program
	LimitOrderProgramID = "C4Xn6ACR3XmTwpWm2fN82QSAx23TQkwVW8wro9Hdpqef"
)

type SolanaClient struct {
	rpcClient *rpc.Client
	programID solana.PublicKey
}

func NewSolanaClient(rpcEndpoint string) (*SolanaClient, error) {
	programID, err := solana.PublicKeyFromBase58(LimitOrderProgramID)
	if err != nil {
		return nil, fmt.Errorf("invalid program ID: %w", err)
	}

	return &SolanaClient{
		rpcClient: rpc.New(rpcEndpoint),
		programID: programID,
	}, nil
}

// FindMarginPDA calculates the PDA for a margin account
// Seeds: ["margin", market_pubkey, owner_pubkey]
func (c *SolanaClient) FindMarginPDA(marketPubkey, ownerPubkey solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("margin"),
		marketPubkey.Bytes(),
		ownerPubkey.Bytes(),
	}

	pda, bump, err := solana.FindProgramAddress(seeds, c.programID)
	if err != nil {
		return solana.PublicKey{}, 0, fmt.Errorf("failed to find margin PDA: %w", err)
	}

	return pda, bump, nil
}

// BuildInitializeMarginTransaction builds an unsigned transaction for initializing a margin account
func (c *SolanaClient) BuildInitializeMarginTransaction(
	ctx context.Context,
	marketPubkey solana.PublicKey,
	ownerPubkey solana.PublicKey,
) (string, solana.PublicKey, error) {
	// Calculate margin account PDA
	marginPDA, _, err := c.FindMarginPDA(marketPubkey, ownerPubkey)
	if err != nil {
		return "", solana.PublicKey{}, err
	}

	// Get latest blockhash
	recentBlockhash, err := c.rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Get config PDA (needed for whitelist verification)
	configPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("config")},
		c.programID,
	)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to find config PDA: %w", err)
	}

	// Get market info to retrieve base_mint and quote_mint
	marketInfo, err := c.rpcClient.GetAccountInfoWithOpts(ctx, marketPubkey, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	})
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to get market info: %w", err)
	}

	// Parse market account to get base_mint and quote_mint
	// Anchor account layout: 8 bytes discriminator + base_mint (32 bytes) + quote_mint (32 bytes) + ...
	if marketInfo == nil || marketInfo.Value == nil || len(marketInfo.Value.Data.GetBinary()) < 8+64 {
		return "", solana.PublicKey{}, fmt.Errorf("invalid market account data")
	}

	marketData := marketInfo.Value.Data.GetBinary()
	offset := 8
	baseMint := solana.PublicKeyFromBytes(marketData[offset : offset+32])
	quoteMint := solana.PublicKeyFromBytes(marketData[offset+32 : offset+64])

	// Derive user's associated token accounts
	userBaseToken, _, err := solana.FindAssociatedTokenAddress(ownerPubkey, baseMint)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to find user base token account: %w", err)
	}

	userQuoteToken, _, err := solana.FindAssociatedTokenAddress(ownerPubkey, quoteMint)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to find user quote token account: %w", err)
	}

	// Get associated token program ID
	associatedTokenProgramID := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	// Build the create_margin instruction
	// Anchor discriminator for create_margin from IDL: [19, 155, 72, 104, 164, 192, 3, 68]
	discriminator := []byte{19, 155, 72, 104, 164, 192, 3, 68}

	// Empty whitelist proof (Vec<[u8; 32]> with length 0)
	whitelistProof := []byte{0x00, 0x00, 0x00, 0x00} // 32-bit length = 0

	instructionData := append(discriminator, whitelistProof...)

	// Create the instruction with account metas (matching IDL order)
	// From IDL: config, market, margin, owner, base_mint, quote_mint, user_base_token, user_quote_token, token_program, associated_token_program, system_program
	accounts := []*solana.AccountMeta{
		{PublicKey: configPDA, IsWritable: false, IsSigner: false},                // config
		{PublicKey: marketPubkey, IsWritable: false, IsSigner: false},             // market
		{PublicKey: marginPDA, IsWritable: true, IsSigner: false},                 // margin (to be created)
		{PublicKey: ownerPubkey, IsWritable: true, IsSigner: true},                // owner (payer + signer)
		{PublicKey: baseMint, IsWritable: false, IsSigner: false},                 // base_mint
		{PublicKey: quoteMint, IsWritable: false, IsSigner: false},                // quote_mint
		{PublicKey: userBaseToken, IsWritable: true, IsSigner: false},             // user_base_token
		{PublicKey: userQuoteToken, IsWritable: true, IsSigner: false},            // user_quote_token
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},    // token_program
		{PublicKey: associatedTokenProgramID, IsWritable: false, IsSigner: false}, // associated_token_program
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},   // system_program
	}

	instruction := solana.NewInstruction(
		c.programID,
		accounts,
		instructionData,
	)

	// Build transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(ownerPubkey),
	)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Initialize empty signatures (user will sign on frontend)
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]solana.Signature, numSigners)

	// Serialize transaction to base64
	txData, err := tx.MarshalBinary()
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	txBase64 := base64.StdEncoding.EncodeToString(txData)

	return txBase64, marginPDA, nil
}

// GetAccountInfo 获取账户信息
func (c *SolanaClient) GetAccountInfo(ctx context.Context, account solana.PublicKey) (*rpc.GetAccountInfoResult, error) {
	return c.rpcClient.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	})
}

// FindMarketPDA calculates the PDA for a market
// Seeds: ["LimitOrderMarket", base_mint, quote_mint]
func (c *SolanaClient) FindMarketPDA(baseMint, quoteMint solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("LimitOrderMarket"),
		baseMint.Bytes(),
		quoteMint.Bytes(),
	}

	pda, bump, err := solana.FindProgramAddress(seeds, c.programID)
	if err != nil {
		return solana.PublicKey{}, 0, fmt.Errorf("failed to find market PDA: %w", err)
	}

	return pda, bump, nil
}

// FindVaultPDA calculates the PDA for a token vault
// Seeds: ["MarketVault", market, mint]
// NOTE: Vaults are now market-specific for complete isolation
func (c *SolanaClient) FindVaultPDA(market solana.PublicKey, mint solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("MarketVault"),
		market.Bytes(),
		mint.Bytes(),
	}

	pda, bump, err := solana.FindProgramAddress(seeds, c.programID)
	if err != nil {
		return solana.PublicKey{}, 0, fmt.Errorf("failed to find vault PDA: %w", err)
	}

	return pda, bump, nil
}

// GetProgramID returns the limit order program ID
func (c *SolanaClient) GetProgramID() solana.PublicKey {
	return c.programID
}

// GetLatestBlockhash gets the latest blockhash
func (c *SolanaClient) GetLatestBlockhash(ctx context.Context) (solana.Hash, error) {
	resp, err := c.rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Hash{}, err
	}
	return resp.Value.Blockhash, nil
}

// BuildInitializeMarketTransaction builds an unsigned transaction for initializing a market AND vaults
// NOTE: Vaults are now created atomically with the market in a single transaction
func (c *SolanaClient) BuildInitializeMarketTransaction(
	ctx context.Context,
	adminPubkey solana.PublicKey,
	baseMint solana.PublicKey,
	quoteMint solana.PublicKey,
	tickSize uint64,
	minBaseLot uint64,
	minQuoteLot uint64,
	makerFeeBps uint16,
	takerFeeBps uint16,
	pythPriceFeed solana.PublicKey,
) (string, solana.PublicKey, error) {
	// Calculate PDAs
	marketPDA, _, err := c.FindMarketPDA(baseMint, quoteMint)
	if err != nil {
		return "", solana.PublicKey{}, err
	}

	baseVaultPDA, _, err := c.FindVaultPDA(marketPDA, baseMint)
	if err != nil {
		return "", solana.PublicKey{}, err
	}

	quoteVaultPDA, _, err := c.FindVaultPDA(marketPDA, quoteMint)
	if err != nil {
		return "", solana.PublicKey{}, err
	}

	// Get config PDA
	configPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("config")},
		c.programID,
	)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to find config PDA: %w", err)
	}

	// Get latest blockhash
	recentBlockhash, err := c.rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Build init_market instruction (now includes vault creation)
	// Anchor discriminator for init_market from IDL: [33, 253, 15, 116, 89, 25, 127, 236]
	initMarketDiscriminator := []byte{33, 253, 15, 116, 89, 25, 127, 236}

	// Determine oracle_type based on pythPriceFeed
	var oracleType uint8
	if pythPriceFeed == solana.SystemProgramID {
		oracleType = 0 // PriceOracleType::None
	} else {
		oracleType = 1 // PriceOracleType::Pyth
	}

	// Serialize InitMarketParams using Anchor/Borsh serialization
	// Fields: tick_size, min_base_lot, min_quote_lot, maker_fee_bps, taker_fee_bps, oracle_type, pyth_price_feed_id
	initMarketData := make([]byte, 0, 8+8+8+8+2+2+1+32)
	initMarketData = append(initMarketData, initMarketDiscriminator...)
	initMarketData = append(initMarketData, uint64ToBytes(tickSize)...)
	initMarketData = append(initMarketData, uint64ToBytes(minBaseLot)...)
	initMarketData = append(initMarketData, uint64ToBytes(minQuoteLot)...)
	initMarketData = append(initMarketData, uint16ToBytes(makerFeeBps)...)
	initMarketData = append(initMarketData, uint16ToBytes(takerFeeBps)...)
	initMarketData = append(initMarketData, oracleType)               // oracle_type enum (u8)
	initMarketData = append(initMarketData, pythPriceFeed.Bytes()...) // pyth_price_feed_id ([u8; 32])

	// Create init_market instruction with account metas
	// Account order from IDL:
	// 1. config (PDA)
	// 2. admin (mut, signer)
	// 3. market (init, mut, PDA)
	// 4. base_mint
	// 5. quote_mint
	// 6. base_vault (init, mut, PDA)
	// 7. quote_vault (init, mut, PDA)
	// 8. token_program
	// 9. system_program
	initMarketAccounts := []*solana.AccountMeta{
		{PublicKey: configPDA, IsWritable: false, IsSigner: false},              // config
		{PublicKey: adminPubkey, IsWritable: true, IsSigner: true},              // admin (payer + signer)
		{PublicKey: marketPDA, IsWritable: true, IsSigner: false},               // market (to be created)
		{PublicKey: baseMint, IsWritable: false, IsSigner: false},               // base_mint
		{PublicKey: quoteMint, IsWritable: false, IsSigner: false},              // quote_mint
		{PublicKey: baseVaultPDA, IsWritable: true, IsSigner: false},            // base_vault (to be created)
		{PublicKey: quoteVaultPDA, IsWritable: true, IsSigner: false},           // quote_vault (to be created)
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},  // token_program
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false}, // system_program
	}

	initMarketInstruction := solana.NewInstruction(
		c.programID,
		initMarketAccounts,
		initMarketData,
	)

	logx.Infof("Building init_market transaction (includes vault creation)")

	// Build transaction with init_market instruction only
	tx, err := solana.NewTransaction(
		[]solana.Instruction{initMarketInstruction},
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(adminPubkey),
	)
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Initialize empty signatures
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]solana.Signature, numSigners)

	// Serialize transaction to base64
	txData, err := tx.MarshalBinary()
	if err != nil {
		return "", solana.PublicKey{}, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	txBase64 := base64.StdEncoding.EncodeToString(txData)

	return txBase64, marketPDA, nil
}

// Helper functions for serialization
func uint64ToBytes(val uint64) []byte {
	b := make([]byte, 8)
	b[0] = byte(val)
	b[1] = byte(val >> 8)
	b[2] = byte(val >> 16)
	b[3] = byte(val >> 24)
	b[4] = byte(val >> 32)
	b[5] = byte(val >> 40)
	b[6] = byte(val >> 48)
	b[7] = byte(val >> 56)
	return b
}

func uint16ToBytes(val uint16) []byte {
	return []byte{byte(val), byte(val >> 8)}
}

// GetMarketAccount fetches market account data from the blockchain
func (c *SolanaClient) GetMarketAccount(ctx context.Context, marketPDA solana.PublicKey) (bool, error) {
	// Get account info from blockchain
	accountInfo, err := c.GetAccountInfo(ctx, marketPDA)
	if err != nil {
		return false, fmt.Errorf("failed to get market account info: %w", err)
	}

	// If account is nil or has no data, market doesn't exist on-chain
	if accountInfo == nil || accountInfo.Value == nil || len(accountInfo.Value.Data.GetBinary()) == 0 {
		return false, nil
	}

	// Market exists on-chain
	return true, nil
}
