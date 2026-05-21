package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	aSDK "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/cpmm"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// AddCpmmLiquidityLogic 处理 CPMM 池添加流动性业务逻辑
type AddCpmmLiquidityLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewAddCpmmLiquidityLogic 创建 CPMM 添加流动性逻辑处理器
func NewAddCpmmLiquidityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddCpmmLiquidityLogic {
	return &AddCpmmLiquidityLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// AddCpmmLiquidity 向 CPMM 池添加流动性
// 该方法负责：
// 1. 参数校验
// 2. 构建添加流动性交易
// 3. 返回未签名交易给客户端
func (l *AddCpmmLiquidityLogic) AddCpmmLiquidity(in *trade.AddCpmmLiquidityRequest) (*trade.AddCpmmLiquidityResponse, error) {
	// 1. 参数校验
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. 构建添加流动性交易
	txBase64, err := l.buildAddLiquidityTx(in)
	if err != nil {
		return nil, err
	}

	// 3. 返回结果
	return &trade.AddCpmmLiquidityResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(), // 交易10分钟后过期
	}, nil
}

// validate 校验添加流动性请求参数
func (l *AddCpmmLiquidityLogic) validate(in *trade.AddCpmmLiquidityRequest) error {
	if in == nil {
		return errors.New("请求不能为空")
	}
	if strings.TrimSpace(in.PoolState) == "" {
		return errors.New("pool_state 必填")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address 必填")
	}

	// 校验 token0 金额必须为正数
	token0Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token0Amount))
	if err != nil {
		return fmt.Errorf("token0_amount 无效: %w", err)
	}
	if token0Dec.Cmp(decimal.Zero) <= 0 {
		return errors.New("token0_amount 必须大于零")
	}

	// 校验 token1 金额必须为正数
	token1Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token1Amount))
	if err != nil {
		return fmt.Errorf("token1_amount 无效: %w", err)
	}
	if token1Dec.Cmp(decimal.Zero) <= 0 {
		return errors.New("token1_amount 必须大于零")
	}

	return nil
}

// buildAddLiquidityTx 构建添加流动性交易
// 该方法负责：
// 1. 获取 RPC 客户端和用户钱包地址
// 2. 查询池信息
// 3. 派生 PDA 地址
// 4. 获取链上实时余额计算 LP 数量
// 5. 校验代币比例
// 6. 构建交易指令
func (l *AddCpmmLiquidityLogic) buildAddLiquidityTx(in *trade.AddCpmmLiquidityRequest) (string, error) {
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

	// 6. 获取链上实时余额
	token0Balance, token0Decimals, err := getTokenAccountBalanceRaw(l.ctx, rpcClient, token0Vault)
	if err != nil {
		return "", fmt.Errorf("获取 token0 金库余额失败: %w", err)
	}
	token1Balance, token1Decimals, err := getTokenAccountBalanceRaw(l.ctx, rpcClient, token1Vault)
	if err != nil {
		return "", fmt.Errorf("获取 token1 金库余额失败: %w", err)
	}

	// 7. 获取 LP 代币总供应量
	lpSupply, _, err := getTokenSupplyRaw(l.ctx, rpcClient, lpMint)
	if err != nil {
		return "", fmt.Errorf("获取 LP 供应量失败: %w", err)
	}

	// 8. 解析输入金额
	token0Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token0Amount))
	if err != nil {
		return "", err
	}
	token1Dec, err := decimal.NewFromString(strings.TrimSpace(in.Token1Amount))
	if err != nil {
		return "", err
	}

	// 9. 转换为最小单位
	token0Multiplier := decimal.NewFromFloat(math.Pow10(int(token0Decimals)))
	token1Multiplier := decimal.NewFromFloat(math.Pow10(int(token1Decimals)))
	token0AmountRaw := token0Dec.Mul(token0Multiplier).IntPart()
	token1AmountRaw := token1Dec.Mul(token1Multiplier).IntPart()

	if token0AmountRaw <= 0 || token1AmountRaw <= 0 {
		return "", errors.New("代币金额必须大于零")
	}

	// 10. 设置容忍度（默认1%）
	toleranceBps := in.SlippageBps
	if toleranceBps == 0 {
		toleranceBps = 100
	}

	// 11. 校验输入代币比例是否与池储备比例一致
	if err := ensureRatioMatchesPool(token0Balance, token1Balance, token0AmountRaw, token1AmountRaw, toleranceBps); err != nil {
		return "", err
	}

	// 12. 按比例计算应得 LP 数量
	lpFrom0 := decimal.NewFromInt(token0AmountRaw).Mul(decimal.NewFromInt(lpSupply)).Div(decimal.NewFromInt(token0Balance))
	lpFrom1 := decimal.NewFromInt(token1AmountRaw).Mul(decimal.NewFromInt(lpSupply)).Div(decimal.NewFromInt(token1Balance))
	lpAmountRaw := decimal.Min(lpFrom0, lpFrom1).IntPart()

	if lpAmountRaw <= 0 {
		return "", errors.New("计算的 LP 数量为零，请检查池流动性")
	}

	// 13. 查找用户的 ATA 地址
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

	// 14. 确保 ATA 存在
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

	// 15. 构建存款指令
	lpAmountParam := uint64(lpAmountRaw)
	maxToken0 := uint64(token0AmountRaw)
	maxToken1 := uint64(token1AmountRaw)

	depositIx, err := raydium_cp_swap.NewDepositInstructionBuilder().
		SetLpTokenAmount(lpAmountParam).
		SetMaximumToken0Amount(maxToken0).
		SetMaximumToken1Amount(maxToken1).
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
		return "", fmt.Errorf("构建存款指令失败: %w", err)
	}

	// 16. 获取最新区块哈希
	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("获取区块哈希失败: %w", err)
	}

	// 17. 创建交易
	instructions := append(setupInstructions, depositIx)
	tx, err := aSDK.NewTransaction(
		instructions,
		latest.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("创建交易失败: %w", err)
	}

	// 18. 初始化签名槽位
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	// 19. 序列化为 base64
	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("序列化交易失败: %w", err)
	}

	return base64.StdEncoding.EncodeToString(raw), nil
}

