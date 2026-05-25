package util

import (
	"fmt"
	"net"
	"strings"
)

// FindAvailablePort 从指定端口开始查找可用端口
// address 格式: "0.0.0.0:8080" 或 ":8080"
// 返回可用的地址，例如: "0.0.0.0:8081"
func FindAvailablePort(address string) (string, error) {
	// 解析地址，分离主机和端口
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid address format: %w", err)
	}

	// 解析端口号
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return "", fmt.Errorf("invalid port number: %w", err)
	}

	// 如果主机为空，使用默认值
	if host == "" {
		host = "0.0.0.0"
	}

	// 从指定端口开始，尝试找到可用端口（最多尝试100个端口）
	for i := 0; i < 100; i++ {
		testPort := port + i
		testAddr := fmt.Sprintf("%s:%d", host, testPort)

		if isPortAvailable(testAddr) {
			return testAddr, nil
		}
	}

	return "", fmt.Errorf("no available port found in range %d-%d", port, port+99)
}

// isPortAvailable 检查端口是否可用
func isPortAvailable(address string) bool {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// GetPortFromAddress 从地址中提取端口号
func GetPortFromAddress(address string) (int, error) {
	_, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return 0, fmt.Errorf("invalid address format: %w", err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return 0, fmt.Errorf("invalid port number: %w", err)
	}

	return port, nil
}

// ReplacePort 替换地址中的端口号
func ReplacePort(address string, newPort int) (string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid address format: %w", err)
	}

	if host == "" {
		host = "0.0.0.0"
	}

	return fmt.Sprintf("%s:%d", host, newPort), nil
}

// IsPortConflictError 判断是否为端口冲突错误
func IsPortConflictError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "bind") ||
		strings.Contains(errMsg, "address already in use") ||
		strings.Contains(errMsg, "only one usage of each socket address")
}
