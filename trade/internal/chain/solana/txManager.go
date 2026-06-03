package solana

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	bin "github.com/gagliardetto/binary"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/pumpfun"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/trade"
	"richcode.cc/dex/pkg/xcode"
	"richcode.cc/dex/trade/internal/clients"
	trade2 "richcode.cc/dex/trade/trade"

	aSDK "github.com/gagliardetto/solana-go"
	alt "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"gorm.io/gorm"
	"richcode.cc/dex/pkg/pumpfun/generated/pump"
)

type GasType int32

const (
	GasType_GasTypeSpeedInvalid GasType = 0
	GasType_NormalSpeed         GasType = 1
	GasType_FastSpeed           GasType = 2
	GasType_SuperFastSpeed      GasType = 3
)

type TxManager struct {
	FeeReceiver aSDK.PublicKey

	Client *ag_rpc.Client

	// jito related
	JitoClient   *ag_rpc.Client
	JitoUUID     string
	jitoTipFloor *JitoTipFloor
	RWLock       sync.RWMutex

	SimulateOnly bool
	DB           *gorm.DB
	rentFee      uint64
}

func NewTxManager(db *gorm.DB, rpcEndpoint, jitoEndPoint, uuid string, simulateOnly bool) (*TxManager, error) {
	var jitoClient *ag_rpc.Client
	if len(jitoEndPoint) > 0 {
		if 0 == len(uuid) {
			return nil, errors.New("uuid not configured but jito is configured")
		}
		headers := make(map[string]string)
		headers["Content-Type"] = "application/json"
		headers["uuid"] = uuid
		jitoClient = ag_rpc.NewWithHeaders(jitoEndPoint, headers)
	}

	tm := &TxManager{
		DB:           db,
		FeeReceiver:  aSDK.MustPublicKeyFromBase58(sol.FeeReceiver),
		Client:       ag_rpc.New(rpcEndpoint),
		JitoClient:   jitoClient,
		JitoUUID:     uuid,
		SimulateOnly: simulateOnly,
	}

	if len(jitoEndPoint) > 0 {
		tm.jitoTipFloor = &JitoTipFloor{
			Time:                        time.Now(),
			LandedTips25ThPercentile:    0.000001,
			LandedTips50ThPercentile:    0.00001,
			LandedTips75ThPercentile:    0.00004,
			LandedTips95ThPercentile:    0.006,
			LandedTips99ThPercentile:    0.018,
			EmaLandedTips50ThPercentile: 0.000014,
		}
		threading.GoSafe(func() {
			tm.CheckJitoFloorFee()
		})
	}
	threading.GoSafe(func() {
		tm.CheckRentFee()
	})

	return tm, nil
}

func (tm *TxManager) CreateMarketOrder(ctx context.Context, createMarketTx *trade.CreateMarketTx) (string, error) {
	logc.Debugf(ctx, "CreateMarketOrder:%#v", createMarketTx)
	in, err := convertCreateMarketTx(createMarketTx)
	if err != nil {
		fmt.Println("convertCreateMarketTx error:", err)
		return "", err
	}
	fmt.Println("input is:", in)

	return tm.CreateMarketTx(ctx, in)
}

func convertCreateMarketTx(in *trade.CreateMarketTx) (*CreateMarketTx, error) {
	fmt.Printf("convertCreateMarketTx input: UserWalletAddress='%s', InTokenCa='%s', OutTokenCa='%s', InTokenProgram='%s', OutTokenProgram='%s'\n",
		in.UserWalletAddress, in.InTokenCa, in.OutTokenCa, in.InTokenProgram, in.OutTokenProgram)

	//TODO: how to pass userwalletaddress?
	userWalletAccount, err := aSDK.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		fmt.Printf("Error parsing UserWalletAddress '%s': %v\n", in.UserWalletAddress, err)
		return nil, err
	}
	inMint, err := aSDK.PublicKeyFromBase58(in.InTokenCa)
	if err != nil {
		fmt.Printf("Error parsing InTokenCa '%s': %v\n", in.InTokenCa, err)
		return nil, err
	}
	outMint, err := aSDK.PublicKeyFromBase58(in.OutTokenCa)
	if err != nil {
		fmt.Printf("Error parsing OutTokenCa '%s': %v\n", in.OutTokenCa, err)
		return nil, err
	}
	if in.InTokenProgram == "" {
		in.InTokenProgram = aSDK.TokenProgramID.String()
	}
	inTokenProgram, err := aSDK.PublicKeyFromBase58(in.InTokenProgram)
	if err != nil {
		fmt.Printf("Error parsing InTokenProgram '%s': %v\n", in.InTokenProgram, err)
		return nil, err
	}
	if in.OutTokenProgram == "" {
		in.OutTokenProgram = aSDK.TokenProgramID.String()
	}
	outTokenProgram, err := aSDK.PublicKeyFromBase58(in.OutTokenProgram)
	if err != nil {
		fmt.Printf("Error parsing OutTokenProgram '%s': %v\n", in.OutTokenProgram, err)
		return nil, err
	}

	return &CreateMarketTx{
		UserId:            in.UserId,
		ChainId:           in.ChainId,
		UserWalletId:      in.UserWalletId,
		UserWalletAccount: userWalletAccount,
		AmountIn:          in.AmountIn,
		IsAntiMev:         in.IsAntiMev,
		Slippage:          in.Slippage,
		IsAutoSlippage:    in.IsAutoSlippage,
		GasType:           in.GasType,
		TradePoolName:     in.TradePoolName,
		InDecimal:         in.InDecimal,
		OutDecimal:        in.OutDecimal,
		InMint:            inMint,
		OutMint:           outMint,
		PairAddr:          in.PairAddr,
		Price:             in.Price,
		UsePriceLimit:     in.UsePriceLimit,
		InTokenProgram:    inTokenProgram,
		OutTokenProgram:   outTokenProgram,
	}, nil
}

