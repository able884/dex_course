package config

import (
	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	Name  string    `json:",env=NAME"`
	Host  string    `json:",default=0.0.0.0,env=HOST"`
	Port  int       `json:",default=8086,env=PORT"`
	Redis RedisConf `json:"Redis"`
}

type RedisConf struct {
	Host     string `json:",env=REDIS_HOST"`
	Port     int    `json:",env=REDIS_PORT"`
	Password string `json:",optional,env=REDIS_PASSWORD"`
	DB       int    `json:",default=0,env=REDIS_DB"`
}

// LoadConfig 从配置文件或环境变量加载配置
func LoadConfig(configFile string) (Config, error) {
	var c Config
	err := conf.Load(configFile, &c)
	return c, err
}
