package block

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/mr-tron/base58"
)

const ProgramStrPumpFun = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
const ProgramStrPumpFunAMM = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"

var (
	PumpAmmBuyDiscriminator  = []byte{102, 6, 61, 18, 1, 218, 235, 234}
	PumpAmmSellDiscriminator = []byte{51, 230, 133, 164, 1, 127, 131, 173}
)

func DecodePumpFunInstruction(inst *types.CompiledInstruction, tx *client.BlockTransaction) (err error) {
	fmt.Println("pump.fun transactions", base58.Encode(tx.Transaction.Signatures[0]))
	return
}

func DecodePumpFunAMMInstruction(inst *types.CompiledInstruction, tx *client.BlockTransaction) (err error) {
	fmt.Println("pump.fun AMM transactions", base58.Encode(tx.Transaction.Signatures[0]))
	discriminator := GetInstructionDiscriminator(inst.Data)

	if bytes.Equal(discriminator, PumpAmmBuyDiscriminator) {
		fmt.Println("AMM Buy instruction")
	} else if bytes.Equal(discriminator, PumpAmmSellDiscriminator) {
		fmt.Println("AMM Sell instruction")
	} else {
		return errors.New("unknown instruction discriminator")
	}
	return
}

func GetInstructionDiscriminator(data []byte) []byte {
	if len(data) < 8 || data == nil {
		return nil
	}
	return data[:8]
}
