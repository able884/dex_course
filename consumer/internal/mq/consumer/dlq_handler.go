package consumer

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/blocto/solana-go-sdk/client"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/consumer/internal/logic/slot"
	"richcode.cc/dex/consumer/internal/svc"
)

// FailedBlock 失败区块记录（用于 DLQ 持久化）
type FailedBlock struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Slot        int64     `gorm:"column:slot;not null;index:idx_slot"`
	MsgID       string    `gorm:"column:msg_id;type:varchar(128)"`
	RetryTimes  int       `gorm:"column:retry_times;default:0"`
	ErrorInfo   string    `gorm:"column:error_info;type:text"`
	OriginTopic string    `gorm:"column:origin_topic;type:varchar(128)"`
	Status      string    `gorm:"column:status;type:varchar(32);default:'pending';index:idx_status"` // pending, retrying, resolved
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 表名
func (FailedBlock) TableName() string {
	return "failed_blocks"
}

// DLQHandler 死信队列处理器
type DLQHandler struct {
	cfg            config.Config
	mqConsumer     rocketmq.PushConsumer
	blockProcessor BlockProcessor
	stopChan       chan struct{}
	svcCtx         *svc.ServiceContext
	db             *gorm.DB
	logger         logx.Logger
	blockParser    *slot.BlockParser
}

// NewDLQHandler 创建 DLQ 处理器
func NewDLQHandler(cfg config.Config, blockProcessor BlockProcessor, svcCtx *svc.ServiceContext) (*DLQHandler, error) {
	ctx := context.Background()

	dh := &DLQHandler{
		cfg:            cfg,
		blockProcessor: blockProcessor,
		stopChan:       make(chan struct{}),
		svcCtx:         svcCtx,
		logger:         logx.WithContext(ctx).WithFields(logx.Field("service", "dlq_handler")),
		blockParser:    slot.NewBlockParser(svcCtx, ctx),
		db:             svcCtx.DB, // 直接使用 ServiceContext 的数据库连接
	}

	if err := dh.initRocketMQ(); err != nil {
		return nil, fmt.Errorf("failed to initialize DLQ consumer: %w", err)
	}

	return dh, nil
}

// initRocketMQ 初始化 RocketMQ DLQ 消费者
func (dh *DLQHandler) initRocketMQ() error {
	mqCfg := dh.cfg.RocketMQ

	// 创建消费者选项（使用不同的消费组避免冲突）
	opts := []consumer.Option{
		consumer.WithNameServer(mqCfg.NameServers),
		consumer.WithGroupName(mqCfg.Consumer.GroupName + "-dlq-handler"),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset), // 从头开始消费 DLQ
		consumer.WithConsumerModel(consumer.Clustering),
	}

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
		return fmt.Errorf("failed to create DLQ consumer: %w", err)
	}

	dh.mqConsumer = c

	return nil
}

// Start 启动 DLQ 处理器
func (dh *DLQHandler) Start(ctx context.Context) error {
	// DLQ 主题名称规则：%DLQ%{ConsumerGroup}
	dlqTopic := fmt.Sprintf("%%DLQ%%%s", dh.cfg.RocketMQ.Consumer.GroupName)

	// 订阅 DLQ 主题
	err := dh.mqConsumer.Subscribe(dlqTopic, consumer.MessageSelector{}, dh.consumeDLQMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe DLQ topic %s: %w", dlqTopic, err)
	}

	// 启动消费者
	if err := dh.mqConsumer.Start(); err != nil {
		return fmt.Errorf("failed to start DLQ consumer: %w", err)
	}

	// 启动定期统计 DLQ 消息数
	go dh.monitorDLQLoop(ctx, dlqTopic)

	return nil
}

// Stop 停止 DLQ 处理器
func (dh *DLQHandler) Stop() {
	dh.logger.Info("Stopping DLQHandler...")
	close(dh.stopChan)

	if dh.mqConsumer != nil {
		dh.logger.Info("Shutting down DLQ RocketMQ consumer...")
		if err := dh.mqConsumer.Shutdown(); err != nil {
			dh.logger.Errorf("Failed to shutdown DLQ consumer: %v", err)
		} else {
			dh.logger.Info("DLQ consumer shutdown successfully")
		}
	}

	dh.logger.Info("DLQHandler stopped")
}

