package trade

import (
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"
	"richcode.cc/dex/pkg/sol"
)

var (
	AllBpDecimal            = decimal.NewFromInt(10000)
	FeeRateDenominatorValue = decimal.NewFromInt(1000000)
	RaydiumV4Fee            = uint64(2500)
	PumpFee                 = uint64(10000)
	PumpSwapFee             = uint64(2500)
)

func CalcMinAmountOutByPrice(slipPageBP uint32, amountIn uint64, isBuy bool, price float64, inDecimal, outDecimal uint8, feeRate uint64) (uint64, uint64, error) {
	// 根据精度将数据转回正常的
	amountDecimal := decimal.NewFromInt(int64(amountIn)).Div(decimal.NewFromInt(sol.Decimals2Value[inDecimal]))
	feeRateDecimal := decimal.NewFromInt(int64(feeRate)).Div(FeeRateDenominatorValue)
	// 根据交易费率直接将amountIn扣除
	feeDecimal := amountDecimal.Mul(feeRateDecimal)
	amountDecimal = amountDecimal.Sub(feeDecimal)

	priceDecimal := decimal.NewFromFloat(price)
	var tokenOut decimal.Decimal
	// 根据价格来计算
	if isBuy {
		tokenOut = amountDecimal.Div(priceDecimal)
	} else {
		tokenOut = amountDecimal.Mul(priceDecimal)
	}
	// 再根据精度将数据乘回去
	tokenOut = tokenOut.Mul(decimal.NewFromInt(sol.Decimals2Value[outDecimal]))

	//根据滑点计算最小输出
	fmt.Println("tokenOut is:", tokenOut)

	//TODO: 零时处理下 /1000. 现在的计算逻辑更新了。
	minOut := tokenOut.Mul(AllBpDecimal.Sub(decimal.NewFromInt(int64(slipPageBP))).Div(AllBpDecimal).Div(decimal.NewFromInt(1000)))
	fmt.Println("slippageBp is:", slipPageBP)
	fmt.Println("minOut is:", minOut)
	if minOut.IsNegative() || tokenOut.IsNegative() {
		return 0, 0, nil
	}

	return uint64(minOut.IntPart()), uint64(tokenOut.IntPart()), nil
}

func CalcMinAmountOutByAmm(slipPageBP uint32, amountIn uint64, isBuy bool, tokenAmount, baseAmount uint64, feeRate uint64) (uint64, uint64, error) {
	if isBuy {
		return CalcMinAmountOutBySwap(slipPageBP, amountIn, baseAmount, tokenAmount, feeRate)
	}
	return CalcMinAmountOutBySwap(slipPageBP, amountIn, tokenAmount, baseAmount, feeRate)
}

func CalcMinAmountOutBySwap(slipPageBP uint32, amountIn uint64, totalInTokenAmount, totalOutTokenAmount uint64, feeRate uint64) (uint64, uint64, error) {
	amountDecimal := decimal.NewFromInt(int64(amountIn))
	feeRateDecimal := decimal.NewFromInt(int64(feeRate)).Div(FeeRateDenominatorValue)
	// 根据交易费率直接将amountIn扣除
	// raydiumv4 看其源码逻辑是放在前 https://github.com/raydium-io/raydium-amm/blob/master/program/src/processor.rs#L2399
	// pumpfun的代码没有官方宣布的开源，但是有自称fork版本的，里面的计算逻辑不是这个amm的方式，而是另一种奇怪的方式 https://github.com/Rust-Sol-Dev/Pump.fun-Smart-Contract/blob/main/programs/bonding_curve/src/state.rs#L101
	feeDecimal := amountDecimal.Mul(feeRateDecimal)
	amountDecimal = amountDecimal.Sub(feeDecimal)

	totalInTokenAmountDecimal := decimal.NewFromInt(int64(totalInTokenAmount))
	totalOutTokenAmountDecimal := decimal.NewFromInt(int64(totalOutTokenAmount))

	productDecimal := totalInTokenAmountDecimal.Mul(totalOutTokenAmountDecimal)

	outDecimal := totalOutTokenAmountDecimal.Sub(productDecimal.Div(totalInTokenAmountDecimal.Add(amountDecimal)))

	//根据滑点计算最小输出
	minOut := outDecimal.Mul(AllBpDecimal.Sub(decimal.NewFromInt(int64(slipPageBP))).Div(AllBpDecimal))
	if !minOut.IsPositive() || !outDecimal.IsPositive() {
		return 0, 0, nil
	}
	return uint64(minOut.IntPart()), uint64(outDecimal.IntPart()), nil
}

