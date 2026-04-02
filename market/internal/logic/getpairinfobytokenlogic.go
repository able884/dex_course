package logic

import (
	"context"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPairInfoByTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPairInfoByTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPairInfoByTokenLogic {
	return &GetPairInfoByTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetPairInfoByTokenLogic) GetPairInfoByToken(in *market.GetPairInfoByTokenRequest) (*market.GetPairInfoByTokenResponse, error) {
	// todo: add your logic here and delete this line

	return &market.GetPairInfoByTokenResponse{}, nil
}
