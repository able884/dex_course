package transfer

import "encoding/json"

// Byte2Struct 将字节数组解码为指定类型的结构体
func Byte2Struct[T any](b []byte) (T, error) {
	var t T
	err := json.Unmarshal(b, &t)
	return t, err
}

// String2Struct 将字符串解码为指定类型的结构体
func String2Struct[T any](b string) (T, error) {
	var t T
	err := json.Unmarshal([]byte(b), &t)
	return t, err
}

// String2StructSlice 将 JSON 数组字符串解码为结构体切片
func String2StructSlice[T any](b string) ([]T, error) {
	var items []T
	err := json.Unmarshal([]byte(b), &items)
	return items, err
}

// Struct2Byte 将结构体编码为字节数组
func Struct2Byte[T any](t T) ([]byte, error) {
	bytes, err := json.Marshal(t)
	return bytes, err
}

// Struct2ByteIgnoreError 将结构体编码为字节数组，忽略错误
func Struct2ByteIgnoreError[T any](t T) []byte {
	b, _ := Struct2Byte(t)
	return b
}

// Struct2String 将结构体编码为字符串
func Struct2String[T any](t T) (string, error) {
	bytes, err := json.Marshal(t)
	return string(bytes), err
}

// Struct2StringWithDefault 将结构体编码为字符串，出错时返回默认值
func Struct2StringWithDefault[T any](t T) string {
	str, err := Struct2String(t)
	if err != nil {
		return ""
	}
	return str
}

// Map2String 将 map 转换为 JSON 字符串
func Map2String[T any](m map[string]T) (string, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Map2Struct 将 map 转换为指定类型的结构体
func Map2Struct[T any, U any](m map[string]T) (U, error) {
	var u U
	// 将 map 转换为 JSON 字节数组
	bytes, err := json.Marshal(m)
	if err != nil {
		return u, err
	}
	// 将 JSON 字节数组反序列化为指定的结构体类型
	err = json.Unmarshal(bytes, &u)
	return u, err
}