// CreateMarketOrder creates a market order transaction on Solana
// It supports both Raydium V4 and PumpFun trading pools
// Parameters:
//   - ctx: context for the request
//   - in: input parameters for creating market order
//   - simulateOnly: if true, only simulate the transaction without sending
//
// Returns the transaction signature or error
func (tm *TxManager) CreateMarketTx(ctx context.Context, in *CreateMarketTx) (string, error) {
	var instructions []aSDK.Instruction
	var err error
	switch in.TradePoolName {
	case constants.RaydiumV4, constants.RaydiumConcentratedLiquidity, constants.RaydiumCPMM, constants.PumpSwap:
		// Create Raydium market order instructions
		fmt.Println("CreateMarketTx: RaydiumV4, RaydiumConcentratedLiquidity, RaydiumCPMM, PumpSwap")
		instructions, err = tm.CreateMarketOrderDex(ctx, in)
		if nil != err {
			return "", err
		}
	case constants.PumpFun:
		// Create PumpFun market order instructions
		instructions, err = tm.CreateMarketOrder4Pumpfun(ctx, in)
		if nil != err {
			return "", err
		}
	default:
		return "", fmt.Errorf("TradePoolName:%s not support", in.TradePoolName)
	}

	// Prepare TEE signing parameters
	teeSignPara := &clients.SignTransactionReq{
		OmniAccount: strconv.FormatUint(in.UserId, 10),
		WalletIndex: in.UserWalletId,
		Address:     in.UserWalletAccount.String(),
	}

	// Sign and send transaction
	txHash, err := tm.SignByTeeAndSend(ctx, instructions, teeSignPara, in.IsAntiMev)
	if err != nil {
		logc.Error(ctx, err)
		return "", err

	}
	return txHash, nil
}

// CreateMarketOrderDex creates instructions for a market order on Raydium DEX
func (tm *TxManager) CreateMarketOrder4PumpSwap(ctx context.Context, in *CreateMarketTx) ([]aSDK.Instruction, error) {
	initiator := in.UserWalletAccount
	outMint := in.OutMint
	//需要创建2个ata账户，尽管token 账户可能存在，但是为了避免网络请求，不做判断，直接认为需要这个费用
	lamportCost := tm.rentFee * 2

	amtDecimal, err := decimal.NewFromString(in.AmountIn)
	if nil != err {
		return nil, err
	}

	amtDecimal = amtDecimal.Mul(decimal.NewFromInt(sol.Decimals2Value[in.InDecimal]))
	amtUint64 := uint64(amtDecimal.IntPart())

	// #1 - Compute Budget: SetComputeUnitPrice
	var cu uint32
	switch in.TradePoolName {
	case constants.PumpSwap:
		cu = sol.PumpSwapCU
	default:
		return nil, fmt.Errorf("trade pool :%s not support", in.TradePoolName)
	}
	// 计算资源费 = 总钱(gasFee) - 固定签名费(GasPerSignature)
	// 计算jito费用
	instructions, lamportCostFee, err := tm.CreateGasAndJitoByGasFee(ctx, in.IsAntiMev, initiator, cu, sol.GasMODE[sol.GasType(in.GasType)])
	if nil != err {
		return nil, err
	}
	lamportCost += lamportCostFee

	// #3 - Associated Token Account Program: CreateIdempotent
	instructionNew, err := sol.CreateAtaIdempotent(initiator, initiator, in.InMint, in.InTokenProgram)
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, instructionNew)

	inAta, _, err := sol.FindAssociatedTokenAddress(initiator, in.InMint, in.InTokenProgram)
	if nil != err {
		return nil, err
	}

	outAta, _, err := sol.FindAssociatedTokenAddress(initiator, outMint, in.OutTokenProgram)
	if nil != err {
		return nil, err
	}

	solBalanceInfo, err := tm.Client.GetBalance(ctx, initiator, ag_rpc.CommitmentProcessed)
	if nil != err {
		return nil, err
	}
	solBalance := solBalanceInfo.Value

	// #4 - System Program: Transfer, if in mint is wrapper sol
	// #5 - Token Program: SyncNative
	serviceFee := uint64(0)
	isBuy := in.InMint == aSDK.WrappedSol
	if isBuy {
		if solBalance < amtUint64 {
			return nil, xcode.SolBalanceNotEnough
		}

		// 计算服务费，计算根据 原始的数量
		serviceFeeDecimal := amtDecimal.Mul(sol.ServericeFeePercent)
		serviceFee = uint64(serviceFeeDecimal.IntPart())
		lamportCost += serviceFee + amtUint64

		instructionNew, err = system.NewTransferInstruction(amtUint64, initiator, inAta).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)

		instructionNew, err = token.NewSyncNativeInstruction(inAta).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)
	}
	if lamportCost > solBalance {
		return nil, xcode.SolGasNotEnough
	}

	// #6 - Associated Token Account Program: CreateIdempotent
	instructionNew, err = sol.CreateAtaIdempotent(initiator, initiator, outMint, in.OutTokenProgram)
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, instructionNew)

	var minAmountOut, amountOut uint64
	// #7 - dex instruction
	switch in.TradePoolName {
	case constants.PumpSwap:
		instructionNew, minAmountOut, amountOut, err = createPumpSwapInstructionV2(ctx, tm.DB, in, amtUint64, isBuy, inAta, outAta, tm.Client)
		if nil != err {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("trade pool :%s not support", in.TradePoolName)
	}
	instructions = append(instructions, instructionNew)

	// #8 - Token Program: CloseAccount
	if in.InMint == aSDK.WrappedSol {
		instructionNew, err = token.NewCloseAccountInstruction(inAta, initiator, initiator, nil).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)
	}

	if outMint == aSDK.WrappedSol && amountOut > 0 {
		instructionNew, err = token.NewCloseAccountInstruction(outAta, initiator, initiator, nil).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)

		serviceFee = uint64(decimal.NewFromInt(int64(amountOut)).Mul(sol.ServericeFeePercent).IntPart())
	}

	logc.Debugf(ctx, "CreateMarketOrderDex, initiator=%s, serviceFee=%d, AmountIn=%d, minAmountOut=%d, inMint=%s, outMint=%s",
		initiator, serviceFee, amtUint64, minAmountOut, in.InMint.String(), outMint.String())

	return instructions, nil
}

