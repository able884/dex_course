package block

import (
	"errors"

	constants "richcode.cc/dex/pkg/constrants"
)

const SolChainId = constants.SolChainId
const SolChainIdInt = constants.SolChainIdInt

// SPLtoken程序地址（原始版本地址）
const ProgramStrToken = constants.ProgramStrToken

// pump fun程序地址
const ProgramStrPumpFun = constants.ProgramStrPumpFun

// pump fun amms程序地址
const ProgramStrPumpFunAMM = constants.ProgramStrPumpFunAMM

// wrapped SOL地址
const TokenStrWrapSol = constants.TokenStrWrapSol

// USDC地址
const TokenStrUSDC = constants.TokenStrUSDC

// USDT地址
const TokenStrUSDT = constants.TokenStrUSDT

// pump swap名称
const PumpSwap = constants.PumpSwap

var ErrNotSupportInstruction = errors.New("not support instruction")

var ErrNotSupportWarp = errors.New("not support swap")
