package svc

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	solclient "github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"richcode.cc/dex/consumer/internal/config"
	"richcode.cc/dex/model/solmodel"
)

type ServiceContext struct {
	Config         config.Config
	solClientLock  sync.Mutex
	solClientIndex int
	solClient      *solclient.Client
	solClients     []*solclient.Client
	BlockModel     solmodel.BlockModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config: c,
	}
}

func NewSolServiceContext(c config.Config) *ServiceContext {
	logx.MustSetup(c.Log)

	logx.Infof("newSolServiceContext: config:%#v", c)

	var solClients []*solclient.Client
	for _, node := range c.Sol.NodeUrl {
		client.New(rpc.WithEndpoint(node), rpc.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}))
		solClients = append(solClients, client.NewClient(node))
	}
	// fmt.Println("solClients: ", c.Sol.NodeUrl)

	// Initialize database connection
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.MySQLConfig.User,
		c.MySQLConfig.Password,
		c.MySQLConfig.Host,
		c.MySQLConfig.Port,
		c.MySQLConfig.DBName,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %v", err))
	}

	// Initialize BlockModel
	blockModel := solmodel.NewBlockModel(db)

	return &ServiceContext{
		Config:     c,
		solClients: solClients,
		BlockModel: blockModel,
	}
}

func (sc *ServiceContext) GetSolClient() *client.Client {
	sc.solClientLock.Lock()
	defer sc.solClientLock.Unlock()
	sc.solClientIndex++
	index := sc.solClientIndex % len(sc.solClients)
	sc.solClient = sc.solClients[index]
	return sc.solClients[index]
}
