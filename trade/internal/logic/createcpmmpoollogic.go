package logic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/trademodel"
	"richcode.cc/dex/pkg/raydium/cpmm"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"

	aSDK "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

// CreateCpmmPoolLogic 处理 CPMM 池创建业务逻辑
type CreateCpmmPoolLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewCreateCpmmPoolLogic 创建 CPMM 池创建逻辑处理器
func NewCreateCpmmPoolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCpmmPoolLogic {
	return &CreateCpmmPoolLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateCpmmPool 创建 CPMM 池
// 该方法负责：
// 1. 参数校验
// 2. 校验配置索引是否存在
// 3. 构建 Raydium CPMM 创建池子的未签名交易
// 4. 返回未签名交易给客户端
func (l *CreateCpmmPoolLogic) CreateCpmmPool(in *trade.CreateCpmmPoolRequest) (*trade.CreateCpmmPoolResponse, error) {
	// 1. 参数校验
	if err := l.validateRequest(in); err != nil {
		return nil, err
	}

	// 2. 校验配置索引是否存在
	feeModel := trademodel.NewCpmmFeeTierModel(l.svcCtx.DB)
	if err := l.ensureConfigIndexExists(feeModel, in.ConfigIndex); err != nil {
		return nil, err
	}

	// 3. 构建创建池交易
	txBase64, err := l.buildCreateCpmmPoolTx(in)
	if err != nil {
		return nil, fmt.Errorf("构建创建池交易失败: %w", err)
	}

	// 4. 返回结果
	return &trade.CreateCpmmPoolResponse{
		TxHash:    "",                                      // 交易哈希在前端签名并发送后才会有
		TxType:    "versioned",                             // Solana v0 交易类型
		TxBase64:  txBase64,                                // base64 编码的未签名交易
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(), // 交易过期时间（blockhash 有效期）
	}, nil
}

// validateRequest 校验创建池请求参数
func (l *CreateCpmmPoolLogic) validateRequest(in *trade.CreateCpmmPoolRequest) error {
	if in == nil {
		return errors.New("请求不能为空")
	}
	if strings.TrimSpace(in.BaseTokenMint) == "" || strings.TrimSpace(in.QuoteTokenMint) == "" {
		return errors.New("代币 mint 必填")
	}
	if strings.EqualFold(in.BaseTokenMint, in.QuoteTokenMint) {
		return errors.New("基础代币和报价代币必须不同")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address 必填")
	}

	// 校验金额必须为正数
	if err := requirePositiveDecimal(in.BaseAmount); err != nil {
		return fmt.Errorf("base_amount 无效: %w", err)
	}
	if err := requirePositiveDecimal(in.QuoteAmount); err != nil {
		return fmt.Errorf("quote_amount 无效: %w", err)
	}
	if err := requirePositiveDecimal(in.InitialPrice); err != nil {
		return fmt.Errorf("initial_price 无效: %w", err)
	}
	if in.ConfigIndex < 0 {
		return errors.New("config_index 必须非负")
	}

	return nil
}

// requirePositiveDecimal 校验字符串是否为正数
func requirePositiveDecimal(val string) error {
	dec, err := decimal.NewFromString(strings.TrimSpace(val))
	if err != nil {
		return err
	}
	if dec.Cmp(decimal.Zero) <= 0 {
		return errors.New("值必须大于零")
	}
	return nil
}

// buildCreateCpmmPoolTx 构建 Raydium CPMM 创建池子的未签名交易
// 该函数会：
// 1. 验证并解析所有输入参数
// 2. 派生所有必需的 PDA 账户地址
// 3. 构建 Initialize 指令
// 4. 创建未签名交易并序列化为 base64
// 5. 返回 base64 编码的交易给前端进行签名
func (l *CreateCpmmPoolLogic) buildCreateCpmmPoolTx(in *trade.CreateCpmmPoolRequest) (string, error) {
	// 1. 解析用户钱包地址
	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("无效的用户钱包地址: %w", err)
	}

	// 2. 解析代币 mint 地址
	baseTokenMint, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.BaseTokenMint))
	if err != nil {
		return "", fmt.Errorf("无效的基础代币 mint: %w", err)
	}

	quoteTokenMint, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.QuoteTokenMint))
	if err != nil {
		return "", fmt.Errorf("无效的报价代币 mint: %w", err)
	}

	// 3. 确保 token0 < token1（Raydium 要求）
	token0Mint := baseTokenMint
	token1Mint := quoteTokenMint
	baseAmount := in.BaseAmount
	quoteAmount := in.QuoteAmount

	if baseTokenMint.String() > quoteTokenMint.String() {
		token0Mint = quoteTokenMint
		token1Mint = baseTokenMint
		baseAmount = in.QuoteAmount
		quoteAmount = in.BaseAmount
	}

	// 4. 获取代币精度
	token0Decimals, err := l.getTokenDecimals(token0Mint.String())
	if err != nil {
		return "", fmt.Errorf("获取 token0 精度失败: %w", err)
	}

	token1Decimals, err := l.getTokenDecimals(token1Mint.String())
	if err != nil {
		return "", fmt.Errorf("获取 token1 精度失败: %w", err)
	}

	l.Infof("代币精度 - Token0: %d, Token1: %d", token0Decimals, token1Decimals)

	// 5. 解析金额并转换为最小单位
	token0AmountDec, err := decimal.NewFromString(strings.TrimSpace(baseAmount))
	if err != nil {
		return "", fmt.Errorf("无效的 token0 金额: %w", err)
	}

	token1AmountDec, err := decimal.NewFromString(strings.TrimSpace(quoteAmount))
	if err != nil {
		return "", fmt.Errorf("无效的 token1 金额: %w", err)
	}

	// 转换为最小单位
	token0Multiplier := decimal.NewFromFloat(math.Pow10(int(token0Decimals)))
	token1Multiplier := decimal.NewFromFloat(math.Pow10(int(token1Decimals)))

	token0Amount := uint64(token0AmountDec.Mul(token0Multiplier).IntPart())
	token1Amount := uint64(token1AmountDec.Mul(token1Multiplier).IntPart())

	// 6. 获取 CPMM 程序 ID
	cpmmProgramID := cpmm.ProgramRaydiumCPMMProgramDevNet
	programID, err := aSDK.PublicKeyFromBase58(cpmmProgramID.String())
	if err != nil {
		return "", fmt.Errorf("无效的 CPMM 程序 ID: %w", err)
	}
	cpmm.SetProgramID(programID)

	// 7. 获取 AMM Config 地址
	ammConfig, err := l.getAmmConfigAddress(in.ConfigIndex, programID)
	if err != nil {
		return "", fmt.Errorf("获取 AMM 配置地址失败: %w", err)
	}

	l.Infof("AMM 配置地址: %s (配置索引 %d)", ammConfig.String(), in.ConfigIndex)

	// 8. 校验 RPC 客户端
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("Solana RPC 客户端未配置")
	}
	solanaClient := l.svcCtx.SolTxMananger.Client

	// 9. 派生所有 PDA 账户
	poolState, lpMint, authority, token0Vault, token1Vault, observationState, err := cpmm.DeriveCpmmPoolPDAs(
		ammConfig,
		token0Mint,
		token1Mint,
		programID,
	)
	if err != nil {
		return "", fmt.Errorf("派生 PDA 失败: %w", err)
	}

	// 10. 获取或创建用户的代币账户 (ATA)
	creatorToken0, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token0Mint)
	if err != nil {
		return "", fmt.Errorf("查找创建者 token0 ATA 失败: %w", err)
	}

	creatorToken1, _, err := aSDK.FindAssociatedTokenAddress(userWallet, token1Mint)
	if err != nil {
		return "", fmt.Errorf("查找创建者 token1 ATA 失败: %w", err)
	}

	creatorLpToken, _, err := aSDK.FindAssociatedTokenAddress(userWallet, lpMint)
	if err != nil {
		return "", fmt.Errorf("查找创建者 LP 代币 ATA 失败: %w", err)
	}

	// 11. 确保 ATA 存在
	var setupInstructions []aSDK.Instruction
	ensureTokenAccount := func(wallet, mint, ata aSDK.PublicKey) error {
		exists, err := l.accountExists(solanaClient, ata)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		createIx, err := associatedtokenaccount.NewCreateInstructionBuilder().
			SetPayer(wallet).
			SetWallet(wallet).
			SetMint(mint).
			ValidateAndBuild()
		if err != nil {
			return fmt.Errorf("构建 ATA 创建指令失败: %w", err)
		}
		setupInstructions = append(setupInstructions, createIx)
		return nil
	}

	if err := ensureTokenAccount(userWallet, token0Mint, creatorToken0); err != nil {
		return "", fmt.Errorf("确保 token0 ATA 失败: %w", err)
	}
	if err := ensureTokenAccount(userWallet, token1Mint, creatorToken1); err != nil {
		return "", fmt.Errorf("确保 token1 ATA 失败: %w", err)
	}

	// 12. 创建池子费用接收账户 (Raydium CPMM 合约固定地址)
	createPoolFeeReceiver := aSDK.MustPublicKeyFromBase58("3oE58BKVt8KuYkGxx8zBojugnymWmBiyafWgMrnb6eYy")

	// 13. 设置开始时间
	openTime := uint64(in.StartTime)
	if openTime == 0 {
		openTime = uint64(time.Now().Unix())
	}

	// 14. 构建 Initialize 指令参数
	initParams := &cpmm.InitializePoolPara{
		InitAmount0:            token0Amount,
		InitAmount1:            token1Amount,
		OpenTime:               openTime,
		Creator:                userWallet,
		AmmConfig:              ammConfig,
		Authority:              authority,
		PoolState:              poolState,
		Token0Mint:             token0Mint,
		Token1Mint:             token1Mint,
		LpMint:                 lpMint,
		CreatorToken0:          creatorToken0,
		CreatorToken1:          creatorToken1,
		CreatorLpToken:         creatorLpToken,
		Token0Vault:            token0Vault,
		Token1Vault:            token1Vault,
		CreatePoolFee:          createPoolFeeReceiver,
		ObservationState:       observationState,
		TokenProgram:           aSDK.TokenProgramID,
		Token0Program:          aSDK.TokenProgramID,
		Token1Program:          aSDK.TokenProgramID,
		AssociatedTokenProgram: aSDK.SPLAssociatedTokenAccountProgramID,
		SystemProgram:          aSDK.SystemProgramID,
		Rent:                   aSDK.SysVarRentPubkey,
	}

	// 15. 创建 Initialize 指令
	initInstruction, err := cpmm.NewInitializeInstruction(initParams)
	if err != nil {
		return "", fmt.Errorf("创建 Initialize 指令失败: %w", err)
	}

	// 16. 获取最新区块哈希
	recentBlockhash, err := solanaClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("获取最新区块哈希失败: %w", err)
	}

	// 17. 构建未签名交易
	instructions := append(setupInstructions, initInstruction)
	tx, err := aSDK.NewTransaction(
		instructions,
		recentBlockhash.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("创建交易失败: %w", err)
	}

	// 18. 初始化空签名槽位（前端会填充签名）
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]aSDK.Signature, numSigners)

	// 19. 序列化未签名交易为 base64
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("序列化交易失败: %w", err)
	}

	txBase64 := base64.StdEncoding.EncodeToString(txBytes)

	// 记录日志
	l.Infof("创建 CPMM 池初始化交易")
	l.Infof(" 池状态: %s", poolState.String())
	l.Infof(" Token0 Mint: %s", token0Mint.String())
	l.Infof(" Token1 Mint: %s", token1Mint.String())
	l.Infof(" LP Mint: %s", lpMint.String())
	l.Infof(" Token0 金额: %d", token0Amount)
	l.Infof(" Token1 金额: %d", token1Amount)

	return txBase64, nil
}

