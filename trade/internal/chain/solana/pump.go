package solana

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/pumpfun"
	ammGenerated "richcode.cc/dex/pkg/pumpfun/generated/pump_amm"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/trade"
	"richcode.cc/dex/pkg/xcode"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	bin "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
)

const (
	PumpSwapAmmBase   = uint64(10000)
	PumpSwapAmmBuyFee = uint64(25) // base on 0.25%
	PumpSwapProgramID = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
	PumpAmmProgramID  = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"
)

var (
	pumpFeeConfigSeed = []byte{
		1, 86, 224, 246, 147, 102, 90, 207,
		68, 219, 21, 104, 191, 23, 91, 170,
		81, 137, 203, 151, 245, 210, 255, 59,
		101, 93, 43, 182, 253, 109, 24, 176,
	}
)

// PumpFun AMM 相关常量
var (
	// Global config account address for pumpfun AMM
	PumpAmmGlobalConfigAddress = ag_solanago.MustPublicKeyFromBase58("ADyA8hdefvWN2dbGGWFotbzWxrAvLW83WG6QCVXvJKqw")
	// Event authority address for pumpfun AMM
	PumpAmmEventAuthorityAddress = ag_solanago.MustPublicKeyFromBase58("GS4CU59F31iL7aR2Q8zVS8DRrcRnXX1yjQ66TqNVQnaR")
	// Fee program for devnet
	DevnetFeeProgram = ag_solanago.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ")
)

// GetPumpFunPDAs 获取 PumpFun (bonding curve) 所需的 PDA 账户
func GetPumpFunPDAs(mint ag_solanago.PublicKey) (global, feeRecipient, eventAuthority, feeConfig ag_solanago.PublicKey, err error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)

	// Global PDA
	globalSeeds := [][]byte{[]byte("global")}
	global, _, err = ag_solanago.FindProgramAddress(globalSeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find global PDA: %w", err)
	}

	// Event Authority PDA
	eventAuthoritySeeds := [][]byte{[]byte("__event_authority")}
	eventAuthority, _, err = ag_solanago.FindProgramAddress(eventAuthoritySeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find event authority PDA: %w", err)
	}

	// Fee recipient PDA
	feeRecipientSeeds := [][]byte{[]byte("fee_recipient")}
	feeRecipient, _, err = ag_solanago.FindProgramAddress(feeRecipientSeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find fee recipient PDA: %w", err)
	}

	// Fee config PDA (derived via fee program)
	feeProgramID := ag_solanago.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ")
	feeConfigSeeds := [][]byte{
		[]byte("fee_config"),
		pumpFeeConfigSeed,
	}
	feeConfig, _, err = ag_solanago.FindProgramAddress(feeConfigSeeds, feeProgramID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find fee config PDA: %w", err)
	}

	return global, feeRecipient, eventAuthority, feeConfig, nil
}

// GetPumpAmmPDAs 获取 PumpFun AMM 所需的 PDA 账户
func GetPumpAmmPDAs(user ag_solanago.PublicKey) (globalVolumeAccumulator, userVolumeAccumulator, eventAuthority, feeConfig ag_solanago.PublicKey, err error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID)

	// Global Volume Accumulator PDA
	globalVolumeAccumulatorSeeds := [][]byte{[]byte("global_volume_accumulator")}
	globalVolumeAccumulator, _, err = ag_solanago.FindProgramAddress(globalVolumeAccumulatorSeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find global volume accumulator PDA: %w", err)
	}

	// User Volume Accumulator PDA
	userVolumeAccumulatorSeeds := [][]byte{[]byte("user_volume_accumulator"), user.Bytes()}
	userVolumeAccumulator, _, err = ag_solanago.FindProgramAddress(userVolumeAccumulatorSeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find user volume accumulator PDA: %w", err)
	}

	// Event Authority PDA
	eventAuthoritySeeds := [][]byte{[]byte("__event_authority")}
	eventAuthority, _, err = ag_solanago.FindProgramAddress(eventAuthoritySeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find event authority PDA: %w", err)
	}

	// Fee program
	feeProgramID := ag_solanago.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ")
	feeConfigSeeds := [][]byte{[]byte("fee_config")}
	feeConfig, _, err = ag_solanago.FindProgramAddress(feeConfigSeeds, feeProgramID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find fee config PDA: %w", err)
	}

	return globalVolumeAccumulator, userVolumeAccumulator, eventAuthority, feeConfig, nil
}

