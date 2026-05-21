package liquidity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"richcode.cc/dex/gateway/internal/ratelimit"
	"richcode.cc/dex/trade/trade"
)

type mockTrade struct {
	tokensResp *trade.GetCpmmTokensResponse
	tokensErr  error
	feeResp    *trade.GetCpmmFeeTiersResponse
	feeErr     error
	poolResp   *trade.CreateCpmmPoolResponse
	poolErr    error
}

func (m *mockTrade) GetCpmmTokens(ctx context.Context, in *trade.GetCpmmTokensRequest, opts ...grpc.CallOption) (*trade.GetCpmmTokensResponse, error) {
	if m.tokensErr != nil {
		return nil, m.tokensErr
	}
	return m.tokensResp, nil
}

func (m *mockTrade) GetCpmmFeeTiers(ctx context.Context, in *trade.GetCpmmFeeTiersRequest, opts ...grpc.CallOption) (*trade.GetCpmmFeeTiersResponse, error) {
	if m.feeErr != nil {
		return nil, m.feeErr
	}
	return m.feeResp, nil
}

func (m *mockTrade) CreateCpmmPool(ctx context.Context, in *trade.CreateCpmmPoolRequest, opts ...grpc.CallOption) (*trade.CreateCpmmPoolResponse, error) {
	if m.poolErr != nil {
		return nil, m.poolErr
	}
	return m.poolResp, nil
}

func (m *mockTrade) CreateClmmPool(ctx context.Context, in *trade.CreateClmmPoolRequest, opts ...grpc.CallOption) (*trade.CreateClmmPoolResponse, error) {
	if m.poolErr != nil {
		return nil, m.poolErr
	}
	if m.poolResp == nil {
		return &trade.CreateClmmPoolResponse{}, nil
	}
	return &trade.CreateClmmPoolResponse{
		TxHash:    m.poolResp.TxHash,
		TxType:    m.poolResp.TxType,
		TxBase64:  m.poolResp.TxBase64,
		ExpiresAt: m.poolResp.ExpiresAt,
	}, nil
}

func newTestHandler(t *testing.T, tradeClient TradeClient) *Handler {
	t.Helper()
	return &Handler{
		trade: tradeClient,
		cfg: Config{
			TokensPath:     "/liquidity/cpmm/tokens",
			FeeTiersPath:   "/liquidity/cpmm/fee-tiers",
			TokensLimit:    100,
			FeeTiersLimit:  100,
			CreatePoolPath: "/liquidity/cpmm/pools",
			CreateLimit:    20,
			CacheTTL:       time.Minute,
		},
		limiter:  ratelimit.New(),
		cacheTTL: time.Minute,
	}
}

func TestTokensHandlerSuccess(t *testing.T) {
	h := newTestHandler(t, &mockTrade{
		tokensResp: &trade.GetCpmmTokensResponse{
			PageNo:   1,
			PageSize: 10,
			Total:    2,
			Tokens: []*trade.CpmmTokenItem{
				{Mint: "mint1", Symbol: "AAA"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/liquidity/cpmm/tokens?page_no=1&page_size=10", nil)
	rr := httptest.NewRecorder()

	h.handleTokens(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "max-age=60", res.Header.Get("Cache-Control"))
	require.NotEmpty(t, rr.Body.String())
	require.Contains(t, rr.Body.String(), "mint1")
}

func TestTokensHandlerError(t *testing.T) {
	h := newTestHandler(t, &mockTrade{
		tokensErr: errors.New("backend down"),
	})
	req := httptest.NewRequest(http.MethodGet, "/liquidity/cpmm/tokens", nil)
	rr := httptest.NewRecorder()

	h.handleTokens(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	require.Contains(t, rr.Body.String(), "backend down")
}

func TestFeeTiersHandler(t *testing.T) {
	h := newTestHandler(t, &mockTrade{
		feeResp: &trade.GetCpmmFeeTiersResponse{
			Version: "v2",
			Tiers: []*trade.CpmmFeeTierItem{
				{Label: "0.25%", ValueBps: 25},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/liquidity/cpmm/fee-tiers", nil)
	rr := httptest.NewRecorder()

	h.handleFeeTiers(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, rr.Body.String(), "0.25%")
}

func TestCreatePoolHandler(t *testing.T) {
	h := newTestHandler(t, &mockTrade{
		poolResp: &trade.CreateCpmmPoolResponse{
			TxHash:   "hash",
			TxType:   "legacy",
			TxBase64: "base64",
		},
	})

	body := `{
		"pool_type": "CPMM",
		"base_token_mint": "mint-base",
		"quote_token_mint": "mint-quote",
		"base_amount": "10",
		"quote_amount": "20",
		"initial_price": "2",
		"fee_tier_bps": 25,
		"user_wallet_address": "wallet"
	}`
	req := httptest.NewRequest(http.MethodPost, "/liquidity/cpmm/pools", strings.NewReader(body))
	rr := httptest.NewRecorder()

	h.handleCreatePool(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, rr.Body.String(), "hash")
}
