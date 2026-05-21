package pumpfun

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
)

// QuoteDirection represents buy or sell direction.
type QuoteDirection int

const (
	QuoteDirectionUnknown QuoteDirection = iota
	QuoteDirectionBuy
	QuoteDirectionSell
)

// QuoteAsset describes whether amount refers to SOL or the Pump token.
type QuoteAsset int

const (
	QuoteAssetUnknown QuoteAsset = iota
	QuoteAssetSOL
	QuoteAssetToken
)

// QuoteParams captures the inputs for a PumpFun quote request.
type QuoteParams struct {
	Direction   QuoteDirection
	InputAsset  QuoteAsset
	Amount      uint64
	SlippageBps uint32
}

// QuoteResult returns the normalized pay/receive breakdown and slippage-protected values.
type QuoteResult struct {
	PayAsset             QuoteAsset
	PayAmount            uint64
	PayAmountWithLimit   uint64
	ReceiveAsset         QuoteAsset
	ReceiveAmount        uint64
	MinReceiveAmount     uint64
	PriceImpactBps       uint32
	VirtualSolReserves   uint64
	VirtualTokenReserves uint64
}

var (
	errInvalidQuoteAmount = errors.New("quote amount must be greater than zero")
	errUnsupportedQuote   = errors.New("unsupported quote combination")
	pumpBaseBig           = new(big.Int).SetUint64(PumpSwapAmmBase)
	pumpFeeBig            = new(big.Int).SetUint64(PumpSwapAmmBuyFee)
	pumpFeeFactorBig      = new(big.Int).Sub(pumpBaseBig, pumpFeeBig)
	bpsBase               = uint64(10000)
)

// QuotePumpBondingCurve loads current bonding curve state and calculates quote details.
func QuotePumpBondingCurve(ctx context.Context, client *ag_rpc.Client, mint ag_solanago.PublicKey, params QuoteParams) (*QuoteResult, error) {
	if params.Amount == 0 {
		return nil, errInvalidQuoteAmount
	}
	state, err := GetBondingCurveState(ctx, client, mint)
	if err != nil {
		return nil, err
	}
	return QuoteWithState(state, params)
}

// QuoteWithState reuses a fetched bonding-curve state (handy for tests or batched queries).
func QuoteWithState(state *BondingCurveState, params QuoteParams) (*QuoteResult, error) {
	if state == nil {
		return nil, fmt.Errorf("nil bonding curve state")
	}
	if params.Amount == 0 {
		return nil, errInvalidQuoteAmount
	}

	result := &QuoteResult{
		PayAmount:            params.Amount,
		PayAmountWithLimit:   params.Amount,
		VirtualSolReserves:   state.VirtualSolReserves,
		VirtualTokenReserves: state.VirtualTokenReserves,
	}

	switch params.Direction {
	case QuoteDirectionBuy:
		return quoteBuy(state, params, result)
	case QuoteDirectionSell:
		return quoteSell(state, params, result)
	default:
		return nil, errUnsupportedQuote
	}
}

func quoteBuy(state *BondingCurveState, params QuoteParams, result *QuoteResult) (*QuoteResult, error) {
	switch params.InputAsset {
	case QuoteAssetSOL:
		tokenOut := CalculateBuyAmount(params.Amount, state.VirtualSolReserves, state.VirtualTokenReserves)
		if tokenOut == 0 {
			return nil, fmt.Errorf("insufficient output for provided SOL")
		}
		result.PayAsset = QuoteAssetSOL
		result.ReceiveAsset = QuoteAssetToken
		result.ReceiveAmount = tokenOut
		result.MinReceiveAmount = applySlippageDown(tokenOut, params.SlippageBps)
		result.PriceImpactBps = calcPriceImpactBps(tokenOut, state.VirtualTokenReserves)
		return result, nil
	case QuoteAssetToken:
		solNeeded, err := calcSolRequiredForTokens(params.Amount, state.VirtualSolReserves, state.VirtualTokenReserves)
		if err != nil {
			return nil, err
		}
		result.PayAsset = QuoteAssetSOL
		result.PayAmount = solNeeded
		result.PayAmountWithLimit = applySlippageUp(solNeeded, params.SlippageBps)
		result.ReceiveAsset = QuoteAssetToken
		result.ReceiveAmount = params.Amount
		result.MinReceiveAmount = applySlippageDown(params.Amount, params.SlippageBps)
		result.PriceImpactBps = calcPriceImpactBps(params.Amount, state.VirtualTokenReserves)
		return result, nil
	default:
		return nil, errUnsupportedQuote
	}
}

