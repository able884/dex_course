package util

import "github.com/shopspring/decimal"

func init() {
	decimal.DivisionPrecision = 18
	// decimal.PowPrecisionNegativeExponent = 18
}

var Decimals2Value = map[uint8]int64{
	0:  1,
	1:  1e1,
	2:  1e2,
	3:  1e3,
	4:  1e4,
	5:  1e5,
	6:  1e6,
	7:  1e7,
	8:  1e8,
	9:  1e9,
	10: 1e10,
	11: 1e11,
	12: 1e12,
	13: 1e13,
	14: 1e14,
	15: 1e15,
	16: 1e16,
	17: 1e17,
	18: 1e18,
}

func ConvertDecimal2Float64(amount int64, tokenDecimal uint8) decimal.Decimal {
	amtDecimal := decimal.NewFromInt(amount)
	amtDecimal = amtDecimal.Div(decimal.NewFromInt(Decimals2Value[tokenDecimal]))

	return amtDecimal
}

func ConvertFloat642String(value float64) string {
	return decimal.NewFromFloat(value).String()
}

func ConverString2Uint64(amount string, tokenDecimal uint8) (uint64, error) {
	amtDecimal, err := decimal.NewFromString(amount)
	if nil != err {
		return 0, err
	}
	amtDecimal = amtDecimal.Mul(decimal.NewFromInt(Decimals2Value[tokenDecimal]))
	amountUint64 := uint64(amtDecimal.IntPart())
	return amountUint64, nil
}
