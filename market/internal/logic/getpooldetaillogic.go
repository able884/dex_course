package logic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	raydiumcpmm "richcode.cc/dex/pkg/raydium/cpmm"
)

type GetPoolDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPoolDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPoolDetailLogic {
	return &GetPoolDetailLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetPoolDetailLogic) GetPoolDetail(in *market.GetPoolDetailRequest) (*market.GetPoolDetailResponse, error) {
	if in == nil || in.PoolState == "" {
		return nil, fmt.Errorf("pool_state is required")
	}

	// 默认使用 CPMM(3)
	poolVersion := in.PoolVersion
	if poolVersion == 0 {
		poolVersion = 3
	}

	var (
		inputMint   string
		outputMint  string
		inputVault  string
		outputVault string
		tradeFee    int64
		launchTime  int64
		liquidity   float64
		vol24h      float64
		apr         float64
		txs24h      uint32
		ammConfig   string
		lpMint      string
	)

	switch poolVersion {
	case 3:
		pool, err := solmodel.NewCpmmPoolInfoModel(l.svcCtx.DB).FindOneByPoolState(l.ctx, in.PoolState)
		if err != nil {
			l.Errorf("GetPoolDetail FindOneByPoolState cpmm failed, poolState:%s err:%v", in.PoolState, err)
			return nil, err
		}
		if pool == nil {
			return nil, fmt.Errorf("pool not found")
		}
		inputMint = pool.InputTokenMint
		outputMint = pool.OutputTokenMint
		inputVault = pool.InputVault
		outputVault = pool.OutputVault
		tradeFee = pool.TradeFeeRate
		launchTime = pool.CreatedAt.Unix()
		liquidity = pool.Liquidity
		vol24h = pool.Volume24h
		apr = pool.Apr24h
		ammConfig = pool.AmmConfig
		lpMint = pool.LpMint
	case 1:
		pool, err := solmodel.NewClmmPoolInfoV1Model(l.svcCtx.DB).FindOneByPoolState(l.ctx, in.PoolState)
		if err != nil {
			l.Errorf("GetPoolDetail FindOneByPoolState clmm v1 failed, poolState:%s err:%v", in.PoolState, err)
			return nil, err
		}
		if pool == nil {
			return nil, fmt.Errorf("pool not found")
		}
		inputMint = pool.InputVaultMint
		outputMint = pool.OutputVaultMint
		inputVault = pool.InputVault
		outputVault = pool.OutputVault
		tradeFee = pool.TradeFeeRate
		launchTime = pool.CreatedAt.Unix()
	case 2:
		pool, err := solmodel.NewClmmPoolInfoV2Model(l.svcCtx.DB).FindOneByPoolState(l.ctx, in.PoolState)
		if err != nil {
			l.Errorf("GetPoolDetail FindOneByPoolState clmm v2 failed, poolState:%s err:%v", in.PoolState, err)
			return nil, err
		}
		if pool == nil {
			return nil, fmt.Errorf("pool not found")
		}
		inputMint = pool.InputVaultMint
		outputMint = pool.OutputVaultMint
		inputVault = pool.InputVault
		outputVault = pool.OutputVault
		tradeFee = pool.TradeFeeRate
		launchTime = pool.CreatedAt.Unix()
	default:
		return nil, fmt.Errorf("unsupported pool_version: %d", poolVersion)
	}

	if poolVersion == 3 && (inputVault == "" || outputVault == "") {
		if iv, ov, err := deriveCpmmVaults(ammConfig, inputMint, outputMint, in.ChainId); err == nil {
			if inputVault == "" {
				inputVault = iv
			}
			if outputVault == "" {
				outputVault = ov
			}
		} else {
			l.Errorf("deriveCpmmVaults failed: %v", err)
		}
	}
	if poolVersion == 3 && lpMint == "" {
		if derivedLpMint, err := deriveCpmmLpMint(in.PoolState, in.ChainId); err == nil {
			lpMint = derivedLpMint
		} else {
			l.Errorf("deriveCpmmLpMint failed poolState:%s err:%v", in.PoolState, err)
		}
	}

	tokenMap, err := l.fetchTokenMap(in.ChainId, []string{inputMint, outputMint})
	if err != nil {
		l.Errorf("GetPoolDetail fetchTokenMap failed, err:%v", err)
	}
	inputSymbol, inputIcon := l.getTokenInfo(tokenMap[inputMint])
	outputSymbol, outputIcon := l.getTokenInfo(tokenMap[outputMint])

	var price float64
	var inputReserve, outputReserve float64

	if poolVersion == 1 || poolVersion == 2 {
		// CLMM 池子：从链上读取 sqrt_price_x64 计算价格
		clmmPrice, err := l.fetchClmmPriceFromChain(in.ChainId, in.PoolState, inputMint, outputMint)
		if err != nil {
			l.Errorf("GetPoolDetail fetchClmmPriceFromChain failed, poolState:%s err:%v", in.PoolState, err)
			// 如果读取失败，回退到使用 vault 余额比例
			inputReserve, outputReserve, price = l.fetchVaultReserves(in.ChainId, inputVault, outputVault)
		} else {
			price = clmmPrice
			// 仍然需要获取 vault 余额用于其他字段
			inputReserve, outputReserve, _ = l.fetchVaultReserves(in.ChainId, inputVault, outputVault)
		}
	} else {
		// CPMM 池子：使用 vault 余额比例计算价格
		inputReserve, outputReserve, price = l.fetchVaultReserves(in.ChainId, inputVault, outputVault)
	}

	var (
		userPooledInput  float64
		userPooledOutput float64
		userStakedLp     float64
		userUnstakedLp   float64
	)
	if in.UserWalletAddress != "" && lpMint != "" {
		// 1) 优先用本地分表数据
		if val, err := l.svcCtx.SolTokenAccountModel.SumByOwnerAndToken(l.ctx, in.ChainId, in.UserWalletAddress, lpMint); err != nil {
			l.Errorf("sum user lp balance failed wallet:%s lpMint:%s err:%v", in.UserWalletAddress, lpMint, err)
		} else {
			userUnstakedLp = val
		}
		// 2) 若本地为 0，则兜底通过 Solana RPC 查询
		if userUnstakedLp == 0 {
			if val, err := l.getOwnerTokenBalanceByMint(l.ctx, in.UserWalletAddress, lpMint); err != nil {
				l.Errorf("rpc fetch user lp balance failed wallet:%s lpMint:%s err:%v", in.UserWalletAddress, lpMint, err)
			} else {
				userUnstakedLp = val
			}
		}
		if totalLpSupply, err := l.getTokenSupplyUI(l.ctx, lpMint); err != nil {
			l.Errorf("get lp supply failed lpMint:%s err:%v", lpMint, err)
		} else if totalLpSupply > 0 && userUnstakedLp > 0 {
			share := userUnstakedLp / totalLpSupply
			userPooledInput = share * inputReserve
			userPooledOutput = share * outputReserve
		}
	}

	// 查询历史价格范围（24H/7D/30D）- 仅对 CLMM 池子查询
	// 从 Redis 缓存读取，不再查询数据库
	var priceRange24HMin, priceRange24HMax, priceRange7DMin, priceRange7DMax, priceRange30DMin, priceRange30DMax float64
	if poolVersion == 1 || poolVersion == 2 {
		// CLMM 池子从缓存读取历史价格范围
		if l.svcCtx.PriceRangeCache != nil {
			priceRange24HMin, priceRange24HMax, priceRange7DMin, priceRange7DMax, priceRange30DMin, priceRange30DMax = l.svcCtx.PriceRangeCache.GetPriceRanges(l.ctx, in.PoolState)
		}
		// 如果缓存中没有数据，返回 0（不再查询数据库）
	}

	return &market.GetPoolDetailResponse{
		ChainId:           in.ChainId,
		PoolState:         in.PoolState,
		InputVaultMint:    inputMint,
		OutputVaultMint:   outputMint,
		InputTokenSymbol:  inputSymbol,
		OutputTokenSymbol: outputSymbol,
		InputTokenIcon:    inputIcon,
		OutputTokenIcon:   outputIcon,
		TradeFeeRate:      tradeFee,
		LaunchTime:        launchTime,
		LiquidityUsd:      liquidity,
		Txs_24H:           txs24h,
		Vol_24H:           vol24h,
		Apr:               apr,
		PoolVersion:       poolVersion,
		LockedPercent:     0,
		InputAmount:       inputReserve,
		OutputAmount:      outputReserve,
		BaseReserve:       inputReserve,
		QuoteReserve:      outputReserve,
		InputReserve:      inputReserve,
		OutputReserve:     outputReserve,
		Price:             price,
		QuotePerBase:      price,
		MarketPrice:       price,
		UserPooledInput:   userPooledInput,
		UserPooledOutput:  userPooledOutput,
		UserStakedLp:      userStakedLp,
		UserUnstakedLp:    userUnstakedLp,
		PriceRange_24HMin: priceRange24HMin,
		PriceRange_24HMax: priceRange24HMax,
		PriceRange_7DMin:  priceRange7DMin,
		PriceRange_7DMax:  priceRange7DMax,
		PriceRange_30DMin: priceRange30DMin,
		PriceRange_30DMax: priceRange30DMax,
	}, nil
}