// PumpAmmCreatorVaultResult holds the result of pump amm creator vault derivation
type PumpAmmCreatorVaultResult struct {
	PoolCreator                   ag_solanago.PublicKey
	CoinCreatorVaultAuthority     ag_solanago.PublicKey
	CoinCreatorVaultAta           ag_solanago.PublicKey
	CoinCreatorVaultAuthorityBump uint8
}

// GetPoolCreator fetches the pool account data and extracts the creator from PumpFun pools
func GetPoolCreator(ctx context.Context, client *ag_rpc.Client, poolAddress ag_solanago.PublicKey) (ag_solanago.PublicKey, error) {
	// Fetch the pool account data
	accountInfo, err := client.GetAccountInfo(ctx, poolAddress)
	if err != nil {
		return ag_solanago.PublicKey{}, fmt.Errorf("failed to fetch pool account: %w", err)
	}

	if accountInfo.Value == nil || accountInfo.Value.Data == nil {
		return ag_solanago.PublicKey{}, fmt.Errorf("pool account not found or has no data")
	}

	data := accountInfo.Value.Data.GetBinary()
	ownerProgram := accountInfo.Value.Owner.String()

	// Check if this is a PumpFun AMM pool
	if ownerProgram == PumpAmmProgramID {
		return extractCreatorFromPumpAmmPool(data)
	}

	// Check if this is an original PumpFun pool
	if ownerProgram == PumpSwapProgramID {
		// For original PumpFun pools, try to deserialize as generated pool account
		var poolAccount ammGenerated.Pool
		decoder := bin.NewBinDecoder(data)
		err = poolAccount.UnmarshalWithDecoder(decoder)
		if err != nil {
			return ag_solanago.PublicKey{}, fmt.Errorf("failed to deserialize original PumpFun pool account: %w", err)
		}
		return poolAccount.Creator, nil
	}

	return ag_solanago.PublicKey{}, fmt.Errorf("unsupported pool type, owner program: %s", ownerProgram)
}

// extractCreatorFromPumpAmmPool extracts creator from PumpFun AMM pool data
func extractCreatorFromPumpAmmPool(data []byte) (ag_solanago.PublicKey, error) {
	// Pool struct layout (after 8-byte discriminator):
	// - pool_bump: u8 (1 byte)
	// - index: u16 (2 bytes)
	// - creator: pubkey (32 bytes)
	// - base_mint: pubkey (32 bytes)
	// - quote_mint: pubkey (32 bytes)
	// - lp_mint: pubkey (32 bytes)
	// - pool_base_token_account: pubkey (32 bytes)
	// - pool_quote_token_account: pubkey (32 bytes)
	// - lp_supply: u64 (8 bytes)
	// - coin_creator: pubkey (32 bytes)

	coinCreatorOffset := 8 + 1 + 2 + 32 + 32 + 32 + 32 + 32 + 32 + 8 // 211 bytes offset

	if len(data) < coinCreatorOffset+32 {
		return ag_solanago.PublicKey{}, fmt.Errorf("account data too short for coin_creator field: need %d bytes, got %d", coinCreatorOffset+32, len(data))
	}

	coinCreatorBytes := data[coinCreatorOffset : coinCreatorOffset+32]
	coinCreator := ag_solanago.PublicKeyFromBytes(coinCreatorBytes)

	return coinCreator, nil
}

// GetCoinCreatorVaultAuthority derives the coin creator vault authority PDA
func GetCoinCreatorVaultAuthority(coinCreator ag_solanago.PublicKey) (ag_solanago.PublicKey, uint8, error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID)

	seeds := [][]byte{
		[]byte("creator_vault"),
		coinCreator.Bytes(),
	}

	pda, bump, err := ag_solanago.FindProgramAddress(seeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, 0, fmt.Errorf("failed to find coin creator vault authority PDA: %w", err)
	}

	return pda, bump, nil
}

