package config

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf

	// Solana 配置
	Sol SolConfig

	// MySQL 配置
	Mysql MysqlConfig

	// 数据库连接字符串
	DSN string

	// Redis 缓存配置(可选)
	Cache cache.CacheConf `json:",optional"`

	// 撮合引擎配置
	Matcher MatcherConfig

	// 清理服务配置
	Cranker CrankerConfig

	// WebSocket 配置
	WebSocket WebSocketConfig

	// 依赖的其他服务
	TradeService  zrpc.RpcClientConf `json:",optional"`
	MarketService zrpc.RpcClientConf `json:",optional"`
}

// Solana 配置
type SolConfig struct {
	ChainId           int64    `json:",default=100000"`
	Jito              string   `json:",optional"`
	NodeUrl           []string `json:",optional"`
	Enable            bool     `json:",default=true"`
	UUID              string   `json:",optional"`
	MatcherPrivateKey string   `json:",optional"` // Base58 编码的 matcher 私钥
}

// MySQL 配置
type MysqlConfig struct {
	User     string `json:",default=root"`
	Password string `json:",default=123456"`
	Host     string `json:",default=localhost"`
	Port     int    `json:",default=3306"`
	DBname   string `json:",default=rc_dex_study"`
}

// 撮合引擎配置
type MatcherConfig struct {
	Enable          bool  `json:",default=true"`
	WorkerPoolSize  int   `json:",default=10"`
	OrderBookDepth  int   `json:",default=100"`
	MatchInterval   int64 `json:",default=1000"`    // ms
	EventBufferSize int   `json:",default=1000"`
}

// 清理服务配置
type CrankerConfig struct {
	Enable            bool  `json:",default=true"`
	ScanInterval      int64 `json:",default=60000"`  // ms
	BatchSize         int   `json:",default=50"`
	ExpiredOrderLimit int   `json:",default=100"`
}

// WebSocket 配置
type WebSocketConfig struct {
	Enable            bool   `json:",default=true"`
	Url               string `json:",optional"`
	ReconnectInterval int64  `json:",default=5000"` // ms
}
