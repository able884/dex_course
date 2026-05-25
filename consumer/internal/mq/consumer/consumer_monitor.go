package consumer

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

// ConsumerMonitor 消费者监控服务
type ConsumerMonitor struct {
	consumer *BlockConsumer
	ctx      context.Context
	logger   logx.Logger
}

// NewConsumerMonitor 创建消费者监控
func NewConsumerMonitor(consumer *BlockConsumer, ctx context.Context) *ConsumerMonitor {
	return &ConsumerMonitor{
		consumer: consumer,
		ctx:      ctx,
		logger:   logx.WithContext(ctx).WithFields(logx.Field("service", "consumer_monitor")),
	}
}

// Start 启动监控（每分钟打印一次统计）
func (m *ConsumerMonitor) Start() {
	m.logger.Info("Consumer monitor started")

	threading.GoSafe(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				m.logger.Info("Consumer monitor stopped")
				return
			case <-ticker.C:
				m.printStats()
			}
		}
	})
}

// Stop 停止监控
func (m *ConsumerMonitor) Stop() {
	m.logger.Info("Stopping consumer monitor")
	m.printStats() // 打印最终统计
}

// printStats 打印统计信息
func (m *ConsumerMonitor) printStats() {
	stats := m.consumer.GetConsumerStats()

	lagSeconds, _ := m.consumer.GetConsumerLag(m.ctx)

	if stats.TotalConsumed == 0 && lagSeconds == -1 {
		m.logger.Info("[Consumer Stats] No messages consumed yet (waiting for messages...)")
		return
	}

	var lagInfo string
	if lagSeconds < 0 {
		lagInfo = "N/A"
	} else if lagSeconds < 10 {
		lagInfo = "healthy"
	} else {
		lagInfo = "lagging"
	}

	m.logger.Infof("[Consumer Stats] Consumed: total=%d, failed=%d | Last: slot=%d, time=%s | Lag: %s",
		stats.TotalConsumed,
		stats.TotalFailed,
		stats.LastConsumeSlot,
		stats.LastConsumeTime.Format("15:04:05"),
		lagInfo,
	)
}