// fetchTokenMap 批量获取代币信息
func (l *GetPoolDetailLogic) fetchTokenMap(chainId int64, addresses []string) (map[string]*solmodel.Token, error) {
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	tokenList, err := tokenModel.FindAllByAddresses(l.ctx, chainId, addresses)
	if err != nil {
		return nil, err
	}
	tokenMap := make(map[string]*solmodel.Token, len(tokenList))
	for i := range tokenList {
		tokenMap[tokenList[i].Address] = &tokenList[i]
	}
	return tokenMap, nil
}

func (l *GetPoolDetailLogic) getTokenInfo(token *solmodel.Token) (symbol, icon string) {
	if token != nil {
		return token.Symbol, token.Icon
	}
	return "Unknown", ""
}

// fetchVaultReserves reads vault token accounts from DB; missing ones will be fetched via Solana RPC if configured.
func (l *GetPoolDetailLogic) fetchVaultReserves(chainID int64, inputVault, outputVault string) (inputReserve, outputReserve, price float64) {
	vaults := []string{}
	if inputVault != "" {
		vaults = append(vaults, inputVault)
	}
	if outputVault != "" {
		vaults = append(vaults, outputVault)
	}
	if len(vaults) == 0 {
		return 0, 0, 0
	}

	type vaultRow struct {
		TokenAccountAddress string
		TokenDecimal        int64
		Balance             int64
	}
	var rows []vaultRow
	records, err := l.svcCtx.SolTokenAccountModel.FindByTokenAccounts(l.ctx, chainID, vaults)
	if err == nil {
		for _, r := range records {
			rows = append(rows, vaultRow{
				TokenAccountAddress: r.TokenAccountAddress,
				TokenDecimal:        r.TokenDecimal,
				Balance:             r.Balance,
			})
		}
	}

	for _, row := range rows {
		denom := math.Pow10(int(row.TokenDecimal))
		ui := float64(row.Balance)
		if denom > 0 {
			ui = ui / denom
		}
		switch row.TokenAccountAddress {
		case inputVault:
			inputReserve = ui
		case outputVault:
			outputReserve = ui
		}
	}

	if l.svcCtx.SolCli != nil {
		ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
		defer cancel()

		if inputVault != "" {
			if val, err := l.getTokenAccountBalance(ctx, inputVault); err == nil {
				inputReserve = val
			} else {
				l.Errorf("fetchVaultReserves rpc inputVault:%s err:%v", inputVault, err)
			}
		}
		if outputVault != "" {
			if val, err := l.getTokenAccountBalance(ctx, outputVault); err == nil {
				outputReserve = val
			} else {
				l.Errorf("fetchVaultReserves rpc outputVault:%s err:%v", outputVault, err)
			}
		}
	}

	if inputReserve > 0 && outputReserve > 0 {
		price = outputReserve / inputReserve
	}
	return
}

