package types

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang-jwt/jwt"
	"google.golang.org/grpc/metadata"
	"richcode.cc/dex/pkg/xcode"
)

const AuthInfoCtxKey = "gateway-user-info"

type AccountAuthClaims struct {
	UserId int64 `json:"user_id"`
	jwt.StandardClaims
}

func GetAuthInfo(ctx context.Context) (*AccountAuthClaims, error) {
	value := metadata.ValueFromIncomingContext(ctx, AuthInfoCtxKey)
	if len(value) == 0 {
		return nil, fmt.Errorf("Invalid Signature")
	}

	userInfoMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(value[0]), &userInfoMap)
	if err != nil {
		return nil, xcode.InvalidSignatureError
	}
	userToken, ok := userInfoMap["json"].(string)
	if !ok {
		return nil, fmt.Errorf("Invalid Signature")
	}
	accountAuthClaims := &AccountAuthClaims{}
	err = json.Unmarshal([]byte(userToken), &accountAuthClaims)
	if err != nil {
		return nil, fmt.Errorf("Invalid Signature")
	}
	return accountAuthClaims, nil
}
