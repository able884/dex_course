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
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm"
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

// 负责从 slot 通道接收区块编号，获取区块数据，解码交易，提取成交信息并存储
type BlockService struct {
	Name        string              // 服务名称，用于区分不同的服务实例（如 "block-real", "block-failed"）
	sc          *svc.ServiceContext // 服务上下文，包含数据库连接、配置等
	c           *client.Client      // Solana RPC 客户端，用于获取区块数据
	logx.Logger                     // 日志记录器
	workerPool  *ants.Pool          // 协程池，用于并发处理任务（当前未使用）
	slotChan    chan uint64         // Slot 通道，接收待处理的区块编号
	slot        uint64              // 当前处理的 slot 编号
	Conn        *websocket.Conn     // WebSocket 连接（当前未使用）
	solPrice    float64             // SOL 价格缓存
	ctx         context.Context     // 上下文，用于控制服务生命周期
	cancel      func(err error)     // 取消函数，用于停止服务
	name        string              // 服务名称（与 Name 字段重复，可考虑移除）
}

// Stop 停止区块处理服务
// 当服务组调用 Stop() 时，会取消上下文并关闭 WebSocket 连接
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

// Start 启动区块处理服务
// 该方法由 go-zero 的 ServiceGroup 在启动时调用，会阻塞在 GetBlockFromHttp() 中持续处理区块
func (s *BlockService) Start() {
	s.GetBlockFromHttp()
}

// NewBlockService 创建新的区块处理服务实例
// 参数:
//   - sc: 服务上下文，包含数据库连接、配置等
//   - name: 服务名称，用于区分不同用途的服务（如 "block-real" 处理实时区块，"block-failed" 处理失败区块）
//   - slotChan: Slot 通道，用于接收待处理的区块编号
//   - index: 服务实例索引，用于区分同一类型的多个实例
//
// 返回: 初始化好的 BlockService
// 返回: 初始化好的 BlockService 实例
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

// GetBlockFromHttp 从 slot 通道持续接收区块编号并处理
// 这是服务的主循环，会一直运行直到服务停止或通道关闭
// 每个接收到的 slot 都会在独立的 goroutine 中异步处理，实现并发处理多个区块
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

// FillTradeWithPairInfo 填充成交信息的区块相关字段
// 为每个解码出的成交记录填充 slot、区块号、链 ID 等基础信息
func (s *BlockService) FillTradeWithPairInfo(trade *types.TradeWithPair, slot int64) {
	trade.Slot = slot
	trade.BlockNum = slot
	trade.ChainIdInt = constants.SolChainIdInt
	trade.ChainId = constants.SolChainId
}

