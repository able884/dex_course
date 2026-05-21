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
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// SwapCpmmLogic 处理 CPMM（恒定乘积做市商）交换业务逻辑
type SwapCpmmLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewSwapCpmmLogic 创建 CPMM 交换逻辑处理器
func NewSwapCpmmLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SwapCpmmLogic {
	return &SwapCpmmLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// SwapCpmm 执行 CPMM 池内代币交换
// 该方法负责：
// 1. 参数校验
// 2. 构建交换交易
// 3. 返回未签名交易给客户端
func (l *SwapCpmmLogic) SwapCpmm(in *trade.SwapCpmmRequest) (*trade.SwapCpmmResponse, error) {
	// 1. 参数校验
	if err := l.validate(in); err != nil {
		return nil, err
	}

	// 2. 构建交换交易
	txBase64, err := l.buildSwapTx(in)
	if err != nil {
		return nil, err
	}

	// 3. 返回结果
	return &trade.SwapCpmmResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(), // 交易10分钟后过期
	}, nil
}

// validate 校验交换请求参数
func (l *SwapCpmmLogic) validate(in *trade.SwapCpmmRequest) error {
	if in == nil {
		return errors.New("请求不能为空")
	}
	if strings.TrimSpace(in.PoolState) == "" {
		return errors.New("pool_state 必填")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address 必填")
	}
	if strings.TrimSpace(in.InputMint) == "" {
		return errors.New("input_mint 必填")
	}

	// 校验金额必须为正数
	amtDec, err := decimal.NewFromString(strings.TrimSpace(in.AmountIn))
	if err != nil {
		return fmt.Errorf("amount_in 无效: %w", err)
	}
	if amtDec.Cmp(decimal.Zero) <= 0 {
		return errors.New("amount_in 必须大于零")
	}

	// 校验滑点不能超过100%
	if in.SlippageBps > 10000 {
		return errors.New("slippage_bps 过大")
	}

	return nil
}

