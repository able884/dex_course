package slot

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	// RedisKeyLatestSlot Redis 中存储最新 slot 的 key
	RedisKeyLatestSlot = "dex:slot:latest"
)

// LatestSlotTracker 最新 slot 追踪器
// 职责：
// 1. 内存存储最新 slot（原子操作，高性能）
// 2. 定时同步到 Redis（容灾，重启恢复）
type LatestSlotTracker struct {
	latestSlot atomic.Uint64 // 内存中的最新 slot（原子变量，并发安全）
	redis      *redis.Redis
	logger     logx.Logger
	ctx        context.Context
	cancel     context.CancelFunc

	syncInterval time.Duration // 同步到 Redis 的间隔
	ticker       *time.Ticker
}

// NewLatestSlotTracker 创建最新 slot 追踪器
func NewLatestSlotTracker(ctx context.Context, redisClient *redis.Redis, syncInterval time.Duration) *LatestSlotTracker {
	trackerCtx, cancel := context.WithCancel(ctx)

	tracker := &LatestSlotTracker{
		redis:        redisClient,
		logger:       logx.WithContext(ctx).WithFields(logx.Field("component", "latest_slot_tracker")),
		ctx:          trackerCtx,
		cancel:       cancel,
		syncInterval: syncInterval,
	}

	// 从 Redis 加载最新 slot（重启恢复）
	tracker.loadFromRedis()

	return tracker
}

// Update 更新最新 slot（内存操作，极快）
func (t *LatestSlotTracker) Update(slot uint64) {
	// 原子操作：只更新更大的 slot
	for {
		current := t.latestSlot.Load()
		if slot <= current {
			return // slot 不是更新的，跳过
		}
		if t.latestSlot.CompareAndSwap(current, slot) {
			return // 更新成功
		}
		// CAS 失败，重试
	}
}

// GetLatest 获取内存中的最新 slot
func (t *LatestSlotTracker) GetLatest() uint64 {
	return t.latestSlot.Load()
}

// GetRedisLatest 获取 Redis 中的最新 slot
func (t *LatestSlotTracker) GetRedisLatest() (uint64, error) {
	val, err := t.redis.Get(RedisKeyLatestSlot)
	if err != nil {
		if err == redis.Nil {
			return 0, nil // Redis 中没有记录
		}
		return 0, fmt.Errorf("get latest slot from redis failed: %w", err)
	}

	slot, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse latest slot failed: %s, err: %w", val, err)
	}

	return slot, nil
}

// SyncToRedis 手动同步到 Redis
func (t *LatestSlotTracker) SyncToRedis() error {
	slot := t.latestSlot.Load()
	if slot == 0 {
		return nil // 还没有接收到任何 slot
	}

	err := t.redis.Set(RedisKeyLatestSlot, fmt.Sprintf("%d", slot))
	if err != nil {
		return fmt.Errorf("sync latest slot to redis failed: %w", err)
	}

	t.logger.Debugf("Synced latest slot to Redis: %d", slot)
	return nil
}

// Start 启动定时同步协程
func (t *LatestSlotTracker) Start() {
	t.ticker = time.NewTicker(t.syncInterval)

	go t.syncLoop()

	t.logger.Infof("LatestSlotTracker started (sync interval: %v)", t.syncInterval)
}

// Stop 停止追踪器（优雅停机时调用）
func (t *LatestSlotTracker) Stop() {
	t.logger.Info("Stopping LatestSlotTracker...")

	// 停止定时器
	if t.ticker != nil {
		t.ticker.Stop()
	}

	// 取消 context
	t.cancel()

	// 强制同步到 Redis（保证数据不丢失）
	if err := t.SyncToRedis(); err != nil {
		t.logger.Errorf("Failed to sync latest slot on stop: %v", err)
	} else {
		t.logger.Infof("Final sync completed: slot=%d", t.latestSlot.Load())
	}

	t.logger.Info("LatestSlotTracker stopped")
}

// syncLoop 定时同步循环
func (t *LatestSlotTracker) syncLoop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.ticker.C:
			if err := t.SyncToRedis(); err != nil {
				t.logger.Errorf("Auto sync failed: %v", err)
			}
		}
	}
}

// loadFromRedis 从 Redis 加载最新 slot（启动时调用）
func (t *LatestSlotTracker) loadFromRedis() {
	slot, err := t.GetRedisLatest()
	if err != nil {
		t.logger.Errorf("Failed to load latest slot from Redis: %v", err)
		return
	}

	if slot > 0 {
		t.latestSlot.Store(slot)
		t.logger.Infof("Loaded latest slot from Redis: %d", slot)
	} else {
		t.logger.Info("No latest slot found in Redis (first startup)")
	}
}
