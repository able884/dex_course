package util

// Ternary 实现三元运算符功能的泛型函数
// 用法: result := Ternary(condition, trueValue, falseValue)
func Ternary[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
}

// TernaryInt 针对 int 类型的三元运算符
func TernaryInt(condition bool, trueVal, falseVal int) int {
	return Ternary(condition, trueVal, falseVal)
}

// TernaryString 针对 string 类型的三元运算符
func TernaryString(condition bool, trueVal, falseVal string) string {
	return Ternary(condition, trueVal, falseVal)
}

// TernaryFloat64 针对 float64 类型的三元运算符
func TernaryFloat64(condition bool, trueVal, falseVal float64) float64 {
	return Ternary(condition, trueVal, falseVal)
}

// TernaryBool 针对 bool 类型的三元运算符
func TernaryBool(condition bool, trueVal, falseVal bool) bool {
	return Ternary(condition, trueVal, falseVal)
}

func BoolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
