package block

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
)

// fetchClmmCurrentPrice 从链上获取 CLMM 池子的当前价格和 tick
// 返回: (价格 Token1/Token0, tick 索引, 错误)
func (s *BlockService) fetchClmmCurrentPrice(ctx context.Context, poolStateStr, inputVaultMint, outputVaultMint string) (float64, int32, error) {
	cli := s.sc.GetSolClient()
	if cli == nil {
		return 0, 0, errors.New("solana rpc client not configured")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从链上获取 PoolState 账户数据
	accountInfo, err := cli.GetAccountInfo(ctxWithTimeout, poolStateStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get pool state account: %w", err)
	}
	if len(accountInfo.Data) == 0 {
		return 0, 0, errors.New("pool state account not found or empty")
	}

	// 解析 PoolState 账户数据
	data := accountInfo.Data
	dec := bin.NewBorshDecoder(data)
	var poolStateAccount amm_v3.PoolStateAccount
	if err := poolStateAccount.UnmarshalWithDecoder(dec); err != nil {
		return 0, 0, fmt.Errorf("failed to decode pool state: %w", err)
	}

	// 获取 sqrt_price_x64 和当前 tick
	sqrtPriceX64 := poolStateAccount.SqrtPriceX64
	currentTick := poolStateAccount.TickCurrent
	poolTokenMint0 := poolStateAccount.TokenMint0.String()
	poolTokenMint1 := poolStateAccount.TokenMint1.String()
	decimals0 := int64(poolStateAccount.MintDecimals0)
	decimals1 := int64(poolStateAccount.MintDecimals1)

	// 使用公共函数计算价格（Token1/Token0）
	price, err := clmm.CalculatePriceFromSqrtPriceX64(
		clmm.Uint128{
			Lo: sqrtPriceX64.Lo,
			Hi: sqrtPriceX64.Hi,
		},
		decimals0,
		decimals1,
		poolTokenMint0,
		poolTokenMint1,
		inputVaultMint,
		outputVaultMint,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate price: %w", err)
	}

	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, 0, fmt.Errorf("invalid calculated price: %f", price)
	}

	logx.Infof("fetchClmmCurrentPrice: poolState=%s, price=%f, tick=%d, inputMint=%s, outputMint=%s",
		poolStateStr, price, currentTick, inputVaultMint, outputVaultMint)

	return price, currentTick, nil
}
