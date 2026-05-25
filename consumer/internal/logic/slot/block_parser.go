package slot

import (
	"context"
	"fmt"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/zeromicro/go-zero/core/logx"
	pb "richcode.cc/dex/consumer/internal/mq/proto"
	"richcode.cc/dex/consumer/internal/svc"
)

// BlockParser 区块解析器
type BlockParser struct {
	sc           *svc.ServiceContext
	logger       logx.Logger
	filter       *TransactionFilter
	ctx          context.Context
	lastSolPrice float64 // 缓存上一次的SOL价格
}

// NewBlockParser 创建区块解析器
func NewBlockParser(sc *svc.ServiceContext, ctx context.Context) *BlockParser {
	return &BlockParser{
		sc:     sc,
		logger: logx.WithContext(ctx).WithFields(logx.Field("service", "block_parser")),
		filter: NewTransactionFilter(sc.Config.BlockFilter),
		ctx:    ctx,
	}
}

// ParseBlock 解析区块数据
// 返回 protobuf BlockMessage，准备发送到 MQ
func (p *BlockParser) ParseBlock(block *client.Block, slot uint64) (*pb.BlockMessage, error) {
	if block == nil {
		return nil, fmt.Errorf("block is nil")
	}

	originalTxCount := len(block.Transactions)

	// 1. 过滤并按协议分组交易
	protocolGroups := p.filter.FilterAndGroupByProtocol(block)
	if len(protocolGroups) == 0 {
		return nil, nil
	}

	// 2. 获取区块时间
	var blockTime int64
	if block.BlockTime != nil {
		blockTime = block.BlockTime.Unix()
	} else {
		blockTime = time.Now().Unix()
	}

	// 3. 计算 SOL 价格（使用完整的block数据，不仅仅是过滤后的交易）
	solPrice := p.calculateSOLPrice(block)
	if solPrice > 0 {
		p.lastSolPrice = solPrice
		p.logger.Infof("Slot %d: Calculated SOL price = $%.2f", slot, solPrice)
	} else if p.lastSolPrice > 0 {
		solPrice = p.lastSolPrice
		p.logger.Debugf("Slot %d: Using cached SOL price = $%.2f", slot, solPrice)
	} else {
		solPrice = 160.0 // 使用默认价格
		p.logger.Debugf("Slot %d: Using default SOL price = $%.2f", slot, solPrice)
	}

	// 4. 创建 BlockMessage
	blockMsg := &pb.BlockMessage{
		Slot:                     slot,
		BlockTime:                blockTime,
		BlockHash:                block.Blockhash,
		Transactions:             make([]*pb.RawTransaction, 0),
		TokenAccounts:            make(map[string]*pb.TokenAccount),
		InnerInstructionsMap:     make(map[int32]*pb.InnerInstructions),
		TransactionCount:         0,
		OriginalTransactionCount: int32(originalTxCount),
		SolPrice:                 solPrice,
		ProtocolGroups:           make(map[string]*pb.ProtocolTransactionGroup),
	}

	// 5. 处理每个协议分组
	var totalFilteredTxCount int32
	for protocolName, txs := range protocolGroups {
		protocolGroup := &pb.ProtocolTransactionGroup{
			ProtocolName:     protocolName,
			Transactions:     make([]*pb.RawTransaction, 0, len(txs)),
			TransactionCount: int32(len(txs)),
		}

		// 解析该协议的交易
		for _, tx := range txs {
			rawTx := p.parseTransaction(&tx)
			if rawTx != nil {
				protocolGroup.Transactions = append(protocolGroup.Transactions, rawTx)
				blockMsg.Transactions = append(blockMsg.Transactions, rawTx)
			}

			// 提取公共数据：Token Accounts
			p.extractTokenAccounts(&tx, blockMsg.TokenAccounts)

			// 提取 Inner Instructions（如果有）
			if tx.Meta != nil && len(tx.Meta.InnerInstructions) > 0 {
				for _, innerInst := range tx.Meta.InnerInstructions {
					innerPb := &pb.InnerInstructions{
						Index:        uint64(innerInst.Index),
						Instructions: make([]*pb.CompiledInstruction, 0, len(innerInst.Instructions)),
					}

					for _, inst := range innerInst.Instructions {
						innerPb.Instructions = append(innerPb.Instructions, &pb.CompiledInstruction{
							ProgramIdIndex: uint32(inst.ProgramIDIndex),
							Accounts:       convertIntToUint32(inst.Accounts),
							Data:           inst.Data,
						})
					}

					blockMsg.InnerInstructionsMap[int32(innerInst.Index)] = innerPb
				}
			}
		}

		// 添加协议分组到消息
		blockMsg.ProtocolGroups[protocolName] = protocolGroup
		totalFilteredTxCount += int32(len(txs))
	}

	blockMsg.TransactionCount = totalFilteredTxCount

	// 6. 添加过滤统计信息（基于协议分组计算）
	protocolCounts := make(map[string]int32)
	for protocolName, group := range blockMsg.ProtocolGroups {
		protocolCounts[protocolName] = group.TransactionCount
	}

	var filterRatio float64
	if originalTxCount > 0 {
		filterRatio = float64(totalFilteredTxCount) / float64(originalTxCount)
	}

	blockMsg.FilterStats = &pb.FilterStats{
		TotalTransactions:    int32(originalTxCount),
		FilteredTransactions: totalFilteredTxCount,
		FilterRatio:          filterRatio,
		ProtocolCounts:       protocolCounts,
	}

	return blockMsg, nil
}