// 创建市价单交易的指令，支持Raydium V4、Raydium Concentrated Liquidity、Raydium CPMM和PumpSwap交易池
func (tm *TxManager) CreateMarketOrderDex(ctx context.Context, in *CreateMarketTx) ([]aSDK.Instruction, error) {
	initiator := in.UserWalletAccount
	outMint := in.OutMint
	//需要创建2个ata账户，尽管token 账户可能存在，但是为了避免网络请求，不做判断，直接认为需要这个费用
	lamportCost := tm.rentFee * 2

	amtDecimal, err := decimal.NewFromString(in.AmountIn)
	if nil != err {
		return nil, err
	}

	amtDecimal = amtDecimal.Mul(decimal.NewFromInt(sol.Decimals2Value[in.InDecimal]))
	amtUint64 := uint64(amtDecimal.IntPart())

	// #1 - Compute Budget: SetComputeUnitPrice
	var cu uint32
	switch in.TradePoolName {
	case constants.RaydiumCPMM:
		cu = sol.RaydiumCpmmSwapCu
	case constants.RaydiumConcentratedLiquidity:
		cu = sol.RaydiumClmmSwapCu
	case constants.RaydiumV4:
		cu = sol.RaydiumV4SwapCU
	case constants.PumpSwap:
		cu = sol.PumpSwapCU
	default:
		return nil, fmt.Errorf("trade pool :%s not support", in.TradePoolName)
	}
	// 计算资源费 = 总钱(gasFee) - 固定签名费(GasPerSignature)
	instructions, lamportCostFee, err := tm.CreateGasAndJitoByGasFee(ctx, in.IsAntiMev, initiator, cu, sol.GasMODE[sol.GasType(in.GasType)])
	if nil != err {
		return nil, err
	}
	lamportCost += lamportCostFee

	// #3 - Associated Token Account Program: CreateIdempotent
	instructionNew, err := sol.CreateAtaIdempotent(initiator, initiator, in.InMint, in.InTokenProgram)
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, instructionNew)

	inAta, _, err := sol.FindAssociatedTokenAddress(initiator, in.InMint, in.InTokenProgram)
	if nil != err {
		return nil, err
	}

	outAta, _, err := sol.FindAssociatedTokenAddress(initiator, outMint, in.OutTokenProgram)
	if nil != err {
		return nil, err
	}

	solBalanceInfo, err := tm.Client.GetBalance(ctx, initiator, ag_rpc.CommitmentProcessed)
	if nil != err {
		return nil, err
	}
	solBalance := solBalanceInfo.Value

	// #4 - System Program: Transfer, if in mint is wrapper sol
	// #5 - Token Program: SyncNative
	serviceFee := uint64(0)
	isBuy := in.InMint == aSDK.WrappedSol
	if isBuy {
		if solBalance < amtUint64 {
			return nil, xcode.SolBalanceNotEnough
		}

		// 计算服务费，计算根据 原始的数量
		serviceFeeDecimal := amtDecimal.Mul(sol.ServericeFeePercent)
		serviceFee = uint64(serviceFeeDecimal.IntPart())
		lamportCost += serviceFee + amtUint64

		instructionNew, err = system.NewTransferInstruction(amtUint64, initiator, inAta).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)

		instructionNew, err = token.NewSyncNativeInstruction(inAta).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)
	}
	if lamportCost > solBalance {
		return nil, xcode.SolGasNotEnough
	}

	// #6 - Associated Token Account Program: CreateIdempotent
	instructionNew, err = sol.CreateAtaIdempotent(initiator, initiator, outMint, in.OutTokenProgram)
	if nil != err {
		return nil, err
	}
	instructions = append(instructions, instructionNew)

	var minAmountOut, amountOut uint64
	// #7 - dex instruction
	switch in.TradePoolName {
	case constants.RaydiumCPMM:
		instructionNew, minAmountOut, amountOut, err = createRaydiumCpmmInstruction(ctx, tm.DB, in, amtUint64, isBuy, inAta, outAta, tm.Client)
		if nil != err {
			return nil, err
		}
	case constants.RaydiumConcentratedLiquidity:
		instructionNew, minAmountOut, amountOut, err = createRaydiumClmmInstruction(ctx, tm.DB, in, amtUint64, isBuy, inAta, outAta, tm.Client)
		if nil != err {
			return nil, err
		}
	case constants.PumpSwap:
		instructionNew, minAmountOut, amountOut, err = createPumpSwapInstructionV2(ctx, tm.DB, in, amtUint64, isBuy, inAta, outAta, tm.Client)
		if nil != err {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("trade pool :%s not support", in.TradePoolName)
	}
	instructions = append(instructions, instructionNew)

	// #8 - Token Program: CloseAccount
	if in.InMint == aSDK.WrappedSol {
		instructionNew, err = token.NewCloseAccountInstruction(inAta, initiator, initiator, nil).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)
	}

	if outMint == aSDK.WrappedSol && amountOut > 0 {
		instructionNew, err = token.NewCloseAccountInstruction(outAta, initiator, initiator, nil).ValidateAndBuild()
		if nil != err {
			return nil, err
		}
		instructions = append(instructions, instructionNew)

		serviceFee = uint64(decimal.NewFromInt(int64(amountOut)).Mul(sol.ServericeFeePercent).IntPart())
	}

	logc.Debugf(ctx, "CreateMarketOrderDex, initiator=%s, serviceFee=%d, AmountIn=%d, minAmountOut=%d, inMint=%s, outMint=%s",
		initiator, serviceFee, amtUint64, minAmountOut, in.InMint.String(), outMint.String())

	return instructions, nil
}

// / 模拟交易-失败则报错
func (tm *TxManager) simulate(ctx context.Context, tx *aSDK.Transaction) error {
	simOut, err := tm.Client.SimulateTransactionWithOpts(ctx, tx, &ag_rpc.SimulateTransactionOpts{
		Commitment: ag_rpc.CommitmentProcessed,
	})
	if err != nil {
		logc.Error(ctx, err)
		return err
	}
	if nil != simOut && nil != simOut.Value && simOut.Value.Err != nil {
		logs := strings.Join(simOut.Value.Logs, " ")
		logc.Infof(ctx, "simOut failed , logs %s , err:%v", logs, simOut.Value.Err)
		return errors.New(logs)
	}
	return nil
}

func GetMulTokenBalance(ctx context.Context, cli *ag_rpc.Client, accounts ...aSDK.PublicKey) ([]uint64, error) {
	res, err := cli.GetMultipleAccountsWithOpts(ctx, accounts, &ag_rpc.GetMultipleAccountsOpts{
		Commitment: ag_rpc.CommitmentProcessed,
	})
	if err != nil {
		return nil, err
	}

	var amounts []uint64
	for i := range res.Value {
		if res.Value[i] == nil || res.Value[i].Data == nil {
			continue
		}
		var coinBalance token.Account
		if err = bin.NewBinDecoder(res.Value[i].Data.GetBinary()).Decode(&coinBalance); nil != err {
			return nil, err
		}

		amounts = append(amounts, coinBalance.Amount)
	}

	return amounts, nil
}

func (tm *TxManager) SignByTeeAndSend(ctx context.Context, insts []aSDK.Instruction, signTransactionReq *clients.SignTransactionReq, isAntiMev bool) (string, error) {
	tx, err := tm.Sign(ctx, insts, signTransactionReq)
	if err != nil {
		return "", err
	}
	if tm.SimulateOnly {
		// 模拟交易是否成功
		err = tm.simulate(ctx, tx)
		if err != nil {
			return "", err
		}

		return tx.Signatures[0].String(), err
	}

	if isAntiMev {
		sig, err := tm.SendViaJitoRetry(ctx, tx)
		if nil != err {
			txData, _ := json.Marshal(tx)
			logc.Infof(ctx, "SendTransaction tx:%s failed:%s", string(txData), err.Error())
			return "", err
		}
		return sig, nil
	}

	// 真正发送交易
	sig, err := tm.Client.SendTransactionWithOpts(ctx, tx, ag_rpc.TransactionOpts{
		SkipPreflight:       false,
		PreflightCommitment: ag_rpc.CommitmentProcessed,
	})
	if nil != err {
		txData, _ := json.Marshal(tx)
		logc.Infof(ctx, "SendTransaction tx:%s failed:%s", string(txData), err.Error())
		return "", err
	}

	return sig.String(), nil
}

