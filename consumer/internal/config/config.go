package config

import (
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	constants "richcode.cc/dex/pkg/constants"
)

var Cfg Config

var (
	SolRpcUseFrequency int
)

type Config struct {
	zrpc.RpcServerConf

	MySQLConfig   MySQLConfig        `json:"Mysql"`
	MarketService zrpc.RpcClientConf `json:"market_service"`
	TradeService  zrpc.RpcClientConf `json:"trade_service,optional"`
	Sol           Chain              `json:"Sol,optional"`

	Consumer      Consumer            `json:"Consumer,optional"`
	BlockPipeline BlockPipelineConfig `json:"BlockPipeline,optional"`

	// 消息队列配置
	RocketMQ RocketMQConfig `json:"RocketMQ,optional"`

	// 交易过滤配置
	BlockFilter BlockFilterConfig `json:"BlockFilter,optional"`

	// 区块处理器配置
	BlockProcessor BlockProcessorConfig `json:"BlockProcessor,optional"`

	// Latest Slot 追踪配置
	LatestSlotTracker LatestSlotTrackerConfig `json:"LatestSlotTracker,optional"`

	// 失败恢复配置
	Recovery RecoveryConfig `json:"Recovery,optional"`

	// 启动回补配置
	StartupBackfill StartupBackfillConfig `json:"StartupBackfill,optional"`

	// 失败 Slot 日志配置
	FailedSlotLog FailedSlotLogConfig `json:"FailedSlotLog,optional"`

	// Geyser gRPC 配置
	Geyser GeyserConfig `json:"Geyser,optional"`
}

type MySQLConfig struct {
	User     string `json:"User"     json:",env=MYSQL_USER"`
	Password string `json:"Password" json:",env=MYSQL_PASSWORD"`
	Host     string `json:"Host"     json:",env=MYSQL_HOST"`
	Port     int    `json:"Port"     json:",env=MYSQL_PORT"`
	DBName   string `json:"DBname"   json:",env=MYSQL_DBNAME"`
}
type Consumer struct {
	Concurrency             int    `json:"Concurrency" json:",env=CONSUMER_CONCURRENCY"`
	NotCompletedConcurrency int    `json:"NotCompletedConcurrency" json:",env=CONSUMER_NOTCOMPLETED_CONCURRENCY"`
	MigrationWallet         string `json:"MigrationWallet,optional" json:",env=PUMP_MIGRATION_WALLET"`
	MigrationConfigIndex    int32  `json:"MigrationConfigIndex,optional" json:",env=PUMP_MIGRATION_CONFIG_INDEX"`
	MigrationTarget         string `json:"MigrationTarget,optional" json:",env=PUMP_MIGRATION_TARGET"`
}

type Chain struct {
	ChainId    int64    `json:"ChainId"              json:",env=SOL_CHAINID"`
	NodeUrl    []string `json:"NodeUrl"              json:",env=SOL_NODEURL"`
	MEVNodeUrl string   `json:"MevNodeUrl,optional"  json:",env=SOL_MEVNODEURL"`
	WSUrl      string   `json:"WSUrl,optional"       json:",env=SOL_WSURL"`
	StartBlock uint64   `json:"StartBlock,optional"  json:",env=SOL_STARTBLOCK"`
}

type BlockPipelineConfig struct {
	PairBatchConcurrency  int                  `json:"PairBatchConcurrency"  json:",env=PAIR_BATCH_CONCURRENCY"`
	PairBatchRetryLimits  PairBatchRetryLimits `json:"PairBatchRetryLimits"`
	MetadataCacheTTLHours int                  `json:"MetadataCacheTTLHours" json:",env=METADATA_CACHE_TTL_HOURS"`
	MarketPushTimeoutMs   int                  `json:"MarketPushTimeoutMs"   json:",env=MARKET_PUSH_TIMEOUT_MS"`
	Metrics               BlockPipelineMetrics `json:"Metrics"`
}

type PairBatchRetryLimits struct {
	MaxAttempts    int   `json:"MaxAttempts"    json:",env=PAIR_BATCH_MAX_ATTEMPTS"`
	WindowMinutes  int   `json:"WindowMinutes"  json:",env=PAIR_BATCH_WINDOW_MINUTES"`
	BackoffSeconds []int `json:"BackoffSeconds"`
}