// Calculate PumpFunSwap BaseAmountOut
func CalcPSBaseAmountOut(
	amounts []uint64, // poolQuoteTokenAmount, poolBaseTokenAmount
	solAmountIn *big.Int, // solAmount
	feeRate uint64,
	baseRate uint64,
) (uint64, error) {
	effectiveSolUsed := new(big.Int).Sub(
		solAmountIn,
		new(big.Int).Div(
			new(big.Int).Mul(solAmountIn, big.NewInt(0).SetUint64(feeRate)),
			big.NewInt(0).SetUint64(baseRate),
		),
	)
	// calculate constant product
	constantProduct := new(big.Int).Mul(
		new(big.Int).SetUint64(amounts[1]),
		new(big.Int).SetUint64(amounts[0]),
	)
	// update base amount
	denominator := new(big.Int).Add(
		new(big.Int).SetUint64(amounts[0]),
		effectiveSolUsed,
	)
	updatedBaseAmount := new(big.Int).Div(constantProduct, denominator)
	tokenReceived := new(big.Int).Sub(
		new(big.Int).SetUint64(amounts[1]),
		updatedBaseAmount,
	)
	if tokenReceived.Sign() <= 0 {
		return 0, fmt.Errorf("calculated tokens received is not positive: %s", tokenReceived.String())
	}
	return tokenReceived.Uint64(), nil
}

// Calculate PumpFunSwap MinQuoteAmountOut
func CalcPSMinQuoteAmountOut(
	amounts []uint64, // poolQuoteTokenAmount, poolBaseTokenAmount
	baseAmountIn *big.Int, // tokenAmount
	slippage uint32,
	feeRate uint64,
	baseRate uint64,
) (uint64, error) {
	effectiveTokenSold := new(big.Int).Sub(
		baseAmountIn,
		new(big.Int).Div(
			new(big.Int).Mul(baseAmountIn, big.NewInt(0).SetUint64(feeRate)),
			big.NewInt(0).SetUint64(baseRate),
		),
	)
	// calculate constant product
	constantProduct := new(big.Int).Mul(
		new(big.Int).SetUint64(amounts[1]),
		new(big.Int).SetUint64(amounts[0]),
	)
	denominator := new(big.Int).Add(
		new(big.Int).SetUint64(amounts[1]),
		effectiveTokenSold,
	)
	updatedQuoteAmount := new(big.Int).Div(constantProduct, denominator)
	solReceived := new(big.Int).Sub(
		new(big.Int).SetUint64(amounts[0]),
		updatedQuoteAmount,
	)
	slippageAdjustment := big.NewFloat(0).Sub(
		big.NewFloat(1),
		big.NewFloat(0).Quo(
			big.NewFloat(0).SetInt64(int64(slippage)),
			big.NewFloat(0).SetInt64(100),
		),
	)
	minQuoteAmount := big.NewFloat(0).Mul(
		big.NewFloat(0).SetInt(solReceived),
		slippageAdjustment,
	)
	minQuoteAmountInt, _ := minQuoteAmount.Int(nil)
	if minQuoteAmountInt.Sign() <= 0 {
		return 0, fmt.Errorf("calculated min quote amount is not positive: %s", minQuoteAmountInt.String())
	}
	return minQuoteAmountInt.Uint64(), nil
}
