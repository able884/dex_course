package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"richcode.cc/dex/message/internal/types"
)

func TestPublish(t *testing.T) {
	// 创建Redis客户端
	host := "172.17.112.1"
	port := "6379"
	pass := "123456" // 如果有密码，请设置
	db := 0

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		DB:       db,
		Password: pass,
	})
	defer rdb.Close()

	ctx := context.Background()

	// 检查连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("❌ Redis连接失败: %v\n", err)
		os.Exit(1)
	}

	// 检查订阅者
	nums, _ := rdb.PubSubNumSub(ctx, "pump_token_update").Result()
	fmt.Printf("当前订阅者数量: %d\n", nums["pump_token_update"])

	// 发送5条测试消息
	for i := 1; i <= 5; i++ {
		td := &types.TokenData{
			ChainId:      100000,
			TokenAddress: fmt.Sprintf("TEST_MSG_%d", i),
			PairAddress:  fmt.Sprintf("PAIR_%d", i),
			TokenName:    fmt.Sprintf("测试消息 %d", i),
			TokenSymbol:  fmt.Sprintf("MSG%d", i),
			LaunchTime:   time.Now().Unix(),
			MktCap:       float64(1000 * i),
			HoldCount:    int64(100 * i),
			Change24:     1.5,
			Txs24h:       int64(50 * i),
			PumpStatus:   2,
		}

		b, _ := json.Marshal(td)
		n, err := rdb.Publish(ctx, "pump_token_update", b).Result()
		if err != nil {
			fmt.Printf("❌ 发布失败: %v\n", err)
			continue
		}

		fmt.Printf("✓ 消息 %d 已发布, 投递给 %d 个订阅者\n", i, n)
		fmt.Printf("  请检查服务端窗口是否显示: '✓ [TOKEN WS] 收到Redis消息'\n\n")

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n发送完成！请检查服务端窗口的输出。")
	fmt.Println("如果服务端没有任何输出，说明:")
	fmt.Println("1. 代码可能没有重新编译")
	fmt.Println("2. 或者服务端的订阅循环可能被阻塞或退出了")
}
