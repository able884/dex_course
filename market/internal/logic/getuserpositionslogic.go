package logic

import (
	"context"
	"fmt"
	"math"
	"time"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	raydiumcpmm "richcode.cc/dex/pkg/raydium/cpmm"
)

type GetUserPositionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserPositionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPositionsLogic {
	return &GetUserPositionsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetUserPositionsLogic) GetUserPositions(in *market.GetUserPositionsRequest) (*market.GetUserPositionsResponse, error) {
	if in.UserWalletAddress == "" {
		return &market.GetUserPositionsResponse{
			Items:    []*market.PositionItem{},
			Total:    0,
			PageNo:   1,
			PageSize: 100,
		}, nil
	}

	// 设置默认值
	chainId := in.ChainId
	if chainId == 0 {
		chainId = 100000 // Solana
	}
	poolType := in.PoolType
	if poolType == "" {
		poolType = "all"
	}
	pageNo := in.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000 // 限制最大页面大小
	}

	// 查询 CLMM 持仓
	var positions []*market.PositionItem
	if poolType == "all" || poolType == "clmm" {
		clmmPositions, err := l.fetchClmmPositions(chainId, in.UserWalletAddress, pageNo, pageSize)
		if err != nil {
			logx.Errorf("Failed to fetch CLMM positions: %v", err)
		} else {
			positions = append(positions, clmmPositions...)
		}
	}

	// 查询 CPMM 持仓
	if poolType == "all" || poolType == "cpmm" {
		cpmmPositions, err := l.fetchCpmmPositions(chainId, in.UserWalletAddress, pageNo, pageSize)
		if err != nil {
			logx.Errorf("Failed to fetch CPMM positions: %v", err)
		} else {
			positions = append(positions, cpmmPositions...)
		}
	}

	return &market.GetUserPositionsResponse{
		Items:    positions,
		Total:    int32(len(positions)),
		PageNo:   pageNo,
		PageSize: pageSize,
	}, nil
}

// fetchClmmPositions 查询 CLMM 持仓
func (l *GetUserPositionsLogic) fetchClmmPositions(chainId int64, userWalletAddress string, pageNo, pageSize int32) ([]*market.PositionItem, error) {
	// 查询持仓数据
	positions, err := l.svcCtx.ClmmPositionModel.FindByUserWallet(l.ctx, userWalletAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to query positions: %w", err)
	}

	if len(positions) == 0 {
		return []*market.PositionItem{}, nil
	}

	// 收集池子地址
	poolStates := make(map[string]bool)
	for _, pos := range positions {
		poolStates[pos.PoolState] = true
	}

	// 获取池子信息
	poolMap, err := l.fetchPoolMap(chainId, poolStates)
	if err != nil {
		logx.Errorf("Failed to fetch pool map: %v", err)
	}

	// 从池子信息中收集代币地址
	tokenAddresses := make(map[string]bool)
	for _, pool := range poolMap {
		switch p := pool.(type) {
		case *solmodel.ClmmPoolInfoV1:
			tokenAddresses[p.InputVaultMint] = true
			tokenAddresses[p.OutputVaultMint] = true
		case *solmodel.ClmmPoolInfoV2:
			tokenAddresses[p.InputVaultMint] = true
			tokenAddresses[p.OutputVaultMint] = true
		}
	}

	// 获取代币信息
	tokenMap, err := l.fetchTokenMap(chainId, tokenAddresses)
	if err != nil {
		logx.Errorf("Failed to fetch token map: %v", err)
		tokenMap = make(map[string]*solmodel.Token)
	}

	// 构建响应
	result := make([]*market.PositionItem, 0, len(positions))
	for _, pos := range positions {
		item := l.buildPositionItem(pos, poolMap, tokenMap, chainId)
		if item != nil {
			result = append(result, item)
		}
	}

	return result, nil
}

