package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/internal/respx"
	"richcode.cc/dex/pkg/xcode"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

var _ http.Hijacker = (*responseWriter)(nil)

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	// rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	return rw.body.Write(p)
}

func (rw *responseWriter) Body() []byte {
	return rw.body.Bytes()
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("not support")
	}
	return hijacker.Hijack()
}

func WrapResponse(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        logCtx := logx.ContextWithFields(r.Context(), logx.Field("path", r.URL.Path))

		// Log incoming request details
		if r.Body != nil {
			buf := new(bytes.Buffer)
			buf.ReadFrom(r.Body)
			bodyStr := buf.String()
			// Restore the body for downstream handlers
			r.Body = io.NopCloser(bytes.NewBufferString(bodyStr))
		}

		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rw, r)
		if isLiquidityRoute(r.URL.Path) {
			handleLiquidityResponse(w, r, rw)
			return
		}
		if rw.statusCode == http.StatusGatewayTimeout {
			respx.JsonResp(w, r, http.StatusGatewayTimeout, rw.body.String(), nil)
			return
		}
		if rw.statusCode != http.StatusOK {
			code, msg := unwrapHttpStatusCode(rw.body.String())
			if code == xcode.InvalidSignatureError.Code() {
				http.Error(w, "", http.StatusUnauthorized)
				return
			}
			if code == 0 {
				code = xcode.InternalError.Code()
			}
			respx.JsonResp(w, r, code, msg, nil)
			return
		}

		// Special handling for add_liquidity_v1 endpoint
		if strings.Contains(r.URL.Path, "add_liquidity_v1") {

			// This is a special case for AddLiquidityV1 which returns a txHash
			// Extract the txHash directly from the response body
			bodyStr := rw.body.String()

			// Check if it contains a txHash field
			if strings.Contains(bodyStr, "txHash") {
				// Try to parse as JSON first
				var addLiqResp struct {
					TxHash string `json:"txHash"`
				}

				err := json.Unmarshal(rw.Body(), &addLiqResp)
				if err != nil {
					logc.Errorf(logCtx, "Failed to parse response as JSON: %v", err)
				}

				if err == nil && addLiqResp.TxHash != "" {
					// Successfully parsed as JSON
					respData := map[string]interface{}{
						"txHash": addLiqResp.TxHash,
					}
					respx.JsonResp(w, r, xcode.Ok, "", respData)
					return
				}

				// If JSON parsing failed, try to extract the txHash directly
				start := strings.Index(bodyStr, "txHash")

				if start > 0 {
					// Find the value after txHash
					valueStart := strings.Index(bodyStr[start:], ":")

					if valueStart > 0 {
						valueStart = start + valueStart + 1

						// Find the end of the value (either comma, closing brace, or quote)
						valueEnd := -1
						for i := valueStart; i < len(bodyStr); i++ {
							if bodyStr[i] == ',' || bodyStr[i] == '}' || bodyStr[i] == '"' {
								valueEnd = i
								break
							}
						}

						if valueEnd > valueStart {
							txHash := strings.TrimSpace(bodyStr[valueStart:valueEnd])
							// Remove any quotes
							txHash = strings.Trim(txHash, "\"'")

							if txHash != "" {
								respData := map[string]interface{}{
									"txHash": txHash,
								}
								respx.JsonResp(w, r, xcode.Ok, "", respData)
								return
							}
						}
					}
				}
			}
		}

		// Regular JSON response handling
        var resp map[string]interface{}
        err := json.Unmarshal(rw.Body(), &resp)
        if err != nil {
            http.Error(w, err.Error(), http.StatusOK)
            return
		}

		respx.JsonResp(w, r, xcode.Ok, "", resp)
	}
}

func unwrapHttpStatusCode(s string) (int, string) {
	connectIndex := strings.Index(s, "connect")
	if connectIndex >= 0 {
		return xcode.InternalError.Code(), ""
	}

	descIndex := strings.Index(s, "desc = ")
	if descIndex == -1 {
		return 0, s
	}
	desc := strings.TrimSpace(s[descIndex+6:])
	segs := strings.Split(desc, " ")
	code, _ := strconv.Atoi(segs[0])
	message := strings.Join(segs[1:], " ")
	if code == 0 {
		message = desc
	}

	return code, message
}

func isLiquidityRoute(path string) bool {
	return strings.HasPrefix(path, "/liquidity/cpmm/")
}

func handleLiquidityResponse(w http.ResponseWriter, r *http.Request, rw *responseWriter) {
	if rw.statusCode != http.StatusOK {
		var payload map[string]interface{}
		if err := json.Unmarshal(rw.Body(), &payload); err != nil {
			respx.JsonResp(w, r, xcode.ServerErr.Code(), err.Error(), nil)
			return
		}
		code := xcode.ServerErr.Code()
		if val, ok := payload["code"].(float64); ok {
			code = int(val)
		}
		data := map[string]interface{}{}
		if trace, ok := payload["traceId"]; ok {
			data["traceId"] = trace
		}
		message := ""
		if errMsg, ok := payload["error"].(string); ok {
			message = errMsg
		}
		respx.JsonResp(w, r, code, message, data)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rw.Body(), &payload); err != nil {
		respx.JsonResp(w, r, xcode.ServerErr.Code(), err.Error(), nil)
		return
	}
	respx.JsonResp(w, r, xcode.Ok, "", payload)
}
