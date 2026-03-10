package block

import (
	"bytes"
	"encoding/binary"
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
		DecodePumpFunAMMBuyInstruction(inst, tx)
	} else if bytes.Equal(discriminator, PumpAmmSellDiscriminator) {
		DecodePumpFunAMMSellInstruction(inst, tx)
	} else {
		return errors.New("unknown instruction discriminator")
	}
	return
}

func DecodePumpFunAMMBuyInstruction(inst *types.CompiledInstruction, tx *client.BlockTransaction) (err error) {
	fmt.Println("AMM Buy instruction", base58.Encode(tx.Transaction.Signatures[0]))
	if len(inst.Accounts) < 23 {
		return fmt.Errorf("invalid accounts length %d", len(inst.Accounts))
	}
	pool := tx.AccountKeys[inst.Accounts[0]]
	user := tx.AccountKeys[inst.Accounts[1]]
	globalConfig := tx.AccountKeys[inst.Accounts[2]]
	baseMint := tx.AccountKeys[inst.Accounts[3]]
	quoteMint := tx.AccountKeys[inst.Accounts[4]]
	userBaseTokenAccount := tx.AccountKeys[inst.Accounts[5]]
	userQuoteTokenAccount := tx.AccountKeys[inst.Accounts[6]]
	poolBaseTokenAccount := tx.AccountKeys[inst.Accounts[7]]
	poolQuoteTokenAccount := tx.AccountKeys[inst.Accounts[8]]
	protocolFeeRecipient := tx.AccountKeys[inst.Accounts[9]]
	protocolFeeRecipientTokenAccount := tx.AccountKeys[inst.Accounts[10]]
	baseTokenProgram := tx.AccountKeys[inst.Accounts[11]]
	quoteTokenProgram := tx.AccountKeys[inst.Accounts[12]]
	systemProgram := tx.AccountKeys[inst.Accounts[13]]
	associatedTokenProgram := tx.AccountKeys[inst.Accounts[14]]
	eventAuthority := tx.AccountKeys[inst.Accounts[15]]
	program := tx.AccountKeys[inst.Accounts[16]]
	coinCreatorVaultAta := tx.AccountKeys[inst.Accounts[17]]
	coinCreatorVaultAuthority := tx.AccountKeys[inst.Accounts[18]]
	globalVolumeAccumulator := tx.AccountKeys[inst.Accounts[19]]
	userVolumeAccumulator := tx.AccountKeys[inst.Accounts[20]]
	feeConfig := tx.AccountKeys[inst.Accounts[21]]
	feeProgram := tx.AccountKeys[inst.Accounts[22]]

	fmt.Println("---------------------buy accounts----------------------")

	fmt.Println("pool:", pool.String())
	fmt.Println("user:", user.String())
	fmt.Println("globalConfig:", globalConfig.String())
	fmt.Println("baseMint:", baseMint.String())
	fmt.Println("quoteMint:", quoteMint.String())
	fmt.Println("userBaseTokenAccount:", userBaseTokenAccount.String())
	fmt.Println("userQuoteTokenAccount:", userQuoteTokenAccount.String())
	fmt.Println("poolBaseTokenAccount:", poolBaseTokenAccount.String())
	fmt.Println("poolQuoteTokenAccount:", poolQuoteTokenAccount.String())
	fmt.Println("protocolFeeRecipient:", protocolFeeRecipient.String())
	fmt.Println("protocolFeeRecipientTokenAccount:", protocolFeeRecipientTokenAccount.String())
	fmt.Println("baseTokenProgram:", baseTokenProgram.String())
	fmt.Println("quoteTokenProgram:", quoteTokenProgram.String())
	fmt.Println("systemProgram:", systemProgram.String())
	fmt.Println("associatedTokenProgram:", associatedTokenProgram.String())
	fmt.Println("eventAuthority:", eventAuthority.String())
	fmt.Println("program:", program.String())
	fmt.Println("coinCreatorVaultAta:", coinCreatorVaultAta.String())
	fmt.Println("coinCreatorVaultAuthority:", coinCreatorVaultAuthority.String())
	fmt.Println("globalVolumeAccumulator:", globalVolumeAccumulator.String())
	fmt.Println("userVolumeAccumulator:", userVolumeAccumulator.String())
	fmt.Println("feeConfig:", feeConfig.String())
	fmt.Println("feeProgram:", feeProgram.String())

	fmt.Println("--------------------- buy data --------------------")

	fmt.Println("baseAmountOut:", binary.LittleEndian.Uint64(inst.Data[8:16]))
	fmt.Println("maxQuoteAmountIn:", binary.LittleEndian.Uint64(inst.Data[16:24]))

	return

}

