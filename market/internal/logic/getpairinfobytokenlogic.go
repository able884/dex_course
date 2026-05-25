package logic

import (
	"context"
	"fmt"

	"richcode.cc/dex/market/internal/constants"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"

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
	pair := &solmodel.Pair{}
	var err error
	if in.ChainId == int64(constants.Sol) || in.ChainId == 100000 {
		pairModel := solmodel.NewPairModel(l.svcCtx.DB)
		pair, err = pairModel.FindOneByChainIdTokenAddress(l.ctx, in.ChainId, in.TokenAddress)
		if err != nil {
			return nil, err
		}
	}

	if pair == nil {
		return nil, fmt.Errorf("pairInfo is nil for token: %s", in.TokenAddress)
	}

	return &market.GetPairInfoByTokenResponse{
		ChainId:                in.ChainId,
		Address:                pair.Address,
		Name:                   pair.Name,
		FactoryAddress:         pair.FactoryAddress,
		BaseTokenAddress:       pair.BaseTokenAddress,
		TokenAddress:           pair.TokenAddress,
		BaseTokenSymbol:        pair.BaseTokenSymbol,
		TokenSymbol:            pair.TokenSymbol,
		BaseTokenDecimal:       pair.BaseTokenDecimal,
		TokenDecimal:           pair.TokenDecimal,
		BaseTokenIsNativeToken: pair.BaseTokenIsNativeToken == 1,
		BaseTokenIsToken0:      pair.BaseTokenIsToken0 == 1,
		InitBaseTokenAmount:    pair.InitBaseTokenAmount,
		InitTokenAmount:        pair.InitTokenAmount,
		CurrentBaseTokenAmount: pair.CurrentBaseTokenAmount,
		CurrentTokenAmount:     pair.CurrentTokenAmount,
		Fdv:                    pair.Fdv,
		// 对内显示 不改成fdv
		MktCap:            pair.MktCap,
		BaseTokenPrice:    pair.BaseTokenPrice,
		TokenPrice:        pair.TokenPrice,
		BlockNum:          pair.BlockNum,
		BlockTime:         pair.BlockTime.Unix(),
		HighestTokenPrice: pair.HighestTokenPrice,
		LatestTradeTime:   pair.LatestTradeTime.Unix(),
	}, nil
}