func (l *GetPoolDetailLogic) getTokenAccountBalance(ctx context.Context, account string) (float64, error) {
	if l.svcCtx.SolCli == nil || account == "" {
		return 0, fmt.Errorf("solana rpc client not configured")
	}
	pub, err := ag_solanago.PublicKeyFromBase58(account)
	if err != nil {
		return 0, fmt.Errorf("invalid account: %w", err)
	}
	resp, err := l.svcCtx.SolCli.GetTokenAccountBalance(ctx, pub, ag_rpc.CommitmentFinalized)
	if err != nil {
		l.Errorf("rpc getTokenAccountBalance failed account:%s err:%v", account, err)
		return 0, err
	}
	if resp == nil || resp.Value == nil {
		return 0, fmt.Errorf("empty token account balance")
	}
	if resp.Value.UiAmount != nil {
		return *resp.Value.UiAmount, nil
	}
	return 0, fmt.Errorf("no ui amount in response")
}

// deriveCpmmLpMint computes LP mint PDA via pool_state.
func deriveCpmmLpMint(poolState string, chainID int64) (string, error) {
	if poolState == "" {
		return "", fmt.Errorf("pool state is empty")
	}
	poolPk, err := ag_solanago.PublicKeyFromBase58(poolState)
	if err != nil {
		return "", fmt.Errorf("invalid pool state: %w", err)
	}
	var programID ag_solanago.PublicKey
	if chainID == constants.SolChainIdInt {
		programID = ag_solanago.MustPublicKeyFromBase58(raydiumcpmm.ProgramRaydiumCPMMProgramDevNet.String())
	} else {
		programID = ag_solanago.MustPublicKeyFromBase58(raydiumcpmm.ProgramRaydiumCPMMProgram.String())
	}
	seeds := [][]byte{
		[]byte("pool_lp_mint"),
		poolPk[:],
	}
	lpMint, _, err := ag_solanago.FindProgramAddress(seeds, programID)
	if err != nil {
		return "", err
	}
	return lpMint.String(), nil
}

