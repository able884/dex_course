package main

import (
	"flag"
	"fmt"

	"richcode.cc/dex/market/internal/config"
	"richcode.cc/dex/market/internal/server"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/internal/ticker"
	"richcode.cc/dex/market/market"
	rds "richcode.cc/dex/market/pkg/redis"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/market.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	// 加载配置文件
	conf.MustLoad(*configFile, &c)

	// 初始化服务上下文
	svcCtx := svc.NewServiceContext(c)

	// 初始化 redis
	rds.Init(&redis.RedisKeyConf{
		RedisConf: redis.RedisConf{
			Host:        c.Redis.Host,
			Type:        c.Redis.Type,
			Pass:        c.Redis.Pass,
			Tls:         c.Redis.Tls,
			PingTimeout: c.Redis.PingTimeout,
		},
	})

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		market.RegisterMarketServer(grpcServer, server.NewMarketServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	// 初始化服务组
	serviceGroup := service.NewServiceGroup()
	defer serviceGroup.Stop()

	// 添加定时任务 PumpTicker 到服务组
	{
		pumpTicker := ticker.NewPumpTicker(svcCtx)
		serviceGroup.Add(pumpTicker)
	}

	// 启动服务组
	go func() {
		serviceGroup.Start()
	}()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
