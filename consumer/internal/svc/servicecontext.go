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

	"richcode.cc/dex/consumer/internal/config"
)

type ServiceContext struct {
	Config         config.Config
	solClientLock  sync.Mutex
	solClientIndex int
	solClient      *solclient.Client
	solClients     []*solclient.Client
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
	fmt.Println("solClients: ", c.Sol.NodeUrl)
	return &ServiceContext{
		Config:     c,
		solClients: solClients,
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
