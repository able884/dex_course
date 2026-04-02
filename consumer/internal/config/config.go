package config

import (
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"

	constants "richcode.cc/dex/pkg/constants"
)

var Cfg Config

var (
	SolRpcUseFrequency int
)

type Config struct {
	zrpc.RpcServerConf

	MySQLConfig MySQLConfig `json:"Mysql"`

	Sol Chain `json:"Sol,optional"`

	Consumer consumer `json:"Consumer,optional"`
}

type MySQLConfig struct {
	User     string `json:"User"     json:",env=MYSQL_USER"`
	Password string `json:"Password" json:",env=MYSQL_PASSWORD"`
	Host     string `json:"Host"     json:",env=MYSQL_HOST"`
	Port     int    `json:"Port"     json:",env=MYSQL_PORT"`
	DBName   string `json:"DBname"   json:",env=MYSQL_DBNAME"`
}

type consumer struct {
	Concurrency             int `json:"Concurrency"  json:",env=CCONSUMER_CONCURRENCY"`
	NotCompletedConcurrency int `json:"NotCompletedConcurrency" json:",env=CONSUMER_NOTCOMPLETED_CONCURRENCY"`
}

type Chain struct {
	ChainId    int64    `json:"ChainId"              json:",env=SOL_CHAINID"`
	NodeUrl    []string `json:"NodeUrl"              json:",env=SOL_NODEURL"`
	MEVNodeUrl string   `json:"MevNodeUrl,optional"  json:",env=SOL_MEVNODEURL"`
	WSUrl      string   `json:"WSUrl,optional"       json:",env=SOL_WSURL"`
	StartBlock uint64   `json:"StartBlock,optional"  json:",env=SOL_STARTBLOCK"`
}

func SaveConf(cf Config) {
	Cfg = cf
}

/*
该方法的作用是根据链ID查找对应的RPC地址，并且实现了简单的轮询机制来分配RPC请求，以避免过度使用单个RPC地址。
*/
func FindChainRpcByChainId(chainId int) (rpc string) {
	var rpcs []string
	var useFrequency *int

	switch chainId {
	case constants.SolChainIdInt:
		rpcs = Cfg.Sol.NodeUrl
		useFrequency = &SolRpcUseFrequency
	default:
		logx.Error("No Rpc Config")
		return
	}

	if len(rpcs) == 0 {
		logx.Error("No Rpc Config")
		return
	}

	*useFrequency++
	index := *useFrequency % len(rpcs)
	rpc = rpcs[index]
	return
}
