package logic

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/limitordermodel"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLimitOrderMarketsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLimitOrderMarketsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLimitOrderMarketsLogic {
	return &GetLimitOrderMarketsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLimitOrderMarketsLogic) GetLimitOrderMarkets(in *market.GetLimitOrderMarketsRequest) (*market.GetLimitOrderMarketsResponse, error) {
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

	// 将 chain_id 映射到 network 字符串
	var network string
	switch in.ChainId {
	case 100, 100000:
		network = "devnet"
	case 101, 900000:
		network = "mainnet"
	case 102, 900001:
		network = "testnet"
	default:
		// 如果 chain_id 为 0 或未知值，返回所有网络的市场
		network = ""
	}

	// 查询市场列表
	var markets []limitordermodel.LimitOrderMarket
	query := l.svcCtx.DB.WithContext(l.ctx).
		Table("limit_order_market")

	// 按网络过滤（如果指定了网络）
	if network != "" {
		query = query.Where("network = ?", network)
	}

	// 统计总数
	var total int64
	err := query.Count(&total).Error
	if err != nil {
		l.Errorf("failed to count markets: %v", err)
		return nil, fmt.Errorf("failed to query markets")
	}

	// 查询列表
	offset := (pageNo - 1) * pageSize
	err = query.Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&markets).Error
	if err != nil {
		l.Errorf("failed to query markets: %v", err)
		return nil, fmt.Errorf("failed to query markets")
	}

	// 构建响应 - 只返回市场基本信息，不查询订单统计
	marketItems := make([]*market.MarketItem, 0, len(markets))
	for _, m := range markets {
		tickScale := int32(m.QuoteDecimals - m.BaseDecimals)
		marketItems = append(marketItems, &market.MarketItem{
			MarketPda:     m.MarketPda,
			BaseMint:      m.BaseMint,
			QuoteMint:     m.QuoteMint,
			BaseSymbol:    m.BaseSymbol,
			QuoteSymbol:   m.QuoteSymbol,
			BaseDecimals:  int32(m.BaseDecimals),
			QuoteDecimals: int32(m.QuoteDecimals),
			TickSize:      formatScaledValue(m.TickSize, tickScale),
			MinQuantity:   formatDisplayValue(m.MinBaseLot, int32(m.BaseDecimals)),
			MakerFeeBps:   int32(m.MakerFeeBps),
			TakerFeeBps:   int32(m.TakerFeeBps),
			Paused:        m.Paused != 0,
			Status:        int32(m.Status),
			InitTxHash:    m.InitTxHash.String,
			ActiveOrders:  0,   // 订单统计由前端选择市场后再查询
			Volume_24H:    "0", // 24h成交量由市场详情接口提供
			BestBid:       "",  // 最佳买价由订单簿接口提供
			BestAsk:       "",  // 最佳卖价由订单簿接口提供
		})
	}

	return &market.GetLimitOrderMarketsResponse{
		Markets: marketItems,
		Total:   int32(total),
	}, nil
}

func formatDisplayValue(value int64, decimals int32) string {
	if decimals <= 0 {
		return strconv.FormatInt(value, 10)
	}
	if value == 0 {
		return "0"
	}
	scale := int64(1)
	for i := int32(0); i < decimals; i++ {
		scale *= 10
	}
	intPart := value / scale
	fracPart := value % scale
	if fracPart == 0 {
		return strconv.FormatInt(intPart, 10)
	}
	fracStr := strconv.FormatInt(fracPart, 10)
	if int32(len(fracStr)) < decimals {
		fracStr = strings.Repeat("0", int(decimals)-len(fracStr)) + fracStr
	}
	fracStr = strings.TrimRight(fracStr, "0")
	return strconv.FormatInt(intPart, 10) + "." + fracStr
}

func formatScaledValue(value int64, scale int32) string {
	if scale == 0 {
		return strconv.FormatInt(value, 10)
	}
	if scale < 0 {
		multiplier := int64(1)
		for i := int32(0); i < -scale; i++ {
			multiplier *= 10
		}
		return strconv.FormatInt(value*multiplier, 10)
	}
	return formatDisplayValue(value, scale)
}
