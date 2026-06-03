package limitorder

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	// Redis 频道名称
	ChannelOrderBook  = "limit_order_orderbook"
	ChannelMargin     = "limit_order_margin"
	ChannelUserOrder  = "limit_order_user_order"
)

// OrderData 订单数据
type OrderData struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Total    string `json:"total"`
}

// OrderBookUpdate 订单簿更新数据
type OrderBookUpdate struct {
	ChainId   int64       `json:"chain_id"`
	MarketPda string      `json:"market_pda"`
	Timestamp int64       `json:"timestamp"`
	Bids      []OrderData `json:"bids"`
	Asks      []OrderData `json:"asks"`
}

// MarginUpdate 保证金账户更新数据
type MarginUpdate struct {
	ChainId           int64  `json:"chain_id"`
	MarketPda         string `json:"market_pda"`
	UserWalletAddress string `json:"user_wallet_address"`
	MarginPda         string `json:"margin_pda"`
	Status            int    `json:"status"`
	BaseFree          string `json:"base_free"`
	BaseLocked        string `json:"base_locked"`
	BaseTotal         string `json:"base_total"`
	QuoteFree         string `json:"quote_free"`
	QuoteLocked       string `json:"quote_locked"`
	QuoteTotal        string `json:"quote_total"`
	BaseSymbol        string `json:"base_symbol"`
	QuoteSymbol       string `json:"quote_symbol"`
	LastSyncSlot      int64  `json:"last_sync_slot"`
	Timestamp         int64  `json:"timestamp"`
}

// UserOrderUpdate 用户订单更新数据
type UserOrderUpdate struct {
	ChainId           int64  `json:"chain_id"`
	MarketPda         string `json:"market_pda"`
	UserWalletAddress string `json:"user_wallet_address"`
	OrderPda          string `json:"order_pda"`
	Side              string `json:"side"` // "buy" or "sell"
	Price             string `json:"price"`
	Quantity          string `json:"quantity"`
	FilledQuantity    string `json:"filled_quantity"`
	Status            int    `json:"status"` // 0=pending, 1=active, 2=filled, 3=cancelled
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	Timestamp         int64  `json:"timestamp"`
}

// Publisher 限价订单消息发布器
type Publisher struct {
	redis *redis.Redis
}

// NewPublisher 创建新的发布器
func NewPublisher(redisClient *redis.Redis) *Publisher {
	return &Publisher{
		redis: redisClient,
	}
}

// PublishOrderBookUpdate 发布订单簿更新
func (p *Publisher) PublishOrderBookUpdate(ctx context.Context, update *OrderBookUpdate) error {
	if p.redis == nil {
		logx.Error("[LIMIT ORDER] Redis client is nil, cannot publish orderbook update")
		return nil
	}

	update.Timestamp = time.Now().Unix()
	data, err := json.Marshal(update)
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to marshal orderbook update: %v", err)
		return err
	}

	_, err = p.redis.Publish(ChannelOrderBook, string(data))
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to publish orderbook update: %v", err)
		return err
	}

	logx.Infof("[LIMIT ORDER] Published orderbook update: market=%s, bids=%d, asks=%d",
		update.MarketPda, len(update.Bids), len(update.Asks))
	return nil
}

// PublishMarginUpdate 发布保证金账户更新
func (p *Publisher) PublishMarginUpdate(ctx context.Context, update *MarginUpdate) error {
	if p.redis == nil {
		logx.Error("[LIMIT ORDER] Redis client is nil, cannot publish margin update")
		return nil
	}

	update.Timestamp = time.Now().Unix()
	data, err := json.Marshal(update)
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to marshal margin update: %v", err)
		return err
	}

	_, err = p.redis.Publish(ChannelMargin, string(data))
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to publish margin update: %v", err)
		return err
	}

	logx.Infof("[LIMIT ORDER] Published margin update: market=%s, user=%s",
		update.MarketPda, update.UserWalletAddress)
	return nil
}

// PublishUserOrderUpdate 发布用户订单更新
func (p *Publisher) PublishUserOrderUpdate(ctx context.Context, update *UserOrderUpdate) error {
	if p.redis == nil {
		logx.Error("[LIMIT ORDER] Redis client is nil, cannot publish user order update")
		return nil
	}

	update.Timestamp = time.Now().Unix()
	data, err := json.Marshal(update)
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to marshal user order update: %v", err)
		return err
	}

	_, err = p.redis.Publish(ChannelUserOrder, string(data))
	if err != nil {
		logx.Errorf("[LIMIT ORDER] Failed to publish user order update: %v", err)
		return err
	}

	logx.Infof("[LIMIT ORDER] Published user order update: market=%s, user=%s, order=%s",
		update.MarketPda, update.UserWalletAddress, update.OrderPda)
	return nil
}
