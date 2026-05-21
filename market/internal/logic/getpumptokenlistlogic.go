package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"richcode.cc/dex/market/internal/constants"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/chain"
	pkgConstants "richcode.cc/dex/pkg/constants"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetPumpTokenListLogic 获取Pump代币列表的业务逻辑处理器
type GetPumpTokenListLogic struct {
	ctx    context.Context              // 上下文对象，用于请求链路追踪和超时控制
	svcCtx *svc.ServiceContext          // 服务上下文，包含数据库、Redis等依赖
	logx.Logger                          // 日志记录器
}

// NewGetPumpTokenListLogic 创建获取Pump代币列表的逻辑处理器
// 参数:
//   ctx - 上下文对象
//   svcCtx - 服务上下文
// 返回: 初始化后的GetPumpTokenListLogic指针
func NewGetPumpTokenListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPumpTokenListLogic {
	return &GetPumpTokenListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPumpTokenList 获取Pump代币列表数据的主方法
// 该方法实现了多级缓存策略，先尝试从Redis获取，缓存未命中则查询数据库
// 参数:
//   in - 请求参数，包含链ID、Pump状态、排序方式、分页信息等
// 返回:
//   *market.GetPumpTokenListResponse - 代币列表响应
//   error - 错误信息
func (l *GetPumpTokenListLogic) GetPumpTokenList(in *market.GetPumpTokenListRequest) (*market.GetPumpTokenListResponse, error) {
	// 初始化分页参数：如果页码小于等于0，默认为第1页
	if in.PageNo <= 0 {
		in.PageNo = 1
	}
	// 初始化分页参数：如果每页大小小于等于0，默认为10条
	if in.PageSize <= 0 {
		in.PageSize = 10
	}
	// 默认使用 pumpamm 类型，如果请求中未指定
	if in.PumpType == "" {
		in.PumpType = "pumpamm"
	}

	// 构建缓存键，格式：pump-token-list-{pumpType}-{pumpStatus}
	pairCacheKey := fmt.Sprintf("pump-token-list-%s-%d", in.PumpType, in.PumpStatus)
	l.Infof("Fetching pump token list with cache key: %s", pairCacheKey)

	// 第一步：尝试从Redis缓存获取数据
	if resultList, ok := l.getFromCache(pairCacheKey); ok {
		l.Infof("pump-token-list从缓存获取到数据： %d items", len(resultList))
		// 缓存命中，直接返回缓存数据
		return &market.GetPumpTokenListResponse{
			List:  resultList,
			Total: int32(len(resultList)),
		}, nil
	}

	// 第二步：缓存未命中，从数据库查询交易对列表
	pairList, err := l.fetchPairList(in)
	if err != nil {
		l.Errorf("失败获取 pair list: %v", err)
		return nil, fmt.Errorf("failed to fetch pair list: %w", err)
	}
	l.Infof("成功获取 %d 个 pair", len(pairList))

	// 如果没有查询到任何交易对，返回空列表
	if len(pairList) == 0 {
		return &market.GetPumpTokenListResponse{
			List:  []*market.PumpTokenItem{},
			Total: 0,
		}, nil
	}

	// 第三步：从交易对列表中提取所有代币地址（去重）
	tokenAddresses := l.extractTokenAddresses(pairList, in.PumpType)
	if len(tokenAddresses) == 0 {
		return &market.GetPumpTokenListResponse{
			List:  []*market.PumpTokenItem{},
			Total: 0,
		}, nil
	}

	// 第四步：批量查询代币详细信息
	tokenMap, err := l.fetchTokenMap(in.ChainId, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token map: %w", err)
	}

	// 第五步：查询每个代币的持有者数量
	tokenHolderMap := l.fetchTokenHolders(in.ChainId, tokenAddresses, tokenMap)

	// 第六步：组装最终的响应数据
	list := l.buildPumpTokenList(pairList, tokenMap, tokenHolderMap, in.ChainId, in.PumpType)

	// 第七步：将结果缓存到Redis，过期时间5秒
	if err := l.cacheResult(pairCacheKey, list); err != nil {
		l.Errorf("Failed to cache result: %v", err)
	}

	// 返回最终响应
	return &market.GetPumpTokenListResponse{
		List:  list,
		Total: int32(len(list)),
	}, nil
}

// getFromCache 从Redis缓存中获取代币列表数据
// 参数:
//   cacheKey - Redis缓存键
// 返回:
//   []*market.PumpTokenItem - 缓存的代币列表
//   bool - 是否成功从缓存获取数据
func (l *GetPumpTokenListLogic) getFromCache(cacheKey string) ([]*market.PumpTokenItem, bool) {
	// 从Redis获取缓存数据
	cachedData, err := l.svcCtx.RDS.Get(cacheKey)
	if err != nil || cachedData == "" {
		// 获取失败或缓存为空，返回false
		return nil, false
	}

	// 反序列化JSON数据
	var resultList []*market.PumpTokenItem
	if err := json.Unmarshal([]byte(cachedData), &resultList); err != nil {
		l.Errorf("Failed to unmarshal cached data: %v", err)
		return nil, false
	}

	// 缓存命中，记录日志并返回数据
	l.Infof("Cache hit, returned %d items", len(resultList))
	return resultList, true
}

// fetchPairList 根据Pump状态查询交易对列表
// 根据不同的Pump状态调用对应的数据库查询方法，每个方法有各自的排序特点
// 参数:
//   in - 请求参数，包含Pump状态、类型、分页信息
// 返回:
//   []solmodel.Pair - 交易对列表
//   error - 错误信息
func (l *GetPumpTokenListLogic) fetchPairList(in *market.GetPumpTokenListRequest) ([]solmodel.Pair, error) {
	// 创建交易对数据模型
	pairModel := solmodel.NewPairModel(l.svcCtx.DB)

	// 根据 pump_type 确定使用的常量（PumpSwap 或 PumpFun）
	var pumpConstant string
	if in.PumpType == "pumpamm" {
		pumpConstant = pkgConstants.PumpSwap
	} else {
		pumpConstant = pkgConstants.PumpFun
	}

	// 根据Pump状态选择不同的查询策略
	switch in.PumpStatus {
	case constants.PumpStatusNewCreation:
		// 查询最新创建的代币，按区块号降序排列
		return pairModel.FindLatestPumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	case constants.PumpStatusCompleting:
		// 查询完成中的代币，按完成进度降序排列
		return pairModel.FindLatestCompletingPumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	case constants.PumpStatusCompleted:
		// 查询已完成的代币，按区块号降序排列
		return pairModel.FindLatestCompletePumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	default:
		// 无效的Pump状态，返回错误
		return nil, fmt.Errorf("invalid pump status: %d", in.PumpStatus)
	}
}

// extractTokenAddresses 从交易对列表中提取代币地址（去重）
// 注意：pumpfun 使用 TokenAddress, pumpamm 使用 BaseTokenAddress
// 参数:
//   pairList - 交易对列表
//   pumpType - Pump类型（pumpfun 或 pumpamm）
// 返回:
//   []string - 去重后的代币地址列表
func (l *GetPumpTokenListLogic) extractTokenAddresses(pairList []solmodel.Pair, pumpType string) []string {
	// 使用map来去重，key是地址，value是空结构体（不占内存）
	uniqueAddrs := make(map[string]struct{})
	for _, pair := range pairList {
		// 根据Pump类型获取对应的代币地址
		tokenAddr := l.getTokenAddressByPumpType(&pair, pumpType)
		if tokenAddr != "" {
			// 将地址存入map，重复的会自动覆盖
			uniqueAddrs[tokenAddr] = struct{}{}
		}
	}
	// 将map的key转换为切片返回
	l.Infof("找到 %d unique token addresses", len(uniqueAddrs))
	addresses := make([]string, 0, len(uniqueAddrs))
	for addr := range uniqueAddrs {
		addresses = append(addresses, addr)
	}
	return addresses
}

// getTokenAddressByPumpType 根据Pump类型从交易对中获取对应的代币地址
// pumpfun: 使用 TokenAddress 字段
// pumpamm: 使用 BaseTokenAddress 字段
// 参数:
//   pair - 交易对对象
//   pumpType - Pump类型
// 返回:
//   string - 代币地址
func (l *GetPumpTokenListLogic) getTokenAddressByPumpType(pair *solmodel.Pair, pumpType string) string {
	if pumpType == "pumpamm" {
		// pumpamm类型使用基础代币地址
		return pair.BaseTokenAddress
	}
	// pumpfun类型使用代币地址
	return pair.TokenAddress
}

// fetchTokenMap 批量获取代币信息并构建地址到代币对象的映射
// 参数:
//   chainId - 链ID
//   addresses - 代币地址列表
// 返回:
//   map[string]*solmodel.Token - 代币地址到代币对象的映射
//   error - 错误信息
func (l *GetPumpTokenListLogic) fetchTokenMap(chainId int64, addresses []string) (map[string]*solmodel.Token, error) {
	// 创建代币数据模型
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	// 批量查询代币信息
	tokenList, err := tokenModel.FindAllByAddresses(l.ctx, chainId, addresses)
	if err != nil {
		return nil, err
	}
	l.Infof("成功获取 %d 个 token", len(tokenList))

	// 构建地址到代币对象的映射
	tokenMap := make(map[string]*solmodel.Token, len(tokenList))
	for i := range tokenList {
		tokenMap[tokenList[i].Address] = &tokenList[i]
	}
	return tokenMap, nil
}

// fetchTokenHolders 批量获取每个代币的持有者数量
// 参数:
//   chainId - 链ID
//   addresses - 代币地址列表
//   tokenMap - 代币信息映射
// 返回:
//   map[string]int64 - 代币地址到持有者数量的映射
func (l *GetPumpTokenListLogic) fetchTokenHolders(chainId int64, addresses []string, tokenMap map[string]*solmodel.Token) map[string]int64 {
	// 初始化持有者数量映射
	holderMap := make(map[string]int64, len(addresses))
	// 创建Solana代币账户数据模型
	solTokenAccountModel := solmodel.NewSolTokenAccountModel(l.svcCtx.DB)

	// 遍历每个代币地址，查询其持有者数量
	for _, address := range addresses {
		// 获取代币信息
		token := tokenMap[address]
		if token == nil {
			// 代币信息不存在，持有者数量设为0
			holderMap[address] = 0
			continue
		}

		// 查询代币持有者数量（只统计代币创建之后的账户）
		holders, err := solTokenAccountModel.CountByTokenAddressWithTime(l.ctx, chainId, address, token.CreatedAt)
		if err != nil {
			l.Errorf("Failed to count holders for token %s: %v", address, err)
			holderMap[address] = 0
			continue
		}
		holderMap[address] = holders
	}

	return holderMap
}

// buildPumpTokenList 构建返回给前端的代币列表
// 参数:
//   pairList - 交易对列表
//   tokenMap - 代币信息映射
//   holderMap - 持有者数量映射
//   chainId - 链ID
//   pumpType - Pump类型
// 返回:
//   []*market.PumpTokenItem - 代币列表响应项
func (l *GetPumpTokenListLogic) buildPumpTokenList(
	pairList []solmodel.Pair,
	tokenMap map[string]*solmodel.Token,
	holderMap map[string]int64,
	chainId int64,
	pumpType string,
) []*market.PumpTokenItem {
	// 初始化响应列表
	list := make([]*market.PumpTokenItem, 0, len(pairList))
	// 获取链图标
	chainIcon := chain.ChainId2ChainIcon(chainId)

	// 遍历每个交易对，构建响应项
	for _, pair := range pairList {
		// 根据 pump_type 获取正确的代币地址
		tokenAddr := l.getTokenAddressByPumpType(&pair, pumpType)
		// 从代币映射中获取代币详细信息
		token := tokenMap[tokenAddr]

		// 根据 pump_type 选择正确的代币符号
		// pumpfun 使用 TokenSymbol, pumpamm 使用 BaseTokenSymbol
		tokenSymbol := pair.TokenSymbol
		if pumpType == "pumpamm" {
			tokenSymbol = pair.BaseTokenSymbol
		}

		// 构建代币列表项
		item := &market.PumpTokenItem{
			ChainId:          pair.ChainId,              // 链ID
			ChainIcon:        chainIcon,                 // 链图标
			TokenAddress:     tokenAddr,                 // 代币地址
			TokenName:        tokenSymbol,               // 代币名称/符号
			LaunchTime:       pair.BlockTime.Unix(),     // 发布时间（Unix时间戳）
			MktCap:           pair.Fdv,                  // 完全稀释市值
			HoldCount:        holderMap[tokenAddr],      // 持有者数量
			DomesticProgress: pair.PumpPoint,            // Pump进度
		}

		// 如果代币信息存在，补充代币图标和社交媒体链接
		if token != nil {
			item.TokenIcon = token.Icon
			item.TwitterUsername = token.TwitterUsername
			item.Telegram = token.Telegram
		}

		// 将该项添加到列表中
		list = append(list, item)
	}

	return list
}

// cacheResult 将查询结果缓存到Redis
// 参数:
//   cacheKey - 缓存键
//   list - 代币列表数据
// 返回:
//   error - 错误信息
func (l *GetPumpTokenListLogic) cacheResult(cacheKey string, list []*market.PumpTokenItem) error {
	// 将列表数据序列化为JSON
	listData, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	// 将JSON数据存入Redis
	if err := l.svcCtx.RDS.Set(cacheKey, string(listData)); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	// 设置缓存过期时间为5秒
	if err := l.svcCtx.RDS.Expire(cacheKey, 5); err != nil {
		return fmt.Errorf("failed to set expiration: %w", err)
	}

	l.Infof("Successfully cached %d items", len(list))
	return nil
}