// buildSwapTx 构建 CPMM 交换交易
// 该方法负责：
// 1. 获取 RPC 客户端
// 2. 解析用户钱包地址和池信息
// 3. 派生 PDA 地址
// 4. 获取代币精度
// 5. 查询报价获取最小接收金额
// 6. 构建交易指令（包括 ATA 创建和交换指令）
// 7. 序列化交易为 base64
func (l *SwapCpmmLogic) buildSwapTx(in *trade.SwapCpmmRequest) (string, error) {
	// 1. 校验 RPC 客户端
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("Solana RPC 客户端未配置")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	// 2. 设置链ID（默认 Solana）
	chainID := in.ChainId
	if chainID == 0 {
		chainID = constants.SolChainIdInt
	}

	// 3. 解析用户钱包地址
	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("无效的用户钱包地址: %w", err)
	}

	// 4. 查询池信息
	poolModel := solmodel.NewCpmmPoolInfoModel(l.svcCtx.DB)
	pool, err := poolModel.FindOneByPoolState(l.ctx, strings.TrimSpace(in.PoolState))
	if err != nil || pool == nil {
		return "", fmt.Errorf("找不到池状态 %s: %w", in.PoolState, err)
	}

	// 5. 解析代币地址
	inputMintPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(pool.InputTokenMint))
	if err != nil {
		return "", fmt.Errorf("无效的池输入代币: %w", err)
	}
	outputMintPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(pool.OutputTokenMint))
	if err != nil {
		return "", fmt.Errorf("无效的池输出代币: %w", err)
	}
	payMintPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.InputMint))
	if err != nil {
		return "", fmt.Errorf("无效的输入代币: %w", err)
	}

	// 6. 确定接收代币
	receiveMintPk := outputMintPk
	if payMintPk.Equals(outputMintPk) {
		receiveMintPk = inputMintPk
	}

	// 7. 如果指定了输出代币，则使用指定的
	if strings.TrimSpace(in.OutputMint) != "" {
		outPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.OutputMint))
		if err != nil {
			return "", fmt.Errorf("无效的输出代币: %w", err)
		}
		receiveMintPk = outPk
	}

	// 8. 校验输入/输出代币必须属于池
	if !payMintPk.Equals(inputMintPk) && !payMintPk.Equals(outputMintPk) {
		return "", errors.New("input_mint 不属于池")
	}
	if !receiveMintPk.Equals(inputMintPk) && !receiveMintPk.Equals(outputMintPk) {
		return "", errors.New("output_mint 不属于池")
	}
	if payMintPk.Equals(receiveMintPk) {
		return "", errors.New("input_mint 和 output_mint 不能相同")
	}

	// 9. 设置程序ID和AMM配置
	ammConfig := aSDK.MustPublicKeyFromBase58(pool.AmmConfig)
	var programID aSDK.PublicKey
	if chainID == constants.SolChainIdInt {
		programID = aSDK.MustPublicKeyFromBase58(cpmm.ProgramRaydiumCPMMProgramDevNet.String())
	} else {
		programID = aSDK.MustPublicKeyFromBase58(cpmm.ProgramRaydiumCPMMProgram.String())
	}
	cpmm.SetProgramID(programID)

	// 10. 派生池 PDA 地址
	poolState, _, authority, token0Vault, token1Vault, observationState, err := cpmm.DeriveCpmmPoolPDAs(
		ammConfig,
		inputMintPk,
		outputMintPk,
		programID,
	)
	if err != nil {
		return "", fmt.Errorf("派生 PDA 失败: %w", err)
	}

	// 11. 校验池状态地址匹配
	if poolState.String() != strings.TrimSpace(pool.PoolState) {
		return "", fmt.Errorf("pool_state 与派生的 PDA 不匹配")
	}

	// 12. 设置输入/输出方向
	payIsToken0 := payMintPk.Equals(inputMintPk)
	inputVault := token0Vault
	outputVault := token1Vault
	inputTokenProgram := parseProgramOrDefault(pool.InputTokenProgram)
	outputTokenProgram := parseProgramOrDefault(pool.OutputTokenProgram)

	if !payIsToken0 {
		inputVault = token1Vault
		outputVault = token0Vault
		inputTokenProgram, outputTokenProgram = outputTokenProgram, inputTokenProgram
		receiveMintPk = inputMintPk
	}

	// 13. 获取代币精度
	payDecimals, err := l.getTokenDecimals(payMintPk.String(), int64(chainID))
	if err != nil {
		return "", fmt.Errorf("获取输入代币精度失败: %w", err)
	}
	recvDecimals, err := l.getTokenDecimals(receiveMintPk.String(), int64(chainID))
	if err != nil {
		return "", fmt.Errorf("获取输出代币精度失败: %w", err)
	}

	// 14. 转换金额为最小单位
	amountInDec, _ := decimal.NewFromString(strings.TrimSpace(in.AmountIn))
	payMultiplier := decimal.NewFromFloat(math.Pow10(int(payDecimals)))
	amountInRaw := amountInDec.Mul(payMultiplier).IntPart()
	if amountInRaw <= 0 {
		return "", errors.New("amount_in 过小")
	}

	// 15. 设置滑点（默认0.5%）
	slippageBps := in.SlippageBps
	if slippageBps == 0 {
		slippageBps = 50
	}
	if slippageBps > 10000 {
		slippageBps = 10000
	}

	// 16. 查询报价获取最小接收金额
	quoteResp, err := l.svcCtx.MarketClient.QuoteCpmm(l.ctx, &market.QuoteCpmmRequest{
		ChainId:     int64(chainID),
		PoolState:   pool.PoolState,
		InputMint:   payMintPk.String(),
		OutputMint:  receiveMintPk.String(),
		AmountIn:    strings.TrimSpace(in.AmountIn),
		SlippageBps: int64(slippageBps),
	})
	if err != nil {
		return "", fmt.Errorf("报价失败: %w", err)
	}

	minRecvStr := strings.TrimSpace(quoteResp.GetMinReceiveAmount())
	if minRecvStr == "" {
		return "", errors.New("报价缺少 min_receive_amount")
	}

	minRecvDec, err := decimal.NewFromString(minRecvStr)
	if err != nil {
		return "", fmt.Errorf("min_receive_amount 无效: %w", err)
	}

	recvMultiplier := decimal.NewFromFloat(math.Pow10(int(recvDecimals)))
	minRecvRaw := minRecvDec.Mul(recvMultiplier).IntPart()
	if minRecvRaw <= 0 {
		return "", errors.New("min_receive_amount 过小")
	}

	// 17. 查找用户的 ATA 地址
	ownerInputAta, _, err := aSDK.FindAssociatedTokenAddress(userWallet, payMintPk)
	if err != nil {
		return "", err
	}
	ownerOutputAta, _, err := aSDK.FindAssociatedTokenAddress(userWallet, receiveMintPk)
	if err != nil {
		return "", err
	}

	// 18. 确保 ATA 存在（不存在则创建）
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

	if err := ensureATA(payMintPk, ownerInputAta); err != nil {
		return "", fmt.Errorf("确保输入 ATA 失败: %w", err)
	}
	if err := ensureATA(receiveMintPk, ownerOutputAta); err != nil {
		return "", fmt.Errorf("确保输出 ATA 失败: %w", err)
	}

	// 19. 构建交换指令
	swapIx, err := cpmm.NewSwapBaseInputInstruction(&cpmm.SwapBaseInputPara{
		AmountIn:           uint64(amountInRaw),
		MinimumAmountOut:   uint64(minRecvRaw),
		Payer:              userWallet,
		Authority:          authority,
		AmmConfig:          ammConfig,
		PoolState:          poolState,
		InputTokenAccount:  ownerInputAta,
		OutputTokenAccount: ownerOutputAta,
		InputVault:         inputVault,
		OutputVault:        outputVault,
		InputTokenProgram:  inputTokenProgram,
		OutputTokenProgram: outputTokenProgram,
		InputTokenMint:     payMintPk,
		OutputTokenMint:    receiveMintPk,
		ObservationState:   observationState,
	})
	if err != nil {
		return "", fmt.Errorf("构建交换指令失败: %w", err)
	}

	// 20. 获取最新区块哈希
	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("获取区块哈希失败: %w", err)
	}

	// 21. 创建交易
	instructions := append(setupInstructions, swapIx)
	tx, err := aSDK.NewTransaction(
		instructions,
		latest.Value.Blockhash,
		aSDK.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("创建交易失败: %w", err)
	}

	// 22. 初始化签名槽位（客户端签名）
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	// 23. 序列化为 base64
	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("序列化交易失败: %w", err)
	}

	return base64.StdEncoding.EncodeToString(raw), nil
}

// parseProgramOrDefault 解析代币程序地址，默认使用标准 Token 程序
func parseProgramOrDefault(program string) aSDK.PublicKey {
	p := strings.TrimSpace(program)
	if p == "" {
		return aSDK.TokenProgramID
	}
	if pk, err := aSDK.PublicKeyFromBase58(p); err == nil {
		return pk
	}
	return aSDK.TokenProgramID
}

// getTokenDecimals 获取代币精度
func (l *SwapCpmmLogic) getTokenDecimals(tokenMint string, chainID int64) (int64, error) {
	if l.svcCtx.MarketTokenClient == nil {
		return 0, errors.New("市场代币客户端不可用")
	}

	resp, err := l.svcCtx.MarketTokenClient.GetTokenInfo(l.ctx, &market.GetTokenInfoRequest{
		ChainId:      chainID,
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