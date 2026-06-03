package pumpfun

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"richcode.cc/dex/pkg/pumpfun/generated/pump"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
)

const (
	PumpSwapProgramID = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
	PumpSwapAmmBase   = uint64(10000)
	PumpSwapAmmBuyFee = uint64(25) // 0.25%
)

var (
	// pumpFeeConfigSeed is the second seed required by PumpFun's fee_config PDA.
	pumpFeeConfigSeed = []byte{
		1, 86, 224, 246, 147, 102, 90, 207,
		68, 219, 21, 104, 191, 23, 91, 170,
		81, 137, 203, 151, 245, 210, 255, 59,
		101, 93, 43, 182, 253, 109, 24, 176,
	}
)

// BondingCurveState captures reserves and metadata for a PumpFun bonding curve account.
type BondingCurveState struct {
	BondingCurve         ag_solanago.PublicKey
	VirtualTokenReserves uint64
	VirtualSolReserves   uint64
	Creator              ag_solanago.PublicKey
}

// GetBondingCurveState fetches the latest bonding-curve info required for quotes or instruction building.
func GetBondingCurveState(ctx context.Context, client *ag_rpc.Client, mint ag_solanago.PublicKey) (*BondingCurveState, error) {
	if client == nil {
		return nil, fmt.Errorf("nil rpc client")
	}
	bondingCurve, _, err := GetBondingCurvePDA(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve PDA: %w", err)
	}

	accountInfo, err := client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil || accountInfo.Value.Data == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	data := accountInfo.Value.Data.GetBinary()
	if len(data) < 24 {
		return nil, fmt.Errorf("invalid bonding curve data length")
	}

	// solana上int64和uint64都是8字节，且是小端序存储，所以需要反转字节顺序后转换成uint64
	virtualTokenReserves := new(big.Int).SetBytes(reverseBytes(data[8:16])).Uint64()
	virtualSolReserves := new(big.Int).SetBytes(reverseBytes(data[16:24])).Uint64()

	creatorOffset := 8 + 8 + 8 + 8 + 8 + 8 + 1
	if len(data) < creatorOffset+32 {
		return nil, fmt.Errorf("bonding curve data too short for creator field")
	}
	creatorBytes := data[creatorOffset : creatorOffset+32]
	creator := ag_solanago.PublicKeyFromBytes(creatorBytes)

	return &BondingCurveState{
		BondingCurve:         bondingCurve,
		VirtualTokenReserves: virtualTokenReserves,
		VirtualSolReserves:   virtualSolReserves,
		Creator:              creator,
	}, nil
}

// GetPumpFunPDAs 获取 PumpFun (bonding curve) 所需的 PDA 账户
func GetPumpFunPDAs(mint ag_solanago.PublicKey) (global, feeRecipient, eventAuthority, feeConfig ag_solanago.PublicKey, err error) {
	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)

	// Global PDA
	globalSeeds := [][]byte{[]byte("global")}
	global, _, err = ag_solanago.FindProgramAddress(globalSeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find global PDA: %w", err)
	}

	// Fee recipient - 通常是固定地址(保存在global的data中)
	feeRecipient = ag_solanago.MustPublicKeyFromBase58("6QgPshH1egekJ2TURfakiiApDdv98qfRuRe7RectX8xs")

	// Event Authority PDA
	eventAuthoritySeeds := [][]byte{[]byte("__event_authority")}
	eventAuthority, _, err = ag_solanago.FindProgramAddress(eventAuthoritySeeds, programID)
	if err != nil {
		return ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, ag_solanago.PublicKey{}, fmt.Errorf("failed to find event authority PDA: %w", err)
	}

	// Fee config PDA
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

// GetCreatorVaultPDA derives the creator vault PDA
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

