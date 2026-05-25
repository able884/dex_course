package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/apache/rocketmq-client-go/v2/rlog"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/service"
	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/consumer/internal/logic/block"
	"richcode.cc/dex/consumer/internal/logic/slot"
	mqconsumer "richcode.cc/dex/consumer/internal/mq/consumer"
	mqproducer "richcode.cc/dex/consumer/internal/mq/producer"
	pb "richcode.cc/dex/consumer/internal/mq/proto"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/consumer/internal/util"
)

var (
	configFile = flag.String("f", "etc/consumer.yaml", "the config file")
	mode       = flag.String("mode", "all", "startup mode: all|producer|consumer")
)

func main() {
	flag.Parse()

	// 禁用 RocketMQ 客户端的 Info 日志
	rlog.SetLogLevel("error")

	// 验证启动模式
	if *mode != "all" && *mode != "producer" && *mode != "consumer" {
		fmt.Printf("❌ Invalid mode: %s. Valid modes: all, producer, consumer\n", *mode)
		os.Exit(1)
	}

	fmt.Printf("🚀 Starting in [%s] mode...\n", *mode)

	var c config.Config
	conf.MustLoad(*configFile, &c)
	config.SaveConf(c)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务上下文
	svcCtx := svc.NewSolServiceContext(c)

	// 管理多个服务
	group := service.NewServiceGroup()
	defer group.Stop()

	// 1. gRPC 服务器（所有模式都启动）
	// 检查端口是否可用，如果不可用则自动选择下一个可用端口
	originalAddr := c.ListenOn
	availableAddr, err := util.FindAvailablePort(originalAddr)
	if err != nil {
		fmt.Printf("❌ Failed to find available port: %v\n", err)
		os.Exit(1)
	}

	// 如果端口发生变化，更新配置并提示用户
	if availableAddr != originalAddr {
		origPort, _ := util.GetPortFromAddress(originalAddr)
		newPort, _ := util.GetPortFromAddress(availableAddr)
		fmt.Printf("⚠️  Port %d is already in use, using port %d instead\n", origPort, newPort)
		c.ListenOn = availableAddr
	}

	// 2. Pump 迁移工作线程（所有模式都启动）
	group.Add(block.NewPumpMigrationWorker(svcCtx))

	// 3. 根据模式启动不同的服务
	switch *mode {
	case "all":
		startProducerAndConsumer(ctx, group, c, svcCtx)
	case "producer":
		startProducerOnly(ctx, group, c, svcCtx)
	case "consumer":
		startConsumerOnly(ctx, group, c, svcCtx)
	}

	// 4. 优雅退出
	setupGracefulShutdown(cancel, group)

	fmt.Printf("🚀 Starting consumer at %s...\n", c.ListenOn)
	group.Start()
}

// startProducerAndConsumer 启动生产者和消费者（all 模式）
func startProducerAndConsumer(ctx context.Context, group *service.ServiceGroup, c config.Config, svcCtx *svc.ServiceContext) {
	fmt.Println("📡 Starting Producer + Consumer...")

	// 1. 启动区块生产者（WebSocket → 解析过滤 → RocketMQ）
	blockProducer, err := mqproducer.NewBlockProducer(c.RocketMQ, ctx)
	if err != nil {
		fmt.Printf("❌ Failed to create block producer: %v\n", err)
		os.Exit(1)
	}

	// 2. 启动区块消费者（从 RocketMQ 消费）
	blockProcessor := newBlockProcessorAdapter(svcCtx, ctx)
	blockConsumer, err := mqconsumer.NewBlockConsumer(c, blockProcessor)
	if err != nil {
		fmt.Printf("❌ Failed to create block consumer: %v\n", err)
		os.Exit(1)
	}
	if err := blockConsumer.Start(ctx); err != nil {
		fmt.Printf("❌ Failed to start block consumer: %v\n", err)
		os.Exit(1)
	}
	group.Add(newConsumerServiceWrapper(blockConsumer))

	// 启动消费者统计监控
	blockProcessor.startStatsMonitor()

	// 3. 启动 DLQ 处理器
	startDLQHandler(ctx, group, c, blockProcessor, svcCtx)

	// 4. WebSocket Block 监听器（订阅区块 → 解析 → 发送到 blockProducer）
	group.Add(slot.NewBlockServiceMQ(ctx, blockProducer, svcCtx))

	fmt.Println("✅ Producer + Consumer started")
}