// GetCoinCreatorVaultAta derives the coin creator vault ATA
func GetCoinCreatorVaultAta(coinCreatorVaultAuthority, quoteMint ag_solanago.PublicKey) (ag_solanago.PublicKey, error) {
	// Use FindAssociatedTokenAddress which internally handles the correct PDA derivation
	ata, _, err := ag_solanago.FindAssociatedTokenAddress(
		coinCreatorVaultAuthority,
		quoteMint,
	)
	if err != nil {
		return ag_solanago.PublicKey{}, fmt.Errorf("failed to find coin creator vault ATA: %w", err)
	}

	return ata, nil
}

// DeriveCoinCreatorVaultAccounts derives the new creator vault accounts required by PumpFun AMM
func DeriveCoinCreatorVaultAccounts(coinCreator ag_solanago.PublicKey, quoteMint ag_solanago.PublicKey) (coinCreatorVaultAuthority ag_solanago.PublicKey, coinCreatorVaultAta ag_solanago.PublicKey, err error) {
	// Step 1: Derive coin creator vault authority
	coinCreatorVaultAuthority, _, err = GetCoinCreatorVaultAuthority(coinCreator)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to derive coin creator vault authority: %w", err)
	}

	// Step 2: Derive coin creator vault ATA
	coinCreatorVaultAta, err = GetCoinCreatorVaultAta(coinCreatorVaultAuthority, quoteMint)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to derive coin creator vault ATA: %w", err)
	}

	return coinCreatorVaultAuthority, coinCreatorVaultAta, nil
}

// GetPoolCreatorVaultFromPool gets pool creator and creator vault info from a pump_amm pool address
func GetPoolCreatorVaultFromPool(ctx context.Context, client *ag_rpc.Client, poolAddress ag_solanago.PublicKey) (*PumpAmmCreatorVaultResult, error) {
	// Step 1: Fetch pool account data
	accountInfo, err := client.GetAccountInfo(ctx, poolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pool account: %w", err)
	}

	if accountInfo.Value == nil || accountInfo.Value.Data == nil {
		return nil, fmt.Errorf("pool account not found")
	}

	data := accountInfo.Value.Data.GetBinary()

	// Step 2: Extract coin creator from pool data
	coinCreator, err := extractCreatorFromPumpAmmPool(data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract coin creator: %w", err)
	}

	// Step 3: Get quote mint from pool data for ATA derivation
	quoteMintOffset := 8 + 1 + 2 + 32 + 32 // 75 bytes offset
	if len(data) < quoteMintOffset+32 {
		return nil, fmt.Errorf("account data too short for quote mint field")
	}
	quoteMintBytes := data[quoteMintOffset : quoteMintOffset+32]
	quoteMint := ag_solanago.PublicKeyFromBytes(quoteMintBytes)

	// Step 4: Derive coin creator vault authority
	coinCreatorVaultAuthority, bump, err := GetCoinCreatorVaultAuthority(coinCreator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin creator vault authority: %w", err)
	}

	// Step 5: Derive coin creator vault ATA
	coinCreatorVaultAta, err := GetCoinCreatorVaultAta(coinCreatorVaultAuthority, quoteMint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin creator vault ATA: %w", err)
	}

	return &PumpAmmCreatorVaultResult{
		PoolCreator:                   coinCreator,
		CoinCreatorVaultAuthority:     coinCreatorVaultAuthority,
		CoinCreatorVaultAta:           coinCreatorVaultAta,
		CoinCreatorVaultAuthorityBump: bump,
	}, nil
}

// ========================================
// PUMPFUN BONDING CURVE FUNCTIONS
// ========================================

// CreatorVaultResult holds the result of creator vault derivation for original PumpFun
type CreatorVaultResult struct {
	Creator      ag_solanago.PublicKey
	CreatorVault ag_solanago.PublicKey
	Bump         uint8
}

// GetBondingCurvePDA derives the bonding curve PDA from a mint address
func GetBondingCurvePDA(mint ag_solanago.PublicKey) (ag_solanago.PublicKey, uint8, error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)

	seeds := [][]byte{
		[]byte("bonding-curve"),
		mint.Bytes(),
	}

	pda, bump, err := ag_solanago.FindProgramAddress(seeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, 0, fmt.Errorf("failed to find bonding curve PDA: %w", err)
	}

	return pda, bump, nil
}

