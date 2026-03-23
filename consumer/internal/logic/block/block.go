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
	"richcode.cc/dex/pkg/types"

	"richcode.cc/dex/consumer/internal/config"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gorilla/websocket"
	"github.com/mr-tron/base58"
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
	slot       uint64
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

func (s *BlockService) FillTradeWithPairInfo(trade *types.TradeWithPair, slot int64) {
	trade.Slot = slot
	trade.BlockNum = slot
	trade.ChainIdInt = constants.SolChainIdInt
	trade.ChainId = constants.SolChainId
}

func (s *BlockService) ProcessBlock(ctx context.Context, slot int64) {
	beginTime := time.Now()

	if slot == 0 {
		return
	}
	s.slot = uint64(slot)

	block := &solmodel.Block{
		Slot:   slot,
		Status: constants.BlockFailed, // 默认设置为失败，后续根据获取块信息的结果更新状态
	}

	blockInfo, err := GetSolBlockInfoDelay(s.sc.GetSolClient(), ctx, uint64(slot))
	if err != nil || blockInfo == nil {
		fmt.Println("get block info error:", err)

		// Anchor 会返回 was skipped 的 slot，直接标记为跳槽并落库
		if strings.Contains(err.Error(), "was skipped") {
			block.Status = constants.BlockSkipped
		}
		_ = s.sc.BlockModel.Insert(ctx, block)
		return
	}

	// Step1: 同步 blockTime、blockHeight 等基础信息
	if blockInfo.BlockTime != nil {
		block.BlockTime = *blockInfo.BlockTime
	}

	if blockInfo.BlockHeight != nil {
		block.BlockHeight = *blockInfo.BlockHeight
	}
	block.Status = constants.BlockProcessed

	// Step2: 计算当期 SOL 价格，为后续成交估值提供基准
	var tokenAccountMap = make(map[string]*TokenAccount)
	solPrice := s.GetBlockSolPrice(ctx, blockInfo, tokenAccountMap)
	if solPrice == 0 {
		solPrice = s.solPrice
	}
	// 区块 -> 交易 -> 转账（Transfer）SOL-(USDT/USDC) -> (USDT|USDC) / SOL = 价格
	block.SolPrice = solPrice

	// 初始化一个存放交易信息的切片，初始容量设为1000
	// 后续会把从区块中解析出来的交易(TradeWithPair)加入到这个切片中，便于统一处理（如分类落库等）
	trades := make([]*types.TradeWithPair, 0, 1000)

	// 遍历区块中的每一笔链上交易，进行处理
	slice.ForEach(blockInfo.Transactions, func(index int, tx client.BlockTransaction) {

		// Step3: 构造解码上下文。这里新建一个 DecodedTx 结构体用来保存当前交易的相关信息：
		// BlockDb            当前处理的区块指针
		// Tx                 当前遍历到的链上交易指针
		// TxIndex            交易在区块内的序号
		// TokenAccountMap    本次区块处理中维护的token账户（复用，提升效率）
		decodeTx := &DecodedTx{
			BlockDb:         block,
			Tx:              &tx,
			TxIndex:         index,
			TokenAccountMap: tokenAccountMap,
		}

		// 解码链上交易，返回解码后的 TradeWithPair 切片
		trade, err := DecodeTx(ctx, s.sc, decodeTx)
		if err != nil {
			// 如果是未识别的合约（unknow program），则直接跳过此条
			if strings.Contains(err.Error(), "unknow program") {
				return
			}
			// 其他解码出错则输出日志并跳过
			fmt.Println("decode tx failed: ", err.Error())
			return
		}

		// 对解码出来的 trade 结果再次过滤，只保留非空项；
		// 并填充 TradeWithPair 的补充信息（如引用了Slot等元数据）
		trade = slice.Filter(trade, func(index int, item *types.TradeWithPair) bool {
			if item == nil {
				return false
			}
			s.FillTradeWithPairInfo(item, slot)
			return true
		})

		// 将本笔交易对应的所有成交记入 trades 切片，后面统一处理
		trades = append(trades, trade...)
	})

	// Step4: 将成交按 Pair 归类，方便后续批量写入
	tradeMap := make(map[string][]*types.TradeWithPair)

	pumpSwapCount := 0
	pumpFunCount := 0

	for _, trade := range trades {
		if len(trade.PairAddr) > 0 {
			tradeMap[trade.PairAddr] = append(tradeMap[trade.PairAddr], trade)
		}
	}

	for _, value := range tradeMap {
		// 简单统计不同 Swap 的成交数量，方便监控
		if value[0].SwapName == constants.PumpFun {
			pumpFunCount++
			continue
		}
		if value[0].SwapName == constants.PumpSwap {
			pumpSwapCount++
			continue
		}
	}

	{
		// 额外挑出 Mint 行为，触发 Token 总量刷新
		tokenMints := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenMint {
				return true
			}
			return false
		})

		s.UpdateTokenMints(ctx, tokenMints)
		s.Infof("processBlock:%v UpdateTokenMints size: %v, dur: %v, tokenMints: %v", slot, len(tokenMints), time.Since(beginTime), len(tokenMints))
	}

	{
		// 额外挑出 Burn 行为，触发 Token 总量刷新
		tokenBurns := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenBurn {
				return true
			}
			return false
		})

		s.UpdateTokenBurns(ctx, tokenBurns)
		s.Infof("processBlock:%v UpdateTokenBurns size: %v, dur: %v, tokenBurns: %v", slot, len(tokenBurns), time.Since(beginTime), len(tokenBurns))
	}

	//并发处理： 保存交易信息，保存token账户信息
	group := threading.NewRoutineGroup()
	group.RunSafe(func() {
		// Step5: 写入成交信息 & TokenAccount 快照
		s.SaveTrades(ctx, constants.SolChainIdInt, tradeMap)
		s.Infof("processBlock:%v saveTrades tx_size: %v, dur: %v, trade_size: %v", slot, len(blockInfo.Transactions), time.Since(beginTime), len(trades))

		s.SaveTokenAccounts(ctx, trades, tokenAccountMap)
		s.Infof("processBlock:%v saveTokenAccounts tx_size: %v, dur: %v, trade_size: %v", slot, len(blockInfo.Transactions), time.Since(beginTime), len(trades))
	})

	// pump swap
	group.RunSafe(func() {
		// Step6: 针对 PumpSwap 交易补充池子元数据
		slice.ForEach(trades, func(_ int, trade *types.TradeWithPair) {
			if trade.SwapName == constants.PumpSwap || trade.SwapName == "PumpSwap" {
				if trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell {
					if err = s.SavePumpSwapPoolInfo(ctx, trade); err != nil {
						s.Errorf("processBlock:%v SavePumpSwapPoolInfo err: %v", slot, err)
					}
				}

			}
		})
	})

	group.Wait()

	// Step7: 区块落库，标识处理完成
	err = s.sc.BlockModel.Insert(ctx, block)
	if err != nil {
		s.Error("insert block error", err)
	}
}

