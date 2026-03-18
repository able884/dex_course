package block

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constrants"

	"richcode.cc/dex/consumer/internal/config"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	solTypes "github.com/blocto/solana-go-sdk/types"
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
	solPrice   float64
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

	// 获取 sol 价格
	var tokenAccountMap = make(map[string]*TokenAccount)
	solPrice := s.GetBlockSolPrice(ctx, blockInfo, tokenAccountMap)
	if solPrice == 0 {
		solPrice = s.solPrice
	}
	// 区块 -> 交易 -> 转账（Transfer）SOL-(USDT/USDC) -> (USDT|USDC) / SOL = 价格
	block.SolPrice = solPrice

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

// GetInnerInstructionByInner 从innerInstruction中获取指定长度的指令
func GetInnerInstructionByInner(instructions []solTypes.CompiledInstruction, startIndex, innerLen int) *client.InnerInstruction {
	if startIndex+innerLen+1 > len(instructions) {
		return nil
	}
	innerInstruction := &client.InnerInstruction{
		Index: uint64(instructions[startIndex].ProgramIDIndex),
	}
	for i := 0; i < innerLen; i++ {
		innerInstruction.Instructions = append(innerInstruction.Instructions, instructions[startIndex+i+1])
	}
	return innerInstruction
}

// FillTokenAccountMap 填充交易中的tokenAccount 数据
func FillTokenAccountMap(tx *client.BlockTransaction, tokenAccountMapIn map[string]*TokenAccount) (tokenAccountMap map[string]*TokenAccount, hasTokenChange bool) {
	if tokenAccountMapIn == nil {
		tokenAccountMapIn = make(map[string]*TokenAccount)
	}
	tokenAccountMap = tokenAccountMapIn
	// 遍历执行交易前各个代币账户余额  	PreTokenBalances：交易前各个代币账户余额
	for _, pre := range tx.Meta.PreTokenBalances {
		var tokenAccount = tx.AccountKeys[pre.AccountIndex].String()
		preValue, _ := strconv.ParseInt(pre.UITokenAmount.Amount, 10, 64)
		tokenAccountMap[tokenAccount] = &TokenAccount{
			Owner:               pre.Owner,                  // owner address
			TokenAccountAddress: tokenAccount,               // token account address
			TokenAddress:        pre.Mint,                   // token address
			TokenDecimal:        pre.UITokenAmount.Decimals, // token decimal
			PreValue:            preValue,
			Closed:              true,
			PreValueUIString:    pre.UITokenAmount.UIAmountString,
		}
	}
	// 遍历执行交易后各个代币账户余额  	PostTokenBalances：交易后各个代币账户余额
	for _, post := range tx.Meta.PostTokenBalances {
		var tokenAccount = tx.AccountKeys[post.AccountIndex].String()
		postValue, _ := strconv.ParseInt(post.UITokenAmount.Amount, 10, 64)
		if tokenAccountMap[tokenAccount] != nil {
			tokenAccountMap[tokenAccount].Closed = false
			tokenAccountMap[tokenAccount].PostValue = postValue
			if tokenAccountMap[tokenAccount].PostValue != tokenAccountMap[tokenAccount].PreValue {
				hasTokenChange = true
			}
		} else {
			hasTokenChange = true
			tokenAccountMap[tokenAccount] = &TokenAccount{
				Owner:               post.Owner,                  // owner address
				TokenAccountAddress: tokenAccount,                // token account address
				TokenAddress:        post.Mint,                   // token address
				TokenDecimal:        post.UITokenAmount.Decimals, // token decimal
				PostValue:           postValue,                   // token balance
				Init:                true,
				PostValueUIString:   post.UITokenAmount.UIAmountString,
			}
		}
	}
	for i := range tx.Transaction.Message.Instructions {
		instruction := &tx.Transaction.Message.Instructions[i]
		program := tx.AccountKeys[instruction.ProgramIDIndex].String()
		// 如果程序地址是token程序地址，则解析初始化账户指令，获取token账户信息，填充tokenAccountMap
		if program == ProgramStrToken {
			DecodeInitAccountInstruction(tx, tokenAccountMap, instruction)
		}
	}
	for _, instructions := range tx.Meta.InnerInstructions {
		for i := range instructions.Instructions {
			instruction := instructions.Instructions[i]
			program := tx.AccountKeys[instruction.ProgramIDIndex].String()
			if program == ProgramStrToken {
				DecodeInitAccountInstruction(tx, tokenAccountMap, &instruction)
			}
		}
	}
	tokenDecimalMap := make(map[string]uint8)
	for _, v := range tokenAccountMap {
		if v.TokenDecimal != 0 {
			tokenDecimalMap[v.TokenAddress] = v.TokenDecimal
		}
	}
	for _, v := range tokenAccountMap {
		if v.TokenDecimal == 0 {
			v.TokenDecimal = tokenDecimalMap[v.TokenAddress]
		}
	}
	return
}

