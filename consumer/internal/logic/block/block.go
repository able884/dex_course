package block

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"richcode.cc/dex/consumer/internal/svc"
	constants "richcode.cc/dex/pkg/constrants"

	"richcode.cc/dex/consumer/internal/config"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gorilla/websocket"
	"github.com/mr-tron/base58"
	"github.com/panjf2000/ants/v2"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

var ErrServiceStop = errors.New("service stop")

type BlockService struct {
	Name string
	sc   *svc.ServiceContext
	c    *client.Client
	logx.Logger
	workerPool *ants.Pool
	slotChan   chan uint64
	Conn       *websocket.Conn
	ctx        context.Context
	cancel     func(err error)
	name       string
}

func (s *BlockService) Stop() {
	s.cancel(ErrServiceStop)
	if s.Conn != nil {
		err := s.Conn.WriteMessage(websocket.TextMessage, []byte("{\"id\":1,\"jsonrpc\":\"2.0\",\"method\": \"blockUnsubscribe\", \"params\": [0]}\n"))
		if err != nil {
			s.Error("programUnsubscribe", err)
		}
		_ = s.Conn.Close()
	}
}

func (s *BlockService) Start() {
	s.GetBlockFromHttp()
}

func NewBlockService(sc *svc.ServiceContext, name string, slotChan chan uint64, index int) *BlockService {
	ctx, cancel := context.WithCancelCause(context.Background())
	pool, _ := ants.NewPool(5)
	solService := &BlockService{
		c: client.New(rpc.WithEndpoint(config.FindChainRpcByChainId(constants.SolChainIdInt)), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
		})),
		sc:         sc,
		Logger:     logx.WithContext(context.Background()).WithFields(logx.Field("service", fmt.Sprintf("%s-%v", name, index))),
		slotChan:   slotChan,
		workerPool: pool,
		ctx:        ctx,
		cancel:     cancel,
		name:       name,
	}
	return solService
}

func (s *BlockService) GetBlockFromHttp() {
	fmt.Print("block service is started")
	ctx := s.ctx
	for {
		select {
		case <-s.ctx.Done():
			fmt.Print("block service is stopped")
			return
		case slot, ok := <-s.slotChan:
			if !ok {
				fmt.Print("slotChan is closed")
				return
			}
			//打印当前最新slot
			fmt.Println("consume slot is:", slot)

			// 使用协程池处理块信息，让读取slot和处理块信息解耦，避免处理块信息过慢导致读取slot的goroutine过多
			threading.RunSafe(func() {
				s.ProcessBlock(ctx, int64(slot))
			})
		}
	}
}

func (s *BlockService) ProcessBlock(ctx context.Context, slot int64) {
	if slot == 0 {
		return
	}

	blockInfo, err := GetSolBlockInfoDelay(s.sc.GetSolClient(), ctx, uint64(slot))
	if err != nil || blockInfo == nil {
		fmt.Println("err :", err)
		return
	}

	slice.ForEach(blockInfo.Transactions, func(index int, tx client.BlockTransaction) {
		if len(tx.Transaction.Signatures) > 0 {
			sig858 := base58.Encode(tx.Transaction.Signatures[0])
			fmt.Println("Transaction signature: ", sig858)
			// 交易过滤（合约id）/指令过滤
		}
	})
}
