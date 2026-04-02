package config

import (
	"github.com/SpectatorNan/gorm-zero/gormc/config/mysql"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Mysql MysqlConf
}

type MysqlConf struct {
	Master mysql.Mysql   `json:"Master"`
	Slave  []mysql.Mysql `json:"Slave,optional"`
}
