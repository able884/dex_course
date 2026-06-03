package limitorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ag_solanago "github.com/gagliardetto/solana-go"
)

// TestLoadKeypairFromFile tests loading a keypair from a JSON file
func TestLoadKeypairFromFile(t *testing.T) {
	// Create a temporary keypair file
	tmpDir := t.TempDir()
	keypairPath := filepath.Join(tmpDir, "test-keypair.json")

	// Generate a test keypair
	testKeypair := ag_solanago.NewWallet()
	keyBytes := testKeypair.PrivateKey[:]

	// Write to file
	data, err := json.Marshal(keyBytes)
	if err != nil {
		t.Fatalf("Failed to marshal keypair: %v", err)
	}

	if err := os.WriteFile(keypairPath, data, 0600); err != nil {
		t.Fatalf("Failed to write keypair file: %v", err)
	}

	// Load keypair
	loadedKey, err := LoadKeypairFromFile(keypairPath)
	if err != nil {
		t.Fatalf("Failed to load keypair: %v", err)
	}

	// Verify
	if !loadedKey.PublicKey().Equals(testKeypair.PublicKey()) {
		t.Errorf("Loaded keypair public key mismatch")
	}

	t.Logf("✅ Successfully loaded keypair from file")
	t.Logf("   Public Key: %s", loadedKey.PublicKey().String())
}

// TestLoadKeypairFromFileOrBase58_File tests loading from file
func TestLoadKeypairFromFileOrBase58_File(t *testing.T) {
	// Create a temporary keypair file
	tmpDir := t.TempDir()
	keypairPath := filepath.Join(tmpDir, "test-keypair.json")

	// Generate a test keypair
	testKeypair := ag_solanago.NewWallet()
	keyBytes := testKeypair.PrivateKey[:]

	// Write to file
	data, err := json.Marshal(keyBytes)
	if err != nil {
		t.Fatalf("Failed to marshal keypair: %v", err)
	}

	if err := os.WriteFile(keypairPath, data, 0600); err != nil {
		t.Fatalf("Failed to write keypair file: %v", err)
	}

	// Load using the flexible function
	loadedKey, err := LoadKeypairFromFileOrBase58(keypairPath)
	if err != nil {
		t.Fatalf("Failed to load keypair: %v", err)
	}

	// Verify
	if !loadedKey.PublicKey().Equals(testKeypair.PublicKey()) {
		t.Errorf("Loaded keypair public key mismatch")
	}

	t.Logf("✅ Successfully loaded keypair from file path")
}

// TestLoadKeypairFromFileOrBase58_Base58 tests loading from base58 string
func TestLoadKeypairFromFileOrBase58_Base58(t *testing.T) {
	// Generate a test keypair
	testKeypair := ag_solanago.NewWallet()
	base58Key := testKeypair.PrivateKey.String()

	// Load using the flexible function
	loadedKey, err := LoadKeypairFromFileOrBase58(base58Key)
	if err != nil {
		t.Fatalf("Failed to load keypair from base58: %v", err)
	}

	// Verify
	if !loadedKey.PublicKey().Equals(testKeypair.PublicKey()) {
		t.Errorf("Loaded keypair public key mismatch")
	}

	t.Logf("✅ Successfully loaded keypair from base58 string")
}

// TestLoadKeypairFromFileOrBase58_JSONArray tests loading from JSON array string
func TestLoadKeypairFromFileOrBase58_JSONArray(t *testing.T) {
	// Generate a test keypair
	testKeypair := ag_solanago.NewWallet()
	keyBytes := testKeypair.PrivateKey[:]

	// Create JSON array string manually (as array of numbers, not base64)
	// This is the format used by Solana CLI
	jsonStr := "["
	for i, b := range keyBytes {
		if i > 0 {
			jsonStr += ","
		}
		jsonStr += fmt.Sprintf("%d", b)
	}
	jsonStr += "]"

	t.Logf("JSON string: %s", jsonStr[:50]+"...") // Log first 50 chars

	// Load using the flexible function
	loadedKey, err := LoadKeypairFromFileOrBase58(jsonStr)
	if err != nil {
		t.Fatalf("Failed to load keypair from JSON array: %v", err)
	}

	// Verify
	if !loadedKey.PublicKey().Equals(testKeypair.PublicKey()) {
		t.Errorf("Loaded keypair public key mismatch")
	}

	t.Logf("✅ Successfully loaded keypair from JSON array string")
}

// TestLoadKeypairFromFile_InvalidFile tests error handling
func TestLoadKeypairFromFile_InvalidFile(t *testing.T) {
	// Try to load non-existent file
	_, err := LoadKeypairFromFile("/nonexistent/path/keypair.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
	t.Logf("✅ Correctly handled non-existent file: %v", err)
}

// TestLoadKeypairFromFile_InvalidJSON tests invalid JSON handling
func TestLoadKeypairFromFile_InvalidJSON(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpDir := t.TempDir()
	keypairPath := filepath.Join(tmpDir, "invalid-keypair.json")

	if err := os.WriteFile(keypairPath, []byte("invalid json"), 0600); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	// Try to load
	_, err := LoadKeypairFromFile(keypairPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	t.Logf("✅ Correctly handled invalid JSON: %v", err)
}

// TestLoadKeypairFromFile_InvalidLength tests invalid key length handling
func TestLoadKeypairFromFile_InvalidLength(t *testing.T) {
	// Create a temporary file with wrong length
	tmpDir := t.TempDir()
	keypairPath := filepath.Join(tmpDir, "short-keypair.json")

	// Write a short key (32 bytes instead of 64)
	shortKey := make([]byte, 32)
	data, _ := json.Marshal(shortKey)

	if err := os.WriteFile(keypairPath, data, 0600); err != nil {
		t.Fatalf("Failed to write short key file: %v", err)
	}

	// Try to load
	_, err := LoadKeypairFromFile(keypairPath)
	if err == nil {
		t.Error("Expected error for invalid key length")
	}
	t.Logf("✅ Correctly handled invalid key length: %v", err)
}

// TestGetDefaultKeypairPath tests getting the default keypair path
func TestGetDefaultKeypairPath(t *testing.T) {
	path := GetDefaultKeypairPath()
	if path == "" {
		t.Skip("Could not determine home directory")
	}

	t.Logf("Default keypair path: %s", path)

	// Check if it follows the expected pattern
	if filepath.Base(path) != "id.json" {
		t.Errorf("Expected default keypair filename to be 'id.json', got %s", filepath.Base(path))
	}
}

// TestLoadDefaultKeypair tests loading the default Solana CLI keypair
func TestLoadDefaultKeypair(t *testing.T) {
	// This test will skip if the default keypair doesn't exist
	keypair, err := LoadDefaultKeypair()
	if err != nil {
		t.Skipf("Default keypair not found (this is expected if not configured): %v", err)
	}

	t.Logf("✅ Successfully loaded default keypair")
	t.Logf("   Public Key: %s", keypair.PublicKey().String())
}
