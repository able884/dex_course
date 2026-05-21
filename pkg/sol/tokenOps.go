package sol

import (
	"richcode.cc/dex/pkg/sol/associatedtoken2022account"
	token2022 "richcode.cc/dex/pkg/sol/token2022_new"

	bin "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/near/borsh-go"
)

type Instruction borsh.Enum

const (
	InstructionCreate Instruction = iota
	InstructionCreateIdempotent
	InstructionRecoverNested
)

var TypeIDCreateAtaIdempotent = bin.TypeIDFromUint8(1)

type CreateAtaIdempotentInstruction struct {
	*ata.Instruction
}

func (CreateAtaIdempotentInstruction) Data() ([]byte, error) {
	return borsh.Serialize(struct {
		Instruction Instruction
	}{
		Instruction: InstructionCreateIdempotent,
	})
}

func CreateAtaIdempotent(payer, walletAddress, mintAddress, owner aSDK.PublicKey) (aSDK.Instruction, error) {
	if owner == aSDK.Token2022ProgramID {
		return CreateToken2022AtaIdempotent(payer, walletAddress, mintAddress)
	}
	inst, err := ata.NewCreateInstruction(payer, walletAddress, mintAddress).ValidateAndBuild()
	if nil != err {
		return nil, err
	}
	// 修改指令为冥等创建-存在直接成功，不存在才创建
	inst.TypeID = TypeIDCreateAtaIdempotent

	a := &CreateAtaIdempotentInstruction{
		Instruction: inst,
	}

	return a, err
}

type CreateToken2022AtaIdempotentInstruction struct {
	*associatedtoken2022account.Instruction
}

func (CreateToken2022AtaIdempotentInstruction) Data() ([]byte, error) {
	return borsh.Serialize(struct {
		Instruction Instruction
	}{
		Instruction: InstructionCreateIdempotent,
	})
}

func CreateToken2022AtaIdempotent(payer, walletAddress, mintAddress aSDK.PublicKey) (aSDK.Instruction, error) {
	inst, err := associatedtoken2022account.NewCreateInstruction(payer, walletAddress, mintAddress).ValidateAndBuild()
	if nil != err {
		return nil, err
	}
	inst.TypeID = TypeIDCreateAtaIdempotent

	a := &CreateToken2022AtaIdempotentInstruction{
		Instruction: inst,
	}

	return a, err
}

func FindAssociatedTokenAddress(walletAddress, mintAddress, owner aSDK.PublicKey) (aSDK.PublicKey, uint8, error) {
	if owner == aSDK.Token2022ProgramID {
		return associatedtoken2022account.FindAssociatedToken2022Address(walletAddress, mintAddress)
	}
	return aSDK.FindAssociatedTokenAddress(walletAddress, mintAddress)
}

func NewTransferCheckedInstruction(
	amount uint64,
	decimals uint8,
	// Accounts:
	source aSDK.PublicKey,
	mint aSDK.PublicKey,
	destination aSDK.PublicKey,
	owner aSDK.PublicKey,
	multisigSigners []aSDK.PublicKey,
	tokenOwnerProgram aSDK.PublicKey,
) (aSDK.Instruction, error) {
	if tokenOwnerProgram == aSDK.Token2022ProgramID {
		return token2022.NewTransferCheckedInstruction(amount, decimals, source, mint, destination, owner, multisigSigners).ValidateAndBuild()
	}
	return token.NewTransferCheckedInstruction(amount, decimals, source, mint, destination, owner, multisigSigners).ValidateAndBuild()
}
