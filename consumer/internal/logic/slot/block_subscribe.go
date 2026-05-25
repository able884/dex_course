package slot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/consumer/internal/svc"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BlockData WebSocket 推送的区块数据（包含区块和 slot）
type BlockData struct {
	Block *client.Block
	Slot  uint64
}

// BlockSubscribeService WebSocket 区块订阅服务
// 使用 slotSubscribe + RPC getBlock 方式获取区块数据
type BlockSubscribeService struct {
	Conn   *websocket.Conn
	sc     *svc.ServiceContext
	logger logx.Logger

	ctx    context.Context
	cancel func(err error)

	// 区块数据通道
	blockChan chan *BlockData

	// RPC 客户端和相关组件
	rpcClient       *client.Client
	failureRecorder *FailureRecorder
	failedSlotLogger *FailedSlotLogger // 失败 slot 日志记录器

	// RPC 重试配置（处理区块未确认的情况）
	rpcRetryMaxAttempts int             // 最大重试次数
	rpcRetryDelays      []time.Duration // 指数退避延迟序列
}

// BlockSubscribeConfig 配置
type BlockSubscribeConfig struct {
	RPCRetryMaxAttempts int             // 默认 5
	RPCRetryDelays      []time.Duration // 指数退避: [100ms, 300ms, 1s, 3s, 10s]
	FailedSlotLogDir    string          // 失败 slot 日志目录
}

// NewBlockSubscribeService 创建区块订阅服务
func NewBlockSubscribeService(
	sc *svc.ServiceContext,
	blockChan chan *BlockData,
	rpcClient *client.Client,
	failureRecorder *FailureRecorder,
	config BlockSubscribeConfig,
) *BlockSubscribeService {
	ctx, cancel := context.WithCancelCause(context.Background())

	// 默认重试配置
	if config.RPCRetryMaxAttempts == 0 {
		config.RPCRetryMaxAttempts = 5
	}
	if len(config.RPCRetryDelays) == 0 {
		config.RPCRetryDelays = []time.Duration{
			100 * time.Millisecond,
			300 * time.Millisecond,
			1 * time.Second,
			3 * time.Second,
			10 * time.Second,
		}
	}
	if config.FailedSlotLogDir == "" {
		config.FailedSlotLogDir = "./logs/failed_slots/"
	}

	// 创建失败 slot 日志记录器
	failedSlotLogger := NewFailedSlotLogger(config.FailedSlotLogDir)

	return &BlockSubscribeService{
		sc:                  sc,
		logger:              logx.WithContext(context.Background()).WithFields(logx.Field("service", "block_subscribe")),
		ctx:                 ctx,
		cancel:              cancel,
		blockChan:           blockChan,
		rpcClient:           rpcClient,
		failureRecorder:     failureRecorder,
		failedSlotLogger:    failedSlotLogger,
		rpcRetryMaxAttempts: config.RPCRetryMaxAttempts,
		rpcRetryDelays:      config.RPCRetryDelays,
	}
}

// Start 启动服务
func (s *BlockSubscribeService) Start() {
	proc.AddShutdownListener(func() {
		s.logger.Info("BlockSubscribeService:ShutdownListener")
		s.cancel(errors.New("shutdown block subscribe service"))
	})
	s.StartBlockSubscribe()
}

// Stop 停止服务
func (s *BlockSubscribeService) Stop() {
	s.logger.Info("stop block subscribe service")
	s.cancel(errors.New("service stopped"))
	if s.Conn != nil {
		_ = s.Conn.Close()
	}
	// 关闭失败 slot 日志文件
	if s.failedSlotLogger != nil {
		_ = s.failedSlotLogger.Close()
	}
}

// StartBlockSubscribe 开始订阅区块
func (s *BlockSubscribeService) StartBlockSubscribe() {
	s.MustConnect()

	threading.GoSafe(func() {
		for {
			select {
			case <-s.ctx.Done():
				s.logger.Info("blockSubscribe stopped")
				return
			default:
			}
			s.ReadSlotMessage()
		}
	})
}

