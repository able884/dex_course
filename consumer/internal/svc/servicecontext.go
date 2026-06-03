package svc

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/market/marketclient"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/trade/tradeclient"
)

type ServiceContext struct {
	Config                    config.Config
	MarketService             marketclient.Market
	TradeService              tradeclient.Trade
	Redis                     *redis.Redis
	MetadataCache             *redis.Redis
	DB                        *gorm.DB // Database connection for direct queries
	BlockPipelineSettings     config.BlockPipelineConfig
	solClientLock             sync.Mutex
	solClientIndex            int
	solClient                 *client.Client
	solClients                []*client.Client
	PairModel                 solmodel.PairModel
	BlockModel                solmodel.BlockModel
	TokenModel                solmodel.TokenModel
	TradeModel                solmodel.TradeModel
	PumpAmmInfoModel          solmodel.PumpAmmInfoModel
	SolTokenAccountModel      solmodel.SolTokenAccountModel
	SolRaydiumCLMMPoolV1Model solmodel.ClmmPoolInfoV1Model
	SolRaydiumCLMMPoolV2Model solmodel.ClmmPoolInfoV2Model
	SolRaydiumCPMMPoolModel   solmodel.CpmmPoolInfoModel
	SolRaydiumPoolModel       solmodel.RaydiumPoolModel
	ClmmPositionModel         solmodel.ClmmPositionModel
	// Limit Order models
	LimitOrderModel       limitordermodel.LimitOrderModel
	LimitOrderMarginModel limitordermodel.LimitOrderMarginModel
	LimitOrderMarketModel limitordermodel.LimitOrderMarketModel
	// Pump migration pipeline
	PumpMigrationChan chan PumpMigrationJob
	PumpMigrationOnce *sync.Map
}

// PumpMigrationJob carries the minimal data needed to initialize a Raydium CPMM pool
// once a Pump pair reaches the migration threshold.
type PumpMigrationJob struct {
	PairAddr    string
	TokenMint   string
	TokenSymbol string
	BaseAmount  float64
	TokenAmount float64
	PumpPoint   float64
	Maker       string
}

func NewServiceContext(c config.Config) *ServiceContext {
	redisClient := initRedisClient(redisKeyConfToRedisConf(c.Redis))
	return &ServiceContext{
		Config:                c,
		Redis:                 redisClient,
		MetadataCache:         redisClient,
		BlockPipelineSettings: c.BlockPipeline,
		PumpMigrationChan:     make(chan PumpMigrationJob, 128),
		PumpMigrationOnce:     &sync.Map{},
	}
}

func NewSolServiceContext(c config.Config) *ServiceContext {
	logx.MustSetup(c.Log)

	logx.Infof("newSolServiceContext: config:%#v", c)

	var solClients []*client.Client
	for _, node := range c.Sol.NodeUrl {
		client.New(rpc.WithEndpoint(node), rpc.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}))
		solClients = append(solClients, client.NewClient(node))
	}

	// Initialize database connection
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.MySQLConfig.User,
		c.MySQLConfig.Password,
		c.MySQLConfig.Host,
		c.MySQLConfig.Port,
		c.MySQLConfig.DBName,
	)
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             5 * time.Second, // 慢 sql 阈值
			LogLevel:                  logger.Warn,     // 日志级别
			IgnoreRecordNotFoundError: true,            // 忽略 record not found
			ParameterizedQueries:      false,           // 禁用参数化查询
			Colorful:                  true,
		},
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(200)
	sqlDB.SetMaxOpenConns(500)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Initialize BlockModel
	blockModel := solmodel.NewBlockModel(db)

	redisClient := initRedisClient(redisKeyConfToRedisConf(c.Redis))

	var tradeSvc tradeclient.Trade
	if c.TradeService.Target != "" {
		tradeSvc = tradeclient.NewTrade(zrpc.MustNewClient(c.TradeService))
	}

	return &ServiceContext{
		Config:                    c,
		MarketService:             marketclient.NewMarket(zrpc.MustNewClient(c.MarketService)),
		TradeService:              tradeSvc,
		Redis:                     redisClient,
		MetadataCache:             redisClient,
		DB:                        db, // Add DB connection
		BlockPipelineSettings:     c.BlockPipeline,
		solClients:                solClients,
		BlockModel:                blockModel,
		PairModel:                 solmodel.NewPairModel(db),
		TokenModel:                solmodel.NewTokenModel(db),
		TradeModel:                solmodel.NewTradeModel(db),
		PumpAmmInfoModel:          solmodel.NewPumpAmmInfoModel(db),
		SolTokenAccountModel:      solmodel.NewSolTokenAccountModel(db),
		SolRaydiumCLMMPoolV1Model: solmodel.NewClmmPoolInfoV1Model(db),
		SolRaydiumCLMMPoolV2Model: solmodel.NewClmmPoolInfoV2Model(db),
		SolRaydiumCPMMPoolModel:   solmodel.NewCpmmPoolInfoModel(db),
		ClmmPositionModel:         solmodel.NewClmmPositionModel(db),
		LimitOrderModel:           limitordermodel.NewLimitOrderModel(db),
		LimitOrderMarginModel:     limitordermodel.NewLimitOrderMarginModel(db),
		LimitOrderMarketModel:     limitordermodel.NewLimitOrderMarketModel(db),
		PumpMigrationChan:         make(chan PumpMigrationJob, 128),
		PumpMigrationOnce:         &sync.Map{},
	}
}

func (sc *ServiceContext) GetSolClient() *client.Client {
	sc.solClientLock.Lock()
	defer sc.solClientLock.Unlock()
	sc.solClientIndex++
	index := sc.solClientIndex % len(sc.solClients)
	sc.solClient = sc.solClients[index]
	return sc.solClients[index]
}

func redisKeyConfToRedisConf(conf redis.RedisKeyConf) redis.RedisConf {
	return redis.RedisConf{
		Host:        conf.Host,
		Type:        conf.Type,
		Pass:        conf.Pass,
		Tls:         conf.Tls,
		NonBlock:    conf.NonBlock,
		PingTimeout: conf.PingTimeout,
	}
}

func initRedisClient(conf redis.RedisConf) *redis.Redis {
	if len(conf.Host) == 0 {
		return nil
	}
	return redis.MustNewRedis(conf)
}
