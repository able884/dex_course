package consumer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
	"richcode.cc/dex/consumer/internal/config"
	pb "richcode.cc/dex/consumer/internal/mq/proto"
)

// 辅助函数：字符串转 ConsumeFromWhere
func parseConsumeFromWhere(s string) consumer.ConsumeFromWhere {
	switch s {
	case "CONSUME_FROM_LAST_OFFSET":
		return consumer.ConsumeFromLastOffset
	case "CONSUME_FROM_FIRST_OFFSET":
		return consumer.ConsumeFromFirstOffset
	case "CONSUME_FROM_TIMESTAMP":
		return consumer.ConsumeFromTimestamp
	default:
		return consumer.ConsumeFromLastOffset
	}
}

// 辅助函数：字符串转 MessageModel
func parseMessageModel(s string) consumer.MessageModel {
	switch s {
	case "CLUSTERING":
		return consumer.Clustering
	case "BROADCASTING":
		return consumer.BroadCasting
	default:
		return consumer.Clustering
	}
}

// BlockProcessor 区块处理接口
type BlockProcessor interface {
	ProcessBlock(ctx context.Context, blockMsg *pb.BlockMessage) error
}

// processResult 消息处理结果
type processResult struct {
	slot       uint64
	err        error
	retryCount int32
}

// BlockConsumer 区块消费者
type BlockConsumer struct {
	// 消费统计 - 必须放在 struct 开头，确保 8 字节对齐（用于 atomic 操作）
	totalConsumed   int64  // 总消费数量
	totalFailed     int64  // 总失败数量
	lastConsumeTime int64  // 最后消费时间（Unix 纳秒）
	lastConsumeSlot uint64 // 最后消费的 slot

	cfg            config.Config
	mqConsumer     rocketmq.PushConsumer
	blockProcessor BlockProcessor
	stopChan       chan struct{}
	logger         logx.Logger

	// 并发处理配置
	concurrency int // 并发处理消息的 goroutine 数量
}

// NewBlockConsumer 创建区块消费者
func NewBlockConsumer(cfg config.Config, blockProcessor BlockProcessor) (*BlockConsumer, error) {
	// 从配置读取并发度，默认为 10
	concurrency := cfg.Consumer.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	bc := &BlockConsumer{
		cfg:            cfg,
		blockProcessor: blockProcessor,
		stopChan:       make(chan struct{}),
		logger:         logx.WithContext(context.Background()).WithFields(logx.Field("service", "block_consumer")),
		concurrency:    concurrency,
	}

	if err := bc.initRocketMQ(); err != nil {
		return nil, fmt.Errorf("failed to initialize RocketMQ consumer: %w", err)
	}

	bc.logger.Infof("BlockConsumer initialized with concurrency=%d", concurrency)

	return bc, nil
}

// initRocketMQ 初始化 RocketMQ 消费者
func (bc *BlockConsumer) initRocketMQ() error {
	mqCfg := bc.cfg.RocketMQ

	// 计算批量拉取大小（基于并发度的 2 倍，确保有足够的消息并发处理）
	batchSize := bc.concurrency * 2
	if batchSize < 10 {
		batchSize = 10
	}
	if batchSize > 50 {
		batchSize = 50
	}

	// 创建消费者选项
	opts := []consumer.Option{
		consumer.WithNameServer(mqCfg.NameServers),
		consumer.WithGroupName(mqCfg.Consumer.GroupName),
		consumer.WithConsumeFromWhere(parseConsumeFromWhere(mqCfg.Consumer.ConsumeFromWhere)),
		consumer.WithConsumerModel(parseMessageModel(mqCfg.Consumer.MessageModel)),
		consumer.WithConsumeMessageBatchMaxSize(batchSize), // 批量拉取消息，内部并发处理
		consumer.WithMaxReconsumeTimes(mqCfg.Consumer.MaxReconsumeTimes),
	}

	bc.logger.Infof("RocketMQ consumer batch size set to %d (concurrency=%d)", batchSize, bc.concurrency)

	// 如果启用了 ACL
	if mqCfg.AccessKey != "" && mqCfg.SecretKey != "" {
		cred := primitive.Credentials{
			AccessKey: mqCfg.AccessKey,
			SecretKey: mqCfg.SecretKey,
		}
		opts = append(opts, consumer.WithCredentials(cred))
	}

	// 创建消费者
	c, err := rocketmq.NewPushConsumer(opts...)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	bc.mqConsumer = c

	return nil
}

