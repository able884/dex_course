package liquidity

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/gateway"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"richcode.cc/dex/gateway/internal/ratelimit"
	"richcode.cc/dex/gateway/middleware"
	"richcode.cc/dex/pkg/xcode"
	"richcode.cc/dex/trade/trade"
	"richcode.cc/dex/trade/tradeclient"
)

type TradeClient interface {
	GetCpmmTokens(ctx context.Context, in *trade.GetCpmmTokensRequest, opts ...grpc.CallOption) (*trade.GetCpmmTokensResponse, error)
	GetCpmmFeeTiers(ctx context.Context, in *trade.GetCpmmFeeTiersRequest, opts ...grpc.CallOption) (*trade.GetCpmmFeeTiersResponse, error)
	CreateCpmmPool(ctx context.Context, in *trade.CreateCpmmPoolRequest, opts ...grpc.CallOption) (*trade.CreateCpmmPoolResponse, error)
	CreateClmmPool(ctx context.Context, in *trade.CreateClmmPoolRequest, opts ...grpc.CallOption) (*trade.CreateClmmPoolResponse, error)
}

type Handler struct {
	trade    TradeClient
	cfg      Config
	limiter  *ratelimit.Limiter
	cacheTTL time.Duration
}

type Config struct {
	TokensPath     string
	FeeTiersPath   string
	CreatePoolPath string
	TokensLimit    int
	FeeTiersLimit  int
	CreateLimit    int
	CacheTTL       time.Duration
}

func NewHandler(cli zrpc.Client, cfg Config) *Handler {
	return &Handler{
		trade:    tradeclient.NewTrade(cli),
		cfg:      cfg,
		limiter:  ratelimit.New(),
		cacheTTL: cfg.CacheTTL,
	}
}

func RegisterRoutes(server *gateway.Server, h *Handler) {
	if h.cfg.TokensPath != "" {
		server.AddRoute(rest.Route{
			Method:  http.MethodGet,
			Path:    h.cfg.TokensPath,
			Handler: h.handleTokens,
		})
	}
	if h.cfg.FeeTiersPath != "" {
		server.AddRoute(rest.Route{
			Method:  http.MethodGet,
			Path:    h.cfg.FeeTiersPath,
			Handler: h.handleFeeTiers,
		})
	}
	if h.cfg.CreatePoolPath != "" {
		server.AddRoute(rest.Route{
			Method:  http.MethodPost,
			Path:    h.cfg.CreatePoolPath,
			Handler: h.handleCreatePool,
		})
	}
}

func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.NewString()
	start := time.Now()
	status := "success"
	defer func() {
		h.track(h.cfg.TokensPath, status, start)
	}()
	if !h.allow(r, h.cfg.TokensPath, h.cfg.TokensLimit) {
		status = "rate_limited"
		h.writeError(w, r, traceID, http.StatusTooManyRequests, xcode.TooManyRequests.Code(), "rate limit exceeded")
		return
	}

	poolType := r.URL.Query().Get("pool_type")
	if poolType == "" {
		poolType = r.URL.Query().Get("poolType")
	}
	if poolType == "" {
		poolType = "CPMM"
	}
	req := &trade.GetCpmmTokensRequest{
		ChainId: int32(parseInt64(r.URL.Query().Get("chain_id"), 100000)),
		PageNo:  uint32(parseInt64(r.URL.Query().Get("page_no"), 1)),
		PageSize: func() uint32 {
			val := parseInt64(r.URL.Query().Get("page_size"), 0)
			if val <= 0 {
				return 0
			}
			return uint32(val)
		}(),
		PoolType: poolType,
	}
	if statuses, ok := r.URL.Query()["status"]; ok {
		req.Statuses = statuses
	}
	if ts := parseInt64(r.URL.Query().Get("updated_after_unix"), 0); ts > 0 {
		req.UpdatedAfterUnix = ts
	}

	resp, err := h.trade.GetCpmmTokens(r.Context(), req)
	if err != nil {
		status = "error"
		h.writeError(w, r, traceID, http.StatusBadGateway, xcode.RPCError.Code(), err.Error())
		return
	}

	h.writeSuccess(w, r, traceID, map[string]interface{}{
		"tokens":   resp.GetTokens(),
		"pageNo":   resp.GetPageNo(),
		"pageSize": resp.GetPageSize(),
		"total":    resp.GetTotal(),
	})
}

func (h *Handler) handleFeeTiers(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.NewString()
	start := time.Now()
	status := "success"
	defer func() {
		h.track(h.cfg.FeeTiersPath, status, start)
	}()
	if !h.allow(r, h.cfg.FeeTiersPath, h.cfg.FeeTiersLimit) {
		status = "rate_limited"
		h.writeError(w, r, traceID, http.StatusTooManyRequests, xcode.TooManyRequests.Code(), "rate limit exceeded")
		return
	}

	poolType := r.URL.Query().Get("pool_type")
	if poolType == "" {
		poolType = r.URL.Query().Get("poolType")
	}
	req := &trade.GetCpmmFeeTiersRequest{
		PoolType: poolType,
	}
	resp, err := h.trade.GetCpmmFeeTiers(r.Context(), req)
	if err != nil {
		status = "error"
		h.writeError(w, r, traceID, http.StatusBadGateway, xcode.RPCError.Code(), err.Error())
		return
	}

	h.writeSuccess(w, r, traceID, map[string]interface{}{
		"version": resp.GetVersion(),
		"tiers":   resp.GetTiers(),
	})
}