func DecodePumpFunAMMSellInstruction(inst *types.CompiledInstruction, tx *client.BlockTransaction) (err error) {
	fmt.Println("pump.fun AMM Sell instruction", base58.Encode(tx.Transaction.Signatures[0]))
	// 解析账户数据: 回顾交易解析（账户存储结构、指令数据结构）
	if len(inst.Accounts) != 21 {
		return fmt.Errorf("invalid accounts length: %d", len(inst.Accounts))
	}

	pool := tx.AccountKeys[inst.Accounts[0]]
	user := tx.AccountKeys[inst.Accounts[1]]
	globalConfig := tx.AccountKeys[inst.Accounts[2]]
	baseMint := tx.AccountKeys[inst.Accounts[3]]
	quoteMint := tx.AccountKeys[inst.Accounts[4]]
	userBaseTokenAccount := tx.AccountKeys[inst.Accounts[5]]
	userQuoteTokenAccount := tx.AccountKeys[inst.Accounts[6]]
	poolBaseTokenAccount := tx.AccountKeys[inst.Accounts[7]]
	poolQuoteTokenAccount := tx.AccountKeys[inst.Accounts[8]]
	protocolFeeRecipient := tx.AccountKeys[inst.Accounts[9]]
	protocolFeeRecipientTokenAccount := tx.AccountKeys[inst.Accounts[10]]
	baseTokenProgram := tx.AccountKeys[inst.Accounts[11]]
	quoteTokenProgram := tx.AccountKeys[inst.Accounts[12]]
	systemProgram := tx.AccountKeys[inst.Accounts[13]]
	associatedTokenProgram := tx.AccountKeys[inst.Accounts[14]]
	eventAuthority := tx.AccountKeys[inst.Accounts[15]]
	program := tx.AccountKeys[inst.Accounts[16]]
	coinCreatorVaultAta := tx.AccountKeys[inst.Accounts[17]]
	coinCreatorVaultAuthority := tx.AccountKeys[inst.Accounts[18]]
	feeConfig := tx.AccountKeys[inst.Accounts[19]]
	feeProgram := tx.AccountKeys[inst.Accounts[20]]

	fmt.Println("------------------- sell accounts ------------------")

	fmt.Println("pool:", pool.String())
	fmt.Println("user:", user.String())
	fmt.Println("globalConfig:", globalConfig.String())
	fmt.Println("baseMint:", baseMint.String())
	fmt.Println("quoteMint:", quoteMint.String())
	fmt.Println("userBaseTokenAccount:", userBaseTokenAccount.String())
	fmt.Println("userQuoteTokenAccount:", userQuoteTokenAccount.String())
	fmt.Println("poolBaseTokenAccount:", poolBaseTokenAccount.String())
	fmt.Println("poolQuoteTokenAccount:", poolQuoteTokenAccount.String())
	fmt.Println("protocolFeeRecipient:", protocolFeeRecipient.String())
	fmt.Println("protocolFeeRecipientTokenAccount:", protocolFeeRecipientTokenAccount.String())
	fmt.Println("baseTokenProgram:", baseTokenProgram.String())
	fmt.Println("quoteTokenProgram:", quoteTokenProgram.String())
	fmt.Println("systemProgram:", systemProgram.String())
	fmt.Println("associatedTokenProgram:", associatedTokenProgram.String())
	fmt.Println("eventAuthority:", eventAuthority.String())
	fmt.Println("program:", program.String())
	fmt.Println("coinCreatorVaultAta:", coinCreatorVaultAta.String())
	fmt.Println("coinCreatorVaultAuthority:", coinCreatorVaultAuthority.String())
	fmt.Println("feeConfig:", feeConfig.String())
	fmt.Println("feeProgram:", feeProgram.String())

	fmt.Println("--------------------- sell data --------------------")

	fmt.Println("baseAmountIn:", binary.LittleEndian.Uint64(inst.Data[8:16]))
	fmt.Println("minQuoteAmountOut:", binary.LittleEndian.Uint64(inst.Data[16:24]))

	return
}

func GetInstructionDiscriminator(data []byte) []byte {
	if len(data) < 8 || data == nil {
		return nil
	}
	return data[:8]
}
