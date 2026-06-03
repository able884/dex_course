package cranker

import (
	"context"
	"fmt"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

// Executor handles on-chain sweep_expired execution.
type Executor struct {
	rpcClient *ag_rpc.Client
	signer    *aSDK.Wallet
	programID aSDK.PublicKey
}

// NewExecutor creates a new cranker executor.
func NewExecutor(rpcClient *ag_rpc.Client, signerPrivateKey string, programID string) (*Executor, error) {
	if rpcClient == nil {
		return nil, fmt.Errorf("rpc client is nil")
	}

	privateKey, err := aSDK.PrivateKeyFromBase58(signerPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid cranker private key: %w", err)
	}

	programPubkey, err := aSDK.PublicKeyFromBase58(programID)
	if err != nil {
		return nil, fmt.Errorf("invalid program ID: %w", err)
	}

	return &Executor{
		rpcClient: rpcClient,
		signer: &aSDK.Wallet{
			PrivateKey: privateKey,
		},
		programID: programPubkey,
	}, nil
}

// SweepExpired sends a sweep_expired transaction on-chain.
func (ex *Executor) SweepExpired(ctx context.Context, order *ExpiredOrder) (string, error) {
	if order == nil {
		return "", fmt.Errorf("order is nil")
	}

	logx.Infof("Executing sweep_expired on-chain: order=%s, market=%s, owner=%s",
		order.OrderPDA, order.MarketPDA, order.Owner)

	configPDA, err := ex.deriveConfigPDA()
	if err != nil {
		return "", fmt.Errorf("failed to derive config PDA: %w", err)
	}

	marketPDA, err := aSDK.PublicKeyFromBase58(order.MarketPDA)
	if err != nil {
		return "", fmt.Errorf("invalid market PDA: %w", err)
	}

	marginPDA, err := ex.deriveMarginPDA(marketPDA, order.Owner)
	if err != nil {
		return "", fmt.Errorf("failed to derive margin PDA: %w", err)
	}

	orderPDA, err := aSDK.PublicKeyFromBase58(order.OrderPDA)
	if err != nil {
		return "", fmt.Errorf("invalid order PDA: %w", err)
	}

	instruction := ex.buildSweepExpiredInstruction(
		configPDA,
		marketPDA,
		marginPDA,
		orderPDA,
	)

	recent, err := ex.rpcClient.GetLatestBlockhash(ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	tx, err := aSDK.NewTransaction(
		[]aSDK.Instruction{instruction},
		recent.Value.Blockhash,
		aSDK.TransactionPayer(ex.signer.PublicKey()),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	_, err = tx.Sign(func(key aSDK.PublicKey) *aSDK.PrivateKey {
		if key.Equals(ex.signer.PublicKey()) {
			return &ex.signer.PrivateKey
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	sig, err := ex.rpcClient.SendTransactionWithOpts(
		ctx,
		tx,
		ag_rpc.TransactionOpts{
			SkipPreflight:       false,
			PreflightCommitment: ag_rpc.CommitmentProcessed,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	logx.Infof("SweepExpired transaction sent: signature=%s", sig)

	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)

		statuses, err := ex.rpcClient.GetSignatureStatuses(ctx, true, sig)
		if err != nil {
			continue
		}

		if statuses != nil && statuses.Value != nil && len(statuses.Value) > 0 {
			status := statuses.Value[0]
			if status != nil {
				if status.Err != nil {
					return "", fmt.Errorf("transaction failed: %v", status.Err)
				}
				if status.ConfirmationStatus == ag_rpc.ConfirmationStatusConfirmed ||
					status.ConfirmationStatus == ag_rpc.ConfirmationStatusFinalized {
					logx.Infof("SweepExpired transaction confirmed: signature=%s", sig)
					return sig.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("transaction confirmation timeout")
}

func (ex *Executor) buildSweepExpiredInstruction(
	config aSDK.PublicKey,
	market aSDK.PublicKey,
	margin aSDK.PublicKey,
	order aSDK.PublicKey,
) aSDK.Instruction {
	discriminator := []byte{10, 72, 70, 57, 62, 128, 19, 22}

	accounts := []*aSDK.AccountMeta{
		{PublicKey: config, IsWritable: false, IsSigner: false},
		{PublicKey: market, IsWritable: false, IsSigner: false},
		{PublicKey: margin, IsWritable: true, IsSigner: false},
		{PublicKey: order, IsWritable: true, IsSigner: false},
	}

	return aSDK.NewInstruction(
		ex.programID,
		accounts,
		discriminator,
	)
}

func (ex *Executor) deriveConfigPDA() (aSDK.PublicKey, error) {
	pda, _, err := aSDK.FindProgramAddress(
		[][]byte{[]byte("config")},
		ex.programID,
	)
	return pda, err
}

func (ex *Executor) deriveMarginPDA(market aSDK.PublicKey, owner string) (aSDK.PublicKey, error) {
	ownerPubkey, err := aSDK.PublicKeyFromBase58(owner)
	if err != nil {
		return aSDK.PublicKey{}, fmt.Errorf("invalid owner: %w", err)
	}

	pda, _, err := aSDK.FindProgramAddress(
		[][]byte{
			[]byte("margin"),
			market[:],
			ownerPubkey[:],
		},
		ex.programID,
	)
	return pda, err
}
