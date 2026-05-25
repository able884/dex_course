package test

import (
	"context"
	"fmt"
	"testing"

	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"richcode.cc/dex/pkg/pumpfun/generated/pump"
)

func TestGetGlobal(t *testing.T) {
	programID := ag_solanago.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")

	// Global PDA
	globalSeeds := [][]byte{[]byte("global")}
	global, _, err := ag_solanago.FindProgramAddress(globalSeeds, programID)
	if err != nil {
		t.Fatalf("failed to find global PDA: %v", err)
	}

	rpcClient := ag_rpc.New("https://api.devnet.solana.com")
	globalAccount, err := rpcClient.GetAccountInfo(context.Background(), global)
	if err != nil {
		t.Fatalf("failed to get global account: %v", err)
	}

	globalConfigAccount, err := pump.ParseAnyAccount(globalAccount.Value.Data.GetBinary())
	if err != nil {
		t.Fatalf("failed to parse any account: %v", err)
	}
	fmt.Println((globalConfigAccount.(*pump.Global)).FeeRecipients)
}
