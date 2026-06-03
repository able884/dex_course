package geyser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/consumer/internal/logic/slot"
	mqproducer "richcode.cc/dex/consumer/internal/mq/producer"
	"richcode.cc/dex/consumer/internal/svc"
)

// GeyserService Geyser 主服务（集成所有组件）
type GeyserService struct {
	ctx     context.Context
	cancel  context.CancelFunc
	svcCtx  *svc.ServiceContext
	cfg     config.GeyserConfig
	logger  logx.Logger
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	// Geyser 核心组件
	subscriber    *GeyserSubscriber
	streamHandler *GeyserStreamHandler
	adapter       *GeyserToBlockAdapter
	fallbackMgr   *FallbackManager

	// 数据通道
	slotDataChan chan *SlotData // Geyser 流数据通道

	// 集成组件（复用现有）
	latestSlotTracker *slot.LatestSlotTracker
	blockProducer     *mqproducer.BlockProducer

	// RPC 降级组件（降级时启用）
	rpcService *slot.BlockServiceMQ
}

// NewGeyserService 创建 Geyser 服务
func NewGeyserService(ctx context.Context, svcCtx *svc.ServiceContext) *GeyserService {
	ctx, cancel := context.WithCancel(ctx)

	// 创建 Latest Slot Tracker（复用现有组件）
	latestSlotTracker := slot.NewLatestSlotTracker(ctx, svcCtx.Redis, svcCtx.Config.LatestSlotTracker.SyncInterval)

	// 创建 Block Producer（复用现有组件）
	blockProducer, err := mqproducer.NewBlockProducer(svcCtx.Config.RocketMQ, ctx)
	if err != nil {
		panic(fmt.Errorf("创建 BlockProducer 失败: %w", err))
	}

	service := &GeyserService{
		ctx:               ctx,
		cancel:            cancel,
		svcCtx:            svcCtx,
		cfg:               svcCtx.Config.Geyser,
		logger:            logx.WithContext(ctx).WithFields(logx.Field("component", "geyser_service")),
		slotDataChan:      make(chan *SlotData, 1000), // 缓冲 1000 个 slot
		latestSlotTracker: latestSlotTracker,
		blockProducer:     blockProducer,
	}

	// 创建 Geyser 组件
	service.subscriber = NewGeyserSubscriber(ctx, svcCtx.Config.Geyser)
	service.adapter = NewGeyserToBlockAdapter(ctx, svcCtx)

	// 创建 FallbackManager（带降级回调）
	service.fallbackMgr = NewFallbackManager(
		ctx,
		svcCtx.Config.Geyser.Fallback,
		service.onSwitchToRPC,    // 降级回调
		service.onSwitchToGeyser, // 恢复回调
	)

	return service
}

// Start 启动 Geyser 服务
func (s *GeyserService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.logger.Info("Geyser 服务已在运行")
		return nil
	}

	s.logger.Info("启动 Geyser 服务...")

	// 1. 启动 Latest Slot Tracker（复用）
	s.latestSlotTracker.Start()

	// 2. Block Producer 已在构造函数中启动，无需额外 Start()

	// 3. 启动 Fallback Manager
	s.fallbackMgr.Start()

	// 4. 连接到 Geyser
	if err := s.connectAndSubscribe(); err != nil {
		s.logger.Errorf("连接 Geyser 失败: %v", err)
		// 触发降级到 RPC 模式
		s.fallbackMgr.TriggerFallback("初始连接失败")
		return nil // 不返回错误，允许降级模式继续运行
	}

	// 5. 启动 Slot 数据处理器
	s.wg.Add(1)
	threading.GoSafe(func() {
		defer s.wg.Done()
		s.processSlotData()
	})

	// 6. 启动健康检查
	s.wg.Add(1)
	threading.GoSafe(func() {
		defer s.wg.Done()
		s.healthCheckLoop()
	})

	s.running = true
	s.logger.Info("Geyser 服务启动成功")
	return nil
}

// connectAndSubscribe 连接到 Geyser 并订阅
func (s *GeyserService) connectAndSubscribe() error {
	// 连接到 Geyser
	if err := s.subscriber.Connect(); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	// 订阅流
	stream, err := s.subscriber.Subscribe()
	if err != nil {
		return fmt.Errorf("订阅失败: %w", err)
	}

	// 创建流处理器
	s.streamHandler = NewGeyserStreamHandler(
		s.ctx,
		stream,
		s.slotDataChan,
		s.latestSlotTracker.Update, // 传递 slot 更新回调
	)

	// 启动流处理器
	s.streamHandler.Start()

	s.logger.Info("Geyser 连接和订阅成功")
	return nil
}

