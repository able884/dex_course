package block

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
)

// Orca DEX 的程序地址。
var ProgramOrca = common.PublicKeyFromString("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc")

// Raydium Concentrated Liquidity DEX 的程序地址。
var ProgramRaydiumConcentratedLiquidity = common.PublicKeyFromString("CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK")

// Meteora DLMM DEX 的程序地址。
var ProgramMeteoraDLMM = common.PublicKeyFromString("LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo")

// PhoneNix DEX 的程序地址。
var ProgramPhoneNix = common.PublicKeyFromString("PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY")

// StableCoinSwapDexes 稳定币交换Dexes
var StableCoinSwapDexes = []common.PublicKey{ProgramOrca, ProgramRaydiumConcentratedLiquidity, ProgramMeteoraDLMM, ProgramPhoneNix}

func GetSolBlockInfoDelay(c *client.Client, ctx context.Context, slot uint64) (resp *client.Block, err error) {
	// 减少helius调用，因为失败也算次数
	time.Sleep(time.Second * 1)
	return GetSolBlockInfo(c, ctx, slot)
}

func GetSolBlockInfo(c *client.Client, ctx context.Context, slot uint64) (resp *client.Block, err error) {
	var count int64
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err = c.GetBlockWithConfig(ctx, slot, client.GetBlockConfig{
			Commitment:         rpc.CommitmentConfirmed,
			TransactionDetails: rpc.GetBlockConfigTransactionDetailsFull,
		})
		switch {
		case err == nil:
			return
		case strings.Contains(err.Error(), "Block not available for slot"):
			count++
			if count > 10 {
				return
			}
			time.Sleep(time.Second)
		case strings.Contains(err.Error(), "limit"):
			count++
			if count > 10 {
				return
			}
			time.Sleep(time.Second)
		default:
			err = fmt.Errorf("GetBlock err:%w", err)
			return
		}
	}
}

// GetInnerInstructionMap 获取交易中的innerInstruction 数据转成 map
func GetInnerInstructionMap(tx *client.BlockTransaction) map[int]*client.InnerInstruction {
	// innerInstruction map
	var innerInstructionMap = make(map[int]*client.InnerInstruction)
	// 遍历内部指令  InnerInstructions：交易中的内部指令
	for i := range tx.Meta.InnerInstructions {
		/*
				TransactionMeta ：
			Err	any	交易执行结果（nil=成功）
			Fee	uint64	手续费（lamports）
			PreBalances	[]int64	交易前SOL余额
			PostBalances	[]int64	交易后SOL余额
			PreTokenBalances	[]TokenBalance	交易前SPL代币余额
			PostTokenBalances	[]TokenBalance	交易后SPL代币余额
			LogMessages	[]string	程序执行日志
			InnerInstructions	[]InnerInstruction	跨程序调用详情
			LoadedAddresses	LoadedAddresses	动态加载的地址
			ReturnData	*ReturnData	程序返回数据
			ComputeUnitsConsumed	*uint64	消耗的计算单元
		*/
		innerInstructionMap[int(tx.Meta.InnerInstructions[i].Index)] = &tx.Meta.InnerInstructions[i]
	}
	return innerInstructionMap
}

func (s *BlockService) GetBlockSolPrice(ctx context.Context, block *client.Block, tokenAccountMap map[string]*TokenAccount) float64 {
	priceList := make([]float64, 0)
	if tokenAccountMap == nil {
		tokenAccountMap = make(map[string]*TokenAccount)
	}

	// 遍历交易
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		//hash := base58.Encode(tx.Transaction.Signatures[0]) // todo: delete me
		//
		//_ = hash
		accountKeys := tx.AccountKeys
		// 获取交易中的innerInstruction 数据转成 map
		innerInstructionMap := GetInnerInstructionMap(tx)
		// 填充交易中的tokenAccount 数据
		tokenAccountMap, hasChange := FillTokenAccountMap(tx, tokenAccountMap)
		if !hasChange {
			continue
		}
		// 遍历交易中的指令  Instructions：交易中的指令
		for _, instruction := range tx.Transaction.Message.Instructions {
			// 遍历交易中的指令，判断是否是稳定币交换指令  StableCoinSwapDexes：稳定币交换Dexes
			if in(StableCoinSwapDexes, accountKeys[instruction.ProgramIDIndex]) {
				// 获取价格
				price := GetBlockSolPriceByTransfer(accountKeys, innerInstructionMap[instruction.ProgramIDIndex], tokenAccountMap)
				if price > 0 {
					priceList = append(priceList, price)
				}
			}
		}
		for _, instructions := range tx.Meta.InnerInstructions {
			for i, instruction := range instructions.Instructions {
				if in(StableCoinSwapDexes, accountKeys[instruction.ProgramIDIndex]) {
					innerInstruction := GetInnerInstructionByInner(instructions.Instructions, i, 2)
					price := GetBlockSolPriceByTransfer(accountKeys, innerInstruction, tokenAccountMap)
					if price > 0 {
						priceList = append(priceList, price)
					}
				}
			}
		}
	}

	price := RemoveMinAndMaxAndCalculateAverage(priceList)

	if price > 0 {
		return price
	}
	if s.solPrice > 0 {
		return s.solPrice
	}
	b, err := s.sc.BlockModel.FindOneByNearSlot(s.ctx, int64(block.ParentSlot))
	if err != nil || b == nil {
		// todo: init price
		return 0
	}
	return b.SolPrice
}

