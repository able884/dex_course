package limitorder

import (
	"encoding/json"
	"fmt"
	"os"

	ag_solanago "github.com/gagliardetto/solana-go"
)

// LoadKeypairFromFile loads a Solana keypair from a JSON file
// The file should be in the standard Solana CLI wallet format (array of 64 bytes)
func LoadKeypairFromFile(filePath string) (ag_solanago.PrivateKey, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ag_solanago.PrivateKey{}, fmt.Errorf("failed to read keypair file: %w", err)
	}

	// Parse JSON array
	var keyBytes []byte
	if err := json.Unmarshal(data, &keyBytes); err != nil {
		return ag_solanago.PrivateKey{}, fmt.Errorf("failed to parse keypair JSON: %w", err)
	}

	// Validate length
	if len(keyBytes) != 64 {
		return ag_solanago.PrivateKey{}, fmt.Errorf("invalid keypair length: expected 64 bytes, got %d", len(keyBytes))
	}

	// Create private key
	privateKey := ag_solanago.PrivateKey(keyBytes)

	return privateKey, nil
}

// LoadKeypairFromFileOrBase58 从文件路径或Base58私钥字符串加载密钥对
// 1. 入参以'['开头 || 是合法文件路径 → 当做密钥文件读取
// 2. 其余场景 → 当做Base58编码私钥字符串解析
func LoadKeypairFromFileOrBase58(input string) (ag_solanago.PrivateKey, error) {
	// Check if input looks like a JSON array
	if len(input) > 0 && input[0] == '[' {
		// Direct JSON array string
		var keyBytes []byte
		if err := json.Unmarshal([]byte(input), &keyBytes); err != nil {
			return ag_solanago.PrivateKey{}, fmt.Errorf("failed to parse keypair JSON: %w", err)
		}
		if len(keyBytes) != 64 {
			return ag_solanago.PrivateKey{}, fmt.Errorf("invalid keypair length: expected 64 bytes, got %d", len(keyBytes))
		}
		return ag_solanago.PrivateKey(keyBytes), nil
	}

	// Check if input is a file path
	if fileExists(input) {
		return LoadKeypairFromFile(input)
	}

	// Try to parse as base58
	privateKey, err := ag_solanago.PrivateKeyFromBase58(input)
	if err != nil {
		return ag_solanago.PrivateKey{}, fmt.Errorf("failed to parse as base58 or file: %w", err)
	}

	return privateKey, nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetDefaultKeypairPath returns the default Solana CLI keypair path
func GetDefaultKeypairPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return homeDir + "/.config/solana/id.json"
}

// LoadDefaultKeypair loads the default Solana CLI keypair
func LoadDefaultKeypair() (ag_solanago.PrivateKey, error) {
	defaultPath := GetDefaultKeypairPath()
	if defaultPath == "" {
		return ag_solanago.PrivateKey{}, fmt.Errorf("failed to get home directory")
	}

	if !fileExists(defaultPath) {
		return ag_solanago.PrivateKey{}, fmt.Errorf("default keypair not found at %s", defaultPath)
	}

	return LoadKeypairFromFile(defaultPath)
}
