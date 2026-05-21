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

// ClmmPoolListLogic 获取CLMM流动性池列表的业务逻辑处理器
type ClmmPoolListLogic struct {
	ctx    context.Context              // 上下文对象
	svcCtx *svc.ServiceContext          // 服务上下文
	logx.Logger                          // 日志记录器
}

// NewGetClmmPoolListLogic 创建获取CLMM流动性池列表的逻辑处理器
// 参数:
//   ctx - 上下文对象
//   svcCtx - 服务上下文
// 返回: 初始化后的ClmmPoolListLogic指针
func NewGetClmmPoolListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClmmPoolListLogic {
	return &ClmmPoolListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// poolInfo 统一的池子信息接口
// 该接口用于统一处理不同版本（V1、V2、CPMM）的池子数据
type poolInfo interface {
	GetPoolState() string              // 获取池子状态地址
	GetInputVaultMint() string         // 获取输入代币地址
	GetOutputVaultMint() string        // 获取输出代币地址
	GetTradeFeeRate() int64            // 获取交易费率
	GetCreatedAtUnix() int64           // 获取创建时间（Unix时间戳）
	GetLiquidityUsd() float64          // 获取流动性（USD）
	GetTxs24h() uint32                 // 获取24小时交易次数
	GetVol24h() float64                // 获取24小时交易量
	GetApr() float64                   // 获取年化收益率
}

// v1PoolWrapper 包装V1池子使其实现poolInfo接口
// 适配器模式，用于统一处理V1版本的CLMM池子数据
type v1PoolWrapper struct {
	*solmodel.ClmmPoolInfoV1  // V1池子数据模型
}

// 以下是v1PoolWrapper实现poolInfo接口的方法
func (p *v1PoolWrapper) GetPoolState() string       { return p.PoolState }              // 获取池子状态地址
func (p *v1PoolWrapper) GetInputVaultMint() string  { return p.InputVaultMint }         // 获取输入代币地址
func (p *v1PoolWrapper) GetOutputVaultMint() string { return p.OutputVaultMint }        // 获取输出代币地址
func (p *v1PoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }           // 获取交易费率
func (p *v1PoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }       // 获取创建时间
func (p *v1PoolWrapper) GetLiquidityUsd() float64   { return 0 }                        // V1池子暂不支持流动性数据
func (p *v1PoolWrapper) GetTxs24h() uint32          { return 0 }                        // V1池子暂不支持24小时交易次数
func (p *v1PoolWrapper) GetVol24h() float64         { return 0 }                        // V1池子暂不支持24小时交易量
func (p *v1PoolWrapper) GetApr() float64            { return 0 }                        // V1池子暂不支持APR

// v2PoolWrapper 包装V2池子使其实现poolInfo接口
// 适配器模式，用于统一处理V2版本的CLMM池子数据
type v2PoolWrapper struct {
	*solmodel.ClmmPoolInfoV2  // V2池子数据模型
}

// 以下是v2PoolWrapper实现poolInfo接口的方法
func (p *v2PoolWrapper) GetPoolState() string       { return p.PoolState }              // 获取池子状态地址
func (p *v2PoolWrapper) GetInputVaultMint() string  { return p.InputVaultMint }         // 获取输入代币地址
func (p *v2PoolWrapper) GetOutputVaultMint() string { return p.OutputVaultMint }        // 获取输出代币地址
func (p *v2PoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }           // 获取交易费率
func (p *v2PoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }       // 获取创建时间
func (p *v2PoolWrapper) GetLiquidityUsd() float64   { return 0 }                        // V2池子暂不支持流动性数据
func (p *v2PoolWrapper) GetTxs24h() uint32          { return 0 }                        // V2池子暂不支持24小时交易次数
func (p *v2PoolWrapper) GetVol24h() float64         { return 0 }                        // V2池子暂不支持24小时交易量
func (p *v2PoolWrapper) GetApr() float64            { return 0 }                        // V2池子暂不支持APR

// cpmmPoolWrapper 包装CPMM池子使其实现poolInfo接口
// 适配器模式，用于统一处理CPMM版本的池子数据
type cpmmPoolWrapper struct {
	*solmodel.CpmmPoolInfo  // CPMM池子数据模型
}

// 以下是cpmmPoolWrapper实现poolInfo接口的方法
func (p *cpmmPoolWrapper) GetPoolState() string       { return p.PoolState }              // 获取池子状态地址
func (p *cpmmPoolWrapper) GetInputVaultMint() string  { return p.InputTokenMint }        // 获取输入代币地址（注意：CPMM使用InputTokenMint）
func (p *cpmmPoolWrapper) GetOutputVaultMint() string { return p.OutputTokenMint }       // 获取输出代币地址（注意：CPMM使用OutputTokenMint）
func (p *cpmmPoolWrapper) GetTradeFeeRate() int64     { return p.TradeFeeRate }          // 获取交易费率
func (p *cpmmPoolWrapper) GetCreatedAtUnix() int64    { return p.CreatedAt.Unix() }      // 获取创建时间
func (p *cpmmPoolWrapper) GetLiquidityUsd() float64   { return p.Liquidity }             // 获取流动性（USD）
func (p *cpmmPoolWrapper) GetTxs24h() uint32          { return 0 }                       // CPMM池子暂不支持24小时交易次数
func (p *cpmmPoolWrapper) GetVol24h() float64         { return p.Volume24h }             // 获取24小时交易量
func (p *cpmmPoolWrapper) GetApr() float64            { return p.Apr24h }                // 获取24小时年化收益率

// GetClmmPoolList 获取CLMM流动性池列表的主方法
// 支持多种池版本：V1(1)、V2(2)、CPMM(3)
// 参数:
//   in - 请求参数，包含链ID、池版本、分页信息等
// 返回:
//   *market.GetClmmPoolListResponse - 池子列表响应
//   error - 错误信息
func (l *ClmmPoolListLogic) GetClmmPoolList(in *market.GetClmmPoolListRequest) (*market.GetClmmPoolListResponse, error) {
	// 第一步：从数据库获取池子列表，同时提取所有相关的代币地址
	pools, tokenAddresses, err := l.fetchPools(in.PoolVersion, in.PageNo, in.PageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pools: %w", err)
	}

	// 如果没有查询到任何池子，返回空列表
	if len(pools) == 0 {
		return &market.GetClmmPoolListResponse{
			List:  []*market.ClmmPoolItem{},
			Total: 0,
		}, nil
	}

	// 第二步：批量获取代币信息，构建地址到代币对象的映射
	tokenMap, err := l.fetchTokenMap(in.ChainId, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token map: %w", err)
	}

	// 第三步：构建响应列表，将池子数据和代币信息组合
	resultList := l.buildPoolItemList(pools, tokenMap, in.ChainId, in.PoolVersion)

	// 返回最终响应
	return &market.GetClmmPoolListResponse{
		List:  resultList,
		Total: int32(len(resultList)),
	}, nil
}

// fetchPools 根据池版本获取池子列表和相关代币地址
// 参数:
//   poolVersion - 池版本（1=V1, 2=V2, 3=CPMM）
//   pageNo - 页码
//   pageSize - 每页大小
// 返回:
//   []poolInfo - 池子列表（统一接口）
//   []string - 代币地址列表
//   error - 错误信息
func (l *ClmmPoolListLogic) fetchPools(poolVersion, pageNo, pageSize int32) ([]poolInfo, []string, error) {
	// 计算分页偏移量
	offset := (pageNo - 1) * pageSize
	var tokenAddresses []string

	// 处理V1版本池子
	if poolVersion == 1 {
		var pools []*solmodel.ClmmPoolInfoV1
		// 查询V1池子，按创建时间倒序排列
		err := l.svcCtx.DB.WithContext(l.ctx).
			Model(&solmodel.ClmmPoolInfoV1{}).
			Order("created_at DESC").
			Limit(int(pageSize)).
			Offset(int(offset)).
			Find(&pools).Error

		if err != nil {
			return nil, nil, err
		}

		// 将V1池子转换为统一接口，并收集相关的代币地址
		poolList := make([]poolInfo, 0, len(pools))
		for _, pool := range pools {
			poolList = append(poolList, &v1PoolWrapper{pool})  // 包装为统一接口
			tokenAddresses = append(tokenAddresses, pool.InputVaultMint, pool.OutputVaultMint)  // 收集代币地址
		}
		return poolList, tokenAddresses, nil
	}

	// 处理CPMM池子：约定 poolVersion == 3
	if poolVersion == 3 {
		return l.fetchCpmmPools(pageNo, pageSize)
	}

	// 处理V2版本池子（默认情况）
	var pools []*solmodel.ClmmPoolInfoV2
	// 查询V2池子，按创建时间倒序排列
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.ClmmPoolInfoV2{}).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&pools).Error

	if err != nil {
		return nil, nil, err
	}

	// 将V2池子转换为统一接口，并收集相关的代币地址
	poolList := make([]poolInfo, 0, len(pools))
	for _, pool := range pools {
		poolList = append(poolList, &v2PoolWrapper{pool})  // 包装为统一接口
		tokenAddresses = append(tokenAddresses, pool.InputVaultMint, pool.OutputVaultMint)  // 收集代币地址
	}
	return poolList, tokenAddresses, nil
}

