package logic

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/chain"
)

type ClmmPoolListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetClmmPoolListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClmmPoolListLogic {
	return &ClmmPoolListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// poolInfo 统一的池子信息接口
type poolInfo interface {
	GetPoolState() string
	GetInputVaultMint() string
	GetOutputVaultMint() string
	GetTradeFeeRate() int64
	GetCreatedAtUnix() int64
	GetLiquidityUsd() float64
	GetTxs24h() uint32
	GetVol24h() float64
	GetApr() float64
}

// v1PoolWrapper 包装 V1 池子使其实现 poolInfo 接口
type v1PoolWrapper struct {
	*solmodel.ClmmPoolInfoV1
}

func (p *v1PoolWrapper) GetPoolState() string       { return p.PoolState }
func (p *v1PoolWrapper) GetInputVaultMint() string  { return p.InputVaultMint }
func (p *v1PoolWrapper) GetOutputVaultMint() string { return p.OutputVaultMint }
func (p *v1PoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }
func (p *v1PoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }
func (p *v1PoolWrapper) GetLiquidityUsd() float64   { return 0 }
func (p *v1PoolWrapper) GetTxs24h() uint32          { return 0 }
func (p *v1PoolWrapper) GetVol24h() float64         { return 0 }
func (p *v1PoolWrapper) GetApr() float64            { return 0 }

// v2PoolWrapper 包装 V2 池子使其实现 poolInfo 接口
type v2PoolWrapper struct {
	*solmodel.ClmmPoolInfoV2
}

func (p *v2PoolWrapper) GetPoolState() string       { return p.PoolState }
func (p *v2PoolWrapper) GetInputVaultMint() string  { return p.InputVaultMint }
func (p *v2PoolWrapper) GetOutputVaultMint() string { return p.OutputVaultMint }
func (p *v2PoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }
func (p *v2PoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }
func (p *v2PoolWrapper) GetLiquidityUsd() float64   { return 0 }
func (p *v2PoolWrapper) GetTxs24h() uint32          { return 0 }
func (p *v2PoolWrapper) GetVol24h() float64         { return 0 }
func (p *v2PoolWrapper) GetApr() float64            { return 0 }

// cpmmPoolWrapper 包装 CPMM 池子实现 poolInfo 接口
type cpmmPoolWrapper struct {
	*solmodel.CpmmPoolInfo
}

func (p *cpmmPoolWrapper) GetPoolState() string       { return p.PoolState }
func (p *cpmmPoolWrapper) GetInputVaultMint() string  { return p.InputTokenMint }
func (p *cpmmPoolWrapper) GetOutputVaultMint() string { return p.OutputTokenMint }
func (p *cpmmPoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }
func (p *cpmmPoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }
func (p *cpmmPoolWrapper) GetLiquidityUsd() float64   { return p.Liquidity }
func (p *cpmmPoolWrapper) GetTxs24h() uint32          { return 0 }
func (p *cpmmPoolWrapper) GetVol24h() float64         { return p.Volume24h }
func (p *cpmmPoolWrapper) GetApr() float64            { return p.Apr24h }

func (l *ClmmPoolListLogic) GetClmmPoolList(in *market.GetClmmPoolListRequest) (*market.GetClmmPoolListResponse, error) {
	// 从数据库获取池子列表
	pools, tokenAddresses, err := l.fetchPools(in.PoolVersion, in.PageNo, in.PageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pools: %w", err)
	}

	if len(pools) == 0 {
		return &market.GetClmmPoolListResponse{
			List:  []*market.ClmmPoolItem{},
			Total: 0,
		}, nil
	}

	// 获取代币信息映射
	tokenMap, err := l.fetchTokenMap(in.ChainId, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token map: %w", err)
	}

	// 构建响应列表
	resultList := l.buildPoolItemList(pools, tokenMap, in.ChainId, in.PoolVersion)

	return &market.GetClmmPoolListResponse{
		List:  resultList,
		Total: int32(len(resultList)),
	}, nil
}

// fetchPools 根据版本获取池子列表和代币地址
func (l *ClmmPoolListLogic) fetchPools(poolVersion, pageNo, pageSize int32) ([]poolInfo, []string, error) {
	offset := (pageNo - 1) * pageSize
	var tokenAddresses []string

	if poolVersion == 1 {
		var pools []*solmodel.ClmmPoolInfoV1
		err := l.svcCtx.DB.WithContext(l.ctx).
			Model(&solmodel.ClmmPoolInfoV1{}).
			Order("created_at DESC").
			Limit(int(pageSize)).
			Offset(int(offset)).
			Find(&pools).Error

		if err != nil {
			return nil, nil, err
		}

		// 转换为统一接口并收集代币地址
		poolList := make([]poolInfo, 0, len(pools))
		for _, pool := range pools {
			poolList = append(poolList, &v1PoolWrapper{pool})
			tokenAddresses = append(tokenAddresses, pool.InputVaultMint, pool.OutputVaultMint)
		}
		return poolList, tokenAddresses, nil
	}

	// CPMM 池子：约定 poolVersion == 3
	if poolVersion == 3 {
		return l.fetchCpmmPools(pageNo, pageSize)
	}

	// V2 池子
	var pools []*solmodel.ClmmPoolInfoV2
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.ClmmPoolInfoV2{}).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&pools).Error

	if err != nil {
		return nil, nil, err
	}

	// 转换为统一接口并收集代币地址
	poolList := make([]poolInfo, 0, len(pools))
	for _, pool := range pools {
		poolList = append(poolList, &v2PoolWrapper{pool})
		tokenAddresses = append(tokenAddresses, pool.InputVaultMint, pool.OutputVaultMint)
	}
	return poolList, tokenAddresses, nil
}

