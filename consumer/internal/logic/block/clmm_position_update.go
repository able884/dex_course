package block

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/shopspring/decimal"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
)

// UpdateClmmPositionValueAndFees 从链上获取持仓价值和未提取手续费并更新数据库
// 在以下情况下调用：
// 1. open_position - 创建持仓时
// 2. increase_liquidity - 增加流动性时
// 3. decrease_liquidity - 减少流动性时
// 4. swap - 交易时（手续费会增加）
func (s *BlockService) UpdateClmmPositionValueAndFees(ctx context.Context, positionNftMint, poolState string) error {
	if positionNftMint == "" {
		return errors.New("position_nft_mint is required")
	}

	// 查找持仓记录
	position, err := s.sc.ClmmPositionModel.FindOneByPositionNftMint(ctx, positionNftMint)
	if err != nil {
		// 持仓不存在，可能还未创建，跳过
		return nil
	}

	// 从链上获取 PersonalPositionState
	personalPositionState, err := s.fetchPersonalPositionState(ctx, positionNftMint)
	if err != nil {
		s.Errorf("UpdateClmmPositionValueAndFees: failed to fetch personal position state: %v, positionNftMint: %s", err, positionNftMint)
		return err
	}

	// 获取未提取手续费
	unclaimedFees0 := int64(personalPositionState.TokenFeesOwed0)
	unclaimedFees1 := int64(personalPositionState.TokenFeesOwed1)

	// 获取池子信息以获取代币信息和当前价格
	poolInfo, err := s.getPoolInfo(ctx, poolState)
	if err != nil {
		s.Errorf("UpdateClmmPositionValueAndFees: failed to get pool info: %v, poolState: %s", err, poolState)
		return err
	}

	// 如果数据库中的价格为 0，从链上获取当前价格
	if poolInfo.CurrentPrice <= 0 {
		price, _, err := s.fetchClmmCurrentPrice(ctx, poolState, poolInfo.Token0Mint, poolInfo.Token1Mint)
		if err == nil && price > 0 {
			poolInfo.CurrentPrice = price
		} else if err != nil {
			s.Errorf("UpdateClmmPositionValueAndFees: Failed to fetch current price from chain. poolState: %s, err: %v", poolState, err)
		}
	}

	// 获取代币信息（用于计算 USD 价值）
	token0, token1, err := s.getTokenInfo(ctx, poolInfo.Token0Mint, poolInfo.Token1Mint, poolState)
	if err != nil {
		s.Errorf("UpdateClmmPositionValueAndFees: failed to get token info: %v", err)
		return err
	}

	// 计算未提取手续费 USD 价值
	unclaimedFeesUsd := s.calculateFeesUsdValue(
		unclaimedFees0, unclaimedFees1,
		token0.Decimals, token1.Decimals,
		token0.PriceUsd, token1.PriceUsd,
	)

	// 计算CLMM 流动性仓位，现在值多少钱。
	positionValueUsd, err := s.calculatePositionValue(
		ctx,
		position,
		personalPositionState,
		poolInfo,
		token0,
		token1,
	)
	if err != nil {
		s.Errorf("UpdateClmmPositionValueAndFees: failed to calculate position value: %v", err)
		// 不返回错误，继续更新手续费
	}

	// 计算持仓份额（token0 和 token1 的数量）
	var token0Amount, token1Amount float64
	if position.Liquidity != "" && position.Liquidity != "0" && poolInfo.CurrentPrice > 0 {
		// 计算价格范围（考虑 decimals）
		priceMin, priceMax := s.calculatePriceRangeFromTicks(
			position.TickLowerIndex,
			position.TickUpperIndex,
			token0.Decimals,
			token1.Decimals,
		)

		// 计算代币数量
		token0Amount, token1Amount = s.calculateClmmPositionAmounts(
			position.Liquidity,
			poolInfo.CurrentPrice,
			priceMin,
			priceMax,
			token0.Decimals,
			token1.Decimals,
		)
	}

	// 更新数据库
	position.UnclaimedFees0 = unclaimedFees0
	position.UnclaimedFees1 = unclaimedFees1
	position.UnclaimedFeesUsd = unclaimedFeesUsd
	if positionValueUsd > 0 {
		position.PositionValueUsd = positionValueUsd
	}
	position.Token0Amount = token0Amount
	position.Token1Amount = token1Amount
	position.UpdatedAt = time.Now()

	err = s.sc.ClmmPositionModel.Update(ctx, position)
	if err != nil {
		s.Errorf("UpdateClmmPositionValueAndFees: failed to update position: %v, positionNftMint: %s", err, positionNftMint)
		return fmt.Errorf("failed to update position: %w", err)
	}

	return nil
}

