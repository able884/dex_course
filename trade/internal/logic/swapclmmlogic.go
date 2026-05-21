package logic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	ag_binary "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

type SwapClmmLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSwapClmmLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SwapClmmLogic {
	return &SwapClmmLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *SwapClmmLogic) SwapClmm(in *trade.SwapClmmRequest) (*trade.SwapClmmResponse, error) {
	if err := l.validate(in); err != nil {
		return nil, err
	}
	txBase64, err := l.buildSwapTx(in)
	if err != nil {
		return nil, err
	}
	return &trade.SwapClmmResponse{
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *SwapClmmLogic) validate(in *trade.SwapClmmRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(in.PoolState) == "" {
		return errors.New("pool_state required")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address required")
	}
	if strings.TrimSpace(in.InputMint) == "" {
		return errors.New("input_mint required")
	}
	amtDec, err := decimal.NewFromString(strings.TrimSpace(in.AmountIn))
	if err != nil {
		return fmt.Errorf("amount_in invalid: %w", err)
	}
	if amtDec.Cmp(decimal.Zero) <= 0 {
		return errors.New("amount_in must be greater than zero")
	}
	if in.SlippageBps > 10000 {
		return errors.New("slippage_bps too large")
	}
	return nil
}

func (l *SwapClmmLogic) buildSwapTx(in *trade.SwapClmmRequest) (string, error) {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	rpcClient := l.svcCtx.SolTxMananger.Client

	chainID := in.ChainId
	if chainID == 0 {
		chainID = constants.SolChainIdInt
	}

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("invalid user wallet address: %w", err)
	}

	// 查询 V1 池子信息
	poolV1Model := solmodel.NewClmmPoolInfoV1Model(l.svcCtx.DB)
	poolV1, err := poolV1Model.FindOneByPoolState(l.ctx, strings.TrimSpace(in.PoolState))
	if err != nil {
		return "", fmt.Errorf("未找到池子状态 %s: %w", in.PoolState, err)
	}

	var inputVaultMint, outputVaultMint, inputVault, outputVault, ammConfig, observationState string
	var remainingAccounts string

	inputVaultMint = poolV1.InputVaultMint
	outputVaultMint = poolV1.OutputVaultMint
	inputVault = poolV1.InputVault
	outputVault = poolV1.OutputVault
	ammConfig = poolV1.AmmConfig
	observationState = poolV1.ObservationState
	remainingAccounts = poolV1.RemainingAccounts

	if inputVaultMint == "" || outputVaultMint == "" || ammConfig == "" || observationState == "" {
		return "", errors.New("pool info incomplete")
	}

	payMintPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.InputMint))
	if err != nil {
		return "", fmt.Errorf("invalid input_mint: %w", err)
	}

	inputVaultMintPk, err := aSDK.PublicKeyFromBase58(inputVaultMint)
	if err != nil {
		return "", fmt.Errorf("invalid pool input vault mint: %w", err)
	}
	outputVaultMintPk, err := aSDK.PublicKeyFromBase58(outputVaultMint)
	if err != nil {
		return "", fmt.Errorf("invalid pool output vault mint: %w", err)
	}

	receiveMintPk := outputVaultMintPk
	if payMintPk.Equals(outputVaultMintPk) {
		receiveMintPk = inputVaultMintPk
	}
	if strings.TrimSpace(in.OutputMint) != "" {
		outPk, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.OutputMint))
		if err != nil {
			return "", fmt.Errorf("invalid output_mint: %w", err)
		}
		receiveMintPk = outPk
	}

	if !payMintPk.Equals(inputVaultMintPk) && !payMintPk.Equals(outputVaultMintPk) {
		return "", errors.New("input_mint not part of pool")
	}
	if !receiveMintPk.Equals(inputVaultMintPk) && !receiveMintPk.Equals(outputVaultMintPk) {
		return "", errors.New("output_mint not part of pool")
	}
	if payMintPk.Equals(receiveMintPk) {
		return "", errors.New("input_mint and output_mint cannot be the same")
	}

	payDecimals, err := l.getTokenDecimals(payMintPk.String(), int64(chainID))
	if err != nil {
		return "", fmt.Errorf("get input token decimals failed: %w", err)
	}
	recvDecimals, err := l.getTokenDecimals(receiveMintPk.String(), int64(chainID))
	if err != nil {
		return "", fmt.Errorf("get output token decimals failed: %w", err)
	}

	amountInDec, _ := decimal.NewFromString(strings.TrimSpace(in.AmountIn))
	payMultiplier := decimal.NewFromFloat(math.Pow10(int(payDecimals)))
	amountInRaw := amountInDec.Mul(payMultiplier).IntPart()
	if amountInRaw <= 0 {
		return "", errors.New("amount_in too small")
	}

	slippageBps := in.SlippageBps
	if slippageBps == 0 {
		slippageBps = 50 // 默认 0.5%
	}
	if slippageBps > 10000 {
		slippageBps = 10000
	}

	poolStatePK, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.PoolState))
	if err != nil {
		return "", fmt.Errorf("invalid pool_state: %w", err)
	}

	isBuy := payMintPk.Equals(inputVaultMintPk)

	// 调用 quote_clmm 接口获取报价和滑点计算
	quoteResp, err := l.svcCtx.MarketClient.QuoteClmm(l.ctx, &market.QuoteClmmRequest{
		ChainId:     int64(chainID),
		PoolState:   strings.TrimSpace(in.PoolState),
		InputMint:   payMintPk.String(),
		OutputMint:  receiveMintPk.String(),
		AmountIn:    strings.TrimSpace(in.AmountIn),
		SlippageBps: int64(slippageBps),
	})
	if err != nil {
		return "", fmt.Errorf("quote clmm failed: %w", err)
	}
	minRecvStr := strings.TrimSpace(quoteResp.GetMinReceiveAmount())
	if minRecvStr == "" {
		return "", errors.New("quote missing min_receive_amount")
	}
	minRecvDec, err := decimal.NewFromString(minRecvStr)
	if err != nil {
		return "", fmt.Errorf("min_receive_amount invalid: %w", err)
	}
	recvMultiplier := decimal.NewFromFloat(math.Pow10(int(recvDecimals)))
	minAmountOutRaw := minRecvDec.Mul(recvMultiplier).IntPart()
	if minAmountOutRaw <= 0 {
		return "", errors.New("min_receive_amount too small")
	}
	minAmountOut := uint64(minAmountOutRaw)

	ownerInputAta, _, err := aSDK.FindAssociatedTokenAddress(userWallet, payMintPk)
	if err != nil {
		return "", err
	}
	ownerOutputAta, _, err := aSDK.FindAssociatedTokenAddress(userWallet, receiveMintPk)
	if err != nil {
		return "", err
	}

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
		return "", fmt.Errorf("ensure input ata failed: %w", err)
	}
	if err := ensureATA(receiveMintPk, ownerOutputAta); err != nil {
		return "", fmt.Errorf("ensure output ata failed: %w", err)
	}

	clmm.SetDevnetProgramID()
	inputVaultPK := aSDK.MustPublicKeyFromBase58(inputVault)
	outputVaultPK := aSDK.MustPublicKeyFromBase58(outputVault)
	if !isBuy {
		inputVaultPK, outputVaultPK = outputVaultPK, inputVaultPK
	}

	// 9. 构建 V1 swap 指令
	// 9.1 获取 tick spacing 和当前 tick（从链上获取最新状态）
	info, err := rpcClient.GetAccountInfoWithOpts(l.ctx, poolStatePK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get pool state account: %w", err)
	}
	if info == nil || info.Value == nil {
		return "", fmt.Errorf("pool state account not found: %s", poolStatePK.String())
	}
	data := info.Value.Data.GetBinary()
	if len(data) == 0 {
		return "", fmt.Errorf("pool state account data is empty")
	}
	decoder := ag_binary.NewBorshDecoder(data)
	var poolStateAccount amm_v3.PoolStateAccount
	if err := poolStateAccount.UnmarshalWithDecoder(decoder); err != nil {
		return "", fmt.Errorf("failed to decode pool state: %w", err)
	}
	actualTick := poolStateAccount.TickCurrent
	tickSpacing := int32(poolStateAccount.TickSpacing)
	if tickSpacing <= 0 {
		return "", fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}

	// 9.2 计算当前 tick 所在的 tick array start index
	const clmmTickArraySize int32 = 60
	block := tickSpacing * clmmTickArraySize
	if block <= 0 {
		return "", fmt.Errorf("invalid block size: tickSpacing=%d, arraySize=%d", tickSpacing, clmmTickArraySize)
	}
	quot := actualTick / block
	if actualTick < 0 && actualTick%block != 0 {
		quot--
	}
	currentTickArrayStartIndex := quot * block

	zeroForOne := isBuy

	// 9.3 确定主 tick array
	var mainTickArrayPK aSDK.PublicKey
	var finalTickArrayStartIndex int32

	// 步骤1：验证计算的当前 tick array 是否已初始化
	currentTickArrayPK, err := l.deriveTickArrayPDA(poolStatePK, currentTickArrayStartIndex)
	if err != nil {
		return "", fmt.Errorf("failed to derive current tick array PDA: %w", err)
	}

	info, err = rpcClient.GetAccountInfoWithOpts(l.ctx, currentTickArrayPK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err == nil && info != nil && info.Value != nil {
		data := info.Value.Data.GetBinary()
		if len(data) > 0 {
			decoder := ag_binary.NewBorshDecoder(data)
			var tickArrayState amm_v3.TickArrayStateAccount
			if err := tickArrayState.UnmarshalWithDecoder(decoder); err == nil {
				if tickArrayState.InitializedTickCount > 0 {
					// 当前计算的 tick array 已初始化，直接使用
					finalTickArrayStartIndex = currentTickArrayStartIndex
					mainTickArrayPK = currentTickArrayPK
				}
			}
		}
	}

	// 步骤2：如果仍未找到，从 remainingAccounts 中查找合适的
	if mainTickArrayPK.IsZero() && remainingAccounts != "" {
		var remainingAccountsFromDB []aSDK.PublicKey
		err := json.Unmarshal([]byte(remainingAccounts), &remainingAccountsFromDB)
		if err != nil {
			// 如果 JSON 解析失败，尝试按逗号分割
			parts := strings.Split(remainingAccounts, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					if pk, err := aSDK.PublicKeyFromBase58(part); err == nil {
						remainingAccountsFromDB = append(remainingAccountsFromDB, pk)
					}
				}
			}
		}

		// 遍历 remainingAccounts，查找已初始化且符合交易方向的 tick array
		for _, pk := range remainingAccountsFromDB {
			info, err := rpcClient.GetAccountInfoWithOpts(l.ctx, pk, &ag_rpc.GetAccountInfoOpts{
				Commitment: ag_rpc.CommitmentFinalized,
			})
			if err != nil || info == nil || info.Value == nil {
				continue
			}

			data := info.Value.Data.GetBinary()
			if len(data) == 0 {
				continue
			}

			decoder := ag_binary.NewBorshDecoder(data)
			var tickArrayState amm_v3.TickArrayStateAccount
			if err := tickArrayState.UnmarshalWithDecoder(decoder); err != nil {
				continue
			}

			// 检查是否已初始化，且符合交易方向
			if tickArrayState.InitializedTickCount > 0 {
				startIndex := tickArrayState.StartTickIndex
				// zeroForOne 需要更小的 tick，!zeroForOne 需要更大的 tick
				if (zeroForOne && startIndex < currentTickArrayStartIndex) ||
					(!zeroForOne && startIndex > currentTickArrayStartIndex) {
					finalTickArrayStartIndex = startIndex
					mainTickArrayPK = pk
					break
				}
			}
		}
	}

	// 步骤3：如果仍未找到，使用计算的 currentTickArrayStartIndex
	if mainTickArrayPK.IsZero() {
		finalTickArrayStartIndex = currentTickArrayStartIndex
		mainTickArrayPK, err = l.deriveTickArrayPDA(poolStatePK, finalTickArrayStartIndex)
		if err != nil {
			return "", fmt.Errorf("failed to derive main tick array PDA: %w", err)
		}
	}

	// 9.4 构建 remaining accounts
	// 策略：从持仓中查找匹配的 tick array start indices，并添加相邻的 tick arrays
	var remainingAccountsList []aSDK.PublicKey
	tickArrayStartIndexSet := make(map[int32]bool)
	existingAccountsSet := make(map[string]bool) // 用于去重

	// 步骤1：从持仓中收集所有 tick array start indices
	var positions []*solmodel.ClmmPosition
	err = l.svcCtx.DB.WithContext(l.ctx).Model(&solmodel.ClmmPosition{}).
		Where("pool_state = ? AND deleted_at IS NULL", in.PoolState).
		Find(&positions).Error
	if err != nil {
		return "", fmt.Errorf("failed to query positions: %w", err)
	}

	for _, pos := range positions {
		tickArrayStartIndexSet[pos.TickArrayLowerStartIndex] = true
		tickArrayStartIndexSet[pos.TickArrayUpperStartIndex] = true
	}

	// 步骤2：添加主 tick array 和相邻的 tick arrays（根据交易方向）
	tickArrayStartIndexSet[finalTickArrayStartIndex] = true
	if zeroForOne {
		tickArrayStartIndexSet[finalTickArrayStartIndex-block] = true
		tickArrayStartIndexSet[finalTickArrayStartIndex-2*block] = true
	} else {
		tickArrayStartIndexSet[finalTickArrayStartIndex+block] = true
		tickArrayStartIndexSet[finalTickArrayStartIndex+2*block] = true
	}

	// 步骤3：为所有 tick array start indices 派生 PDA 并验证是否已初始化
	for startIndex := range tickArrayStartIndexSet {
		// 跳过主 tick array（它会在指令中单独指定）
		if startIndex == finalTickArrayStartIndex {
			continue
		}

		tickArrayPK, err := l.deriveTickArrayPDA(poolStatePK, startIndex)
		if err != nil {
			return "", fmt.Errorf("failed to derive tick array PDA for index %d: %w", startIndex, err)
		}

		// 验证 tick array 是否已初始化（只添加已初始化的账户）
		info, err := rpcClient.GetAccountInfoWithOpts(l.ctx, tickArrayPK, &ag_rpc.GetAccountInfoOpts{
			Commitment: ag_rpc.CommitmentFinalized,
		})
		if err == nil && info != nil && info.Value != nil {
			data := info.Value.Data.GetBinary()
			if len(data) > 0 {
				// 检查账户所有者是否为 CLMM 程序（已初始化）
				if info.Value.Owner.Equals(amm_v3.ProgramID) {
					pkStr := tickArrayPK.String()
					if !existingAccountsSet[pkStr] {
						remainingAccountsList = append(remainingAccountsList, tickArrayPK)
						existingAccountsSet[pkStr] = true
					}
				}
			}
		}
	}

	// 9.5 构建 V1 swap 指令参数
	var sqrtPriceLimitX64 ag_binary.Uint128
	sqrtPriceLimitX64.Lo = 0
	sqrtPriceLimitX64.Hi = 0

	swapV1Para := &clmm.SwapV1Para{
		AmountIn:             uint64(amountInRaw),
		OtherAmountThreshold: minAmountOut,
		SqrtPriceLimitX64:    sqrtPriceLimitX64,
		IsBaseInput:          isBuy,
		Payer:                userWallet,
		AmmConfig:            aSDK.MustPublicKeyFromBase58(ammConfig),
		PoolState:            poolStatePK,
		InputTokenAccount:    ownerInputAta,
		OutputTokenAccount:   ownerOutputAta,
		InputVault:           inputVaultPK,
		OutputVault:          outputVaultPK,
		ObservationState:     aSDK.MustPublicKeyFromBase58(observationState),
		TokenProgram:         aSDK.TokenProgramID,
		TickerArray:          mainTickArrayPK,
		RemainAccounts:       remainingAccountsList,
	}
	var swapIx aSDK.Instruction
	swapIx, err = clmm.NewSwapV1Instruction(swapV1Para)
	if err != nil {
		return "", fmt.Errorf("构建 CLMM V1 swap 指令失败: %w", err)
	}

	latest, err := rpcClient.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("get blockhash failed: %w", err)
	}

	instructions := append(setupInstructions, swapIx)
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

func (l *SwapClmmLogic) getTokenDecimals(tokenMint string, chainID int64) (int64, error) {
	if l.svcCtx.MarketTokenClient == nil {
		return 0, errors.New("market token client not available")
	}
	resp, err := l.svcCtx.MarketTokenClient.GetTokenInfo(l.ctx, &market.GetTokenInfoRequest{
		ChainId:      chainID,
		TokenAddress: tokenMint,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get token info for %s: %w", tokenMint, err)
	}
	if resp == nil {
		return 0, fmt.Errorf("token info not found for %s", tokenMint)
	}
	return resp.Decimals, nil
}

// deriveTickArrayPDA 派生 tick array PDA
func (l *SwapClmmLogic) deriveTickArrayPDA(poolState aSDK.PublicKey, startIndex int32) (aSDK.PublicKey, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, startIndex); err != nil {
		return aSDK.PublicKey{}, err
	}
	seeds := [][]byte{
		[]byte("tick_array"),
		poolState.Bytes(),
		buf.Bytes(),
	}
	pda, _, err := aSDK.FindProgramAddress(seeds, amm_v3.ProgramID)
	return pda, err
}