// GetCreatorVaultPDA derives the creator vault PDA from a creator address for original PumpFun
func GetCreatorVaultPDA(creator ag_solanago.PublicKey) (ag_solanago.PublicKey, uint8, error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)

	seeds := [][]byte{
		[]byte("creator-vault"),
		creator.Bytes(),
	}

	pda, bump, err := ag_solanago.FindProgramAddress(seeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, 0, fmt.Errorf("failed to find creator vault PDA: %w", err)
	}

	return pda, bump, nil
}

// GetCreatorVaultFromMint gets the creator and creator vault from a token mint address for original PumpFun
func GetCreatorVaultFromMint(ctx context.Context, client *ag_rpc.Client, mint ag_solanago.PublicKey) (*CreatorVaultResult, error) {
	// Step 1: Get bonding curve PDA
	bondingCurvePDA, _, err := GetBondingCurvePDA(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive bonding curve PDA: %w", err)
	}

	// Step 2: Fetch bonding curve account data
	accountInfo, err := client.GetAccountInfo(ctx, bondingCurvePDA)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bonding curve account: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil || accountInfo.Value.Data == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	data := accountInfo.Value.Data.GetBinary()

	// Step 3: Parse the creator from the account data
	// BondingCurve struct layout (after 8-byte discriminator):
	// - virtual_token_reserves: u64 (8 bytes)
	// - virtual_sol_reserves: u64 (8 bytes)
	// - real_token_reserves: u64 (8 bytes)
	// - real_sol_reserves: u64 (8 bytes)
	// - token_total_supply: u64 (8 bytes)
	// - complete: bool (1 byte)
	// - creator: pubkey (32 bytes)

	creatorOffset := 8 + 8 + 8 + 8 + 8 + 8 + 1 // 8 (discriminator) + 41 (struct fields) = 49 bytes offset

	if len(data) < creatorOffset+32 {
		return nil, fmt.Errorf("account data too short for creator field: need %d bytes, got %d", creatorOffset+32, len(data))
	}

	creatorBytes := data[creatorOffset : creatorOffset+32]
	creator := ag_solanago.PublicKeyFromBytes(creatorBytes)

	// Step 4: Get creator vault PDA
	creatorVault, bump, err := GetCreatorVaultPDA(creator)
	if err != nil {
		return nil, fmt.Errorf("failed to derive creator vault PDA: %w", err)
	}

	return &CreatorVaultResult{
		Creator:      creator,
		CreatorVault: creatorVault,
		Bump:         bump,
	}, nil
}

func buy(
	ctx context.Context,
	solAmountStr string,
	slippage uint32,
	poolBaseTokenAccount, poolQuoteTokenAccount ag_solanago.PublicKey,
	cli *ag_rpc.Client,
) (uint64, uint64, error) {
	solAmountIn, ok := big.NewInt(0).SetString(solAmountStr, 10)
	if !ok {
		return 0, 0, fmt.Errorf("invalid sol amount: %s", solAmountStr)
	}

	amounts, err := GetMulTokenBalance(ctx, cli, poolQuoteTokenAccount, poolBaseTokenAccount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get token balance: %v", err)
	}

	baseAmountOut, err := trade.CalcPSBaseAmountOut(amounts, solAmountIn, PumpSwapAmmBuyFee, PumpSwapAmmBase)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate base amount out: %v", err)
	}

	slippageRatio := big.NewInt(0).Sub(
		big.NewInt(100),
		big.NewInt(int64(slippage)),
	)
	baseAmountOutAdjusted := big.NewInt(0).Mul(
		big.NewInt(0).SetUint64(baseAmountOut),
		slippageRatio,
	)
	baseAmountOutAdjusted = big.NewInt(0).Div(baseAmountOutAdjusted, big.NewInt(100))

	maxQuoteAmountIn := solAmountIn

	return baseAmountOutAdjusted.Uint64(), maxQuoteAmountIn.Uint64(), nil
}