// fetchPersonalPositionState 从链上获取 PersonalPositionState
func (s *BlockService) fetchPersonalPositionState(ctx context.Context, positionNftMint string) (*amm_v3.PersonalPositionStateAccount, error) {
	cli := s.sc.GetSolClient()
	if cli == nil {
		return nil, errors.New("solana rpc client not configured")
	}

	// 这里需要根据实际的 CLMM 程序 ID 来推导 PDA
	// 暂时使用已知的 PersonalPosition 地址（如果交易中已经包含）
	// 或者从持仓记录中获取
	position, err := s.sc.ClmmPositionModel.FindOneByPositionNftMint(ctx, positionNftMint)
	if err != nil {
		return nil, fmt.Errorf("position not found: %w", err)
	}

	personalPositionAddr := position.PersonalPosition
	if personalPositionAddr == "" {
		return nil, errors.New("personal_position address not found in database")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从链上获取账户数据
	accountInfo, err := cli.GetAccountInfo(ctxWithTimeout, personalPositionAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get personal position account: %w", err)
	}
	if len(accountInfo.Data) == 0 {
		return nil, errors.New("personal position account not found or empty")
	}

	// 解析 PersonalPositionState
	data := accountInfo.Data
	dec := bin.NewBorshDecoder(data)
	var personalPositionState amm_v3.PersonalPositionStateAccount
	if err := personalPositionState.UnmarshalWithDecoder(dec); err != nil {
		return nil, fmt.Errorf("failed to decode personal position state: %w", err)
	}

	return &personalPositionState, nil
}

// poolInfo 池子信息结构
type poolInfo struct {
	Token0Mint   string
	Token1Mint   string
	CurrentPrice float64
}

// getPoolInfo 获取池子信息
func (s *BlockService) getPoolInfo(ctx context.Context, poolState string) (*poolInfo, error) {
	// 先尝试从 V1 池子获取
	v1Pool, err := s.sc.SolRaydiumCLMMPoolV1Model.FindOneByPoolState(ctx, poolState)
	if err == nil && v1Pool != nil {
		return &poolInfo{
			Token0Mint:   v1Pool.InputVaultMint,
			Token1Mint:   v1Pool.OutputVaultMint,
			CurrentPrice: v1Pool.CurrentPrice,
		}, nil
	}

	// 尝试从 V2 池子获取
	v2Pool, err := s.sc.SolRaydiumCLMMPoolV2Model.FindOneByPoolState(ctx, poolState)
	if err == nil && v2Pool != nil {
		return &poolInfo{
			Token0Mint:   v2Pool.InputVaultMint,
			Token1Mint:   v2Pool.OutputVaultMint,
			CurrentPrice: v2Pool.CurrentPrice,
		}, nil
	}

	return nil, fmt.Errorf("pool not found: %s", poolState)
}

// tokenInfo 代币信息结构
type tokenInfo struct {
	Decimals int64
	PriceUsd float64
}

// getTokenInfo 获取代币信息
func (s *BlockService) getTokenInfo(ctx context.Context, token0Mint, token1Mint, poolState string) (*tokenInfo, *tokenInfo, error) {
	// 从数据库获取代币信息
	token0, err := s.sc.TokenModel.FindOneByChainIdAddress(ctx, SolChainIdInt, token0Mint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get token0: %w", err)
	}

	token1, err := s.sc.TokenModel.FindOneByChainIdAddress(ctx, SolChainIdInt, token1Mint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get token1: %w", err)
	}

	// 从 Pair 表获取代币价格
	price0 := 0.0
	price1 := 0.0
	pair, err := s.sc.PairModel.FindOneByChainIdAddress(ctx, SolChainIdInt, poolState)
	if err == nil && pair != nil {
		// 判断 token0 和 token1 哪个是 base token，哪个是 quote token
		if pair.BaseTokenAddress == token0Mint {
			price0 = pair.BaseTokenPrice
			price1 = pair.TokenPrice
		} else if pair.BaseTokenAddress == token1Mint {
			price0 = pair.TokenPrice
			price1 = pair.BaseTokenPrice
		}
	}

	return &tokenInfo{
			Decimals: int64(token0.Decimals),
			PriceUsd: price0,
		}, &tokenInfo{
			Decimals: int64(token1.Decimals),
			PriceUsd: price1,
		}, nil
}

// calculateFeesUsdValue 计算未提取手续费的 USD 价值
func (s *BlockService) calculateFeesUsdValue(fees0, fees1 int64, decimals0, decimals1 int64, price0, price1 float64) float64 {
	// 转换为实际代币数量
	fees0Amount := decimal.New(fees0, -int32(decimals0))
	fees1Amount := decimal.New(fees1, -int32(decimals1))

	// 计算 USD 价值
	fees0Usd := fees0Amount.Mul(decimal.NewFromFloat(price0))
	fees1Usd := fees1Amount.Mul(decimal.NewFromFloat(price1))

	totalUsd := fees0Usd.Add(fees1Usd)
	return totalUsd.InexactFloat64()
}

// calculatePositionValue 计算CLMM 流动性仓位，现在值多少钱。
func (s *BlockService) calculatePositionValue(
	ctx context.Context,
	position *solmodel.ClmmPosition,
	personalPositionState *amm_v3.PersonalPositionStateAccount,
	poolInfo *poolInfo,
	token0, token1 *tokenInfo,
) (float64, error) {
	// 从流动性计算持仓中的代币数量
	// 这需要根据当前价格和 tick 范围来计算
	// 简化实现：使用流动性估算

	// 解析流动性
	liquidity := parseLiquidityString(position.Liquidity)
	if liquidity == nil {
		return 0, errors.New("invalid liquidity string")
	}

	// 获取当前价格
	currentPrice := poolInfo.CurrentPrice
	if currentPrice <= 0 {
		// 如果数据库中没有价格，尝试从链上获取
		price, _, err := s.fetchClmmCurrentPrice(ctx, position.PoolState, poolInfo.Token0Mint, poolInfo.Token1Mint)
		if err != nil {
			return 0, fmt.Errorf("failed to get current price: %w", err)
		}
		currentPrice = price
	}

	// 计算价格范围
	priceMin := tickToPrice(position.TickLowerIndex)
	priceMax := tickToPrice(position.TickUpperIndex)

	// 判断是否在范围内
	isInRange := currentPrice >= priceMin && currentPrice <= priceMax

	// 根据价格范围和流动性计算持仓价值
	// 这是一个简化的计算，实际应该使用 CLMM 的公式
	var valueUsd float64
	if isInRange {
		// 在范围内，持仓包含两种代币
		// 使用流动性估算代币数量
		// 这里使用简化的方法，实际应该使用 CLMM 的精确公式
		liquidityFloat := liquidityToFloat(liquidity)
		// 简化估算：假设流动性均匀分布
		valueUsd = liquidityFloat * (token0.PriceUsd + token1.PriceUsd) / 2
	} else {
		// 在范围外，持仓只包含一种代币
		if currentPrice < priceMin {
			// 价格低于下限，持仓全部是 Token0
			// 使用流动性估算
			liquidityFloat := liquidityToFloat(liquidity)
			valueUsd = liquidityFloat * token0.PriceUsd
		} else {
			// 价格高于上限，持仓全部是 Token1
			liquidityFloat := liquidityToFloat(liquidity)
			valueUsd = liquidityFloat * token1.PriceUsd
		}
	}

	return valueUsd, nil
}

// parseLiquidityString 解析流动性字符串 "hi,lo"
func parseLiquidityString(liquidityStr string) *big.Int {
	if liquidityStr == "" || liquidityStr == "0" {
		return big.NewInt(0)
	}

	var hi, lo uint64
	_, err := fmt.Sscanf(liquidityStr, "%d,%d", &hi, &lo)
	if err != nil {
		return nil
	}

	// 将 Uint128 转换为 big.Int
	result := new(big.Int).SetUint64(hi)
	result.Lsh(result, 64)
	result.Add(result, new(big.Int).SetUint64(lo))

	return result
}

// liquidityToFloat 将流动性转换为浮点数（简化方法）
func liquidityToFloat(liquidity *big.Int) float64 {
	// 这是一个简化的转换，实际应该使用 CLMM 的精确公式
	// 将流动性除以一个合理的缩放因子
	scale := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil))
	result := new(big.Float).Quo(new(big.Float).SetInt(liquidity), scale)
	val, _ := result.Float64()
	return val
}