// CalculateBuyAmount 计算购买时需要的token数量
func CalculateBuyAmount(solAmountIn uint64, virtualSolReserves, virtualTokenReserves uint64) uint64 {
	// AMM 公式: tokenOut = (tokenReserve * solIn) / (solReserve + solIn)
	// 减去费用: solIn = solIn * (10000 - fee) / 10000
	solAfterFee := (solAmountIn * (PumpSwapAmmBase - PumpSwapAmmBuyFee)) / PumpSwapAmmBase

	// 使用 big.Int 避免溢出
	numerator := new(big.Int).Mul(
		new(big.Int).SetUint64(virtualTokenReserves),
		new(big.Int).SetUint64(solAfterFee),
	)

	denominator := new(big.Int).Add(
		new(big.Int).SetUint64(virtualSolReserves),
		new(big.Int).SetUint64(solAfterFee),
	)

	tokenOut := new(big.Int).Div(numerator, denominator)

	return tokenOut.Uint64()
}

// CalculateSellAmount 计算卖出时能获得的SOL数量
func CalculateSellAmount(tokenAmountIn uint64, virtualSolReserves, virtualTokenReserves uint64) uint64 {
	// AMM 公式: solOut = (solReserve * tokenIn) / (tokenReserve + tokenIn)
	// 减去费用
	solOut := (virtualSolReserves * tokenAmountIn) / (virtualTokenReserves + tokenAmountIn)
	solAfterFee := (solOut * (PumpSwapAmmBase - PumpSwapAmmBuyFee)) / PumpSwapAmmBase

	return solAfterFee
}

// BuildBuyInstruction 构建原始 PumpFun (bonding curve) 的 buy 指令
func BuildBuyInstruction(
	user ag_solanago.PublicKey,
	mint ag_solanago.PublicKey,
	solAmountIn uint64,
	slippageBps uint32,
	client *ag_rpc.Client,
	price float64,
	inDecimals, outDecimals uint8,
) (ag_solanago.Instruction, error) {
	ctx := context.Background()

	// 获取 PumpFun PDAs
	global, feeRecipient, eventAuthority, feeConfig, err := GetPumpFunPDAs(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get pump fun PDAs: %w", err)
	}

	state, err := GetBondingCurveState(ctx, client, mint)
	if err != nil {
		return nil, err
	}
	bondingCurve := state.BondingCurve

	// 获取 bonding curve 的 associated token account
	associatedBondingCurve, _, err := ag_solanago.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to find associated bonding curve: %w", err)
	}

	// 获取用户的 associated token account
	associatedUser, _, err := ag_solanago.FindAssociatedTokenAddress(user, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to find associated user account: %w", err)
	}

	// 计算预期获得的token数量
	tokenAmount := CalculateBuyAmount(solAmountIn, state.VirtualSolReserves, state.VirtualTokenReserves)

	// 应用滑点保护：计算最少要获得的 token 数量
	minTokenAmount := tokenAmount * (10000 - uint64(slippageBps)) / 10000

	fmt.Println("BuildBuyInstruction: tokenAmount is:", tokenAmount, "minTokenAmount is:", minTokenAmount, "solAmountIn is:", solAmountIn, "virtualSolReserves is:", state.VirtualSolReserves, "virtualTokenReserves is:", state.VirtualTokenReserves, "slippageBps is:", slippageBps)

	// 从bonding curve数据中提取creator
	creator := state.Creator

	// 获取 creator vault PDA
	creatorVault, _, err := GetCreatorVaultPDA(creator)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator vault PDA: %w", err)
	}

	// 计算 User Volume Accumulator PDA
	userVolumeAccumulatorSeeds := [][]byte{[]byte("user_volume_accumulator"), user.Bytes()}
	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)
	userVolumeAccumulator, _, err := ag_solanago.FindProgramAddress(userVolumeAccumulatorSeeds, programID)
	if err != nil {
		return nil, fmt.Errorf("failed to find user volume accumulator: %w", err)
	}

	// 计算 Global Volume Accumulator PDA
	globalVolumeAccumulatorSeeds := [][]byte{[]byte("global_volume_accumulator")}
	globalVolumeAccumulator, _, err := ag_solanago.FindProgramAddress(globalVolumeAccumulatorSeeds, programID)
	if err != nil {
		return nil, fmt.Errorf("failed to find global volume accumulator: %w", err)
	}

	// 构建 buy_exact_sol_in 指令
	log.Printf("构建 buy_exact_sol_in 指令，花费精确SQL：{}，最少获得TOKEN：{}", solAmountIn, minTokenAmount)
	instruction, err := pump.NewBuyExactSolInInstruction(
		solAmountIn,                 // spendableSolIn - 精确花费的 SOL 数量
		minTokenAmount,              // minTokensOut - 最少获得的 token 数量（滑点保护）
		pump.OptionBool{V0: true},   // trackVolume
		global,                      // global
		feeRecipient,                // feeRecipient
		mint,                        // mint
		bondingCurve,                // bondingCurve
		associatedBondingCurve,      // associatedBondingCurve
		associatedUser,              // associatedUser
		user,                        // user
		ag_solanago.SystemProgramID, // systemProgram
		ag_solanago.TokenProgramID,  // tokenProgram
		creatorVault,                // creatorVault
		eventAuthority,              // eventAuthority
		programID,                   // program
		globalVolumeAccumulator,     // globalVolumeAccumulator
		userVolumeAccumulator,       // userVolumeAccumulator
		feeConfig,                   // feeConfig
		ag_solanago.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ"), // feeProgram
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create buy instruction: %w", err)
	}

	return instruction, nil
}

