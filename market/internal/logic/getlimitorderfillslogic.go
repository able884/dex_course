package logic

import (
	"context"
	"strconv"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLimitOrderFillsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLimitOrderFillsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLimitOrderFillsLogic {
	return &GetLimitOrderFillsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLimitOrderFillsLogic) GetLimitOrderFills(in *market.GetLimitOrderFillsRequest) (*market.GetLimitOrderFillsResponse, error) {
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

	// 构建查询（关联市场信息）
	type FillWithMarket struct {
		limitordermodel.LimitOrderFill
		BaseSymbol  string
		QuoteSymbol string
	}

	query := l.svcCtx.DB.WithContext(l.ctx).
		Model(&limitordermodel.LimitOrderFill{}).
		Select("limit_order_fill.*, limit_order_market.base_symbol, limit_order_market.quote_symbol").
		Joins("LEFT JOIN limit_order_market ON limit_order_fill.market_id = limit_order_market.id")

	// 过滤条件
	if in.OrderPda != "" {
		query = query.Where("limit_order_fill.maker_order_id IN (SELECT id FROM limit_order WHERE order_pda = ?) OR "+
			"limit_order_fill.taker_order_id IN (SELECT id FROM limit_order WHERE order_pda = ?)",
			in.OrderPda, in.OrderPda)
	}
	if in.UserWalletAddress != "" {
		query = query.Where("limit_order_fill.maker = ? OR limit_order_fill.taker = ?",
			in.UserWalletAddress, in.UserWalletAddress)
	}
	if in.MarketPda != "" {
		query = query.Where("limit_order_fill.market_pda = ?", in.MarketPda)
	}

	// 统计总数
	var total int64
	err := query.Count(&total).Error
	if err != nil {
		l.Errorf("failed to count fills: %v", err)
		return &market.GetLimitOrderFillsResponse{
			Fills: []*market.OrderFillItem{},
			Total: 0,
		}, nil
	}

	// 查询成交记录列表
	var fills []FillWithMarket
	offset := (pageNo - 1) * pageSize
	err = query.Order("limit_order_fill.created_at DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Scan(&fills).Error
	if err != nil {
		l.Errorf("failed to query fills: %v", err)
		return &market.GetLimitOrderFillsResponse{
			Fills: []*market.OrderFillItem{},
			Total: 0,
		}, nil
	}

	// 构建响应
	fillItems := make([]*market.OrderFillItem, 0, len(fills))
	for _, fill := range fills {
		fillItems = append(fillItems, &market.OrderFillItem{
			MarketPda:   fill.MarketPda,
			BaseSymbol:  fill.BaseSymbol,
			QuoteSymbol: fill.QuoteSymbol,
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

	return &market.GetLimitOrderFillsResponse{
		Fills: fillItems,
		Total: int32(total),
	}, nil
}
