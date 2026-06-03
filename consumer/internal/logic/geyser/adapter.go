package geyser

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	pb "github.com/rpcpool/yellowstone-grpc/examples/golang/proto"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/logic/slot"
	blockpb "richcode.cc/dex/consumer/internal/mq/proto"
	"richcode.cc/dex/consumer/internal/svc"
)

// GeyserToBlockAdapter 将 Geyser 流数据适配为 BlockMessage
type GeyserToBlockAdapter struct {
	blockParser      *slot.BlockParser
	logger           logx.Logger
	ctx              context.Context
	conversionErrors atomic.Int64 // 转换错误计数
}

// NewGeyserToBlockAdapter 创建新的适配器
func NewGeyserToBlockAdapter(ctx context.Context, svcCtx *svc.ServiceContext) *GeyserToBlockAdapter {
	return &GeyserToBlockAdapter{
		blockParser: slot.NewBlockParser(svcCtx, ctx),
		logger:      logx.WithContext(ctx).WithFields(logx.Field("component", "geyser_adapter")),
		ctx:         ctx,
	}
}

// AdaptSlotData 将 SlotData 转换为 BlockMessage
func (a *GeyserToBlockAdapter) AdaptSlotData(slotData *SlotData) (*blockpb.BlockMessage, error) {
	if slotData == nil || len(slotData.GetTransactions()) == 0 {
		return nil, fmt.Errorf("slotData is empty")
	}

	// 将 Geyser 交易转换为 client.Block 格式
	block, err := a.geyserToClientBlock(slotData)
	if err != nil {
		return nil, fmt.Errorf("转换 Geyser 数据为 Block 格式失败: %w", err)
	}

	// 复用现有的 BlockParser 进行解析
	blockMsg, err := a.blockParser.ParseBlock(block, slotData.Slot)
	if err != nil {
		return nil, fmt.Errorf("解析区块失败: %w", err)
	}

	return blockMsg, nil
}

// geyserToClientBlock 将 Geyser SlotData 转换为 client.Block
func (a *GeyserToBlockAdapter) geyserToClientBlock(slotData *SlotData) (*client.Block, error) {
	txs := slotData.GetTransactions()
	if len(txs) == 0 {
		return nil, fmt.Errorf("no transactions in slot data")
	}

	// 创建 Block 结构
	block := &client.Block{
		Blockhash:         "", // Geyser 可能不提供 blockhash
		PreviousBlockhash: "",
		ParentSlot:        0,
		Transactions:      make([]client.BlockTransaction, 0, len(txs)),
		BlockHeight:       nil,
		BlockTime:         nil,
	}

	// 转换每个交易
	for _, geyserTx := range txs {
		blockTx, err := a.convertGeyserTransaction(geyserTx)
		if err != nil {
			// 静默计数，避免高频错误日志（每100个错误打印一次）
			errCount := a.conversionErrors.Add(1)
			if errCount%100 == 1 {
				a.logger.Errorf("转换 Geyser 交易失败 (已累计 %d 次): %v", errCount, err)
			}
			continue
		}
		block.Transactions = append(block.Transactions, *blockTx)
	}

	return block, nil
}

