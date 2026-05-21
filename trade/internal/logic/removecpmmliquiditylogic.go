package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/cpmm"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// RemoveCpmmLiquidityLogic 处理 CPMM 池移除流动性业务逻辑
type RemoveCpmmLiquidityLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewRemoveCpmmLiquidityLogic 创建 CPMM 移除流动性逻辑处理器
func NewRemoveCpmmLiquidityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveCpmmLiquidityLogic {
	return &RemoveCpmmLiquidityLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// RemoveCpmmLiquidity 从 CPMM 池移除流动性
// 该方法负责：
// 1. 参数校验
// 2. 构建移除流动性交易
// 3. 返回未签名交易给客户端
func (l *RemoveCpmmLiquidityLogic) RemoveCpmmLiquidity(in *trade.RemoveCpmmLiquidityRequest) (*trade.RemoveCpmmLiquidityResponse, error) {
	// 1. 参数校验
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. 构建移除流动性交易
	txBase64, err := l.buildRemoveLiquidityTx(in)
	if err != nil {
		return nil, err
	}

	// 3. 返回结果
	return &trade.RemoveCpmmLiquidityResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(), // 交易10分钟后过期
	}, nil
}

// validate 校验移除流动性请求参数
func (l *RemoveCpmmLiquidityLogic) validate(in *trade.RemoveCpmmLiquidityRequest) error {
	if in == nil {
		return errors.New("请求不能为空")
	}
	if strings.TrimSpace(in.PoolState) == "" {
		return errors.New("pool_state 必填")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address 必填")
	}

	// 校验 LP 金额
	if _, err := decimal.NewFromString(strings.TrimSpace(in.LpAmount)); err != nil {
		return fmt.Errorf("lp_amount 无效: %w", err)
	}

	// 校验最小输出金额（如果提供）
	if in.MinToken0Amount != "" {
		if _, err := decimal.NewFromString(strings.TrimSpace(in.MinToken0Amount)); err != nil {
			return fmt.Errorf("min_token0_amount 无效: %w", err)
		}
	}
	if in.MinToken1Amount != "" {
		if _, err := decimal.NewFromString(strings.TrimSpace(in.MinToken1Amount)); err != nil {
			return fmt.Errorf("min_token1_amount 无效: %w", err)
		}
	}

	return nil
}

