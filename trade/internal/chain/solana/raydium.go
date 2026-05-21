package solana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logc"
	"gorm.io/gorm"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/cpmm"
	"richcode.cc/dex/pkg/trade"
	"richcode.cc/dex/pkg/xcode"
)

func getCpmmKeys(ctx context.Context, db *gorm.DB, pairAddr string) (*solmodel.CpmmPoolInfo, error) {
	cpmmKeys, err := solmodel.NewCpmmPoolInfoModel(db).FindOneByPoolState(ctx, pairAddr)
	if err != nil {
		return nil, err
	}
	if cpmmKeys.AmmConfig == "" || cpmmKeys.PoolState == "" || cpmmKeys.InputVault == "" || cpmmKeys.OutputVault == "" ||
		cpmmKeys.InputTokenMint == "" || cpmmKeys.OutputTokenMint == "" || cpmmKeys.TradeFeeRate == 0 || cpmmKeys.ObservationState == "" {
		logc.Errorf(ctx, "cpmmKeys filed nil:%#v", cpmmKeys)
		return nil, xcode.InternalError
	}
	return cpmmKeys, nil
}

func createRaydiumClmmInstruction(ctx context.Context, db *gorm.DB, in *CreateMarketTx, amtUint64 uint64, isBuy bool, inAta, outAta aSDK.PublicKey, client *ag_rpc.Client) (aSDK.Instruction, uint64, uint64, error) {
	fmt.Println("createRaydiumClmmInstruction in is:", in)
	// Set the correct CLMM program ID for devnet
	clmm.SetDevnetProgramID()

	priceDecimal, err := decimal.NewFromString(in.Price)
	if err != nil {
		return nil, 0, 0, err
	}
	// 先去查v2的信息
	clmmKeys, err := solmodel.NewClmmPoolInfoV2Model(db).FindOneByPoolState(ctx, in.PairAddr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 查不到v2的再去查v1
			clmmKeysV1, err := solmodel.NewClmmPoolInfoV1Model(db).FindOneByPoolState(ctx, in.PairAddr)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					//v1 也查不到，那么clmm的池子不存在，报错
					return nil, 0, 0, xcode.PoolNotFound
				}
				return nil, 0, 0, err
			}
			// 查到v1了，就按照v1的构造
			minAmountOut, amountOut, err := trade.CalcMinAmountOutByPrice(in.Slippage, amtUint64, isBuy, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal, uint64(clmmKeysV1.TradeFeeRate))
			if nil != err {
				return nil, 0, 0, err
			}
			inputVault := aSDK.MustPublicKeyFromBase58(clmmKeysV1.InputVault)
			outputVault := aSDK.MustPublicKeyFromBase58(clmmKeysV1.OutputVault)
			if !isBuy {
				inputVault, outputVault = outputVault, inputVault
			}

			// Use tick arrays from database (these came from successful transactions)
			remainingAccounts := convertRemainAccout(clmmKeysV1.RemainingAccounts)
			mainTickArray := aSDK.MustPublicKeyFromBase58(clmmKeysV1.TickArray)

			// Detailed logging for tick arrays
			logc.Infof(ctx, "🎯 CLMM V1 Tick Array Details:")
			logc.Infof(ctx, "  📋 Pool State: %s", clmmKeysV1.PoolState)
			logc.Infof(ctx, "  🔑 Main Tick Array: %s", mainTickArray.String())
			logc.Infof(ctx, "  📊 Raw Remaining Accounts String: %s", clmmKeysV1.RemainingAccounts)
			logc.Infof(ctx, "  🔢 Remaining Accounts Count: %d", len(remainingAccounts))

			// Log each tick array with more detail
			if len(remainingAccounts) == 0 {
				logc.Errorf(ctx, "  ⚠️  WARNING: No remaining tick arrays found in database!")
			} else {
				for i, account := range remainingAccounts {
					logc.Infof(ctx, "  🎱 Tick Array [%d]: %s", i, account.String())
				}
			}

			// Total tick arrays available (main + remaining)
			totalTickArrays := len(remainingAccounts) + 1 // +1 for main tick array
			logc.Infof(ctx, "  📈 Total Tick Arrays Available: %d (1 main + %d remaining)", totalTickArrays, len(remainingAccounts))

			swapV1Para := &clmm.SwapV1Para{
				// Parameters:
				// Amount of tokens to swap in
				AmountIn: amtUint64,
				//  amount of tokens to receive after swap
				OtherAmountThreshold: minAmountOut,
				//SqrtPriceLimitX64:    sqrtPriceLimitX64,
				IsBaseInput:        true,
				Payer:              in.UserWalletAccount,
				AmmConfig:          aSDK.MustPublicKeyFromBase58(clmmKeysV1.AmmConfig),
				PoolState:          aSDK.MustPublicKeyFromBase58(clmmKeysV1.PoolState),
				InputTokenAccount:  inAta,
				OutputTokenAccount: outAta,
				InputVault:         inputVault,
				OutputVault:        outputVault,
				ObservationState:   aSDK.MustPublicKeyFromBase58(clmmKeysV1.ObservationState),
				TokenProgram:       aSDK.TokenProgramID,
				TickerArray:        mainTickArray,
				RemainAccounts:     remainingAccounts,
			}

			// Log the final instruction parameters
			logc.Infof(ctx, "🔧 Final V1 Swap Instruction Parameters:")
			logc.Infof(ctx, "  💰 Amount In: %d", amtUint64)
			logc.Infof(ctx, "  🎯 Min Amount Out: %d", minAmountOut)
			logc.Infof(ctx, "  🏛️  Main Tick Array: %s", mainTickArray.String())
			logc.Infof(ctx, "  📝 Remaining Accounts: %d", len(remainingAccounts))
			instructionNew, err := clmm.NewSwapV1Instruction(swapV1Para)
			if nil != err {
				return nil, 0, 0, err
			}
			return instructionNew, minAmountOut, amountOut, nil
		}
		return nil, 0, 0, err
	}
	// 查到v2了
	minAmountOut, amountOut, err := trade.CalcMinAmountOutByPrice(in.Slippage, amtUint64, isBuy, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal, uint64(clmmKeys.TradeFeeRate))
	if nil != err {
		return nil, 0, 0, err
	}
	inputVault := aSDK.MustPublicKeyFromBase58(clmmKeys.InputVault)
	outputVault := aSDK.MustPublicKeyFromBase58(clmmKeys.OutputVault)
	if !isBuy {
		inputVault, outputVault = outputVault, inputVault
	}

	// Use tick arrays from database V2 (these came from successful transactions)
	remainingAccounts := convertRemainAccout(clmmKeys.RemainingAccounts)

	// Detailed logging for V2 tick arrays
	logc.Infof(ctx, "🎯 CLMM V2 Tick Array Details:")
	logc.Infof(ctx, "  📋 Pool State: %s", clmmKeys.PoolState)
	logc.Infof(ctx, "  📊 Raw Remaining Accounts String: %s", clmmKeys.RemainingAccounts)
	logc.Infof(ctx, "  🔢 Remaining Accounts Count: %d", len(remainingAccounts))

	// Log each tick array with more detail
	if len(remainingAccounts) == 0 {
		logc.Errorf(ctx, "  ⚠️  WARNING: No remaining tick arrays found in V2 database!")
	} else {
		for i, account := range remainingAccounts {
			logc.Infof(ctx, "  🎱 V2 Tick Array [%d]: %s", i, account.String())
		}
	}

	// Note: V2 doesn't have a separate main tick array like V1
	logc.Infof(ctx, "  📈 Total V2 Tick Arrays Available: %d", len(remainingAccounts))

	swapV2Para := &clmm.SwapV2Para{
		// Parameters:
		// Amount of tokens to swap in
		AmountIn: amtUint64,
		//  amount of tokens to receive after swap
		OtherAmountThreshold: minAmountOut,
		//SqrtPriceLimitX64:    sqrtPriceLimitX64,
		IsBaseInput:        true,
		Payer:              in.UserWalletAccount,
		AmmConfig:          aSDK.MustPublicKeyFromBase58(clmmKeys.AmmConfig),
		PoolState:          aSDK.MustPublicKeyFromBase58(clmmKeys.PoolState),
		InputTokenAccount:  inAta,
		OutputTokenAccount: outAta,
		InputVault:         inputVault,
		OutputVault:        outputVault,
		ObservationState:   aSDK.MustPublicKeyFromBase58(clmmKeys.ObservationState),
		TokenProgram:       aSDK.TokenProgramID,
		TokenProgram2022:   aSDK.Token2022ProgramID,
		MemoProgram:        aSDK.MustPublicKeyFromBase58(clmmKeys.MemoProgram),
		InputVaultMint:     in.InMint,
		OutputVaultMint:    in.OutMint,
		RemainAccounts:     remainingAccounts,
	}

	// Log the final V2 instruction parameters
	logc.Infof(ctx, "🔧 Final V2 Swap Instruction Parameters:")
	logc.Infof(ctx, "  💰 Amount In: %d", amtUint64)
	logc.Infof(ctx, "  🎯 Min Amount Out: %d", minAmountOut)
	logc.Infof(ctx, "  📝 Remaining Accounts: %d", len(remainingAccounts))
	instructionNew, err := clmm.NewSwapV2Instruction(swapV2Para)
	if nil != err {
		return nil, 0, 0, err
	}

	return instructionNew, minAmountOut, amountOut, nil
}