// convertGeyserTransaction 将 Geyser SubscribeUpdateTransaction 转换为 client.BlockTransaction
func (a *GeyserToBlockAdapter) convertGeyserTransaction(geyserTx *pb.SubscribeUpdateTransaction) (*client.BlockTransaction, error) {
	if geyserTx == nil || geyserTx.Transaction == nil {
		return nil, fmt.Errorf("geyser transaction is nil")
	}

	// 从 Geyser Transaction 提取数据
	tx := geyserTx.Transaction
	meta := geyserTx.Transaction.Meta

	// 构建 Transaction
	transaction := types.Transaction{
		Signatures: make([]types.Signature, 0),
		Message:    types.Message{},
	}

	// 提取 AccountKeys 列表
	accountKeys := make([]common.PublicKey, 0)

	// 转换签名
	if tx.Signature != nil {
		sig := types.Signature{}
		copy(sig[:], tx.Signature)
		transaction.Signatures = append(transaction.Signatures, sig)
	}

	// 转换 Message
	if tx.Transaction != nil {
		// AccountKeys
		for _, key := range tx.Transaction.Message.AccountKeys {
			if len(key) == 32 {
				pk := common.PublicKey{}
				copy(pk[:], key)
				accountKeys = append(accountKeys, pk)
			}
		}

		// RecentBlockhash (注意字段名是 RecentBlockHash，大小写)
		// RecentBlockHash 应该是 base58 编码的字符串
		if len(tx.Transaction.Message.RecentBlockhash) >= 32 {
			// 将字节转换为 PublicKey，然后转换为 base58 字符串
			hash := common.PublicKey{}
			copy(hash[:], tx.Transaction.Message.RecentBlockhash)
			transaction.Message.RecentBlockHash = hash.ToBase58()
		}

		// Instructions
		instructions := make([]types.CompiledInstruction, 0)
		for _, inst := range tx.Transaction.Message.Instructions {
			instructions = append(instructions, types.CompiledInstruction{
				ProgramIDIndex: int(inst.ProgramIdIndex),
				Accounts:       convertBytesToInt(inst.Accounts),
				Data:           inst.Data,
			})
		}
		transaction.Message.Instructions = instructions
	}

	// 构建 BlockTransaction
	blockTx := &client.BlockTransaction{
		Transaction: transaction,
		Meta:        nil,
		AccountKeys: accountKeys,
	}

	// 转换 Meta
	if meta != nil {
		blockTx.Meta = &client.TransactionMeta{
			Err:               nil,
			Fee:               meta.Fee,
			PreBalances:       convertUint64ToInt64(meta.PreBalances),
			PostBalances:      convertUint64ToInt64(meta.PostBalances),
			InnerInstructions: convertGeyserInnerInstructions(meta.InnerInstructions),
			PreTokenBalances:  convertGeyserTokenBalances(meta.PreTokenBalances),
			PostTokenBalances: convertGeyserTokenBalances(meta.PostTokenBalances),
			LogMessages:       meta.LogMessages,
		}

		// 处理错误
		if meta.Err != nil {
			// Geyser 的错误格式可能不同，需要适配
			blockTx.Meta.Err = meta.Err
		}
	}

	return blockTx, nil
}

// convertGeyserInnerInstructions 转换内部指令
func convertGeyserInnerInstructions(geyserInnerInsts []*pb.InnerInstructions) []client.InnerInstruction {
	if len(geyserInnerInsts) == 0 {
		return nil
	}

	result := make([]client.InnerInstruction, 0, len(geyserInnerInsts))
	for _, inner := range geyserInnerInsts {
		instructions := make([]types.CompiledInstruction, 0, len(inner.Instructions))
		for _, inst := range inner.Instructions {
			instructions = append(instructions, types.CompiledInstruction{
				ProgramIDIndex: int(inst.ProgramIdIndex),
				Accounts:       convertBytesToInt(inst.Accounts),
				Data:           inst.Data,
			})
		}

		result = append(result, client.InnerInstruction{
			Index:        uint64(inner.Index),
			Instructions: instructions,
		})
	}

	return result
}

// convertGeyserTokenBalances 转换 token balances
func convertGeyserTokenBalances(geyserBalances []*pb.TokenBalance) []rpc.TransactionMetaTokenBalance {
	if len(geyserBalances) == 0 {
		return nil
	}

	result := make([]rpc.TransactionMetaTokenBalance, 0, len(geyserBalances))
	for _, bal := range geyserBalances {
		tokenBalance := rpc.TransactionMetaTokenBalance{
			AccountIndex: uint64(bal.AccountIndex),
			Mint:         bal.Mint,
			Owner:        bal.Owner,
		}

		// 注意：rpc.TransactionMetaTokenBalance 可能不包含 UITokenAmount 字段
		// 如果需要，可以在后续版本中添加

		result = append(result, tokenBalance)
	}

	return result
}

// convertBytesToInt 将字节数组转换为 int 数组
func convertBytesToInt(bytes []byte) []int {
	if len(bytes) == 0 {
		return nil
	}

	result := make([]int, len(bytes))
	for i, b := range bytes {
		result[i] = int(b)
	}
	return result
}

// convertUint64ToInt64 将 uint64 数组转换为 int64 数组
func convertUint64ToInt64(values []uint64) []int64 {
	if len(values) == 0 {
		return nil
	}

	result := make([]int64, len(values))
	for i, v := range values {
		result[i] = int64(v)
	}
	return result
}