// tickToPrice 从 tick 索引计算价格（不考虑 decimals，用于简化计算）
func tickToPrice(tick int32) float64 {
	const qRatio = 1.0001
	return math.Pow(qRatio, float64(tick))
}

// calculatePriceRangeFromTicks 从 tick 索引计算价格范围（考虑 decimals）
// CLMM 中 tick 到价格的转换公式需要考虑代币的 decimals
// 基础公式: price_base = 1.0001^tick
// 实际价格: price = price_base * (10^decimals0) / (10^decimals1)
func (s *BlockService) calculatePriceRangeFromTicks(
	tickLower, tickUpper int32,
	decimals0, decimals1 int64,
) (priceMin, priceMax float64) {
	const qRatio = 1.0001

	// 计算基础价格（token_mint_1/token_mint_0）
	priceMinBase := math.Pow(qRatio, float64(tickLower))
	priceMaxBase := math.Pow(qRatio, float64(tickUpper))

	// 调整小数位数：price = price_base * (10^decimals0) / (10^decimals1)
	decimalsMultiplier := math.Pow10(int(decimals0)) / math.Pow10(int(decimals1))
	priceMin = priceMinBase * decimalsMultiplier
	priceMax = priceMaxBase * decimalsMultiplier

	// 确保价格顺序正确（min < max）
	if priceMin > priceMax {
		priceMin, priceMax = priceMax, priceMin
	}

	return
}