func quoteSell(state *BondingCurveState, params QuoteParams, result *QuoteResult) (*QuoteResult, error) {
	switch params.InputAsset {
	case QuoteAssetToken:
		solOut := CalculateSellAmount(params.Amount, state.VirtualSolReserves, state.VirtualTokenReserves)
		if solOut == 0 {
			return nil, fmt.Errorf("insufficient SOL output for provided tokens")
		}
		result.PayAsset = QuoteAssetToken
		result.ReceiveAsset = QuoteAssetSOL
		result.ReceiveAmount = solOut
		result.MinReceiveAmount = applySlippageDown(solOut, params.SlippageBps)
		result.PriceImpactBps = calcPriceImpactBps(params.Amount, state.VirtualTokenReserves)
		return result, nil
	case QuoteAssetSOL:
		tokensRequired, err := calcTokensRequiredForSol(params.Amount, state.VirtualSolReserves, state.VirtualTokenReserves)
		if err != nil {
			return nil, err
		}
		result.PayAsset = QuoteAssetToken
		result.PayAmount = tokensRequired
		result.PayAmountWithLimit = applySlippageUp(tokensRequired, params.SlippageBps)
		result.ReceiveAsset = QuoteAssetSOL
		result.ReceiveAmount = params.Amount
		result.MinReceiveAmount = applySlippageDown(params.Amount, params.SlippageBps)
		result.PriceImpactBps = calcPriceImpactBps(tokensRequired, state.VirtualTokenReserves)
		return result, nil
	default:
		return nil, errUnsupportedQuote
	}
}

func calcPriceImpactBps(delta, reserve uint64) uint32 {
	if reserve == 0 || delta == 0 {
		return 0
	}
	num := new(big.Int).Mul(new(big.Int).SetUint64(delta), new(big.Int).SetUint64(bpsBase))
	den := new(big.Int).SetUint64(reserve)
	q := new(big.Int).Div(num, den)
	max := new(big.Int).SetUint64(bpsBase)
	if q.Cmp(max) > 0 {
		return uint32(bpsBase)
	}
	return uint32(q.Uint64())
}

func applySlippageDown(value uint64, slippage uint32) uint64 {
	if slippage == 0 {
		return value
	}
	base := uint64(10000)
	if slippage >= 10000 {
		return 0
	}
	num := new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(base-uint64(slippage)))
	den := new(big.Int).SetUint64(base)
	return new(big.Int).Div(num, den).Uint64()
}

func applySlippageUp(value uint64, slippage uint32) uint64 {
	if slippage == 0 {
		return value
	}
	num := new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(10000+uint64(slippage)))
	den := new(big.Int).SetUint64(10000)
	return divRoundUp(num, den).Uint64()
}

func calcSolRequiredForTokens(tokenAmount, virtualSolReserves, virtualTokenReserves uint64) (uint64, error) {
	if tokenAmount == 0 || tokenAmount >= virtualTokenReserves {
		return 0, fmt.Errorf("token amount exceeds reserves")
	}
	numerator := new(big.Int).Mul(new(big.Int).SetUint64(tokenAmount), new(big.Int).SetUint64(virtualSolReserves))
	denominator := new(big.Int).Sub(new(big.Int).SetUint64(virtualTokenReserves), new(big.Int).SetUint64(tokenAmount))
	solAfterFee := divRoundUp(numerator, denominator)
	solIn := divRoundUp(new(big.Int).Mul(solAfterFee, pumpBaseBig), pumpFeeFactorBig)
	return toUint64(solIn)
}

func calcTokensRequiredForSol(solAmount, virtualSolReserves, virtualTokenReserves uint64) (uint64, error) {
	if solAmount == 0 || solAmount >= virtualSolReserves {
		return 0, fmt.Errorf("sol amount exceeds reserves")
	}
	solBeforeFee := divRoundUp(new(big.Int).Mul(new(big.Int).SetUint64(solAmount), pumpBaseBig), pumpFeeFactorBig)
	if new(big.Int).SetUint64(virtualSolReserves).Cmp(solBeforeFee) <= 0 {
		return 0, fmt.Errorf("insufficient virtual reserves for requested sol amount")
	}
	numerator := new(big.Int).Mul(solBeforeFee, new(big.Int).SetUint64(virtualTokenReserves))
	denominator := new(big.Int).Sub(new(big.Int).SetUint64(virtualSolReserves), solBeforeFee)
	tokenIn := divRoundUp(numerator, denominator)
	return toUint64(tokenIn)
}

func divRoundUp(num, den *big.Int) *big.Int {
	if den.Sign() == 0 {
		return big.NewInt(0)
	}
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func toUint64(v *big.Int) (uint64, error) {
	if v.Sign() < 0 {
		return 0, fmt.Errorf("negative value")
	}
	if v.BitLen() > 64 {
		return 0, fmt.Errorf("value exceeds uint64")
	}
	return v.Uint64(), nil
}
