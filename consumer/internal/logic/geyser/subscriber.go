package geyser

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/rpcpool/yellowstone-grpc/examples/golang/proto"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"richcode.cc/dex/consumer/internal/config"
)

// CommitmentLevel Geyser 承诺级别
type CommitmentLevel string

const (
	CommitmentProcessed CommitmentLevel = "processed" // 已处理
	CommitmentConfirmed CommitmentLevel = "confirmed" // 已确认
	CommitmentFinalized CommitmentLevel = "finalized" // 已最终确认
)

// GeyserSubscriber Geyser gRPC 订阅管理器
type GeyserSubscriber struct {
	cfg                  config.GeyserConfig
	ctx                  context.Context
	cancel               context.CancelFunc
	conn                 *grpc.ClientConn
	client               pb.GeyserClient
	stream               pb.Geyser_SubscribeClient
	logger               logx.Logger
	currentEndpointIndex int
	mu                   sync.RWMutex

	// 健康状态
	connected           atomic.Bool
	lastHealthCheck     atomic.Int64 // Unix 时间戳
	consecutiveFailures atomic.Int32 // 连续失败次数

	// 订阅状态
	subscribed atomic.Bool
}

// NewGeyserSubscriber 创建新的 Geyser 订阅器
func NewGeyserSubscriber(ctx context.Context, cfg config.GeyserConfig) *GeyserSubscriber {
	ctx, cancel := context.WithCancel(ctx)

	return &GeyserSubscriber{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: logx.WithContext(ctx).WithFields(logx.Field("component", "geyser_subscriber")),
	}
}

// Connect 建立到 Geyser 端点的 gRPC 连接
func (s *GeyserSubscriber) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected.Load() {
		s.logger.Info("已连接到 Geyser")
		return nil
	}

	// 直接在锁内获取 endpoint，避免调用 getCurrentEndpoint() 造成死锁
	var endpoint string
	if len(s.cfg.Endpoints) > 0 {
		endpoint = s.cfg.Endpoints[s.currentEndpointIndex]
	}

	// 配置 keepalive 参数
	kacp := keepalive.ClientParameters{
		Time:                10 * time.Second, // 每 10 秒发送一次 ping
		Timeout:             s.cfg.ConnectionTimeout,
		PermitWithoutStream: true,
	}

	// 构建 gRPC 拨号选项
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(kacp),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100 * 1024 * 1024), // 100MB 最大消息大小
		),
	}

	// 添加 TLS 凭证
	if s.cfg.EnableTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	// 带超时的拨号
	dialCtx, dialCancel := context.WithTimeout(s.ctx, s.cfg.ConnectionTimeout)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, endpoint, opts...)
	if err != nil {
		s.recordFailure()
		return fmt.Errorf("连接失败 %s: %w", endpoint, err)
	}

	s.conn = conn
	s.client = pb.NewGeyserClient(conn)
	s.connected.Store(true)
	s.consecutiveFailures.Store(0)
	s.lastHealthCheck.Store(time.Now().Unix())
	return nil
}

// Subscribe 创建带账户过滤器的订阅流
func (s *GeyserSubscriber) Subscribe() (pb.Geyser_SubscribeClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected.Load() {
		return nil, fmt.Errorf("未连接到 Geyser")
	}

	if s.subscribed.Load() {
		return nil, fmt.Errorf("已订阅")
	}

	// 创建订阅流
	stream, err := s.client.Subscribe(s.ctx)
	if err != nil {
		s.recordFailure()
		return nil, fmt.Errorf("创建订阅流失败: %w", err)
	}

	// 构建订阅请求
	req := s.buildSubscribeRequest()

	// 发送订阅请求
	if err := stream.Send(req); err != nil {
		s.recordFailure()
		return nil, fmt.Errorf("发送订阅请求失败: %w", err)
	}

	s.stream = stream
	s.subscribed.Store(true)
	s.logger.Info("成功订阅 Geyser 流")

	return stream, nil
}