// accountExists 调用 RPC 接口判断账户是否存在
func (l *CreateCpmmPoolLogic) accountExists(client *ag_rpc.Client, account aSDK.PublicKey) (bool, error) {
	resp, err := client.GetAccountInfoWithOpts(l.ctx, account, &ag_rpc.GetAccountInfoOpts{
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

// getAmmConfigAddress 根据合约地址和 config_index 获取对应的 AMM Config 固定地址
func (l *CreateCpmmPoolLogic) getAmmConfigAddress(configIndex int32, programID aSDK.PublicKey) (aSDK.PublicKey, error) {
	programAddress := programID.String()

	// 从数据库查询 fee tier 配置
	var tier trademodel.CpmmFeeTier
	err := l.svcCtx.DB.WithContext(l.ctx).
		Where("program_address = ? AND config_index = ?", programAddress, configIndex).
		Take(&tier).Error
	if err != nil {
		return aSDK.PublicKey{}, fmt.Errorf("找不到程序 %s 和索引 %d 的 AMM 配置: %w", programAddress, configIndex, err)
	}

	// 检查地址字段是否为空
	if tier.Address == "" {
		return aSDK.PublicKey{}, fmt.Errorf("程序 %s, 索引 %d 的 AMM 配置地址为空", programAddress, configIndex)
	}

	ammConfig, err := aSDK.PublicKeyFromBase58(tier.Address)
	if err != nil {
		return aSDK.PublicKey{}, fmt.Errorf("解析 AMM 配置地址 %s 失败: %w", tier.Address, err)
	}

	l.Infof("AMM 配置: program=%s, config_index=%d, fee_tier_bps=%d, address=%s",
		programAddress, configIndex, tier.ValueBps, ammConfig.String())

	return ammConfig, nil
}

// ensureConfigIndexExists 确保配置索引存在
func (l *CreateCpmmPoolLogic) ensureConfigIndexExists(model trademodel.CpmmFeeTierModel, configIndex int32) error {
	var tier trademodel.CpmmFeeTier
	err := l.svcCtx.DB.WithContext(l.ctx).
		Where("config_index = ?", configIndex).
		Take(&tier).Error
	if err != nil {
		return fmt.Errorf("配置索引 %d 不存在", configIndex)
	}
	return nil
}

// getTokenDecimals 通过调用 market 服务获取代币的 decimals
func (l *CreateCpmmPoolLogic) getTokenDecimals(tokenMint string) (int64, error) {
	if l.svcCtx.MarketTokenClient == nil {
		return 0, errors.New("市场代币客户端不可用")
	}

	resp, err := l.svcCtx.MarketTokenClient.GetTokenInfo(l.ctx, &market.GetTokenInfoRequest{
		ChainId:      int64(l.svcCtx.Config.SolConfig.ChainId),
		TokenAddress: tokenMint,
	})
	if err != nil {
		logx.Errorf("调用market服务获取代币信息失败: %v", err)
		return 0, fmt.Errorf("获取代币信息失败 %s: %w", tokenMint, err)
	}

	if resp == nil {
		logx.Errorf("代币信息未找到 %s", tokenMint)
		return 0, fmt.Errorf("代币信息未找到 %s", tokenMint)
	}

	return resp.Decimals, nil
}
