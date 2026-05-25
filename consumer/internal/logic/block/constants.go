package block

import (
	"errors"

	constants "richcode.cc/dex/pkg/constants"
)

const SolChainId = constants.SolChainId
const SolChainIdInt = constants.SolChainIdInt

const ProgramStrToken = constants.ProgramStrToken

const ProgramStrPumpFun = constants.ProgramStrPumpFun
const ProgramStrPumpFunAMM = constants.ProgramStrPumpFunAMM

const TokenStrWrapSol = constants.TokenStrWrapSol
const TokenStrUSDC = constants.TokenStrUSDC
const TokenStrUSDT = constants.TokenStrUSDT

const PumpSwap = constants.PumpSwap

var ErrNotSupportInstruction = errors.New("not support instruction")
var ErrNotSupportWarp = errors.New("not support swap")
var ErrTokenAmountIsZero = errors.New("tokenAmount is zero")

const (
	MetricComponentTradeBatch    = "trade_batch"
	MetricComponentMetadataCache = "metadata_cache"
	MetricComponentMarketPush    = "market_push"
	MetricComponentRetry         = "retry"
)

const (
	LogFieldSlot    = "slot"
	LogFieldPair    = "pair"
	LogFieldToken   = "token"
	LogFieldAttempt = "attempt"
	LogFieldBatchID = "batch_id"
)

const (
	RedisKeyTokenMetadataPrefix = "token-meta"
	RedisKeyTokenAccountPrefix  = "token-account"
	RedisKeyPairStatePrefix     = "pair-state"
)