// startProducerOnly 只启动生产者（producer 模式）
func startProducerOnly(ctx context.Context, group *service.ServiceGroup, c config.Config, svcCtx *svc.ServiceContext) {
	fmt.Println("📤 Starting Producer Only...")

	// 1. 启动区块生产者
	blockProducer, err := mqproducer.NewBlockProducer(c.RocketMQ, ctx)
	if err != nil {
		fmt.Printf("❌ Failed to create block producer: %v\n", err)
		os.Exit(1)
	}

	// 2. WebSocket Block 监听器
	group.Add(slot.NewBlockServiceMQ(ctx, blockProducer, svcCtx))

	fmt.Println("✅ Producer started (WebSocket → RocketMQ)")
	fmt.Println("💡 To consume messages, start consumer instances with: --mode consumer")
}

// startConsumerOnly 只启动消费者（consumer 模式）
func startConsumerOnly(ctx context.Context, group *service.ServiceGroup, c config.Config, svcCtx *svc.ServiceContext) {
	fmt.Println("📥 Starting Consumer Only...")

	// 1. 启动区块消费者
	blockProcessor := newBlockProcessorAdapter(svcCtx, ctx)
	blockConsumer, err := mqconsumer.NewBlockConsumer(c, blockProcessor)
	if err != nil {
		fmt.Printf("❌ Failed to create block consumer: %v\n", err)
		os.Exit(1)
	}
	if err := blockConsumer.Start(ctx); err != nil {
		fmt.Printf("❌ Failed to start block consumer: %v\n", err)
		os.Exit(1)
	}
	group.Add(newConsumerServiceWrapper(blockConsumer))

	// 启动消费者统计监控
	blockProcessor.startStatsMonitor()

	// 2. 启动 DLQ 处理器
	startDLQHandler(ctx, group, c, blockProcessor, svcCtx)

	fmt.Println("✅ Consumer started (RocketMQ → Database)")
	fmt.Println("💡 This instance will consume from RocketMQ topic: " + c.RocketMQ.Topics.Blocks)
}

// startDLQHandler 启动 DLQ 处理器
func startDLQHandler(ctx context.Context, group *service.ServiceGroup, c config.Config, blockProcessor *blockProcessorAdapter, svcCtx *svc.ServiceContext) {
	dlqHandler, err := mqconsumer.NewDLQHandler(c, blockProcessor, svcCtx)
	if err != nil {
		fmt.Printf("⚠️  Warning: Failed to create DLQ handler: %v\n", err)
		fmt.Printf("   DLQ handler is disabled.\n")
	} else {
		if err := dlqHandler.Start(ctx); err != nil {
			fmt.Printf("⚠️  Warning: Failed to start DLQ handler: %v\n", err)
			fmt.Printf("   This is normal on first startup.\n")
		} else {
			fmt.Printf("✅ DLQ handler started\n")
			group.Add(newDLQServiceWrapper(dlqHandler))
		}
	}
}

// blockProcessorAdapter 适配器：处理 protobuf BlockMessage
type blockProcessorAdapter struct {
	svcCtx     *svc.ServiceContext
	workerPool *block.ProtocolWorkerPool
	batchSaver *block.BatchSaver
	logger     logx.Logger
	ctx        context.Context

	// 统计计数器
	stats struct {
		blocksProcessed atomic.Int64 // 处理的区块总数
		blocksSucceeded atomic.Int64 // 成功处理的区块数
		blocksFailed    atomic.Int64 // 失败的区块数
		pairsCreated    atomic.Int64 // 创建的交易对数量
		txProcessed     atomic.Int64 // 处理的交易总数
	}
}

func newBlockProcessorAdapter(svcCtx *svc.ServiceContext, ctx context.Context) *blockProcessorAdapter {
	return &blockProcessorAdapter{
		svcCtx:     svcCtx,
		workerPool: block.NewProtocolWorkerPool(svcCtx, ctx),
		batchSaver: block.NewBatchSaver(svcCtx, ctx),
		logger:     logx.WithContext(ctx).WithFields(logx.Field("service", "consumer_adapter")),
		ctx:        ctx,
	}
}

