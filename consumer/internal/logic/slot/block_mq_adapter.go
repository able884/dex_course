package slot

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	mqproducer "richcode.cc/dex/consumer/internal/mq/producer"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
)

// BlockServiceMQ MQ 模式的 Block 服务（WebSocket blockSubscribe + 解析 + RocketMQ 生产）
type BlockServiceMQ struct {
	blockSubscribe *BlockSubscribeService
	blockParser    *BlockParser
	blockProducer  *mqproducer.BlockProducer
	blockModel     solmodel.BlockModel // Block 数据模型
	rpcClient      *client.Client      // 仅用于 RecoveryManager
	ctx            context.Context
	blockChan      chan *BlockData
	logger         logx.Logger
	svcCtx         *svc.ServiceContext // 服务上下文（用于访问配置）

	// 状态追踪和恢复管理
	latestSlotTracker *LatestSlotTracker
	failureRecorder   *FailureRecorder
	recoveryManager   *RecoveryManager

	// Worker Pool 配置
	workerCount int

	// 统计计数器
	stats struct {
		blocksReceived       atomic.Int64 // 从 WebSocket 接收到的 block 总数
		blocksFiltered       atomic.Int64 // 过滤掉的区块数（无相关交易）
		blocksSent           atomic.Int64 // 成功发送到 RocketMQ 的区块数
		sendErrors           atomic.Int64 // 发送失败次数
		parseErrors          atomic.Int64 // 解析失败次数
		totalTransactions    atomic.Int64 // 总交易数
		filteredTransactions atomic.Int64 // 过滤后的交易数
		activeWorkers        atomic.Int32 // 当前活跃的 worker 数量
		channelUsage         atomic.Int32 // Channel 使用率（百分比）
	}
}

// NewBlockServiceMQ 创建用于 MQ 的 BlockService
func NewBlockServiceMQ(ctx context.Context, blockProducer *mqproducer.BlockProducer, svcCtx *svc.ServiceContext) *BlockServiceMQ {
	// 从配置读取性能参数
	cfg := svcCtx.Config.BlockProcessor
	blockChannelSize := cfg.SlotChannelSize // 复用配置项名称
	workerCount := cfg.WorkerCount

	// 创建一个内部 channel 用于接收 WebSocket block 消息
	blockChan := make(chan *BlockData, blockChannelSize)

	// 创建 RPC 客户端（仅用于 RecoveryManager）
	rpcClient := client.NewClient(svcCtx.Config.Sol.NodeUrl[0])

	// 创建 LatestSlotTracker
	latestSlotTracker := NewLatestSlotTracker(ctx, svcCtx.Redis, svcCtx.Config.LatestSlotTracker.SyncInterval)

	// 创建 FailureRecorder
	failureRecorder := NewFailureRecorder(ctx, svcCtx.Redis)

	// 创建 BlockParser
	blockParser := NewBlockParser(svcCtx, ctx)

	// 创建 RecoveryManager（恢复时仍需要 RPC）
	recoveryConfig := RecoveryManagerConfig{
		Interval:            svcCtx.Config.Recovery.Interval,
		BatchSize:           svcCtx.Config.Recovery.BatchSize,
		MaxRetryCount:       svcCtx.Config.Recovery.MaxRetryCount,
		RPCRetryMaxAttempts: svcCtx.Config.BlockProcessor.BlockNotAvailableRetry.MaxAttempts,
		RPCRetryDelay:       time.Duration(svcCtx.Config.BlockProcessor.BlockNotAvailableRetry.RetryDelayMs) * time.Millisecond,
	}
	recoveryManager := NewRecoveryManager(ctx, failureRecorder, blockProducer, blockParser, rpcClient, recoveryConfig)

	// 创建 BlockSubscribeService 配置
	blockSubscribeConfig := BlockSubscribeConfig{
		RPCRetryMaxAttempts: svcCtx.Config.BlockProcessor.BlockNotAvailableRetry.MaxAttempts,
		RPCRetryDelays: []time.Duration{
			50 * time.Millisecond,  // 快速重试
			100 * time.Millisecond, // 第二次
			200 * time.Millisecond, // 第三次 - 总计 350ms
		},
		FailedSlotLogDir: svcCtx.Config.FailedSlotLog.LogDir,
	}

	service := &BlockServiceMQ{
		blockSubscribe:    NewBlockSubscribeService(svcCtx, blockChan, rpcClient, failureRecorder, blockSubscribeConfig),
		blockParser:       blockParser,
		blockProducer:     blockProducer,
		blockModel:        svcCtx.BlockModel,
		rpcClient:         rpcClient,
		ctx:               ctx,
		blockChan:         blockChan,
		svcCtx:            svcCtx,
		workerCount:       workerCount,
		latestSlotTracker: latestSlotTracker,
		failureRecorder:   failureRecorder,
		recoveryManager:   recoveryManager,
		logger:            logx.WithContext(ctx).WithFields(logx.Field("service", "block_mq_adapter")),
	}

	return service
}

