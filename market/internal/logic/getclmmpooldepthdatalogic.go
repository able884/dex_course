package logic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"

	ag_binary "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
)

type GetClmmPoolDepthDataLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetClmmPoolDepthDataLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetClmmPoolDepthDataLogic {
	return &GetClmmPoolDepthDataLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetClmmPoolDepthDataLogic) GetClmmPoolDepthData(in *market.GetClmmPoolDepthDataRequest) (*market.GetClmmPoolDepthDataResponse, error) {
	if in == nil || in.PoolState == "" {
		return nil, fmt.Errorf("pool_state is required")
	}

	// 解析 pool_state 地址
	poolStatePK, err := aSDK.PublicKeyFromBase58(in.PoolState)
	if err != nil {
		return nil, fmt.Errorf("invalid pool_state: %w", err)
	}

	// 从链上获取 PoolState 账户数据（用于获取小数位数）
	if l.svcCtx.SolCli == nil {
		return nil, errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolCli

	poolStateInfo, err := rpcClient.GetAccountInfoWithOpts(l.ctx, poolStatePK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pool state account: %w", err)
	}
	if poolStateInfo == nil || poolStateInfo.Value == nil {
		return nil, errors.New("pool state account not found")
	}

	// 解析 PoolState 账户数据
	data := poolStateInfo.Value.Data.GetBinary()
	decoder := ag_binary.NewBorshDecoder(data)
	var poolState amm_v3.PoolStateAccount
	if err := poolState.UnmarshalWithDecoder(decoder); err != nil {
		return nil, fmt.Errorf("failed to decode pool state: %w", err)
	}

	decimals0 := int64(poolState.MintDecimals0)
	decimals1 := int64(poolState.MintDecimals1)

	// 使用 GetProgramAccounts 查询所有属于该池子的 TickArray
	// 根据 Raydium 代码（client/src/main.rs:2231-2237），过滤条件：
	// 1. Memcmp: 偏移量 8（跳过 discriminator），匹配 pool_id 的 32 字节
	// 2. DataSize: TickArrayState::LEN
	// 根据 programs/amm/src/states/tick_array.rs:30:
	// TickArrayState::LEN = 8 + 32 + 4 + TickState::LEN * 60 + 1 + 115
	// 根据 programs/amm/src/states/tick_array.rs:289:
	// TickState::LEN = 4 + 16 + 16 + 16 + 16 + 16*3 + 16 + 16 + 8 + 8 + 4 = 168
	// 所以 TickArrayState::LEN = 8 + 32 + 4 + 168*60 + 1 + 115 = 10240
	poolStateBytes := poolStatePK.Bytes()

	filters := []ag_rpc.RPCFilter{
		{
			Memcmp: &ag_rpc.RPCFilterMemcmp{
				Offset: 8, // 跳过 8 字节 discriminator
				Bytes:  poolStateBytes,
			},
		},
		{
			DataSize: 10240, // TickArrayState::LEN 的准确大小（根据 Raydium 源代码计算）
		},
	}

	// 查询所有 TickArray 账户
	programAccounts, err := rpcClient.GetProgramAccountsWithOpts(
		l.ctx,
		amm_v3.ProgramID,
		&ag_rpc.GetProgramAccountsOpts{
			Filters: filters,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts: %w", err)
	}

	// 解析所有 TickArray 并提取流动性数据
	var allDataPoints []*market.DepthDataPoint
	l.Infof("found %d tick array accounts for pool %s", len(programAccounts), in.PoolState)

	for _, account := range programAccounts {
		// 验证 pool_id 匹配（根据 Raydium 代码 client/src/main.rs:2251）
		// 虽然 Memcmp 已经过滤了，但为了安全起见再次验证
		accountData := account.Account.Data.GetBinary()

		// 跳过 discriminator (8 字节)，读取 pool_id (32 字节)
		if len(accountData) < 40 {
			l.Infof("account data too short: %d bytes", len(accountData))
			continue
		}

		// 检查 pool_id 是否匹配（跳过 discriminator）
		accountPoolIdBytes := accountData[8:40]
		if !bytes.Equal(accountPoolIdBytes, poolStateBytes) {
			l.Infof("pool_id mismatch at offset 8, skipping account. Expected: %s, Got: %x",
				poolStatePK.String(), accountPoolIdBytes)
			continue
		}

		// 使用 TickArrayStateAccount 解析（它会处理 discriminator）
		decoder := ag_binary.NewBorshDecoder(accountData)
		var tickArrayStateAccount amm_v3.TickArrayStateAccount
		if err := tickArrayStateAccount.UnmarshalWithDecoder(decoder); err != nil {
			l.Infof("failed to decode tick array state account: %v", err)
			continue
		}

		// 验证 pool_id（从解析后的结构体）
		if !tickArrayStateAccount.PoolId.Equals(poolStatePK) {
			l.Infof("parsed pool_id mismatch, skipping. Expected: %s, Got: %s",
				poolStatePK.String(), tickArrayStateAccount.PoolId.String())
			continue
		}

		// 转换为 TickArrayState（去掉 discriminator 相关字段）
		tickArrayState := amm_v3.TickArrayState{
			PoolId:               tickArrayStateAccount.PoolId,
			StartTickIndex:       tickArrayStateAccount.StartTickIndex,
			Ticks:                tickArrayStateAccount.Ticks,
			InitializedTickCount: tickArrayStateAccount.InitializedTickCount,
			RecentEpoch:          tickArrayStateAccount.RecentEpoch,
			Padding:              tickArrayStateAccount.Padding,
		}

		l.Infof("processing tick array: start_tick_index=%d, initialized_count=%d",
			tickArrayState.StartTickIndex, tickArrayState.InitializedTickCount)

		// 遍历整个 ticks 数组（60 个），根据 Raydium 代码（client/src/main.rs:2258-2262）
		// 只处理 liquidity_gross != 0 的 tick（表示已初始化）
		for i := 0; i < len(tickArrayState.Ticks); i++ {
			tickState := tickArrayState.Ticks[i]
			// 只处理已初始化的 tick（liquidity_gross > 0）
			if tickState.LiquidityGross.Hi == 0 && tickState.LiquidityGross.Lo == 0 {
				continue
			}

			// 获取 tick 值
			// 根据 Raydium 代码，TickState 中的 tick 字段存储了实际的 tick 值
			tick := tickState.Tick

			// 如果 tick 为 0，可能是未初始化的 tick，使用 start_tick_index + i 计算
			// 但根据 Raydium 代码，tick 字段应该已经存储了正确的值
			if tick == 0 && tickState.LiquidityGross.Hi == 0 && tickState.LiquidityGross.Lo == 0 {
				continue
			}

			// 计算 tick 对应的价格
			price := tickToPrice(tick)
			// 调整小数位数
			decimalsDiff := decimals1 - decimals0
			if decimalsDiff != 0 {
				price = price * math.Pow10(int(decimalsDiff))
			}

			// 转换流动性为字符串
			liquidity := uint128ToFloat64(tickState.LiquidityGross)

			allDataPoints = append(allDataPoints, &market.DepthDataPoint{
				Price:     price,
				Liquidity: fmt.Sprintf("%.0f", liquidity),
				Tick:      tick,
			})
		}
	}

	l.Infof("total data points collected: %d", len(allDataPoints))

	return &market.GetClmmPoolDepthDataResponse{
		Count:        int64(len(allDataPoints)),
		Line:         allDataPoints,
		TimeRangeMin: 0, // 历史价格范围现在从池子详情接口获取
		TimeRangeMax: 0, // 历史价格范围现在从池子详情接口获取
	}, nil
}

// tickToPrice 将 tick 转换为价格
func tickToPrice(tick int32) float64 {
	return math.Pow(1.0001, float64(tick))
}

// uint128ToFloat64 将 Uint128 转换为 float64
func uint128ToFloat64(u ag_binary.Uint128) float64 {
	return float64(u.Hi)*math.Pow(2, 64) + float64(u.Lo)
}
