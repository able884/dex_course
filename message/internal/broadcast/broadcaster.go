package broadcast

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"richcode.cc/dex/message/internal/types"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

// Broadcaster 处理向WebSocket客户端广播消息
type Broadcaster struct {
	redisClient  *redis.Client
	getClients   func() map[*websocket.Conn]*types.TokenClient
	removeClient func(*websocket.Conn)
}

// NewBroadcaster 创建一个新的广播器
func NewBroadcaster(
	redisClient *redis.Client,
	getClients func() map[*websocket.Conn]*types.TokenClient,
	removeClient func(*websocket.Conn),
) *Broadcaster {
	return &Broadcaster{
		redisClient:  redisClient,
		getClients:   getClients,
		removeClient: removeClient,
	}
}

// BroadcastNewToken 向所有订阅的客户端发送新代币通知
func (b *Broadcaster) BroadcastNewToken(tokenData *types.TokenData) {
	message := types.TokenMessage{
		Type: "new_token",
		Data: tokenData,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logx.Errorf("[TOKEN WS] Marshal error: %v", err)
		return
	}

	clients := b.getClients()
	category := types.PumpStatusToCategory(tokenData.PumpStatus)

	for conn, client := range clients {
		if client.ChainId == tokenData.ChainId && client.IsSubscribedToCategory(category) {
			select {
			case client.Send <- data:
				logx.Infof("[TOKEN WS] Sent new token: name=%s, chain=%d", tokenData.TokenName, client.ChainId)
			default:
				logx.Errorf("[TOKEN WS] Send failed, channel full: removing client")
				go func(c *websocket.Conn) {
					b.removeClient(c)
					c.Close()
				}(conn)
			}
		}
	}
}

// BroadcastTokenStatusUpdate 向所有订阅的客户端发送代币状态更新
func (b *Broadcaster) BroadcastTokenStatusUpdate(tokenData *types.TokenData) {
	message := types.TokenMessage{
		Type: "token_status_update",
		Data: tokenData,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logx.Errorf("[TOKEN WS] Marshal error: %v", err)
		return
	}

	clients := b.getClients()

	for conn, client := range clients {
		if client.ChainId == tokenData.ChainId {
			select {
			case client.Send <- data:
				logx.Infof("[TOKEN WS] Sent token update: name=%s, chain=%d", tokenData.TokenName, client.ChainId)
			default:
				logx.Errorf("[TOKEN WS] Send failed, channel full: removing client")
				go func(c *websocket.Conn) {
					b.removeClient(c)
					c.Close()
				}(conn)
			}
		}
	}
}

// StartSubscription 监听Redis代币更新并广播它们
func (b *Broadcaster) StartSubscription(ctx context.Context) {
	// Subscribe to Redis channels used by the message service.
	pubsub := b.redisClient.Subscribe(ctx, "pump_token_new", "pump_token_update")
	defer pubsub.Close()

	// Ensure subscription is established before proceeding so we can log a clear
	// "subscription ready" message. go-redis will deliver a Subscription message first.
	if ack, err := pubsub.Receive(ctx); err != nil {
		logx.Errorf("[TOKEN WS] Redis subscription init failed: addr=%s, err=%v", b.redisClient.Options().Addr, err)
		return
	} else {
		switch a := ack.(type) {
		case *redis.Subscription:
			logx.Infof("[TOKEN WS] Redis subscribed: addr=%s, channel=%s, count=%d", b.redisClient.Options().Addr, a.Channel, a.Count)
		default:
			logx.Infof("[TOKEN WS] Redis subscription handshake received: %+v", a)
		}
	}

	logx.Infof("[TOKEN WS] Subscription service started and listening on channels: %s (redis=%s)", strings.Join([]string{"pump_token_new", "pump_token_update"}, ","), b.redisClient.Options().Addr)
	fmt.Printf("✓ [TOKEN WS] 订阅服务已启动，监听频道: pump_token_new, pump_token_update (redis=%s)\n", b.redisClient.Options().Addr)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			logx.Info("[TOKEN WS] Subscription service stopped")
			fmt.Println("✓ [TOKEN WS] 订阅服务已停止")
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			// Log the raw message meta for observability (truncate payload to avoid huge logs).
			payload := msg.Payload
			if len(payload) > 512 {
				payload = payload[:512] + "... (truncated)"
			}
			logx.Infof("[TOKEN WS] Received from Redis: channel=%s, bytes=%d, payload=%s", msg.Channel, len(msg.Payload), payload)
			fmt.Printf("✓ [TOKEN WS] 收到Redis消息: channel=%s, bytes=%d, payload=%s\n", msg.Channel, len(msg.Payload), payload)

			var tokenData types.TokenData
			if err := json.Unmarshal([]byte(msg.Payload), &tokenData); err != nil {
				logx.Errorf("[TOKEN WS] Unmarshal error: %v", err)
				continue
			}

			switch msg.Channel {
			case "pump_token_new":
				logx.Infof("[TOKEN WS] Broadcasting new token: %s", tokenData.TokenName)
				fmt.Printf("✓ [TOKEN WS] 广播新代币: %s (chain_id=%d)\n", tokenData.TokenName, tokenData.ChainId)
				b.BroadcastNewToken(&tokenData)
			case "pump_token_update":
				logx.Infof("[TOKEN WS] Broadcasting token update: %s", tokenData.TokenName)
				fmt.Printf("✓ [TOKEN WS] 广播代币更新: %s (chain_id=%d)\n", tokenData.TokenName, tokenData.ChainId)
				b.BroadcastTokenStatusUpdate(&tokenData)
			}
		}
	}
}