type BlockPipelineMetrics struct {
	Namespace         string `json:"Namespace"         json:",env=BLOCK_METRICS_NAMESPACE"`
	EnableVerboseLogs bool   `json:"EnableVerboseLogs" json:",env=BLOCK_METRICS_VERBOSE"`
}

const (
	defaultPairBatchConcurrency  = 8
	defaultMaxAttempts           = 3
	defaultRetryWindowMinutes    = 5
	defaultMetadataCacheTTLHours = 24
	defaultMarketPushTimeoutMs   = 2000
	defaultMetricsNamespace      = "block_persistence"
)

var defaultBackoffSeconds = []int{5, 30, 120}

func SaveConf(cf Config) {
	if err := cf.normalize(); err != nil {
		logx.Errorf("invalid configuration: %v", err)
	}
	Cfg = cf
}

func FindChainRpcByChainId(chainId int) (rpc string) {
	var rpcs []string
	var useFrequency *int

	switch chainId {
	case constants.SolChainIdInt:
		rpcs = Cfg.Sol.NodeUrl
		useFrequency = &SolRpcUseFrequency
	default:
		logx.Error("No Rpc Config")
		return
	}

	if len(rpcs) == 0 {
		logx.Error("No Rpc Config")
		return
	}

	*useFrequency++
	index := *useFrequency % len(rpcs)
	rpc = rpcs[index]
	return
}

func (cfg *Config) normalize() error {
	if err := cfg.BlockPipeline.normalize(); err != nil {
		return err
	}
	cfg.Consumer.normalize()

	// RocketMQ 配置验证
	if err := cfg.RocketMQ.normalize(); err != nil {
		return fmt.Errorf("rocketmq config error: %w", err)
	}

	// 交易过滤配置验证
	if err := cfg.BlockFilter.normalize(); err != nil {
		return fmt.Errorf("block filter config error: %w", err)
	}

	// 区块处理器配置验证
	if err := cfg.BlockProcessor.normalize(); err != nil {
		return fmt.Errorf("block processor config error: %w", err)
	}

	// Latest Slot 追踪配置验证
	if err := cfg.LatestSlotTracker.normalize(); err != nil {
		return fmt.Errorf("latest slot tracker config error: %w", err)
	}

	// 失败恢复配置验证
	if err := cfg.Recovery.normalize(); err != nil {
		return fmt.Errorf("recovery config error: %w", err)
	}

	// 启动回补配置验证
	if err := cfg.StartupBackfill.normalize(); err != nil {
		return fmt.Errorf("startup backfill config error: %w", err)
	}

	// 失败 Slot 日志配置验证
	if err := cfg.FailedSlotLog.normalize(); err != nil {
		return fmt.Errorf("failed slot log config error: %w", err)
	}

	// Geyser 配置验证
	if err := cfg.Geyser.normalize(); err != nil {
		return fmt.Errorf("geyser config error: %w", err)
	}

	return nil
}

func (c *Consumer) normalize() {
	if c.MigrationTarget == "" {
		c.MigrationTarget = "raydium_cpmm"
	}
	if c.MigrationConfigIndex == 0 && c.MigrationTarget == "raydium_cpmm" {
		// Leave 0 untouched because 0 is a valid fee tier; only set default when negative.
	}
	if c.MigrationConfigIndex < 0 {
		c.MigrationConfigIndex = 2
	}
}

