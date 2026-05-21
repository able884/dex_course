package sol

import (
	"github.com/shopspring/decimal"
	"richcode.cc/dex/pkg/util"
)

const (
	FeeReceiver = "77r1L6TyggUhwFkk3wFrMMkYS7xK6xJu78wuzMjr2PHZ"

	RaydiumV4SwapCU   = 150_000
	PumpFunSwapCU     = 100_000
	RaydiumClmmSwapCu = 250_000
	RaydiumCpmmSwapCu = 150_000
	PumpSwapCU        = 150_000

	GasPerSignature    = 5_000
	GasPerSignatureSol = 0.000005

	TransferSolCU                 = 500   //refer to https://solscan.io/tx/fiAwZ5G3tgDjuKZGztzn8NY8tU34biVjuTvHidnX9S8AWTGfmCabaKeP1ioARZkc13NoDjmR7pcdEpDrShGRkt4
	TransferTokenCU               = 36118 //refer to https://solscan.io/tx/3Jy3X2bguDXKF6vBNKd5rBA5d3ncbzQXXDgiG6tb7cFVvoNbh274Qz6ZUnZc8fyTKNr67ARpTGgNRW7Z4Zs7X7LC
	PriorityGasPriceTransferSol   = 10000000
	PriorityGasPriceTransferToken = 500000

	Microlamports = 1e9 * 1e6
	MinDecimal    = 0
	MaxDecimal    = 18
	SolDecimal    = 9

	JitoMaxFee = 0.05 // 0.05 sol

	// TipFee Deprecated
	TipFee               = 0.001
	TipFeeLamport uint64 = 1000000
)

type GasType int32

var ServericeFeePercent = decimal.NewFromFloat(0.01)

const (
	GasType_GasTypeSpeedInvalid GasType = 0
	GasType_NormalSpeed         GasType = 1
	GasType_FastSpeed           GasType = 2
	GasType_SuperFastSpeed      GasType = 3
)

var PricePerPriorityMODE = map[GasType]uint64{
	0:                      0,         // use for test
	GasType_NormalSpeed:    1000000,   //0.00015 sol
	GasType_FastSpeed:      30000000,  //0.0045 sol
	GasType_SuperFastSpeed: 100000000, //0.015 sol
}

var GasMODE = map[GasType]uint64{
	0:                      GasPerSignature, // use for test
	GasType_NormalSpeed:    150000,          //0.00015 sol
	GasType_FastSpeed:      4500000,         //0.0045 sol
	GasType_SuperFastSpeed: 15000000,        //0.015 sol
}

type Swap_Direction uint8

const (
	Swap_Direction_Buy Swap_Direction = iota
	Swap_Direction_Sell
)

var Decimals2Value = util.Decimals2Value
