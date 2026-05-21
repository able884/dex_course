package config

import (
	"github.com/SpectatorNan/gorm-zero/gormc/config/mysql"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Mysql MysqlConf
	Sol   SolConf `json:"Sol,optional"`
}

type MysqlConf struct {
	Master mysql.Mysql   `json:"Master"`
	Slave  []mysql.Mysql `json:"Slave,optional"`
}

type SolConf struct {
	ChainId int64    `json:"ChainId,optional"`
	Enable  bool     `json:"Enable,optional"`
	NodeUrl []string `json:"NodeUrl,optional"`
}
