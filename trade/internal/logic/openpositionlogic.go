package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	ag_binary "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type OpenPositionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewOpenPositionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OpenPositionLogic {
	return &OpenPositionLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *OpenPositionLogic) OpenPosition(in *trade.OpenPositionRequest) (*trade.OpenPositionResponse, error) {
	if err := l.validate(in); err != nil {
		return nil, err
	}
	txBase64, err := l.buildOpenPositionTx(in)
	if err != nil {
		return nil, err
	}
	return &trade.OpenPositionResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *OpenPositionLogic) validate(in *trade.OpenPositionRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(in.PoolState) == "" {
		return errors.New("pool_state required")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address required")
	}
	token0Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token0Amount))
	if err != nil {
		return fmt.Errorf("token0_amount invalid: %w", err)
	}
	if token0Dec.Cmp(decimal.Zero) <= 0 {
		return errors.New("token0_amount must be greater than zero")
	}
	token1Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token1Amount))
	if err != nil {
		return fmt.Errorf("token1_amount invalid: %w", err)
	}
	if token1Dec.Cmp(decimal.Zero) <= 0 {
		return errors.New("token1_amount must be greater than zero")
	}
	return nil
}

func (l *OpenPositionLogic) buildOpenPositionTx(in *trade.OpenPositionRequest) (string, error) {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("invalid user wallet address: %w", err)
	}

	poolStatePK, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.PoolState))
	if err != nil {
		return "", fmt.Errorf("invalid pool_state: %w", err)
	}

	// 从链上获取 PoolState 账户数据
	poolStateInfo, err := rpcClient.GetAccountInfoWithOpts(l.ctx, poolStatePK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get pool state account: %w", err)
	}
	if poolStateInfo == nil || poolStateInfo.Value == nil {
		return "", errors.New("pool state account not found")
	}

	// 解析 PoolState 账户数据
	data := poolStateInfo.Value.Data.GetBinary()
	decoder := ag_binary.NewBorshDecoder(data)
	var poolState amm_v3.PoolStateAccount
	if err := poolState.UnmarshalWithDecoder(decoder); err != nil {
		return "", fmt.Errorf("failed to decode pool state: %w", err)
	}

	token0Mint := poolState.TokenMint0
	token1Mint := poolState.TokenMint1
	tokenVault0 := poolState.TokenVault0
	tokenVault1 := poolState.TokenVault1
	tickSpacing := int64(poolState.TickSpacing)
	decimals0 := int64(poolState.MintDecimals0)
	decimals1 := int64(poolState.MintDecimals1)

	// 获取 token program
	tokenProgram0 := aSDK.TokenProgramID
	tokenProgram1 := aSDK.TokenProgramID
	if !poolState.TokenMint0.IsZero() {
		mint0Info, err := rpcClient.GetAccountInfoWithOpts(l.ctx, token0Mint, &ag_rpc.GetAccountInfoOpts{
			Commitment: ag_rpc.CommitmentFinalized,
		})
		if err == nil && mint0Info != nil && mint0Info.Value != nil {
			tokenProgram0 = mint0Info.Value.Owner
		}
	}
	if !poolState.TokenMint1.IsZero() {
		mint1Info, err := rpcClient.GetAccountInfoWithOpts(l.ctx, token1Mint, &ag_rpc.GetAccountInfoOpts{
			Commitment: ag_rpc.CommitmentFinalized,
		})
		if err == nil && mint1Info != nil && mint1Info.Value != nil {
			tokenProgram1 = mint1Info.Value.Owner
		}
	}

	// 解析用户输入的金额
	token0Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token0Amount))
	if err != nil {
		return "", fmt.Errorf("token0_amount invalid: %w", err)
	}
	token1Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token1Amount))
	if err != nil {
		return "", fmt.Errorf("token1_amount invalid: %w", err)
	}

	// 转换为原子单位
	amount0Atomic, err := toAtomicAmount(token0Dec, decimals0)
	if err != nil {
		return "", fmt.Errorf("amount0 conversion failed: %w", err)
	}
	amount1Atomic, err := toAtomicAmount(token1Dec, decimals1)
	if err != nil {
		return "", fmt.Errorf("amount1 conversion failed: %w", err)
	}

	// 计算 tick 范围
	tickLower, tickUpper, err := l.calculateTickRange(in, poolState, decimals0, decimals1, tickSpacing)
	if err != nil {
		return "", fmt.Errorf("calculate tick range failed: %w", err)
	}

	// 计算 tick array start indices
	tickArrayLowerStart := calcTickArrayStartIndex(tickLower, int32(tickSpacing))
	tickArrayUpperStart := calcTickArrayStartIndex(tickUpper, int32(tickSpacing))

	slippageBps := in.SlippageBps
	if slippageBps == 0 {
		slippageBps = 50 // 默认 0.5%
	}
	if slippageBps > 10000 {
		slippageBps = 10000 // 最大 100%
	}

	amount0Max, err := applySlippage(amount0Atomic, slippageBps)
	if err != nil {
		return "", fmt.Errorf("apply slippage to amount0 failed: %w", err)
	}
	amount1Max, err := applySlippage(amount1Atomic, slippageBps)
	if err != nil {
		return "", fmt.Errorf("apply slippage to amount1 failed: %w", err)
	}

	// 生成 position mint（需要作为签名者）
	positionMintKeypair, err := aSDK.NewRandomPrivateKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate position mint key: %w", err)
	}
	positionMint := positionMintKeypair.PublicKey()

	// 构建 OpenPositionWithToken22Nft 指令
	openIx, err := l.buildOpenPositionInstruction(openPositionParams{
		UserWallet:          userWallet,
		PoolState:           poolStatePK,
		TokenVault0:         tokenVault0,
		TokenVault1:         tokenVault1,
		TokenMint0:          token0Mint,
		TokenMint1:          token1Mint,
		TokenProgram0:       tokenProgram0,
		TokenProgram1:       tokenProgram1,
		Amount0:             amount0Atomic,
		Amount1:             amount1Atomic,
		Amount0Max:          amount0Max,
		Amount1Max:          amount1Max,
		TickLower:           tickLower,
		TickUpper:           tickUpper,
		TickArrayLowerStart: tickArrayLowerStart,
		TickArrayUpperStart: tickArrayUpperStart,
		TickSpacing:         tickSpacing,
		PositionMint:        positionMint,
	})
	if err != nil {
		return "", fmt.Errorf("build open position instruction failed: %w", err)
	}

	// 确保 ATA 账户存在
	var setupInstructions []aSDK.Instruction
	tokenAccount0, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token0Mint)
	if err != nil {
		return "", fmt.Errorf("derive token account0 failed: %w", err)
	}
	tokenAccount1, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token1Mint)
	if err != nil {
		return "", fmt.Errorf("derive token account1 failed: %w", err)
	}

	ensureATA := func(mint aSDK.PublicKey, ata aSDK.PublicKey, tokenProgram aSDK.PublicKey) error {
		exists, err := accountExists(l.ctx, rpcClient, ata)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		// 使用标准 ATA 创建指令
		ix, err := associatedtokenaccount.NewCreateInstructionBuilder().
			SetPayer(userWallet).
			SetWallet(userWallet).
			SetMint(mint).
			ValidateAndBuild()
		if err != nil {
			return err
		}
		setupInstructions = append(setupInstructions, ix)
		return nil
	}

	if err := ensureATA(token0Mint, tokenAccount0, tokenProgram0); err != nil {
		return "", fmt.Errorf("ensure token0 ata failed: %w", err)
	}
	if err := ensureATA(token1Mint, tokenAccount1, tokenProgram1); err != nil {
		return "", fmt.Errorf("ensure token1 ata failed: %w", err)
	}

	// 获取最新 blockhash
	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("get blockhash failed: %w", err)
	}

	instructions := append(setupInstructions, openIx)
	tx, err := aSDK.NewTransaction(
		instructions,
		latest.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("create transaction failed: %w", err)
	}
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	// Position mint 需要作为签名者，使用 PartialSign 签名
	// 这样用户钱包只需要签名自己的部分，position mint 由后端签名
	signerMap := make(map[string]*aSDK.PrivateKey)
	keyCopy := make(aSDK.PrivateKey, len(positionMintKeypair))
	copy(keyCopy, positionMintKeypair)
	signerMap[positionMint.String()] = &keyCopy

	_, err = tx.PartialSign(func(key aSDK.PublicKey) *aSDK.PrivateKey {
		if pk, ok := signerMap[key.String()]; ok {
			return pk
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to partial sign position mint: %w", err)
	}

	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal transaction failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

type openPositionParams struct {
	UserWallet          aSDK.PublicKey
	PoolState           aSDK.PublicKey
	TokenVault0         aSDK.PublicKey
	TokenVault1         aSDK.PublicKey
	TokenMint0          aSDK.PublicKey
	TokenMint1          aSDK.PublicKey
	TokenProgram0       aSDK.PublicKey
	TokenProgram1       aSDK.PublicKey
	Amount0             uint64
	Amount1             uint64
	Amount0Max          uint64 // 应用滑点后的最大金额
	Amount1Max          uint64 // 应用滑点后的最大金额
	TickLower           int32
	TickUpper           int32
	TickArrayLowerStart int32
	TickArrayUpperStart int32
	TickSpacing         int64
	PositionMint        aSDK.PublicKey
}

func (l *OpenPositionLogic) buildOpenPositionInstruction(p openPositionParams) (aSDK.Instruction, error) {
	builder := amm_v3.NewOpenPositionWithToken22NftInstructionBuilder().
		SetTickLowerIndex(p.TickLower).
		SetTickUpperIndex(p.TickUpper).
		SetTickArrayLowerStartIndex(p.TickArrayLowerStart).
		SetTickArrayUpperStartIndex(p.TickArrayUpperStart).
		SetLiquidity(ag_binary.Uint128{}).
		SetAmount0Max(p.Amount0Max).
		SetAmount1Max(p.Amount1Max).
		SetWithMetadata(false).
		SetBaseFlag(true).
		SetPayerAccount(p.UserWallet).
		SetPositionNftOwnerAccount(p.UserWallet).
		SetPositionNftMintAccount(p.PositionMint)

		// 根据 position mint 和 token 2022 program 推导出 position nft 账户地址
	positionNftAccount, err := findTokenAccountForProgram(p.UserWallet, p.PositionMint, aSDK.Token2022ProgramID)
	if err != nil {
		return nil, fmt.Errorf("derive position nft account failed: %w", err)
	}
	builder.SetPositionNftAccountAccount(positionNftAccount)

	// 设置池子账户
	builder.SetPoolStateAccount(p.PoolState)
	// 根据 pool state 和 tick 范围推导出 protocol position PDA地址
	protocolPos, err := findProtocolPositionAddress(p.PoolState, p.TickLower, p.TickUpper)
	if err != nil {
		return nil, fmt.Errorf("derive protocol position failed: %w", err)
	}
	builder.SetProtocolPositionAccount(protocolPos)

	tickArrayLower, err := findTickArrayAddress(p.PoolState, p.TickArrayLowerStart)
	if err != nil {
		return nil, fmt.Errorf("derive tick array lower failed: %w", err)
	}
	tickArrayUpper, err := findTickArrayAddress(p.PoolState, p.TickArrayUpperStart)
	if err != nil {
		return nil, fmt.Errorf("derive tick array upper failed: %w", err)
	}
	builder.SetTickArrayLowerAccount(tickArrayLower).
		SetTickArrayUpperAccount(tickArrayUpper)

		// 根据 position mint 推导出 personal position PDA地址
	personalPos, err := findPersonalPositionPDA(p.PositionMint)
	if err != nil {
		return nil, fmt.Errorf("derive personal position failed: %w", err)
	}
	builder.SetPersonalPositionAccount(personalPos)

	tokenAccount0, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint0)
	if err != nil {
		return nil, fmt.Errorf("derive token account0 failed: %w", err)
	}
	tokenAccount1, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint1)
	if err != nil {
		return nil, fmt.Errorf("derive token account1 failed: %w", err)
	}

	builder.SetTokenAccount0Account(tokenAccount0).
		SetTokenAccount1Account(tokenAccount1).
		SetTokenVault0Account(p.TokenVault0).
		SetTokenVault1Account(p.TokenVault1).
		SetRentAccount(aSDK.SysVarRentPubkey).
		SetSystemProgramAccount(aSDK.SystemProgramID).
		SetAssociatedTokenProgramAccount(amm_v3.Addresses["ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"])

	tokenProgram := aSDK.TokenProgramID
	if !p.TokenProgram0.IsZero() {
		tokenProgram = p.TokenProgram0
	}
	tokenProgram2022 := aSDK.Token2022ProgramID

	builder.SetTokenProgramAccount(tokenProgram).
		SetTokenProgram2022Account(tokenProgram2022).
		SetVault0MintAccount(p.TokenMint0).
		SetVault1MintAccount(p.TokenMint1)

	instruction, err := builder.ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	return instruction, nil
}