func convertRemainAccout(remainAccounts string) []aSDK.PublicKey {
	//accounts := strings.Split(remainAccounts, ",")
	var accounts []string
	err := json.Unmarshal([]byte(remainAccounts), &accounts)
	if err != nil {
		logc.Errorf(context.Background(), "🔴 Failed to parse remaining accounts JSON: %v, raw string: %s", err, remainAccounts)
		return nil
	}

	logc.Infof(context.Background(), "🔄 Converting %d tick array strings to PublicKeys:", len(accounts))
	for i, account := range accounts {
		logc.Infof(context.Background(), "  🔗 [%d]: %s", i, account)
	}

	res := make([]aSDK.PublicKey, len(accounts))
	for k, v := range accounts {
		res[k] = aSDK.MustPublicKeyFromBase58(v)
	}

	logc.Infof(context.Background(), "✅ Successfully converted %d tick arrays", len(res))
	return res
}

func createRaydiumCpmmInstruction(ctx context.Context, db *gorm.DB, in *CreateMarketTx, amtUint64 uint64, isBuy bool, inAta, outAta aSDK.PublicKey, cli *ag_rpc.Client) (aSDK.Instruction, uint64, uint64, error) {
	cpmmKeys, err := getCpmmKeys(ctx, db, in.PairAddr)
	if err != nil {
		return nil, 0, 0, err
	}
	inputVault, err := aSDK.PublicKeyFromBase58(cpmmKeys.InputVault)
	if err != nil {
		return nil, 0, 0, err
	}
	outputVault, err := aSDK.PublicKeyFromBase58(cpmmKeys.OutputVault)
	if err != nil {
		return nil, 0, 0, err
	}
	if !isBuy {
		inputVault, outputVault = outputVault, inputVault
	}

	var minAmountOut, amountOut uint64
	priceDecimal, err := decimal.NewFromString(in.Price)
	if err != nil {
		return nil, 0, 0, err
	}
	if in.UsePriceLimit && priceDecimal.IsPositive() {
		minAmountOut, amountOut, err = trade.CalcMinAmountOutByPrice(in.Slippage, amtUint64, isBuy, priceDecimal.InexactFloat64(), in.InDecimal, in.OutDecimal, uint64(cpmmKeys.TradeFeeRate))
		if nil != err {
			return nil, 0, 0, err
		}
	} else {
		amounts, err := GetMulTokenBalance(ctx, cli, inputVault, outputVault)
		if nil != err {
			return nil, 0, 0, err
		}
		if len(amounts) != 2 {
			logc.Errorf(ctx, "GetMulTokenBalance len(amounts) != 2, inputVault:%s,outputVault:%s", inputVault.String(), outputVault.String())
			return nil, 0, 0, xcode.InternalError
		}
		minAmountOut, amountOut, err = trade.CalcMinAmountOutBySwap(in.Slippage, amtUint64, amounts[0], amounts[1], uint64(cpmmKeys.TradeFeeRate))
		if nil != err {
			return nil, 0, 0, err
		}
	}

	swapBaseInPara := &cpmm.SwapBaseInputPara{
		// Parameters:
		// Amount of tokens to swap in
		AmountIn: amtUint64,
		//  amount of tokens to receive after swap
		MinimumAmountOut: minAmountOut,
		// account
		Payer:              in.UserWalletAccount,
		Authority:          aSDK.MustPublicKeyFromBase58(cpmmKeys.Authority),
		AmmConfig:          aSDK.MustPublicKeyFromBase58(cpmmKeys.AmmConfig),
		PoolState:          aSDK.MustPublicKeyFromBase58(cpmmKeys.PoolState),
		InputTokenAccount:  inAta,
		OutputTokenAccount: outAta,
		InputVault:         inputVault,
		OutputVault:        outputVault,
		InputTokenProgram:  in.InTokenProgram,
		OutputTokenProgram: in.OutTokenProgram,
		InputTokenMint:     in.InMint,
		OutputTokenMint:    in.OutMint,
		ObservationState:   aSDK.MustPublicKeyFromBase58(cpmmKeys.ObservationState),
	}
	instructionNew, err := cpmm.NewSwapBaseInputInstruction(swapBaseInPara)
	if nil != err {
		return nil, 0, 0, err
	}

	return instructionNew, minAmountOut, amountOut, nil
}

// getAdditionalTickArrays function removed - we now use tick arrays from database
// which come from successful transactions and are more reliable than dynamic fetching