// performStartupGapDetection 执行启动 gap 检测
func (s *BlockServiceMQ) performStartupGapDetection() {
	s.logger.Info("Starting startup gap detection...")

	// 创建 StartupGapDetector
	detector := NewStartupGapDetector(
		s.latestSlotTracker,
		s.failureRecorder,
		s.rpcClient,
		s.svcCtx.Config.StartupBackfill.MaxGap,
		s.svcCtx.Config.StartupBackfill.LogDir,
	)

	// 执行检测和处理
	if err := detector.DetectAndHandle(s.ctx); err != nil {
		s.logger.Errorf("Startup gap detection failed: %v", err)
	} else {
		s.logger.Info("Startup gap detection completed successfully")
	}
}

// Start 启动服务
func (s *BlockServiceMQ) Start() {
	// 1. 启动 LatestSlotTracker（定时同步到 Redis）
	s.latestSlotTracker.Start()
	s.logger.Info("LatestSlotTracker started")

	// 2. 启动 RecoveryManager（定时恢复失败的 slot）
	if s.recoveryManager != nil {
		s.recoveryManager.Start()
		s.logger.Info("RecoveryManager started")
	}

	// 3. 启动 WebSocket block 订阅（必须在 gap 检测之前启动）
	s.blockSubscribe.Start()
	s.logger.Infof("BlockSubscribeService started (channel size: %d)", cap(s.blockChan))

	// 4. 执行启动 gap 检测（在 WebSocket 启动后，worker 启动前）
	threading.GoSafe(func() {
		s.performStartupGapDetection()
	})

	// 5. 启动 Worker Pool（多个并发 worker 处理 block）
	s.logger.Infof("Starting %d workers...", s.workerCount)
	for i := 0; i < s.workerCount; i++ {
		workerID := i
		threading.GoSafe(func() {
			s.blockWorker(workerID)
		})
	}

	// 3. 启动 Channel 使用率监控协程
	threading.GoSafe(func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.updateChannelUsage()
			}
		}
	})

	// 5. 启动统计监控协程（每分钟打印一次）
	threading.GoSafe(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.printStats()
			}
		}
	})

	s.logger.Info("BlockServiceMQ started successfully")
}

// blockWorker Worker 协程，从 blockChan 读取 block 并处理
func (s *BlockServiceMQ) blockWorker(workerID int) {
	s.logger.Infof("Worker #%d started", workerID)
	defer s.logger.Infof("Worker #%d exited", workerID)

	for {
		select {
		case <-s.ctx.Done():
			return
		case blockData, ok := <-s.blockChan:
			if !ok {
				// Channel 已关闭
				return
			}

			// 处理 block（包含统计）
			s.processBlockWithStats(blockData)
		}
	}
}

// processBlockWithStats 处理 block 并更新统计
func (s *BlockServiceMQ) processBlockWithStats(blockData *BlockData) {
	// 标记 worker 为活跃状态
	s.stats.activeWorkers.Add(1)
	defer s.stats.activeWorkers.Add(-1)

	// 处理 block
	s.processBlock(blockData)
}