func (cfg *BlockPipelineConfig) normalize() error {
	if cfg.PairBatchConcurrency < 0 {
		return fmt.Errorf("block pipeline pair batch concurrency must be non-negative")
	}
	if cfg.PairBatchConcurrency == 0 {
		cfg.PairBatchConcurrency = defaultPairBatchConcurrency
	}
	if err := cfg.PairBatchRetryLimits.normalize(); err != nil {
		return err
	}
	if cfg.MetadataCacheTTLHours < 0 {
		return fmt.Errorf("metadata cache TTL hours must be non-negative")
	}
	if cfg.MetadataCacheTTLHours == 0 {
		cfg.MetadataCacheTTLHours = defaultMetadataCacheTTLHours
	}
	if cfg.MarketPushTimeoutMs < 0 {
		return fmt.Errorf("market push timeout ms must be non-negative")
	}
	if cfg.MarketPushTimeoutMs == 0 {
		cfg.MarketPushTimeoutMs = defaultMarketPushTimeoutMs
	}
	if cfg.Metrics.Namespace == "" {
		cfg.Metrics.Namespace = defaultMetricsNamespace
	}
	return nil
}

func (limits *PairBatchRetryLimits) normalize() error {
	if limits.MaxAttempts < 0 {
		return fmt.Errorf("pair batch max attempts must be non-negative")
	}
	if limits.MaxAttempts == 0 {
		limits.MaxAttempts = defaultMaxAttempts
	}
	if limits.WindowMinutes < 0 {
		return fmt.Errorf("pair batch retry window minutes must be non-negative")
	}
	if limits.WindowMinutes == 0 {
		limits.WindowMinutes = defaultRetryWindowMinutes
	}
	if len(limits.BackoffSeconds) == 0 {
		limits.BackoffSeconds = append([]int(nil), defaultBackoffSeconds...)
	}
	for i, v := range limits.BackoffSeconds {
		if v <= 0 {
			return fmt.Errorf("pair batch backoff seconds must be positive (index %d)", i)
		}
	}
	return nil
}

// RocketMQConfig RocketMQ 配置
type RocketMQConfig struct {
	// NameServers RocketMQ NameServer 地址列表
	NameServers []string `json:"NameServers" json:",env=ROCKETMQ_NAME_SERVERS"`
	// AccessKey 访问密钥（可选，启用 ACL 时使用）
	AccessKey string `json:"AccessKey,optional" json:",env=ROCKETMQ_ACCESS_KEY"`
	// SecretKey 密钥（可选，启用 ACL 时使用）
	SecretKey string `json:"SecretKey,optional" json:",env=ROCKETMQ_SECRET_KEY"`
	// MaxMessageSize 最大消息大小（字节）
	MaxMessageSize int `json:"MaxMessageSize" json:",default=16777216"` // 16MB
	// Topics 主题配置
	Topics RocketMQTopics `json:"Topics"`
	// Producer 生产者配置
	Producer RocketMQProducerConfig `json:"Producer,optional"`
	// Consumer 消费者配置
	Consumer RocketMQConsumerConfig `json:"Consumer,optional"`
}

// RocketMQTopics RocketMQ 主题配置
type RocketMQTopics struct {
	// Blocks 区块消息主题
	Blocks string `json:"Blocks" json:",default=solana-blocks"`
	// Retry 重试消息主题
	Retry string `json:"Retry" json:",default=solana-blocks-retry"`
	// Migration Pump 迁移任务主题
	Migration string `json:"Migration" json:",default=pump-migration"`
}

// RocketMQProducerConfig RocketMQ 生产者配置
type RocketMQProducerConfig struct {
	// SendTimeout 发送超时时间
	SendTimeout time.Duration `json:"SendTimeout" json:",default=3s"`
	// RetryTimes 重试次数
	RetryTimes int `json:"RetryTimes" json:",default=2"`
	// CompressLevel 压缩级别（0-9，0=不压缩）
	CompressLevel int `json:"CompressLevel,optional"`
}

// RocketMQConsumerConfig RocketMQ 消费者配置
type RocketMQConsumerConfig struct {
	// GroupName 消费者组名称
	GroupName string `json:"GroupName" json:",default=block-consumer-group"`
	// ConsumeFromWhere 从哪里开始消费
	ConsumeFromWhere string `json:"ConsumeFromWhere" json:",default=CONSUME_FROM_LAST_OFFSET"`
	// MessageModel 消息模型（CLUSTERING 集群模式 / BROADCASTING 广播模式）
	MessageModel string `json:"MessageModel" json:",default=CLUSTERING"`
	// MaxReconsumeTimes 最大重试次数
	MaxReconsumeTimes int32 `json:"MaxReconsumeTimes" json:",default=3"`
	// ConsumeTimeout 消费超时时间
	ConsumeTimeout time.Duration `json:"ConsumeTimeout" json:",default=15m"`
}

