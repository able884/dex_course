package logic

import (
	"context"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"

	"github.com/zeromicro/go-zero/core/logx"
)

type PushTokenInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewPushTokenInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PushTokenInfoLogic {
	return &PushTokenInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *PushTokenInfoLogic) PushTokenInfo(in *market.PushTokenInfoRequest) (*market.PushTokenInfoResponse, error) {
	// todo: add your logic here and delete this line

	return &market.PushTokenInfoResponse{}, nil
}
