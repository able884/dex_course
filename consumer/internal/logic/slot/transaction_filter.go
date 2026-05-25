package slot

import (
	"encoding/hex"
	"strings"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/config"
)

// TransactionFilter 交易过滤器（支持配置化）
type TransactionFilter struct {
	config      config.BlockFilterConfig
	programMap  map[string]*protocolRule  // ProgramID -> 协议规则
	enabled     bool
}

// protocolRule 协议规则
type protocolRule struct {
	name         string
	programID    string
	instructions map[string]string // discriminator(hex) -> instruction name
	matchAll     bool              // 是否匹配所有指令（instructions为空时为true）
}

// NewTransactionFilter 创建交易过滤器
func NewTransactionFilter(cfg config.BlockFilterConfig) *TransactionFilter {
	filter := &TransactionFilter{
		config:     cfg,
		programMap: make(map[string]*protocolRule),
		enabled:    cfg.Enabled,
	}

	// 构建快速查找表
	for _, rule := range cfg.TargetProtocols {
		protocolRule := &protocolRule{
			name:         rule.Name,
			programID:    rule.ProgramID,
			instructions: make(map[string]string),
			matchAll:     len(rule.Instructions) == 0, // 没有指定指令则匹配所有
		}

		// 构建指令鉴别符映射
		for _, inst := range rule.Instructions {
			// 标准化 discriminator（移除可能的 0x 前缀，转小写）
			discriminator := strings.TrimPrefix(strings.ToLower(inst.Discriminator), "0x")
			protocolRule.instructions[discriminator] = inst.Name
		}

		filter.programMap[rule.ProgramID] = protocolRule
	}

	logx.Infof("TransactionFilter initialized: enabled=%v, protocols=%d",
		filter.enabled, len(filter.programMap))

	return filter
}

// FilterRelevantTransactions 过滤相关交易
// 只保留包含目标协议指令的交易
func (f *TransactionFilter) FilterRelevantTransactions(block *client.Block) []client.BlockTransaction {
	if !f.enabled || block == nil || len(block.Transactions) == 0 {
		return block.Transactions
	}

	// 预估10%是相关的
	relevantTxs := make([]client.BlockTransaction, 0, len(block.Transactions)/10)

	for i := range block.Transactions {
		tx := &block.Transactions[i]

		// 检查交易是否匹配过滤规则
		if f.matchTransaction(tx) {
			relevantTxs = append(relevantTxs, *tx)
		}
	}

	return relevantTxs
}

// FilterAndGroupByProtocol 过滤并按协议分组交易
// 返回：map[协议名称][]交易
func (f *TransactionFilter) FilterAndGroupByProtocol(block *client.Block) map[string][]client.BlockTransaction {
	result := make(map[string][]client.BlockTransaction)

	if !f.enabled || block == nil || len(block.Transactions) == 0 {
		return result
	}

	for i := range block.Transactions {
		tx := &block.Transactions[i]

		// 获取交易匹配的协议列表
		protocols := f.getMatchingProtocols(tx)

		// 将交易添加到对应的协议分组
		for _, protocolName := range protocols {
			result[protocolName] = append(result[protocolName], *tx)
		}
	}

	return result
}

// getMatchingProtocols 获取交易匹配的协议列表
func (f *TransactionFilter) getMatchingProtocols(tx *client.BlockTransaction) []string {
	protocols := make([]string, 0)
	matched := make(map[string]bool) // 防止重复

	// 1. 检查顶层指令
	// SPL Token/Token2022 只根据顶层指令判断（仅处理 mint/burn）
	// 其他业务协议（DEX）需要检查顶层和内部指令
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		protocolName := f.getInstructionProtocol(tx, inst)
		if protocolName != "" && !matched[protocolName] {
			protocols = append(protocols, protocolName)
			matched[protocolName] = true
		}
	}

	// 2. 检查内部指令（仅针对非 SPL Token 协议）
	// SPL Token 协议不检查内部指令，因为它只处理 mint/burn，这些都是顶层指令
	if tx.Meta != nil {
		for _, innerInsts := range tx.Meta.InnerInstructions {
			for i := range innerInsts.Instructions {
				inst := &innerInsts.Instructions[i]
				protocolName := f.getInstructionProtocol(tx, inst)
				if protocolName != "" && !matched[protocolName] {
					// SPL Token 协议不处理内部指令
					if f.isSPLTokenProtocol(protocolName) {
						continue
					}
					protocols = append(protocols, protocolName)
					matched[protocolName] = true
				}
			}
		}
	}

	return protocols
}

// isSPLTokenProtocol 判断是否为 SPL Token 协议
func (f *TransactionFilter) isSPLTokenProtocol(protocolName string) bool {
	return protocolName == "SPL_Token" || protocolName == "SPL_Token_2022"
}

