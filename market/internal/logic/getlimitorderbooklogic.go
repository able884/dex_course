package logic

import (
	"context"
	"fmt"
	"strconv"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/limitordermodel"
	"richcode.cc/dex/pkg/limitorder"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLimitOrderBookLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLimitOrderBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLimitOrderBookLogic {
	return &GetLimitOrderBookLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

type OrderBookAgg struct {
	Price      float64
	Qty        float64
	Total      float64
	OrderCount int32
}

// Limit order queries
func (l *GetLimitOrderBookLogic) GetLimitOrderBook(in *market.GetLimitOrderBookRequest) (*market.GetLimitOrderBookResponse, error) {
	// 参数验证
	if in.MarketPda == "" {
		return nil, fmt.Errorf("market_pda is required")
	}

	// 深度默认为20
	depth := in.Depth
	if depth <= 0 {
		depth = 20
	}

	// 查询市场信息
	var marketInfo limitordermodel.LimitOrderMarket
	err := l.svcCtx.DB.WithContext(l.ctx).
		Where("market_pda = ?", in.MarketPda).
		First(&marketInfo).Error
	if err != nil {
		l.Errorf("failed to query market: %v", err)
		return nil, fmt.Errorf("market not found")
	}

	// 查询买单 (Bid: side=1, 按价格降序排列)
	var bidAggs []OrderBookAgg
	err = l.svcCtx.DB.WithContext(l.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("price_display as price, SUM(remaining_display) as qty, SUM(remaining_display * price_display) as total, COUNT(*) as order_count").
		Where("market_id = ? AND status = ? AND side = ?", marketInfo.Id, 1, 1).
		Group("price_display").
		Order("price_display DESC").
		Limit(int(depth)).
		Scan(&bidAggs).Error
	if err != nil {
		l.Errorf("failed to query bid orders: %v", err)
		bidAggs = []OrderBookAgg{}
	}

	// 查询卖单 (Ask: side=2, 按价格升序排列)
	var askAggs []OrderBookAgg
	err = l.svcCtx.DB.WithContext(l.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("price_display as price, SUM(remaining_display) as qty, SUM(remaining_display * price_display) as total, COUNT(*) as order_count").
		Where("market_id = ? AND status = ? AND side = ?", marketInfo.Id, 1, 2).
		Group("price_display").
		Order("price_display ASC").
		Limit(int(depth)).
		Scan(&askAggs).Error
	if err != nil {
		l.Errorf("failed to query ask orders: %v", err)
		askAggs = []OrderBookAgg{}
	}

	// 构建买单响应
	bids := make([]*market.OrderBookLevel, 0, len(bidAggs))
	for _, agg := range bidAggs {
		bids = append(bids, &market.OrderBookLevel{
			Price:      strconv.FormatFloat(agg.Price, 'f', -1, 64),
			Quantity:   strconv.FormatFloat(agg.Qty, 'f', -1, 64),
			TotalValue: strconv.FormatFloat(agg.Total, 'f', -1, 64),
			OrderCount: agg.OrderCount,
		})
	}

	// 构建卖单响应
	asks := make([]*market.OrderBookLevel, 0, len(askAggs))
	for _, agg := range askAggs {
		asks = append(asks, &market.OrderBookLevel{
			Price:      strconv.FormatFloat(agg.Price, 'f', -1, 64),
			Quantity:   strconv.FormatFloat(agg.Qty, 'f', -1, 64),
			TotalValue: strconv.FormatFloat(agg.Total, 'f', -1, 64),
			OrderCount: agg.OrderCount,
		})
	}

	// 计算最佳买卖价、价差和中间价
	var bestBid, bestAsk, spread, midPrice string
	if len(bids) > 0 {
		bestBid = bids[0].Price
	}
	if len(asks) > 0 {
		bestAsk = asks[0].Price
	}
	if bestBid != "" && bestAsk != "" {
		bidPrice, _ := strconv.ParseFloat(bestBid, 64)
		askPrice, _ := strconv.ParseFloat(bestAsk, 64)
		spread = strconv.FormatFloat(askPrice-bidPrice, 'f', -1, 64)
		midPrice = strconv.FormatFloat((bidPrice+askPrice)/2, 'f', -1, 64)
	}

	response := &market.GetLimitOrderBookResponse{
		Bids:     bids,
		Asks:     asks,
		BestBid:  bestBid,
		BestAsk:  bestAsk,
		Spread:   spread,
		MidPrice: midPrice,
	}

	// 发布订单簿更新到 WebSocket
	go func() {
		// 转换为 WebSocket 消息格式
		wsBids := make([]limitorder.OrderData, 0, len(bids))
		for _, bid := range bids {
			wsBids = append(wsBids, limitorder.OrderData{
				Price:    bid.Price,
				Quantity: bid.Quantity,
				Total:    bid.TotalValue,
			})
		}

		wsAsks := make([]limitorder.OrderData, 0, len(asks))
		for _, ask := range asks {
			wsAsks = append(wsAsks, limitorder.OrderData{
				Price:    ask.Price,
				Quantity: ask.Quantity,
				Total:    ask.TotalValue,
			})
		}

		update := &limitorder.OrderBookUpdate{
			ChainId:   in.ChainId,
			MarketPda: in.MarketPda,
			Bids:      wsBids,
			Asks:      wsAsks,
		}

		if err := l.svcCtx.LimitOrderPublisher.PublishOrderBookUpdate(context.Background(), update); err != nil {
			l.Errorf("failed to publish orderbook update: %v", err)
		}
	}()

	return response, nil
}
