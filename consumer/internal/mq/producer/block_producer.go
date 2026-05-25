package producer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
	"richcode.cc/dex/consumer/internal/config"
	pb "richcode.cc/dex/consumer/internal/mq/proto"
)

// BlockProducer 区块消息生产者
type BlockProducer struct {
	producer rocketmq.Producer
	topic    string
	logger   logx.Logger
	config   config.RocketMQProducerConfig
	ctx      context.Context
}

// NewBlockProducer 创建区块消息生产者
func NewBlockProducer(cfg config.RocketMQConfig, ctx context.Context) (*BlockProducer, error) {
	// 构建 NameServer 地址
	nameServers := cfg.NameServers
	if len(nameServers) == 0 {
		return nil, fmt.Errorf("RocketMQ NameServers is empty")
	}

	// 创建生产者选项
	opts := []producer.Option{
		producer.WithNameServer(nameServers),
		producer.WithRetry(cfg.Producer.RetryTimes),
		producer.WithSendMsgTimeout(cfg.Producer.SendTimeout),
	}

	// 如果配置了 AccessKey，则启用 ACL
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		credentials := primitive.Credentials{
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		}
		opts = append(opts, producer.WithCredentials(credentials))
	}

	// 创建生产者实例
	p, err := rocketmq.NewProducer(opts...)
	if err != nil {
		return nil, fmt.Errorf("create RocketMQ producer failed: %w", err)
	}

	// 启动生产者
	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start RocketMQ producer failed: %w", err)
	}

	logger := logx.WithContext(ctx).WithFields(logx.Field("service", "block_producer"))
	logger.Infof("BlockProducer started, topic=%s, nameServers=%v", cfg.Topics.Blocks, nameServers)

	return &BlockProducer{
		producer: p,
		topic:    cfg.Topics.Blocks,
		logger:   logger,
		config:   cfg.Producer,
		ctx:      ctx,
	}, nil
}

// SendBlock 发送区块消息到 RocketMQ
func (p *BlockProducer) SendBlock(blockMsg *pb.BlockMessage) error {
	if blockMsg == nil {
		return fmt.Errorf("block message is nil")
	}

	startTime := time.Now()

	// 1. 序列化为 protobuf 字节
	data, err := proto.Marshal(blockMsg)
	if err != nil {
		return fmt.Errorf("marshal protobuf failed: %w", err)
	}

	originalSize := len(data)

	// 2. Gzip 压缩
	compressed, err := p.compress(data)
	if err != nil {
		return fmt.Errorf("compress data failed: %w", err)
	}

	compressedSize := len(compressed)
	compressionRatio := float64(compressedSize) / float64(originalSize) * 100

	// 仅在消息较大时记录（避免日志过多）
	if originalSize > 500*1024 { // 大于 500KB
		p.logger.Infof("Large block message: original=%d bytes, compressed=%d bytes (%.1f%%)",
			originalSize, compressedSize, compressionRatio)
	}

	// 3. 检查消息大小（RocketMQ 默认最大 4MB，可配置到 16MB）
	if compressedSize > 16*1024*1024 {
		return fmt.Errorf("compressed message too large: %d bytes (max 16MB)", compressedSize)
	}

	// 4. 构建 RocketMQ 消息
	msg := &primitive.Message{
		Topic: p.topic,
		Body:  compressed,
	}

	// 设置消息属性
	msg.WithProperty("slot", strconv.FormatUint(blockMsg.Slot, 10))
	msg.WithProperty("block_time", strconv.FormatInt(blockMsg.BlockTime, 10))
	msg.WithProperty("tx_count", strconv.FormatInt(int64(blockMsg.TransactionCount), 10))
	msg.WithProperty("compressed", "gzip")
	msg.WithProperty("encoding", "protobuf")

	// 使用 slot 作为 OrderKey，保证同一 slot 的消息顺序
	msg.WithShardingKey(strconv.FormatUint(blockMsg.Slot, 10))

	// 5. 发送消息（顺序消息）
	result, err := p.producer.SendSync(p.ctx, msg)
	if err != nil {
		return fmt.Errorf("send message to RocketMQ failed: %w", err)
	}

	elapsed := time.Since(startTime)

	// 仅在发送较慢时记录（避免日志过多）
	if elapsed > 1*time.Second {
		p.logger.Infof("Slow send detected: slot=%d, msgId=%s, queue=%d, offset=%d, elapsed=%v",
			blockMsg.Slot,
			result.MsgID,
			result.MessageQueue.QueueId,
			result.QueueOffset,
			elapsed)
	}

	return nil
}

// compress 使用 gzip 压缩数据
func (p *BlockProducer) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	// 使用配置的压缩级别（默认 6）
	compressLevel := p.config.CompressLevel
	if compressLevel < 1 || compressLevel > 9 {
		compressLevel = 6 // 默认压缩级别
	}

	gzipWriter, err := gzip.NewWriterLevel(&buf, compressLevel)
	if err != nil {
		return nil, err
	}

	if _, err := gzipWriter.Write(data); err != nil {
		return nil, err
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Close 关闭生产者
func (p *BlockProducer) Close() error {
	p.logger.Info("Shutting down BlockProducer")
	return p.producer.Shutdown()
}

// SendAsync 异步发送区块消息
func (p *BlockProducer) SendAsync(blockMsg *pb.BlockMessage, callback func(result *primitive.SendResult, err error)) error {
	if blockMsg == nil {
		return fmt.Errorf("block message is nil")
	}

	// 1. 序列化为 protobuf 字节
	data, err := proto.Marshal(blockMsg)
	if err != nil {
		return fmt.Errorf("marshal protobuf failed: %w", err)
	}

	// 2. Gzip 压缩
	compressed, err := p.compress(data)
	if err != nil {
		return fmt.Errorf("compress data failed: %w", err)
	}

	// 3. 构建 RocketMQ 消息
	msg := &primitive.Message{
		Topic: p.topic,
		Body:  compressed,
	}

	msg.WithProperty("slot", strconv.FormatUint(blockMsg.Slot, 10))
	msg.WithProperty("compressed", "gzip")
	msg.WithProperty("encoding", "protobuf")
	msg.WithShardingKey(strconv.FormatUint(blockMsg.Slot, 10))

	// 4. 异步发送
	err = p.producer.SendAsync(p.ctx, func(ctx context.Context, result *primitive.SendResult, err error) {
		if err != nil {
			p.logger.Errorf("Async send failed: slot=%d, error=%v", blockMsg.Slot, err)
		} else {
			p.logger.Infof("Async send success: slot=%d, msgId=%s", blockMsg.Slot, result.MsgID)
		}
		if callback != nil {
			callback(result, err)
		}
	}, msg)

	return err
}

// BatchSendBlocks 批量发送区块消息
func (p *BlockProducer) BatchSendBlocks(blockMsgs []*pb.BlockMessage) error {
	if len(blockMsgs) == 0 {
		return nil
	}

	p.logger.Infof("Batch sending %d block messages", len(blockMsgs))

	var firstErr error
	successCount := 0

	for _, blockMsg := range blockMsgs {
		if err := p.SendBlock(blockMsg); err != nil {
			p.logger.Errorf("Failed to send block: slot=%d, error=%v", blockMsg.Slot, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successCount++
		}
	}

	p.logger.Infof("Batch send completed: success=%d, failed=%d", successCount, len(blockMsgs)-successCount)

	return firstErr
}