func (cfg *RocketMQConfig) normalize() error {
	if len(cfg.NameServers) == 0 {
		return fmt.Errorf("rocketmq name servers cannot be empty")
	}
	// 设置最大消息大小默认值 16MB（支持大消息）
	if cfg.MaxMessageSize == 0 {
		cfg.MaxMessageSize = 16 * 1024 * 1024 // 16MB
	}
	if cfg.Topics.Blocks == "" {
		cfg.Topics.Blocks = "solana-blocks"
	}
	if cfg.Topics.Retry == "" {
		cfg.Topics.Retry = "solana-blocks-retry"
	}
	if cfg.Topics.Migration == "" {
		cfg.Topics.Migration = "pump-migration"
	}
	if cfg.Producer.SendTimeout == 0 {
		cfg.Producer.SendTimeout = 3 * time.Second
	}
	if cfg.Producer.RetryTimes == 0 {
		cfg.Producer.RetryTimes = 2
	}
	if cfg.Consumer.GroupName == "" {
		cfg.Consumer.GroupName = "block-consumer-group"
	}
	if cfg.Consumer.ConsumeFromWhere == "" {
		cfg.Consumer.ConsumeFromWhere = "CONSUME_FROM_LAST_OFFSET"
	}
	if cfg.Consumer.MessageModel == "" {
		cfg.Consumer.MessageModel = "CLUSTERING"
	}
	if cfg.Consumer.MaxReconsumeTimes == 0 {
		cfg.Consumer.MaxReconsumeTimes = 3
	}
	if cfg.Consumer.ConsumeTimeout == 0 {
		cfg.Consumer.ConsumeTimeout = 15 * time.Minute
	}
	return nil
}

// BlockFilterConfig 区块交易过滤配置
type BlockFilterConfig struct {
	// Enabled 是否启用过滤（默认启用）
	Enabled bool `json:"Enabled" json:",default=true"`
	// TargetProtocols 目标协议列表
	TargetProtocols []ProtocolFilterRule `json:"TargetProtocols"`
}

// ProtocolFilterRule 协议过滤规则
type ProtocolFilterRule struct {
	// Name 协议名称（用于日志和统计）
	Name string `json:"Name"`
	// ProgramID 程序 ID
	ProgramID string `json:"ProgramID"`
	// Instructions 指令过滤规则（可选，为空则匹配所有指令）
	Instructions []InstructionFilterRule `json:"Instructions,optional"`
}

// InstructionFilterRule 指令过滤规则
type InstructionFilterRule struct {
	// Name 指令名称（用于日志和统计）
	Name string `json:"Name"`
	// Discriminator 指令鉴别符（十六进制字符串，如 "66063d1201daebea"）
	Discriminator string `json:"Discriminator"`
}

func (cfg *BlockFilterConfig) normalize() error {
	// 默认启用过滤
	if !cfg.Enabled {
		return nil
	}

	// 如果没有配置任何协议，使用默认协议
	if len(cfg.TargetProtocols) == 0 {
		cfg.TargetProtocols = getDefaultProtocolRules()
	}

	// 验证配置
	for i, rule := range cfg.TargetProtocols {
		if rule.Name == "" {
			return fmt.Errorf("protocol rule %d: name cannot be empty", i)
		}
		if rule.ProgramID == "" {
			return fmt.Errorf("protocol rule %s: program ID cannot be empty", rule.Name)
		}

		// 验证指令鉴别符格式
		for j, inst := range rule.Instructions {
			if inst.Name == "" {
				return fmt.Errorf("protocol %s instruction %d: name cannot be empty", rule.Name, j)
			}
			if inst.Discriminator == "" {
				return fmt.Errorf("protocol %s instruction %s: discriminator cannot be empty", rule.Name, inst.Name)
			}
			// 验证是否为有效的十六进制字符串
			if len(inst.Discriminator)%2 != 0 {
				return fmt.Errorf("protocol %s instruction %s: discriminator must be even-length hex string", rule.Name, inst.Name)
			}
		}
	}

	return nil
}