func in[T comparable](list []T, a T) bool {
	for i := 0; i < len(list); i++ {
		if list[i] == a {
			return true
		}
	}
	return false
}

// GetBlockSolPriceByTransfer 获取交易中的SOL价格
func GetBlockSolPriceByTransfer(accountKeys []common.PublicKey, innerInstructions *client.InnerInstruction, tokenAccountMap map[string]*TokenAccount) (solPrice float64) {
	if innerInstructions == nil {
		return
	}
	var transferSOL *token.TransferParam
	var transferUSD *token.TransferParam
	var connect bool
	// 遍历内部指令中的指令，判断是否是稳定币交换指令
	for j := range innerInstructions.Instructions {
		// 解析transfer
		transfer, err := DecodeTokenTransfer(accountKeys, &innerInstructions.Instructions[j])
		if err != nil {
			// err = fmt.Errorf("DecodeTokenTransfer err:%w", err)
			transferSOL = nil
			transferUSD = nil
			connect = false
			continue
		}
		from := tokenAccountMap[transfer.From.String()] // 发送方账户
		if from == nil {
			transferSOL = nil
			transferUSD = nil
			connect = false
			continue
		}
		to := tokenAccountMap[transfer.To.String()] // 接收方账户
		if to == nil {
			transferSOL = nil
			transferUSD = nil
			connect = false
			continue
		}
		if from.TokenAddress == TokenStrWrapSol { // 检查发送方是否是 sol 币
			transferSOL = transfer
			if connect && transferUSD != nil {
				solPrice = float64(transferUSD.Amount) / float64(transferSOL.Amount) * 1000
				if IsSwapTransfer(transferSOL, transferUSD, tokenAccountMap) {
					break
				} else {
					transferUSD = nil
				}
			}
			connect = true
		} else if from.TokenAddress == TokenStrUSDC || from.TokenAddress == TokenStrUSDT { // 检查发送方是否是 usdc 或 usdt 币
			transferUSD = transfer
			if connect && transferSOL != nil {
				solPrice = float64(transferUSD.Amount) / float64(transferSOL.Amount) * 1000
				if IsSwapTransfer(transferSOL, transferUSD, tokenAccountMap) {
					break
				} else {
					transferSOL = nil
				}
			}
			connect = true
		} else {
			transferSOL = nil
			transferUSD = nil
			connect = false
		}
	}
	if transferSOL != nil && transferUSD != nil && connect {
		solPrice = float64(transferUSD.Amount) / float64(transferSOL.Amount) * 1000
	} else {
		solPrice = 0
	}
	return
}

// IsSwapTransfer 判断是否是交换指令
func IsSwapTransfer(a, b *token.TransferParam, tokenAccountMap map[string]*TokenAccount) bool {
	if a == nil || b == nil {
		return false
	}
	aFrom := tokenAccountMap[a.From.String()]
	aTo := tokenAccountMap[a.To.String()]
	bFrom := tokenAccountMap[b.From.String()]
	bTo := tokenAccountMap[b.To.String()]
	if aFrom == nil || aTo == nil || bFrom == nil || bTo == nil {
		return false
	}
	if aFrom.Owner == bTo.Owner {
		return true
	}
	if bFrom.Owner == aTo.Owner {
		return true
	}
	return false
}
