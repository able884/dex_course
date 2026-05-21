package cpmm

import (
	ag_solanago "github.com/gagliardetto/solana-go"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
)

// SetProgramID overrides the underlying Raydium CPMM program id that
// the generated instruction builders will use.
func SetProgramID(programID ag_solanago.PublicKey) {
	raydium_cp_swap.SetProgramID(programID)
}