func (tm *TxManager) Sign(ctx context.Context, insts []aSDK.Instruction, signTransactionReq *clients.SignTransactionReq) (*aSDK.Transaction, error) {
	// Get latest blockhash with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	feePayer, err := aSDK.PublicKeyFromBase58(signTransactionReq.Address)
	if err != nil {
		return nil, err
	}

	tx, err := aSDK.NewTransaction(insts, resp.Value.Blockhash, aSDK.TransactionPayer(feePayer))
	if err != nil {
		return nil, err
	}

	//TODO: 私钥使用环境变量
	privateKeyBase64 := os.Getenv("PRIVATE_KEY")
	if privateKeyBase64 == "" {
		return nil, fmt.Errorf("SOLANA_PRIVATE_KEY environment variable not set")
	}

	// Decode base64 private key
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %v", err)
	}

	// Create ed25519 private key
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)

	// Sign the transaction message
	messageContent, err := tx.Message.MarshalBinary()
	if err != nil {
		logc.Error(ctx, err)
		return nil, xcode.InternalError
	}

	signature := ed25519.Sign(privateKey, messageContent)

	// Set signatures for all required signers
	signerKeys := tx.Message.AccountKeys[0:tx.Message.Header.NumRequiredSignatures]
	tx.Signatures = make([]aSDK.Signature, len(signerKeys))

	// Copy signature to all required signers (assuming single signer for now)
	for i := range signerKeys {
		copy(tx.Signatures[i][:], signature)
	}

	return tx, nil
}

// BuildUnsignedTransaction builds an unsigned transaction for third-party wallet signing
func (tm *TxManager) BuildUnsignedTransaction(ctx context.Context, createMarketTx *trade.CreateMarketTx) (string, error) {
	logx.WithContext(ctx).Infof("Building unsigned transaction for third-party wallet signing")

	in, err := convertCreateMarketTx(createMarketTx)
	if err != nil {
		return "", err
	}

	// Get instructions without signing
	var instructions []aSDK.Instruction
	switch in.TradePoolName {
	case constants.PumpFun:
		instructions, err = tm.CreateMarketOrder4Pumpfun(ctx, in)
		if err != nil {
			return "", err
		}
	case constants.RaydiumV4, constants.RaydiumConcentratedLiquidity, constants.RaydiumCPMM:
		instructions, err = tm.CreateMarketOrderDex(ctx, in)
		if err != nil {
			return "", err
		}
	case constants.PumpSwap:
		instructions, err = tm.CreateMarketOrder4PumpSwap(ctx, in)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("TradePoolName:%s not support", in.TradePoolName)
	}

	// Get latest blockhash with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create unsigned transaction
	feePayer, err := aSDK.PublicKeyFromBase58(createMarketTx.UserWalletAddress)
	if err != nil {
		return "", err
	}

	tx, err := aSDK.NewTransaction(instructions, resp.Value.Blockhash, aSDK.TransactionPayer(feePayer))
	if err != nil {
		return "", err
	}

	// Initialize empty signatures for the transaction
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]aSDK.Signature, numSigners)

	// Serialize the complete transaction (with empty signatures)
	txData, err := tx.MarshalBinary()
	if err != nil {
		logx.WithContext(ctx).Errorf("Failed to serialize transaction: %v", err)
		return "", err
	}

	// Return the serialized transaction as base64
	return base64.StdEncoding.EncodeToString(txData), nil
}

// BuildUnsignedPumpCreateTransaction builds an unsigned transaction for PumpFun token creation
func (tm *TxManager) BuildUnsignedPumpCreateTransaction(ctx context.Context, in *trade2.CreatePumpTokenRequest) (string, error) {
	// Parse inputs
	logx.Infof("开始构建未签名的创建PumpFun代币的交易，输入参数: %#v", in)
	user, err := aSDK.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		return "", fmt.Errorf("invalid user_wallet_address: %w", err)
	}
	mint, err := aSDK.PublicKeyFromBase58(in.Mint)
	if err != nil {
		return "", fmt.Errorf("invalid mint: %w", err)
	}

	// Derive PumpFun PDAs and accounts
	global, _, eventAuthority, _, err := pumpfun.GetPumpFunPDAs(mint)
	if err != nil {
		return "", fmt.Errorf("GetPumpFunPDAs err: %w", err)
	}
	bondingCurve, _, err := pumpfun.GetBondingCurvePDA(mint)
	if err != nil {
		return "", fmt.Errorf("GetBondingCurvePDA err: %w", err)
	}
	associatedBondingCurve, _, err := aSDK.FindAssociatedTokenAddress(bondingCurve, mint)
	if err != nil {
		return "", fmt.Errorf("FindAssociatedTokenAddress err: %w", err)
	}

	// Metaplex Token Metadata Program ID
	mplTokenMetadata := aSDK.MustPublicKeyFromBase58("metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s")
	// Derive metadata PDA: seeds ["metadata", mpl_program_id, mint]
	metadata, _, err := aSDK.FindProgramAddress([][]byte{[]byte("metadata"), mplTokenMetadata.Bytes(), mint.Bytes()}, mplTokenMetadata)
	if err != nil {
		return "", fmt.Errorf("derive metadata PDA err: %w", err)
	}

	// Required program IDs
	systemProgram := system.ProgramID
	tokenProgram := token.ProgramID
	associatedTokenProgram := aSDK.SPLAssociatedTokenAccountProgramID
	rent := aSDK.SysVarRentPubkey

	// Derive mint_authority PDA: seeds ["mint-authority"] under pump program
	mintAuthority, _, err := aSDK.FindProgramAddress([][]byte{[]byte("mint-authority")}, pump.ProgramID)
	if err != nil {
		return "", fmt.Errorf("derive mint_authority PDA err: %w", err)
	}

	// Build PumpFun create instruction
	instr, err := pump.NewCreateInstruction(
		in.Name,
		in.Symbol,
		in.Uri,
		user,          // creatorParam
		mint,          // mint (signer)
		mintAuthority, // mint_authority PDA (seed: "mint-authority")
		bondingCurve,
		associatedBondingCurve,
		global,
		mplTokenMetadata,
		metadata,
		user, // user (signer)
		systemProgram,
		tokenProgram,
		associatedTokenProgram,
		rent,
		eventAuthority,
		pump.ProgramID,
	)
	if err != nil {
		return "", fmt.Errorf("build create instruction err: %w", err)
	}

	// latest blockhash
	bh, err := tm.Client.GetLatestBlockhash(ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("GetLatestBlockhash err: %w", err)
	}

	// assemble transaction (unsigned)
	tx, err := aSDK.NewTransaction([]aSDK.Instruction{instr}, bh.Value.Blockhash, aSDK.TransactionPayer(user))
	if err != nil {
		return "", fmt.Errorf("NewTransaction err: %w", err)
	}

	// initialize empty signatures for all required signers
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]aSDK.Signature, numSigners)

	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("MarshalBinary err: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// 派生 protocolPositionPDA
func DeriveProtocolPositionPDA(poolState aSDK.PublicKey, tickLower, tickUpper int32, programID aSDK.PublicKey) (aSDK.PublicKey, uint8, error) {
	seed := [][]byte{
		[]byte("position"),
		poolState[:],
		int32ToBytesBigEndian(tickLower),
		int32ToBytesBigEndian(tickUpper),
	}
	pda, bump, err := aSDK.FindProgramAddress(seed, programID)
	return pda, bump, err
}

// 辅助函数：int32 转大端字节序
func int32ToBytesBigEndian(i int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(i))
	return b
}

