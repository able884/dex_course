package util

type WEthToken struct {
	ChainId int64
	Name    string
	Address string
	Icon    string
	Symbol  string
	Decimal int64
}

var BaseWEthToken = WEthToken{
	ChainId: 8453,
	Name:    "ETH",
	Symbol:  "ETH",
	Address: "0x4200000000000000000000000000000000000006",
	Icon:    "/static/img/chain/base.png",
	Decimal: 18,
}

var SolanaWSolToken = WEthToken{
	ChainId: 100000,
	Name:    "SOL",
	Symbol:  "SOL",
	Address: "So11111111111111111111111111111111111111112",
	Icon:    "/static/img/chain/sol.png",
	Decimal: 9,
}

var TronWTrxToken = WEthToken{
	ChainId: 110000,
	Name:    "TRX",
	Symbol:  "TRX",
	Address: "TNUC9Qb1rRpS5CbWLmNMxXBjyFoydXjWFR",
	Icon:    "/static/img/chain/trx.png",
	Decimal: 6,
}

var EthWEthToken = WEthToken{
	ChainId: 1,
	Name:    "ETH",
	Symbol:  "ETH",
	Address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
	Icon:    "/static/img/chain/eth.png",
	Decimal: 18,
}

var BscWEthToken = WEthToken{
	ChainId: 56,
	Name:    "BNB",
	Symbol:  "BNB",
	Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
	Icon:    "/static/img/chain/bsc.png",
	Decimal: 18,
}

func GetBaseToken(chainId int64) WEthToken {
	switch chainId {
	case 1:
		return EthWEthToken
	case 8453:
		return BaseWEthToken
	case 100000:
		return SolanaWSolToken
	case 110000:
		return TronWTrxToken
	case 56:
		return BscWEthToken
	}

	return WEthToken{ChainId: chainId, Decimal: 18}
}