// getOwnerTokenBalanceByMint sums all token accounts of an owner for a given mint via RPC (ui amount).
func (l *GetPoolDetailLogic) getOwnerTokenBalanceByMint(ctx context.Context, owner string, mint string) (float64, error) {
	if l.svcCtx.SolCli == nil || owner == "" || mint == "" {
		return 0, fmt.Errorf("solana rpc client not configured")
	}
	ownerPk, err := ag_solanago.PublicKeyFromBase58(owner)
	if err != nil {
		return 0, fmt.Errorf("invalid owner: %w", err)
	}
	mintPk, err := ag_solanago.PublicKeyFromBase58(mint)
	if err != nil {
		return 0, fmt.Errorf("invalid mint: %w", err)
	}
	resp, err := l.svcCtx.SolCli.GetTokenAccountsByOwner(ctx, ownerPk, &ag_rpc.GetTokenAccountsConfig{
		Mint: &mintPk,
	}, &ag_rpc.GetTokenAccountsOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Value) == 0 {
		return 0, nil
	}
	var total float64
	for _, v := range resp.Value {
		val, err := l.getTokenAccountBalance(ctx, v.Pubkey.String())
		if err != nil {
			l.Errorf("getTokenAccountBalance rpc failed owner:%s account:%s err:%v", owner, v.Pubkey.String(), err)
			continue
		}
		total += val
	}
	return total, nil
}