// fetchPoolMap 批量获取池子信息
func (l *GetUserPositionsLogic) fetchPoolMap(chainId int64, poolStates map[string]bool) (map[string]interface{}, error) {
	poolMap := make(map[string]interface{})
	poolStateList := make([]string, 0, len(poolStates))
	for ps := range poolStates {
		poolStateList = append(poolStateList, ps)
	}

	// 查询 V1 池子
	var v1Pools []*solmodel.ClmmPoolInfoV1
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.ClmmPoolInfoV1{}).
		Where("pool_state IN ?", poolStateList).
		Find(&v1Pools).Error
	if err == nil {
		for _, pool := range v1Pools {
			poolMap[pool.PoolState] = pool
		}
	}

	// 查询 V2 池子
	var v2Pools []*solmodel.ClmmPoolInfoV2
	err = l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.ClmmPoolInfoV2{}).
		Where("pool_state IN ?", poolStateList).
		Find(&v2Pools).Error
	if err == nil {
		for _, pool := range v2Pools {
			poolMap[pool.PoolState] = pool
		}
	}

	return poolMap, nil
}

// fetchTokenMap 批量获取代币信息
func (l *GetUserPositionsLogic) fetchTokenMap(chainId int64, tokenAddresses map[string]bool) (map[string]*solmodel.Token, error) {
	tokenList := make([]string, 0, len(tokenAddresses))
	for addr := range tokenAddresses {
		tokenList = append(tokenList, addr)
	}

	if len(tokenList) == 0 {
		return make(map[string]*solmodel.Token), nil
	}

	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	tokens, err := tokenModel.FindAllByAddresses(l.ctx, chainId, tokenList)
	if err != nil {
		return nil, err
	}

	tokenMap := make(map[string]*solmodel.Token, len(tokens))
	for i := range tokens {
		tokenMap[tokens[i].Address] = &tokens[i]
	}

	return tokenMap, nil
}

// buildPositionItem 构建持仓项
func (l *GetUserPositionsLogic) buildPositionItem(
	pos *solmodel.ClmmPosition,
	poolMap map[string]interface{},
	tokenMap map[string]*solmodel.Token,
	chainId int64,
) *market.PositionItem {
	item := &market.PositionItem{
		ChainId:            chainId,
		UserWalletAddress:  pos.UserWalletAddress,
		PoolState:          pos.PoolState,
		PositionNftMint:    pos.PositionNftMint,
		PositionNftAccount: pos.PositionNftAccount,
		PersonalPosition:   pos.PersonalPosition,
		TickLowerIndex:     pos.TickLowerIndex,
		TickUpperIndex:     pos.TickUpperIndex,
		Liquidity:          pos.Liquidity,
		TxHash:             pos.TxHash,
		BlockTime:          pos.BlockTimeStamp,
		CreatedAt:          pos.CreatedAt.Unix(),
		UpdatedAt:          pos.UpdatedAt.Unix(),
	}

	// 从池子信息中获取代币地址和池子版本
	var token0Mint, token1Mint string
	var poolVersion int32
	var feeTier int64

	if pool, ok := poolMap[pos.PoolState]; ok {
		switch p := pool.(type) {
		case *solmodel.ClmmPoolInfoV1:
			token0Mint = p.InputVaultMint
			token1Mint = p.OutputVaultMint
			poolVersion = 1
			feeTier = p.TradeFeeRate
		case *solmodel.ClmmPoolInfoV2:
			token0Mint = p.InputVaultMint
			token1Mint = p.OutputVaultMint
			poolVersion = 2
			feeTier = p.TradeFeeRate
		}
	} else {
		// 如果池子信息不存在，使用持仓中的代币地址（如果有）
		// 这里需要从持仓数据中获取，但当前表结构中没有直接存储
		// 暂时返回 nil，后续可以从链上获取
		return nil
	}

	item.PoolVersion = poolVersion
	item.FeeTier = feeTier
	item.Token0Mint = token0Mint
	item.Token1Mint = token1Mint

	// 获取代币信息
	if token0, ok := tokenMap[token0Mint]; ok {
		item.Token0Symbol = token0.Symbol
		item.Token0Name = token0.Name
		item.Token0Decimals = int32(token0.Decimals)
		item.Token0Icon = token0.Icon
	}
	if token1, ok := tokenMap[token1Mint]; ok {
		item.Token1Symbol = token1.Symbol
		item.Token1Name = token1.Name
		item.Token1Decimals = int32(token1.Decimals)
		item.Token1Icon = token1.Icon
	}

	// 计算价格范围（从 tick 索引计算）和当前价格（从数据库读取）
	if poolVersion == 1 || poolVersion == 2 {
		// 获取代币 decimals
		var decimals0, decimals1 int64
		if token0, ok := tokenMap[token0Mint]; ok {
			decimals0 = int64(token0.Decimals)
		}
		if token1, ok := tokenMap[token1Mint]; ok {
			decimals1 = int64(token1.Decimals)
		}

		// 优先使用数据库中的价格（consumer 服务会定期更新）
		var currentPrice float64
		if pool, ok := poolMap[pos.PoolState]; ok {
			switch p := pool.(type) {
			case *solmodel.ClmmPoolInfoV1:
				currentPrice = p.CurrentPrice
			case *solmodel.ClmmPoolInfoV2:
				currentPrice = p.CurrentPrice
			}
		}

		// 如果数据库中没有价格或价格为0，使用中间值估算（避免链上查询影响性能）
		// 注意：如果需要实时价格，应该通过 consumer 服务定期更新数据库中的价格
		if currentPrice <= 0 {
			// 使用价格范围的中间值作为估算
			priceMin, priceMax := l.calculatePriceRangeFromTicks(
				pos.TickLowerIndex,
				pos.TickUpperIndex,
				decimals0,
				decimals1,
				token0Mint,
				token1Mint,
			)
			currentPrice = (priceMin + priceMax) / 2
			logx.Debugf("Using estimated price for pool %s: %f (price range: %f - %f)",
				pos.PoolState, currentPrice, priceMin, priceMax)
		}

		// 计算价格范围（从 tick 索引计算，考虑 decimals）
		priceMin, priceMax := l.calculatePriceRangeFromTicks(
			pos.TickLowerIndex,
			pos.TickUpperIndex,
			decimals0,
			decimals1,
			token0Mint,
			token1Mint,
		)
		item.PriceMin = priceMin
		item.PriceMax = priceMax

		// 设置当前价格
		if currentPrice > 0 {
			item.CurrentPrice = currentPrice
			// 判断是否在范围内
			item.IsInRange = currentPrice >= priceMin && currentPrice <= priceMax
		} else {
			// 如果数据库中没有价格，使用中间值作为估算
			item.CurrentPrice = (priceMin + priceMax) / 2
			item.IsInRange = true // 默认认为在范围内
		}

		item.Token0Amount = pos.Token0Amount
		item.Token1Amount = pos.Token1Amount
	}

	// 从数据库读取持仓价值和未提取手续费（在 consumer 中已更新）
	item.PositionValue = pos.PositionValueUsd
	item.UnclaimedFees = pos.UnclaimedFeesUsd

	return item
}