// parseTransaction 解析单个交易为 protobuf 格式
func (p *BlockParser) parseTransaction(tx *client.BlockTransaction) *pb.RawTransaction {
	if tx == nil {
		return nil
	}

	// 构建交易消息
	rawTx := &pb.RawTransaction{
		Signatures:  convertSignatures(tx.Transaction.Signatures),
		AccountKeys: convertAccountKeysToBytes(tx.AccountKeys),
	}

	// 构建 Message
	rawTx.Message = &pb.Message{
		AccountKeys:     convertAccountKeysToBytes(tx.AccountKeys),
		RecentBlockhash: []byte(tx.Transaction.Message.RecentBlockHash),
		Instructions:    make([]*pb.CompiledInstruction, 0, len(tx.Transaction.Message.Instructions)),
	}

	// 添加顶层指令
	for _, inst := range tx.Transaction.Message.Instructions {
		rawTx.Message.Instructions = append(rawTx.Message.Instructions, &pb.CompiledInstruction{
			ProgramIdIndex: uint32(inst.ProgramIDIndex),
			Accounts:       convertIntToUint32(inst.Accounts),
			Data:           inst.Data,
		})
	}

	// 添加元数据（如果有）
	if tx.Meta != nil {
		rawTx.Meta = &pb.TransactionMeta{
			Err:               []byte(convertError(tx.Meta.Err)),
			Fee:               tx.Meta.Fee,
			PreBalances:       tx.Meta.PreBalances,
			PostBalances:      tx.Meta.PostBalances,
			PreTokenBalances:  convertTokenBalances(tx.Meta.PreTokenBalances),
			PostTokenBalances: convertTokenBalances(tx.Meta.PostTokenBalances),
		}

		// 添加 Inner Instructions
		if len(tx.Meta.InnerInstructions) > 0 {
			rawTx.Meta.InnerInstructions = make([]*pb.InnerInstructions, 0, len(tx.Meta.InnerInstructions))
			for _, innerInst := range tx.Meta.InnerInstructions {
				innerPb := &pb.InnerInstructions{
					Index:        uint64(innerInst.Index),
					Instructions: make([]*pb.CompiledInstruction, 0, len(innerInst.Instructions)),
				}

				for _, inst := range innerInst.Instructions {
					innerPb.Instructions = append(innerPb.Instructions, &pb.CompiledInstruction{
						ProgramIdIndex: uint32(inst.ProgramIDIndex),
						Accounts:       convertIntToUint32(inst.Accounts),
						Data:           inst.Data,
					})
				}

				rawTx.Meta.InnerInstructions = append(rawTx.Meta.InnerInstructions, innerPb)
			}
		}
	}

	return rawTx
}