// BuildSellInstruction 构建原始 PumpFun (bonding curve) 的 sell 指令
func BuildSellInstruction(
	userTokenAccount ag_solanago.PublicKey,
	user ag_solanago.PublicKey,
	mint ag_solanago.PublicKey,
	tokenAmountIn uint64,
	slippageBps uint32,
	exactOut bool,
	client *ag_rpc.Client,
	price float64,
	inDecimals, outDecimals uint8,
) (ag_solanago.Instruction, uint64, error) {
	ctx := context.Background()

	// 获取 PumpFun PDAs
	global, feeRecipient, eventAuthority, feeConfig, err := GetPumpFunPDAs(mint)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pump fun PDAs: %w", err)
	}

	state, err := GetBondingCurveState(ctx, client, mint)
	if err != nil {
		return nil, 0, err
	}
	bondingCurve := state.BondingCurve

	// 获取 bonding curve 的 associated token account
	associatedBondingCurve, _, err := ag_solanago.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find associated bonding curve: %w", err)
	}

	// 计算能获得的SOL数量
	solAmount := CalculateSellAmount(tokenAmountIn, state.VirtualSolReserves, state.VirtualTokenReserves)

	// 应用滑点
	minSolAmount := (solAmount * uint64(10000-slippageBps)) / 10000

	creator := state.Creator

	// 获取 creator vault PDA
	creatorVault, _, err := GetCreatorVaultPDA(creator)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get creator vault PDA: %w", err)
	}

	programID := ag_solanago.MustPublicKeyFromBase58(PumpSwapProgramID)

	// 构建 sell 指令
	log.Printf("构建 sell 指令，卖出TOKEN数量：{}，预期获得SOL数量：{}，最少获得SOL数量：{}", tokenAmountIn, solAmount, minSolAmount)
	instruction, err := pump.NewSellInstruction(
		tokenAmountIn,               // amount
		minSolAmount,                // minSolOutput
		global,                      // global
		feeRecipient,                // feeRecipient
		mint,                        // mint
		bondingCurve,                // bondingCurve
		associatedBondingCurve,      // associatedBondingCurve
		userTokenAccount,            // associatedUser
		user,                        // user
		ag_solanago.SystemProgramID, // systemProgram
		creatorVault,                // creatorVault
		ag_solanago.TokenProgramID,  // tokenProgram
		eventAuthority,              // eventAuthority
		programID,                   // program
		feeConfig,                   // feeConfig
		ag_solanago.MustPublicKeyFromBase58("pfeeUxB6jkeY1Hxd7CsFCAjcbHA9rWtchMGdZ6VojVZ"), // feeProgram
	)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to create sell instruction: %w", err)
	}

	return instruction, solAmount, nil
}

// reverseBytes reverses a byte slice (for little-endian conversion)
func reverseBytes(b []byte) []byte {
	reversed := make([]byte, len(b))
	for i := range b {
		reversed[len(b)-1-i] = b[i]
	}
	return reversed
}
