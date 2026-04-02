package util

import (
	"math"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

func Decimal2Float64(d decimal.Decimal) float64 {
	v, _ := d.Float64()
	return v
}

func Float642Decimal(d float64) decimal.Decimal {
	if math.IsInf(d, 0) || math.IsNaN(d) {
		return decimal.NewFromFloat(0)
	}
	return decimal.NewFromFloat(d)
}

// Float64ToString
func Float64ToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// StringToFloat64
func StringToFloat64(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		logx.Errorf("Error parsing string to float64: %v", err)
	}
	return f
}

// Int64ToString
func Int64ToString(i int64) string {
	return strconv.FormatInt(i, 10)
}

// StringToInt64
func StringToInt64(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		logx.Errorf("Error parsing string to int64: %v", err)
	}
	return i
}
