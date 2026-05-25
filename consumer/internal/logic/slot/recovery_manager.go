package slot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/zeromicro/go-zero/core/logx"
	mqproducer "richcode.cc/dex/consumer/internal/mq/producer"
)

// RecoveryManager 恢复管理器
// 职责：
// 1. 定时从失败队列获取 slot
// 2. 重新处理（RPC → 过滤 → 发 MQ）
// 3. 管理重试次数
type RecoveryManager struct {
	failureRecorder *FailureRecorder
	blockProducer   *mqproducer.BlockProducer
	blockParser     *BlockParser // 区块解析器（包含过滤和SOL价格计算）
	rpcClient       *client.Client
	logger          logx.Logger
	ctx             context.Context
	cancel          context.CancelFunc

	// 配置
	interval      time.Duration // 恢复间隔
	batchSize     int           // 每批处理多少个 slot
	maxRetryCount int           // 最大重试次数

	// 内存记录重试次数（key: slot, value: retry count）
	retryCount map[uint64]int
	mu         sync.RWMutex

	// 定时器
	ticker *time.Ticker

	// RPC 重试配置
	rpcRetryMaxAttempts int
	rpcRetryDelay       time.Duration
}

// RecoveryManagerConfig 恢复管理器配置
type RecoveryManagerConfig struct {
	Interval            time.Duration
	BatchSize           int
	MaxRetryCount       int
	RPCRetryMaxAttempts int           // RPC "Block not available" 重试次数
	RPCRetryDelay       time.Duration // RPC 重试延迟
}

// NewRecoveryManager 创建恢复管理器
func NewRecoveryManager(
	ctx context.Context,
	failureRecorder *FailureRecorder,
	blockProducer *mqproducer.BlockProducer,
	blockParser *BlockParser,
	rpcClient *client.Client,
	config RecoveryManagerConfig,
) *RecoveryManager {
	recoveryCtx, cancel := context.WithCancel(ctx)

	return &RecoveryManager{
		failureRecorder:     failureRecorder,
		blockProducer:       blockProducer,
		blockParser:         blockParser,
		rpcClient:           rpcClient,
		logger:              logx.WithContext(ctx).WithFields(logx.Field("component", "recovery_manager")),
		ctx:                 recoveryCtx,
		cancel:              cancel,
		interval:            config.Interval,
		batchSize:           config.BatchSize,
		maxRetryCount:       config.MaxRetryCount,
		retryCount:          make(map[uint64]int),
		rpcRetryMaxAttempts: config.RPCRetryMaxAttempts,
		rpcRetryDelay:       config.RPCRetryDelay,
	}
}

// Start 启动恢复协程
func (m *RecoveryManager) Start() {
	m.ticker = time.NewTicker(m.interval)

	go m.recoveryLoop()

	m.logger.Infof("RecoveryManager started (interval: %v, batch: %d, max retry: %d)",
		m.interval, m.batchSize, m.maxRetryCount)
}

// Stop 停止恢复管理器
func (m *RecoveryManager) Stop() {
	m.logger.Info("Stopping RecoveryManager...")

	if m.ticker != nil {
		m.ticker.Stop()
	}

	m.cancel()

	m.logger.Info("RecoveryManager stopped")
}

// recoveryLoop 恢复主循环
func (m *RecoveryManager) recoveryLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.ticker.C:
			m.processRecoveryBatch()
		}
	}
}

// processRecoveryBatch 处理一批恢复任务
func (m *RecoveryManager) processRecoveryBatch() {
	// 1. 获取失败队列
	slots, err := m.failureRecorder.GetFailedSlots(m.batchSize)
	if err != nil {
		m.logger.Errorf("Get failed slots error: %v", err)
		return
	}

	if len(slots) == 0 {
		return // 没有失败的 slot
	}

	m.logger.Infof("Processing recovery batch: %d slots", len(slots))

	// 2. 遍历处理每个 slot
	successCount := 0
	giveUpCount := 0

	for _, slot := range slots {
		// 检查重试次数
		m.mu.RLock()
		count := m.retryCount[slot]
		m.mu.RUnlock()

		if count >= m.maxRetryCount {
			// 超过最大重试次数，放弃
			m.logger.Infof("Slot %d exceeded max retries (%d), giving up", slot, m.maxRetryCount)
			m.failureRecorder.RemoveFailure(slot)

			m.mu.Lock()
			delete(m.retryCount, slot)
			m.mu.Unlock()

			giveUpCount++
			continue
		}

		// 恢复 slot
		err := m.recoverSlot(slot)
		if err != nil {
			m.logger.Errorf("Recover slot %d failed (attempt %d/%d): %v", slot, count+1, m.maxRetryCount, err)

			// 增加重试计数
			m.mu.Lock()
			m.retryCount[slot] = count + 1
			m.mu.Unlock()
		} else {
			m.mu.Lock()
			delete(m.retryCount, slot)
			m.mu.Unlock()

			successCount++
		}
	}

	m.logger.Infof("Recovery batch completed: success=%d, gave_up=%d, remaining=%d",
		successCount, giveUpCount, len(slots)-successCount-giveUpCount)
}