// getTokenSupplyUI fetches mint total supply in UI amount using Solana RPC.
// queryHistoricalPriceRange 从交易历史表查询历史价格范围
func (l *GetPoolDetailLogic) queryHistoricalPriceRange(chainId int64, poolState, timeRange string) (float64, float64) {
	// 计算时间范围
	timeRangeUpper := strings.ToUpper(strings.TrimSpace(timeRange))
	var startTime time.Time
	now := time.Now()

	switch timeRangeUpper {
	case "24H", "24h":
		startTime = now.Add(-24 * time.Hour)
	case "7D", "7d":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "30D", "30d":
		startTime = now.Add(-30 * 24 * time.Hour)
	default:
		// 默认使用 24H
		startTime = now.Add(-24 * time.Hour)
	}

	// 查询交易历史表
	var prices []float64

	// 遍历时间范围内的所有日期，查询每个分片表
	currentDate := startTime
	nowTime := time.Now()

	for currentDate.Before(nowTime) || currentDate.Equal(nowTime.Truncate(24*time.Hour)) {
		// 获取分片表名
		tableName := fmt.Sprintf("trade_%d_%02d_%02d", currentDate.Year(), currentDate.Month(), currentDate.Day())

		// 查询该分片表中的价格数据
		var dayPrices []struct {
			Price float64 `gorm:"column:price"`
		}

		queryTime := currentDate
		if currentDate.Equal(startTime.Truncate(24 * time.Hour)) {
			queryTime = startTime // 使用实际的开始时间
		}

		err := l.svcCtx.DB.WithContext(l.ctx).
			Table(tableName).
			Where("chain_id = ? AND pair_addr = ? AND block_time >= ? AND block_time < ? AND deleted_at IS NULL AND base_token_price_usd > 0",
				chainId, poolState, queryTime, currentDate.Add(24*time.Hour)).
			Select("(token_price_usd / base_token_price_usd) as price").
			Find(&dayPrices).Error

		// 如果表不存在，跳过（分片表可能不存在）
		if err != nil && !strings.Contains(err.Error(), "doesn't exist") {
			// 静默失败，不记录日志
		} else {
			// 收集价格数据
			for _, p := range dayPrices {
				if p.Price > 0 {
					prices = append(prices, p.Price)
				}
			}
		}

		// 移动到下一天
		currentDate = currentDate.Add(24 * time.Hour)
	}

	// 如果没有找到数据，返回 0
	if len(prices) == 0 {
		return 0, 0
	}

	// 计算最小和最大价格
	minPrice := prices[0]
	maxPrice := prices[0]
	for _, p := range prices {
		if p < minPrice {
			minPrice = p
		}
		if p > maxPrice {
			maxPrice = p
		}
	}

	return minPrice, maxPrice
}

func (l *GetPoolDetailLogic) getTokenSupplyUI(ctx context.Context, mint string) (float64, error) {
	if l.svcCtx.SolCli == nil || mint == "" {
		return 0, fmt.Errorf("solana rpc client not configured")
	}
	mintPk, err := ag_solanago.PublicKeyFromBase58(mint)
	if err != nil {
		return 0, fmt.Errorf("invalid mint: %w", err)
	}
	resp, err := l.svcCtx.SolCli.GetTokenSupply(ctx, mintPk, ag_rpc.CommitmentFinalized)
	if err != nil {
		return 0, err
	}
	if resp == nil || resp.Value == nil {
		return 0, fmt.Errorf("empty token supply response")
	}
	if resp.Value.UiAmount != nil {
		return *resp.Value.UiAmount, nil
	}
	amount, err := strconv.ParseFloat(resp.Value.Amount, 64)
	if err != nil {
		return 0, err
	}
	denom := math.Pow10(int(resp.Value.Decimals))
	if denom == 0 {
		return 0, fmt.Errorf("invalid supply decimals")
	}
	return amount / denom, nil
}