func sell(
	ctx context.Context,
	baseAmountInStr string,
	slippage uint32,
	poolBaseTokenAccount, poolQuoteTokenAccount ag_solanago.PublicKey,
	cli *ag_rpc.Client,
) (uint64, uint64, error) {
	baseAmountIn, ok := big.NewInt(0).SetString(baseAmountInStr, 10)
	if !ok {
		return 0, 0, fmt.Errorf("invalid base amount: %s", baseAmountInStr)
	}

	amounts, err := GetMulTokenBalance(ctx, cli, poolQuoteTokenAccount, poolBaseTokenAccount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get token balance: %v", err)
	}

	minQuoteAmountOut, err := trade.CalcPSMinQuoteAmountOut(amounts, baseAmountIn, slippage, PumpSwapAmmBuyFee, PumpSwapAmmBase)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate min quote amount out: %v", err)
	}

	return baseAmountIn.Uint64(), minQuoteAmountOut, nil
}

// createPumpSwapInstruction 创建 PumpFun AMM 的 swap 指令
// 注意：这个函数用于PumpFun AMM，需要包含 creator vault 账户
func createPumpSwapInstruction(
	ctx context.Context,
	in *CreateMarketTx,
	isBuy bool,
	inAta, outAta ag_solanago.PublicKey,
	poolBaseTokenAccount ag_solanago.PublicKey,
	poolQuoteTokenAccount ag_solanago.PublicKey,
	protocolFeeRecipient ag_solanago.PublicKey,
	protocolFeeRecipientTokenAccount ag_solanago.PublicKey, // baseTokenAccount for protocol fee recipient
	cli *ag_rpc.Client,
) (ag_solanago.Instruction, uint64, uint64, error) {
	// 获取 pool creator 和 creator vault 账户
	poolAddress := ag_solanago.MustPublicKeyFromBase58(in.PairAddr)
	coinCreator, err := GetPoolCreator(ctx, cli, poolAddress)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get pool creator: %w", err)
	}

	// 对于 buy，quote mint 是 in.InMint (SOL/WSOL)
	// 对于 sell，quote mint 是 in.OutMint (SOL/WSOL)
	quoteMint := in.InMint
	if !isBuy {
		quoteMint = in.OutMint
	}

	// Derive creator vault accounts
	coinCreatorVaultAuthority, coinCreatorVaultAta, err := DeriveCoinCreatorVaultAccounts(coinCreator, quoteMint)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to derive creator vault accounts: %w", err)
	}

	// 获取 PumpFun AMM PDAs
	globalVolumeAccumulator, userVolumeAccumulator, eventAuthority, feeConfig, err := GetPumpAmmPDAs(in.UserWalletAccount)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get pump amm PDAs: %w", err)
	}

	if isBuy {
		baseAmountOut, maxQuoteAmountIn, err := buy(ctx, in.AmountIn, in.Slippage, poolBaseTokenAccount, poolQuoteTokenAccount, cli)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to calculate buy amounts: %v", err)
		}

		// InMint is quoteTokenAccount, OutMint is baseTokenAccount
		baseMint := in.OutMint
		quoteMint := in.InMint
		userBaseTokenAccount := outAta
		userQuoteTokenAccount := inAta

		// 使用 pump_amm 的 Buy 指令
		instruction, err := ammGenerated.NewBuyInstruction(
			baseAmountOut,
			maxQuoteAmountIn,
			ammGenerated.OptionBool{V0: true}, // trackVolumeParam
			poolAddress,
			in.UserWalletAccount,
			PumpAmmGlobalConfigAddress,
			baseMint,
			quoteMint,
			userBaseTokenAccount,
			userQuoteTokenAccount,
			poolBaseTokenAccount,
			poolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientTokenAccount,
			in.OutTokenProgram, // base token program
			in.InTokenProgram,  // quote token program
			ag_solanago.SystemProgramID,
			ag_solanago.SPLAssociatedTokenAccountProgramID,
			eventAuthority,
			ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID),
			coinCreatorVaultAta,
			coinCreatorVaultAuthority,
			globalVolumeAccumulator,
			userVolumeAccumulator,
			feeConfig,
			DevnetFeeProgram,
		)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to build buy instruction: %v", err)
		}
		return instruction, baseAmountOut, maxQuoteAmountIn, nil
	} else {
		baseAmountIn, minQuoteAmountOut, err := sell(ctx, in.AmountIn, in.Slippage, poolBaseTokenAccount, poolQuoteTokenAccount, cli)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to calculate sell amounts: %v", err)
		}

		// InMint is baseTokenAccount, OutMint is quoteTokenAccount
		baseMint := in.InMint
		quoteMint := in.OutMint
		userBaseTokenAccount := inAta
		userQuoteTokenAccount := outAta

		// 使用 pump_amm 的 Sell 指令
		instruction, err := ammGenerated.NewSellInstruction(
			baseAmountIn,
			minQuoteAmountOut,
			poolAddress,
			in.UserWalletAccount,
			PumpAmmGlobalConfigAddress,
			baseMint,
			quoteMint,
			userBaseTokenAccount,
			userQuoteTokenAccount,
			poolBaseTokenAccount,
			poolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientTokenAccount,
			in.InTokenProgram,  // base token program
			in.OutTokenProgram, // quote token program
			ag_solanago.SystemProgramID,
			ag_solanago.SPLAssociatedTokenAccountProgramID,
			eventAuthority,
			ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID),
			coinCreatorVaultAta,
			coinCreatorVaultAuthority,
			feeConfig,
			DevnetFeeProgram,
		)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to build sell instruction: %v", err)
		}
		return instruction, baseAmountIn, minQuoteAmountOut, nil
	}
}

