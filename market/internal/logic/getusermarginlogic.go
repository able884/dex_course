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
	"gorm.io/gorm"
)

type GetUserMarginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserMarginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserMarginLogic {
	return &GetUserMarginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetUserMarginLogic) GetUserMargin(in *market.GetUserMarginRequest) (*market.GetUserMarginResponse, error) {
	// 参数验证
	if in.UserWalletAddress == "" {
		return nil, fmt.Errorf("user_wallet_address is required")
	}
	if in.MarketPda == "" {
		return nil, fmt.Errorf("market_pda is required")
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

	// 查询保证金账户
	var margin limitordermodel.LimitOrderMargin
	err = l.svcCtx.DB.WithContext(l.ctx).
		Where("owner = ? AND market_id = ?", in.UserWalletAddress, marketInfo.Id).
		First(&margin).Error

	// 如果找不到保证金账户，返回 nil 而不是错误
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			l.Infof("margin account not found for user %s in market %s", in.UserWalletAddress, in.MarketPda)
			return &market.GetUserMarginResponse{
				Margin: nil,
			}, nil
		}
		l.Errorf("failed to query margin account: %v", err)
		return nil, fmt.Errorf("failed to query margin account")
	}

	baseFree := strconv.FormatFloat(margin.BaseFreeDisplay, 'f', -1, 64)
	baseLocked := strconv.FormatFloat(margin.BaseLockedDisplay, 'f', -1, 64)
	quoteFree := strconv.FormatFloat(margin.QuoteFreeDisplay, 'f', -1, 64)
	quoteLocked := strconv.FormatFloat(margin.QuoteLockedDisplay, 'f', -1, 64)

	baseFreeFloat, _ := strconv.ParseFloat(baseFree, 64)
	baseLockedFloat, _ := strconv.ParseFloat(baseLocked, 64)
	quoteFreeFloat, _ := strconv.ParseFloat(quoteFree, 64)
	quoteLockedFloat, _ := strconv.ParseFloat(quoteLocked, 64)

	// 构建响应
	response := &market.GetUserMarginResponse{
		Margin: &market.MarginAccountItem{
			MarginPda:    margin.MarginPda,
			MarketPda:    margin.MarketPda,
			BaseSymbol:   marketInfo.BaseSymbol,
			QuoteSymbol:  marketInfo.QuoteSymbol,
			BaseFree:     baseFree,
			BaseLocked:   baseLocked,
			QuoteFree:    quoteFree,
			QuoteLocked:  quoteLocked,
			BaseTotal:    strconv.FormatFloat(baseFreeFloat+baseLockedFloat, 'f', -1, 64),
			QuoteTotal:   strconv.FormatFloat(quoteFreeFloat+quoteLockedFloat, 'f', -1, 64),
			LastSyncSlot: margin.LastSyncSlot,
			Status:       int32(margin.Status),
			InitTxHash:   margin.InitTxHash.String,
		},
	}

	// 发布保证金更新到 WebSocket
	go func() {
		update := &limitorder.MarginUpdate{
			ChainId:           in.ChainId,
			MarketPda:         in.MarketPda,
			UserWalletAddress: in.UserWalletAddress,
			MarginPda:         margin.MarginPda,
			Status:            margin.Status,
			BaseFree:          baseFree,
			BaseLocked:        baseLocked,
			BaseTotal:         strconv.FormatFloat(baseFreeFloat+baseLockedFloat, 'f', -1, 64),
			QuoteFree:         quoteFree,
			QuoteLocked:       quoteLocked,
			QuoteTotal:        strconv.FormatFloat(quoteFreeFloat+quoteLockedFloat, 'f', -1, 64),
			BaseSymbol:        marketInfo.BaseSymbol,
			QuoteSymbol:       marketInfo.QuoteSymbol,
			LastSyncSlot:      margin.LastSyncSlot,
		}

		if err := l.svcCtx.LimitOrderPublisher.PublishMarginUpdate(context.Background(), update); err != nil {
			l.Errorf("failed to publish margin update: %v", err)
		}
	}()

	return response, nil
}
