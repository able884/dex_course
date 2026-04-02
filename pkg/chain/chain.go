package chain

import (
	"richcode.cc/dex/pkg/constants"

	"github.com/ethereum/go-ethereum/common"
)

const (
	// MysqlMaxDecimal
	// decimal(64,18)   最大是 9999999999999999999999999999999999999999999999
	// 但是 float64 会把它描述成 10000000000000000000000000000000000000000000000
	// 所以 MysqlMaxDecimal 会少一位
	MysqlMaxDecimal float64 = 1000000000000000000000000000000000000000000000

	// MysqlMaxDecimal32
	// decimal(32,18)
	MysqlMaxDecimal32 float64 = 99999999999999
)

func ChainId2ChainIcon(chainId int64) string {
	switch chainId {
	case constants.EthChainIdInt:
		return constants.ChainIconEth
	case constants.BscChainIdInt:
		return constants.ChainIconBsc
	case constants.BaseChainIdInt:
		return constants.ChainIconBase
	case constants.SolChainIdInt:
		return constants.ChainIconSol
	case constants.TrxChainIdInt:
		return constants.ChainIconTron
	default:
		return ""
	}
}

func EvmCompareAddresses(addr1, addr2 string) bool {
	return common.HexToAddress(addr1).Cmp(common.HexToAddress(addr2)) == 0
}

func ChainName2ChainId(chainName string) int64 {
	var chainId int64
	switch chainName {
	case constants.ChainNameEth:
		chainId = constants.EthChainIdInt
	case constants.ChainNameBase:
		chainId = constants.BaseChainIdInt
	case constants.ChainNameBsc:
		chainId = constants.BscChainIdInt
	}
	return chainId
}

func ChainId2TokenCa(chainId int64) string {
	switch chainId {
	case constants.EthChainIdInt:
		return constants.BaseTokenAddressEth
	case constants.BscChainIdInt:
		return constants.BaseTokenAddressBsc
	case constants.BaseChainIdInt:
		return constants.BaseTokenAddressBase
	case constants.SolChainIdInt:
		return constants.BaseTokenAddressSol
	default:
		return ""
	}
}

func ChainId2ChainName(chainId int64) string {
	switch chainId {
	case constants.EthChainIdInt:
		return constants.ChainNameEth
	case constants.BscChainIdInt:
		return constants.ChainNameBsc
	case constants.BaseChainIdInt:
		return constants.ChainNameBase
	case constants.SolChainIdInt:
		return constants.BaseTokenAddressSol
	default:
		return ""
	}
}
