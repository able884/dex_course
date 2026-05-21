package clmm

import (
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
)

type SwapV2Para struct {
	// Parameters:
	AmountIn             uint64
	OtherAmountThreshold uint64
	SqrtPriceLimitX64    ag_binary.Uint128
	IsBaseInput          bool
	// Accounts:
	Payer              ag_solanago.PublicKey
	AmmConfig          ag_solanago.PublicKey
	PoolState          ag_solanago.PublicKey
	InputTokenAccount  ag_solanago.PublicKey
	OutputTokenAccount ag_solanago.PublicKey
	InputVault         ag_solanago.PublicKey
	OutputVault        ag_solanago.PublicKey
	ObservationState   ag_solanago.PublicKey
	TokenProgram       ag_solanago.PublicKey
	TokenProgram2022   ag_solanago.PublicKey
	MemoProgram        ag_solanago.PublicKey
	InputVaultMint     ag_solanago.PublicKey
	OutputVaultMint    ag_solanago.PublicKey
	RemainAccounts     []ag_solanago.PublicKey
}

func NewSwapV2Instruction(para *SwapV2Para) (ag_solanago.Instruction, error) {
	ins := amm_v3.NewSwapV2Instruction(
		para.AmountIn,
		para.OtherAmountThreshold,
		para.SqrtPriceLimitX64,
		para.IsBaseInput,

		para.Payer,
		para.AmmConfig,
		para.PoolState,
		para.InputTokenAccount,
		para.OutputTokenAccount,
		para.InputVault,
		para.OutputVault,
		para.ObservationState,
		para.TokenProgram,
		para.TokenProgram2022,
		para.MemoProgram,
		para.InputVaultMint,
		para.OutputVaultMint)
	for _, v := range para.RemainAccounts {
		ins.Append(ag_solanago.Meta(v).WRITE())
	}
	return ins.ValidateAndBuild()
}
