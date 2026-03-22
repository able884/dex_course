package block

import (
	"fmt"

	"github.com/blocto/solana-go-sdk/client"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/mr-tron/base58"
	"richcode.cc/dex/pkg/types"
)

const (
	PumpStatusNotStart  = 0
	PumpStatusCreate    = -1
	PumpStatusTrading   = 1
	PumpStatusMigrating = 2
	PumpStatusEnd       = 3
)

func DecodePumpFunInstruction(inst *solTypes.CompiledInstruction, tx *client.BlockTransaction) (trade *types.TradeWithPair, err error) {
	fmt.Println("pump.fun transactions", base58.Encode(tx.Transaction.Signatures[0]))
	return
}

func GetInstructionDiscriminator(data []byte) []byte {
	if len(data) < 8 || data == nil {
		return nil
	}
	return data[:8]
}