// ReadSlotMessage 读取 slot 消息
// 改用 slotSubscribe，收到 slot 后通过 RPC 获取区块
func (s *BlockSubscribeService) ReadSlotMessage() {
	defer func() {
		if cause := recover(); cause != nil {
			s.logger.Errorf("ReadSlotMessage panic: %v", cause)
			s.MustConnect()
		}
	}()

	_, message, err := s.Conn.ReadMessage()
	if err != nil {
		if strings.Contains(err.Error(), "close") || strings.Contains(err.Error(), "broken pipe") {
			s.logger.Info("WebSocket connection closed, reconnecting...")
			s.MustConnect()
		} else {
			s.logger.Errorf("ReadMessage error: %v", err)
		}
		return
	}

	// 解析 WebSocket 响应
	var wsResp SlotSubscribeResponse
	if err := json.Unmarshal(message, &wsResp); err != nil {
		s.logger.Errorf("json.Unmarshal error: %v, message preview: %s", err, string(message[:min(200, len(message))]))
		return
	}

	// 检查是否是订阅成功的响应
	if wsResp.Result != 0 {
		s.logger.Infof("SlotSubscribe subscription confirmed: id=%d", wsResp.Result)
		return
	}

	// 获取 slot 信息
	slot := wsResp.Params.Result.Slot
	if slot == 0 {
		return
	}

	// 通过 RPC 获取区块数据（带重试机制）
	block, err := s.getBlockBySlotWithRetry(slot)
	if err != nil {
		// 判断错误类型
		if isSlotSkippedError(err) {
			// Slot 被跳过，不需要处理
			s.logger.Infof("Slot %d was skipped by consensus, ignoring", slot)
			return
		}

		if isBlockNotAvailableError(err) {
			// 区块暂时不可用（重试多次后仍失败），记录到失败队列和日志文件
			s.logger.Infof("Slot %d block not available after retries, recording to failed queue", slot)
			if recErr := s.failureRecorder.RecordFailure(slot); recErr != nil {
				s.logger.Errorf("Failed to record slot %d to failure queue: %v", slot, recErr)
			}
			// 记录到日志文件
			_ = s.failedSlotLogger.LogFailedSlot(slot, "block_not_available", err.Error(), s.rpcRetryMaxAttempts, "websocket")
			return
		}

		// 其他错误，也记录到失败队列和日志文件
		s.logger.Errorf("Failed to get block for slot %d: %v, recording to failed queue", slot, err)
		if recErr := s.failureRecorder.RecordFailure(slot); recErr != nil {
			s.logger.Errorf("Failed to record slot %d to failure queue: %v", slot, recErr)
		}
		// 记录到日志文件
		_ = s.failedSlotLogger.LogFailedSlot(slot, "rpc_error", err.Error(), s.rpcRetryMaxAttempts, "websocket")
		return
	}

	// 检查区块数据有效性
	if block.BlockHeight == nil || block.BlockTime == nil {
		s.logger.Infof("Block data incomplete for slot %d (missing height or time), skipping", slot)
		return
	}

	// 将区块数据（包含 block 和 slot）发送到处理通道
	blockData := &BlockData{
		Block: block,
		Slot:  slot,
	}

	select {
	case s.blockChan <- blockData:
	case <-s.ctx.Done():
		return
	default:
		s.logger.Infof("Block channel full, dropping block: slot=%d", slot)
	}
}