// ProcessBlock 处理单个区块的核心方法
//  1. 获取区块基础信息（时间、高度等）
//  2. 计算 SOL 价格
//  3. 解码所有交易，提取成交信息
//  4. 处理 Mint/Burn 事件
//  5. 保存成交数据和 Token 账户快照
//  6. 更新池子信息
//  7. 保存区块记录
//  7. 保存区块记录
func (s *BlockService) ProcessBlock(ctx context.Context, slot int64) {
	beginTime := time.Now()

	if slot == 0 {
		return
	}
	// Step0: 初始化区块对象，并将状态默认标记为失败，后续流程成功再回写
	// 采用"失败优先"策略，只有处理成功才更新状态，确保异常情况下能正确标记失败
	s.slot = uint64(slot)

	// 初始化区块数据库模型，默认状态为失败
	block := &solmodel.Block{
		Slot:   slot,
		Status: constants.BlockFailed, // 默认设置为失败，后续根据获取块信息的结果更新状态
	}

	// 通过 RPC 获取区块详细信息（包含所有交易数据）
	// GetSolBlockInfoDelay 会延迟 1 秒后获取，避免 RPC 调用过于频繁导致的 "Block not available for slot" 错误
	blockInfo, err := GetSolBlockInfoDelay(s.sc.GetSolClient(), ctx, uint64(slot))
	if err != nil || blockInfo == nil {
		fmt.Println("get block info error:", err)

		// Solana 节点可能返回 "was skipped" 错误，表示该 slot 被跳过（可能是网络问题或节点同步问题）
		// 这种情况直接标记为跳槽并落库，不进行后续处理
		if strings.Contains(err.Error(), "was skipped") {
			block.Status = constants.BlockSkipped
		}

		// 保存失败或跳槽的区块记录
		_ = s.sc.BlockModel.Insert(ctx, block)
		return
	}

	// Step1: 同步区块基础信息（时间、高度等）
	// BlockTime 和 BlockHeight 是指针类型，需要先判断是否为 nil 再解引用
	if blockInfo.BlockTime != nil {
		block.BlockTime = *blockInfo.BlockTime
	}

	if blockInfo.BlockHeight != nil {
		block.BlockHeight = *blockInfo.BlockHeight
	}

	// 标记区块已成功处理
	block.Status = constants.BlockProcessed

	/*
		    make() 是 Go 语言中用于初始化内置引用类型的函数
			// 1. slice（切片）
			s := make([]int, 5, 10)    // 长度5，容量10的int切片

			// 2. map（映射）
			m := make(map[string]int)  // string→int的映射

			// 3. channel（通道）
			ch := make(chan int, 10)   // 缓冲大小为10的int通道
	*/
	// Step2: 计算当期 SOL 价格，为后续成交估值提供基准
	// 价格计算逻辑：从区块中的 SOL-USDT/USDC 转账交易中提取价格
	// 公式：(USDT|USDC) 数量 / SOL 数量 = SOL 价格
	// tokenAccountMap 用于存储交易前后所有 Token 账户的余额变化，供价格计算使用
	var tokenAccountMap = make(map[string]*TokenAccount)
	solPrice := s.GetBlockSolPrice(ctx, blockInfo, tokenAccountMap)

	// 如果当前区块无法计算出价格，使用缓存的上一区块价格
	if solPrice == 0 {
		solPrice = s.solPrice
	}
	block.SolPrice = solPrice

	// Step3: 遍历区块中的所有交易，解码并提取成交信息
	// 预分配容量为 1000 的切片，减少内存重新分配
	// 初始化一个存放交易信息的切片，初始容量设为1000
	// 后续会把从区块中解析出来的交易(TradeWithPair)加入到这个切片中，便于统一处理（如分类落库等）
	trades := make([]*types.TradeWithPair, 0, 1000)
	slice.ForEach(blockInfo.Transactions, func(index int, tx client.BlockTransaction) {

		// 构造解码上下文，包含区块信息、交易数据、Token 账户映射等
		decodeTx := &DecodedTx{
			BlockDb:         block,           // 区块数据库模型
			Tx:              &tx,             // 当前交易
			TxIndex:         index,           // 交易在区块中的索引
			TokenAccountMap: tokenAccountMap, // Token 账户余额映射
		}

		// 解码交易，提取成交信息
		// 一个交易可能包含多个指令，每个指令可能产生一个或多个成交记录
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

		// 过滤掉 nil 的成交记录，并为有效记录填充区块信息
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

	// Step4: 将成交按交易对（Pair）归类，方便后续批量写入数据库
	// 同一个交易对的多个成交记录会被分组到一起，提高写入效率
	tradeMap := make(map[string][]*types.TradeWithPair)

	// 统计不同 Swap 类型的成交数量（用于监控）
	pumpSwapCount := 0
	pumpFunCount := 0

	// 按交易对地址分组成交记录
	for _, trade := range trades {
		if len(trade.PairAddr) > 0 {
			tradeMap[trade.PairAddr] = append(tradeMap[trade.PairAddr], trade)
		}
	}

	// 统计不同 Swap 的成交数量
	for _, value := range tradeMap {
		if value[0].SwapName == constants.PumpFun {
			pumpFunCount++
			continue
		}
		if value[0].SwapName == constants.PumpSwap {
			pumpSwapCount++
			continue
		}
	}

	// 处理 Mint（铸造）事件：更新 Token 总量
	// Mint 是指创建新的代币，会增加代币的总供应量
	{
		tokenMints := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenMint {
				return true
			}
			return false
		})

		s.UpdateTokenMints(ctx, tokenMints)
		s.Infof("processBlock:%v UpdateTokenMints size: %v, dur: %v, tokenMints: %v", slot, len(tokenMints), time.Since(beginTime), len(tokenMints))
	}

	// 处理 Burn（销毁）事件：更新 Token 总量
	// Burn 是指销毁代币，会减少代币的总供应量
	{
		tokenBurns := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenBurn {
				return true
			}
			return false
		})

		s.UpdateTokenBurns(ctx, tokenBurns)
		s.Infof("processBlock:%v UpdateTokenBurns size: %v, dur: %v, tokenBurns: %v", slot, len(tokenBurns), time.Since(beginTime), len(tokenBurns))
	}

	// Step5 & Step6: 并发保存数据，提高处理效率
	// 使用 RoutineGroup 管理多个并发任务，等待所有任务完成后再继续

	// 任务1：保存成交信息和 Token 账户快照
	group := threading.NewRoutineGroup()
	group.RunSafe(func() {
		// 按交易对批量保存成交记录
		s.SaveTrades(ctx, constants.SolChainIdInt, tradeMap)
		s.Infof("processBlock:%v saveTrades tx_size: %v, dur: %v, trade_size: %v", slot, len(blockInfo.Transactions), time.Since(beginTime), len(trades))

		// 保存 Token 账户余额快照（用于后续查询和分析）
		s.SaveTokenAccounts(ctx, trades, tokenAccountMap)
		s.Infof("processBlock:%v saveTokenAccounts tx_size: %v, dur: %v, trade_size: %v", slot, len(blockInfo.Transactions), time.Since(beginTime), len(trades))
	})

	// 任务2：更新 PumpSwap 池子信息
	group.RunSafe(func() {
		// 针对 PumpSwap 类型的交易，更新池子的元数据（流动性、价格等）
		slice.ForEach(trades, func(_ int, trade *types.TradeWithPair) {
			if trade.SwapName == constants.PumpSwap || trade.SwapName == "PumpSwap" {
				// 只处理买卖交易，不处理其他类型
				if trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell {
					if err = s.SavePumpSwapPoolInfo(ctx, trade); err != nil {
						s.Errorf("processBlock:%v SavePumpSwapPoolInfo err: %v", slot, err)
					}
				}

			}
		})
	})

	// 等待所有并发任务完成
	group.Wait()

	// Step7: 保存区块记录到数据库，标记处理完成
	// 此时所有数据都已保存，区块状态为已处理（BlockProcessed）
	err = s.sc.BlockModel.Insert(ctx, block)
	if err != nil {
		s.Error("insert block error", err)
	}
}