// getDefaultProtocolRules 获取默认的协议过滤规则
func getDefaultProtocolRules() []ProtocolFilterRule {
	return []ProtocolFilterRule{
		{
			Name:      "PumpFun",
			ProgramID: "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P",
			// 不指定指令列表，表示匹配所有 PumpFun 交易
		},
		{
			Name:      "PumpSwap",
			ProgramID: "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA",
		},
		{
			Name:      "Raydium_CLMM",
			ProgramID: "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK",
		},
		{
			Name:      "Raydium_CPMM",
			ProgramID: "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C",
		},
		{
			Name:      "SPL_Token",
			ProgramID: "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
			// 可以选择只过滤特定指令，例如：
			// Instructions: []InstructionFilterRule{
			// 	{Name: "Transfer", Discriminator: "03"},
			// 	{Name: "MintTo", Discriminator: "07"},
			// },
		},
		{
			Name:      "SPL_Token_2022",
			ProgramID: "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb",
		},
	}
}

// BlockProcessorConfig 区块处理器性能配置
type BlockProcessorConfig struct {
	// SlotChannelSize Block 缓冲区大小（默认 500）
	SlotChannelSize int `json:"SlotChannelSize" json:",default=500"`
	// WorkerCount 并发 Worker 数量（默认 5）
	WorkerCount int `json:"WorkerCount" json:",default=5"`
	// MaxConcurrentRPC 最大并发 RPC 请求数（默认 8）（仅用于 RecoveryManager）
	MaxConcurrentRPC int `json:"MaxConcurrentRPC" json:",default=8"`
	// RPCDelayMs RPC 请求之间的延迟（毫秒，默认 100ms）（仅用于 RecoveryManager）
	RPCDelayMs int `json:"RPCDelayMs" json:",default=100"`
	// BlockNotAvailableRetry "Block not available" 错误重试配置（仅用于 RecoveryManager）
	BlockNotAvailableRetry BlockNotAvailableRetryConfig `json:"BlockNotAvailableRetry"`
}

// BlockNotAvailableRetryConfig "Block not available" 错误重试配置
type BlockNotAvailableRetryConfig struct {
	// MaxAttempts 最大重试次数（默认 3）
	MaxAttempts int `json:"MaxAttempts" json:",default=3"`
	// RetryDelayMs 每次重试间隔（毫秒，默认 1000ms）
	RetryDelayMs int `json:"RetryDelayMs" json:",default=1000"`
}

func (cfg *BlockProcessorConfig) normalize() error {
	// 设置默认值
	if cfg.SlotChannelSize <= 0 {
		cfg.SlotChannelSize = 500
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 5
	}
	if cfg.MaxConcurrentRPC <= 0 {
		cfg.MaxConcurrentRPC = 8
	}
	if cfg.RPCDelayMs <= 0 {
		cfg.RPCDelayMs = 100
	}
	if cfg.BlockNotAvailableRetry.MaxAttempts <= 0 {
		cfg.BlockNotAvailableRetry.MaxAttempts = 3
	}
	if cfg.BlockNotAvailableRetry.RetryDelayMs <= 0 {
		cfg.BlockNotAvailableRetry.RetryDelayMs = 1000
	}

	// 验证范围
	if cfg.SlotChannelSize < 100 {
		return fmt.Errorf("block channel size must be at least 100, got %d", cfg.SlotChannelSize)
	}
	if cfg.SlotChannelSize > 10000 {
		return fmt.Errorf("block channel size must be at most 10000, got %d", cfg.SlotChannelSize)
	}
	if cfg.WorkerCount < 1 {
		return fmt.Errorf("worker count must be at least 1, got %d", cfg.WorkerCount)
	}
	if cfg.WorkerCount > 100 {
		return fmt.Errorf("worker count must be at most 100, got %d", cfg.WorkerCount)
	}
	if cfg.MaxConcurrentRPC < cfg.WorkerCount {
		return fmt.Errorf("max concurrent RPC (%d) should be at least equal to worker count (%d)", cfg.MaxConcurrentRPC, cfg.WorkerCount)
	}
	if cfg.MaxConcurrentRPC > 500 {
		return fmt.Errorf("max concurrent RPC must be at most 500, got %d", cfg.MaxConcurrentRPC)
	}
	if cfg.RPCDelayMs < 0 {
		return fmt.Errorf("rpc delay must be non-negative, got %d", cfg.RPCDelayMs)
	}
	if cfg.RPCDelayMs > 5000 {
		return fmt.Errorf("rpc delay must be at most 5000ms, got %d", cfg.RPCDelayMs)
	}
	if cfg.BlockNotAvailableRetry.MaxAttempts < 1 {
		return fmt.Errorf("block not available retry max attempts must be at least 1, got %d", cfg.BlockNotAvailableRetry.MaxAttempts)
	}
	if cfg.BlockNotAvailableRetry.MaxAttempts > 10 {
		return fmt.Errorf("block not available retry max attempts must be at most 10, got %d", cfg.BlockNotAvailableRetry.MaxAttempts)
	}
	if cfg.BlockNotAvailableRetry.RetryDelayMs < 100 {
		return fmt.Errorf("block not available retry delay must be at least 100ms, got %d", cfg.BlockNotAvailableRetry.RetryDelayMs)
	}
	if cfg.BlockNotAvailableRetry.RetryDelayMs > 10000 {
		return fmt.Errorf("block not available retry delay must be at most 10000ms, got %d", cfg.BlockNotAvailableRetry.RetryDelayMs)
	}

	return nil
}