// buildSubscribeRequest 构建带交易过滤器的订阅请求
func (s *GeyserSubscriber) buildSubscribeRequest() *pb.SubscribeRequest {
	// 映射承诺级别
	commitment := s.mapCommitmentLevel(s.cfg.Commitment)

	// 构建交易订阅过滤器（订阅包含特定 Program ID 的交易）
	transactions := make(map[string]*pb.SubscribeRequestFilterTransactions)

	// 创建过滤器的 bool 指针
	voteFilter := false
	failedFilter := false

	// 使用 "all_transactions" 作为键，订阅所有包含配置的 Program IDs 的交易
	transactions["all_transactions"] = &pb.SubscribeRequestFilterTransactions{
		Vote:            &voteFilter,                  // 排除投票交易
		Failed:          &failedFilter,                // 排除失败的交易
		AccountInclude:  s.cfg.Filters.AccountInclude, // 包含这些 Program IDs 的交易
		AccountExclude:  []string{},                   // 不排除任何账户
		AccountRequired: []string{},                   // 不强制要求特定账户
	}

	s.logger.Infof("订阅交易过滤器 - Program IDs: %v, Commitment: %s, Vote: %v, Failed: %v",
		s.cfg.Filters.AccountInclude, s.cfg.Commitment, voteFilter, failedFilter)

	return &pb.SubscribeRequest{
		Transactions: transactions,
		Commitment:   &commitment,
	}
}

// mapCommitmentLevel 将承诺级别字符串映射到 protobuf 枚举
func (s *GeyserSubscriber) mapCommitmentLevel(commitment string) pb.CommitmentLevel {
	switch commitment {
	case "processed":
		return pb.CommitmentLevel_PROCESSED
	case "confirmed":
		return pb.CommitmentLevel_CONFIRMED
	case "finalized":
		return pb.CommitmentLevel_FINALIZED
	default:
		return pb.CommitmentLevel_CONFIRMED
	}
}

// Reconnect 尝试重新连接到 Geyser
func (s *GeyserSubscriber) Reconnect() error {
	s.logger.Info("尝试重新连接到 Geyser...")

	// 关闭现有连接
	s.Close()

	// 如果配置了多个端点，尝试下一个
	if len(s.cfg.Endpoints) > 1 {
		s.mu.Lock()
		s.currentEndpointIndex = (s.currentEndpointIndex + 1) % len(s.cfg.Endpoints)
		s.mu.Unlock()
	}

	// 重连前等待
	time.Sleep(s.cfg.ReconnectInterval)

	// 重新连接
	if err := s.Connect(); err != nil {
		return err
	}

	// 重新订阅
	if _, err := s.Subscribe(); err != nil {
		return err
	}

	s.logger.Info("成功重新连接到 Geyser")
	return nil
}

// IsHealthy 检查连接是否健康
func (s *GeyserSubscriber) IsHealthy() bool {
	if !s.connected.Load() {
		return false
	}

	// 检查上次健康检查是否太久之前
	lastCheck := time.Unix(s.lastHealthCheck.Load(), 0)
	if time.Since(lastCheck) > s.cfg.HealthCheckInterval*2 {
		return false
	}

	// 检查连续失败次数
	if s.consecutiveFailures.Load() >= int32(s.cfg.Fallback.MaxConsecutiveFailures) {
		return false
	}

	return true
}

// UpdateHealthCheck 更新最后健康检查时间戳
func (s *GeyserSubscriber) UpdateHealthCheck() {
	s.lastHealthCheck.Store(time.Now().Unix())
}

// GetConsecutiveFailures 返回连续失败次数
func (s *GeyserSubscriber) GetConsecutiveFailures() int {
	return int(s.consecutiveFailures.Load())
}

// ResetFailures 重置失败计数器
func (s *GeyserSubscriber) ResetFailures() {
	s.consecutiveFailures.Store(0)
}

// recordFailure 记录连接失败
func (s *GeyserSubscriber) recordFailure() {
	failures := s.consecutiveFailures.Add(1)
	if failures == 1 || failures == 3 || failures == 5 || failures%10 == 0 {
		s.logger.Errorf("⚠️ Geyser 连接失败（连续: %d 次）", failures)
	}
}

// getCurrentEndpoint 返回当前端点
func (s *GeyserSubscriber) getCurrentEndpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.cfg.Endpoints) == 0 {
		return ""
	}

	return s.cfg.Endpoints[s.currentEndpointIndex]
}

// Close 关闭 Geyser 连接
func (s *GeyserSubscriber) Close() error {
	s.logger.Info("关闭 Geyser 订阅器...")

	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscribed.Store(false)
	s.connected.Store(false)

	if s.stream != nil {
		if err := s.stream.CloseSend(); err != nil {
			s.logger.Errorf("关闭流失败: %v", err)
		}
		s.stream = nil
	}

	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			s.logger.Errorf("关闭连接失败: %v", err)
			return err
		}
		s.conn = nil
	}

	s.client = nil

	s.logger.Info("Geyser 订阅器已关闭")
	return nil
}

// Shutdown 优雅地关闭订阅器
func (s *GeyserSubscriber) Shutdown() {
	s.logger.Info("正在关闭 Geyser 订阅器...")
	s.cancel()
	s.Close()
	s.logger.Info("Geyser 订阅器关闭完成")
}