// consumeDLQMessage 消费 DLQ 消息
func (dh *DLQHandler) consumeDLQMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		// 解析 Slot
		slot, err := dh.decodeSlot(msg.Body)
		if err != nil {
			// 解码失败，直接消费成功（避免阻塞 DLQ）
			return consumer.ConsumeSuccess, nil
		}

		// 尝试重新处理
		// 注意：DLQ 消息应该谨慎处理，避免再次失败导致死循环
		// 这里仅记录，不自动重试。需要人工介入或通过 API 触发重试
		dh.persistFailedSlot(ctx, slot, msg)

		// 更新指标

		// 消费成功（从 DLQ 中移除）
		return consumer.ConsumeSuccess, nil
	}

	return consumer.ConsumeSuccess, nil
}

// persistFailedSlot 持久化失败的 Slot（用于后续人工处理）
func (dh *DLQHandler) persistFailedSlot(ctx context.Context, slot uint64, msg *primitive.MessageExt) {
	if dh.db == nil {
		dh.logger.Error("Database not configured, cannot persist failed slot")
		return
	}

	failedBlock := &FailedBlock{
		Slot:        int64(slot),
		MsgID:       msg.MsgId,
		RetryTimes:  int(msg.ReconsumeTimes),
		ErrorInfo:   "Moved to DLQ after max retries",
		OriginTopic: msg.GetProperty("ORIGIN_MESSAGE_ID"),
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := dh.db.Create(failedBlock).Error; err != nil {
		dh.logger.Errorf("Failed to persist failed slot %d: %v", slot, err)
	} else {
		dh.logger.Infof("Persisted failed slot %d to DLQ database", slot)
	}
}

// RetryDLQMessage 重试 DLQ 消息（通过 API 触发）
func (dh *DLQHandler) RetryDLQMessage(ctx context.Context, slot uint64) error {
	dh.logger.Infof("Retrying DLQ message for slot %d", slot)

	// 1. 从数据库查询失败记录
	if dh.db == nil {
		return fmt.Errorf("database not configured")
	}

	var failedBlock FailedBlock
	result := dh.db.Where("slot = ? AND status = ?", slot, "pending").First(&failedBlock)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return fmt.Errorf("no pending DLQ record found for slot %d", slot)
		}
		return fmt.Errorf("failed to query DLQ record: %w", result.Error)
	}

	// 2. 更新状态为 retrying
	failedBlock.Status = "retrying"
	failedBlock.UpdatedAt = time.Now()
	if err := dh.db.Save(&failedBlock).Error; err != nil {
		dh.logger.Errorf("Failed to update DLQ record status: %v", err)
	}

	// 3. 重新获取区块数据（从 Solana RPC）
	solClient := dh.svcCtx.GetSolClient()
	blockData, err := dh.fetchBlockFromRPC(ctx, solClient, slot)
	if err != nil {
		// 更新错误信息
		failedBlock.Status = "pending"
		failedBlock.ErrorInfo = fmt.Sprintf("RPC fetch failed: %v", err)
		failedBlock.RetryTimes++
		failedBlock.UpdatedAt = time.Now()
		dh.db.Save(&failedBlock)
		return fmt.Errorf("failed to fetch block from RPC: %w", err)
	}

	// 4. 将区块数据解析为 BlockMessage
	blockMsg, err := dh.blockParser.ParseBlock(blockData, slot)
	if err != nil {
		failedBlock.Status = "pending"
		failedBlock.ErrorInfo = fmt.Sprintf("Parse failed: %v", err)
		failedBlock.RetryTimes++
		failedBlock.UpdatedAt = time.Now()
		dh.db.Save(&failedBlock)
		return fmt.Errorf("failed to parse block: %w", err)
	}

	if blockMsg == nil {
		// 区块没有相关交易，标记为已解决
		failedBlock.Status = "resolved"
		failedBlock.ErrorInfo = "Block has no relevant transactions"
		failedBlock.UpdatedAt = time.Now()
		dh.db.Save(&failedBlock)
		dh.logger.Infof("DLQ slot %d resolved: no relevant transactions", slot)
		return nil
	}

	// 5. 调用 blockProcessor 重新处理
	err = dh.blockProcessor.ProcessBlock(ctx, blockMsg)
	if err != nil {
		failedBlock.Status = "pending"
		failedBlock.ErrorInfo = fmt.Sprintf("Process failed: %v", err)
		failedBlock.RetryTimes++
		failedBlock.UpdatedAt = time.Now()
		dh.db.Save(&failedBlock)
		return fmt.Errorf("failed to reprocess block: %w", err)
	}

	// 6. 处理成功，更新状态为 resolved
	failedBlock.Status = "resolved"
	failedBlock.ErrorInfo = ""
	failedBlock.UpdatedAt = time.Now()
	if err := dh.db.Save(&failedBlock).Error; err != nil {
		dh.logger.Errorf("Failed to update DLQ record to resolved: %v", err)
	}

	dh.logger.Infof("Successfully retried DLQ slot %d", slot)
	return nil
}

