package solana

import (
	"context"

	aSDK "github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/sol"
)

// CreateGasAndJitoByGasFee create by gas fee
func (tm *TxManager) CreateGasAndJitoByGasFee(ctx context.Context, isAntiMev bool, initiator aSDK.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]aSDK.Instruction, uint64, error) {
	var instructionNew aSDK.Instruction
	var instructions []aSDK.Instruction

	// Comment out jito-related code to avoid panic when jito is not configured
	// tipFee := tm.ListJitoFloorFee()

	// if tipFee <= 0 || tipFee >= sol.JitoMaxFee {
	// 	return nil, 0, xcode.AntiErr
	// }
	// jitoFeeInLamport := ConverFloat642Uint64(tipFee, sol.SolDecimal)
	jitoFeeInLamport := uint64(0)

	gasPriceMicroLamports := (gasFeeInLamport - sol.GasPerSignature) * 1e6 / uint64(cuLimit)
	var err error
	if gasPriceMicroLamports != 0 {
		instructionNew, err = computebudget.NewSetComputeUnitPriceInstruction(gasPriceMicroLamports).ValidateAndBuild()
		if nil != err {
			return nil, 0, err
		}
		instructions = append(instructions, instructionNew)

		// #2 - Compute Budget: SetComputeUnitLimit
		instructionNew, err = computebudget.NewSetComputeUnitLimitInstruction(cuLimit).ValidateAndBuild()
		if nil != err {
			return nil, 0, err
		}
		instructions = append(instructions, instructionNew)
	}

	// Comment out jito transfer instruction
	// if isAntiMev {
	// 	instructionNew, err = system.NewTransferInstruction(jitoFeeInLamport, initiator, TipAddress).ValidateAndBuild()
	// 	if nil != err {
	// 		return nil, 0, err
	// 	}
	// 	instructions = append(instructions, instructionNew)
	// }
	logx.WithContext(ctx).Debugf("CreateGasAndJitoByGasPrice, initiator=%s, jitoFeeInLamport=%d, gasPrice=%d, cuLimit=%d, isAntiMev=%v",
		initiator, jitoFeeInLamport, gasPriceMicroLamports, cuLimit, isAntiMev)

	feeInLamport := jitoFeeInLamport + gasFeeInLamport

	return instructions, feeInLamport, nil
}