// processBlock 处理单个 block（解析 → 过滤 → 发送）
func (s *BlockServiceMQ) processBlock(blockData *BlockData) {
	block := blockData.Block
	slot := blockData.Slot

	// 1. 统计：收到区块
	s.stats.blocksReceived.Add(1)
	txCount := int64(len(block.Transactions))
	s.stats.totalTransactions.Add(txCount)

	// 2. 更新最新 slot
	s.latestSlotTracker.Update(slot)

	// 3. 解析区块（过滤交易 + 计算 SOL 价格）
	blockMsg, err := s.blockParser.ParseBlock(block, slot)
	if err != nil {
		s.logger.Errorf("Block parse failed: slot=%d, error=%v", slot, err)
		s.stats.parseErrors.Add(1)
		s.failureRecorder.RecordFailure(slot)
		return
	}

	// 4. 如果过滤后没有相关交易，跳过（不记录）
	if blockMsg == nil {
		s.stats.blocksFiltered.Add(1)
		return
	}

	// 5. 统计：过滤后的交易数
	s.stats.filteredTransactions.Add(int64(blockMsg.TransactionCount))

	// 6. 发送到 RocketMQ
	if err := s.blockProducer.SendBlock(blockMsg); err != nil {
		s.logger.Errorf("Block send to MQ failed: slot=%d, error=%v", slot, err)
		s.stats.sendErrors.Add(1)
		s.failureRecorder.RecordFailure(slot)
		return
	}

	// 7. 成功
	s.stats.blocksSent.Add(1)
}

// updateChannelUsage 更新 Channel 使用率
func (s *BlockServiceMQ) updateChannelUsage() {
	channelLen := len(s.blockChan)
	channelCap := cap(s.blockChan)
	if channelCap > 0 {
		usage := int32(channelLen * 100 / channelCap)
		s.stats.channelUsage.Store(usage)
	}
}

// printStats 打印统计信息
func (s *BlockServiceMQ) printStats() {
	blocksReceived := s.stats.blocksReceived.Load()
	filtered := s.stats.blocksFiltered.Load()
	sent := s.stats.blocksSent.Load()
	sendErrors := s.stats.sendErrors.Load()
	parseErrors := s.stats.parseErrors.Load()
	totalTx := s.stats.totalTransactions.Load()
	filteredTx := s.stats.filteredTransactions.Load()
	activeWorkers := s.stats.activeWorkers.Load()
	channelUsage := s.stats.channelUsage.Load()

	// 计算过滤率
	var blockFilterRate float64
	if blocksReceived > 0 {
		blockFilterRate = float64(filtered) / float64(blocksReceived) * 100
	}

	var txFilterRate float64
	if totalTx > 0 {
		txFilterRate = float64(filteredTx) / float64(totalTx) * 100
	}

	// Channel 状态
	channelLen := len(s.blockChan)
	channelCap := cap(s.blockChan)

	s.logger.Infof("[Producer Stats] Workers: active=%d/%d | Channel: %d/%d (%d%%) | Blocks: received=%d, filtered=%d (%.1f%%), sent_to_mq=%d | Transactions: total=%d, filtered=%d (%.1f%%) | Errors: parse=%d, send=%d",
		activeWorkers, s.workerCount,
		channelLen, channelCap, channelUsage,
		blocksReceived, filtered, blockFilterRate, sent,
		totalTx, filteredTx, txFilterRate,
		parseErrors, sendErrors)
}

// Stop 停止服务
func (s *BlockServiceMQ) Stop() {
	s.logger.Info("Stopping BlockServiceMQ")

	// 打印最终统计
	s.printStats()

	// 1. 停止 WebSocket block 订阅（停止接收新 block）
	s.blockSubscribe.Stop()
	s.logger.Info("WebSocket block subscription stopped")

	// 2. 停止 RecoveryManager（停止恢复协程）
	if s.recoveryManager != nil {
		s.recoveryManager.Stop()
		s.logger.Info("RecoveryManager stopped")
	}

	// 3. 停止 LatestSlotTracker（强制同步到 Redis）
	if s.latestSlotTracker != nil {
		s.latestSlotTracker.Stop()
		s.logger.Info("LatestSlotTracker stopped (latest slot synced to Redis)")
	}

	// 4. Context 已经被取消，处理 goroutine 会自然退出
	// 给一点时间让 goroutine 完成当前正在处理的区块
	time.Sleep(2 * time.Second)
	s.logger.Info("Waiting for active workers to complete...")

	// 4. 关闭 BlockProducer（关闭 RocketMQ 连接）
	if s.blockProducer != nil {
		if err := s.blockProducer.Close(); err != nil {
			s.logger.Errorf("Failed to close block producer: %v", err)
		} else {
			s.logger.Info("BlockProducer closed successfully")
		}
	}

	s.logger.Info("BlockServiceMQ stopped successfully")
}
