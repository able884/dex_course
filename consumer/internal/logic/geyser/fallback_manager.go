package geyser

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/consumer/internal/config"
)

// FallbackMode 降级模式
type FallbackMode int32

const (
	ModeGeyser FallbackMode = 0 // Geyser 模式
	ModeRPC    FallbackMode = 1 // RPC 降级模式
)

// FallbackManager 降级管理器（管理 Geyser 和 RPC 之间的切换）
type FallbackManager struct {
	cfg      config.GeyserFallbackConfig
	ctx      context.Context
	cancel   context.CancelFunc
	logger   logx.Logger
	mode     atomic.Int32  // 当前模式
	lastSlot atomic.Uint64 // 最后处理的 slot

	// 回调函数
	onSwitchToRPC    func() error // 切换到 RPC 模式的回调
	onSwitchToGeyser func() error // 切换到 Geyser 模式的回调

	// 统计
	stats struct {
		fallbackCount    atomic.Int64 // 降级次数
		recoveryCount    atomic.Int64 // 恢复成功次数
		recoveryAttempts atomic.Int64 // 恢复尝试次数
		lastFallbackAt   atomic.Int64 // 最后降级时间（Unix 纳秒）
		lastRecoveryAt   atomic.Int64 // 最后恢复时间（Unix 纳秒）
	}
}

// NewFallbackManager 创建降级管理器
func NewFallbackManager(
	ctx context.Context,
	cfg config.GeyserFallbackConfig,
	onSwitchToRPC func() error,
	onSwitchToGeyser func() error,
) *FallbackManager {
	ctx, cancel := context.WithCancel(ctx)

	fm := &FallbackManager{
		cfg:              cfg,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logx.WithContext(ctx).WithFields(logx.Field("component", "fallback_manager")),
		onSwitchToRPC:    onSwitchToRPC,
		onSwitchToGeyser: onSwitchToGeyser,
	}

	// 初始模式为 Geyser
	fm.mode.Store(int32(ModeGeyser))

	return fm
}

// Start 启动降级管理器
func (fm *FallbackManager) Start() {
	if !fm.cfg.Enabled {
		fm.logger.Info("降级管理器已禁用")
		return
	}

	fm.logger.Info("启动降级管理器...")

	// 启动恢复检查协程
	threading.GoSafe(func() {
		fm.recoveryCheckLoop()
	})

	fm.logger.Info("降级管理器已启动")
}

// recoveryCheckLoop 恢复检查循环（定期尝试从 RPC 模式恢复到 Geyser 模式）
func (fm *FallbackManager) recoveryCheckLoop() {
	ticker := time.NewTicker(fm.cfg.RecoveryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fm.ctx.Done():
			return
		case <-ticker.C:
			// 只有在 RPC 模式下才尝试恢复
			if fm.GetMode() == ModeRPC {
				fm.attemptRecovery()
			}
		}
	}
}

// attemptRecovery 尝试恢复到 Geyser 模式
func (fm *FallbackManager) attemptRecovery() {
	attempts := fm.stats.recoveryAttempts.Add(1)

	// 调用恢复回调
	if fm.onSwitchToGeyser != nil {
		if err := fm.onSwitchToGeyser(); err != nil {
			if attempts == 1 || attempts == 5 || attempts%20 == 0 {
				fm.logger.Infof("⚠️ Geyser 恢复失败（第 %d 次尝试）: %v", attempts, err)
			}
			return
		}
	}

	// 切换模式
	fm.mode.Store(int32(ModeGeyser))

	// 更新统计
	fm.stats.recoveryCount.Add(1)
	fm.stats.lastRecoveryAt.Store(time.Now().UnixNano())

	fm.logger.Infof("✅ 成功恢复到 Geyser 模式（尝试 %d 次后成功）", attempts)
}

