package block

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// PriceRange 表示价格范围
type PriceRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// PriceRangeCache 价格范围缓存管理器
type PriceRangeCache struct {
	mu      sync.RWMutex
	memory  map[string]*PoolPriceRanges // key: poolState
	redis   *redis.Redis
	chainID int64
}

// PoolPriceRanges 池子的价格范围
type PoolPriceRanges struct {
	PoolState string                 `json:"pool_state"`
	Ranges    map[string]*PriceRange `json:"ranges"` // key: "24H", "7D", "30D"
	UpdatedAt time.Time              `json:"updated_at"`
}

// NewPriceRangeCache 创建价格范围缓存管理器
func NewPriceRangeCache(redisClient *redis.Redis, chainID int64) *PriceRangeCache {
	return &PriceRangeCache{
		memory:  make(map[string]*PoolPriceRanges),
		redis:   redisClient,
		chainID: chainID,
	}
}

// LoadFromRedis 从 Redis 加载所有池子的价格范围到内存
func (c *PriceRangeCache) LoadFromRedis(ctx context.Context) error {
	if c.redis == nil {
		return fmt.Errorf("redis client not configured")
	}

	// Redis key 格式: clmm_price_range:{chain_id}:{pool_state}
	pattern := fmt.Sprintf("clmm_price_range:%d:*", c.chainID)
	keys, err := c.redis.Keys(pattern)
	if err != nil {
		return fmt.Errorf("failed to get keys from redis: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	loadedCount := 0
	corruptedCount := 0
	for _, key := range keys {
		val, err := c.redis.Get(key)
		if err != nil {
			logx.Errorf("PriceRangeCache: failed to get key %s from redis: %v", key, err)
			continue
		}

		// 检查数据是否为空
		if val == "" || len(val) == 0 {
			logx.Infof("PriceRangeCache: empty data for key %s, deleting corrupted key", key)
			_, _ = c.redis.Del(key)
			corruptedCount++
			continue
		}

		var ranges PoolPriceRanges
		if err := json.Unmarshal([]byte(val), &ranges); err != nil {
			logx.Errorf("PriceRangeCache: failed to unmarshal data for key %s, data length: %d, error: %v. Deleting corrupted key.", key, len(val), err)
			// 删除损坏的数据
			_, _ = c.redis.Del(key)
			corruptedCount++
			continue
		}

		// 验证数据完整性
		if ranges.Ranges == nil {
			logx.Infof("PriceRangeCache: invalid data structure for key %s (Ranges is nil), deleting corrupted key", key)
			_, _ = c.redis.Del(key)
			corruptedCount++
			continue
		}

		c.memory[ranges.PoolState] = &ranges
		loadedCount++
	}

	if corruptedCount > 0 {
		logx.Infof("PriceRangeCache: loaded %d pool price ranges from redis, deleted %d corrupted keys", loadedCount, corruptedCount)
	} else {
		logx.Infof("PriceRangeCache: loaded %d pool price ranges from redis", loadedCount)
	}
	return nil
}

// UpdatePrice 更新池子的价格范围
// 只有当新价格比当前范围更小或更大时才更新
// 同时清理过期的价格范围，确保时间窗口准确
func (c *PriceRangeCache) UpdatePrice(ctx context.Context, poolState string, price float64, blockTime time.Time) {
	if price <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 获取或创建池子的价格范围
	ranges, exists := c.memory[poolState]
	if !exists {
		ranges = &PoolPriceRanges{
			PoolState: poolState,
			Ranges:    make(map[string]*PriceRange),
			UpdatedAt: blockTime,
		}
		c.memory[poolState] = ranges
	}

	now := time.Now()
	updated := false

	// 先清理过期的范围，确保时间窗口准确
	c.cleanExpiredRangesForPool(ranges, now)

	// 更新 24H 范围（仅在时间窗口内）
	if blockTime.After(now.Add(-24 * time.Hour)) {
		if c.updateRange(ranges, "24H", price) {
			updated = true
		}
	}

	// 更新 7D 范围（仅在时间窗口内）
	if blockTime.After(now.Add(-7 * 24 * time.Hour)) {
		if c.updateRange(ranges, "7D", price) {
			updated = true
		}
	}

	// 更新 30D 范围（仅在时间窗口内）
	if blockTime.After(now.Add(-30 * 24 * time.Hour)) {
		if c.updateRange(ranges, "30D", price) {
			updated = true
		}
	}

	// 如果范围有更新，写入 Redis
	if updated {
		ranges.UpdatedAt = blockTime
		if err := c.saveToRedis(ctx, ranges); err != nil {
			logx.Errorf("PriceRangeCache: failed to save to redis for pool %s: %v", poolState, err)
		}
	}
}

// cleanExpiredRangesForPool 清理单个池子的过期范围
func (c *PriceRangeCache) cleanExpiredRangesForPool(ranges *PoolPriceRanges, now time.Time) {
	// 清理 24H 范围（如果最后更新时间超过 24H）
	if _, ok := ranges.Ranges["24H"]; ok {
		if ranges.UpdatedAt.Before(now.Add(-24 * time.Hour)) {
			delete(ranges.Ranges, "24H")
		}
	}

	// 清理 7D 范围（如果最后更新时间超过 7D）
	if _, ok := ranges.Ranges["7D"]; ok {
		if ranges.UpdatedAt.Before(now.Add(-7 * 24 * time.Hour)) {
			delete(ranges.Ranges, "7D")
		}
	}

	// 清理 30D 范围（如果最后更新时间超过 30D）
	if _, ok := ranges.Ranges["30D"]; ok {
		if ranges.UpdatedAt.Before(now.Add(-30 * 24 * time.Hour)) {
			delete(ranges.Ranges, "30D")
		}
	}
}

// updateRange 更新单个时间范围，返回是否实际更新了
func (c *PriceRangeCache) updateRange(ranges *PoolPriceRanges, timeRange string, price float64) bool {
	rangeData, exists := ranges.Ranges[timeRange]
	if !exists {
		// 首次创建
		ranges.Ranges[timeRange] = &PriceRange{
			Min: price,
			Max: price,
		}
		return true
	}

	// 检查是否需要更新
	updated := false
	if price < rangeData.Min {
		rangeData.Min = price
		updated = true
	}
	if price > rangeData.Max {
		rangeData.Max = price
		updated = true
	}

	return updated
}

// saveToRedis 保存到 Redis
func (c *PriceRangeCache) saveToRedis(ctx context.Context, ranges *PoolPriceRanges) error {
	if c.redis == nil {
		return fmt.Errorf("redis client not configured")
	}

	// 验证数据完整性
	if ranges == nil {
		return fmt.Errorf("ranges is nil")
	}
	if ranges.Ranges == nil {
		ranges.Ranges = make(map[string]*PriceRange)
	}

	key := c.getRedisKey(ranges.PoolState)
	data, err := json.Marshal(ranges)
	if err != nil {
		return fmt.Errorf("failed to marshal ranges: %w", err)
	}

	// 验证序列化后的数据不为空
	if len(data) == 0 {
		return fmt.Errorf("marshaled data is empty")
	}

	// 设置过期时间：30D + 1天缓冲
	expireSeconds := int(31 * 24 * 3600)
	if err := c.redis.Setex(key, string(data), expireSeconds); err != nil {
		return fmt.Errorf("failed to save to redis: %w", err)
	}

	// 验证写入的数据可以正确读取（可选，用于调试）
	// 注意：这可能会影响性能，可以在生产环境中移除
	if verifyVal, err := c.redis.Get(key); err == nil {
		if verifyVal != string(data) {
			logx.Infof("PriceRangeCache: data verification failed for key %s, data may be corrupted", key)
		}
	}

	return nil
}

// GetPriceRange 获取池子的价格范围
func (c *PriceRangeCache) GetPriceRange(poolState, timeRange string) (min, max float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ranges, exists := c.memory[poolState]
	if !exists {
		return 0, 0
	}

	rangeData, exists := ranges.Ranges[timeRange]
	if !exists {
		return 0, 0
	}

	return rangeData.Min, rangeData.Max
}

// GetPriceRanges 获取池子的所有价格范围
// 在获取时也会检查并清理过期数据，确保返回的数据在时间窗口内
func (c *PriceRangeCache) GetPriceRanges(poolState string) (priceRange24HMin, priceRange24HMax, priceRange7DMin, priceRange7DMax, priceRange30DMin, priceRange30DMax float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ranges, exists := c.memory[poolState]
	if !exists {
		return 0, 0, 0, 0, 0, 0
	}

	// 清理过期数据，确保时间窗口准确
	now := time.Now()
	c.cleanExpiredRangesForPool(ranges, now)

	if r24H, ok := ranges.Ranges["24H"]; ok {
		priceRange24HMin = r24H.Min
		priceRange24HMax = r24H.Max
	}
	if r7D, ok := ranges.Ranges["7D"]; ok {
		priceRange7DMin = r7D.Min
		priceRange7DMax = r7D.Max
	}
	if r30D, ok := ranges.Ranges["30D"]; ok {
		priceRange30DMin = r30D.Min
		priceRange30DMax = r30D.Max
	}

	return
}

// getRedisKey 获取 Redis key
func (c *PriceRangeCache) getRedisKey(poolState string) string {
	return fmt.Sprintf("clmm_price_range:%d:%s", c.chainID, poolState)
}

// CleanExpiredRanges 清理过期的价格范围（定期清理，移除超过时间窗口的数据）
// 注意：这个函数主要用于清理完全无用的池子记录，实际的时间窗口检查在 UpdatePrice 和 GetPriceRange 中进行
func (c *PriceRangeCache) CleanExpiredRanges(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cleanedCount := 0

	for poolState, ranges := range c.memory {
		// 清理过期的范围
		c.cleanExpiredRangesForPool(ranges, now)

		// 如果所有范围都被清理，删除整个池子记录
		if len(ranges.Ranges) == 0 {
			delete(c.memory, poolState)
			// 从 Redis 删除
			key := c.getRedisKey(poolState)
			if c.redis != nil {
				_, _ = c.redis.Del(key)
			}
			cleanedCount++
		}
	}

	if cleanedCount > 0 {
		logx.Infof("PriceRangeCache: cleaned %d expired pool price ranges", cleanedCount)
	}
}
