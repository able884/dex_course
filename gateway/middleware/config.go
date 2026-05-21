package middleware

import (
	"time"

	"github.com/zeromicro/go-zero/gateway"
)

var GlobalConfig Config

// Config gateway
type Config struct {
	gateway.GatewayConf
	Auth struct {
		Prefix    string `json:",env=AUTH_PREFIX"`
		JwtSecret string `json:",env=AUTH_JWTSECRET"`
		TgToken   string `json:",env=AUTH_TGTOKEN"`
	}
	RequestLimit []struct {
		Path string
		QPS  int
	}
	Redis struct {
		Host        string        `json:",env=REDIS_HOST"`
		Port        int           `json:",env=REDIS_PORT"`
		Auth        string        `json:",optional,env=REDIS_AUTH"`
		MaxIdle     int           `json:",env=REDIS_MAXIDLE"`
		MaxActive   int           `json:",env=REDIS_MAXACTIVE"`
		Db          int           `json:",env=REDIS_DB"`
		Type        string        `json:",env=REDIS_TYPE"`
		Tls         bool          `json:",env=REDIS_TLS"`
		PingTimeOut time.Duration `json:",env=REDIS_PINGTIMEOUT"`
	}
	Websocket struct {
		Host string `json:",env=WEBSOCKET_HOST"`
		Path string `json:",env=WEBSOCKET_PATH"`
	}
	Whitelist struct {
		Path []string
	}
	IPRestriction struct {
		IsoCode []string
	}
	Liquidity struct {
		RequireJwt      bool `json:",optional"`
		CacheTTLSeconds int  `json:",default=60"`
		Paths           struct {
			Tokens     string
			FeeTiers   string
			CreatePool string
		}
		RateLimit struct {
			TokensQPS     int
			FeeTiersQPS   int
			CreatePoolQPS int
		}
	}
}
