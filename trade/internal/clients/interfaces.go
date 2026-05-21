package clients

import aSDK "github.com/gagliardetto/solana-go"

type SignTransactionReq struct {
	OmniAccount   string
	WalletIndex   uint32
	ChainId       uint64
	Address       string
	MsgToSign     []byte
	UsingTreasury bool
}

type SignTransactionResp struct {
	Signature []byte
}

type TransferParam struct {
	SignInfo          *SignTransactionReq
	FromAddr          string
	ToAddr            string
	TokenAddr         string
	TokenDecimal      uint8
	Amount            string
	TokenOwnerProgram aSDK.PublicKey
	IsNative          bool
}