// getInstructionProtocol 获取指令所属的协议名称
func (f *TransactionFilter) getInstructionProtocol(tx *client.BlockTransaction, inst *types.CompiledInstruction) string {
	// 获取 ProgramID
	if int(inst.ProgramIDIndex) >= len(tx.AccountKeys) {
		return ""
	}

	programID := tx.AccountKeys[inst.ProgramIDIndex].String()

	// 查找协议规则
	rule, exists := f.programMap[programID]
	if !exists {
		return ""
	}

	// 如果规则设置为匹配所有指令，直接返回协议名
	if rule.matchAll {
		return rule.name
	}

	// 否则检查指令鉴别符
	if len(inst.Data) < 8 {
		return ""
	}

	// 提取前8字节作为鉴别符
	discriminator := hex.EncodeToString(inst.Data[:8])

	// 检查是否在允许的指令列表中
	if _, matched := rule.instructions[discriminator]; matched {
		return rule.name
	}

	return ""
}

// matchTransaction 检查交易是否匹配过滤规则
func (f *TransactionFilter) matchTransaction(tx *client.BlockTransaction) bool {
	// 1. 检查顶层指令
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]

		if f.matchInstruction(tx, inst) {
			return true
		}
	}

	// 2. 检查内部指令 (Inner Instructions)
	if tx.Meta != nil {
		for _, innerInsts := range tx.Meta.InnerInstructions {
			for i := range innerInsts.Instructions {
				inst := &innerInsts.Instructions[i]

				if f.matchInstruction(tx, inst) {
					return true
				}
			}
		}
	}

	return false
}

// matchInstruction 检查单个指令是否匹配
func (f *TransactionFilter) matchInstruction(tx *client.BlockTransaction, inst *types.CompiledInstruction) bool {
	// 获取 ProgramID
	if int(inst.ProgramIDIndex) >= len(tx.AccountKeys) {
		return false
	}

	programID := tx.AccountKeys[inst.ProgramIDIndex].String()

	// 查找协议规则
	rule, exists := f.programMap[programID]
	if !exists {
		return false
	}

	// 如果规则设置为匹配所有指令，直接返回 true
	if rule.matchAll {
		return true
	}

	// 否则检查指令鉴别符
	if len(inst.Data) < 8 {
		// 指令数据太短，无法提取鉴别符
		return false
	}

	// 提取前8字节作为鉴别符
	discriminator := hex.EncodeToString(inst.Data[:8])

	// 检查是否在允许的指令列表中
	_, matched := rule.instructions[discriminator]
	return matched
}

// FilterStats 过滤统计信息
type FilterStats struct {
	TotalTransactions    int
	FilteredTransactions int
	FilterRatio          float64
	ProtocolCounts       map[string]int // 协议名称 -> 交易数
	InstructionCounts    map[string]int // "协议名:指令名" -> 指令数
}

// GetFilterStats 获取过滤统计信息
func (f *TransactionFilter) GetFilterStats(block *client.Block, filteredTxs []client.BlockTransaction) *FilterStats {
	stats := &FilterStats{
		TotalTransactions:    len(block.Transactions),
		FilteredTransactions: len(filteredTxs),
		ProtocolCounts:       make(map[string]int),
		InstructionCounts:    make(map[string]int),
	}

	if stats.TotalTransactions > 0 {
		stats.FilterRatio = float64(stats.FilteredTransactions) / float64(stats.TotalTransactions)
	}

	// 统计各协议和指令的数量
	for i := range filteredTxs {
		f.countTransactionStats(&filteredTxs[i], stats)
	}

	return stats
}

// countTransactionStats 统计单个交易的协议和指令
func (f *TransactionFilter) countTransactionStats(tx *client.BlockTransaction, stats *FilterStats) {
	// 统计顶层指令
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		f.countInstructionStats(tx, inst, stats)
	}

	// 统计内部指令
	if tx.Meta != nil {
		for _, innerInsts := range tx.Meta.InnerInstructions {
			for i := range innerInsts.Instructions {
				inst := &innerInsts.Instructions[i]
				f.countInstructionStats(tx, inst, stats)
			}
		}
	}
}

// countInstructionStats 统计单个指令
func (f *TransactionFilter) countInstructionStats(tx *client.BlockTransaction, inst *types.CompiledInstruction, stats *FilterStats) {
	if int(inst.ProgramIDIndex) >= len(tx.AccountKeys) {
		return
	}

	programID := tx.AccountKeys[inst.ProgramIDIndex].String()
	rule, exists := f.programMap[programID]
	if !exists {
		return
	}

	// 统计协议数
	stats.ProtocolCounts[rule.name]++

	// 统计指令数
	if len(inst.Data) >= 8 {
		discriminator := hex.EncodeToString(inst.Data[:8])
		if instName, ok := rule.instructions[discriminator]; ok {
			key := rule.name + ":" + instName
			stats.InstructionCounts[key]++
		} else if rule.matchAll {
			// 如果是匹配所有指令的规则，记录为 "协议名:all"
			key := rule.name + ":all"
			stats.InstructionCounts[key]++
		}
	}
}