// fetchBlockFromRPC 从 Solana RPC 获取区块数据
func (dh *DLQHandler) fetchBlockFromRPC(ctx context.Context, solClient *client.Client, slot uint64) (*client.Block, error) {
	dh.logger.Infof("Fetching block from RPC: slot=%d", slot)

	// 调用 RPC 获取区块
	block, err := solClient.GetBlock(ctx, slot)
	if err != nil {
		return nil, fmt.Errorf("RPC GetBlock failed: %w", err)
	}

	return block, nil
}

// monitorDLQLoop 定期监控 DLQ 消息数
func (dh *DLQHandler) monitorDLQLoop(ctx context.Context, dlqTopic string) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-dh.stopChan:
			return
		case <-ticker.C:
			// 统计数据库中的 pending DLQ 消息数
			if dh.db != nil {
				var count int64
				err := dh.db.Model(&FailedBlock{}).Where("status = ?", "pending").Count(&count).Error
				if err != nil {
					dh.logger.Errorf("Failed to count DLQ messages: %v", err)
				} else {
					dh.logger.Infof("DLQ pending messages: %d", count)

					// 如果有待处理的消息，记录详细信息
					if count > 0 {
						var failedBlocks []FailedBlock
						dh.db.Where("status = ?", "pending").
							Order("created_at DESC").
							Limit(10).
							Find(&failedBlocks)

						for _, fb := range failedBlocks {
							dh.logger.Infof("Pending DLQ: slot=%d, retries=%d, error=%s",
								fb.Slot, fb.RetryTimes, fb.ErrorInfo)
						}
					}
				}
			}
		}
	}
}

// decodeSlot 解码 Slot
func (dh *DLQHandler) decodeSlot(data []byte) (uint64, error) {
	if len(data) != 8 {
		return 0, fmt.Errorf("invalid slot data length: %d", len(data))
	}

	slot := binary.BigEndian.Uint64(data)
	return slot, nil
}

// GetDLQMessages 获取所有 DLQ 消息（用于人工查看）
func (dh *DLQHandler) GetDLQMessages(ctx context.Context) ([]DLQMessage, error) {
	if dh.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var failedBlocks []*FailedBlock

	// 查询所有 pending 状态的失败记录，按创建时间倒序
	if err := dh.db.Where("status = ?", "pending").Order("created_at DESC").Find(&failedBlocks).Error; err != nil {
		return nil, fmt.Errorf("failed to query DLQ messages: %w", err)
	}

	// 转换为 DLQMessage 格式
	messages := make([]DLQMessage, 0, len(failedBlocks))
	for _, fb := range failedBlocks {
		messages = append(messages, DLQMessage{
			Slot:        uint64(fb.Slot),
			MsgID:       fb.MsgID,
			RetryTimes:  fb.RetryTimes,
			ErrorInfo:   fb.ErrorInfo,
			OriginTopic: fb.OriginTopic,
			CreatedAt:   fb.CreatedAt,
			Status:      fb.Status,
		})
	}

	return messages, nil
}

// DLQMessage DLQ 消息信息
type DLQMessage struct {
	Slot        uint64
	MsgID       string
	RetryTimes  int
	ErrorInfo   string
	OriginTopic string
	CreatedAt   time.Time
	Status      string // pending, retrying, resolved
}