// calculateClmmPositionAmounts 计算 CLMM 持仓中的代币数量
// 公式参考：https://docs.uniswap.org/concepts/protocol/understanding-liquidity#calculating-token-amounts-for-a-price-range
// 以及 Raydium 源码：programs/amm/src/libraries/liquidity_math.rs
//
// 注意：
// - liquidity 是 u128 类型，不是 Q64.64 格式
// - 价格是已经考虑 decimals 的实际价格（不是 Q64.64 格式）
// - 公式：amount0 = L * (sqrt(P_upper) - sqrt(P_current)) / (sqrt(P_upper) * sqrt(P_current))
// - 公式：amount1 = L * (sqrt(P_current) - sqrt(P_lower))
// - 其中 L 是 liquidity，P 是实际价格（已考虑 decimals）
func (s *BlockService) calculateClmmPositionAmounts(
	liquidityStr string,
	currentPrice, priceMin, priceMax float64,
	decimals0, decimals1 int64,
) (token0Amount, token1Amount float64) {
	// 解析流动性
	liquidity := parseLiquidityString(liquidityStr)
	if liquidity == nil || liquidity.Sign() <= 0 {
		s.Infof("calculateClmmPositionAmounts: Invalid liquidity. liquidityStr: %s", liquidityStr)
		return 0, 0
	}

	// 确保价格范围有效
	if priceMin <= 0 || priceMax <= 0 || priceMin >= priceMax {
		s.Infof("calculateClmmPositionAmounts: Invalid price range. priceMin: %f, priceMax: %f", priceMin, priceMax)
		return 0, 0
	}

	// 计算 sqrt 价格（价格已经考虑了 decimals，所以直接开方即可）
	sqrtCurrent := math.Sqrt(currentPrice)
	sqrtMin := math.Sqrt(priceMin)
	sqrtMax := math.Sqrt(priceMax)

	// 将流动性转换为 big.Float 以保持精度
	liquidityFloat := new(big.Float).SetInt(liquidity)

	// 根据当前价格与价格范围的关系计算代币数量
	if currentPrice <= priceMin {
		// 当前价格低于价格下限，持仓全部是 token0
		// amount0 = L * (sqrt(P_upper) - sqrt(P_lower)) / (sqrt(P_upper) * sqrt(P_lower))
		if sqrtMax > sqrtMin && sqrtMax > 0 && sqrtMin > 0 {
			// 使用 big.Float 进行计算以保持精度
			sqrtMaxFloat := new(big.Float).SetFloat64(sqrtMax)
			sqrtMinFloat := new(big.Float).SetFloat64(sqrtMin)

			// numerator = L * (sqrtMax - sqrtMin)
			numerator := new(big.Float).Sub(sqrtMaxFloat, sqrtMinFloat)
			numerator.Mul(numerator, liquidityFloat)

			// denominator = sqrtMax * sqrtMin
			denominator := new(big.Float).Mul(sqrtMaxFloat, sqrtMinFloat)

			// amount0 = numerator / denominator
			result := new(big.Float).Quo(numerator, denominator)
			token0Amount, _ = result.Float64()
			token1Amount = 0
		}
	} else if currentPrice >= priceMax {
		// 当前价格高于价格上限，持仓全部是 token1
		// amount1 = L * (sqrt(P_upper) - sqrt(P_lower))
		if sqrtMax > sqrtMin {
			sqrtMaxFloat := new(big.Float).SetFloat64(sqrtMax)
			sqrtMinFloat := new(big.Float).SetFloat64(sqrtMin)

			// amount1 = L * (sqrtMax - sqrtMin)
			result := new(big.Float).Sub(sqrtMaxFloat, sqrtMinFloat)
			result.Mul(result, liquidityFloat)
			token1Amount, _ = result.Float64()
			token0Amount = 0
		}
	} else {
		// 当前价格在范围内，持仓包含两种代币
		// amount0 = L * (sqrt(P_upper) - sqrt(P_current)) / (sqrt(P_upper) * sqrt(P_current))
		// amount1 = L * (sqrt(P_current) - sqrt(P_lower))
		if sqrtMax > sqrtCurrent && sqrtCurrent > sqrtMin && sqrtMax > 0 && sqrtCurrent > 0 {
			sqrtMaxFloat := new(big.Float).SetFloat64(sqrtMax)
			sqrtCurrentFloat := new(big.Float).SetFloat64(sqrtCurrent)
			sqrtMinFloat := new(big.Float).SetFloat64(sqrtMin)

			// amount0 = L * (sqrtMax - sqrtCurrent) / (sqrtMax * sqrtCurrent)
			numerator0 := new(big.Float).Sub(sqrtMaxFloat, sqrtCurrentFloat)
			numerator0.Mul(numerator0, liquidityFloat)
			denominator0 := new(big.Float).Mul(sqrtMaxFloat, sqrtCurrentFloat)
			result0 := new(big.Float).Quo(numerator0, denominator0)
			token0Amount, _ = result0.Float64()

			// amount1 = L * (sqrtCurrent - sqrtMin)
			result1 := new(big.Float).Sub(sqrtCurrentFloat, sqrtMinFloat)
			result1.Mul(result1, liquidityFloat)
			token1Amount, _ = result1.Float64()
		}
	}

	// 确保数量为正数
	if token0Amount < 0 {
		token0Amount = 0
	}
	if token1Amount < 0 {
		token1Amount = 0
	}

	// 重要：公式计算出的 amount0 和 amount1 是原子单位（atomic units），
	// 需要除以 10^decimals 才能得到实际的代币数量
	decimals0Multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals0), nil))
	decimals1Multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals1), nil))

	token0AmountFloat := new(big.Float).SetFloat64(token0Amount)
	token1AmountFloat := new(big.Float).SetFloat64(token1Amount)

	token0AmountFloat.Quo(token0AmountFloat, decimals0Multiplier)
	token1AmountFloat.Quo(token1AmountFloat, decimals1Multiplier)

	token0Amount, _ = token0AmountFloat.Float64()
	token1Amount, _ = token1AmountFloat.Float64()

	// 调试日志：检查计算结果是否合理
	if token0Amount > 1e12 || token1Amount > 1e12 {
		s.Infof("calculateClmmPositionAmounts: WARNING - Calculated amounts seem too large. liquidityStr: %s, liquidity: %s, currentPrice: %f, priceMin: %f, priceMax: %f, decimals0: %d, decimals1: %d, token0Amount: %f, token1Amount: %f",
			liquidityStr, liquidity.String(), currentPrice, priceMin, priceMax, decimals0, decimals1, token0Amount, token1Amount)
	}

	return token0Amount, token1Amount
}
