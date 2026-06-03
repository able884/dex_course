package solana

import (
	"context"

	aSDK "github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/xcode"
)

// 计算资源费 = 总钱(gasFee) - 固定签名费(GasPerSignature)
// ctx                上下文（日志、超时控制）
// isAntiMev          是否启用反MEV功能
// initiator          交易发起者的公钥
// cuLimit            计算单元限制
// gasFeeInLamport    预期的总gas费用（以lamports为单位）
func (tm *TxManager) CreateGasAndJitoByGasFee(ctx context.Context, isAntiMev bool, initiator aSDK.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]aSDK.Instruction, uint64, error) {
	var instructionNew aSDK.Instruction
	var instructions []aSDK.Instruction

	// 获取 Jito 防抢跑小费，校验合法性，转换成链上单位
	tipFee := tm.ListJitoFloorFee()

	if tipFee <= 0 || tipFee >= sol.JitoMaxFee {
		// 如果小费不合法，就不开启防 MEV 功能，避免交易失败。
		return nil, 0, xcode.AntiErr
	}
	// 将小费转换成 lamports，作为链上交易的一部分。注意，这个小费是给 Jito 骑士的，不是给矿工的。
	jitoFeeInLamport := ConverFloat642Uint64(tipFee, sol.SolDecimal)
	// jitoFeeInLamport := uint64(0)

	// 计算gas价格 = 用户设置的总 Gas 费 - 固定签名成本，除以计算单元限制，得到每个计算单元的价格（以 lamports 为单位）。
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
	if isAntiMev {
		// 如果开启防 MEV，就给 Jito 小费地址转 SOL。
		instructionNew, err = system.NewTransferInstruction(jitoFeeInLamport, initiator, TipAddress).ValidateAndBuild()
		if nil != err {
			return nil, 0, err
		}
		instructions = append(instructions, instructionNew)
	}
	logx.WithContext(ctx).Debugf("CreateGasAndJitoByGasPrice, initiator=%s, jitoFeeInLamport=%d, gasPrice=%d, cuLimit=%d, isAntiMev=%v",
		initiator, jitoFeeInLamport, gasPriceMicroLamports, cuLimit, isAntiMev)

	feeInLamport := jitoFeeInLamport + gasFeeInLamport

	return instructions, feeInLamport, nil
}

// ConverFloat642Uint64 将浮点数转换为 uint64
// value: 浮点数金额（如 jito 地板价小费）
// decimal: 精度（Solana 固定用 1e9 = sol.SolDecimal）
func ConverFloat642Uint64(value float64, decimal uint64) uint64 {
	// 防止负数
	if value < 0 {
		return 0
	}

	// 核心：浮点数 × 精度 → 四舍五入 → 转无符号整数
	amount := value * float64(decimal)
	return uint64(amount + 0.5) // +0.5 实现四舍五入
}
