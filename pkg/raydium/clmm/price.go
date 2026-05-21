package clmm

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

// Uint128 represents a 128-bit unsigned integer
// This is a simplified version compatible with amm_v3.Uint128
type Uint128 struct {
	Lo uint64
	Hi uint64
}

// CalculatePriceFromSqrtPriceX64 从 sqrt_price_x64 计算 CLMM 池子的价格
// 价格计算公式：price = (sqrt_price_x64 / 2^64)^2 * (10^decimals0) / (10^decimals1)
//
// 参数：
//   - sqrtPriceX64: CLMM 池子的 sqrt_price_x64 值（Uint128 格式，包含 Hi 和 Lo 字段）
//   - decimals0: token0 的小数位数
//   - decimals1: token1 的小数位数
//   - poolTokenMint0: 池子中 token0 的 mint 地址（字符串）
//   - poolTokenMint1: 池子中 token1 的 mint 地址（字符串）
//   - inputMint: 输入代币的 mint 地址（字符串）
//   - outputMint: 输出代币的 mint 地址（字符串）
//
// 返回：
//   - float64: outputMint/inputMint 的价格
//   - error: 如果计算失败则返回错误
func CalculatePriceFromSqrtPriceX64(
	sqrtPriceX64 Uint128,
	decimals0 int64,
	decimals1 int64,
	poolTokenMint0 string,
	poolTokenMint1 string,
	inputMint string,
	outputMint string,
) (float64, error) {
	// 将 Uint128 转换为 big.Int 以保持精度
	// Uint128 结构：Hi (uint64) 和 Lo (uint64)
	sqrtPriceBig := new(big.Int)
	sqrtPriceBig.Lsh(big.NewInt(int64(sqrtPriceX64.Hi)), 64)
	sqrtPriceBig.Add(sqrtPriceBig, big.NewInt(int64(sqrtPriceX64.Lo)))

	// Q64.64 格式：sqrt_price_x64 = sqrt(token_mint_1/token_mint_0) * 2^64
	// 其中 token_mint_0 < token_mint_1 (按地址排序)
	// 所以 sqrt(token_mint_1/token_mint_0) = sqrt_price_x64 / 2^64
	// 使用 big.Float 进行计算以保持精度
	// 计算 2^64，需要使用 big.Int 因为 int64 无法表示
	q64BigInt := new(big.Int).Exp(big.NewInt(2), big.NewInt(64), nil)
	q64 := new(big.Float).SetInt(q64BigInt) // 2^64

	sqrtPriceFloat := new(big.Float).SetInt(sqrtPriceBig)
	sqrtRatio := new(big.Float).Quo(sqrtPriceFloat, q64)

	// price = (sqrt(token_mint_1/token_mint_0))^2 = token_mint_1/token_mint_0
	priceRatio := new(big.Float).Mul(sqrtRatio, sqrtRatio)

	// 调整小数位数：price = priceRatio * (10^decimals0) / (10^decimals1)
	decimals0Multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals0), nil))
	decimals1Multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals1), nil))

	priceBig := new(big.Float).Mul(priceRatio, decimals0Multiplier)
	priceBig.Quo(priceBig, decimals1Multiplier)

	// 转换为 float64
	priceToken1PerToken0, _ := priceBig.Float64()
	if priceToken1PerToken0 <= 0 || math.IsNaN(priceToken1PerToken0) || math.IsInf(priceToken1PerToken0, 0) {
		return 0, fmt.Errorf("invalid calculated price: %f", priceToken1PerToken0)
	}

	// 确定价格方向：需要返回 outputMint/inputMint 的价格
	// 如果 inputMint == token_mint_0 且 outputMint == token_mint_1，则 price = token_mint_1/token_mint_0
	// 如果 inputMint == token_mint_1 且 outputMint == token_mint_0，则 price = 1 / (token_mint_1/token_mint_0)
	if inputMint == poolTokenMint0 && outputMint == poolTokenMint1 {
		// 方向一致，直接返回
		return priceToken1PerToken0, nil
	} else if inputMint == poolTokenMint1 && outputMint == poolTokenMint0 {
		// 方向相反，需要取倒数
		if priceToken1PerToken0 == 0 {
			return 0, errors.New("cannot invert zero price")
		}
		return 1.0 / priceToken1PerToken0, nil
	} else {
		// mint 地址不匹配，这可能不应该发生，但为了安全返回计算出的价格
		// 假设 inputMint 对应 token_mint_0，outputMint 对应 token_mint_1
		return priceToken1PerToken0, nil
	}
}
