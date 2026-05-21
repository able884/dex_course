package trade

type CreateMarketTx struct {
	UserId            uint64
	ChainId           uint64
	UserWalletId      uint32
	UserWalletAddress string
	AmountIn          string
	IsAntiMev         bool
	IsAutoSlippage    bool
	Slippage          uint32
	GasType           int32
	TradePoolName     string
	InDecimal         uint8
	OutDecimal        uint8
	InTokenCa         string
	OutTokenCa        string
	PairAddr          string
	Price             string
	UsePriceLimit     bool
	InTokenProgram    string
	OutTokenProgram   string
}