// LatestSlotTrackerConfig Latest Slot 追踪配置
type LatestSlotTrackerConfig struct {
	// SyncInterval 同步到 Redis 的间隔（默认 5s）
	SyncInterval time.Duration `json:"SyncInterval" json:",default=5s"`
}

func (cfg *LatestSlotTrackerConfig) normalize() error {
	// 设置默认值
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 5 * time.Second
	}

	// 验证范围
	if cfg.SyncInterval < 1*time.Second {
		return fmt.Errorf("sync interval must be at least 1s, got %s", cfg.SyncInterval)
	}
	if cfg.SyncInterval > 60*time.Second {
		return fmt.Errorf("sync interval must be at most 60s, got %s", cfg.SyncInterval)
	}

	return nil
}

// RecoveryConfig 失败恢复配置
type RecoveryConfig struct {
	// Enabled 是否启用恢复服务（默认 true）
	Enabled bool `json:"Enabled" json:",default=true"`
	// Interval 恢复检查间隔（默认 5s）
	Interval time.Duration `json:"Interval" json:",default=5s"`
	// BatchSize 每批处理的失败 slot 数量（默认 50）
	BatchSize int `json:"BatchSize" json:",default=50"`
	// MaxRetryCount 最大重试次数（默认 3）
	MaxRetryCount int `json:"MaxRetryCount" json:",default=3"`
}

func (cfg *RecoveryConfig) normalize() error {
	// 设置默认值
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxRetryCount <= 0 {
		cfg.MaxRetryCount = 3
	}

	// 验证范围
	if cfg.Interval < 1*time.Second {
		return fmt.Errorf("recovery interval must be at least 1s, got %s", cfg.Interval)
	}
	if cfg.Interval > 60*time.Second {
		return fmt.Errorf("recovery interval must be at most 60s, got %s", cfg.Interval)
	}
	if cfg.BatchSize < 10 {
		return fmt.Errorf("recovery batch size must be at least 10, got %d", cfg.BatchSize)
	}
	if cfg.BatchSize > 500 {
		return fmt.Errorf("recovery batch size must be at most 500, got %d", cfg.BatchSize)
	}
	if cfg.MaxRetryCount < 1 {
		return fmt.Errorf("max retry count must be at least 1, got %d", cfg.MaxRetryCount)
	}
	if cfg.MaxRetryCount > 10 {
		return fmt.Errorf("max retry count must be at most 10, got %d", cfg.MaxRetryCount)
	}

	return nil
}

