package slot

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	// RedisKeyFailed Redis 中存储失败 slot 的 key (Sorted Set)
	RedisKeyFailed = "dex:slot:failed"
)

// FailureRecorder 失败记录器
// 职责：
// 1. 记录所有失败的 slot（RPC 失败 + 消费者失败）
// 2. 提供查询失败队列接口
// 3. 支持删除已恢复的 slot
type FailureRecorder struct {
	redis  *redis.Redis
	logger logx.Logger
	ctx    context.Context
}

// NewFailureRecorder 创建失败记录器
func NewFailureRecorder(ctx context.Context, redisClient *redis.Redis) *FailureRecorder {
	return &FailureRecorder{
		redis:  redisClient,
		logger: logx.WithContext(ctx).WithFields(logx.Field("component", "failure_recorder")),
		ctx:    ctx,
	}
}

// RecordFailure 记录失败的 slot
// 使用 Sorted Set: score 和 member 都是 slot 值
// 好处：自动去重 + 按 slot 顺序排序
func (r *FailureRecorder) RecordFailure(slot uint64) error {
	// ZADD dex:slot:failed {slot} {slot}
	// score 和 member 都设置为 slot，保证按 slot 顺序恢复
	score := int64(slot)
	member := fmt.Sprintf("%d", slot)

	_, err := r.redis.ZaddCtx(r.ctx, RedisKeyFailed, score, member)
	if err != nil {
		r.logger.Errorf("Record failure failed: slot=%d, error=%v", slot, err)
		return fmt.Errorf("record failure failed: %w", err)
	}

	r.logger.Infof("Recorded failed slot: %d", slot)
	return nil
}

// GetFailedSlots 获取失败的 slot 列表（按 slot 升序）
// limit: 最多返回多少个 slot
func (r *FailureRecorder) GetFailedSlots(limit int) ([]uint64, error) {
	// ZRANGE dex:slot:failed 0 {limit-1}
	members, err := r.redis.ZrangeCtx(r.ctx, RedisKeyFailed, 0, int64(limit-1))
	if err != nil {
		r.logger.Errorf("Get failed slots error: %v", err)
		return nil, fmt.Errorf("get failed slots error: %w", err)
	}

	slots := make([]uint64, 0, len(members))
	for _, member := range members {
		slot, err := strconv.ParseUint(member, 10, 64)
		if err != nil {
			r.logger.Errorf("Parse slot failed: %s, error=%v", member, err)
			continue
		}
		slots = append(slots, slot)
	}

	return slots, nil
}

// RemoveFailure 从失败队列移除 slot（恢复成功后调用）
func (r *FailureRecorder) RemoveFailure(slot uint64) error {
	member := fmt.Sprintf("%d", slot)

	// ZREM dex:slot:failed {slot}
	_, err := r.redis.ZremCtx(r.ctx, RedisKeyFailed, member)
	if err != nil {
		r.logger.Errorf("Remove failure failed: slot=%d, error=%v", slot, err)
		return fmt.Errorf("remove failure failed: %w", err)
	}
	return nil
}

// GetFailedCount 获取失败队列长度
func (r *FailureRecorder) GetFailedCount() (int64, error) {
	// ZCARD dex:slot:failed
	count, err := r.redis.ZcardCtx(r.ctx, RedisKeyFailed)
	if err != nil {
		r.logger.Errorf("Get failed count error: %v", err)
		return 0, fmt.Errorf("get failed count error: %w", err)
	}

	return int64(count), nil
}

// BatchRecordFailures 批量记录失败的 slot
// 用于启动回补时批量添加缺失的 slot
func (r *FailureRecorder) BatchRecordFailures(slots []uint64) error {
	if len(slots) == 0 {
		return nil
	}

	// 批量添加（简化实现：循环调用 Zadd）
	for _, slot := range slots {
		score := int64(slot)
		member := fmt.Sprintf("%d", slot)
		_, err := r.redis.ZaddCtx(r.ctx, RedisKeyFailed, score, member)
		if err != nil {
			r.logger.Errorf("Batch record failure failed: slot=%d, error=%v", slot, err)
			// 继续处理其他 slot
		}
	}

	r.logger.Infof("Batch recorded %d failed slots", len(slots))
	return nil
}

// GetFailedSlotsInRange 获取指定范围内的失败 slot
// 用于调试和监控
func (r *FailureRecorder) GetFailedSlotsInRange(minSlot, maxSlot uint64) ([]uint64, error) {
	// ZRANGEBYSCORE dex:slot:failed {minSlot} {maxSlot}
	minScore := int64(minSlot)
	maxScore := int64(maxSlot)

	pairs, err := r.redis.ZrangebyscoreWithScoresCtx(r.ctx, RedisKeyFailed, minScore, maxScore)
	if err != nil {
		r.logger.Errorf("Get failed slots in range error: min=%d, max=%d, error=%v", minSlot, maxSlot, err)
		return nil, fmt.Errorf("get failed slots in range error: %w", err)
	}

	slots := make([]uint64, 0, len(pairs))
	for _, pair := range pairs {
		slot, err := strconv.ParseUint(pair.Key, 10, 64)
		if err != nil {
			r.logger.Errorf("Parse slot failed: %s, error=%v", pair.Key, err)
			continue
		}
		slots = append(slots, slot)
	}

	return slots, nil
}

// Clear 清空失败队列（仅用于测试或紧急情况）
func (r *FailureRecorder) Clear() error {
	_, err := r.redis.DelCtx(r.ctx, RedisKeyFailed)
	if err != nil {
		r.logger.Errorf("Clear failed slots error: %v", err)
		return fmt.Errorf("clear failed slots error: %w", err)
	}

	r.logger.Infof("Cleared all failed slots")
	return nil
}
