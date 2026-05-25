package svc

import (
	"fmt"
	"time"

	lighthouse "github.com/lighthouse-web3/lighthouse-go-sdk/lighthouse"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/trade/internal/chain/solana"
	"richcode.cc/dex/trade/internal/config"
	"richcode.cc/dex/trade/internal/metrics"
	"richcode.cc/dex/trade/internal/types"
)

type ServiceContext struct {
	Config            config.Config
	Redis             *redis.Redis
	MarketClient      market.MarketClient
	MarketTokenClient types.MarketTokenClient
	SolTxMananger     *solana.TxManager
	DB                *gorm.DB
	ClmmPositionModel solmodel.ClmmPositionModel
	IpfsGateway       string
	NFTStorageKey     string
	LhClient          *lighthouse.Client
	LiquidityCfg      config.LiquidityConfig
	LiquidityMetric   types.LiquidityMetrics
}

func NewServiceContext(c config.Config) *ServiceContext {
	// redisService := c.Redis.NewRedis()
	rds := redis.MustNewRedis(redis.RedisConf{
		Host:        c.Redis.Host,
		Type:        c.Redis.Type,
		Pass:        c.Redis.Pass,
		Tls:         c.Redis.Tls,
		PingTimeout: c.Redis.PingTimeout,
	})
	if !rds.Ping() {
		panic("rds ping err")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		c.MySQLConfig.User,
		c.MySQLConfig.Password,
		c.MySQLConfig.Host,
		c.MySQLConfig.Port,
		c.MySQLConfig.DBName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})

	if err != nil {
		panic(fmt.Sprintf("connect to mysql error: %v, dsn: %v", err, dsn))
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}

	sqlDB.SetMaxOpenConns(500)
	sqlDB.SetMaxIdleConns(200)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err = sqlDB.Ping(); err != nil {
		panic(err)
	}

	marketClient := market.NewMarketClient(zrpc.MustNewClient(c.MarketService).Conn())

	liqCfg := c.LiquidityCfg
	if liqCfg.DSN == "" {
		liqCfg.DSN = dsn
	}
	if liqCfg.TokenPageSize <= 0 {
		liqCfg.TokenPageSize = 200
	}
	if liqCfg.CacheTTLSeconds <= 0 {
		liqCfg.CacheTTLSeconds = 60
	}

	svc := &ServiceContext{
		Config:            c,
		Redis:             rds,
		MarketClient:      marketClient,
		MarketTokenClient: marketClient,
		DB:                db,
		ClmmPositionModel: solmodel.NewClmmPositionModel(db),
		IpfsGateway:       c.Ipfs.Gateway,
		NFTStorageKey:     c.Ipfs.NFTStorageApiKey,
		LiquidityCfg:      liqCfg,
		LiquidityMetric:   metrics.NewLiquidityMetrics(),
	}

	// Initialize Lighthouse client if configured
	if c.Ipfs.Provider == "lighthouse" && c.Ipfs.NFTStorageApiKey != "" {
		svc.LhClient = lighthouse.NewClient(nil, lighthouse.WithAPIKey(c.Ipfs.NFTStorageApiKey))
		logx.Infof("Lighthouse client initialized")
	}

	if c.SolConfig.Enable {
		logx.Infof("SolConfig enabled, initializing SolTxMananger...")
		solTxMananger, err := solana.NewTxManager(db, c.SolConfig.NodeUrl[0], c.SolConfig.Jito, c.SolConfig.UUID, c.SimulateOnly)
		if err != nil {
			panic(err)
		}
		svc.SolTxMananger = solTxMananger
		logx.Infof("SolTxMananger initialized successfully")
	} else {
		logx.Infof("SolConfig disabled, SolTxMananger not initialized")
	}

	return svc
}
