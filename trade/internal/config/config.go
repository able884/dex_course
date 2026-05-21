package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	MarketService zrpc.RpcClientConf `json:"market_service"`
	MySQLConfig   MySQLConfig        `json:"Mysql"`
	SolConfig     ChainConfig        `json:"Sol"`
	SimulateOnly  bool               `json:"simulate_only,optional" json:",env=SIMULATE_ONLY"`
	Ipfs          IpfsConfig         `json:"Ipfs"`
	LiquidityCfg  LiquidityConfig    `json:"Liquidity"`
}

type ChainConfig struct {
	ChainId uint64   `json:"ChainId" json:",env=SOL_CHAINID"`
	Enable  bool     `json:"Enable"  json:",env=SOL_ENABLE"`
	NodeUrl []string `json:"NodeUrl" json:",env=SOL_NODEURL"`
	Jito    string   `json:"Jito"    json:",env=SOL_JITO"`
	UUID    string   `json:"UUID"    json:",env=SOL_UUID"`
}

type MySQLConfig struct {
	User     string `json:"User"     json:",env=MYSQL_USER"`
	Password string `json:"Password" json:",env=MYSQL_PASSWORD"`
	Host     string `json:"Host"     json:",env=MYSQL_HOST"`
	Port     int    `json:"Port"     json:",env=MYSQL_PORT"`
	DBName   string `json:"DBname"   json:",env=MYSQL_DBNAME"`
}

type IpfsConfig struct {
	Provider         string `json:"Provider"`         // nftstorage | pinata (当前默认 nftstorage)
	NFTStorageApiKey string `json:"NFTStorageApiKey"` // NFT.Storage API Key
	Gateway          string `json:"Gateway"`          // eg: https://ipfs.io/ipfs/
}

type LiquidityConfig struct {
	DSN             string `json:"DSN"`
	TokenPageSize   int    `json:"TokenPageSize"`
	CacheTTLSeconds int    `json:"CacheTTLSeconds"`
}
