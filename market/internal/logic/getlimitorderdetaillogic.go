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

type GetLimitOrderDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLimitOrderDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLimitOrderDetailLogic {
	return &GetLimitOrderDetailLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLimitOrderDetailLogic) GetLimitOrderDetail(in *market.GetLimitOrderDetailRequest) (*market.GetLimitOrderDetailResponse, error) {
	// 参数验证
	if in.OrderPda == "" {
		return nil, fmt.Errorf("order_pda is required")
	}

	// 查询订单详情（关联市场信息）
	type OrderWithMarket struct {
		limitordermodel.LimitOrder
		BaseSymbol  string
		QuoteSymbol string
	}

	var orderDetail OrderWithMarket
	err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&limitordermodel.LimitOrder{}).
		Select("limit_order.*, limit_order_market.base_symbol, limit_order_market.quote_symbol").
		Joins("LEFT JOIN limit_order_market ON limit_order.market_id = limit_order_market.id").
		Where("limit_order.order_pda = ?", in.OrderPda).
		First(&orderDetail).Error
	if err != nil {
		l.Errorf("failed to query order detail: %v", err)
		return nil, fmt.Errorf("order not found")
	}

	// 查询成交记录
	var fills []limitordermodel.LimitOrderFill
	l.svcCtx.DB.WithContext(l.ctx).
		Where("maker_order_id = ? OR taker_order_id = ?", orderDetail.Id, orderDetail.Id).
		Order("created_at DESC").
		Find(&fills)

	fillItems := make([]*market.OrderFillItem, 0, len(fills))
	for _, fill := range fills {
		fillItems = append(fillItems, &market.OrderFillItem{
			MarketPda:   fill.MarketPda,
			BaseSymbol:  orderDetail.BaseSymbol,
			QuoteSymbol: orderDetail.QuoteSymbol,
			Maker:       fill.Maker,
			Taker:       fill.Taker,
			Quantity:    strconv.FormatFloat(fill.QtyDisplay, 'f', -1, 64),
			Price:       strconv.FormatFloat(fill.PriceDisplay, 'f', -1, 64),
			Fee:         strconv.FormatFloat(fill.FeeDisplay, 'f', -1, 64),
			TotalValue:  strconv.FormatFloat(fill.TotalValueDisplay, 'f', -1, 64),
			TxHash:      fill.TxHash,
			Slot:        fill.Slot,
			CreatedAt:   fill.CreatedAt.Unix(),
		})
	}

	// 构建响应
	detail := &market.OrderDetailItem{
		OrderPda:          orderDetail.OrderPda,
		OrderId:           orderDetail.OrderId,
		MarketPda:         orderDetail.MarketPda,
		BaseSymbol:        orderDetail.BaseSymbol,
		QuoteSymbol:       orderDetail.QuoteSymbol,
		Owner:             orderDetail.Owner,
		MarginPda:         orderDetail.MarginPda,
		Side:              int32(orderDetail.Side),
		Price:             strconv.FormatFloat(orderDetail.PriceDisplay, 'f', -1, 64),
		Quantity:          strconv.FormatFloat(orderDetail.QtyDisplay, 'f', -1, 64),
		Remaining:         strconv.FormatFloat(orderDetail.RemainingDisplay, 'f', -1, 64),
		TotalValue:        strconv.FormatFloat(orderDetail.TotalValueDisplay, 'f', -1, 64),
		FilledPercent:     strconv.FormatFloat(orderDetail.FilledPercent, 'f', 2, 64),
		Status:            int32(orderDetail.Status),
		ExpirySlot:        orderDetail.ExpirySlot,
		SelfTradeBehavior: int32(orderDetail.SelfTradeBehavior),
		CreateTxHash:      orderDetail.CreateTxHash.String,
		CancelTxHash:      orderDetail.CancelTxHash.String,
		CreatedAt:         orderDetail.CreatedAt.Unix(),
		UpdatedAt:         orderDetail.UpdatedAt.Unix(),
		Fills:             fillItems,
	}

	// 处理可选时间字段
	if orderDetail.FilledAt.Valid {
		detail.FilledAt = orderDetail.FilledAt.Time.Unix()
	}
	if orderDetail.CancelledAt.Valid {
		detail.CancelledAt = orderDetail.CancelledAt.Time.Unix()
	}
	if orderDetail.ExpiredAt.Valid {
		detail.ExpiredAt = orderDetail.ExpiredAt.Time.Unix()
	}

	return &market.GetLimitOrderDetailResponse{
		Order: detail,
	}, nil
}