// processSlotData 处理 Slot 数据（从 slotDataChan 接收并发送到 RocketMQ）
func (s *GeyserService) processSlotData() {
	s.logger.Info("启动 Slot 数据处理器...")

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Slot 数据处理器退出")
			return

		case slotData := <-s.slotDataChan:
			// 转换为 BlockMessage
			blockMsg, err := s.adapter.AdaptSlotData(slotData)
			if err != nil {
				s.logger.Errorf("适配 slot %d 失败: %v", slotData.Slot, err)
				continue
			}

			// 发送到 RocketMQ（复用现有 BlockProducer）
			if err := s.blockProducer.SendBlock(blockMsg); err != nil {
				s.logger.Errorf("发送 slot %d 到 RocketMQ 失败: %v", slotData.Slot, err)
				// 记录失败的 slot，由 RecoveryManager 补偿
				continue
			}
		}
	}
}

// healthCheckLoop 健康检查循环
func (s *GeyserService) healthCheckLoop() {
	ticker := time.NewTicker(s.cfg.HealthCheckInterval)
	defer ticker.Stop()

	s.logger.Info("启动健康检查循环...")

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// 只在 Geyser 模式下检查健康
			if s.fallbackMgr.IsGeyserMode() {
				s.checkHealth()
			}
		}
	}
}

// checkHealth 检查 Geyser 健康状态
func (s *GeyserService) checkHealth() {
	// 更新健康检查时间戳
	s.subscriber.UpdateHealthCheck()

	// 检查连接健康
	s.fallbackMgr.CheckGeyserHealth(s.subscriber)

	// 可以在此处添加更多健康检查逻辑
	// 例如：检查最后接收消息的时间、检查 slot 延迟等
}

// onSwitchToRPC 切换到 RPC 模式的回调
func (s *GeyserService) onSwitchToRPC() error {
	s.logger.Info("正在切换到 RPC 模式...")

	// 1. 停止 Geyser 流处理器
	if s.streamHandler != nil {
		s.streamHandler.Stop()
		s.streamHandler = nil
	}

	// 2. 关闭 Geyser 订阅
	if s.subscriber != nil {
		s.subscriber.Close()
	}

	// 3. 启动 RPC 订阅（复用现有 BlockServiceMQ）
	s.rpcService = slot.NewBlockServiceMQ(s.ctx, s.blockProducer, s.svcCtx)

	// 启动 RPC 服务
	s.rpcService.Start()

	s.logger.Info("已切换到 RPC 模式")
	return nil
}

// onSwitchToGeyser 切换到 Geyser 模式的回调
func (s *GeyserService) onSwitchToGeyser() error {
	s.logger.Info("正在切换到 Geyser 模式...")

	// 1. 停止 RPC 服务
	if s.rpcService != nil {
		s.rpcService.Stop()
		s.rpcService = nil
	}

	// 2. 重新连接并订阅 Geyser
	if err := s.connectAndSubscribe(); err != nil {
		s.logger.Errorf("重新连接 Geyser 失败: %v", err)
		return err
	}

	s.logger.Info("已切换到 Geyser 模式")
	return nil
}

// Stop 停止 Geyser 服务
func (s *GeyserService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		s.logger.Info("Geyser 服务未运行")
		return
	}

	s.logger.Info("停止 Geyser 服务...")

	// 1. 取消 context
	s.cancel()

	// 2. 停止各个组件
	if s.streamHandler != nil {
		s.streamHandler.Stop()
	}

	if s.subscriber != nil {
		s.subscriber.Shutdown()
	}

	if s.fallbackMgr != nil {
		s.fallbackMgr.Stop()
	}

	if s.blockProducer != nil {
		s.blockProducer.Close()
	}

	if s.latestSlotTracker != nil {
		s.latestSlotTracker.Stop()
	}

	// 如果在 RPC 模式，停止 RPC 服务
	if s.rpcService != nil {
		s.rpcService.Stop()
	}

	// 3. 关闭通道
	close(s.slotDataChan)

	// 4. 等待所有 goroutine 退出
	s.wg.Wait()

	s.running = false
	s.logger.Info("Geyser 服务已停止")
}

// GetStats 获取服务统计信息
func (s *GeyserService) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"running": s.running,
		"mode":    "geyser",
	}

	// Fallback Manager 统计
	if s.fallbackMgr != nil {
		stats["fallback"] = s.fallbackMgr.GetStats()
	}

	// Stream Handler 统计
	if s.streamHandler != nil {
		stats["stream"] = s.streamHandler.GetStats()
	}

	// Subscriber 健康状态
	if s.subscriber != nil {
		stats["subscriber"] = map[string]interface{}{
			"healthy":              s.subscriber.IsHealthy(),
			"consecutive_failures": s.subscriber.GetConsecutiveFailures(),
		}
	}

	return stats
}

// IsRunning 检查服务是否在运行
func (s *GeyserService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