// getBlockBySlotWithRetry 通过 RPC 获取指定 slot 的区块（带指数退避重试）
// 处理区块可能还未被 confirmed 的情况
func (s *BlockSubscribeService) getBlockBySlotWithRetry(slot uint64) (*client.Block, error) {
	// RPC 配置
	cfg := client.GetBlockConfig{
		Commitment:         "confirmed", // 使用 confirmed 而不是 finalized，减少延迟
		TransactionDetails: "full",
	}

	var lastErr error

	// 重试循环（指数退避）
	for attempt := 1; attempt <= s.rpcRetryMaxAttempts; attempt++ {
		// 创建带超时的 context
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)

		startTime := time.Now()
		blockResponse, err := s.rpcClient.GetBlockWithConfig(ctx, slot, cfg)
		elapsed := time.Since(startTime)

		cancel() // 释放 context 资源

		if err == nil {
			// 成功获取
			if blockResponse == nil {
				return nil, fmt.Errorf("block response is nil")
			}

			if attempt > 1 {
				s.logger.Infof("✅ Slot %d RPC succeeded on attempt %d/%d after %v", slot, attempt, s.rpcRetryMaxAttempts, elapsed)
			}

			return blockResponse, nil
		}

		// 记录错误
		lastErr = err

		// 判断是否为 "Block not available" 错误（区块未确认）
		if isBlockNotAvailableError(err) {
			// 如果还有重试机会，等待后重试（指数退避）
			if attempt < s.rpcRetryMaxAttempts {
				delay := s.getRetryDelay(attempt)
				s.logger.Infof("⏳ Slot %d block not available (attempt %d/%d), retrying in %v...", slot, attempt, s.rpcRetryMaxAttempts, delay)

				select {
				case <-time.After(delay):
					continue
				case <-s.ctx.Done():
					return nil, errors.New("context cancelled during retry")
				}
			} else {
				// 最后一次尝试失败
				s.logger.Infof("❌ Slot %d block not available after %d attempts", slot, s.rpcRetryMaxAttempts)
			}
		} else if isSlotSkippedError(err) {
			// Slot 被跳过，不重试
			return nil, err
		} else {
			// 其他类型的错误，不重试
			s.logger.Errorf("RPC GetBlock failed: slot=%d, error=%v", slot, err)
			break
		}
	}

	return nil, fmt.Errorf("RPC GetBlock failed after %d attempts: %w", s.rpcRetryMaxAttempts, lastErr)
}

// getRetryDelay 获取重试延迟（指数退避）
func (s *BlockSubscribeService) getRetryDelay(attempt int) time.Duration {
	// attempt 从 1 开始，索引从 0 开始
	idx := attempt - 1
	if idx < len(s.rpcRetryDelays) {
		return s.rpcRetryDelays[idx]
	}
	// 如果超出配置的延迟数组，使用最后一个延迟
	return s.rpcRetryDelays[len(s.rpcRetryDelays)-1]
}

// MustConnect 建立 WebSocket 连接并订阅
func (s *BlockSubscribeService) MustConnect() {
	dialer := websocket.DefaultDialer

	for {
		s.logger.Infof("Connecting to WebSocket: %v", s.sc.Config.Sol.WSUrl)
		dialer.HandshakeTimeout = time.Second * 5

		c, _, err := dialer.Dial(s.sc.Config.Sol.WSUrl, nil)
		if err != nil {
			s.logger.Errorf("WebSocket dial failed: %v, retrying...", err)
			time.Sleep(1 * time.Second)
			continue
		}

		s.Conn = c

		// 构建 slotSubscribe 请求
		subscribeMsg := SlotSubscribeRequest{
			Jsonrpc: "2.0",
			ID:      1,
			Method:  "slotSubscribe",
			Params:  []interface{}{}, // slotSubscribe 不需要参数
		}

		msgBytes, err := json.Marshal(subscribeMsg)
		if err != nil {
			s.logger.Errorf("Marshal subscribe message error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// 发送订阅请求
		for i := 0; i < 10; i++ {
			err = c.WriteMessage(websocket.TextMessage, msgBytes)
			if err != nil {
				s.logger.Errorf("Send slotSubscribe request failed: %v", err)
				time.Sleep(1 * time.Second)
			} else {
				s.logger.Info("SlotSubscribe request sent, waiting for slots...")
				return
			}
		}

		// 如果发送失败，关闭连接重试
		_ = c.Close()
		time.Sleep(1 * time.Second)
	}
}

// SlotSubscribeRequest slotSubscribe 订阅请求
type SlotSubscribeRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// SlotSubscribeResponse slotSubscribe 响应
// 与 websocket.go 中的 SlotResp 结构一致
type SlotSubscribeResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  int    `json:"result,omitempty"` // 订阅成功时的 subscription ID
	Params  struct {
		Result struct {
			Slot   uint64 `json:"slot"`
			Parent uint64 `json:"parent"`
			Root   uint64 `json:"root"`
		} `json:"result"`
		Subscription int `json:"subscription"`
	} `json:"params,omitempty"`
}
