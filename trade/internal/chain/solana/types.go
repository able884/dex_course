package solana

import (
	"github.com/gagliardetto/solana-go"
)

type CreateMarketTx struct {
	UserId            uint64
	ChainId           uint64
	UserWalletId      uint32
	UserWalletAccount solana.PublicKey
	AmountIn          string
	IsAntiMev         bool
	Slippage          uint32
	IsAutoSlippage    bool
	GasType           int32
	TradePoolName     string
	InDecimal         uint8
	OutDecimal        uint8
	InMint            solana.PublicKey
	OutMint           solana.PublicKey
	PairAddr          string
	Price             string
	UsePriceLimit     bool
	InTokenProgram    solana.PublicKey
	OutTokenProgram   solana.PublicKey
}
