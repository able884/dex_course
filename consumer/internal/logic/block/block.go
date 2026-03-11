package block

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constrants"

	"richcode.cc/dex/consumer/internal/config"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gorilla/websocket"
	"github.com/panjf2000/ants/v2"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

var ErrServiceStop = errors.New("service stop")

var ErrUnknowProgram = errors.New("unknow program")

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
			// fmt.Println("consume slot is:", slot)

			// 使用协程池处理块信息，让读取slot和处理块信息解耦，避免处理块信息过慢导致读取slot的goroutine过多
			threading.RunSafe(func() {
				s.ProcessBlock(ctx, int64(slot))
			})
		}
	}
}

func (s *BlockService) ProcessBlock(ctx context.Context, slot int64) {
	beginTime := time.Now()

	if slot == 0 {
		return
	}

	block := &solmodel.Block{
		Slot:   slot,
		Status: constants.BlockFailed, // 默认设置为失败，后续根据获取块信息的结果更新状态
	}

	blockInfo, err := GetSolBlockInfoDelay(s.sc.GetSolClient(), ctx, uint64(slot))
	if err != nil || blockInfo == nil {
		fmt.Println("err :", err)
		// 如果是空区块，则跳过
		if strings.Contains(err.Error(), "was skipped") {
			block.Status = constants.BlockSkipped
		}
		_ = s.sc.BlockModel.Insert(ctx, block)
		return
	}

	// Set the block time from the retrieved block info
	if blockInfo.BlockTime != nil {
		block.BlockTime = *blockInfo.BlockTime
		blockTime := blockInfo.BlockTime.Format("2006-01-02 15:04:05")
		s.Infof("processBlock:%v getBlockInfo blockTime: %v,cur: %v, dur: %v, queue size: %v", slot, blockTime, time.Now().Format("15:04:05"), time.Since(beginTime), len(s.slotChan))
	} else {
		s.Infof("processBlock:%v getBlockInfo blockTime is nil,cur: %v, dur: %v, queue size: %v", slot, time.Now().Format("15:04:05"), time.Since(beginTime), len(s.slotChan))
	}

	if blockInfo.BlockHeight != nil {
		block.BlockHeight = *blockInfo.BlockHeight
	}
	block.Status = constants.BlockProcessed

	// TODDO: 获取 sol 价格
	block.SolPrice = 0

	slice.ForEach(blockInfo.Transactions, func(index int, tx client.BlockTransaction) {
		// if len(tx.Transaction.Signatures) > 0 {
		// 	sig858 := base58.Encode(tx.Transaction.Signatures[0])
		// 	fmt.Println("Transaction signature: ", sig858)
		// 	// 交易过滤（合约id）/指令过滤
		// }
		DecodeTx(&tx)
	})

	// 保存块信息到数据库
	err = s.sc.BlockModel.Insert(ctx, block)
	if err != nil {
		s.Error("insert block error", err)
	}
}

func DecodeTx(tx *client.BlockTransaction) {
	if tx == nil {
		return
	}

	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		err := DecodeInstruction(inst, tx)
		if err != nil {
			return
		}
	}
}

func DecodeInstruction(inst *types.CompiledInstruction, tx *client.BlockTransaction) (err error) {
	if inst == nil {
		return errors.New("instruction is null")
	}

	if len(tx.AccountKeys) == 0 {
		return errors.New("account keys is empty")
	}

	program := tx.AccountKeys[inst.ProgramIDIndex].String()

	switch program {
	case ProgramStrPumpFun:
		return DecodePumpFunInstruction(inst, tx)
	case ProgramStrPumpFunAMM:
		return DecodePumpFunAMMInstruction(inst, tx)
	default:
		return ErrUnknowProgram
	}
}