// 解析每一个区块的交易
func DecodeTx(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx) (trades []*types.TradeWithPair, err error) {
	if dtx.Tx == nil || dtx.BlockDb == nil {
		return
	}

	tx := dtx.Tx
	dtx.TxHash = base58.Encode(tx.Transaction.Signatures[0])

	if tx.Meta.Err != nil {
		return
	}

	dtx.InnerInstructionMap = GetInnerInstructionMap(tx)

	// 遍历交易内所有指令，挨个解析生成业务侧的成交结构
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		var trade *types.TradeWithPair
		trade, err = DecodeInstruction(ctx, sc, dtx, inst, i)
		if err != nil {
			return nil, err
		}
		trades = append(trades, trade)
	}
	return
}

func DecodeInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, index int) (trade *types.TradeWithPair, err error) {
	if len(dtx.Tx.AccountKeys) == 0 {
		return nil, errors.New("account keys is empty")
	}

	if int(instruction.ProgramIDIndex) >= len(dtx.Tx.AccountKeys) {
		return nil, fmt.Errorf("program ID index %d out of bounds for account keys length %d", instruction.ProgramIDIndex, len(dtx.Tx.AccountKeys))
	}

	tx := dtx.Tx
	var innerInstructions *client.InnerInstruction
	program := tx.AccountKeys[instruction.ProgramIDIndex].String()

	switch program {
	case ProgramStrPumpFun:
		trade, err = DecodePumpFunInstruction(instruction, tx)
		return
	case ProgramStrPumpFunAMM:
		decoder := &PumpAmmDecoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
		}
		trade, err = decoder.DecodePumpFunAMMInstruction()
		return
	case common.TokenProgramID.String():
		trade, err = DecodeTokenProgramInstruction(ctx, sc, dtx, instruction, index)

		if trade != nil {
			fmt.Println("find token program tx: %v", trade.TxHash)
		}
		return trade, err
	case common.Token2022ProgramID.String():
		innerInstructions = dtx.InnerInstructionMap[index]
		decoder := &Token2022Decoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
			innerInstruction:    innerInstructions,
		}
		trade, err = decoder.DecodeToken2022DecoderInstruction()
		if err != nil {
			logx.Errorf("error find token2022 tx: %v, err : %v", dtx.TxHash, err)
			return nil, err
		}
		logx.Infof("find token2022 tx: %v, pairInfo: %#v", dtx.TxHash, trade.PairInfo)
		return trade, err
	default:
		return nil, ErrUnknowProgram
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
