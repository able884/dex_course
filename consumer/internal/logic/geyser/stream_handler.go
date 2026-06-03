package geyser

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/rpcpool/yellowstone-grpc/examples/golang/proto"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

// SlotData 单个 slot 的聚合数据
type SlotData struct {
	Slot         uint64
	Transactions []*pb.SubscribeUpdateTransaction
	BlockTime    int64
	mu           sync.RWMutex
}

// AddTransaction 添加交易到 slot
func (sd *SlotData) AddTransaction(tx *pb.SubscribeUpdateTransaction) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.Transactions = append(sd.Transactions, tx)
}

// GetTransactions 获取所有交易（只读）
func (sd *SlotData) GetTransactions() []*pb.SubscribeUpdateTransaction {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.Transactions
}

// GeyserStreamHandler 处理 Geyser 流数据
type GeyserStreamHandler struct {
	ctx                context.Context
	cancel             context.CancelFunc
	stream             pb.Geyser_SubscribeClient
	slotDataMap        sync.Map // map[uint64]*SlotData
	currentSlot        atomic.Uint64
	logger             logx.Logger
	flushInterval      time.Duration
	slotDataChan       chan *SlotData
	latestSlotCallback func(uint64) // 更新 latest slot tracker 的回调

	// 统计信息
	stats struct {
		transactionsReceived atomic.Int64 // 收到的交易总数
		slotsProcessed       atomic.Int64 // 已处理的 slot 数
		flushes              atomic.Int64 // 刷新次数
	}
}

// NewGeyserStreamHandler 创建新的流处理器
func NewGeyserStreamHandler(
	ctx context.Context,
	stream pb.Geyser_SubscribeClient,
	slotDataChan chan *SlotData,
	latestSlotCallback func(uint64),
) *GeyserStreamHandler {
	ctx, cancel := context.WithCancel(ctx)

	return &GeyserStreamHandler{
		ctx:                ctx,
		cancel:             cancel,
		stream:             stream,
		slotDataChan:       slotDataChan,
		latestSlotCallback: latestSlotCallback,
		flushInterval:      1 * time.Second, // 1 秒窗口进行 slot 聚合
		logger:             logx.WithContext(ctx).WithFields(logx.Field("component", "geyser_stream_handler")),
	}
}

// Start 启动流处理
func (h *GeyserStreamHandler) Start() {
	h.logger.Info("启动 Geyser 流处理器...")

	// 启动刷新定时器
	threading.GoSafe(func() {
		h.flushTimer()
	})

	// 启动流接收器
	threading.GoSafe(func() {
		h.receiveStream()
	})

	// 启动统计日志（每分钟打印一次）
	threading.GoSafe(func() {
		h.statsLogger()
	})

	h.logger.Info("Geyser 流处理器已启动")
}

// receiveStream 从 Geyser 流接收消息
func (h *GeyserStreamHandler) receiveStream() {
	defer func() {
		h.logger.Info("流接收器已退出")
	}()

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			// 从流接收更新
			update, err := h.stream.Recv()
			if err != nil {
				h.logger.Errorf("从流接收失败: %v", err)
				// 流错误，退出并让 FallbackManager 处理重连
				return
			}

			// 处理更新
			h.processUpdate(update)
		}
	}
}

// processUpdate 处理单个 SubscribeUpdate 消息
func (h *GeyserStreamHandler) processUpdate(update *pb.SubscribeUpdate) {
	// 提取交易更新
	if tx := update.GetTransaction(); tx != nil {
		h.processTransaction(tx)
	}

	// 也可以处理其他类型的更新（如果需要）：
	// - update.GetAccount() 用于账户更新
	// - update.GetSlot() 用于 slot 更新
	// - update.GetBlock() 用于区块更新
}

// processTransaction 处理交易更新
func (h *GeyserStreamHandler) processTransaction(tx *pb.SubscribeUpdateTransaction) {
	// 从交易获取 slot
	slot := tx.GetSlot()
	if slot == 0 {
		return
	}

	// 更新统计
	h.stats.transactionsReceived.Add(1)

	// 检查是否为新 slot
	currentSlot := h.currentSlot.Load()
	if slot > currentSlot {
		// 检测到新 slot，刷新之前的 slot
		if currentSlot > 0 {
			h.flushSlot(currentSlot)
		}

		// 更新当前 slot
		h.currentSlot.Store(slot)

		// 通知 latest slot tracker
		if h.latestSlotCallback != nil {
			h.latestSlotCallback(slot)
		}
	}

	// 获取或创建 slot 数据
	slotData := h.getOrCreateSlotData(slot)

	// 添加交易到 slot
	slotData.AddTransaction(tx)
}