// extractTokenAccounts 提取 Token 账户信息
func (p *BlockParser) extractTokenAccounts(tx *client.BlockTransaction, tokenAccounts map[string]*pb.TokenAccount) {
	if tx.Meta == nil {
		return
	}

	// 从 PostTokenBalances 提取 Token 账户信息
	for _, balance := range tx.Meta.PostTokenBalances {
		accountKey := tx.AccountKeys[balance.AccountIndex].String()

		// 跳过已存在的账户（避免重复）
		if _, exists := tokenAccounts[accountKey]; exists {
			continue
		}

		// 提取 UI Token Amount 数据
		postValueUI := balance.UITokenAmount.UIAmountString
		decimals := uint32(balance.UITokenAmount.Decimals)

		tokenAccounts[accountKey] = &pb.TokenAccount{
			Owner:               balance.Owner,
			TokenAccountAddress: accountKey,
			TokenAddress:        balance.Mint,
			TokenDecimal:        decimals,
			PostValueUiString:   postValueUI,
			PreValueUiString:    "", // Will be filled from PreTokenBalances if needed
		}
	}
}

// Helper functions for type conversion

func convertSignatures(sigs []types.Signature) [][]byte {
	result := make([][]byte, len(sigs))
	for i, sig := range sigs {
		result[i] = sig[:]
	}
	return result
}

func convertAccountKeys(keys []common.PublicKey) []string {
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = key.String()
	}
	return result
}

func convertAccountKeysToBytes(keys []common.PublicKey) [][]byte {
	result := make([][]byte, len(keys))
	for i, key := range keys {
		result[i] = key.Bytes()
	}
	return result
}

func convertIntToUint32(data []int) []uint32 {
	result := make([]uint32, len(data))
	for i, v := range data {
		result[i] = uint32(v)
	}
	return result
}

func convertError(err interface{}) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}

func convertTokenBalances(balances []rpc.TransactionMetaTokenBalance) []*pb.TokenBalance {
	if len(balances) == 0 {
		return nil
	}

	result := make([]*pb.TokenBalance, 0, len(balances))
	for _, balance := range balances {
		uiTokenAmount := &pb.UITokenAmount{
			Decimals:       uint32(balance.UITokenAmount.Decimals),
			Amount:         balance.UITokenAmount.Amount,
			UiAmountString: balance.UITokenAmount.UIAmountString,
		}

		result = append(result, &pb.TokenBalance{
			AccountIndex:  uint32(balance.AccountIndex),
			Mint:          balance.Mint,
			Owner:         balance.Owner,
			UiTokenAmount: uiTokenAmount,
		})
	}
	return result
}

// ParseConfig 解析配置
type ParseConfig struct {
	// 是否包含完整的日志消息
	IncludeLogMessages bool
	// 是否包含 Token 余额
	IncludeTokenBalances bool
	// 最大交易数（超过则分批）
	MaxTransactionsPerMessage int
}

// DefaultParseConfig 默认解析配置
var DefaultParseConfig = ParseConfig{
	IncludeLogMessages:        true,
	IncludeTokenBalances:      true,
	MaxTransactionsPerMessage: 1000,
}

// 稳定币DEX程序ID
var (
	programOrca                   = common.PublicKeyFromString("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc")
	programRaydiumConcentratedLiq = common.PublicKeyFromString("CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK")
	programMeteoraDLMM            = common.PublicKeyFromString("LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo")
	programPhoenix                = common.PublicKeyFromString("PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY")

	stableCoinSwapDexes = []common.PublicKey{
		programOrca,
		programRaydiumConcentratedLiq,
		programMeteoraDLMM,
		programPhoenix,
	}

	// 稳定币地址
	tokenWrapSOL = "So11111111111111111111111111111111111111112"
	tokenUSDC    = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	tokenUSDT    = "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"
)

// calculateSOLPrice 计算区块的SOL价格
func (p *BlockParser) calculateSOLPrice(block *client.Block) float64 {
	if block == nil || len(block.Transactions) == 0 {
		return 0
	}

	priceList := make([]float64, 0)

	// 遍历所有交易，查找稳定币DEX的swap交易
	for i := range block.Transactions {
		tx := &block.Transactions[i]

		// 跳过失败的交易
		if tx.Meta != nil && tx.Meta.Err != nil {
			continue
		}

		// 构建TokenAccountMap
		tokenAccountMap := p.buildTokenAccountMap(tx)
		if len(tokenAccountMap) == 0 {
			continue
		}

		accountKeys := tx.AccountKeys

		// 检查顶层指令
		for instIndex, instruction := range tx.Transaction.Message.Instructions {
			programID := accountKeys[instruction.ProgramIDIndex]
			if p.isStableCoinDex(programID) {
				// 查找inner instructions（根据指令索引查找）
				var innerInst *client.InnerInstruction
				if tx.Meta != nil {
					for _, inner := range tx.Meta.InnerInstructions {
						if int(inner.Index) == instIndex {
							innerInst = &inner
							break
						}
					}
				}

				price := p.extractSOLPriceFromSwap(accountKeys, innerInst, tokenAccountMap)
				if price > 0 {
					priceList = append(priceList, price)
				}
			}
		}
	}

	if len(priceList) == 0 {
		return 0
	}

	// 去除最大最小值后求平均
	return removeMinMaxAndAverage(priceList)
}

