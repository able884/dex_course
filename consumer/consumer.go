package main

import (
	"flag"
	"fmt"

	"richcode.cc/dex/consumer/consumer"
	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/consumer/internal/logic/block"
	"richcode.cc/dex/consumer/internal/logic/slot"
	"richcode.cc/dex/consumer/internal/server"
	"richcode.cc/dex/consumer/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/consumer.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	config.SaveConf(c)
	ctx := svc.NewSolServiceContext(c)

	// 管理多个服务
	group := service.NewServiceGroup()
	defer group.Stop()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		consumer.RegisterConsumerServer(grpcServer, server.NewConsumerServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	// defer s.Stop()

	group.Add(s)

	{
		// 添加消息队列
		slotChan := make(chan uint64, 50)

		// 失败区块队列
		errChan := make(chan uint64, 1)

		// 消费者：消费slot
		for i := 0; i < c.Consumer.Concurrency; i++ {
			group.Add(block.NewBlockService(ctx, "block-real", slotChan, i))
		}

		// 失败区块处理
		for i := 0; i < c.Consumer.NotCompletedConcurrency; i++ {
			group.Add(block.NewBlockService(ctx, "block-failed", errChan, i))
		}

		// 生产者：获取最新的slot
		group.Add(slot.NewSlotServiceGroup(ctx, slotChan, errChan))
	}

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	// s.Start()
	group.Start()
}