// buildRemoveLiquidityTx 构建移除流动性交易
// 该方法负责：
// 1. 获取 RPC 客户端和用户钱包地址
// 2. 查询池信息
// 3. 派生 PDA 地址
// 4. 转换金额为最小单位
// 5. 构建交易指令
func (l *RemoveCpmmLiquidityLogic) buildRemoveLiquidityTx(in *trade.RemoveCpmmLiquidityRequest) (string, error) {
	// 1. 校验 RPC 客户端
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("Solana RPC 客户端未配置")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	// 2. 解析用户钱包地址
	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("无效的用户钱包地址: %w", err)
	}

	// 3. 查询池信息
	poolModel := solmodel.NewCpmmPoolInfoModel(l.svcCtx.DB)
	pool, err := poolModel.FindOneByPoolState(l.ctx, strings.TrimSpace(in.PoolState))
	if err != nil || pool == nil {
		return "", fmt.Errorf("找不到池状态 %s: %w", in.PoolState, err)
	}

	// 4. 解析代币地址和配置
	token0Mint := aSDK.MustPublicKeyFromBase58(pool.InputTokenMint)
	token1Mint := aSDK.MustPublicKeyFromBase58(pool.OutputTokenMint)
	ammConfig := aSDK.MustPublicKeyFromBase58(pool.AmmConfig)
	programID := cpmm.ProgramRaydiumCPMMProgram
	if in.ChainId == constants.SolChainIdInt {
		programID = cpmm.ProgramRaydiumCPMMProgramDevNet
	}
	programPk := aSDK.MustPublicKeyFromBase58(programID.String())
	cpmm.SetProgramID(programPk)

	// 5. 派生池 PDA 地址
	_, lpMint, authority, token0Vault, token1Vault, _, err := cpmm.DeriveCpmmPoolPDAs(
		ammConfig,
		token0Mint,
		token1Mint,
		programPk,
	)
	if err != nil {
		return "", fmt.Errorf("派生 PDA 失败: %w", err)
	}

	// 6. 获取代币精度
	lpDecimals, err := l.getTokenDecimals(lpMint.String())
	if err != nil {
		return "", fmt.Errorf("获取 LP 精度失败: %w", err)
	}
	token0Decimals, err := l.getTokenDecimals(token0Mint.String())
	if err != nil {
		return "", fmt.Errorf("获取 token0 精度失败: %w", err)
	}
	token1Decimals, err := l.getTokenDecimals(token1Mint.String())
	if err != nil {
		return "", fmt.Errorf("获取 token1 精度失败: %w", err)
	}

	// 7. 设置乘数
	lpMultiplier := decimal.NewFromFloat(math.Pow10(int(lpDecimals)))
	token0Multiplier := decimal.NewFromFloat(math.Pow10(int(token0Decimals)))
	token1Multiplier := decimal.NewFromFloat(math.Pow10(int(token1Decimals)))

	// 8. 转换 LP 金额为最小单位
	lpAmountDec, _ := decimal.NewFromString(strings.TrimSpace(in.LpAmount))
	lpAmountRaw := lpAmountDec.Mul(lpMultiplier).IntPart()
	if lpAmountRaw <= 0 {
		return "", errors.New("lp_amount 必须大于零")
	}

	// 9. 转换最小输出金额（可选）
	minToken0Raw := int64(0)
	minToken1Raw := int64(0)
	if in.MinToken0Amount != "" {
		if dec, err := decimal.NewFromString(strings.TrimSpace(in.MinToken0Amount)); err == nil {
			minToken0Raw = dec.Mul(token0Multiplier).IntPart()
		}
	}
	if in.MinToken1Amount != "" {
		if dec, err := decimal.NewFromString(strings.TrimSpace(in.MinToken1Amount)); err == nil {
			minToken1Raw = dec.Mul(token1Multiplier).IntPart()
		}
	}

	// 10. 查找用户的 ATA 地址
	ownerToken0, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token0Mint)
	if err != nil {
		return "", err
	}
	ownerToken1, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token1Mint)
	if err != nil {
		return "", err
	}
	ownerLpToken, _, err := aSDK.FindAssociatedTokenAddress(userWallet, lpMint)
	if err != nil {
		return "", err
	}

	// 11. 确保 ATA 存在
	var setupInstructions []aSDK.Instruction
	ensureATA := func(mint aSDK.PublicKey, ata aSDK.PublicKey) error {
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

	if err := ensureATA(token0Mint, ownerToken0); err != nil {
		return "", fmt.Errorf("确保 token0 ATA 失败: %w", err)
	}
	if err := ensureATA(token1Mint, ownerToken1); err != nil {
		return "", fmt.Errorf("确保 token1 ATA 失败: %w", err)
	}
	if err := ensureATA(lpMint, ownerLpToken); err != nil {
		return "", fmt.Errorf("确保 LP ATA 失败: %w", err)
	}

	// 12. 构建取款指令
	withdrawIx, err := raydium_cp_swap.NewWithdrawInstructionBuilder().
		SetLpTokenAmount(uint64(lpAmountRaw)).
		SetMinimumToken0Amount(uint64(minToken0Raw)).
		SetMinimumToken1Amount(uint64(minToken1Raw)).
		SetOwnerAccount(userWallet).
		SetAuthorityAccount(authority).
		SetPoolStateAccount(aSDK.MustPublicKeyFromBase58(pool.PoolState)).
		SetOwnerLpTokenAccount(ownerLpToken).
		SetToken0AccountAccount(ownerToken0).
		SetToken1AccountAccount(ownerToken1).
		SetToken0VaultAccount(token0Vault).
		SetToken1VaultAccount(token1Vault).
		SetTokenProgramAccount(aSDK.TokenProgramID).
		SetTokenProgram2022Account(aSDK.Token2022ProgramID).
		SetVault0MintAccount(token0Mint).
		SetVault1MintAccount(token1Mint).
		SetLpMintAccount(lpMint).
		ValidateAndBuild()
	if err != nil {
		return "", fmt.Errorf("构建取款指令失败: %w", err)
	}

	// 13. 获取最新区块哈希
	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("获取区块哈希失败: %w", err)
	}

	// 14. 创建交易
	instructions := append(setupInstructions, withdrawIx)
	tx, err := aSDK.NewTransaction(
		instructions,
		latest.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("创建交易失败: %w", err)
	}

	// 15. 初始化签名槽位
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	// 16. 序列化为 base64
	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("序列化交易失败: %w", err)
	}

	return base64.StdEncoding.EncodeToString(raw), nil
}

// getTokenDecimals 获取代币精度
func (l *RemoveCpmmLiquidityLogic) getTokenDecimals(tokenMint string) (int64, error) {
	if l.svcCtx.MarketTokenClient == nil {
		return 0, errors.New("市场代币客户端不可用")
	}

	resp, err := l.svcCtx.MarketTokenClient.GetTokenInfo(l.ctx, &market.GetTokenInfoRequest{
		ChainId:      int64(l.svcCtx.Config.SolConfig.ChainId),
		TokenAddress: tokenMint,
	})
	if err != nil {
		return 0, fmt.Errorf("获取代币信息失败 %s: %w", tokenMint, err)
	}
	if resp == nil {
		return 0, fmt.Errorf("代币信息未找到 %s", tokenMint)
	}

	return resp.Decimals, nil
}