// calculatePriceRangeFromTicks 从 tick 索引计算价格范围
// CLMM 中 tick 到价格的转换公式需要考虑代币的 decimals
// 基础公式: price_base = 1.0001^tick
// 实际价格: price = price_base * (10^decimals0) / (10^decimals1)
// 其中 tick 对应的是 token_mint_1/token_mint_0（按地址排序）
func (l *GetUserPositionsLogic) calculatePriceRangeFromTicks(
	tickLower, tickUpper int32,
	decimals0, decimals1 int64,
	token0Mint, token1Mint string,
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

// fetchCpmmPositions 查询 CPMM 持仓
func (l *GetUserPositionsLogic) fetchCpmmPositions(chainId int64, userWalletAddress string, pageNo, pageSize int32) ([]*market.PositionItem, error) {
	// 第一步：查询所有 CPMM 池子（限制数量以提高性能，最多查询 200 个池子）
	var cpmmPools []*solmodel.CpmmPoolInfo
	limit := 200 // 固定限制，避免查询过多池子
	if int(pageSize*3) < limit {
		limit = int(pageSize * 3) // 至少查询 3 倍分页大小的池子
	}
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.CpmmPoolInfo{}).
		Order("created_at DESC").
		Limit(limit).
		Find(&cpmmPools).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query CPMM pools: %w", err)
	}

	if len(cpmmPools) == 0 {
		return []*market.PositionItem{}, nil
	}

	// 第二步：批量派生所有 LP Mint 地址
	lpMintMap := make(map[string]string) // poolState -> lpMint
	lpMints := make([]string, 0, len(cpmmPools))
	for _, pool := range cpmmPools {
		lpMint, err := l.deriveCpmmLpMint(pool.PoolState, chainId)
		if err != nil {
			logx.Debugf("Failed to derive LP mint for pool %s: %v", pool.PoolState, err)
			continue
		}
		lpMintMap[pool.PoolState] = lpMint
		lpMints = append(lpMints, lpMint)
	}

	if len(lpMints) == 0 {
		return []*market.PositionItem{}, nil
	}

	// 批量查询所有 LP Token 余额（从数据库）- 使用批量查询方法避免 N+1 问题
	lpBalanceMap := make(map[string]float64) // lpMint -> balance
	if l.svcCtx.SolTokenAccountModel != nil && len(lpMints) > 0 {
		// 使用批量查询方法，一次性查询所有 LP Mint 的余额
		balances, err := l.svcCtx.SolTokenAccountModel.SumByOwnerAndTokens(l.ctx, chainId, userWalletAddress, lpMints)
		if err == nil {
			lpBalanceMap = balances
		} else {
			logx.Infof("Failed to batch query LP balances: %v, falling back to individual queries", err)
			// 回退到单个查询（虽然慢，但至少能工作）
			for _, lpMint := range lpMints {
				balance, err := l.svcCtx.SolTokenAccountModel.SumByOwnerAndToken(l.ctx, chainId, userWalletAddress, lpMint)
				if err == nil && balance > 0 {
					lpBalanceMap[lpMint] = balance
				}
			}
		}
	}

	// 第三步：过滤出有余额的池子
	poolsWithBalance := make([]*solmodel.CpmmPoolInfo, 0)
	for _, pool := range cpmmPools {
		lpMint, ok := lpMintMap[pool.PoolState]
		if !ok {
			continue
		}
		balance, hasBalance := lpBalanceMap[lpMint]
		if !hasBalance || balance <= 0 {
			// 如果数据库中没有余额，跳过（不再进行链上查询，以提高性能）
			continue
		}
		poolsWithBalance = append(poolsWithBalance, pool)
	}

	// 应用分页
	offset := (pageNo - 1) * pageSize
	end := offset + pageSize
	if offset >= int32(len(poolsWithBalance)) {
		return []*market.PositionItem{}, nil
	}
	if end > int32(len(poolsWithBalance)) {
		end = int32(len(poolsWithBalance))
	}
	poolsWithBalance = poolsWithBalance[offset:end]

	if len(poolsWithBalance) == 0 {
		return []*market.PositionItem{}, nil
	}

	// 第四步：收集代币地址并批量查询代币信息
	tokenAddresses := make(map[string]bool)
	for _, pool := range poolsWithBalance {
		tokenAddresses[pool.InputTokenMint] = true
		tokenAddresses[pool.OutputTokenMint] = true
	}

	tokenMap, err := l.fetchTokenMap(chainId, tokenAddresses)
	if err != nil {
		logx.Errorf("Failed to fetch token map: %v", err)
		tokenMap = make(map[string]*solmodel.Token)
	}

	// 第五步：构建持仓项
	result := make([]*market.PositionItem, 0, len(poolsWithBalance))
	for _, pool := range poolsWithBalance {
		lpMint := lpMintMap[pool.PoolState]
		lpBalance := lpBalanceMap[lpMint]
		if lpBalance <= 0 {
			continue
		}

		item := l.buildCpmmPositionItem(pool, lpBalance, tokenMap, chainId, userWalletAddress)
		if item != nil {
			result = append(result, item)
		}
	}

	return result, nil
}

