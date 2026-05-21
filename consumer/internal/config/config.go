package config

import (
	"fmt"

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
	// MigrationConfigIndex selects the Raydium CPMM fee tier (config_index). Use -1 to fall back to default.
	MigrationConfigIndex int32 `json:"MigrationConfigIndex,optional" json:",env=PUMP_MIGRATION_CONFIG_INDEX"`
	// MigrationTarget: "raydium_cpmm" or "pump_amm"
	MigrationTarget string `json:"MigrationTarget,optional" json:",env=PUMP_MIGRATION_TARGET"`
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
	return nil
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