// deriveCpmmVaults uses program seeds to compute vault PDAs; avoids relying on DB for vault addresses.
func deriveCpmmVaults(ammConfig, inputMint, outputMint string, chainID int64) (string, string, error) {
	ammPk, err := ag_solanago.PublicKeyFromBase58(ammConfig)
	if err != nil {
		return "", "", fmt.Errorf("invalid amm config: %w", err)
	}
	inMintPk, err := ag_solanago.PublicKeyFromBase58(inputMint)
	if err != nil {
		return "", "", fmt.Errorf("invalid input mint: %w", err)
	}
	outMintPk, err := ag_solanago.PublicKeyFromBase58(outputMint)
	if err != nil {
		return "", "", fmt.Errorf("invalid output mint: %w", err)
	}

	var programID ag_solanago.PublicKey
	if chainID == constants.SolChainIdInt {
		programID = ag_solanago.MustPublicKeyFromBase58(raydiumcpmm.ProgramRaydiumCPMMProgramDevNet.String())
	} else {
		programID = ag_solanago.MustPublicKeyFromBase58(raydiumcpmm.ProgramRaydiumCPMMProgram.String())
	}

	token0 := inMintPk
	token1 := outMintPk
	swap := false
	if token0.String() > token1.String() {
		token0, token1 = token1, token0
		swap = true
	}

	_, _, _, token0Vault, token1Vault, _, err := raydiumcpmm.DeriveCpmmPoolPDAs(ammPk, token0, token1, programID)
	if err != nil {
		return "", "", err
	}

	if swap {
		return token1Vault.String(), token0Vault.String(), nil
	}
	return token0Vault.String(), token1Vault.String(), nil
}

// fetchClmmPriceFromChain 从链上读取 CLMM 池子的 sqrt_price_x64 并计算价格
// 价格计算公式：price = (sqrt_price_x64 / 2^64)^2 * (10^decimals0) / (10^decimals1)
func (l *GetPoolDetailLogic) fetchClmmPriceFromChain(chainID int64, poolState, inputMint, outputMint string) (float64, error) {
	if l.svcCtx.SolCli == nil {
		return 0, errors.New("solana rpc client not configured")
	}

	// 解析 pool_state 地址
	poolStatePK, err := ag_solanago.PublicKeyFromBase58(poolState)
	if err != nil {
		return 0, fmt.Errorf("invalid pool_state: %w", err)
	}

	// 从链上获取 PoolState 账户数据
	ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
	defer cancel()

	poolStateInfo, err := l.svcCtx.SolCli.GetAccountInfoWithOpts(ctx, poolStatePK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get pool state account: %w", err)
	}
	if poolStateInfo == nil || poolStateInfo.Value == nil {
		return 0, errors.New("pool state account not found")
	}

	// 解析 PoolState 账户数据
	data := poolStateInfo.Value.Data.GetBinary()
	decoder := ag_binary.NewBorshDecoder(data)
	var poolStateAccount amm_v3.PoolStateAccount
	if err := poolStateAccount.UnmarshalWithDecoder(decoder); err != nil {
		return 0, fmt.Errorf("failed to decode pool state: %w", err)
	}

	// 获取 sqrt_price_x64
	sqrtPriceX64 := poolStateAccount.SqrtPriceX64
	poolTokenMint0 := poolStateAccount.TokenMint0.String()
	poolTokenMint1 := poolStateAccount.TokenMint1.String()
	decimals0 := int64(poolStateAccount.MintDecimals0)
	decimals1 := int64(poolStateAccount.MintDecimals1)

	// 使用公共函数计算价格
	price, err := clmm.CalculatePriceFromSqrtPriceX64(
		clmm.Uint128{
			Lo: sqrtPriceX64.Lo,
			Hi: sqrtPriceX64.Hi,
		},
		decimals0,
		decimals1,
		poolTokenMint0,
		poolTokenMint1,
		inputMint,
		outputMint,
	)
	if err != nil {
		return 0, err
	}

	// 如果 mint 地址不匹配，记录警告
	if inputMint != poolTokenMint0 && inputMint != poolTokenMint1 {
		l.Errorf("fetchClmmPriceFromChain: mint addresses don't match. inputMint=%s, outputMint=%s, poolTokenMint0=%s, poolTokenMint1=%s",
			inputMint, outputMint, poolTokenMint0, poolTokenMint1)
	}

	return price, nil
}
