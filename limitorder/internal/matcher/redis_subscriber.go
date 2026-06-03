package matcher

import (
	"context"
	"encoding/json"

	go_redis "github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

const orderUpdateChannel = "limit_order_user_order"

// OrderUpdateSubscriber keeps the in-memory order book in sync with order updates.
type OrderUpdateSubscriber struct {
	redis  *go_redis.Client
	engine *Engine
}

func NewOrderUpdateSubscriber(redisClient *go_redis.Client, engine *Engine) *OrderUpdateSubscriber {
	return &OrderUpdateSubscriber{
		redis:  redisClient,
		engine: engine,
	}
}

func (s *OrderUpdateSubscriber) Start(ctx context.Context) {
	if s.redis == nil || s.engine == nil {
		return
	}

	go s.run(ctx)
}

func (s *OrderUpdateSubscriber) run(ctx context.Context) {
	// 订阅 limit_order_user_order频道，接收订单更新消息，
	// 消息包含 order_pda 和 market_pda；收到消息后调用 matcher.Engine 的 SyncOrderFromDB 方法，从数据库同步订单数据到内存订单簿
	pubsub := s.redis.Subscribe(ctx, orderUpdateChannel)
	defer pubsub.Close()

	if _, err := pubsub.Receive(ctx); err != nil {
		logx.Errorf("Order update subscriber failed to subscribe: %v", err)
		return
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg == nil {
				continue
			}
			orderPDA, marketPDA := parseOrderUpdate(msg.Payload)
			if orderPDA == "" {
				logx.Errorf("Order update missing order_pda: payload=%s", msg.Payload)
				continue
			}
			if err := s.engine.SyncOrderFromDB(ctx, marketPDA, orderPDA); err != nil {
				logx.Errorf("Order update sync failed: order=%s, err=%v", orderPDA, err)
			}
		}
	}
}

func parseOrderUpdate(payload string) (orderPDA string, marketPDA string) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		logx.Errorf("Failed to parse order update payload: %v", err)
		return "", ""
	}

	orderPDA = getStringField(data["order_pda"])
	marketPDA = getStringField(data["market_pda"])

	if orderPDA == "" {
		if orderMap, ok := data["order"].(map[string]interface{}); ok {
			orderPDA = getStringField(orderMap["order_pda"])
		}
	}

	if marketPDA == "" {
		if orderMap, ok := data["order"].(map[string]interface{}); ok {
			marketPDA = getStringField(orderMap["market_pda"])
		}
	}

	return orderPDA, marketPDA
}

func getStringField(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}