// fetchCpmmPools 查询CPMM池子列表
// 参数:
//   pageNo - 页码
//   pageSize - 每页大小
// 返回:
//   []poolInfo - CPMM池子列表（统一接口）
//   []string - 代币地址列表
//   error - 错误信息
func (l *ClmmPoolListLogic) fetchCpmmPools(pageNo, pageSize int32) ([]poolInfo, []string, error) {
	// 计算分页偏移量
	offset := (pageNo - 1) * pageSize
	var pools []*solmodel.CpmmPoolInfo
	// 查询CPMM池子，按创建时间倒序排列
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&solmodel.CpmmPoolInfo{}).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&pools).Error
	if err != nil {
		return nil, nil, err
	}

	// 将CPMM池子转换为统一接口，并收集相关的代币地址
	poolList := make([]poolInfo, 0, len(pools))
	tokenAddresses := make([]string, 0, len(pools)*2)  // 每个池子有2个代币地址
	for _, pool := range pools {
		poolList = append(poolList, &cpmmPoolWrapper{pool})  // 包装为统一接口
		tokenAddresses = append(tokenAddresses, pool.InputTokenMint, pool.OutputTokenMint)  // 收集代币地址
	}
	return poolList, tokenAddresses, nil
}

// fetchTokenMap 批量获取代币信息并构建地址到代币对象的映射
// 参数:
//   chainId - 链ID
//   addresses - 代币地址列表
// 返回:
//   map[string]*solmodel.Token - 代币地址到代币对象的映射
//   error - 错误信息
func (l *ClmmPoolListLogic) fetchTokenMap(chainId int64, addresses []string) (map[string]*solmodel.Token, error) {
	// 创建代币数据模型
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	// 批量查询代币信息
	tokenList, err := tokenModel.FindAllByAddresses(l.ctx, chainId, addresses)
	if err != nil {
		logx.Errorf("Failed to get token info: %v", err)
		return nil, err
	}

	// 构建地址到代币对象的映射
	tokenMap := make(map[string]*solmodel.Token, len(tokenList))
	for i := range tokenList {
		tokenMap[tokenList[i].Address] = &tokenList[i]
	}
	return tokenMap, nil
}