// recoverSlot 恢复单个 slot
func (m *RecoveryManager) recoverSlot(slot uint64) error {
	// 1. RPC 获取 block
	block, err := m.getBlockBySlot(slot)
	if err != nil {
		// 判断错误类型
		if isSlotSkippedError(err) {
			// Slot 被跳过，从失败队列移除
			m.logger.Infof("Slot %d was skipped by consensus, removing from failed queue", slot)
			m.failureRecorder.RemoveFailure(slot)
			return nil
		}

		// 其他错误，返回失败（会重试）
		return fmt.Errorf("RPC GetBlock failed: %w", err)
	}

	// 2. 解析区块（过滤交易 + 计算 SOL 价格）
	blockMsg, err := m.blockParser.ParseBlock(block, slot)
	if err != nil {
		return fmt.Errorf("parse block failed: %w", err)
	}

	if blockMsg == nil {
		// 无相关交易，从失败队列移除
		m.logger.Infof("Slot %d has no relevant transactions, removing from failed queue", slot)
		m.failureRecorder.RemoveFailure(slot)
		return nil
	}

	// 3. 发送到 MQ
	err = m.blockProducer.SendBlock(blockMsg)
	if err != nil {
		return fmt.Errorf("send to MQ failed: %w", err)
	}

	// 4. 发送成功，从失败队列移除
	m.failureRecorder.RemoveFailure(slot)
	m.logger.Infof("✅ Recovered slot %d successfully (%d tx)", slot, blockMsg.TransactionCount)

	return nil
}

// getBlockBySlot RPC 获取指定 slot 的区块
// 实现 "Block not available" 错误重试机制
func (m *RecoveryManager) getBlockBySlot(slot uint64) (*client.Block, error) {
	// RPC 配置
	cfg := client.GetBlockConfig{
		Commitment:         "confirmed",
		TransactionDetails: "full",
	}

	var lastErr error

	// 重试循环
	for attempt := 1; attempt <= m.rpcRetryMaxAttempts; attempt++ {
		// 创建带超时的 context
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)

		startTime := time.Now()
		blockResponse, err := m.rpcClient.GetBlockWithConfig(ctx, slot, cfg)
		elapsed := time.Since(startTime)

		cancel() // 释放 context 资源

		if err == nil {
			// 成功获取
			if blockResponse == nil {
				return nil, fmt.Errorf("block response is nil")
			}

			if attempt > 1 {
				m.logger.Infof("✅ Recovery: Slot %d RPC succeeded on attempt %d/%d after %v", slot, attempt, m.rpcRetryMaxAttempts, elapsed)
			}

			return blockResponse, nil
		}

		// 记录错误
		lastErr = err

		// 判断是否为 "Block not available" 错误
		if isBlockNotAvailableError(err) {
			// 如果还有重试机会，等待后重试
			if attempt < m.rpcRetryMaxAttempts {
				m.logger.Infof("⏳ Recovery: Slot %d block not available (attempt %d/%d), retrying in %v...", slot, attempt, m.rpcRetryMaxAttempts, m.rpcRetryDelay)
				time.Sleep(m.rpcRetryDelay)
				continue
			} else {
				// 最后一次尝试失败
				m.logger.Infof("❌ Recovery: Slot %d block not available after %d attempts", slot, m.rpcRetryMaxAttempts)
			}
		} else {
			// 其他类型的错误，不重试
			break
		}
	}

	return nil, fmt.Errorf("RPC GetBlock failed: %w", lastErr)
}

// GetRetryCount 获取 slot 的重试次数（用于监控）
func (m *RecoveryManager) GetRetryCount(slot uint64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.retryCount[slot]
}

// GetStats 获取恢复管理器统计信息
func (m *RecoveryManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"retry_slots_count": len(m.retryCount),
		"interval":          m.interval.String(),
		"batch_size":        m.batchSize,
		"max_retry_count":   m.maxRetryCount,
	}
}

// isSlotSkippedError 检查错误是否为 slot 被跳过错误
func isSlotSkippedError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return errMsg == "slot was skipped" ||
		errMsg == "Slot was skipped, or missing in long-term storage"
}

// isBlockNotAvailableError 检查错误是否为 "Block not available" 错误
func isBlockNotAvailableError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return errMsg == "Block not available for slot" ||
		len(errMsg) > 30 && errMsg[:30] == "Block not available for slot"
}
