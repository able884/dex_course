package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	ag_binary "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type DecreaseClmmLiquidityLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDecreaseClmmLiquidityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DecreaseClmmLiquidityLogic {
	return &DecreaseClmmLiquidityLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DecreaseClmmLiquidityLogic) DecreaseClmmLiquidity(in *trade.DecreaseClmmLiquidityRequest) (*trade.DecreaseClmmLiquidityResponse, error) {
	if err := l.validate(in); err != nil {
		return nil, err
	}

	txBase64, err := l.buildDecreaseLiquidityTx(in)
	if err != nil {
		return nil, err
	}

	return &trade.DecreaseClmmLiquidityResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *DecreaseClmmLiquidityLogic) validate(in *trade.DecreaseClmmLiquidityRequest) error {
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
	// 至少需要提供 liquidity 或 amount0_min/amount1_min 之一
	if strings.TrimSpace(in.Liquidity) == "" && strings.TrimSpace(in.Amount0Min) == "" && strings.TrimSpace(in.Amount1Min) == "" {
		return errors.New("at least one of liquidity, amount0_min, or amount1_min is required")
	}
	return nil
}

func (l *DecreaseClmmLiquidityLogic) buildDecreaseLiquidityTx(in *trade.DecreaseClmmLiquidityRequest) (string, error) {
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

	// 使用持仓的 tick 范围
	tickLower := position.TickLowerIndex
	tickUpper := position.TickUpperIndex

	// 计算 tick array start indices
	tickArrayLowerStart := calcTickArrayStartIndex(tickLower, int32(tickSpacing))
	tickArrayUpperStart := calcTickArrayStartIndex(tickUpper, int32(tickSpacing))

	// 3. 解析 liquidity 或 amount0_min/amount1_min
	var liquidity ag_binary.Uint128
	var amount0Min, amount1Min uint64

	if strings.TrimSpace(in.Liquidity) != "" {
		// 如果提供了 liquidity，直接使用
		liquidityStr := strings.TrimSpace(in.Liquidity)
		// 解析 Uint128 字符串（格式可能是 "123456" 或 "123456:0"）
		liqBig, ok := new(big.Int).SetString(liquidityStr, 10)
		if !ok {
			return "", fmt.Errorf("invalid liquidity format: %s", liquidityStr)
		}
		// 转换为 Uint128
		liqBytes := liqBig.Bytes()
		if len(liqBytes) > 16 {
			return "", errors.New("liquidity exceeds Uint128")
		}
		// 填充到 16 字节（小端序）
		liqBytesPadded := make([]byte, 16)
		copy(liqBytesPadded, liqBytes)
		liquidity = ag_binary.Uint128{
			Lo: uint64(liqBytesPadded[0]) | uint64(liqBytesPadded[1])<<8 | uint64(liqBytesPadded[2])<<16 | uint64(liqBytesPadded[3])<<24 |
				uint64(liqBytesPadded[4])<<32 | uint64(liqBytesPadded[5])<<40 | uint64(liqBytesPadded[6])<<48 | uint64(liqBytesPadded[7])<<56,
			Hi: uint64(liqBytesPadded[8]) | uint64(liqBytesPadded[9])<<8 | uint64(liqBytesPadded[10])<<16 | uint64(liqBytesPadded[11])<<24 |
				uint64(liqBytesPadded[12])<<32 | uint64(liqBytesPadded[13])<<40 | uint64(liqBytesPadded[14])<<48 | uint64(liqBytesPadded[15])<<56,
		}
	}

	// 解析 amount0_min 和 amount1_min（用于滑点保护）
	if strings.TrimSpace(in.Amount0Min) != "" {
		amount0MinDec, err := decimal.NewFromString(strings.TrimSpace(in.Amount0Min))
		if err != nil {
			return "", fmt.Errorf("amount0_min invalid: %w", err)
		}
		amount0Min, err = toAtomicAmount(amount0MinDec, decimals0)
		if err != nil {
			return "", fmt.Errorf("amount0_min conversion failed: %w", err)
		}
	}
	if strings.TrimSpace(in.Amount1Min) != "" {
		amount1MinDec, err := decimal.NewFromString(strings.TrimSpace(in.Amount1Min))
		if err != nil {
			return "", fmt.Errorf("amount1_min invalid: %w", err)
		}
		amount1Min, err = toAtomicAmount(amount1MinDec, decimals1)
		if err != nil {
			return "", fmt.Errorf("amount1_min conversion failed: %w", err)
		}
	}

	// 如果未提供 liquidity，且未提供 amount0_min/amount1_min，返回错误
	if liquidity.Lo == 0 && liquidity.Hi == 0 && amount0Min == 0 && amount1Min == 0 {
		return "", errors.New("must provide either liquidity or amount0_min/amount1_min")
	}

	// 4. 构建 DecreaseLiquidityV2 指令
	decreaseIx, err := l.buildDecreaseLiquidityInstruction(decreaseLiquidityParams{
		UserWallet:          userWallet,
		PositionNftMint:     positionNftMint,
		PoolState:           poolStatePK,
		TokenVault0:         tokenVault0,
		TokenVault1:         tokenVault1,
		TokenMint0:          token0Mint,
		TokenMint1:          token1Mint,
		Liquidity:            liquidity,
		Amount0Min:          amount0Min,
		Amount1Min:          amount1Min,
		TickLower:           tickLower,
		TickUpper:           tickUpper,
		TickArrayLowerStart: tickArrayLowerStart,
		TickArrayUpperStart: tickArrayUpperStart,
	})
	if err != nil {
		return "", fmt.Errorf("build decrease liquidity instruction failed: %w", err)
	}

	// 获取最新 blockhash
	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("get blockhash failed: %w", err)
	}

	instructions := []aSDK.Instruction{decreaseIx}
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

