package types

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"richcode.cc/dex/market/market"
)

type GetCpmmTokensInput struct {
	ChainID      int64
	Statuses     []string
	PageNo       int64
	PageSize     int64
	UpdatedAfter *time.Time
}

type CpmmToken struct {
	Mint      string
	Symbol    string
	Name      string
	Decimals  int32
	Logo      string
	Status    string
	ChainID   int64
	Tags      []string
	UpdatedAt time.Time
}

type GetCpmmTokensOutput struct {
	Tokens   []*CpmmToken
	PageNo   int64
	PageSize int64
	Total    int64
}

type GetCpmmFeeTiersInput struct {
	PoolType string
}

type CpmmFeeTier struct {
	PoolType       string
	ProgramAddress string
	ValueBps       int32
	Label          string
	Description    string
	TickSpacing    int32
	ConfigIndex    int32
	Address        string
	PriorityOrder  int32
	Version        string
	UpdatedAt      time.Time
}

type GetCpmmFeeTiersOutput struct {
	Tiers   []*CpmmFeeTier
	Version string
}

type LiquidityMetrics interface {
	RecordTokenFetch(status string, duration time.Duration)
	RecordFeeFetch(status string, duration time.Duration)
}

type NoopLiquidityMetrics struct{}

func (NoopLiquidityMetrics) RecordTokenFetch(status string, duration time.Duration) {}
func (NoopLiquidityMetrics) RecordFeeFetch(status string, duration time.Duration)   {}

type MarketTokenClient interface {
	GetPumpTokenList(ctx context.Context, in *market.GetPumpTokenListRequest, opts ...grpc.CallOption) (*market.GetPumpTokenListResponse, error)
	GetTokenInfo(ctx context.Context, in *market.GetTokenInfoRequest, opts ...grpc.CallOption) (*market.GetTokenInfoResponse, error)
}
