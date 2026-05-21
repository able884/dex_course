package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrapResponseLiquiditySuccess(t *testing.T) {
	handler := WrapResponse(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"traceId":"trace-1","tokens":[]}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/liquidity/cpmm/tokens", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	var payload map[string]interface{}
	requireJSON(t, rr.Body.Bytes(), &payload)
	data := payload["data"].(map[string]interface{})
	if data["traceId"] != "trace-1" {
		t.Fatalf("expected traceId to propagate")
	}
}

func TestWrapResponseLiquidityError(t *testing.T) {
	handler := WrapResponse(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"code":515,"error":"backend","traceId":"trace-err"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/liquidity/cpmm/fee-tiers", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	var payload map[string]interface{}
	requireJSON(t, rr.Body.Bytes(), &payload)
	if payload["code"].(float64) != 515 {
		t.Fatalf("expected code 515")
	}
	data := payload["data"].(map[string]interface{})
	if data["traceId"] != "trace-err" {
		t.Fatalf("trace id missing")
	}
}

func requireJSON(t *testing.T, raw []byte, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("json error: %v", err)
	}
}