// ensureRatioMatchesPool 校验用户输入的代币金额是否与池子当前储备比例一致
// 参数：
//   - poolToken0: 池中的 token0 余额
//   - poolToken1: 池中的 token1 余额
//   - reqToken0: 用户输入的 token0 金额
//   - reqToken1: 用户输入的 token1 金额
//   - toleranceBps: 容忍度（bps）
func ensureRatioMatchesPool(poolToken0, poolToken1, reqToken0, reqToken1 int64, toleranceBps uint32) error {
	if poolToken0 <= 0 || poolToken1 <= 0 {
		return nil
	}
	if reqToken0 <= 0 || reqToken1 <= 0 {
		return errors.New("请求的代币金额必须为正数")
	}

	// 计算期望比例和实际比例
	expectedRatio := decimal.NewFromInt(poolToken1).Div(decimal.NewFromInt(poolToken0))
	actualRatio := decimal.NewFromInt(reqToken1).Div(decimal.NewFromInt(reqToken0))

	// 计算比例差异和允许的误差范围
	diff := actualRatio.Sub(expectedRatio).Abs()
	allowed := expectedRatio.Mul(decimal.NewFromInt(int64(toleranceBps))).Div(decimal.NewFromInt(10000))

	// 防止允许误差为零
	if allowed.LessThan(decimal.NewFromBigInt(big.NewInt(1), -12)) {
		allowed = decimal.NewFromBigInt(big.NewInt(1), -12)
	}

	// 校验比例是否在允许范围内
	if diff.GreaterThan(allowed) {
		return fmt.Errorf("代币金额比例不匹配: 期望约 %s, 实际 %s (容忍度 %d bps)",
			expectedRatio.StringFixed(8), actualRatio.StringFixed(8), toleranceBps)
	}

	return nil
}

// accountExists 检查账户是否存在
func accountExists(ctx context.Context, client *ag_rpc.Client, account aSDK.PublicKey) (bool, error) {
	resp, err := client.GetAccountInfoWithOpts(ctx, account, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentProcessed,
	})
	if err != nil {
		if errors.Is(err, ag_rpc.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if resp == nil || resp.Value == nil {
		return false, nil
	}
	return true, nil
}

// getTokenAccountBalanceRaw 获取代币账户余额（原始值）
func getTokenAccountBalanceRaw(ctx context.Context, client *ag_rpc.Client, account aSDK.PublicKey) (int64, int64, error) {
	resp, err := client.GetTokenAccountBalance(ctx, account, ag_rpc.CommitmentFinalized)
	if err != nil {
		return 0, 0, err
	}
	if resp == nil || resp.Value == nil {
		return 0, 0, errors.New("余额响应为空")
	}
	if resp.Value.Amount == "" {
		return 0, 0, errors.New("金额为空")
	}

	amountDec, err := decimal.NewFromString(resp.Value.Amount)
	if err != nil {
		return 0, 0, err
	}

	return amountDec.IntPart(), int64(resp.Value.Decimals), nil
}

// getTokenSupplyRaw 获取代币供应量（原始值）
func getTokenSupplyRaw(ctx context.Context, client *ag_rpc.Client, mint aSDK.PublicKey) (int64, int64, error) {
	resp, err := client.GetTokenSupply(ctx, mint, ag_rpc.CommitmentFinalized)
	if err != nil {
		return 0, 0, err
	}
	if resp == nil || resp.Value == nil {
		return 0, 0, errors.New("供应量响应为空")
	}

	amountDec, err := decimal.NewFromString(resp.Value.Amount)
	if err != nil {
		return 0, 0, err
	}

	return amountDec.IntPart(), int64(resp.Value.Decimals), nil
}