// Helper methods required for the CreateAddLiquidityInstructions function

// getPoolInfo fetches pool information from the chain
func (tm *TxManager) getPoolInfo(ctx context.Context, poolState aSDK.PublicKey) (*struct {
	ProgramID string
}, error) {
	accountInfo, err := tm.Client.GetAccountInfo(ctx, poolState)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool account info: %w", err)
	}
	programID := accountInfo.Value.Owner.String()
	return &struct {
		ProgramID string
	}{
		ProgramID: programID,
	}, nil
}

// getPoolAccounts retrieves the token vaults and observation state for a pool
// func (tm *TxManager) getPoolAccounts(ctx context.Context, poolState aSDK.PublicKey, tokenMintA aSDK.PublicKey, tokenMintB aSDK.PublicKey, programID aSDK.PublicKey) (tokenVaultA aSDK.PublicKey, tokenVaultB aSDK.PublicKey, observationState aSDK.PublicKey, err error) {
// 		// 1. 先生成 poolState
// 		// amm_config := aSDK.MustPublicKeyFromBase58("FiyUUSnhBgLhBgVGWNBVozhzSbAbFCU1Q8iWMHH3xUhA")
// 		amm_config, err := GetAmmConfigPDA(7, programID)
// 		if err != nil {
// 			return
// 		}
// 		// 保证 tokenMintA < tokenMintB
// 		if bytes.Compare(tokenMintA[:], tokenMintB[:]) > 0 {
// 			tokenMintA, tokenMintB = tokenMintB, tokenMintA
// 		}
// 		logx.Infof("✅ Amm config: %s", amm_config.String())
// 		poolState, _, err = aSDK.FindProgramAddress(
// 			[][]byte{
// 				[]byte("pool"),
// 				amm_config[:],
// 				tokenMintA[:],
// 				tokenMintB[:],
// 			},
// 			programID,
// 		)
// 		if err != nil {
// 			return
// 		}
// 		logx.Infof("✅ Pool state11: %s", poolState.String())

// 		accountInfo, err := tm.Client.GetAccountInfo(ctx, poolState)

// 		logx.Infof("✅ Account info1111: %s", accountInfo.Value.Owner.String())
// 	tokenVaultA, _, err = aSDK.FindProgramAddress(
// 		[][]byte{
// 			[]byte("token_vault"),
// 			poolState[:],
// 			tokenMintA[:],
// 		},
// 		programID,
// 	)
// 	if err != nil {
// 		return
// 	}

// 	tokenVaultB, _, err = aSDK.FindProgramAddress(
// 		[][]byte{
// 			[]byte("token_vault"),
// 			poolState[:],
// 			tokenMintB[:],
// 		},
// 		programID,
// 	)
// 	if err != nil {
// 		return
// 	}

// 	observationState, _, err = aSDK.FindProgramAddress(
// 		[][]byte{
// 			[]byte("observation"),
// 			poolState[:],
// 		},
// 		programID,
// 	)
// 	return
// }

// GetPoolVaultsByPoolStateAddr 获取 poolState 账户的 token vault 地址
func (tm *TxManager) GetPoolVaultsByPoolStateAddr(ctx context.Context, poolStateAddr aSDK.PublicKey) (aSDK.PublicKey, aSDK.PublicKey, aSDK.PublicKey, aSDK.PublicKey, aSDK.PublicKey, int, error) {
	accountInfo, err := tm.Client.GetAccountInfo(ctx, poolStateAddr)
	if err != nil {
		return aSDK.PublicKey{}, aSDK.PublicKey{}, aSDK.PublicKey{}, aSDK.PublicKey{}, aSDK.PublicKey{}, 0, fmt.Errorf("failed to get poolState account info: %w", err)
	}
	data := accountInfo.Value.Data.GetBinary()
	tokenMint0 := aSDK.PublicKeyFromBytes(data[73:105])
	tokenMint1 := aSDK.PublicKeyFromBytes(data[105:137])
	tokenVault0 := aSDK.PublicKeyFromBytes(data[137:169])
	tokenVault1 := aSDK.PublicKeyFromBytes(data[169:201])
	observationKey := aSDK.PublicKeyFromBytes(data[201:233])
	tickSpacingBytes := data[235:237]                                // []byte
	tickSpacing := int(binary.LittleEndian.Uint16(tickSpacingBytes)) // int
	fmt.Println("tickSpacing:", tickSpacing)

	return tokenMint0, tokenMint1, tokenVault0, tokenVault1, observationKey, tickSpacing, nil
}

// findOrCreateATAInstruction finds or creates an associated token account
func (tm *TxManager) findOrCreateATAInstruction(ctx context.Context, mint, owner aSDK.PublicKey) (aSDK.PublicKey, error) {
	// In a real implementation, you would:
	// 1. Check if the ATA exists
	// 2. If not, create an instruction to create it
	// 3. Return the ATA public key

	// Derive the associated token address
	ata, _, err := aSDK.FindProgramAddress(
		[][]byte{
			owner[:],
			aSDK.TokenProgramID[:],
			mint[:],
		},
		aSDK.SPLAssociatedTokenAccountProgramID,
	)
	if err != nil {
		return aSDK.PublicKey{}, err
	}

	return ata, nil
}

