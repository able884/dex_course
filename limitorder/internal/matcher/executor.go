package matcher

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

// Executor handles on-chain execution of fills
type Executor struct {
	rpcClient      *ag_rpc.Client
	matcherKeypair *aSDK.Wallet
	programID      aSDK.PublicKey
}

// NewExecutor creates a new executor
func NewExecutor(rpcURL string, matcherPrivateKey string, programID string) (*Executor, error) {
	// 创建 RPC 客户端
	rpcClient := ag_rpc.New(rpcURL)

	// 解析 matcher 私钥
	privateKey, err := aSDK.PrivateKeyFromBase58(matcherPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid matcher private key: %w", err)
	}

	matcherKeypair := &aSDK.Wallet{
		PrivateKey: privateKey,
	}

	// 解析 program ID
	programPubkey, err := aSDK.PublicKeyFromBase58(programID)
	if err != nil {
		return nil, fmt.Errorf("invalid program ID: %w", err)
	}

	return &Executor{
		rpcClient:      rpcClient,
		matcherKeypair: matcherKeypair,
		programID:      programPubkey,
	}, nil
}

// 执行单个成交：构建并发送交易，调用链上 match_orders 指令
func (ex *Executor) ExecuteFill(ctx context.Context, fill *Fill, marketInfo *MarketInfo) error {
	logx.Infof("Executing fill on-chain: maker=%s, taker=%s, price=%d, qty=%d",
		fill.MakerOrderPDA, fill.TakerOrderPDA, fill.Price, fill.Quantity)

	// 1. 派生所有需要的 PDA
	configPDA, err := ex.deriveConfigPDA()
	if err != nil {
		return fmt.Errorf("failed to derive config PDA: %w", err)
	}

	marketPDA, err := aSDK.PublicKeyFromBase58(marketInfo.MarketPDA)
	if err != nil {
		return fmt.Errorf("invalid market PDA: %w", err)
	}

	makerMarginPDA, err := ex.deriveMarginPDA(marketPDA, fill.MakerOwner)
	if err != nil {
		return fmt.Errorf("failed to derive maker margin PDA: %w", err)
	}

	takerMarginPDA, err := ex.deriveMarginPDA(marketPDA, fill.TakerOwner)
	if err != nil {
		return fmt.Errorf("failed to derive taker margin PDA: %w", err)
	}

	makerOrderPDA, err := aSDK.PublicKeyFromBase58(fill.MakerOrderPDA)
	if err != nil {
		return fmt.Errorf("invalid maker order PDA: %w", err)
	}

	takerOrderPDA, err := aSDK.PublicKeyFromBase58(fill.TakerOrderPDA)
	if err != nil {
		return fmt.Errorf("invalid taker order PDA: %w", err)
	}

	// 解析 pyth price feed（可选）
	var pythPriceFeed aSDK.PublicKey
	if marketInfo.PythPriceFeed != "" {
		var err error
		pythPriceFeed, err = aSDK.PublicKeyFromBase58(marketInfo.PythPriceFeed)
		if err != nil {
			return fmt.Errorf("invalid pyth price feed: %w", err)
		}
	} else {
		// 如果没有配置 Pyth price feed，使用默认的系统程序 ID
		// 这样链上验证会跳过价格检查
		pythPriceFeed = aSDK.SystemProgramID
		logx.Infof("No Pyth price feed configured for market %s, using system program as placeholder", marketInfo.MarketPDA)
	}

	// 2. 构建 match_orders 指令
	instruction, err := ex.buildMatchOrdersInstruction(
		configPDA,
		marketPDA,
		makerMarginPDA,
		takerMarginPDA,
		makerOrderPDA,
		takerOrderPDA,
		pythPriceFeed,
		fill.Quantity,
	)
	if err != nil {
		return fmt.Errorf("failed to build instruction: %w", err)
	}

	// 3. 获取最新区块哈希
	recent, err := ex.rpcClient.GetLatestBlockhash(ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// 4. 构建交易
	tx, err := aSDK.NewTransaction(
		[]aSDK.Instruction{instruction},
		recent.Value.Blockhash,
		aSDK.TransactionPayer(ex.matcherKeypair.PublicKey()),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// 5. 签名交易
	_, err = tx.Sign(func(key aSDK.PublicKey) *aSDK.PrivateKey {
		if key.Equals(ex.matcherKeypair.PublicKey()) {
			return &ex.matcherKeypair.PrivateKey
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// 6. 发送交易
	sig, err := ex.rpcClient.SendTransactionWithOpts(
		ctx,
		tx,
		ag_rpc.TransactionOpts{
			SkipPreflight:       false,
			PreflightCommitment: ag_rpc.CommitmentProcessed,
		},
	)
	fmt.Println(makerOrderPDA, takerOrderPDA, err)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	logx.Infof("Match transaction sent: signature=%s", sig)

	// 7. 等待确认（使用轮询方式）
	for i := 0; i < 30; i++ { // 最多等待 30 秒
		time.Sleep(1 * time.Second)

		statuses, err := ex.rpcClient.GetSignatureStatuses(ctx, true, sig)
		if err != nil {
			continue
		}

		if statuses != nil && statuses.Value != nil && len(statuses.Value) > 0 {
			status := statuses.Value[0]
			if status != nil {
				if status.Err != nil {
					return fmt.Errorf("transaction failed: %v", status.Err)
				}
				if status.ConfirmationStatus == ag_rpc.ConfirmationStatusConfirmed ||
					status.ConfirmationStatus == ag_rpc.ConfirmationStatusFinalized {
					logx.Infof("Match transaction confirmed: signature=%s", sig)
					return nil
				}
			}
		}
	}

	return fmt.Errorf("transaction confirmation timeout")
}

// 构建 match_orders 指令，包含所有必要的账户和参数
func (ex *Executor) buildMatchOrdersInstruction(
	config aSDK.PublicKey,
	market aSDK.PublicKey,
	makerMargin aSDK.PublicKey,
	takerMargin aSDK.PublicKey,
	makerOrder aSDK.PublicKey,
	takerOrder aSDK.PublicKey,
	pythPriceFeed aSDK.PublicKey,
	matchQtyLots int64,
) (aSDK.Instruction, error) {
	// match_orders discriminator: [17, 1, 201, 93, 7, 51, 251, 134]
	discriminator := []byte{17, 1, 201, 93, 7, 51, 251, 134}

	// Serialize MatchParams
	data := make([]byte, 0, 16)
	data = append(data, discriminator...)

	// match_qty_lots (u64, little-endian)
	qtyBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(qtyBytes, uint64(matchQtyLots))
	data = append(data, qtyBytes...)

	// Build accounts array
	accounts := []*aSDK.AccountMeta{
		{PublicKey: config, IsWritable: false, IsSigner: false},                       // 0: config
		{PublicKey: market, IsWritable: true, IsSigner: false},                        // 1: market (must be writable)
		{PublicKey: makerMargin, IsWritable: true, IsSigner: false},                   // 2: maker_margin
		{PublicKey: takerMargin, IsWritable: true, IsSigner: false},                   // 3: taker_margin
		{PublicKey: makerOrder, IsWritable: true, IsSigner: false},                    // 4: maker_order
		{PublicKey: takerOrder, IsWritable: true, IsSigner: false},                    // 5: taker_order
		{PublicKey: pythPriceFeed, IsWritable: false, IsSigner: false},                // 6: pyth_price_feed
		{PublicKey: ex.matcherKeypair.PublicKey(), IsWritable: false, IsSigner: true}, // 7: caller (matcher)
	}

	return aSDK.NewInstruction(
		ex.programID,
		accounts,
		data,
	), nil
}

// deriveConfigPDA derives the config PDA
func (ex *Executor) deriveConfigPDA() (aSDK.PublicKey, error) {
	pda, _, err := aSDK.FindProgramAddress(
		[][]byte{[]byte("config")},
		ex.programID,
	)
	return pda, err
}

// deriveMarginPDA derives a margin PDA for a user
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

// MarketInfo contains market information needed for execution
type MarketInfo struct {
	MarketPDA     string
	BaseMint      string
	QuoteMint     string
	PythPriceFeed string
}
