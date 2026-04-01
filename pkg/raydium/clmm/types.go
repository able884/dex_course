package clmm

import (
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

// SwapSingleV2 结构体表示单个交换操作所需的账户信息
type SwapSingleV2 struct {
	// 交易的发起人（需要签名权限）
	Payer types.AccountMeta

	// AMM 池的配置，用于获取协议费用
	AmmConfig common.PublicKey

	// AMM 池状态账户，存储池的详细信息
	PoolState types.AccountMeta

	// 用户输入代币账户
	InputTokenAccount types.AccountMeta

	// 用户输出代币账户
	OutputTokenAccount types.AccountMeta

	// 输入代币的资金库（vault）
	InputVault types.AccountMeta

	// 输出代币的资金库（vault）
	OutputVault types.AccountMeta

	// Oracle 观察状态账户，提供最近的价格信息
	ObservationState types.AccountMeta

	// Solana SPL 代币程序（标准）
	TokenProgram common.PublicKey

	// Solana SPL 2022 代币程序（支持扩展功能）
	TokenProgram2022 common.PublicKey

	// 交易备注（可选）
	MemoProgram common.PublicKey

	// 输入代币的 mint 信息
	InputVaultMint common.PublicKey

	// 输出代币的 mint 信息
	OutputVaultMint common.PublicKey

	// 其余账户（如流动性池的 tick 数组）
	RemainingAccounts []types.AccountMeta
}
