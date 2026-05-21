package cpmm

import (
	ag_solanago "github.com/gagliardetto/solana-go"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
)

type SwapBaseInputPara struct {
	// Parameters:
	AmountIn         uint64
	MinimumAmountOut uint64
	// Accounts:
	Payer              ag_solanago.PublicKey
	Authority          ag_solanago.PublicKey
	AmmConfig          ag_solanago.PublicKey
	PoolState          ag_solanago.PublicKey
	InputTokenAccount  ag_solanago.PublicKey
	OutputTokenAccount ag_solanago.PublicKey
	InputVault         ag_solanago.PublicKey
	OutputVault        ag_solanago.PublicKey
	InputTokenProgram  ag_solanago.PublicKey
	OutputTokenProgram ag_solanago.PublicKey
	InputTokenMint     ag_solanago.PublicKey
	OutputTokenMint    ag_solanago.PublicKey
	ObservationState   ag_solanago.PublicKey
}

func NewSwapBaseInputInstruction(para *SwapBaseInputPara) (ag_solanago.Instruction, error) {
	ins := raydium_cp_swap.NewSwapBaseInputInstruction(
		para.AmountIn,
		para.MinimumAmountOut,

		para.Payer,
		para.Authority,
		para.AmmConfig,
		para.PoolState,
		para.InputTokenAccount,
		para.OutputTokenAccount,
		para.InputVault,
		para.OutputVault,
		para.InputTokenProgram,
		para.OutputTokenProgram,
		para.InputTokenMint,
		para.OutputTokenMint,
		para.ObservationState,
	)

	return ins.ValidateAndBuild()
}
