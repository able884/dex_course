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

type IncreaseClmmLiquidityLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewIncreaseClmmLiquidityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IncreaseClmmLiquidityLogic {
	return &IncreaseClmmLiquidityLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *IncreaseClmmLiquidityLogic) IncreaseClmmLiquidity(in *trade.IncreaseClmmLiquidityRequest) (*trade.IncreaseClmmLiquidityResponse, error) {
	if err := l.validate(in); err != nil {
		return nil, err
	}

	txBase64, err := l.buildIncreaseLiquidityTx(in)
	if err != nil {
		return nil, err
	}

	return &trade.IncreaseClmmLiquidityResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *IncreaseClmmLiquidityLogic) validate(in *trade.IncreaseClmmLiquidityRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(in.PositionNftMint) == "" {
		return errors.New("position_nft_mint required")
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

func (l *IncreaseClmmLiquidityLogic) buildIncreaseLiquidityTx(in *trade.IncreaseClmmLiquidityRequest) (string, error) {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("invalid user wallet address: %w", err)
	}

	positionNftMint, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.PositionNftMint))
	if err != nil {
		return "", fmt.Errorf("invalid position_nft_mint: %w", err)
	}

	poolStatePK, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.PoolState))
	if err != nil {
		return "", fmt.Errorf("invalid pool_state: %w", err)
	}

	// 1. 从数据库获取持仓信息
	position, err := l.svcCtx.ClmmPositionModel.FindOneByPositionNftMint(l.ctx, in.PositionNftMint)
	if err != nil {
		return "", fmt.Errorf("position not found: %w", err)
	}

	// 验证持仓属于该用户
	if position.UserWalletAddress != in.UserWalletAddress {
		return "", errors.New("position does not belong to user")
	}

	// 验证池子地址匹配
	if position.PoolState != in.PoolState {
		return "", errors.New("pool_state mismatch")
	}

	// 2. 从链上获取 PoolState 账户数据
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
	decimals0 := int64(poolState.MintDecimals0)
	decimals1 := int64(poolState.MintDecimals1)
	tickSpacing := int64(poolState.TickSpacing)

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

	// 3. 使用持仓的 tick 范围（不支持修改范围）
	tickLower := position.TickLowerIndex
	tickUpper := position.TickUpperIndex

	// 计算 tick array start indices
	tickArrayLowerStart := calcTickArrayStartIndex(tickLower, int32(tickSpacing))
	tickArrayUpperStart := calcTickArrayStartIndex(tickUpper, int32(tickSpacing))

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

	// 应用滑点
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

	// 4. 构建 IncreaseLiquidityV2 指令
	increaseIx, err := l.buildIncreaseLiquidityInstruction(increaseLiquidityParams{
		UserWallet:          userWallet,
		PositionNftMint:     positionNftMint,
		PoolState:           poolStatePK,
		TokenVault0:         tokenVault0,
		TokenVault1:         tokenVault1,
		TokenMint0:          token0Mint,
		TokenMint1:          token1Mint,
		TokenProgram0:       tokenProgram0,
		TokenProgram1:       tokenProgram1,
		Amount0Max:          amount0Max,
		Amount1Max:          amount1Max,
		TickLower:           tickLower,
		TickUpper:           tickUpper,
		TickArrayLowerStart: tickArrayLowerStart,
		TickArrayUpperStart: tickArrayUpperStart,
	})
	if err != nil {
		return "", fmt.Errorf("build increase liquidity instruction failed: %w", err)
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

	instructions := append(setupInstructions, increaseIx)
	tx, err := aSDK.NewTransaction(
		instructions,
		latest.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("create transaction failed: %w", err)
	}
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal transaction failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

type increaseLiquidityParams struct {
	UserWallet          aSDK.PublicKey
	PositionNftMint     aSDK.PublicKey
	PoolState           aSDK.PublicKey
	TokenVault0         aSDK.PublicKey
	TokenVault1         aSDK.PublicKey
	TokenMint0          aSDK.PublicKey
	TokenMint1          aSDK.PublicKey
	TokenProgram0       aSDK.PublicKey
	TokenProgram1       aSDK.PublicKey
	Amount0Max          uint64
	Amount1Max          uint64
	TickLower           int32
	TickUpper           int32
	TickArrayLowerStart int32
	TickArrayUpperStart int32
}

func (l *IncreaseClmmLiquidityLogic) buildIncreaseLiquidityInstruction(p increaseLiquidityParams) (aSDK.Instruction, error) {
	// 派生 NFT 账户
	positionNftAccount, err := findTokenAccountForProgram(p.UserWallet, p.PositionNftMint, aSDK.Token2022ProgramID)
	if err != nil {
		return nil, fmt.Errorf("derive position nft account failed: %w", err)
	}

	// 派生 PersonalPosition PDA
	personalPos, err := findPersonalPositionPDA(p.PositionNftMint)
	if err != nil {
		return nil, fmt.Errorf("derive personal position failed: %w", err)
	}

	// 派生 ProtocolPosition PDA
	protocolPos, err := findProtocolPositionAddress(p.PoolState, p.TickLower, p.TickUpper)
	if err != nil {
		return nil, fmt.Errorf("derive protocol position failed: %w", err)
	}

	// 派生 TickArray PDAs
	tickArrayLower, err := findTickArrayAddress(p.PoolState, p.TickArrayLowerStart)
	if err != nil {
		return nil, fmt.Errorf("derive tick array lower failed: %w", err)
	}
	tickArrayUpper, err := findTickArrayAddress(p.PoolState, p.TickArrayUpperStart)
	if err != nil {
		return nil, fmt.Errorf("derive tick array upper failed: %w", err)
	}

	// 派生用户 Token 账户
	tokenAccount0, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint0)
	if err != nil {
		return nil, fmt.Errorf("derive token account0 failed: %w", err)
	}
	tokenAccount1, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint1)
	if err != nil {
		return nil, fmt.Errorf("derive token account1 failed: %w", err)
	}

	// 构建 IncreaseLiquidityV2 指令
	builder := amm_v3.NewIncreaseLiquidityV2InstructionBuilder().
		SetLiquidity(ag_binary.Uint128{}). // liquidity=0 表示根据 amount0_max 或 amount1_max 计算
		SetAmount0Max(p.Amount0Max).
		SetAmount1Max(p.Amount1Max).
		SetBaseFlag(true). // true 表示基于 amount0_max 计算流动性
		SetNftOwnerAccount(p.UserWallet).
		SetNftAccountAccount(positionNftAccount).
		SetPoolStateAccount(p.PoolState).
		SetProtocolPositionAccount(protocolPos).
		SetPersonalPositionAccount(personalPos).
		SetTickArrayLowerAccount(tickArrayLower).
		SetTickArrayUpperAccount(tickArrayUpper).
		SetTokenAccount0Account(tokenAccount0).
		SetTokenAccount1Account(tokenAccount1).
		SetTokenVault0Account(p.TokenVault0).
		SetTokenVault1Account(p.TokenVault1).
		SetVault0MintAccount(p.TokenMint0).
		SetVault1MintAccount(p.TokenMint1)

	tokenProgram := aSDK.TokenProgramID
	if !p.TokenProgram0.IsZero() {
		tokenProgram = p.TokenProgram0
	}
	builder.SetTokenProgramAccount(tokenProgram).
		SetTokenProgram2022Account(aSDK.Token2022ProgramID)

	instruction, err := builder.ValidateAndBuild()
	if err != nil {
		return nil, fmt.Errorf("validate and build instruction failed: %w", err)
	}

	return instruction, nil
}

