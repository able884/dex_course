package ticker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/market/internal/constants"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	rds "richcode.cc/dex/market/pkg/redis"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/chain"
	pkgConstants "richcode.cc/dex/pkg/constants"
)

// PairTypeConfig 交易对类型配置
//
// 使用示例：
//
//	config := PairTypeConfig{
//	    Name:         "PumpFun",           // DEX 名称，必须与数据库 pair 表的 name 字段一致
//	    UpdatePeriod: 3 * time.Second,    // 更新频率
//	    LockKey:      "lock:PairTicker:PumpFun",  // Redis 分布式锁 key
//	    LockExpire:   5,                  // 锁过期时间
//	    LockTimeout:  1,                  // 获取锁超时时间
//	}
type PairTypeConfig struct {
	Name         string        // 交易对类型名称，如 "PumpFun", "RaydiumV4" 等
	UpdatePeriod time.Duration // 更新周期
	LockKey      string        // 分布式锁的 key
	LockExpire   int           // 锁过期时间（秒）
	LockTimeout  int           // 获取锁的超时时间（秒）
}

// PumpTicker 交易对缓存更新器
//
// 功能说明：
//   - 支持多种交易对类型（PumpFun、PumpSwap、RaydiumV4、RaydiumClmm、RaydiumCpmm 等）
//   - 每种类型独立配置更新频率和锁策略
//   - 自动缓存不同状态的代币数据（新创建、完成中、已完成）
//   - 使用分布式锁保证集群环境下的数据一致性
//
// 缓存 Key 格式：
//
//	pair-token-list:{pairType}:{status}
//	例如：pair-token-list:PumpFun:1 (PumpFun 的新创建状态)
//	     pair-token-list:PumpSwap:1 (PumpSwap 的新创建状态)
//	     pair-token-list:RaydiumV4:2 (RaydiumV4 的完成中状态)
type PumpTicker struct {
	sc              *svc.ServiceContext
	logger          logx.Logger
	ctx             context.Context
	pairTypeConfigs []PairTypeConfig // 支持的交易对类型配置列表
}

func (t *PumpTicker) Stop() {

}

// NewPumpTicker 创建交易对缓存更新器
//
// 默认配置：
//   - PumpFun:     3秒更新一次（高频）
//   - PumpSwap:    3秒更新一次（高频）
//   - RaydiumV4:   1分钟更新一次
//   - RaydiumClmm: 1分钟更新一次
//   - RaydiumCpmm: 1分钟更新一次
//
// 如需添加新的交易对类型，在 pairTypeConfigs 中添加配置即可
func NewPumpTicker(sc *svc.ServiceContext) *PumpTicker {
	ctx := context.Background()
	logger := logx.WithContext(ctx).WithFields(logx.Field("service", "pump-ticker"))

	// 配置各种交易对类型的更新策略
	pairTypeConfigs := []PairTypeConfig{
		{
			Name:         pkgConstants.PumpFun,
			UpdatePeriod: 3 * time.Second, // PumpFun 更新频率高，3秒
			LockKey:      "lock:PairTicker:PumpFun",
			LockExpire:   5,
			LockTimeout:  1,
		},
		{
			Name:         pkgConstants.PumpSwap,
			UpdatePeriod: 3 * time.Second, // PumpSwap 更新频率高，3秒
			LockKey:      "lock:PairTicker:PumpSwap",
			LockExpire:   5,
			LockTimeout:  1,
		},
		{
			Name:         pkgConstants.RaydiumV4,
			UpdatePeriod: 1 * time.Minute, // Raydium 更新频率较低，1分钟
			LockKey:      "lock:PairTicker:RaydiumV4",
			LockExpire:   5,
			LockTimeout:  5,
		},
		{
			Name:         pkgConstants.RaydiumConcentratedLiquidity,
			UpdatePeriod: 1 * time.Minute,
			LockKey:      "lock:PairTicker:RaydiumClmm",
			LockExpire:   5,
			LockTimeout:  5,
		},
		{
			Name:         pkgConstants.RaydiumCPMM,
			UpdatePeriod: 1 * time.Minute,
			LockKey:      "lock:PairTicker:RaydiumCpmm",
			LockExpire:   5,
			LockTimeout:  5,
		},
	}

	return &PumpTicker{
		sc:              sc,
		logger:          logger,
		ctx:             ctx,
		pairTypeConfigs: pairTypeConfigs,
	}
}

// Start 启动所有配置的交易对类型的缓存更新任务
func (t *PumpTicker) Start() {
	// 为每种交易对类型启动独立的更新任务
	for _, config := range t.pairTypeConfigs {
		config := config // 捕获循环变量

		// 针对 PumpFun 和 PumpSwap 类型，启动新创建代币的快速更新
		if config.Name == pkgConstants.PumpFun || config.Name == pkgConstants.PumpSwap {
			threading.GoSafe(func() {
				t.logger.Infof("starting new creation cache update task for %s", config.Name)
				t.startPairTypeTicker(config, constants.PumpStatusNewCreation, 60*60*24*7)
			})
		}

		// 为所有类型启动正在完成和已完成状态的更新
		threading.GoSafe(func() {
			t.logger.Infof("starting completing cache update task for %s", config.Name)
			t.startPairTypeTicker(config, constants.PumpStatusCompleting, 60)
		})

		threading.GoSafe(func() {
			t.logger.Infof("starting completed cache update task for %s", config.Name)
			t.startPairTypeTicker(config, constants.PumpStatusCompleted, 60)
		})
	}
}