func (l *OpenPositionLogic) calculateTickRange(in *trade.OpenPositionRequest, poolState amm_v3.PoolStateAccount, decimals0, decimals1, tickSpacing int64) (int32, int32, error) {
	// 如果没有提供价格范围，使用全范围
	priceMinStr := strings.TrimSpace(in.PriceMin)
	priceMaxStr := strings.TrimSpace(in.PriceMax)

	if priceMinStr == "" || priceMaxStr == "" {
		// 使用全范围
		return clmmMinTickIndex, clmmMaxTickIndex, nil
	}

	// 解析价格
	priceMinDec, err := decimal.NewFromString(priceMinStr)
	if err != nil {
		return 0, 0, fmt.Errorf("price_min invalid: %w", err)
	}
	priceMaxDec, err := decimal.NewFromString(priceMaxStr)
	if err != nil {
		return 0, 0, fmt.Errorf("price_max invalid: %w", err)
	}

	if priceMinDec.Cmp(decimal.Zero) <= 0 || priceMaxDec.Cmp(decimal.Zero) <= 0 {
		return 0, 0, errors.New("price_min and price_max must be greater than zero")
	}
	if priceMinDec.Cmp(priceMaxDec) >= 0 {
		return 0, 0, errors.New("price_min must be lower than price_max")
	}

	// 调整价格以考虑小数位数
	priceMinAdjusted := adjustPriceForDecimals(priceMinDec, decimals0, decimals1)
	priceMaxAdjusted := adjustPriceForDecimals(priceMaxDec, decimals0, decimals1)

	// 转换为 tick
	minTick, err := priceToTick(priceMinAdjusted)
	if err != nil {
		return 0, 0, fmt.Errorf("price_min to tick failed: %w", err)
	}
	maxTick, err := priceToTick(priceMaxAdjusted)
	if err != nil {
		return 0, 0, fmt.Errorf("price_max to tick failed: %w", err)
	}

	// 对齐到 tick spacing
	lower := alignTickToSpacing(minTick, tickSpacing, false)
	upper := alignTickToSpacing(maxTick, tickSpacing, true)

	// 确保在有效范围内
	if lower < clmmMinTickIndex {
		lower = clmmMinTickIndex
	}
	if upper > clmmMaxTickIndex {
		upper = clmmMaxTickIndex
	}
	if lower >= upper {
		return 0, 0, errors.New("invalid tick range after alignment")
	}

	return lower, upper, nil
}
