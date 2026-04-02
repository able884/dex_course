package rds

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
)

var (
	Client *redis.Redis
)

func Init(c *redis.RedisKeyConf) {
	redisClient := redis.MustNewRedis(redis.RedisConf{
		Host:        c.Host,
		Type:        c.Type,
		Pass:        c.Pass,
		Tls:         c.Tls,
		PingTimeout: c.PingTimeout,
	})
	Client = redisClient
}
