package clmm

import (
	"github.com/blocto/solana-go-sdk/common"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"

	ag_solanago "github.com/gagliardetto/solana-go"
)

var (
	ProgramRaydiumConcentratedLiquidity = common.PublicKeyFromString("CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK")
	ProgramClMMDevNet                   = common.PublicKeyFromString("GodmWN2QXikut2nwHuFGpCPeN6nBb2vuCFknVy3edxVw")
)

// SetDevnetProgramID sets the CLMM program ID to use the devnet deployed program
func SetDevnetProgramID() {
	devnetProgramID := ag_solanago.MustPublicKeyFromBase58("GodmWN2QXikut2nwHuFGpCPeN6nBb2vuCFknVy3edxVw")
	amm_v3.SetProgramID(devnetProgramID)
}