// getOrCreateSlotData 获取或创建 slot 数据
func (h *GeyserStreamHandler) getOrCreateSlotData(slot uint64) *SlotData {
	// 尝试加载现有的
	if value, ok := h.slotDataMap.Load(slot); ok {
		return value.(*SlotData)
	}

	// 创建新的 slot 数据
	slotData := &SlotData{
		Slot:         slot,
		Transactions: make([]*pb.SubscribeUpdateTransaction, 0, 100), // 为性能预分配
		BlockTime:    time.Now().Unix(),
	}

	// 存储并返回
	actual, loaded := h.slotDataMap.LoadOrStore(slot, slotData)
	if loaded {
		// 另一个 goroutine 先创建了，使用那个
		return actual.(*SlotData)
	}

	return slotData
}

// flushTimer 定期刷新旧 slot
func (h *GeyserStreamHandler) flushTimer() {
	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			// 如果当前 slot 在 flushInterval 内没有收到新交易，则刷新
			currentSlot := h.currentSlot.Load()
			if currentSlot > 0 {
				h.flushSlot(currentSlot)
			}
		}
	}
}

// flushSlot 将 slot 的数据刷新到 channel
func (h *GeyserStreamHandler) flushSlot(slot uint64) {
	// 从 map 加载并删除
	value, ok := h.slotDataMap.LoadAndDelete(slot)
	if !ok {
		return
	}

	slotData := value.(*SlotData)

	// 如果没有交易则跳过
	if len(slotData.GetTransactions()) == 0 {
		return
	}

	// 更新统计
	h.stats.slotsProcessed.Add(1)
	h.stats.flushes.Add(1)

	txCount := len(slotData.GetTransactions())

	// 发送到 channel（非阻塞以避免死锁）
	select {
	case h.slotDataChan <- slotData:
		// 移除高频成功日志，改为只在交易数量较多时打印
		if txCount > 100 {
			h.logger.Infof("已刷新大 slot %d，包含 %d 笔交易", slot, txCount)
		}
	case <-h.ctx.Done():
		return
	default:
		// Channel 满是严重问题，保留错误日志
		h.logger.Errorf("⚠️ Slot 数据 channel 已满，丢弃 slot %d（包含 %d 笔交易）", slot, txCount)
	}
}

// statsLogger 定期打印统计日志（避免高频日志）
func (h *GeyserStreamHandler) statsLogger() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastTxCount, lastSlotCount int64

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			currentTxCount := h.stats.transactionsReceived.Load()
			currentSlotCount := h.stats.slotsProcessed.Load()

			txDelta := currentTxCount - lastTxCount
			slotDelta := currentSlotCount - lastSlotCount

			if txDelta > 0 || slotDelta > 0 {
				h.logger.Infof("📊 Geyser 流统计 [1min]: 交易 +%d (总计 %d) | Slot +%d (总计 %d)",
					txDelta, currentTxCount, slotDelta, currentSlotCount)
			}

			lastTxCount = currentTxCount
			lastSlotCount = currentSlotCount
		}
	}
}

// GetStats 返回处理器统计信息
func (h *GeyserStreamHandler) GetStats() map[string]int64 {
	return map[string]int64{
		"transactions_received": h.stats.transactionsReceived.Load(),
		"slots_processed":       h.stats.slotsProcessed.Load(),
		"flushes":               h.stats.flushes.Load(),
	}
}

// Stop 停止流处理器
func (h *GeyserStreamHandler) Stop() {
	h.logger.Info("停止 Geyser 流处理器...")
	h.cancel()

	// 刷新剩余的 slot
	h.slotDataMap.Range(func(key, value interface{}) bool {
		slot := key.(uint64)
		h.flushSlot(slot)
		return true
	})

	h.logger.Info("Geyser 流处理器已停止")
}