// buildPoolItemList 构建池子响应项列表
// 参数:
//   pools - 池子列表（统一接口）
//   tokenMap - 代币信息映射
//   chainId - 链ID
//   poolVersion - 池版本
// 返回:
//   []*market.ClmmPoolItem - 池子响应项列表
func (l *ClmmPoolListLogic) buildPoolItemList(
	pools []poolInfo,
	tokenMap map[string]*solmodel.Token,
	chainId int64,
	poolVersion int32,
) []*market.ClmmPoolItem {
	// 初始化响应列表
	resultList := make([]*market.ClmmPoolItem, 0, len(pools))
	// 获取链图标
	chainIcon := chain.ChainId2ChainIcon(chainId)

	// 遍历每个池子，构建响应项
	for _, pool := range pools {
		// 构建单个池子项
		poolItem := l.buildPoolItem(pool, tokenMap, poolVersion)
		// 补充链ID和链图标
		poolItem.ChainId = chainId
		poolItem.ChainIcon = chainIcon
		resultList = append(resultList, poolItem)
	}

	return resultList
}

// buildPoolItem 构建单个池子响应项
// 参数:
//   pool - 池子对象（统一接口）
//   tokenMap - 代币信息映射
//   poolVersion - 池版本
// 返回:
//   *market.ClmmPoolItem - 池子响应项
func (l *ClmmPoolListLogic) buildPoolItem(
	pool poolInfo,
	tokenMap map[string]*solmodel.Token,
	poolVersion int32,
) *market.ClmmPoolItem {
	// 获取输入和输出代币地址
	inputMint := pool.GetInputVaultMint()
	outputMint := pool.GetOutputVaultMint()

	// 获取输入代币信息（符号和图标）
	inputToken := tokenMap[inputMint]
	inputSymbol, inputIcon := l.getTokenInfo(inputToken)

	// 获取输出代币信息（符号和图标）
	outputToken := tokenMap[outputMint]
	outputSymbol, outputIcon := l.getTokenInfo(outputToken)

	// 构建池子响应项
	return &market.ClmmPoolItem{
		PoolState:         pool.GetPoolState(),         // 池子状态地址
		InputVaultMint:    inputMint,                    // 输入代币地址
		OutputVaultMint:   outputMint,                   // 输出代币地址
		InputTokenSymbol:  inputSymbol,                  // 输入代币符号
		OutputTokenSymbol: outputSymbol,                 // 输出代币符号
		InputTokenIcon:    inputIcon,                    // 输入代币图标
		OutputTokenIcon:   outputIcon,                   // 输出代币图标
		TradeFeeRate:      pool.GetTradeFeeRate(),       // 交易费率
		LaunchTime:        pool.GetCreatedAtUnix(),      // 创建时间
		LiquidityUsd:      pool.GetLiquidityUsd(),       // 流动性（USD）
		Txs_24H:           pool.GetTxs24h(),             // 24小时交易次数
		Vol_24H:           pool.GetVol24h(),             // 24小时交易量
		Apr:               pool.GetApr(),                // 年化收益率
		PoolVersion:       poolVersion,                  // 池版本
	}
}

// getTokenInfo 从代币对象中获取符号和图标
// 如果代币对象为nil，返回默认值
// 参数:
//   token - 代币对象
// 返回:
//   symbol - 代币符号
//   icon - 代币图标
func (l *ClmmPoolListLogic) getTokenInfo(token *solmodel.Token) (symbol, icon string) {
	if token != nil {
		// 代币对象存在，返回其符号和图标
		return token.Symbol, token.Icon
	}
	// 代币对象不存在，返回默认值
	return "Unknown", ""
}
