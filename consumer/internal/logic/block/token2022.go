package block

import (
	"context"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/duke-git/lancet/v2/slice"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_token "github.com/gagliardetto/solana-go/programs/token"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/pkg/types"
)

type Token2022Decoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction
	innerInstruction    *client.InnerInstruction
}

func (decoder *Token2022Decoder) DecodeToken2022DecoderInstruction() (*types.TradeWithPair, error) {
	accountMetas := slice.Map[int, *ag_solanago.AccountMeta](decoder.compiledInstruction.Accounts, func(_ int, index int) *ag_solanago.AccountMeta {
		return &ag_solanago.AccountMeta{
			PublicKey: ag_solanago.PublicKeyFromBytes(decoder.dtx.Tx.AccountKeys[index].Bytes()),
			// no need
			IsWritable: false,
			IsSigner:   false,
		}
	})

	switch decoder.compiledInstruction.Data[0] {

	case ag_token.Instruction_MintTo:
		mintTo := ag_token.MintTo{}
		_ = mintTo.UnmarshalWithDecoder(ag_binary.NewBorshDecoder(decoder.compiledInstruction.Data[1:]))
		_ = mintTo.SetAccounts(accountMetas)

		trade := &types.TradeWithPair{
			InstructionMintTo: types.InstructionMintTo{
				Mint:   common.PublicKeyFromBytes(mintTo.GetMintAccount().PublicKey.Bytes()),
				To:     common.PublicKeyFromBytes(mintTo.GetDestinationAccount().PublicKey.Bytes()),
				Amount: *mintTo.Amount,
			},
			Type:   types.TradeTokenMint,
			TxHash: decoder.dtx.TxHash,
		}

		return trade, nil
	case ag_token.Instruction_Burn:

		burn := ag_token.Burn{}
		_ = burn.UnmarshalWithDecoder(ag_binary.NewBorshDecoder(decoder.compiledInstruction.Data[1:]))
		_ = burn.SetAccounts(accountMetas)

		trade := &types.TradeWithPair{
			InstructionBurn: types.InstructionBurn{
				Mint:    common.PublicKeyFromBytes(burn.GetMintAccount().PublicKey.Bytes()),
				Account: common.PublicKeyFromBytes(burn.GetSourceAccount().PublicKey.Bytes()),
				Amount:  *burn.Amount,
			},
			Type:   types.TradeTokenBurn,
			TxHash: decoder.dtx.TxHash,
		}

		return trade, nil

	default:
		return nil, ErrNotSupportInstruction
	}
}
