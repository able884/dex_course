package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/zeromicro/go-zero/core/logx"
	pb "richcode.cc/dex/consumer/internal/mq/proto"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/types"
)

// ProtocolType 协议类型
type ProtocolType string

const (
	ProtocolPumpFun           ProtocolType = "PumpFun"
	ProtocolPumpSwap          ProtocolType = "PumpSwap"
	ProtocolRaydiumCLMM       ProtocolType = "Raydium_CLMM"
	ProtocolRaydiumCPMM       ProtocolType = "Raydium_CPMM"
	ProtocolRaydiumCPMMDevNet ProtocolType = "Raydium_CPMM_DevNet"
	ProtocolSPLToken          ProtocolType = "SPL_Token"
	ProtocolSPLToken2022      ProtocolType = "SPL_Token_2022"
)

// ProcessResult 处理结果
type ProcessResult struct {
	Protocol     ProtocolType
	Success      bool
	Error        error
	ProcessedTxs int
	DataToSave   interface{} // 需要保存的数据（[]*types.TradeWithPair）
	ProcessTime  time.Duration
}

// ProtocolProcessData 协议处理数据
type ProtocolProcessData struct {
	Trades          []*types.TradeWithPair
	Block           *solmodel.Block
	SolPrice        float64
	TokenAccountMap map[string]*TokenAccount
}

// ProtocolWorkerPool 协议工作池
type ProtocolWorkerPool struct {
	sc     *svc.ServiceContext
	logger logx.Logger
	ctx    context.Context
}

// NewProtocolWorkerPool 创建协议工作池
func NewProtocolWorkerPool(sc *svc.ServiceContext, ctx context.Context) *ProtocolWorkerPool {
	return &ProtocolWorkerPool{
		sc:     sc,
		logger: logx.WithContext(ctx).WithFields(logx.Field("service", "protocol_worker_pool")),
		ctx:    ctx,
	}
}

// ProcessBlock 并发处理区块中的所有协议交易
func (p *ProtocolWorkerPool) ProcessBlock(ctx context.Context, blockMsg *pb.BlockMessage) ([]*ProcessResult, error) {
	if blockMsg == nil {
		return nil, fmt.Errorf("block message is nil")
	}

	// 预处理：构建公共数据
	blockDb, tokenAccountMap, solPrice := p.preprocessBlock(blockMsg)

	// 如果没有协议分组，返回空结果
	if len(blockMsg.ProtocolGroups) == 0 {
		return []*ProcessResult{}, nil
	}

	// 使用 WaitGroup 等待所有 worker 完成
	var wg sync.WaitGroup
	resultChan := make(chan *ProcessResult, len(blockMsg.ProtocolGroups))

	// 为每个协议分组启动一个 goroutine 处理
	for protocolName, protocolGroup := range blockMsg.ProtocolGroups {
		wg.Add(1)

		// 并发处理每个协议分组
		go func(pName string, pGroup *pb.ProtocolTransactionGroup) {
			defer wg.Done()

			result := p.processProtocolGroup(ctx, pName, pGroup, blockMsg, &ProtocolProcessData{
				Block:           blockDb,
				SolPrice:        solPrice,
				TokenAccountMap: tokenAccountMap,
			})
			resultChan <- result
		}(protocolName, protocolGroup)
	}

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	results := make([]*ProcessResult, 0, len(blockMsg.ProtocolGroups))
	for result := range resultChan {
		results = append(results, result)
	}

	// 统计结果
	successCount := 0
	totalTxs := 0
	for _, result := range results {
		if result.Success {
			successCount++
			totalTxs += result.ProcessedTxs
		} else {
			p.logger.Errorf("Protocol processing failed: protocol=%s, error=%v",
				result.Protocol, result.Error)
		}
	}

	return results, nil
}

// processProtocolGroup 处理单个协议分组
func (p *ProtocolWorkerPool) processProtocolGroup(
	ctx context.Context,
	protocolName string,
	protocolGroup *pb.ProtocolTransactionGroup,
	blockMsg *pb.BlockMessage,
	processData *ProtocolProcessData,
) *ProcessResult {
	startTime := time.Now()

	result := &ProcessResult{
		Protocol: ProtocolType(protocolName),
	}

	// 解码该协议分组的所有交易
	trades := make([]*types.TradeWithPair, 0, len(protocolGroup.Transactions))

	for txIndex, rawTx := range protocolGroup.Transactions {
		// 转换为 client.BlockTransaction
		blockTx := p.convertToBlockTransaction(rawTx, blockMsg)

		// 构建解码上下文
		decodeTx := &DecodedTx{
			BlockDb:         processData.Block,
			Tx:              &blockTx,
			TxIndex:         txIndex,
			TokenAccountMap: processData.TokenAccountMap,
			SolPrice:        processData.SolPrice,
		}

		// 解码交易
		txTrades, err := DecodeTx(ctx, p.sc, decodeTx)
		if err != nil {
			// 跳过无法解码的交易
			p.logger.Debugf("Failed to decode transaction for protocol %s: %v", protocolName, err)
			continue
		}

		// 填充交易信息
		for _, trade := range txTrades {
			if trade != nil {
				trade.Slot = int64(blockMsg.Slot)
				trade.BlockNum = int64(blockMsg.Slot)
				trade.ChainIdInt = constants.SolChainIdInt
				trade.ChainId = constants.SolChainId
				trades = append(trades, trade)
			}
		}
	}

	// 保存处理结果
	processData.Trades = trades
	result.Success = true
	result.DataToSave = processData
	result.ProcessedTxs = len(trades)
	result.ProcessTime = time.Since(startTime)

	if len(trades) > 0 {
		p.logger.Infof("Processed protocol group: %s, txs=%d, trades=%d, time=%v",
			protocolName, len(protocolGroup.Transactions), len(trades), result.ProcessTime)
	}

	return result
}

