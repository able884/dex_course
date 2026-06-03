package limitorder

import (
	"context"
	"strconv"

	"richcode.cc/dex/model/limitordermodel"
)

// PublishOrderFromModel 从订单模型发布用户订单更新
func (p *Publisher) PublishOrderFromModel(ctx context.Context, order *limitordermodel.LimitOrder, chainId int64) error {
	if order == nil {
		return nil
	}

	// 确定订单方向
	var side string
	if order.Side == 1 {
		side = "buy"
	} else if order.Side == 2 {
		side = "sell"
	} else {
		side = "unknown"
	}

	// 计算已成交数量
	filledQty := order.QtyDisplay - order.RemainingDisplay

	update := &UserOrderUpdate{
		ChainId:           chainId,
		MarketPda:         order.MarketPda,
		UserWalletAddress: order.Owner,
		OrderPda:          order.OrderPda,
		Side:              side,
		Price:             strconv.FormatFloat(order.PriceDisplay, 'f', -1, 64),
		Quantity:          strconv.FormatFloat(order.QtyDisplay, 'f', -1, 64),
		FilledQuantity:    strconv.FormatFloat(filledQty, 'f', -1, 64),
		Status:            int(order.Status),
		CreatedAt:         order.CreatedAt.Unix(),
		UpdatedAt:         order.UpdatedAt.Unix(),
	}

	return p.PublishUserOrderUpdate(ctx, update)
}