type decreaseLiquidityParams struct {
	UserWallet          aSDK.PublicKey
	PositionNftMint     aSDK.PublicKey
	PoolState           aSDK.PublicKey
	TokenVault0         aSDK.PublicKey
	TokenVault1         aSDK.PublicKey
	TokenMint0          aSDK.PublicKey
	TokenMint1          aSDK.PublicKey
	Liquidity           ag_binary.Uint128
	Amount0Min          uint64
	Amount1Min          uint64
	TickLower           int32
	TickUpper           int32
	TickArrayLowerStart int32
	TickArrayUpperStart int32
}

func (l *DecreaseClmmLiquidityLogic) buildDecreaseLiquidityInstruction(p decreaseLiquidityParams) (aSDK.Instruction, error) {
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

	// 派生用户 Token 账户（接收代币的账户）
	recipientTokenAccount0, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint0)
	if err != nil {
		return nil, fmt.Errorf("derive recipient token account0 failed: %w", err)
	}
	recipientTokenAccount1, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint1)
	if err != nil {
		return nil, fmt.Errorf("derive recipient token account1 failed: %w", err)
	}

	// 构建 DecreaseLiquidityV2 指令
	builder := amm_v3.NewDecreaseLiquidityV2InstructionBuilder().
		SetLiquidity(p.Liquidity).
		SetAmount0Min(p.Amount0Min).
		SetAmount1Min(p.Amount1Min).
		SetNftOwnerAccount(p.UserWallet).
		SetNftAccountAccount(positionNftAccount).
		SetPersonalPositionAccount(personalPos).
		SetPoolStateAccount(p.PoolState).
		SetProtocolPositionAccount(protocolPos).
		SetTokenVault0Account(p.TokenVault0).
		SetTokenVault1Account(p.TokenVault1).
		SetTickArrayLowerAccount(tickArrayLower).
		SetTickArrayUpperAccount(tickArrayUpper).
		SetRecipientTokenAccount0Account(recipientTokenAccount0).
		SetRecipientTokenAccount1Account(recipientTokenAccount1).
		SetVault0MintAccount(p.TokenMint0).
		SetVault1MintAccount(p.TokenMint1)

	instruction, err := builder.ValidateAndBuild()
	if err != nil {
		return nil, fmt.Errorf("validate and build instruction failed: %w", err)
	}

	return instruction, nil
}

