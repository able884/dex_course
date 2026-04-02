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

type GetPumpTokenListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPumpTokenListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPumpTokenListLogic {
	return &GetPumpTokenListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// Get pump token list data
func (l *GetPumpTokenListLogic) GetPumpTokenList(in *market.GetPumpTokenListRequest) (*market.GetPumpTokenListResponse, error) {
	// 初始化分页参数
	if in.PageNo <= 0 {
		in.PageNo = 1
	}
	if in.PageSize <= 0 {
		in.PageSize = 10
	}
	// 默认使用 pumpamm
	if in.PumpType == "" {
		in.PumpType = "pumpamm"
	}

	pairCacheKey := fmt.Sprintf("pump-token-list-%s-%d", in.PumpType, in.PumpStatus)
	l.Infof("Fetching pump token list with cache key: %s", pairCacheKey)

	// 尝试从缓存获取数据
	if resultList, ok := l.getFromCache(pairCacheKey); ok {
		return &market.GetPumpTokenListResponse{
			List:  resultList,
			Total: int32(len(resultList)),
		}, nil
	}

	// 缓存未命中，从数据库查询
	pairList, err := l.fetchPairList(in)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pair list: %w", err)
	}

	if len(pairList) == 0 {
		return &market.GetPumpTokenListResponse{
			List:  []*market.PumpTokenItem{},
			Total: 0,
		}, nil
	}

	// 构建代币地址列表
	tokenAddresses := l.extractTokenAddresses(pairList, in.PumpType)
	if len(tokenAddresses) == 0 {
		return &market.GetPumpTokenListResponse{
			List:  []*market.PumpTokenItem{},
			Total: 0,
		}, nil
	}

	// 获取代币信息
	tokenMap, err := l.fetchTokenMap(in.ChainId, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token map: %w", err)
	}

	// 获取持有者数量
	tokenHolderMap := l.fetchTokenHolders(in.ChainId, tokenAddresses, tokenMap)

	// 构建响应列表
	list := l.buildPumpTokenList(pairList, tokenMap, tokenHolderMap, in.ChainId, in.PumpType)

	// 缓存结果
	if err := l.cacheResult(pairCacheKey, list); err != nil {
		l.Errorf("Failed to cache result: %v", err)
	}

	return &market.GetPumpTokenListResponse{
		List:  list,
		Total: int32(len(list)),
	}, nil
}

// getFromCache 从缓存获取数据
func (l *GetPumpTokenListLogic) getFromCache(cacheKey string) ([]*market.PumpTokenItem, bool) {
	cachedData, err := l.svcCtx.RDS.Get(cacheKey)
	if err != nil || cachedData == "" {
		return nil, false
	}

	var resultList []*market.PumpTokenItem
	if err := json.Unmarshal([]byte(cachedData), &resultList); err != nil {
		l.Errorf("Failed to unmarshal cached data: %v", err)
		return nil, false
	}

	l.Infof("Cache hit, returned %d items", len(resultList))
	return resultList, true
}

// fetchPairList 根据状态查询 pair 列表
// 根据不同状态调用对应的查询方法，每个方法有各自的排序特点
func (l *GetPumpTokenListLogic) fetchPairList(in *market.GetPumpTokenListRequest) ([]solmodel.Pair, error) {
	pairModel := solmodel.NewPairModel(l.svcCtx.DB)

	// 根据 pump_type 确定使用的常量
	var pumpConstant string
	if in.PumpType == "pumpamm" {
		pumpConstant = pkgConstants.PumpSwap
	} else {
		pumpConstant = pkgConstants.PumpFun
	}

	switch in.PumpStatus {
	case constants.PumpStatusNewCreation:
		// 查询最新创建的代币，按区块号降序
		return pairModel.FindLatestPumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	case constants.PumpStatusCompleting:
		// 查询完成中的代币，按完成进度降序
		return pairModel.FindLatestCompletingPumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	case constants.PumpStatusCompleted:
		// 查询已完成的代币，按区块号降序
		return pairModel.FindLatestCompletePumpLimit(l.ctx, pumpConstant, in.PageNo, in.PageSize)
	default:
		return nil, fmt.Errorf("invalid pump status: %d", in.PumpStatus)
	}
}