// DecodeTokenTransfer 解析transfer指令
func DecodeTokenTransfer(accountKeys []common.PublicKey, instruction *solTypes.CompiledInstruction) (transfer *token.TransferParam, err error) {
	transfer = &token.TransferParam{}
	// 解析transfer指令
	if accountKeys[instruction.ProgramIDIndex].String() == common.Token2022ProgramID.String() {
		// Instruction Accounts 参与指令的账户在交易账户列表中的索引，source、destination、authority 三个账户
		if len(instruction.Accounts) < 3 {
			err = errors.New("not enough accounts")
			return
		}
		// Instruction Data ：指令的二进制数据（传给程序的具体参数arguments） 两个参数，discriminator描述符、Amount数量
		if len(instruction.Data) < 1 {
			err = errors.New("data len to0 small")
			return
		}
		// 第一个参数discriminator描述符 判断指令类型是 transfer 还是 transferChecked(3和12 只是索引值)
		if instruction.Data[0] == byte(token.InstructionTransfer) {
			// transfer指令长度一共9个字节
			if len(instruction.Data) != 9 {
				err = errors.New("data len not equal 9")
				return
			}
			if len(instruction.Accounts) < 3 {
				err = errors.New("account len too small")
				return
			}
			transfer.From = accountKeys[instruction.Accounts[0]]                // 发送方账户
			transfer.To = accountKeys[instruction.Accounts[1]]                  // 接收方账户
			transfer.Auth = accountKeys[instruction.Accounts[2]]                // 授权签名账户
			transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:9]) // 转账金额
		} else if instruction.Data[0] == byte(token.InstructionTransferChecked) {
			// transferChecked指令长度一共10个字节
			if len(instruction.Data) < 10 {
				err = errors.New("data len not equal 10")
				return
			}
			if len(instruction.Accounts) < 4 {
				err = errors.New("account len too small")
				return
			}
			transfer.From = accountKeys[instruction.Accounts[0]] // 发送方账户
			// mint := accountKeys[instruction.Accounts[1]]		// 代币Mint账户
			transfer.To = accountKeys[instruction.Accounts[2]]                   // 接收方账户
			transfer.Auth = accountKeys[instruction.Accounts[3]]                 // 授权签名账户
			transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:10]) // 转账金额
			// decimal := instruction.Data[10]
		} else {
			err = errors.New("not transfer Instruction")
			return
		}
		return transfer, nil
	}

	if accountKeys[instruction.ProgramIDIndex].String() != ProgramStrToken { // 检查指令是否是代币程序
		err = errors.New("not token program")
		return
	}
	if len(instruction.Accounts) < 3 {
		err = errors.New("not enough accounts")
		return
	}
	if len(instruction.Data) < 1 {
		err = errors.New("data len to0 small")
		return
	}
	if instruction.Data[0] == byte(token.InstructionTransfer) { // 检查指令是否是转账指令
		if len(instruction.Data) != 9 {
			err = errors.New("data len not equal 9")
			return
		}
		if len(instruction.Accounts) < 3 {
			err = errors.New("account len too small")
			return
		}
		transfer.From = accountKeys[instruction.Accounts[0]]                // 发送方账户
		transfer.To = accountKeys[instruction.Accounts[1]]                  // 接收方账户
		transfer.Auth = accountKeys[instruction.Accounts[2]]                // 授权签名账户
		transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:9]) // 转账金额
	} else if instruction.Data[0] == byte(token.InstructionTransferChecked) { // 检查指令是否是转账指令（Checked）
		if len(instruction.Data) != 10 {
			err = errors.New("data len not equal 10")
			return
		}
		if len(instruction.Accounts) < 4 {
			err = errors.New("account len too small")
			return
		}
		transfer.From = accountKeys[instruction.Accounts[0]] // 发送方账户
		// mint := accountKeys[instruction.Accounts[1]] // 代币Mint账户
		transfer.To = accountKeys[instruction.Accounts[2]]                   // 接收方账户
		transfer.Auth = accountKeys[instruction.Accounts[3]]                 // 授权签名账户
		transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:10]) // 转账金额
		// decimal := instruction.Data[10]
	} else {
		err = errors.New("not transfer Instruction")
		return
	}

	return
}

// DecodeInitAccountInstruction 解析初始化账户指令
// 输入 tx 为当前处理的区块交易信息，tokenAccountMap 为已有的token账户映射表，instruction 为当前指令
// 该方法会根据不同的初始化账户指令类型（InitializeAccount, InitializeAccount2, InitializeAccount3）更新tokenAccountMap
func DecodeInitAccountInstruction(tx *client.BlockTransaction, tokenAccountMap map[string]*TokenAccount, instruction *solTypes.CompiledInstruction) {
	if len(instruction.Data) == 0 {
		return
	}
	var mint, tokenAccount, owner string
	switch token.Instruction(instruction.Data[0]) {
	// init account
	case token.InstructionInitializeAccount:
		if len(instruction.Accounts) < 3 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = tx.AccountKeys[instruction.Accounts[2]].String()
	case token.InstructionInitializeAccount2:
		if len(instruction.Accounts) < 2 || len(instruction.Data) < 33 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(instruction.Data[1:]).String()
	case token.InstructionInitializeAccount3:
		if len(instruction.Accounts) < 2 || len(instruction.Data) < 33 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(instruction.Data[1:]).String()
	default:
		return
	}
	if tokenAccountMap[tokenAccount] != nil && tokenAccountMap[tokenAccount].TokenAddress == mint {
		return
	} else {
		tokenAccountMap[tokenAccount] = &TokenAccount{
			Init:                true,
			Owner:               owner,
			TokenAddress:        mint,
			TokenAccountAddress: tokenAccount,
			TokenDecimal:        0,
			PreValue:            0,
			PostValue:           0,
		}
	}
}