// int32ToBytes returns little-endian bytes for int32
func int32ToBytes(n int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

// findTickArrays finds the tick arrays for the lower and upper ticks
func (tm *TxManager) findTickArrays(ctx context.Context, poolState aSDK.PublicKey, tickSpacing int, tickLowerPrice, tickUpperPrice int32, programID aSDK.PublicKey) (tickArrayLower, tickArrayUpper aSDK.PublicKey, tickArrayLowerStartIndex, tickArrayUpperStartIndex int, tickLower, tickUpper int, err error) {

	//除以10的六次方
	fmt.Println("1tickLowerPrice:", tickLowerPrice)
	fmt.Println("2tickUpperPrice:", tickUpperPrice)

	tickLowerDec := decimal.NewFromInt32(tickLowerPrice).Div(decimal.NewFromInt(1000000))
	tickUpperDec := decimal.NewFromInt32(tickUpperPrice).Div(decimal.NewFromInt(1000000))

	fmt.Println("3tickLowerPrice:", tickLowerDec)
	fmt.Println("3tickUpperPrice:", tickUpperDec)

	// Calculate tick array start indices
	tickArraySize := int(60) // Standard size for Raydium CLMM
	// 如果你需要 tickLower/tickUpper 作为整数参与 tick 计算，先将 decimal 转 float64，再转 int
	tickLower = tickWithSpacing(priceToTick(tickLowerDec.InexactFloat64()), int(tickSpacing))
	tickUpper = tickWithSpacing(priceToTick(tickUpperDec.InexactFloat64()), int(tickSpacing))
	tickArrayLowerStartIndex = getTickArrayStartIndexByTick(tickLower, tickSpacing, tickArraySize)
	tickArrayUpperStartIndex = getTickArrayStartIndexByTick(tickUpper, tickSpacing, tickArraySize)

	// 	tickSpacing: 60
	// tickLowerPrice: 0
	// tickUpperPrice: 1
	// tickLower: 0
	// tickUpper: 0
	// tickArrayLowerStartIndex: 0
	// tickArrayUpperStartIndex: 0
	fmt.Println("tickLower:", tickLower)
	fmt.Println("tickUpper:", tickUpper)
	fmt.Println("tickArrayLowerStartIndex:", tickArrayLowerStartIndex)
	fmt.Println("tickArrayUpperStartIndex:", tickArrayUpperStartIndex)

	tickArrayLower, bumpLower, err := aSDK.FindProgramAddress(
		[][]byte{
			[]byte("tick_array"),
			poolState[:],
			int32ToBytes(int32(tickArrayLowerStartIndex)),
		},
		programID,
	)
	if err != nil {
		logx.Errorf("Failed to derive tickArrayLower: %v", err)
	}
	logx.Infof("tickArrayLower: %s, bump: %d, startIndex: %d", tickArrayLower.String(), bumpLower, tickArrayLowerStartIndex)

	tickArrayUpper, bumpUpper, err := aSDK.FindProgramAddress(
		[][]byte{
			[]byte("tick_array"),
			poolState[:],
			int32ToBytes(int32(tickArrayUpperStartIndex)),
		},
		programID,
	)
	if err != nil {
		logx.Errorf("Failed to derive tickArrayUpper: %v", err)
	}
	logx.Infof("tickArrayUpper: %s, bump: %d, startIndex: %d", tickArrayUpper.String(), bumpUpper, tickArrayUpperStartIndex)

	return
}

// priceToTick 计算 tick 索引
func priceToTick(price float64) int {
	const Q_RATIO = 1.0001
	return int(math.Log(price) / math.Log(Q_RATIO))
}

// tickWithSpacing 对齐到 tickSpacing 的整数倍
func tickWithSpacing(tick, tickSpacing int) int {
	compressed := tick / tickSpacing
	if tick < 0 && tick%tickSpacing != 0 {
		compressed -= 1 // round towards negative infinity
	}
	return compressed * tickSpacing
}

func getTickArrayStartIndexByTick(tickIndex, tickSpacing, tickArraySize int) int {
	ticksInArray := tickSpacing * tickArraySize
	start := tickIndex / ticksInArray
	if tickIndex < 0 && tickIndex%ticksInArray != 0 {
		start -= 1
	}
	return start * ticksInArray
}

// buildOpenPositionInstruction creates the OpenPosition instruction
func (tm *TxManager) buildOpenPositionInstruction(
	userWallet aSDK.PublicKey,
	positionMint aSDK.PublicKey,
	poolState aSDK.PublicKey,
	tokenAMint aSDK.PublicKey,
	tokenBMint aSDK.PublicKey,
	userTokenA aSDK.PublicKey,
	userTokenB aSDK.PublicKey,
	tokenVaultA aSDK.PublicKey,
	tokenVaultB aSDK.PublicKey,
	tickLower int32,
	tickUpper int32,
	tickArrayLower aSDK.PublicKey,
	tickArrayUpper aSDK.PublicKey,
	positionPDA aSDK.PublicKey,
	observationState aSDK.PublicKey,
	baseTokenIndex uint8,
	amount0Max uint64,
	amount1Max uint64,
	tickArrayLowerStartIndex int,
	tickArrayUpperStartIndex int,
	protocol_position_pda aSDK.PublicKey,
) aSDK.Instruction {
	logx.Infof("🔧 buildOpenPositionInstruction: Building OpenPosition instruction (smaller transaction)")
	logx.Infof("📋 Instruction parameters:")
	logx.Infof("   - User wallet: %s", userWallet.String())
	logx.Infof("   - Position mint: %s", positionMint.String())
	logx.Infof("   - Pool state: %s", poolState.String())
	logx.Infof("   - Token A mint: %s", tokenAMint.String())
	logx.Infof("   - Token B mint: %s", tokenBMint.String())
	logx.Infof("   - User token A: %s", userTokenA.String())
	logx.Infof("   - User token B: %s", userTokenB.String())
	logx.Infof("   - Token vault A: %s", tokenVaultA.String())
	logx.Infof("   - Token vault B: %s", tokenVaultB.String())
	logx.Infof("   - Tick lower: %d", tickLower)
	logx.Infof("   - Tick upper: %d", tickUpper)
	logx.Infof("   - Tick array lower: %s", tickArrayLower.String())
	logx.Infof("   - Tick array upper: %s", tickArrayUpper.String())
	logx.Infof("   - Position PDA: %s", positionPDA.String())
	logx.Infof("   - Observation state: %s", observationState.String())
	logx.Infof("   - Base token index: %d", baseTokenIndex)
	logx.Infof("   - Amount 0 max: %d", amount0Max)
	logx.Infof("   - Amount 1 max: %d", amount1Max)

	// === Derive Metaplex MetadataAccount PDA for the position mint ===
	metadataProgramID := aSDK.MustPublicKeyFromBase58("metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s")
	metadataAccount, _, err := aSDK.FindProgramAddress(
		[][]byte{
			[]byte("metadata"),
			metadataProgramID[:],
			positionMint[:],
		},
		metadataProgramID,
	)
	if err != nil {
		logx.Errorf("❌ Failed to derive MetadataAccount PDA: %v", err)
	} else {
		logx.Infof("✅ MetadataAccount PDA: %s", metadataAccount.String())
	}

	// Note: amount0Max and amount1Max are now passed as parameters from the caller
	// and are already calculated based on the sorted token order

	// Create the position token account (ATA for position NFT)
	positionATA, _, err := aSDK.FindProgramAddress(
		[][]byte{
			userWallet[:],
			aSDK.TokenProgramID[:],
			positionMint[:],
		},
		aSDK.SPLAssociatedTokenAccountProgramID,
	)
	if err != nil {
		logx.Errorf("❌ Failed to derive position ATA: %v", err)
		// Fall back to using userWallet as positionATA in case of error
		positionATA = userWallet
	}

	// Use binary.Uint128 with correct values for liquidity
	// For now, setting a placeholder liquidity amount
	liquidity := bin.Uint128{
		Lo: amount0Max, // Using amount0Max as Lo for simplicity
		Hi: 0,          // Using 0 as Hi for simplicity
	}

	fmt.Println("tickLower:", tickLower)
	fmt.Println("tickUpper:", tickUpper)
	fmt.Println("tickArrayLowerStartIndex:", tickArrayLowerStartIndex)
	fmt.Println("tickArrayUpperStartIndex:", tickArrayUpperStartIndex)
	fmt.Println("amount0Max:", amount0Max)
	fmt.Println("amount1Max:", amount1Max)

	// openPositionInst := amm_v3.NewOpenPositionInstruction(
	// 	int32(tickLower),
	// 	int32(tickUpper),
	// 	int32(tickArrayLowerStartIndex),
	// 	int32(tickArrayUpperStartIndex),
	// 	liquidity,
	// 	amount0Max,
	// 	amount1Max,
	// 	//accounts:
	// 	userWallet,
	// 	userWallet,
	// 	positionMint,
	// 	positionATA,
	// 	metadataAccount,
	// 	poolState,
	// 	protocol_position_pda,
	// 	tickArrayLower,
	// 	tickArrayUpper,
	// 	positionPDA,
	// 	userTokenA,
	// 	userTokenB,
	// 	tokenVaultA,
	// 	tokenVaultB,
	// 	aSDK.SysVarRentPubkey,
	// 	aSDK.SystemProgramID,
	// 	aSDK.TokenProgramID,
	// 	aSDK.SPLAssociatedTokenAccountProgramID,
	// 	metadataProgramID,
	// )

	// Manually build the OpenPosition instruction with correct account order
	// This ensures only the first 3 accounts are marked as signers
	accountMetas := aSDK.AccountMetaSlice{
		// 1. payer (用户钱包) - 需要签名
		aSDK.Meta(userWallet).WRITE().SIGNER(),
		// 2. position_nft_owner (用户钱包) - 需要签名
		aSDK.Meta(userWallet).SIGNER(),
		// 3. position_nft_mint (positionMint) - 需要签名
		aSDK.Meta(positionMint).WRITE().SIGNER(),
		// 4. position_nft_account (positionATA) - 不需要签名
		aSDK.Meta(positionATA).WRITE(),
		// 5. metadata_account - 不需要签名
		aSDK.Meta(metadataAccount).WRITE(),
		// 6. pool_state - 不需要签名
		aSDK.Meta(poolState).WRITE(),
		// 7. protocol_position - 不需要签名
		aSDK.Meta(protocol_position_pda).WRITE(),
		// 8. tick_array_lower - 不需要签名
		aSDK.Meta(tickArrayLower).WRITE(),
		// 9. tick_array_upper - 不需要签名
		aSDK.Meta(tickArrayUpper).WRITE(),
		// 10. personal_position - 不需要签名
		aSDK.Meta(positionPDA).WRITE(),
		// 11. token_account_0 (userTokenA) - 不需要签名
		aSDK.Meta(userTokenA).WRITE(),
		// 12. token_account_1 (userTokenB) - 不需要签名
		aSDK.Meta(userTokenB).WRITE(),
		// 13. token_vault_0 (tokenVaultA) - 不需要签名
		aSDK.Meta(tokenVaultA).WRITE(),
		// 14. token_vault_1 (tokenVaultB) - 不需要签名
		aSDK.Meta(tokenVaultB).WRITE(),
		// 15. rent - 不需要签名
		aSDK.Meta(aSDK.SysVarRentPubkey),
		// 16. system_program - 不需要签名
		aSDK.Meta(aSDK.SystemProgramID),
		// 17. token_program - 不需要签名
		aSDK.Meta(aSDK.TokenProgramID),
		// 18. associated_token_program - 不需要签名
		aSDK.Meta(aSDK.SPLAssociatedTokenAccountProgramID),
		// 19. metadata_program - 不需要签名
		aSDK.Meta(metadataProgramID),
	}

	// Create instruction data for OpenPosition
	// The discriminator for OpenPosition is {77, 184, 74, 214, 112, 86, 241, 199}
	instructionData := []byte{135, 128, 47, 77, 15, 152, 240, 49}
	// ag_binary.TypeID([8]byte{})
	// Add instruction parameters
	// We need to encode: tick_lower_index, tick_upper_index, tick_array_lower_start_index, tick_array_upper_start_index, liquidity, amount_0_max, amount_1_max
	// Each parameter should be encoded as little-endian

	// Add tick_lower_index (i32, 4 bytes)
	tickLowerBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(tickLowerBytes, uint32(tickLower))
	instructionData = append(instructionData, tickLowerBytes...)

	// Add tick_upper_index (i32, 4 bytes)
	tickUpperBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(tickUpperBytes, uint32(tickUpper))
	instructionData = append(instructionData, tickUpperBytes...)

	// Add tick_array_lower_start_index (i32, 4 bytes)
	tickArrayLowerStartIndexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(tickArrayLowerStartIndexBytes, uint32(tickArrayLowerStartIndex))
	instructionData = append(instructionData, tickArrayLowerStartIndexBytes...)

	// Add tick_array_upper_start_index (i32, 4 bytes)
	tickArrayUpperStartIndexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(tickArrayUpperStartIndexBytes, uint32(tickArrayUpperStartIndex))
	instructionData = append(instructionData, tickArrayUpperStartIndexBytes...)

	// Add liquidity (u128, 16 bytes)
	liquidityBytes := make([]byte, 16)
	binary.LittleEndian.PutUint64(liquidityBytes[:8], liquidity.Lo)
	binary.LittleEndian.PutUint64(liquidityBytes[8:], liquidity.Hi)
	instructionData = append(instructionData, liquidityBytes...)

	// Add amount_0_max (u64, 8 bytes)
	amount0MaxBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amount0MaxBytes, amount0Max)
	instructionData = append(instructionData, amount0MaxBytes...)

	// Add amount_1_max (u64, 8 bytes)
	amount1MaxBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amount1MaxBytes, amount1Max)
	instructionData = append(instructionData, amount1MaxBytes...)

	// Add withMetadata (bool, 1 byte) - true for create metadata
	withMetadata := byte(1) // true
	instructionData = append(instructionData, withMetadata)

	// Add optionBaseFlag (u8, 1 byte) - 0 for no base flag
	optionBaseFlag := byte(0)
	instructionData = append(instructionData, optionBaseFlag)

	// Add baseFlag (bool, 1 byte) - false for no base flag
	baseFlag := byte(0) // false
	instructionData = append(instructionData, baseFlag)

	// Create the instruction
	openPositionInst := aSDK.NewInstruction(
		aSDK.MustPublicKeyFromBase58("A1izdbCxDvLjZ2WZFkPdSLNBrrYrhBqxmmzCkm82G4ys"), // Raydium CLMM program ID
		accountMetas,
		instructionData,
	)

	// If the builder supports setting MetadataAccount, add it here:
	// .SetMetadataAccount(metadataAccount)
	// If not, log for further integration.

	// Build the instruction
	// instruction, err := openPositionInst.ValidateAndBuild()
	// if err != nil || instruction == nil {
	// 	logx.Errorf("❌ Failed to build OpenPosition instruction: %v", err)
	// 	// Fall back to a simple transfer if the instruction build fails
	// 	return system.NewTransferInstruction(
	// 		1000, // Minimal amount
	// 		userWallet,
	// 		userWallet,
	// 	).Build()
	// }

	logx.Infof("✅ Successfully built OpenPosition instruction")
	return openPositionInst
}