// startPairTypeTicker 为指定的交易对类型和状态启动定时更新任务
func (t *PumpTicker) startPairTypeTicker(config PairTypeConfig, status int, expireSeconds int) {
	ticker := time.NewTicker(config.UpdatePeriod)
	defer ticker.Stop()

	// 获取分布式锁，确保同一时刻只有一个实例在更新
	_, err := rds.MustLock(context.Background(), t.sc.RDS, config.LockKey, config.LockExpire, config.LockTimeout)
	if err != nil {
		return
	}

	// 立即执行一次更新
	threading.RunSafe(func() {
		t.updatePairTypeCache(config.Name, status, expireSeconds)
	})

	// 定时更新
	for range ticker.C {
		threading.RunSafe(func() {
			t.updatePairTypeCache(config.Name, status, expireSeconds)
		})
	}
}

// updatePairTypeCache 更新指定交易对类型和状态的缓存
func (t *PumpTicker) updatePairTypeCache(pairType string, status int, expireSeconds int) {
	pairModel := solmodel.NewPairModel(t.sc.DB)
	t.updateCacheForStatus(pairModel, status, pairType, expireSeconds)
}

// updateCacheForStatus 更新特定状态和类型的代币缓存
func (t *PumpTicker) updateCacheForStatus(pairModel solmodel.PairModel, status int, pairType string, expireSeconds int) {
	// 根据状态和类型获取交易对列表
	pairList, err := t.fetchPairsByStatus(pairModel, status, pairType)
	if err != nil {
		t.logger.Errorf("failed to fetch pairs [%s][status:%d]: %v", pairType, status, err)
		return
	}

	if len(pairList) == 0 {
		return
	}

	// 构建代币项
	items, err := t.buildPumpTokenItems(pairList)
	if err != nil {
		t.logger.Errorf("failed to build token items [%s][status:%d]: %v", pairType, status, err)
		return
	}

	// 缓存数据
	if err := t.cachePairTokenList(items, pairType, status, expireSeconds); err != nil {
		t.logger.Errorf("failed to cache data [%s][status:%d]: %v", pairType, status, err)
		return
	}
}

// fetchPairsByStatus 根据 pump 状态获取交易对
func (t *PumpTicker) fetchPairsByStatus(pairModel solmodel.PairModel, status int, pumpType string) ([]solmodel.Pair, error) {
	const (
		pageNo   = 1
		pageSize = 10
	)

	switch status {
	case constants.PumpStatusNewCreation:
		return pairModel.FindLatestPumpLimit(t.ctx, pumpType, pageNo, pageSize)
	case constants.PumpStatusCompleting:
		return pairModel.FindLatestCompletingPumpLimit(t.ctx, pumpType, pageNo, pageSize)
	case constants.PumpStatusCompleted:
		return pairModel.FindLatestCompletePumpLimit(t.ctx, pumpType, pageNo, pageSize)
	default:
		return nil, fmt.Errorf("unsupported pump status: %d", status)
	}
}

// buildPumpTokenItems 从交易对列表构建 pump 代币项
func (t *PumpTicker) buildPumpTokenItems(pairList []solmodel.Pair) ([]*market.PumpTokenItem, error) {
	// 提取代币地址
	tokenAddresses := make([]string, 0, len(pairList))
	for _, pair := range pairList {
		if pair.TokenAddress != "" {
			tokenAddresses = append(tokenAddresses, pair.TokenAddress)
		}
	}

	// 从数据库获取代币信息
	tokenModel := solmodel.NewTokenModel(t.sc.DB)
	tokenList, err := tokenModel.FindAllByAddresses(t.ctx, int64(constants.Sol), tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to find tokens: %w", err)
	}

	// 构建代币映射表以便快速查找
	tokenMap := make(map[string]*solmodel.Token, len(tokenList))
	for i := range tokenList {
		tokenMap[tokenList[i].Address] = &tokenList[i]
	}

	// 构建 pump 代币项列表
	items := make([]*market.PumpTokenItem, 0, len(pairList))
	for _, pair := range pairList {
		token := tokenMap[pair.TokenAddress]
		var tokenIcon, twitterUsername, telegram string
		if token != nil {
			tokenIcon = token.Icon
			twitterUsername = token.TwitterUsername
			telegram = token.Telegram
		}

		items = append(items, &market.PumpTokenItem{
			ChainId:          pair.ChainId,
			ChainIcon:        chain.ChainId2ChainIcon(100000),
			TokenAddress:     pair.TokenAddress,
			TokenIcon:        tokenIcon,
			TokenName:        pair.TokenSymbol,
			LaunchTime:       pair.BlockTime.Unix(),
			MktCap:           pair.Fdv,
			DomesticProgress: pair.PumpPoint,
			TwitterUsername:  twitterUsername,
			Telegram:         telegram,
			PairAddress:      pair.Address,
		})
	}

	return items, nil
}

// cachePairTokenList 将交易对代币列表缓存到 Redis
// 缓存 key 格式: pair-token-list:{pairType}:{status}
func (t *PumpTicker) cachePairTokenList(items []*market.PumpTokenItem, pairType string, status int, expireSeconds int) error {
	listData, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// 使用交易对类型和状态组合生成缓存 key
	cacheKey := fmt.Sprintf("pair-token-list:%s:%d", pairType, status)
	if err := t.sc.RDS.Set(cacheKey, string(listData)); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	if err := t.sc.RDS.Expire(cacheKey, expireSeconds); err != nil {
		return fmt.Errorf("failed to set cache expiration: %w", err)
	}

	return nil
}
