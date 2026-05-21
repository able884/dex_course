package cpmm

import (
	"github.com/blocto/solana-go-sdk/common"
	ag_solanago "github.com/gagliardetto/solana-go"
)

const (
	// AmmConfigSeed is the seed used to derive AMM Config PDA
	AmmConfigSeed = "amm_config_seed"
)

var (
	ProgramRaydiumCPMMProgram       = common.PublicKeyFromString("CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C")
	ProgramRaydiumCPMMProgramDevNet = common.PublicKeyFromString("6Phx6fHxjmyXLpqFdPgSwe8VkCnaBX2FMH3ZHchGHvA2") // 自己部署的 raydium cp-swap 合约（devnet）
)

// DeriveAmmConfigAddress 派生 AMM Config 的 PDA 地址
// configIndex: AMM 配置的索引，通常对应不同的 fee tier
func DeriveAmmConfigAddress(configIndex uint16, programID ag_solanago.PublicKey) (ag_solanago.PublicKey, uint8, error) {
	// 将 configIndex 转换为大端字节序
	indexBytes := make([]byte, 2)
	indexBytes[0] = byte(configIndex >> 8)
	indexBytes[1] = byte(configIndex)

	seeds := [][]byte{
		[]byte(AmmConfigSeed),
		indexBytes,
	}

	return ag_solanago.FindProgramAddress(seeds, programID)
}
