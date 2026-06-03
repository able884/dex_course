package logic

import (
	"context"
	"fmt"
	"strconv"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserLimitOrdersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserLimitOrdersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLimitOrdersLogic {
	return &GetUserLimitOrdersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetUserLimitOrdersLogic) GetUserLimitOrders(in *market.GetUserLimitOrdersRequest) (*market.GetUserLimitOrdersResponse, error) {
	// 参数验证
	if in.UserWalletAddress == "" {
		return nil, fmt.Errorf("user_wallet_address is required")
	}

	// 默认分页参数
	pageNo := in.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 构建查询
	type OrderWithMarket struct {
		limitordermodel.LimitOrder
		BaseSymbol  string
		QuoteSymbol string
	}

	query := l.svcCtx.DB.WithContext(l.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("limit_order.*, limit_order_market.base_symbol, limit_order_market.quote_symbol").
		Joins("LEFT JOIN limit_order_market ON limit_order.market_id = limit_order_market.id").
		Where("limit_order.owner = ?", in.UserWalletAddress)

	// 市场过滤
	if in.MarketPda != "" {
		query = query.Where("limit_order.market_pda = ?", in.MarketPda)
	}

	// 状态过滤
	if in.Status > 0 {
		query = query.Where("limit_order.status = ?", in.Status)
	}

	// 统计总数
	var total int64
	err := query.Count(&total).Error
	if err != nil {
		l.Errorf("failed to count orders: %v", err)
		return nil, fmt.Errorf("failed to query orders")
	}

	// 查询订单列表
	var orders []OrderWithMarket
	offset := (pageNo - 1) * pageSize
	err = query.Order("limit_order.created_at DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Scan(&orders).Error
	if err != nil {
		l.Errorf("failed to query orders: %v", err)
		return nil, fmt.Errorf("failed to query orders")
	}

	// 构建响应
	orderItems := make([]*market.UserOrderItem, 0, len(orders))
	for _, order := range orders {
		orderItems = append(orderItems, &market.UserOrderItem{
			OrderPda:      order.OrderPda,
			MarketPda:     order.MarketPda,
			BaseSymbol:    order.BaseSymbol,
			QuoteSymbol:   order.QuoteSymbol,
			Side:          int32(order.Side),
			Price:         strconv.FormatFloat(order.PriceDisplay, 'f', -1, 64),
			Quantity:      strconv.FormatFloat(order.QtyDisplay, 'f', -1, 64),
			Remaining:     strconv.FormatFloat(order.RemainingDisplay, 'f', -1, 64),
			FilledPercent: strconv.FormatFloat(order.FilledPercent, 'f', 2, 64),
			Status:        int32(order.Status),
			CreatedAt:     order.CreatedAt.Unix(),
			ExpirySlot:    order.ExpirySlot,
			TotalValue:    strconv.FormatFloat(order.TotalValueDisplay, 'f', -1, 64),
		})
	}

	return &market.GetUserLimitOrdersResponse{
		Orders:   orderItems,
		Total:    int32(total),
		PageNo:   pageNo,
		PageSize: pageSize,
	}, nil
}