// createPumpSwapInstructionV2 创建 PumpFun AMM 的 swap 指令（V2版本）
// 注意：这个函数用于PumpFun AMM，需要包含 creator vault 账户
func createPumpSwapInstructionV2(ctx context.Context, db *gorm.DB, in *CreateMarketTx, amtUint64 uint64, isBuy bool, inAta, outAta ag_solanago.PublicKey, cli *ag_rpc.Client) (ag_solanago.Instruction, uint64, uint64, error) {
	ammKeys, err := solmodel.NewPumpAmmInfoModel(db).FindOneByPoolAccount(ctx, in.PairAddr)
	if err != nil {
		return nil, 0, 0, err
	}

	// Get pool creator from on-chain data
	poolAddress := ag_solanago.MustPublicKeyFromBase58(in.PairAddr)
	coinCreator, err := GetPoolCreator(ctx, cli, poolAddress)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get pool creator: %w", err)
	}

	logx.WithContext(ctx).Infof("coinCreator is: %s", coinCreator.String())

	// 对于 buy，quote mint 是 in.InMint (SOL/WSOL)
	// 对于 sell，quote mint 是 in.OutMint (SOL/WSOL)
	quoteMint := in.InMint
	if !isBuy {
		quoteMint = in.OutMint
	}

	// Derive creator vault accounts
	coinCreatorVaultAuthority, coinCreatorVaultAta, err := DeriveCoinCreatorVaultAccounts(coinCreator, quoteMint)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to derive creator vault accounts: %w", err)
	}

	logx.WithContext(ctx).Infof("coinCreatorVaultAuthority: %s, coinCreatorVaultAta: %s",
		coinCreatorVaultAuthority.String(), coinCreatorVaultAta.String())

	// 获取 PumpFun AMM PDAs
	globalVolumeAccumulator, userVolumeAccumulator, eventAuthority, feeConfig, err := GetPumpAmmPDAs(in.UserWalletAccount)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get pump amm PDAs: %w", err)
	}

	var minAmountOut, amountOut uint64
	if in.UsePriceLimit {
		priceDecimal, err := decimal.NewFromString(in.Price)
		if err != nil {
			return nil, 0, 0, err
		}
		minAmountOut, amountOut, err = trade.CalcMinAmountOutByPrice(in.Slippage, amtUint64, isBuy, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal, trade.PumpSwapFee)
		if nil != err {
			return nil, 0, 0, err
		}
	} else {
		poolTokenAccount, err := ag_solanago.PublicKeyFromBase58(ammKeys.PoolBaseTokenAccount)
		if err != nil {
			return nil, 0, 0, err
		}
		poolSolAccount, err := ag_solanago.PublicKeyFromBase58(ammKeys.PoolQuoteTokenAccount)
		if err != nil {
			return nil, 0, 0, err
		}

		amounts, err := GetMulTokenBalance(ctx, cli, poolTokenAccount, poolSolAccount)
		if nil != err {
			return nil, 0, 0, err
		}
		if len(amounts) != 2 {
			logc.Errorf(ctx, "GetMulTokenBalance len(amounts) != 2, poolTokenAccount:%s,poolSolAccount:%s", poolTokenAccount.String(), poolSolAccount.String())
			return nil, 0, 0, xcode.InternalError
		}
		minAmountOut, amountOut, err = trade.CalcMinAmountOutByAmm(in.Slippage, amtUint64, isBuy, amounts[0], amounts[1], trade.PumpSwapFee)
		if nil != err {
			return nil, 0, 0, err
		}
	}

	// pumpswap的base是meme quote是sol
	poolBaseTokenAccount := ag_solanago.MustPublicKeyFromBase58(ammKeys.PoolBaseTokenAccount)
	poolQuoteTokenAccount := ag_solanago.MustPublicKeyFromBase58(ammKeys.PoolQuoteTokenAccount)
	protocolFeeRecipient := ag_solanago.MustPublicKeyFromBase58(ammKeys.ProtocolFeeRecipientAccount)
	protocolFeeRecipientTokenAccount := ag_solanago.MustPublicKeyFromBase58(ammKeys.ProtocolFeeRecipientTokenAccount)

	var instruction ag_solanago.Instruction
	if isBuy {
		// Buy: InMint=WSOL, OutMint=Token
		instruction, err = ammGenerated.NewBuyInstruction(
			minAmountOut,                      // baseAmountOut
			amtUint64,                         // maxQuoteAmountIn
			ammGenerated.OptionBool{V0: true}, // trackVolumeParam
			poolAddress,
			in.UserWalletAccount,
			PumpAmmGlobalConfigAddress,
			in.OutMint, // baseMint
			in.InMint,  // quoteMint
			outAta,     // userBaseTokenAccount
			inAta,      // userQuoteTokenAccount
			poolBaseTokenAccount,
			poolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientTokenAccount,
			in.OutTokenProgram, // baseTokenProgram
			in.InTokenProgram,  // quoteTokenProgram
			ag_solanago.SystemProgramID,
			ag_solanago.SPLAssociatedTokenAccountProgramID,
			eventAuthority,
			ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID),
			coinCreatorVaultAta,
			coinCreatorVaultAuthority,
			globalVolumeAccumulator,
			userVolumeAccumulator,
			feeConfig,
			DevnetFeeProgram,
		)
	} else {
		// Sell: InMint=Token, OutMint=WSOL
		instruction, err = ammGenerated.NewSellInstruction(
			amtUint64,    // baseAmountIn
			minAmountOut, // minQuoteAmountOut
			poolAddress,
			in.UserWalletAccount,
			PumpAmmGlobalConfigAddress,
			in.InMint,  // baseMint
			in.OutMint, // quoteMint
			inAta,      // userBaseTokenAccount
			outAta,     // userQuoteTokenAccount
			poolBaseTokenAccount,
			poolQuoteTokenAccount,
			protocolFeeRecipient,
			protocolFeeRecipientTokenAccount,
			in.InTokenProgram,  // baseTokenProgram
			in.OutTokenProgram, // quoteTokenProgram
			ag_solanago.SystemProgramID,
			ag_solanago.SPLAssociatedTokenAccountProgramID,
			eventAuthority,
			ag_solanago.MustPublicKeyFromBase58(PumpAmmProgramID),
			coinCreatorVaultAta,
			coinCreatorVaultAuthority,
			feeConfig,
			DevnetFeeProgram,
		)
	}

	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to build swap instruction: %v", err)
	}

	return instruction, minAmountOut, amountOut, nil
}