// StartupBackfillConfig 启动回补配置
type StartupBackfillConfig struct {
	// MaxGap 最大全量回补的 gap 大小（默认 10000）
	MaxGap uint64 `json:"MaxGap" json:",default=10000"`
	// LogDir 大 gap 日志文件目录（默认 "./logs/backfill/"）
	LogDir string `json:"LogDir" json:",default=./logs/backfill/"`
}

func (cfg *StartupBackfillConfig) normalize() error {
	// 设置默认值
	if cfg.MaxGap == 0 {
		cfg.MaxGap = 10000
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "./logs/backfill/"
	}

	// 验证范围
	if cfg.MaxGap < 100 {
		return fmt.Errorf("max gap must be at least 100, got %d", cfg.MaxGap)
	}
	if cfg.MaxGap > 100000 {
		return fmt.Errorf("max gap must be at most 100000, got %d", cfg.MaxGap)
	}

	return nil
}

// FailedSlotLogConfig 失败 Slot 日志配置
type FailedSlotLogConfig struct {
	// LogDir 日志文件目录（默认 "./logs/failed_slots/"）
	LogDir string `json:"LogDir" json:",default=./logs/failed_slots/"`
}

func (cfg *FailedSlotLogConfig) normalize() error {
	// 设置默认值
	if cfg.LogDir == "" {
		cfg.LogDir = "./logs/failed_slots/"
	}

	return nil
}

// GeyserConfig Geyser gRPC 配置
type GeyserConfig struct {
	// Enabled 是否启用 Geyser 模式（默认 false）
	Enabled bool `json:"Enabled" json:",default=false"`
	// Endpoints Geyser 端点列表（支持多个备用端点）
	Endpoints []string `json:"Endpoints,optional"`
	// Commitment 承诺级别（processed/confirmed/finalized，默认 confirmed）
	Commitment string `json:"Commitment" json:",default=confirmed"`
	// EnableTLS 是否启用 TLS（默认 true）
	EnableTLS bool `json:"EnableTLS" json:",default=true"`
	// ReconnectInterval 重连间隔（默认 5s）
	ReconnectInterval time.Duration `json:"ReconnectInterval" json:",default=5s"`
	// HealthCheckInterval 健康检查间隔（默认 10s）
	HealthCheckInterval time.Duration `json:"HealthCheckInterval" json:",default=10s"`
	// ConnectionTimeout 连接超时（默认 30s）
	ConnectionTimeout time.Duration `json:"ConnectionTimeout" json:",default=30s"`
	// StreamTimeout 流超时（默认 60s）
	StreamTimeout time.Duration `json:"StreamTimeout" json:",default=60s"`
	// Filters 订阅过滤器
	Filters GeyserFiltersConfig `json:"Filters,optional"`
	// Fallback 降级配置
	Fallback GeyserFallbackConfig `json:"Fallback,optional"`
}

// GeyserFiltersConfig Geyser 过滤器配置
type GeyserFiltersConfig struct {
	// AccountInclude 包含的账户/程序 ID 列表（服务端过滤）
	AccountInclude []string `json:"AccountInclude,optional"`
}

// GeyserFallbackConfig Geyser 降级配置
type GeyserFallbackConfig struct {
	// Enabled 是否启用自动降级（默认 true）
	Enabled bool `json:"Enabled" json:",default=true"`
	// MaxConsecutiveFailures 连续失败几次触发降级（默认 3）
	MaxConsecutiveFailures int `json:"MaxConsecutiveFailures" json:",default=3"`
	// RecoveryCheckInterval 恢复检查间隔（默认 30s）
	RecoveryCheckInterval time.Duration `json:"RecoveryCheckInterval" json:",default=30s"`
	// SlotDelayThreshold slot 延迟阈值，超过此值触发降级（默认 10s）
	SlotDelayThreshold time.Duration `json:"SlotDelayThreshold" json:",default=10s"`
}