func (a *blockProcessorAdapter) ProcessBlock(ctx context.Context, blockMsg *pb.BlockMessage) error {
	if blockMsg == nil {
		return fmt.Errorf("block message is nil")
	}

	// 更新统计：处理的区块数
	a.stats.blocksProcessed.Add(1)
	a.stats.txProcessed.Add(int64(blockMsg.TransactionCount))

	// 1. 使用协议工作池并发处理各协议交易
	results, err := a.workerPool.ProcessBlock(ctx, blockMsg)
	if err != nil {
		a.stats.blocksFailed.Add(1)
		return fmt.Errorf("worker pool processing failed: %w", err)
	}

	// 2. 批量保存处理结果到数据库
	saveResults, err := a.batchSaver.SaveResults(ctx, blockMsg.Slot, results)
	if err != nil {
		a.stats.blocksFailed.Add(1)
		return fmt.Errorf("batch save failed: %w", err)
	}

	// 计算保存的交易对数量
	var pairCount int64
	for _, sr := range saveResults {
		if sr.Success {
			pairCount += sr.RowsAffected
		}
	}

	// 更新统计：成功处理的区块数和创建的交易对数量
	a.stats.blocksSucceeded.Add(1)
	a.stats.pairsCreated.Add(pairCount)
	return nil
}

// printStats 打印消费者统计信息
func (a *blockProcessorAdapter) printStats() {
	blocksProcessed := a.stats.blocksProcessed.Load()
	blocksSucceeded := a.stats.blocksSucceeded.Load()
	blocksFailed := a.stats.blocksFailed.Load()
	pairsCreated := a.stats.pairsCreated.Load()
	txProcessed := a.stats.txProcessed.Load()

	// 计算成功率
	var successRate float64
	if blocksProcessed > 0 {
		successRate = float64(blocksSucceeded) / float64(blocksProcessed) * 100
	}

	a.logger.Infof("[Consumer Stats] Blocks: processed=%d, succeeded=%d (%.1f%%), failed=%d | Transactions: processed=%d | Pairs: created=%d",
		blocksProcessed, blocksSucceeded, successRate, blocksFailed,
		txProcessed, pairsCreated)
}

// startStatsMonitor 启动统计监控
func (a *blockProcessorAdapter) startStatsMonitor() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.printStats()
			}
		}
	}()
}

// setupGracefulShutdown 设置优雅退出
func setupGracefulShutdown(cancel context.CancelFunc, group *service.ServiceGroup) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// 第一次信号：优雅退出
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
		fmt.Println("Press Ctrl+C again to force exit immediately")

		// 启动超时强制退出
		go func() {
			time.Sleep(5 * time.Second)
			fmt.Println("\n⏱️  Shutdown timeout (5s), forcing exit...")
			os.Exit(1)
		}()

		// 启动第二次信号监听：立即强制退出
		go func() {
			sig := <-sigChan
			fmt.Printf("\n⚠️  Received second signal %v, forcing immediate exit!\n", sig)
			os.Exit(1)
		}()

		// 1. 取消 context，通知所有服务停止
		cancel()

		// 2. 显式调用 ServiceGroup.Stop() 停止所有服务
		fmt.Println("Stopping all services...")
		group.Stop()

		fmt.Println("✅ All services stopped, exiting...")
		os.Exit(0)
	}()
}

// consumerServiceWrapper 将 BlockConsumer 包装为 go-zero Service
type consumerServiceWrapper struct {
	consumer *mqconsumer.BlockConsumer
}

func newConsumerServiceWrapper(consumer *mqconsumer.BlockConsumer) *consumerServiceWrapper {
	return &consumerServiceWrapper{consumer: consumer}
}

func (w *consumerServiceWrapper) Start() {
	// BlockConsumer 已经在外部启动，这里不需要做任何事
	fmt.Println("BlockConsumer service wrapper started")
}

func (w *consumerServiceWrapper) Stop() {
	fmt.Println("Stopping BlockConsumer...")
	w.consumer.Stop()
	fmt.Println("BlockConsumer stopped")
}

// dlqServiceWrapper 将 DLQHandler 包装为 go-zero Service
type dlqServiceWrapper struct {
	dlqHandler *mqconsumer.DLQHandler
}

func newDLQServiceWrapper(dlqHandler *mqconsumer.DLQHandler) *dlqServiceWrapper {
	return &dlqServiceWrapper{dlqHandler: dlqHandler}
}

func (w *dlqServiceWrapper) Start() {
	// DLQHandler 已经在外部启动，这里不需要做任何事
	fmt.Println("DLQHandler service wrapper started")
}

func (w *dlqServiceWrapper) Stop() {
	fmt.Println("Stopping DLQHandler...")
	w.dlqHandler.Stop()
	fmt.Println("DLQHandler stopped")
}