// DecodeTx 解码单个交易，提取其中的成交信息
// Solana 交易可能包含多个指令（instructions），每个指令可能产生一个成交记录
// 参数:
//   - ctx: 上下文
//   - sc: 服务上下文 文
//   - dtx: 解码上下文，包含交易数据、区块信
//   - dtx: 解码上下文，包含交易数据、区块信息等
//
// 返回: 成交记录列表
// 返回: 成交记录列表和错误
func DecodeTx(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx) (trades []*types.TradeWithPair, err error) {

	// 参数校验
	if dtx.Tx == nil || dtx.BlockDb == nil {
		return
	}

	tx := dtx.Tx
	// 计算交易哈希：使用第一个签名的 base58 编码
	dtx.TxHash = base58.Encode(tx.Transaction.Signatures[0])

	// 如果交易执行失败，不处理（Solana 交易可能执行失败但仍被包含在区块中）
	if tx.Meta.Err != nil {
		return
	}

	// 构建内部指令映射表，方便后续快速查找
	// 内部指令是程序在执行主指令时产生的子指令
	dtx.InnerInstructionMap = GetInnerInstructionMap(tx)

	// 遍历交易中的所有指令，逐个解码
	// 每个指令可能对应一个链上程序调用（如 Swap、Transfer 等）
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		var trade *types.TradeWithPair
		// 根据指令类型调用相应的解码器
		trade, err = DecodeInstruction(ctx, sc, dtx, inst, i)
		if err != nil {
			if strings.Contains(err.Error(), "unknow program") || strings.Contains(err.Error(), "not support instruction") {
				continue
			}
			logx.Errorf("decode instruction error: %v, hash: %v, index: %d", err, dtx.TxHash, i)
			continue
		}
		// 将解码出的成交记录添加到列表（可能为 nil，表示该指令不产生成交）
		trades = append(trades, trade)
	}
	return
}

// DecodeInstruction 根据指令类型解码指令，提取成交信息
// Solana 上的每个指令都对应一个链上程序（Program），不同的程序有不同的解码逻辑
// 参数:
//   - ctx: 上下文
//   - sc: 服务上下文
//   - dtx: 解码上下文