func (tm *TxManager) CreateMarketOrder4Pumpfun(ctx context.Context, in *CreateMarketTx) ([]ag_solanago.Instruction, error) {
	fmt.Println("********************创建购买pumpfun代币的交易****************************")
	initiator := in.UserWalletAccount

	//需要创建1个ata账户，尽管token 账户可能存在，但是为了避免网络请求，不做判断，直接认为需要这个费用
	lamportCost := tm.rentFee

	if in.InMint != ag_solanago.WrappedSol && in.OutMint != ag_solanago.WrappedSol {
		return nil, errors.New("ErrWrongMint")
	}
	swapDirection := sol.Swap_Direction_Buy
	tokenMint := in.OutMint

	if in.OutMint == ag_solanago.WrappedSol {
		swapDirection = sol.Swap_Direction_Sell
		tokenMint = in.InMint
	}

	logx.WithContext(ctx).Infof("BuyInPumpfun is initlizated by %s", initiator.String())

	amtDecimal, err := decimal.NewFromString(in.AmountIn)
	if nil != err {
		return nil, err
	}
	amtDecimal = amtDecimal.Mul(decimal.NewFromInt(sol.Decimals2Value[in.InDecimal]))
	amountUint64 := uint64(amtDecimal.IntPart())

	priceDecimal, err := decimal.NewFromString(in.Price)
	if err != nil {
		return nil, err
	}

	// #1 - Compute Budget: SetComputeUnitPrice
	instructions, lamportCostFee, err := tm.CreateGasAndJitoByGasFee(ctx, in.IsAntiMev, initiator, sol.PumpFunSwapCU, sol.GasMODE[1])
	lamportCost += lamportCostFee
	if nil != err {
		return nil, err
	}

	// #3 - Associated Token Account Program: CreateIdempotent
	// pumpfun 的目前都是 普通的token
	instructionNew, err := sol.CreateAtaIdempotent(initiator, initiator, tokenMint, ag_solanago.TokenProgramID)
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, instructionNew)

	// #4 - System Program: Transfer, if in mint is wrapper sol
	// #5 - Token Program: SyncNative
	solBalanceInfo, err := tm.Client.GetBalance(ctx, initiator, ag_rpc.CommitmentFinalized)
	if nil != err {
		return nil, err
	}
	solBalance := solBalanceInfo.Value

	serviceFee := uint64(0)

	if swapDirection == sol.Swap_Direction_Buy {
		if solBalance < uint64(amtDecimal.IntPart()) {
			return nil, xcode.SolBalanceNotEnough
		}

		// 计算服务费，实际购买的sol数量*1%
		serviceFee = uint64(amtDecimal.Mul(sol.ServericeFeePercent).IntPart())
		lamportCost += serviceFee + amountUint64
	}
	if lamportCost > solBalance {
		logx.WithContext(ctx).Errorf("Insufficient SOL: need %d lamports, have %d lamports (%.9f SOL needed, %.9f SOL available)",
			lamportCost, solBalance, float64(lamportCost)/1e9, float64(solBalance)/1e9)
		logx.WithContext(ctx).Errorf("Cost breakdown: rentFee=%d, serviceFee=%d, amountUint64=%d, totalCost=%d",
			tm.rentFee, serviceFee, amountUint64, lamportCost)
		return nil, xcode.SolGasNotEnough
	}

	// //////////////////////////////////////////////////////
	// #7 - Build Buy Instruction
	if swapDirection == sol.Swap_Direction_Buy {
		logx.WithContext(ctx).Infof("CreateMarketOrder4Pumpfun::Swap_Direction_Buy, amount to buy in sol=%d", amountUint64)

		// 构建花费精确SOL数量的购买指令，价格滑点按照用户输入的百分比来计算
		buyInstruction, err := pumpfun.BuildBuyInstruction(initiator, tokenMint, amountUint64, in.Slippage, tm.Client, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal)
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, buyInstruction)
	} else {
		logx.WithContext(ctx).Infof("CreateMarketOrder4Pumpfun::Swap_Direction_Sell, amount to sell = %d, in.InDecimal=%d", amountUint64, in.InDecimal)
		tokenAta, _, err := ag_solanago.FindAssociatedTokenAddress(initiator, tokenMint)
		if nil != err {
			return nil, err
		}

		buyInstruction, solVolume, err := pumpfun.BuildSellInstruction(tokenAta, initiator, tokenMint, amountUint64, in.Slippage, false, tm.Client, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal)
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, buyInstruction)

		out := decimal.NewFromInt(int64(solVolume))
		serviceFee = uint64(out.Mul(sol.ServericeFeePercent).IntPart())
	}

	return instructions, nil
}