// preprocessBlock 预处理区块：构建公共数据（不解析交易）
func (p *ProtocolWorkerPool) preprocessBlock(blockMsg *pb.BlockMessage) (
	blockDb *solmodel.Block,
	tokenAccountMap map[string]*TokenAccount,
	solPrice float64,
) {
	// 1. 构建区块数据库记录
	blockDb = &solmodel.Block{
		Slot:      int64(blockMsg.Slot),
		BlockTime: time.Unix(blockMsg.BlockTime, 0),
		SolPrice:  blockMsg.SolPrice,
		Status:    constants.BlockProcessed,
	}

	// 2. 构建 TokenAccountMap
	tokenAccountMap = make(map[string]*TokenAccount)
	for addr, ta := range blockMsg.TokenAccounts {
		tokenAccountMap[addr] = &TokenAccount{
			Owner:               ta.Owner,
			TokenAccountAddress: ta.TokenAccountAddress,
			TokenAddress:        ta.TokenAddress,
			TokenSymbol:         ta.TokenSymbol,
			TokenName:           ta.TokenName,
			TokenDecimal:        uint8(ta.TokenDecimal),
			PreValue:            ta.PreValue,
			PostValue:           ta.PostValue,
			Init:                ta.Init,
			Closed:              ta.Closed,
			PreValueUIString:    ta.PreValueUiString,
			PostValueUIString:   ta.PostValueUiString,
		}
	}

	solPrice = blockMsg.SolPrice

	return blockDb, tokenAccountMap, solPrice
}

// convertToBlockTransaction 将 protobuf RawTransaction 转换为 client.BlockTransaction
func (p *ProtocolWorkerPool) convertToBlockTransaction(rawTx *pb.RawTransaction, blockMsg *pb.BlockMessage) client.BlockTransaction {
	// 转换 Signatures
	signatures := make([]solTypes.Signature, len(rawTx.Signatures))
	for i, sig := range rawTx.Signatures {
		copy(signatures[i][:], sig)
	}

	// 转换 AccountKeys
	accountKeys := make([]common.PublicKey, len(rawTx.AccountKeys))
	for i, key := range rawTx.AccountKeys {
		pk := common.PublicKeyFromBytes(key)
		accountKeys[i] = pk
	}

	// 转换 Message
	var message solTypes.Message
	if rawTx.Message != nil {
		// 转换 Message AccountKeys
		msgAccountKeys := make([]common.PublicKey, len(rawTx.Message.AccountKeys))
		for i, key := range rawTx.Message.AccountKeys {
			pk := common.PublicKeyFromBytes(key)
			msgAccountKeys[i] = pk
		}

		// 转换 Instructions
		instructions := make([]solTypes.CompiledInstruction, len(rawTx.Message.Instructions))
		for i, inst := range rawTx.Message.Instructions {
			accounts := make([]int, len(inst.Accounts))
			for j, acc := range inst.Accounts {
				accounts[j] = int(acc)
			}

			instructions[i] = solTypes.CompiledInstruction{
				ProgramIDIndex: int(inst.ProgramIdIndex),
				Accounts:       accounts,
				Data:           inst.Data,
			}
		}

		message = solTypes.Message{
			Accounts:        msgAccountKeys,
			RecentBlockHash: string(rawTx.Message.RecentBlockhash),
			Instructions:    instructions,
		}
	}

	// 转换 Meta
	var meta *client.TransactionMeta
	if rawTx.Meta != nil {
		meta = &client.TransactionMeta{
			Fee:          rawTx.Meta.Fee,
			PreBalances:  rawTx.Meta.PreBalances,
			PostBalances: rawTx.Meta.PostBalances,
		}

		// 转换 Inner Instructions
		if len(rawTx.Meta.InnerInstructions) > 0 {
			innerInsts := make([]client.InnerInstruction, len(rawTx.Meta.InnerInstructions))
			for i, inner := range rawTx.Meta.InnerInstructions {
				instructions := make([]solTypes.CompiledInstruction, len(inner.Instructions))
				for j, inst := range inner.Instructions {
					accounts := make([]int, len(inst.Accounts))
					for k, acc := range inst.Accounts {
						accounts[k] = int(acc)
					}
					instructions[j] = solTypes.CompiledInstruction{
						ProgramIDIndex: int(inst.ProgramIdIndex),
						Accounts:       accounts,
						Data:           inst.Data,
					}
				}
				innerInsts[i] = client.InnerInstruction{
					Index:        inner.Index,
					Instructions: instructions,
				}
			}
			meta.InnerInstructions = innerInsts
		}
	}

	return client.BlockTransaction{
		Transaction: solTypes.Transaction{
			Signatures: signatures,
			Message:    message,
		},
		Meta:        meta,
		AccountKeys: accountKeys,
	}
}