// deriveCpmmLpMint 派生 CPMM LP Mint 地址
func (l *GetUserPositionsLogic) deriveCpmmLpMint(poolState string, chainID int64) (string, error) {
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

// buildCpmmPositionItem 构建 CPMM 持仓项
func (l *GetUserPositionsLogic) buildCpmmPositionItem(
	pool *solmodel.CpmmPoolInfo,
	lpBalance float64,
	tokenMap map[string]*solmodel.Token,
	chainId int64,
	userWalletAddress string,
) *market.PositionItem {
	item := &market.PositionItem{
		ChainId:           chainId,
		UserWalletAddress: userWalletAddress,
		PoolState:         pool.PoolState,
		PoolVersion:       3, // CPMM
		FeeTier:           pool.TradeFeeRate,
		LpBalance:         lpBalance,
		Token0Mint:        pool.InputTokenMint,
		Token1Mint:        pool.OutputTokenMint,
		CreatedAt:         pool.CreatedAt.Unix(),
		UpdatedAt:         pool.UpdatedAt.Unix(),
	}

	// 获取代币信息
	if token0, ok := tokenMap[pool.InputTokenMint]; ok {
		item.Token0Symbol = token0.Symbol
		item.Token0Name = token0.Name
		item.Token0Decimals = int32(token0.Decimals)
		item.Token0Icon = token0.Icon
	}
	if token1, ok := tokenMap[pool.OutputTokenMint]; ok {
		item.Token1Symbol = token1.Symbol
		item.Token1Name = token1.Name
		item.Token1Decimals = int32(token1.Decimals)
		item.Token1Icon = token1.Icon
	}

	// 计算用户持仓的代币数量和价值
	// 需要获取池子的储备量和 LP 总供应量
	if l.svcCtx.SolCli != nil {
		ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
		defer cancel()

		// 获取池子储备量
		inputReserve, outputReserve, err := l.getPoolReserves(ctx, pool.InputVault, pool.OutputVault)
		if err == nil {
			// 获取 LP 总供应量
			lpMint, err := l.deriveCpmmLpMint(pool.PoolState, chainId)
			if err == nil {
				lpSupply, err := l.getLpSupply(ctx, lpMint)
				if err == nil && lpSupply > 0 {
					// 计算用户份额
					userShare := lpBalance / lpSupply
					item.Token0Amount = inputReserve * userShare
					item.Token1Amount = outputReserve * userShare

					// 计算持仓价值（需要代币价格，这里先使用池子流动性作为估算）
					if pool.Liquidity > 0 {
						item.LpValue = pool.Liquidity * userShare
						item.PositionValue = item.LpValue
					}
				}
			}
		}
	}

	// 设置池子信息
	item.PoolLiquidityUsd = pool.Liquidity
	item.PoolVolume_24H = pool.Volume24h
	item.PoolApr = pool.Apr24h

	return item
}

// getPoolReserves 获取池子储备量
func (l *GetUserPositionsLogic) getPoolReserves(ctx context.Context, inputVault, outputVault string) (float64, float64, error) {
	var inputReserve, outputReserve float64

	if inputVault != "" {
		if val, err := l.getTokenAccountBalance(ctx, inputVault); err == nil {
			inputReserve = val
		}
	}
	if outputVault != "" {
		if val, err := l.getTokenAccountBalance(ctx, outputVault); err == nil {
			outputReserve = val
		}
	}

	return inputReserve, outputReserve, nil
}

// getTokenAccountBalance 获取代币账户余额
func (l *GetUserPositionsLogic) getTokenAccountBalance(ctx context.Context, account string) (float64, error) {
	if l.svcCtx.SolCli == nil || account == "" {
		return 0, fmt.Errorf("solana rpc client not configured")
	}
	pub, err := ag_solanago.PublicKeyFromBase58(account)
	if err != nil {
		return 0, fmt.Errorf("invalid account: %w", err)
	}
	resp, err := l.svcCtx.SolCli.GetTokenAccountBalance(ctx, pub, ag_rpc.CommitmentFinalized)
	if err != nil {
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

// getLpSupply 获取 LP Token 总供应量
func (l *GetUserPositionsLogic) getLpSupply(ctx context.Context, lpMint string) (float64, error) {
	if l.svcCtx.SolCli == nil || lpMint == "" {
		return 0, fmt.Errorf("solana rpc client not configured")
	}
	mintPk, err := ag_solanago.PublicKeyFromBase58(lpMint)
	if err != nil {
		return 0, fmt.Errorf("invalid LP mint: %w", err)
	}
	resp, err := l.svcCtx.SolCli.GetTokenSupply(ctx, mintPk, ag_rpc.CommitmentFinalized)
	if err != nil {
		return 0, err
	}
	if resp == nil || resp.Value == nil {
		return 0, fmt.Errorf("empty token supply")
	}
	if resp.Value.UiAmount != nil {
		return *resp.Value.UiAmount, nil
	}
	return 0, fmt.Errorf("no ui amount in response")
}