// TriggerFallback 触发降级到 RPC 模式
func (fm *FallbackManager) TriggerFallback(reason string) error {
	if !fm.cfg.Enabled {
		return nil
	}

	// 检查是否已经在 RPC 模式
	if fm.GetMode() == ModeRPC {
		fm.logger.Infof("已在 RPC 模式，跳过降级")
		return nil
	}

	fm.logger.Infof("触发降级到 RPC 模式: %s", reason)

	// 调用降级回调
	if fm.onSwitchToRPC != nil {
		if err := fm.onSwitchToRPC(); err != nil {
			fm.logger.Errorf("切换到 RPC 模式失败: %v", err)
			return err
		}
	}

	// 切换模式
	fm.mode.Store(int32(ModeRPC))

	// 更新统计
	fm.stats.fallbackCount.Add(1)
	fm.stats.lastFallbackAt.Store(time.Now().UnixNano())

	fm.logger.Info("成功降级到 RPC 模式")
	return nil
}

// CheckGeyserHealth 检查 Geyser 健康状态（由外部定期调用）
func (fm *FallbackManager) CheckGeyserHealth(subscriber *GeyserSubscriber) {
	if !fm.cfg.Enabled {
		return
	}

	// 只有在 Geyser 模式下才检查健康状态
	if fm.GetMode() != ModeGeyser {
		return
	}

	// 检查连接健康
	if !subscriber.IsHealthy() {
		failures := subscriber.GetConsecutiveFailures()
		fm.logger.Infof("⚠️ Geyser 连接不健康，连续失败次数: %d，准备降级...", failures)

		// 触发降级
		fm.TriggerFallback("Geyser 连接不健康")
	}
}

// CheckSlotDelay 检查 slot 延迟（由外部调用）
func (fm *FallbackManager) CheckSlotDelay(currentSlot uint64, latestSlot uint64) {
	if !fm.cfg.Enabled {
		return
	}

	// 只有在 Geyser 模式下才检查延迟
	if fm.GetMode() != ModeGeyser {
		return
	}

	// 计算延迟（以 slot 数量计）
	if latestSlot > currentSlot {
		slotDelay := latestSlot - currentSlot

		// 估算时间延迟（假设每个 slot 约 400ms）
		estimatedDelay := time.Duration(slotDelay) * 400 * time.Millisecond

		if estimatedDelay > fm.cfg.SlotDelayThreshold {
			fm.logger.Errorf("Slot 延迟过大: %d slots (约 %v)", slotDelay, estimatedDelay)
			fm.TriggerFallback("Slot 延迟超过阈值")
		}
	}
}

// UpdateLastSlot 更新最后处理的 slot
func (fm *FallbackManager) UpdateLastSlot(slot uint64) {
	fm.lastSlot.Store(slot)
}

// GetMode 获取当前模式
func (fm *FallbackManager) GetMode() FallbackMode {
	return FallbackMode(fm.mode.Load())
}

// IsGeyserMode 是否为 Geyser 模式
func (fm *FallbackManager) IsGeyserMode() bool {
	return fm.GetMode() == ModeGeyser
}

// IsRPCMode 是否为 RPC 模式
func (fm *FallbackManager) IsRPCMode() bool {
	return fm.GetMode() == ModeRPC
}

// GetStats 获取统计信息
func (fm *FallbackManager) GetStats() map[string]interface{} {
	var lastFallbackAt, lastRecoveryAt time.Time

	if ts := fm.stats.lastFallbackAt.Load(); ts > 0 {
		lastFallbackAt = time.Unix(0, ts)
	}
	if ts := fm.stats.lastRecoveryAt.Load(); ts > 0 {
		lastRecoveryAt = time.Unix(0, ts)
	}

	return map[string]interface{}{
		"mode":              fm.GetMode(),
		"fallback_count":    fm.stats.fallbackCount.Load(),
		"recovery_count":    fm.stats.recoveryCount.Load(),
		"recovery_attempts": fm.stats.recoveryAttempts.Load(),
		"last_fallback_at":  lastFallbackAt,
		"last_recovery_at":  lastRecoveryAt,
		"last_slot":         fm.lastSlot.Load(),
	}
}

// Stop 停止降级管理器
func (fm *FallbackManager) Stop() {
	fm.logger.Info("停止降级管理器...")
	fm.cancel()

	// 打印最终统计
	stats := fm.GetStats()
	fm.logger.Infof("降级管理器统计: %+v", stats)

	fm.logger.Info("降级管理器已停止")
}