// buildTokenAccountMap 构建TokenAccountMap
func (p *BlockParser) buildTokenAccountMap(tx *client.BlockTransaction) map[string]string {
	tokenAccountMap := make(map[string]string)

	if tx.Meta == nil {
		return tokenAccountMap
	}

	// 从PostTokenBalances提取mint信息
	for _, balance := range tx.Meta.PostTokenBalances {
		accountKey := tx.AccountKeys[balance.AccountIndex].String()
		tokenAccountMap[accountKey] = balance.Mint
	}

	return tokenAccountMap
}

// isStableCoinDex 判断是否是稳定币DEX
func (p *BlockParser) isStableCoinDex(programID common.PublicKey) bool {
	for _, dex := range stableCoinSwapDexes {
		if programID.String() == dex.String() {
			return true
		}
	}
	return false
}

// extractSOLPriceFromSwap 从swap交易中提取SOL价格
func (p *BlockParser) extractSOLPriceFromSwap(
	accountKeys []common.PublicKey,
	innerInst *client.InnerInstruction,
	tokenAccountMap map[string]string,
) float64 {
	if innerInst == nil || len(innerInst.Instructions) == 0 {
		return 0
	}

	var solAmount uint64
	var usdAmount uint64
	var foundSOL bool
	var foundUSD bool

	// 遍历inner instructions查找transfer指令
	for _, inst := range innerInst.Instructions {
		// 检查是否是transfer指令 (discriminator: 0x03)
		if len(inst.Data) < 9 || inst.Data[0] != 0x03 {
			continue
		}

		if len(inst.Accounts) < 2 {
			continue
		}

		// 解析金额 (little-endian uint64, bytes 1-9)
		amount := uint64(inst.Data[1]) |
			uint64(inst.Data[2])<<8 |
			uint64(inst.Data[3])<<16 |
			uint64(inst.Data[4])<<24 |
			uint64(inst.Data[5])<<32 |
			uint64(inst.Data[6])<<40 |
			uint64(inst.Data[7])<<48 |
			uint64(inst.Data[8])<<56

		// 获取from账户的mint
		fromAccount := accountKeys[inst.Accounts[0]].String()
		mint := tokenAccountMap[fromAccount]

		if mint == tokenWrapSOL {
			solAmount = amount
			foundSOL = true
		} else if mint == tokenUSDC || mint == tokenUSDT {
			usdAmount = amount
			foundUSD = true
		}

		// 如果找到了SOL和USD的转账，计算价格
		if foundSOL && foundUSD && solAmount > 0 {
			// USDC/USDT都是6位小数，SOL是9位小数
			// price = (usdAmount / 10^6) / (solAmount / 10^9) = usdAmount * 1000 / solAmount
			return float64(usdAmount) * 1000.0 / float64(solAmount)
		}
	}

	return 0
}

// removeMinMaxAndAverage 去除最大最小值后求平均
func removeMinMaxAndAverage(prices []float64) float64 {
	if len(prices) == 0 {
		return 0
	}

	if len(prices) == 1 {
		return prices[0]
	}

	if len(prices) == 2 {
		return (prices[0] + prices[1]) / 2
	}

	// 找出最大值和最小值
	minVal := prices[0]
	maxVal := prices[0]
	sum := 0.0

	for _, price := range prices {
		if price < minVal {
			minVal = price
		}
		if price > maxVal {
			maxVal = price
		}
		sum += price
	}

	// 去除最大最小值后求平均
	count := len(prices) - 2
	if count <= 0 {
		return (minVal + maxVal) / 2
	}

	return (sum - minVal - maxVal) / float64(count)
}