// extractTokenAddresses 提取代币地址列表
// pumpfun 使用 TokenAddress, pumpamm 使用 BaseTokenAddress
func (l *GetPumpTokenListLogic) extractTokenAddresses(pairList []solmodel.Pair, pumpType string) []string {
	addresses := make([]string, 0, len(pairList))
	for _, pair := range pairList {
		tokenAddr := l.getTokenAddressByPumpType(&pair, pumpType)
		if tokenAddr != "" {
			addresses = append(addresses, tokenAddr)
		}
	}
	return addresses
}

// getTokenAddressByPumpType 根据 pump_type 获取对应的代币地址
// pumpfun: 使用 TokenAddress
// pumpamm: 使用 BaseTokenAddress
func (l *GetPumpTokenListLogic) getTokenAddressByPumpType(pair *solmodel.Pair, pumpType string) string {
	if pumpType == "pumpamm" {
		return pair.BaseTokenAddress
	}
	return pair.TokenAddress
}

// fetchTokenMap 批量获取代币信息并构建映射
func (l *GetPumpTokenListLogic) fetchTokenMap(chainId int64, addresses []string) (map[string]*solmodel.Token, error) {
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

// fetchTokenHolders 批量获取代币持有者数量
func (l *GetPumpTokenListLogic) fetchTokenHolders(chainId int64, addresses []string, tokenMap map[string]*solmodel.Token) map[string]int64 {
	holderMap := make(map[string]int64, len(addresses))
	solTokenAccountModel := solmodel.NewSolTokenAccountModel(l.svcCtx.DB)

	for _, address := range addresses {
		token := tokenMap[address]
		if token == nil {
			holderMap[address] = 0
			continue
		}

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

// buildPumpTokenList 构建返回的代币列表
func (l *GetPumpTokenListLogic) buildPumpTokenList(
	pairList []solmodel.Pair,
	tokenMap map[string]*solmodel.Token,
	holderMap map[string]int64,
	chainId int64,
	pumpType string,
) []*market.PumpTokenItem {
	list := make([]*market.PumpTokenItem, 0, len(pairList))
	chainIcon := chain.ChainId2ChainIcon(chainId)

	for _, pair := range pairList {
		// 根据 pump_type 获取正确的代币地址
		tokenAddr := l.getTokenAddressByPumpType(&pair, pumpType)
		token := tokenMap[tokenAddr]

		// 根据 pump_type 选择正确的 TokenSymbol
		// pumpfun 使用 TokenSymbol, pumpamm 使用 BaseTokenSymbol
		tokenSymbol := pair.TokenSymbol
		if pumpType == "pumpamm" {
			tokenSymbol = pair.BaseTokenSymbol
		}

		item := &market.PumpTokenItem{
			ChainId:          pair.ChainId,
			ChainIcon:        chainIcon,
			TokenAddress:     tokenAddr,
			TokenName:        tokenSymbol,
			LaunchTime:       pair.BlockTime.Unix(),
			MktCap:           pair.Fdv,
			HoldCount:        holderMap[tokenAddr],
			DomesticProgress: pair.PumpPoint,
		}

		if token != nil {
			item.TokenIcon = token.Icon
			item.TwitterUsername = token.TwitterUsername
			item.Telegram = token.Telegram
		}

		list = append(list, item)
	}

	return list
}

// cacheResult 缓存查询结果
func (l *GetPumpTokenListLogic) cacheResult(cacheKey string, list []*market.PumpTokenItem) error {
	listData, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	if err := l.svcCtx.RDS.Set(cacheKey, string(listData)); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	if err := l.svcCtx.RDS.Expire(cacheKey, 5); err != nil {
		return fmt.Errorf("failed to set expiration: %w", err)
	}

	l.Infof("Successfully cached %d items", len(list))
	return nil
}
