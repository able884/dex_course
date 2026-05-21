package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/zeromicro/go-zero/core/logc"
	"richcode.cc/dex/pkg/types"

	"github.com/golang-jwt/jwt"
	"github.com/zeromicro/go-zero/core/logx"
)

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logCtx := logx.ContextWithFields(r.Context(),
			logx.Field("method", r.Method),
			logx.Field("host", r.Host),
			logx.Field("path", r.URL.Path),
			logx.Field("remote", r.RemoteAddr))

		logc.Infof(logCtx, "auth request: host=%s, path=%s, remote=%s", r.Host, r.URL.Path, r.RemoteAddr)

		whitelistMap := make(map[string]bool)
		for _, v := range GlobalConfig.Whitelist.Path {
			whitelistMap[v] = true
		}

		if whitelistMap[r.URL.Path] {
			handleSkipAuth(next, w, r)
			return
		} else {
			handleAuth(next, w, r)
		}
	}
}

func handleAuth(next http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	authorizationHeader := r.Header.Get("Authorization")
	var userInfo map[string]interface{}
	// Determine which authentication method is used
	getToken := r.URL.Query().Get("token")
	if authorizationHeader == "" && getToken == "" {
		next(w, r)
		return
		// userInfo = handleTgAuth(r)
	} else {
		userInfo = handleJwtAuth(authorizationHeader, getToken)
	}
	if userInfo == nil {
		next(w, r)
		return
	}
	if !appendToGrpcMetadata(r, userInfo) {
		next(w, r)
		return
	}

	// Call the next handler
	next(w, r)
}

func handleSkipAuth(next http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	authorizationHeader := r.Header.Get("Authorization")
	var userInfo map[string]interface{}
	// Determine which authentication method is used
	getToken := r.URL.Query().Get("token")
	if authorizationHeader == "" && getToken == "" {
		next.ServeHTTP(w, r)
		return
	} else {
		userInfo = handleJwtAuth(authorizationHeader, getToken)
	}
	if userInfo == nil {
		next.ServeHTTP(w, r)
		return
	}
	if !appendToGrpcMetadata(r, userInfo) {
		next.ServeHTTP(w, r)
		return
	}
	if !appendToHttpHeader(r, userInfo) {
		next.ServeHTTP(w, r)
		return
	}

	// Call the next handler
	next(w, r)
}

func handleJwtAuth(authorizationHeader string, getToken string) map[string]interface{} {
	secret := GlobalConfig.Auth.JwtSecret
	// Extract and validate JWT token
	var accessToken string

	header := strings.Split(authorizationHeader, " ")
	if len(header) == 2 && header[0] == "Bearer" {
		accessToken = header[1]
	} else {
		accessToken = getToken
	}

	claim := &types.AccountAuthClaims{}
	token, err := jwt.ParseWithClaims(accessToken, claim, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("token parse error")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil
	}
	data, err := json.Marshal(claim)
	if err != nil {
		return nil
	}

	if err != nil || !token.Valid {
		return nil
	}

	userInfo := map[string]interface{}{
		"auth_type": "jwt",
		"data":      accessToken,
		"json":      string(data),
	}
	return userInfo
}

func handleTgAuth(r *http.Request) map[string]interface{} {
	initData := r.Header.Get("InitData")
	if initData == "" {
		initData = r.URL.Query().Get("InitData")
	}

	if len(initData) == 0 {
		return nil
	}

	tgToken := GlobalConfig.Auth.TgToken

	if !validateTelegramInitData(initData, tgToken) {
		return nil
	}

	userInfo := map[string]interface{}{
		"auth_type": "telegram",
		"data":      initData,
	}
	return userInfo
}

func appendToGrpcMetadata(r *http.Request, data map[string]interface{}) bool {
	// Serialize user information to JSON string
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return false
	}
	userInfo := string(dataBytes)
	// Go-zero will recognize this key and append it to gRPC metadata
	r.Header.Set("Grpc-Metadata-User-Info", userInfo)

	return true
}

func appendToHttpHeader(r *http.Request, data map[string]interface{}) bool {
	// Serialize user information to JSON string
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return false
	}
	userInfo := string(dataBytes)
	// Go-zero will recognize this key and append it to gRPC metadata
	r.Header.Set("Http-Metadata-User-Info", userInfo)

	return true
}

func validateTelegramInitData(initData string, botToken string) bool {
	// Parse initData into key-value pairs
	values, err := url.ParseQuery(initData)
	if err != nil {
		logx.Error("Failed to parse initData:", err)
		return false
	}

	// Get hash parameter
	telegramHash := values.Get("hash")
	if telegramHash == "" {
		logx.Error("hash parameter does not exist")
		return false
	}

	// Remove hash parameter
	values.Del("hash")

	// Create data check string
	var dataCheckStrings []string
	for key, val := range values {
		dataCheckStrings = append(dataCheckStrings, fmt.Sprintf("%s=%s", key, val[0]))
	}
	sort.Strings(dataCheckStrings)
	dataCheckString := strings.Join(dataCheckStrings, "\n")

	// Calculate HMAC-SHA256 signature
	secretKey := sha256.Sum256([]byte(botToken))
	h := hmac.New(sha256.New, secretKey[:])
	h.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(h.Sum(nil))

	if calculatedHash != telegramHash {
		logx.Error("hash does not match the official TG signature")
		return false
	}

	return true
}
