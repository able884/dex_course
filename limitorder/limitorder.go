package main

import (
	"flag"
	"fmt"

	"richcode.cc/dex/limitorder/internal/config"
	"richcode.cc/dex/limitorder/internal/server"
	"richcode.cc/dex/limitorder/internal/svc"
	"richcode.cc/dex/limitorder/limitorder"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/limitorder.yaml", "the config file")

func main() {
	flag.Parse()

	sqlx.DisableLog()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		limitorder.RegisterLimitOrderServer(grpcServer, server.NewLimitOrderServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