func (cfg *GeyserConfig) normalize() error {
	// 如果未启用 Geyser，跳过验证
	if !cfg.Enabled {
		return nil
	}

	// 验证端点配置
	if len(cfg.Endpoints) == 0 {
		return fmt.Errorf("geyser endpoints cannot be empty when enabled")
	}

	// 设置默认值
	if cfg.Commitment == "" {
		cfg.Commitment = "confirmed"
	}

	// 验证承诺级别
	validCommitments := map[string]bool{
		"processed":  true,
		"confirmed":  true,
		"finalized":  true,
	}
	if !validCommitments[cfg.Commitment] {
		return fmt.Errorf("invalid commitment level: %s (must be processed/confirmed/finalized)", cfg.Commitment)
	}

	// 设置超时默认值
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 5 * time.Second
	}
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = 10 * time.Second
	}
	if cfg.ConnectionTimeout <= 0 {
		cfg.ConnectionTimeout = 30 * time.Second
	}
	if cfg.StreamTimeout <= 0 {
		cfg.StreamTimeout = 60 * time.Second
	}

	// 验证超时范围
	if cfg.ReconnectInterval < 1*time.Second {
		return fmt.Errorf("reconnect interval must be at least 1s, got %s", cfg.ReconnectInterval)
	}
	if cfg.ReconnectInterval > 60*time.Second {
		return fmt.Errorf("reconnect interval must be at most 60s, got %s", cfg.ReconnectInterval)
	}
	if cfg.HealthCheckInterval < 5*time.Second {
		return fmt.Errorf("health check interval must be at least 5s, got %s", cfg.HealthCheckInterval)
	}
	if cfg.HealthCheckInterval > 300*time.Second {
		return fmt.Errorf("health check interval must be at most 300s, got %s", cfg.HealthCheckInterval)
	}
	if cfg.ConnectionTimeout < 5*time.Second {
		return fmt.Errorf("connection timeout must be at least 5s, got %s", cfg.ConnectionTimeout)
	}
	if cfg.ConnectionTimeout > 300*time.Second {
		return fmt.Errorf("connection timeout must be at most 300s, got %s", cfg.ConnectionTimeout)
	}
	if cfg.StreamTimeout < 10*time.Second {
		return fmt.Errorf("stream timeout must be at least 10s, got %s", cfg.StreamTimeout)
	}
	if cfg.StreamTimeout > 600*time.Second {
		return fmt.Errorf("stream timeout must be at most 600s, got %s", cfg.StreamTimeout)
	}

	// 验证过滤器配置
	if len(cfg.Filters.AccountInclude) == 0 {
		return fmt.Errorf("geyser filters must include at least one account/program ID")
	}

	// 验证降级配置
	if cfg.Fallback.MaxConsecutiveFailures <= 0 {
		cfg.Fallback.MaxConsecutiveFailures = 3
	}
	if cfg.Fallback.MaxConsecutiveFailures < 1 {
		return fmt.Errorf("max consecutive failures must be at least 1, got %d", cfg.Fallback.MaxConsecutiveFailures)
	}
	if cfg.Fallback.MaxConsecutiveFailures > 10 {
		return fmt.Errorf("max consecutive failures must be at most 10, got %d", cfg.Fallback.MaxConsecutiveFailures)
	}

	if cfg.Fallback.RecoveryCheckInterval <= 0 {
		cfg.Fallback.RecoveryCheckInterval = 30 * time.Second
	}
	if cfg.Fallback.RecoveryCheckInterval < 10*time.Second {
		return fmt.Errorf("recovery check interval must be at least 10s, got %s", cfg.Fallback.RecoveryCheckInterval)
	}
	if cfg.Fallback.RecoveryCheckInterval > 300*time.Second {
		return fmt.Errorf("recovery check interval must be at most 300s, got %s", cfg.Fallback.RecoveryCheckInterval)
	}

	if cfg.Fallback.SlotDelayThreshold <= 0 {
		cfg.Fallback.SlotDelayThreshold = 10 * time.Second
	}
	if cfg.Fallback.SlotDelayThreshold < 5*time.Second {
		return fmt.Errorf("slot delay threshold must be at least 5s, got %s", cfg.Fallback.SlotDelayThreshold)
	}
	if cfg.Fallback.SlotDelayThreshold > 60*time.Second {
		return fmt.Errorf("slot delay threshold must be at most 60s, got %s", cfg.Fallback.SlotDelayThreshold)
	}

	return nil
}