func (h *Handler) allow(r *http.Request, path string, limit int) bool {
	if limit <= 0 {
		return true
	}
	key := fmt.Sprintf("%s:%s", path, clientIP(r))
	return h.limiter.Allow(key, limit, time.Minute)
}

func (h *Handler) writeSuccess(w http.ResponseWriter, r *http.Request, traceID string, data map[string]interface{}) {
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", int(h.cacheTTL.Seconds())))
	data["traceId"] = traceID
	h.writeJSON(w, r, data)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, traceID string, status, code int, msg string) {
	w.WriteHeader(status)
	h.writeJSON(w, r, map[string]interface{}{
		"code":    code,
		"error":   msg,
		"traceId": traceID,
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

type createPoolBody struct {
	ChainID        int32  `json:"chain_id"`
	PoolType       string `json:"pool_type"`
	BaseTokenMint  string `json:"base_token_mint"`
	QuoteTokenMint string `json:"quote_token_mint"`
	BaseAmount     string `json:"base_amount"`
	QuoteAmount    string `json:"quote_amount"`
	InitialPrice   string `json:"initial_price"`
	ConfigIndex    int32  `json:"config_index"`
	StartTime      int64  `json:"start_time"`
	UserWallet     string `json:"user_wallet_address"`
	FeeTierBps     int32  `json:"fee_tier_bps"`
	PriceMode      string `json:"price_mode"`
	PriceMin       string `json:"price_min"`
	PriceMax       string `json:"price_max"`
	Amount0        string `json:"amount_0"`
	Amount1        string `json:"amount_1"`
	RangeMode      string `json:"range_mode"`
	SlippageBps    uint32 `json:"slippage_bps"`
}

func (h *Handler) handleCreatePool(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.NewString()
	start := time.Now()
	status := "success"
	defer func() {
		h.track(h.cfg.CreatePoolPath, status, start)
	}()
	if !h.allow(r, h.cfg.CreatePoolPath, h.cfg.CreateLimit) {
		status = "rate_limited"
		h.writeError(w, r, traceID, http.StatusTooManyRequests, xcode.TooManyRequests.Code(), "rate limit exceeded")
		return
	}

	var body createPoolBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		status = "error"
		h.writeError(w, r, traceID, http.StatusBadRequest, xcode.RequestErr.Code(), "invalid body")
		return
	}

	poolType := strings.ToUpper(strings.TrimSpace(body.PoolType))
	if poolType == "" {
		poolType = "CPMM"
	}

	var (
		txHash   string
		txType   string
		txBase64 string
		expires  int64
	)

	switch poolType {
	case "CLMM":
		resp, err := h.trade.CreateClmmPool(r.Context(), &trade.CreateClmmPoolRequest{
			ChainId:           body.ChainID,
			PoolType:          poolType,
			BaseTokenMint:     body.BaseTokenMint,
			QuoteTokenMint:    body.QuoteTokenMint,
			InitialPrice:      body.InitialPrice,
			FeeTierBps:        body.FeeTierBps,
			ConfigIndex:       body.ConfigIndex,
			StartTime:         body.StartTime,
			UserWalletAddress: body.UserWallet,
			PriceMode:         body.PriceMode,
			PriceMin:          body.PriceMin,
			PriceMax:          body.PriceMax,
			Amount_0:          body.Amount0,
			Amount_1:          body.Amount1,
			RangeMode:         body.RangeMode,
			SlippageBps:       body.SlippageBps,
		})
		if err != nil {
			status = "error"
			h.writeError(w, r, traceID, http.StatusBadGateway, xcode.RPCError.Code(), err.Error())
			return
		}
		txHash = resp.GetTxHash()
		txType = resp.GetTxType()
		txBase64 = resp.GetTxBase64()
		expires = resp.GetExpiresAt()
	default:
		resp, err := h.trade.CreateCpmmPool(r.Context(), &trade.CreateCpmmPoolRequest{
			ChainId:           body.ChainID,
			PoolType:          poolType,
			BaseTokenMint:     body.BaseTokenMint,
			QuoteTokenMint:    body.QuoteTokenMint,
			BaseAmount:        body.BaseAmount,
			QuoteAmount:       body.QuoteAmount,
			InitialPrice:      body.InitialPrice,
			ConfigIndex:       body.ConfigIndex,
			StartTime:         body.StartTime,
			UserWalletAddress: body.UserWallet,
		})
		if err != nil {
			status = "error"
			h.writeError(w, r, traceID, http.StatusBadGateway, xcode.RPCError.Code(), err.Error())
			return
		}
		txHash = resp.GetTxHash()
		txType = resp.GetTxType()
		txBase64 = resp.GetTxBase64()
		expires = resp.GetExpiresAt()
	}

	h.writeJSON(w, r, map[string]interface{}{
		"traceId":   traceID,
		"txHash":    txHash,
		"txType":    txType,
		"txBase64":  txBase64,
		"expiresAt": expires,
	})
}

func parseInt64(val string, def int64) int64 {
	if val == "" {
		return def
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handler) track(path, status string, start time.Time) {
	if path == "" {
		return
	}
	middleware.RecordLiquidityMetric(path, status, time.Since(start))
}
