package logic

import (
	"context"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTokenInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetTokenInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTokenInfoLogic {
	return &GetTokenInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetTokenInfoLogic) GetTokenInfo(in *market.GetTokenInfoRequest) (*market.GetTokenInfoResponse, error) {
	// todo: add your logic here and delete this line

	return &market.GetTokenInfoResponse{}, nil
}