// Start 启动消费者
func (bc *BlockConsumer) Start(ctx context.Context) error {
	// 订阅主题
	topic := bc.cfg.RocketMQ.Topics.Blocks
	// bc.consumeMessage -> 消费消息的函数
	err := bc.mqConsumer.Subscribe(topic, consumer.MessageSelector{}, bc.consumeMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe topic %s: %w", topic, err)
	}

	// 启动消费者
	if err := bc.mqConsumer.Start(); err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	return nil
}

// Stop 停止消费者
func (bc *BlockConsumer) Stop() {
	bc.logger.Info("Stopping BlockConsumer...")
	close(bc.stopChan)

	if bc.mqConsumer != nil {
		bc.logger.Info("Shutting down RocketMQ consumer...")
		if err := bc.mqConsumer.Shutdown(); err != nil {
			bc.logger.Errorf("Failed to shutdown RocketMQ consumer: %v", err)
		} else {
			bc.logger.Info("RocketMQ consumer shutdown successfully")
		}
	}

	bc.logger.Info("BlockConsumer stopped")
}

// consumeMessage 消费消息回调（并发处理批量消息）
func (bc *BlockConsumer) consumeMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	if len(msgs) == 0 {
		return consumer.ConsumeSuccess, nil
	}

	// 使用信号量控制并发度
	semaphore := make(chan struct{}, bc.concurrency)
	var wg sync.WaitGroup

	// 用于收集错误的 channel
	resultChan := make(chan processResult, len(msgs))

	// 并发处理每条消息
	for _, msg := range msgs {
		wg.Add(1)

		go func(m *primitive.MessageExt) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 处理单条消息
			result := bc.processSingleMessage(ctx, m)
			resultChan <- result
		}(msg)
	}

	// 等待所有 goroutine 完成
	wg.Wait()
	close(resultChan)

	// 收集结果
	var failedResults []processResult
	successCount := 0

	for result := range resultChan {
		if result.err != nil {
			failedResults = append(failedResults, result)
		} else {
			successCount++
		}
	}

	// 如果有失败的消息，判断是否需要重试
	if len(failedResults) > 0 {
		// 检查第一个失败消息的重试次数
		firstFailed := failedResults[0]

		if firstFailed.retryCount < bc.cfg.RocketMQ.Consumer.MaxReconsumeTimes {
			// 还有重试机会，返回重试
			bc.logger.Infof("❌ Batch has %d failed messages, will retry (retryCount=%d)",
				len(failedResults), firstFailed.retryCount)
			return consumer.ConsumeRetryLater, firstFailed.err
		} else {
			// 超过重试次数，让失败消息进入 DLQ
			bc.logger.Errorf("❌ Batch has %d failed messages after max retries, sending to DLQ",
				len(failedResults))
			return consumer.ConsumeSuccess, nil
		}
	}

	return consumer.ConsumeSuccess, nil
}

