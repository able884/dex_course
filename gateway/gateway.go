package main

import (
	"errors"
	"flag"
	"time"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/gateway"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"richcode.cc/dex/gateway/internal/handler/liquidity"
	"richcode.cc/dex/gateway/middleware"
)

var configFile = flag.String("f", "etc/gateway.yaml", "config file")

func main() {
	flag.Parse()
	conf.MustLoad(*configFile, &middleware.GlobalConfig)

	gw := gateway.MustNewServer(middleware.GlobalConfig.GatewayConf)
	defer gw.Stop()
	rest.WithNotAllowedHandler(middleware.NewCorsMiddleware().Handler())(gw.Server)
	gw.Use(middleware.NewCorsMiddleware().Handle)
	gw.Use(middleware.HeaderMiddleware)
	gw.Use(middleware.AuthMiddleware)
	gw.Use(middleware.WrapResponse)

	if err := registerLiquidityRoutes(gw); err != nil {
		logx.Errorf("register liquidity routes failed: %v", err)
	}

	logx.Infof("Starting gateway at %s:%d ...", middleware.GlobalConfig.GatewayConf.Host, middleware.GlobalConfig.GatewayConf.Port)
	gw.Start()
}

func registerLiquidityRoutes(gw *gateway.Server) error {
	paths := middleware.GlobalConfig.Liquidity.Paths
	conf := findTradeGrpcConf()
	if conf == nil {
		return errors.New("trade upstream not configured")
	}
	handler := liquidity.NewHandler(
		zrpc.MustNewClient(*conf),
		liquidity.Config{
			TokensPath:     paths.Tokens,
			FeeTiersPath:   paths.FeeTiers,
			CreatePoolPath: paths.CreatePool,
			TokensLimit:    middleware.GlobalConfig.Liquidity.RateLimit.TokensQPS,
			FeeTiersLimit:  middleware.GlobalConfig.Liquidity.RateLimit.FeeTiersQPS,
			CreateLimit:    middleware.GlobalConfig.Liquidity.RateLimit.CreatePoolQPS,
			CacheTTL:       time.Duration(middleware.GlobalConfig.Liquidity.CacheTTLSeconds) * time.Second,
		},
	)
	liquidity.RegisterRoutes(gw, handler)
	return nil
}

func findTradeGrpcConf() *zrpc.RpcClientConf {
	for _, up := range middleware.GlobalConfig.Upstreams {
		if up.Name == "trade" && up.Grpc != nil {
			return up.Grpc
		}
	}
	if len(middleware.GlobalConfig.Upstreams) > 0 {
		for _, up := range middleware.GlobalConfig.Upstreams {
			if up.Grpc != nil {
				return up.Grpc
			}
		}
	}
	return nil
}
