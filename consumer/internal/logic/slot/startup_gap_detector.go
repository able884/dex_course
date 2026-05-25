package slot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/zeromicro/go-zero/core/logx"
)

// StartupGapDetector 启动时 gap 检测器
// 职责：
// 1. 检测 Redis 最新 slot 和 WebSocket 首个 slot 的 gap
// 2. gap <= 1: 无需回补
// 3. 1 < gap <= MaxGap: 全部回补到失败队列
// 4. gap > MaxGap: 只回补最近 MaxGap 个，其余写日志
type StartupGapDetector struct {
	latestSlotTracker *LatestSlotTracker
	failureRecorder   *FailureRecorder
	rpcClient         *client.Client
	logger            logx.Logger
	maxGap            uint64
	logDir            string
}

// BackfillLog 回补日志结构
type BackfillLog struct {
	Timestamp          string `json:"timestamp"`
	RedisLatestSlot    uint64 `json:"redis_latest_slot"`
	WebSocketFirstSlot uint64 `json:"websocket_first_slot"`
	RPCCurrentSlot     uint64 `json:"rpc_current_slot"`
	GapSize            uint64 `json:"gap_size"`
	RecoveredRange     *Range `json:"recovered_range,omitempty"`
	UnrecoveredRange   *Range `json:"unrecovered_range,omitempty"`
	Message            string `json:"message"`
}

// Range slot 范围
type Range struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// NewStartupGapDetector 创建启动 gap 检测器
func NewStartupGapDetector(
	latestSlotTracker *LatestSlotTracker,
	failureRecorder *FailureRecorder,
	rpcClient *client.Client,
	maxGap uint64,
	logDir string,
) *StartupGapDetector {
	return &StartupGapDetector{
		latestSlotTracker: latestSlotTracker,
		failureRecorder:   failureRecorder,
		rpcClient:         rpcClient,
		logger:            logx.WithContext(context.Background()).WithFields(logx.Field("component", "startup_gap_detector")),
		maxGap:            maxGap,
		logDir:            logDir,
	}
}

// DetectAndHandle 检测并处理启动 gap
func (d *StartupGapDetector) DetectAndHandle(ctx context.Context) error {
	d.logger.Info("Starting startup gap detection...")

	// 1. 从 Redis 获取上次记录的最新 slot
	redisSlot, err := d.latestSlotTracker.GetRedisLatest()
	if err != nil {
		d.logger.Errorf("Failed to get Redis latest slot: %v", err)
		return fmt.Errorf("get redis latest slot failed: %w", err)
	}

	if redisSlot == 0 {
		d.logger.Info("No Redis slot found (first startup), skipping gap detection")
		return nil
	}

	d.logger.Infof("Redis latest slot: %d", redisSlot)

	// 2. 通过 RPC 获取当前最新 slot
	rpcSlot, err := d.getRPCCurrentSlot(ctx)
	if err != nil {
		d.logger.Errorf("Failed to get RPC current slot: %v", err)
		return fmt.Errorf("get rpc current slot failed: %w", err)
	}

	d.logger.Infof("RPC current slot: %d", rpcSlot)

	// 3. 等待 WebSocket 第一个 slot（最多 10 秒）
	wsSlot, err := d.waitForWebSocketSlot(ctx, 10*time.Second)
	if err != nil {
		d.logger.Errorf("Failed to get WebSocket first slot: %v", err)
		return fmt.Errorf("get websocket first slot failed: %w", err)
	}

	d.logger.Infof("WebSocket first slot: %d", wsSlot)

	// 4. 计算 gap
	if wsSlot <= redisSlot {
		d.logger.Info("No gap detected (WebSocket slot <= Redis slot)")
		return nil
	}

	gap := wsSlot - redisSlot - 1

	if gap == 0 {
		d.logger.Info("No gap detected (gap = 0)")
		return nil
	}

	d.logger.Infof("Gap detected: %d slots (Redis: %d -> WebSocket: %d)", gap, redisSlot, wsSlot)

	// 5. 根据 gap 大小选择策略
	if gap <= d.maxGap {
		// 全部回补
		return d.backfillAll(redisSlot+1, wsSlot-1, redisSlot, wsSlot, rpcSlot)
	} else {
		// 只回补最近 MaxGap 个 + 写日志
		return d.backfillPartial(redisSlot+1, wsSlot-1, redisSlot, wsSlot, rpcSlot)
	}
}

