package logic

import (
	"context"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetNativeTokenPriceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetNativeTokenPriceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetNativeTokenPriceLogic {
	return &GetNativeTokenPriceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetNativeTokenPriceLogic) GetNativeTokenPrice(in *market.GetNativeTokenPriceRequest) (*market.GetNativeTokenPriceResponse, error) {
	// todo: add your logic here and delete this line

	return &market.GetNativeTokenPriceResponse{}, nil
}
