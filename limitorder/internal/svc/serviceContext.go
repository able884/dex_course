package svc

import (
	"context"
	"crypto/tls"
	"time"

	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	go_redis "github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"richcode.cc/dex/limitorder/internal/chain"
	"richcode.cc/dex/limitorder/internal/config"
	"richcode.cc/dex/limitorder/internal/cranker"
	"richcode.cc/dex/limitorder/internal/matcher"
	"richcode.cc/dex/model/limitordermodel"
)

type ServiceContext struct {
	Config config.Config
	DB     *gorm.DB

	LimitOrderConfigModel limitordermodel.LimitOrderConfigModel
	LimitOrderMarketModel limitordermodel.LimitOrderMarketModel
	LimitOrderModel       limitordermodel.LimitOrderModel
	LimitOrderFillModel   limitordermodel.LimitOrderFillModel
	LimitOrderMarginModel limitordermodel.LimitOrderMarginModel

	SolanaClient *chain.SolanaClient

	MatcherEngine  *matcher.Engine
	CrankerService *cranker.Service
	RedisClient    *go_redis.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := gorm.Open(mysql.Open(c.DSN), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	var solanaClient *chain.SolanaClient
	if c.Sol.Enable && len(c.Sol.NodeUrl) > 0 {
		solanaClient, err = chain.NewSolanaClient(c.Sol.NodeUrl[0])
		if err != nil {
			panic(err)
		}
	}

	var matcherEngine *matcher.Engine
	var crankerService *cranker.Service
	var redisClient *go_redis.Client

	if c.Matcher.Enable {
		sqlDB, err := db.DB()
		if err != nil {
			panic(err)
		}

		rpcURL := ""
		if len(c.Sol.NodeUrl) > 0 {
			rpcURL = c.Sol.NodeUrl[0]
		}

		matcherEngine = matcher.NewEngine(sqlDB, matcher.EngineConfig{
			WorkerPoolSize:    c.Matcher.WorkerPoolSize,
			MatchInterval:     time.Duration(c.Matcher.MatchInterval) * time.Millisecond,
			EventBufferSize:   c.Matcher.EventBufferSize,
			RPCURL:            rpcURL,
			MatcherPrivateKey: c.Sol.MatcherPrivateKey,
			ProgramID:         "C4Xn6ACR3XmTwpWm2fN82QSAx23TQkwVW8wro9Hdpqef",
		})

		// 启动匹配引擎，解析内存订单簿，执行匹配逻辑，生成成交明细Fills，并调用链上 match_orders 指令完成撮合成交
		if e := matcherEngine.Start(context.Background()); e != nil {
			panic(e)
		}
	}

	if len(c.Cache) > 0 {
		node := c.Cache[0]
		opts := &go_redis.Options{
			Addr:     node.Host,
			Password: node.Pass,
		}
		if node.Tls {
			opts.TLSConfig = &tls.Config{}
		}
		redisClient = go_redis.NewClient(opts)
	}

	if matcherEngine != nil && redisClient != nil {
		subscriber := matcher.NewOrderUpdateSubscriber(redisClient, matcherEngine)
		// redis 订阅器，监听订单更新消息，将数据库订单数据同步到内存订单簿
		subscriber.Start(context.Background())
	}

	if c.Cranker.Enable {
		rpcURL := ""
		if len(c.Sol.NodeUrl) > 0 {
			rpcURL = c.Sol.NodeUrl[0]
		}
		var rpcClient *ag_rpc.Client
		if rpcURL != "" {
			rpcClient = ag_rpc.New(rpcURL)
		}

		sqlxConn := sqlx.NewMysql(c.DSN)
		crankerService = cranker.NewService(sqlxConn, cranker.ServiceConfig{
			ScanInterval:      time.Duration(c.Cranker.ScanInterval) * time.Millisecond,
			BatchSize:         c.Cranker.BatchSize,
			ExpiredOrderLimit: c.Cranker.ExpiredOrderLimit,
			Enable:            c.Cranker.Enable,
			SignerPrivateKey:  c.Sol.MatcherPrivateKey,
			ProgramID:         chain.LimitOrderProgramID,
		}, rpcClient)

		// 启动清理服务，定期扫描过期订单，调用链上 cancel_order 指令清理过期订单
		if err := crankerService.Start(context.Background()); err != nil {
			logx.Errorf("Failed to start cranker service: %v", err)
		}
	}

	return &ServiceContext{
		Config: c,
		DB:     db,

		LimitOrderConfigModel: limitordermodel.NewLimitOrderConfigModel(db),
		LimitOrderMarketModel: limitordermodel.NewLimitOrderMarketModel(db),
		LimitOrderModel:       limitordermodel.NewLimitOrderModel(db),
		LimitOrderFillModel:   limitordermodel.NewLimitOrderFillModel(db),
		LimitOrderMarginModel: limitordermodel.NewLimitOrderMarginModel(db),

		SolanaClient: solanaClient,

		MatcherEngine:  matcherEngine,
		CrankerService: crankerService,
		RedisClient:    redisClient,
	}
}