// backfillAll 全部回补
func (d *StartupGapDetector) backfillAll(startSlot, endSlot, redisSlot, wsSlot, rpcSlot uint64) error {
	d.logger.Infof("Backfilling all %d slots: [%d, %d]", endSlot-startSlot+1, startSlot, endSlot)

	// 批量添加到失败队列
	var slots []uint64
	for slot := startSlot; slot <= endSlot; slot++ {
		slots = append(slots, slot)
	}

	if err := d.failureRecorder.BatchRecordFailures(slots); err != nil {
		d.logger.Errorf("Batch record failures failed: %v", err)
		return fmt.Errorf("batch record failures failed: %w", err)
	}

	d.logger.Infof("Successfully recorded %d slots to failure queue", len(slots))

	// 写日志（记录成功回补）
	logData := BackfillLog{
		Timestamp:          time.Now().Format(time.RFC3339),
		RedisLatestSlot:    redisSlot,
		WebSocketFirstSlot: wsSlot,
		RPCCurrentSlot:     rpcSlot,
		GapSize:            wsSlot - redisSlot - 1,
		RecoveredRange: &Range{
			Start: startSlot,
			End:   endSlot,
		},
		Message: fmt.Sprintf("Full backfill completed: %d slots", len(slots)),
	}

	if err := d.writeBackfillLog(logData); err != nil {
		d.logger.Errorf("Write backfill log failed: %v", err)
		// 不返回错误，因为回补已成功
	}

	return nil
}

// backfillPartial 部分回补（只回补最近 MaxGap 个）
func (d *StartupGapDetector) backfillPartial(startSlot, endSlot, redisSlot, wsSlot, rpcSlot uint64) error {
	// 计算回补范围：从 wsSlot-MaxGap 到 wsSlot-1
	recoverStart := wsSlot - d.maxGap
	recoverEnd := wsSlot - 1

	d.logger.Infof("Gap too large (%d slots), backfilling only recent %d slots: [%d, %d]",
		endSlot-startSlot+1, d.maxGap, recoverStart, recoverEnd)

	// 批量添加到失败队列
	var slots []uint64
	for slot := recoverStart; slot <= recoverEnd; slot++ {
		slots = append(slots, slot)
	}

	if err := d.failureRecorder.BatchRecordFailures(slots); err != nil {
		d.logger.Errorf("Batch record failures failed: %v", err)
		return fmt.Errorf("batch record failures failed: %w", err)
	}

	d.logger.Infof("Successfully recorded %d slots to failure queue", len(slots))

	// 写日志（记录未回补的范围）
	logData := BackfillLog{
		Timestamp:          time.Now().Format(time.RFC3339),
		RedisLatestSlot:    redisSlot,
		WebSocketFirstSlot: wsSlot,
		RPCCurrentSlot:     rpcSlot,
		GapSize:            wsSlot - redisSlot - 1,
		RecoveredRange: &Range{
			Start: recoverStart,
			End:   recoverEnd,
		},
		UnrecoveredRange: &Range{
			Start: startSlot,
			End:   recoverStart - 1,
		},
		Message: fmt.Sprintf("Partial backfill: recovered %d slots, %d slots require manual backfill",
			len(slots), recoverStart-startSlot),
	}

	if err := d.writeBackfillLog(logData); err != nil {
		d.logger.Errorf("Write backfill log failed: %v", err)
		// 不返回错误，因为回补已成功
	}

	d.logger.Infof("⚠️  Manual backfill required for slots [%d, %d] (%d slots)",
		startSlot, recoverStart-1, recoverStart-startSlot)

	return nil
}

// writeBackfillLog 写回补日志到 JSON 文件
func (d *StartupGapDetector) writeBackfillLog(logData BackfillLog) error {
	// 1. 创建目录
	if err := os.MkdirAll(d.logDir, 0755); err != nil {
		return fmt.Errorf("create log directory failed: %w", err)
	}

	// 2. 生成文件名（包含时间戳）
	filename := fmt.Sprintf("backfill_gap_%d.json", time.Now().Unix())
	fullPath := filepath.Join(d.logDir, filename)

	// 3. 序列化为 JSON
	data, err := json.MarshalIndent(logData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal log data failed: %w", err)
	}

	// 4. 写入文件
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("write log file failed: %w", err)
	}

	d.logger.Infof("Backfill log written to: %s", fullPath)
	return nil
}

// getRPCCurrentSlot 通过 RPC 获取当前最新 slot
func (d *StartupGapDetector) getRPCCurrentSlot(ctx context.Context) (uint64, error) {
	slot, err := d.rpcClient.GetSlot(ctx)
	if err != nil {
		return 0, fmt.Errorf("RPC GetSlot failed: %w", err)
	}
	return slot, nil
}

// waitForWebSocketSlot 等待 WebSocket 接收第一个 slot
func (d *StartupGapDetector) waitForWebSocketSlot(ctx context.Context, timeout time.Duration) (uint64, error) {
	// 等待 latestSlotTracker 更新（WebSocket 会更新它）
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timeoutTimer.C:
			return 0, fmt.Errorf("timeout waiting for WebSocket slot")
		case <-ticker.C:
			slot := d.latestSlotTracker.GetLatest()
			if slot > 0 {
				return slot, nil
			}
		}
	}
}