// findRaydiumPoolPDAs finds the correct PDAs for Raydium CLMM pools
// This function implements the PDA derivation logic from the Raydium SDK
func findRaydiumPoolPDAs(ctx context.Context, programID aSDK.PublicKey, token0, token1, ammConfig aSDK.PublicKey) (
	poolState aSDK.PublicKey,
	tokenVault0 aSDK.PublicKey,
	tokenVault1 aSDK.PublicKey,
	observationState aSDK.PublicKey,
	tickArrayBitmap aSDK.PublicKey,
	err error) {

	// Define seed constants
	poolSeed := []byte("pool")
	poolVaultSeed := []byte("pool_vault") // Corrected from "token_vault" to "pool_vault"
	observationSeed := []byte("observation")
	bitmapSeed := []byte("pool_tick_array_bitmap_extension")

	// First, derive the pool state PDA using ammConfig and token mints
	// This matches the getPdaPoolId function in JS
	poolIdSeeds := [][]byte{
		poolSeed,
		ammConfig[:],
		token0[:],
		token1[:],
	}

	poolState, _, err = aSDK.FindProgramAddress(poolIdSeeds, programID)
	if err != nil {
		return
	}

	// Log derivation data for debugging
	logx.WithContext(ctx).Infof("Deriving PDAs for: token0=%s, token1=%s, ammConfig=%s, programID=%s",
		token0.String(), token1.String(), ammConfig.String(), programID.String())
	logx.WithContext(ctx).Infof("Derived pool state PDA: %s", poolState.String())

	// Derive token vault PDAs using poolVaultSeed
	// This matches the getPdaPoolVaultId function in JS
	tokenVault0Seeds := [][]byte{
		poolVaultSeed,
		poolState[:],
		token0[:],
	}
	tokenVault0, _, err = aSDK.FindProgramAddress(tokenVault0Seeds, programID)
	if err != nil {
		return
	}

	tokenVault1Seeds := [][]byte{
		poolVaultSeed,
		poolState[:],
		token1[:],
	}
	tokenVault1, _, err = aSDK.FindProgramAddress(tokenVault1Seeds, programID)
	if err != nil {
		return
	}

	logx.WithContext(ctx).Infof("Derived token vault 0 PDA: %s", tokenVault0.String())
	logx.WithContext(ctx).Infof("Derived token vault 1 PDA: %s", tokenVault1.String())

	// Derive the observation state PDA using the pool state
	// This matches the getPdaObservationAccount function in JS
	observationStateSeeds := [][]byte{
		observationSeed,
		poolState[:],
	}
	observationState, _, err = aSDK.FindProgramAddress(observationStateSeeds, programID)
	if err != nil {
		return
	}

	// Derive the tick array bitmap PDA using the pool state
	// This matches the getPdaExBitmapAccount function in JS
	tickArrayBitmapSeeds := [][]byte{
		bitmapSeed,
		poolState[:],
	}
	tickArrayBitmap, _, err = aSDK.FindProgramAddress(tickArrayBitmapSeeds, programID)

	return
}