//   - index: 指令在交易中的索引
//
// tion: 要解码的指令
//
//   - ind
//   - ind
//   - instruction: 要解码的指令
//   - index: 指令在交易中的索引
//
// 返回: 成交记录（可能为 nil）和错误
func DecodeInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, index int) (trade *types.TradeWithPair, err error) {
	// 参数校验
	if len(dtx.Tx.AccountKeys) == 0 {
		return nil, errors.New("account keys is empty")
	}

	// 检查程序 ID 索引是否越界
	if int(instruction.ProgramIDIndex) >= len(dtx.Tx.AccountKeys) {
		return nil, fmt.Errorf("program ID index %d out of bounds for account keys length %d", instruction.ProgramIDIndex, len(dtx.Tx.AccountKeys))
	}

	tx := dtx.Tx
	var innerInstructions *client.InnerInstruction
	// 获取指令对应的程序地址
	program := tx.AccountKeys[instruction.ProgramIDIndex].String()

	// 根据程序地址选择相应的解码器
	switch program {
	case ProgramStrPumpFun:
		// PumpFun 程序：用于代币发行和交易的平台
		trade, err = DecodePumpFunInstruction(ctx, sc, dtx, instruction, index)
		return
	case ProgramStrPumpFunAMM:
		// PumpFun AMM 程序：自动做市商，处理 Swap 交易
		decoder := &PumpAmmDecoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
		}
		trade, err = decoder.DecodePumpFunAMMInstruction()
		return
	case common.TokenProgramID.String():
		// Token 程序：标准代币程序，处理代币转账、铸造、销毁等
		trade, err = DecodeTokenProgramInstruction(ctx, sc, dtx, instruction, index)

		if trade != nil {
			fmt.Println("find token program tx: %v", trade.TxHash)
		}
		return trade, err
	case common.Token2022ProgramID.String():
		// Token2022 程序：Token 程序的升级版，支持更多功能
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
	case clmm.ProgramRaydiumConcentratedLiquidity.String():
		decoder := &ConcentratedLiquidityDecoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
			innerInstruction:    innerInstructions,
		}
		trade, err = decoder.DecodeRaydiumConcentratedLiquidityInstruction()
		if err != nil {
			logx.Errorf("error find inner clmm tx: %v, err : %v", dtx.TxHash, err)
			return nil, err
		}
		logx.Infof("find inner clmm tx: %v, pairInfo: %#v", dtx.TxHash, trade.PairInfo)
		return trade, nil
	default:
		// 未知程序，返回错误（这是正常的，不是所有程序都需要处理）
		return nil, ErrUnknowProgram
	}
}

// : 起始索引
// GetInnerInstructionByInner 从指令列表中提取内部指令
// 内部指令是程序在执行主指令时产生的子指令，用于更细粒度的操作追踪
// : 起始索引
// 参数:
// - innerLen: 内部指令长度
//
//   - instructions: 指令列表
//   - startIndex: 起始索引
//   - innerLen: 内部指令长度
//
// 返回: 内部指令对象或 nil
func GetInnerInstructionByInner(instructions []solTypes.CompiledInstruction, startIndex, innerLen int) *client.InnerInstruction {
	// 边界检查
	if startIndex+innerLen+1 > len(instructions) {
		return nil
	}
	// 创建内部指令对象
	innerInstruction := &client.InnerInstruction{
		Index: uint64(instructions[startIndex].ProgramIDIndex),
	}
	// 提取内部指令列表
	for i := 0; i < innerLen; i++ {
		innerInstruction.Instructions = append(innerInstruction.Instructions, instructions[startIndex+i+1])
	}
	return innerInstruction
}

//  1. 计算 SOL 价格（通过 SOL-USDT/USDC 转账）
//  2. 追踪持仓变化
//
// ken 账户映射（可能为 nil）
//
//  3. 识别代币转账、铸造、销毁等操作
//
//  2. 追踪持仓变化
//
// ken 账户映射（可能为 nil）
//
//  3. 识别代币转账、铸造、销毁等操作
//
// 参数:
//   - tx: 区块交易对象
//   - tokenAccountMapIn: 输入的 Token 账户映射（可能为 nil）
//
// 返回:
//   - tokenAccountMap: 填充后的 Token 账户映射
//   - hasTokenChange: 是否有 Token 余额变化
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

// DecodeTokenTransfer 解码 Token 转账指令
// 支持标准 Token 程序和 Token2022 程序的转账指令
// 参数:
//   - accountKeys: 账户公钥列表
//   - instruction: 要解码的指令
//
// 返回: 转账参数和错误
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

// 初始化指令：
//   - InitializeAccount: 标准初始化
//   - Init
//
// 码 Token 账户初始化指令
// 当创建新的 Token 账户时，需要记录账户信息（mint
// DecodeInitAccountInstruction 解码 Token 账户初始化指令
// 当创建新的 Token 账户时，需要记录账户信息（mint、owner 等）
// 支持三种初始化指令：
//   - InitializeAccount: 标准初始化
//   - InitializeAccount2: 带 owner 参数的初始化
//   - InitializeAccount3: 另一种初始化方式
//
// 参数:
//   - tx: 区块交易对象
//   - tokenAccountMap: Token 账户映射，用于存储新创建的账户信息
//   - instruction: 初始化指令
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