// fetchCpmmPools CPMM 池子查询
func (l *ClmmPoolListLogic) fetchCpmmPools(pageNo, pageSize int32) ([]poolInfo, []string, error) {
	offset := (pageNo - 1) * pageSize
	var pools []*solmodel.CpmmPoolInfo
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.CpmmPoolInfo{}).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&pools).Error
	if err != nil {
		return nil, nil, err
	}

	poolList := make([]poolInfo, 0, len(pools))
	tokenAddresses := make([]string, 0, len(pools)*2)
	for _, pool := range pools {
		poolList = append(poolList, &cpmmPoolWrapper{pool})
		tokenAddresses = append(tokenAddresses, pool.InputTokenMint, pool.OutputTokenMint)
	}
	return poolList, tokenAddresses, nil
}

// fetchTokenMap 批量获取代币信息并构建映射
func (l *ClmmPoolListLogic) fetchTokenMap(chainId int64, addresses []string) (map[string]*solmodel.Token, error) {
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	tokenList, err := tokenModel.FindAllByAddresses(l.ctx, chainId, addresses)
	if err != nil {
		logx.Errorf("Failed to get token info: %v", err)
		return nil, err
	}

	tokenMap := make(map[string]*solmodel.Token, len(tokenList))
	for i := range tokenList {
		tokenMap[tokenList[i].Address] = &tokenList[i]
	}
	return tokenMap, nil
}

// buildPoolItemList 构建池子项列表
func (l *ClmmPoolListLogic) buildPoolItemList(
	pools []poolInfo,
	tokenMap map[string]*solmodel.Token,
	chainId int64,
	poolVersion int32,
) []*market.ClmmPoolItem {
	resultList := make([]*market.ClmmPoolItem, 0, len(pools))
	chainIcon := chain.ChainId2ChainIcon(chainId)

	for _, pool := range pools {
		poolItem := l.buildPoolItem(pool, tokenMap, poolVersion)
		poolItem.ChainId = chainId
		poolItem.ChainIcon = chainIcon
		resultList = append(resultList, poolItem)
	}

	return resultList
}

// buildPoolItem 构建单个池子项
func (l *ClmmPoolListLogic) buildPoolItem(
	pool poolInfo,
	tokenMap map[string]*solmodel.Token,
	poolVersion int32,
) *market.ClmmPoolItem {
	inputMint := pool.GetInputVaultMint()
	outputMint := pool.GetOutputVaultMint()

	// 获取输入代币信息
	inputToken := tokenMap[inputMint]
	inputSymbol, inputIcon := l.getTokenInfo(inputToken)

	// 获取输出代币信息
	outputToken := tokenMap[outputMint]
	outputSymbol, outputIcon := l.getTokenInfo(outputToken)

	return &market.ClmmPoolItem{
		PoolState:         pool.GetPoolState(),
		InputVaultMint:    inputMint,
		OutputVaultMint:   outputMint,
		InputTokenSymbol:  inputSymbol,
		OutputTokenSymbol: outputSymbol,
		InputTokenIcon:    inputIcon,
		OutputTokenIcon:   outputIcon,
		TradeFeeRate:      pool.GetTradeFeeRate(),
		LaunchTime:        pool.GetCreatedAtUnix(),
		LiquidityUsd:      pool.GetLiquidityUsd(),
		Txs_24H:           pool.GetTxs24h(),
		Vol_24H:           pool.GetVol24h(),
		Apr:               pool.GetApr(),
		PoolVersion:       poolVersion,
	}
}

// getTokenInfo 获取代币符号和图标
func (l *ClmmPoolListLogic) getTokenInfo(token *solmodel.Token) (symbol, icon string) {
	if token != nil {
		return token.Symbol, token.Icon
	}
	return "Unknown", ""
}