// PrintALTTable loads and logs the ALT table content for debugging
func (tm *TxManager) PrintALTTable(ctx context.Context, altBase58 string) error {
	client := tm.Client // ag_rpc.Client must implement GetAccountInfo
	altAddr := aSDK.MustPublicKeyFromBase58(altBase58)
	altState, err := alt.GetAddressLookupTable(ctx, client, altAddr)
	if err != nil {
		logx.WithContext(ctx).Errorf("Failed to load ALT: %v", err)
		return err
	}
	logx.WithContext(ctx).Infof("ALT Authority: %v", altState.Authority)
	logx.WithContext(ctx).Infof("ALT DeactivationSlot: %d", altState.DeactivationSlot)
	logx.WithContext(ctx).Infof("ALT LastExtendedSlot: %d", altState.LastExtendedSlot)
	logx.WithContext(ctx).Infof("ALT Addresses:")
	for i, addr := range altState.Addresses {
		logx.WithContext(ctx).Infof("  [%d] %s", i, addr.String())
	}
	return nil
}

func GetAmmConfigPDA(index uint16, programID aSDK.PublicKey) (aSDK.PublicKey, error) {
	indexBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(indexBytes, index) // <--- 用 BigEndian
	seeds := [][]byte{
		[]byte("amm_config"),
		indexBytes,
	}

	pda, _, err := aSDK.FindProgramAddress(seeds, programID)
	return pda, err
}