// processSingleMessage 处理单条消息
func (bc *BlockConsumer) processSingleMessage(ctx context.Context, msg *primitive.MessageExt) processResult {
	result := processResult{
		retryCount: msg.ReconsumeTimes,
	}

	// 检查消息属性
	encoding := msg.GetProperty("encoding")
	compressed := msg.GetProperty("compressed")

	// 解码区块消息
	decodeStart := time.Now()
	blockMsg, err := bc.decodeBlockMessage(msg.Body, compressed, encoding)
	decodeTime := time.Since(decodeStart)

	if err != nil {
		bc.logger.Errorf("❌ Failed to decode block message: %v", err)
		// 解码失败，不需要重试（数据问题）
		return result
	}

	result.slot = blockMsg.Slot

	// 计算消息在队列中的延迟
	messageAge := time.Since(time.Unix(msg.BornTimestamp/1000, 0))

	// 处理区块
	processStart := time.Now()
	err = bc.blockProcessor.ProcessBlock(ctx, blockMsg)
	processTime := time.Since(processStart)

	if err != nil {
		bc.logger.Errorf("❌ Failed to process block: slot=%d, error=%v", blockMsg.Slot, err)
		atomic.AddInt64(&bc.totalFailed, 1)
		result.err = err
		return result
	}

	// 处理成功，更新统计
	atomic.AddInt64(&bc.totalConsumed, 1)
	atomic.StoreInt64(&bc.lastConsumeTime, time.Now().UnixNano())
	atomic.StoreUint64(&bc.lastConsumeSlot, blockMsg.Slot)

	bc.logger.Infof("✅ Block processed: slot=%d, age_in_queue=%v, decode=%v, process=%v",
		blockMsg.Slot, messageAge, decodeTime, processTime)

	return result
}

// decodeBlockMessage 解码区块消息
func (bc *BlockConsumer) decodeBlockMessage(data []byte, compressed, encoding string) (*pb.BlockMessage, error) {
	var err error
	processedData := data

	// 1. 解压缩（如果需要）
	if compressed == "gzip" {
		processedData, err = bc.decompress(data)
		if err != nil {
			return nil, fmt.Errorf("decompress failed: %w", err)
		}
	}

	// 2. 反序列化 protobuf
	if encoding == "protobuf" || encoding == "" {
		blockMsg := &pb.BlockMessage{}
		if err := proto.Unmarshal(processedData, blockMsg); err != nil {
			return nil, fmt.Errorf("unmarshal protobuf failed: %w", err)
		}
		return blockMsg, nil
	}

	return nil, fmt.Errorf("unsupported encoding: %s", encoding)
}

// decompress 解压缩 gzip 数据
func (bc *BlockConsumer) decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader failed: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read gzip data failed: %w", err)
	}

	return decompressed, nil
}

// GetConsumerLag 获取消费延迟（仅用于监控）
// 返回自上次成功消费以来的时间间隔（秒）
func (bc *BlockConsumer) GetConsumerLag(ctx context.Context) (int64, error) {
	lastConsumeTime := atomic.LoadInt64(&bc.lastConsumeTime)

	if lastConsumeTime == 0 {
		// 还没有消费过任何消息
		return -1, nil
	}

	// 计算消费延迟（当前时间 - 最后消费时间）
	now := time.Now().UnixNano()
	lagNanos := now - lastConsumeTime
	lagSeconds := lagNanos / int64(time.Second)

	return lagSeconds, nil
}

// GetConsumerStats 获取消费者统计信息
func (bc *BlockConsumer) GetConsumerStats() ConsumerStats {
	return ConsumerStats{
		TotalConsumed:   atomic.LoadInt64(&bc.totalConsumed),
		TotalFailed:     atomic.LoadInt64(&bc.totalFailed),
		LastConsumeTime: time.Unix(0, atomic.LoadInt64(&bc.lastConsumeTime)),
		LastConsumeSlot: atomic.LoadUint64(&bc.lastConsumeSlot),
	}
}

// ConsumerStats 消费者统计信息
type ConsumerStats struct {
	TotalConsumed   int64     // 总成功消费数
	TotalFailed     int64     // 总失败数
	LastConsumeTime time.Time // 最后消费时间
	LastConsumeSlot uint64    // 最后消费的 slot
}
