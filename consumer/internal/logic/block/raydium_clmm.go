package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/duke-git/lancet/v2/slice"
	bin "github.com/gagliardetto/binary"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	constants "richcode.cc/dex/pkg/constrants"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/pkg/types"
	"richcode.cc/dex/pkg/util"
)

const (
	// 指令数据最小长度
	minInstructionDataLength = 8
	// 默认代币精度
	defaultTokenDecimal = 6
)

type ConcentratedLiquidityDecoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction
	innerInstruction    *client.InnerInstruction
}

// getTokenAccountInfo 获取代币账户信息
func (decoder *ConcentratedLiquidityDecoder) getTokenAccountInfo(accountKey string) (*TokenAccount, error) {
	info := decoder.dtx.TokenAccountMap[accountKey]
	if info == nil {
		return nil, fmt.Errorf("token account info not found for account: %s, tx hash: %s", accountKey, decoder.dtx.TxHash)
	}
	return info, nil
}

// determineBaseAndTokenAccounts 确定基础代币和交易代币账户
func (decoder *ConcentratedLiquidityDecoder) determineBaseAndTokenAccounts(account0Info, account1Info *TokenAccount) (baseAccount, tokenAccount *TokenAccount) {
	// 如果其中一个是 WSOL,则它作为基础代币
	if account1Info.TokenAddress == TokenStrWrapSol {
		return account1Info, account0Info
	}
	// 默认 account0 作为基础代币
	return account0Info, account1Info
}

// calculateTokenAmount 根据金额和精度计算代币数量
func calculateTokenAmount(amount int64, tokenDecimal uint8) float64 {
	return decimal.New(amount, -int32(tokenDecimal)).InexactFloat64()
}

// createBasicTradeInfo 创建基础交易信息
func (decoder *ConcentratedLiquidityDecoder) createBasicTradeInfo(poolStateAccount, makerAccount string, tradeType string) *types.TradeWithPair {
	trade := &types.TradeWithPair{
		ChainId:          SolChainId,
		TxHash:           decoder.dtx.TxHash,
		PairAddr:         poolStateAccount,
		Maker:            makerAccount,
		Type:             tradeType,
		To:               poolStateAccount,
		Slot:             decoder.dtx.BlockDb.Slot,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		HashId:           fmt.Sprintf("%v#%d", decoder.dtx.BlockDb.Slot, decoder.dtx.TxIndex),
		TransactionIndex: decoder.dtx.TxIndex,
		SwapName:         constants.RaydiumConcentratedLiquidity,
	}
	return trade
}

// createPairInfo 创建交易对信息
func (decoder *ConcentratedLiquidityDecoder) createPairInfo(poolStateAccount string, baseAccount, tokenAccount *TokenAccount) types.Pair {
	return types.Pair{
		ChainId:          SolChainId,
		Addr:             poolStateAccount,
		BaseTokenAddr:    baseAccount.TokenAddress,
		BaseTokenDecimal: baseAccount.TokenDecimal,
		BaseTokenSymbol:  util.GetBaseToken(SolChainIdInt).Symbol,
		TokenAddr:        tokenAccount.TokenAddress,
		TokenDecimal:     tokenAccount.TokenDecimal,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:         decoder.dtx.BlockDb.Slot,
		Name:             constants.RaydiumConcentratedLiquidity,
	}
}

// updatePoolAmounts 更新池中的代币数量
func (decoder *ConcentratedLiquidityDecoder) updatePoolAmounts(trade *types.TradeWithPair, accountInfo *TokenAccount, baseTokenAddr, tokenAddr string) {
	if accountInfo == nil {
		return
	}

	amount := calculateTokenAmount(accountInfo.PostValue, accountInfo.TokenDecimal)

	if accountInfo.TokenAddress == baseTokenAddr {
		trade.CurrentBaseTokenInPoolAmount = amount
		trade.PairInfo.CurrentBaseTokenAmount = amount
	} else if accountInfo.TokenAddress == tokenAddr {
		trade.CurrentTokenInPoolAmount = amount
		trade.PairInfo.CurrentTokenAmount = amount
	}
}

// buildPairInfoForOpenPosition 为 OpenPosition 构建交易对信息
func (decoder *ConcentratedLiquidityDecoder) buildPairInfoForOpenPosition(
	poolStateAddr, tokenAccount0, tokenAccount1 string,
	token0Info, token1Info *TokenAccount,
) types.Pair {
	var (
		baseTokenAddr, tokenAddr       string
		baseTokenSymbol, tokenSymbol   string
		baseTokenDecimal, tokenDecimal uint8
	)

	// 根据代币信息确定基础代币和交易代币
	if token0Info != nil && token0Info.TokenAddress == TokenStrWrapSol {
		// token0 是 WSOL,作为基础代币
		baseTokenAddr = token0Info.TokenAddress
		baseTokenSymbol = util.GetBaseToken(SolChainIdInt).Symbol
		baseTokenDecimal = token0Info.TokenDecimal
		if token1Info != nil {
			tokenAddr = token1Info.TokenAddress
			tokenSymbol = ""
			tokenDecimal = token1Info.TokenDecimal
		} else {
			tokenAddr = tokenAccount1
			tokenSymbol = ""
			tokenDecimal = defaultTokenDecimal
		}
	} else if token1Info != nil && token1Info.TokenAddress == TokenStrWrapSol {
		// token1 是 WSOL,作为基础代币
		baseTokenAddr = token1Info.TokenAddress
		baseTokenSymbol = util.GetBaseToken(SolChainIdInt).Symbol
		baseTokenDecimal = token1Info.TokenDecimal
		if token0Info != nil {
			tokenAddr = token0Info.TokenAddress
			tokenSymbol = ""
			tokenDecimal = token0Info.TokenDecimal
		} else {
			tokenAddr = tokenAccount0
			tokenSymbol = ""
			tokenDecimal = defaultTokenDecimal
		}
	} else if token0Info != nil && token1Info != nil {
		// 两个都不是 WSOL,默认 token0 作为基础代币
		baseTokenAddr = token0Info.TokenAddress
		baseTokenSymbol = ""
		baseTokenDecimal = token0Info.TokenDecimal
		tokenAddr = token1Info.TokenAddress
		tokenSymbol = ""
		tokenDecimal = token1Info.TokenDecimal
	} else {
		// 两个代币信息都不可用,使用默认值
		baseTokenAddr = tokenAccount0
		tokenAddr = tokenAccount1
		baseTokenSymbol = ""
		tokenSymbol = ""
		baseTokenDecimal = defaultTokenDecimal
		tokenDecimal = defaultTokenDecimal
	}

	return types.Pair{
		ChainId:          SolChainId,
		Addr:             poolStateAddr,
		BaseTokenAddr:    baseTokenAddr,
		BaseTokenSymbol:  baseTokenSymbol,
		BaseTokenDecimal: baseTokenDecimal,
		TokenAddr:        tokenAddr,
		TokenSymbol:      tokenSymbol,
		TokenDecimal:     tokenDecimal,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:         decoder.dtx.BlockDb.Slot,
		Name:             constants.RaydiumConcentratedLiquidity,
	}
}

// determineBaseAndTokenMints 确定基础代币和交易代币地址
func (decoder *ConcentratedLiquidityDecoder) determineBaseAndTokenMints(tokenMint0, tokenMint1 string) (baseTokenAddr, tokenAddr string) {
	// 如果 tokenMint1 是 WSOL,则它作为基础代币
	if tokenMint1 == TokenStrWrapSol {
		return tokenMint1, tokenMint0
	}
	// 默认 tokenMint0 作为基础代币
	return tokenMint0, tokenMint1
}

// OpenPositionParams OpenPosition 指令参数
type OpenPositionParams struct {
	TickLowerIndex           int32
	TickUpperIndex           int32
	TickArrayLowerStartIndex int32
	TickArrayUpperStartIndex int32
	Liquidity                bin.Uint128
	Amount0Max               uint64
	Amount1Max               uint64
}

// decodeOpenPositionParams 解析 OpenPosition 指令参数
func (decoder *ConcentratedLiquidityDecoder) decodeOpenPositionParams() (*OpenPositionParams, error) {
	data := decoder.compiledInstruction.Data
	if len(data) < minInstructionDataLength {
		return nil, fmt.Errorf("instruction data too short: expected at least %d bytes, got %d", minInstructionDataLength, len(data))
	}

	// 跳过8字节的指令鉴别符
	dec := bin.NewBorshDecoder(data[minInstructionDataLength:])

	params := &OpenPositionParams{}

	if err := dec.Decode(&params.TickLowerIndex); err != nil {
		return nil, fmt.Errorf("failed to decode tickLowerIndex: %w", err)
	}
	if err := dec.Decode(&params.TickUpperIndex); err != nil {
		return nil, fmt.Errorf("failed to decode tickUpperIndex: %w", err)
	}
	if err := dec.Decode(&params.TickArrayLowerStartIndex); err != nil {
		return nil, fmt.Errorf("failed to decode tickArrayLowerStartIndex: %w", err)
	}
	if err := dec.Decode(&params.TickArrayUpperStartIndex); err != nil {
		return nil, fmt.Errorf("failed to decode tickArrayUpperStartIndex: %w", err)
	}
	if err := dec.Decode(&params.Liquidity); err != nil {
		return nil, fmt.Errorf("failed to decode liquidity: %w", err)
	}
	if err := dec.Decode(&params.Amount0Max); err != nil {
		return nil, fmt.Errorf("failed to decode amount0Max: %w", err)
	}
	if err := dec.Decode(&params.Amount1Max); err != nil {
		return nil, fmt.Errorf("failed to decode amount1Max: %w", err)
	}

	return params, nil
}

// DecodeRaydiumConcentratedLiquidityInstruction 解码 Raydium 集中流动性指令
func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityInstruction() (*types.TradeWithPair, error) {
	discriminator := GetInstructionDiscriminator(decoder.compiledInstruction.Data)
	fmt.Println("clmm decoder discriminator is:", discriminator, amm_v3.Instruction_SwapV2[:])
	if bytes.Equal(discriminator, amm_v3.Instruction_Swap[:]) {
		return decoder.DecodeRaydiumConcentratedLiquiditySwap()
	} else if bytes.Equal(discriminator, amm_v3.Instruction_SwapV2[:]) {
		return decoder.DecodeRaydiumConcentratedLiquiditySwapV2()
	} else if bytes.Equal(discriminator, amm_v3.Instruction_CreatePool[:]) {
		return decoder.DecodeRaydiumConcentratedLiquidityCreatePool()
	} else if bytes.Equal(discriminator, amm_v3.Instruction_OpenPosition[:]) {
		return decoder.DecodeRaydiumConcentratedLiquidityOpenPosition()
	} else if bytes.Equal(discriminator, amm_v3.Instruction_IncreaseLiquidityV2[:]) {
		return decoder.DecodeRaydiumConcentratedLiquidityIncreaseLiquidityV2()
	} else if bytes.Equal(discriminator, amm_v3.Instruction_DecreaseLiquidityV2[:]) {
		return decoder.DecodeRaydiumConcentratedLiquidityDecreaseLiquidityV2()
	} else {
		return nil, ErrNotSupportInstruction
	}
}

// DecodeRaydiumConcentratedLiquidityIncreaseLiquidityV2 解码增加流动性V2指令
func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityIncreaseLiquidityV2() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	nftOwnerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]
	tokenAccount0 := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]]
	tokenAccount1 := tx.AccountKeys[decoder.compiledInstruction.Accounts[8]]

	// 获取代币账户信息
	account0Info, err := decoder.getTokenAccountInfo(tokenAccount0.String())
	if err != nil {
		return nil, err
	}
	account1Info, err := decoder.getTokenAccountInfo(tokenAccount1.String())
	if err != nil {
		return nil, err
	}

	// 确定基础代币和交易代币
	baseAccount, tokenAccount := decoder.determineBaseAndTokenAccounts(account0Info, account1Info)

	// 创建基础交易信息
	trade := decoder.createBasicTradeInfo(
		poolStateAccount.String(),
		nftOwnerAccount.String(),
		types.TradeRaydiumConcentratedLiquidityIncreaseLiquidity,
	)

	// 创建交易对信息
	trade.PairInfo = decoder.createPairInfo(poolStateAccount.String(), baseAccount, tokenAccount)

	// 更新池中的代币数量
	decoder.updatePoolAmounts(trade, account0Info, baseAccount.TokenAddress, tokenAccount.TokenAddress)
	decoder.updatePoolAmounts(trade, account1Info, baseAccount.TokenAddress, tokenAccount.TokenAddress)

	return trade, nil
}

// DecodeRaydiumConcentratedLiquidityDecreaseLiquidityV2 解码减少流动性V2指令
func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityDecreaseLiquidityV2() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	nftOwnerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]
	tokenVault0 := tx.AccountKeys[decoder.compiledInstruction.Accounts[5]]
	tokenVault1 := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]

	// 获取代币账户信息
	account0Info, err := decoder.getTokenAccountInfo(tokenVault0.String())
	if err != nil {
		return nil, err
	}
	account1Info, err := decoder.getTokenAccountInfo(tokenVault1.String())
	if err != nil {
		return nil, err
	}

	// 确定基础代币和交易代币
	baseAccount, tokenAccount := decoder.determineBaseAndTokenAccounts(account0Info, account1Info)

	// 创建基础交易信息
	trade := decoder.createBasicTradeInfo(
		poolStateAccount.String(),
		nftOwnerAccount.String(),
		types.TradeRaydiumConcentratedLiquidityDecreaseLiquidity,
	)

	// 创建交易对信息
	trade.PairInfo = decoder.createPairInfo(poolStateAccount.String(), baseAccount, tokenAccount)

	// 更新池中的代币数量
	decoder.updatePoolAmounts(trade, account0Info, baseAccount.TokenAddress, tokenAccount.TokenAddress)
	decoder.updatePoolAmounts(trade, account1Info, baseAccount.TokenAddress, tokenAccount.TokenAddress)

	return trade, nil
}

func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityOpenPosition() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	payerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]                   // payerAccount
	positionNftOwnerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]]        // positionNftOwnerAccount
	positionNftMintAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]         // positionNftMintAccount
	positionNftAccountAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]      // positionNftAccountAccount
	metadataAccountAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[4]]         // metadataAccountAccount
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]               // poolStateAccount
	protocolPositionAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]]        // protocolPositionAccount
	tickArrayLowerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[8]]          // tickArrayLowerAccount
	tickArrayUpperAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[9]]          // tickArrayUpperAccount
	personalPositionAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[10]]       // personalPositionAccount
	tokenAccount0account := tx.AccountKeys[decoder.compiledInstruction.Accounts[11]]          // tokenAccount0account
	tokenAccount1account := tx.AccountKeys[decoder.compiledInstruction.Accounts[12]]          // tokenAccount1account
	tokenVault0account := tx.AccountKeys[decoder.compiledInstruction.Accounts[13]]            // tokenVault0account
	tokenVault1account := tx.AccountKeys[decoder.compiledInstruction.Accounts[14]]            // tokenVault1account
	rentAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[15]]                   // rentAccount
	systemProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[16]]          // systemProgramAccount
	tokenProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[17]]           // tokenProgramAccount
	associatedTokenProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[18]] // associatedTokenProgramAccount
	metadataProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[19]]        // metadataProgramAccount

	trade := &types.TradeWithPair{}
	trade.ChainId = SolChainId
	trade.TxHash = decoder.dtx.TxHash
	trade.PairAddr = poolStateAccount.String()
	trade.Type = "open_position"
	trade.Maker = payerAccount.String()
	trade.To = poolStateAccount.String()
	trade.Slot = decoder.dtx.BlockDb.Slot
	trade.BlockTime = decoder.dtx.BlockDb.BlockTime.Unix()
	trade.HashId = fmt.Sprintf("%v#%d", decoder.dtx.BlockDb.Slot, decoder.dtx.TxIndex)
	trade.TransactionIndex = decoder.dtx.TxIndex
	trade.SwapName = constants.RaydiumConcentratedLiquidity

	// 解析 OpenPosition 指令参数
	openPositionParams, err := decoder.decodeOpenPositionParams()
	if err != nil {
		return nil, err
	}

	// 组装OpenPosition参数和账户
	trade.CLMMOpenPositionInfo = &types.CLMMOpenPositionInfo{
		TickLowerIndex:           &openPositionParams.TickLowerIndex,
		TickUpperIndex:           &openPositionParams.TickUpperIndex,
		TickArrayLowerStartIndex: &openPositionParams.TickArrayLowerStartIndex,
		TickArrayUpperStartIndex: &openPositionParams.TickArrayUpperStartIndex,
		Liquidity:                &openPositionParams.Liquidity,
		Amount0Max:               &openPositionParams.Amount0Max,
		Amount1Max:               &openPositionParams.Amount1Max,
		Payer:                    payerAccount.String(),
		PositionNftOwner:         positionNftOwnerAccount.String(),
		PositionNftMint:          positionNftMintAccount.String(),
		PositionNftAccount:       positionNftAccountAccount.String(),
		MetadataAccount:          metadataAccountAccount.String(),
		PoolState:                poolStateAccount.String(),
		ProtocolPosition:         protocolPositionAccount.String(),
		TickArrayLower:           tickArrayLowerAccount.String(),
		TickArrayUpper:           tickArrayUpperAccount.String(),
		PersonalPosition:         personalPositionAccount.String(),
		TokenAccount0:            tokenAccount0account.String(),
		TokenAccount1:            tokenAccount1account.String(),
		TokenVault0:              tokenVault0account.String(),
		TokenVault1:              tokenVault1account.String(),
		Rent:                     rentAccount.String(),
		SystemProgram:            systemProgramAccount.String(),
		TokenProgram:             tokenProgramAccount.String(),
		AssociatedTokenProgram:   associatedTokenProgramAccount.String(),
		MetadataProgram:          metadataProgramAccount.String(),
	}

	// 获取代币账户信息
	token0Info := decoder.dtx.TokenAccountMap[tokenAccount0account.String()]
	token1Info := decoder.dtx.TokenAccountMap[tokenAccount1account.String()]

	// 构建交易对信息
	pairInfo := decoder.buildPairInfoForOpenPosition(
		poolStateAccount.String(),
		tokenAccount0account.String(),
		tokenAccount1account.String(),
		token0Info,
		token1Info,
	)
	trade.PairInfo = pairInfo

	trade.ClmmPoolInfoV1 = &types.CLMMPoolInfo{
		PoolState: solTypes.AccountMeta{PubKey: common.PublicKeyFromString(poolStateAccount.String())},
		TickArray: common.PublicKeyFromString(tickArrayUpperAccount.String()),
		RemainingAccounts: []solTypes.AccountMeta{
			{
				PubKey:     common.PublicKeyFromString(tickArrayLowerAccount.String()),
				IsSigner:   false,
				IsWritable: false,
			},
		},
	}

	return trade, nil
}

// DecodeRaydiumConcentratedLiquidityCreatePool 解码创建池指令
func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityCreatePool() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	poolCreatorAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]
	ammConfigAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]]
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]
	tokenMint0 := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]
	tokenMint1 := tx.AccountKeys[decoder.compiledInstruction.Accounts[4]]
	inputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[5]]
	outputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]
	observationStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]]
	tickArrayBitmapAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[8]]

	// 确定基础代币和交易代币
	baseTokenAddr, tokenAddr := decoder.determineBaseAndTokenMints(tokenMint0.String(), tokenMint1.String())

	// 创建基础交易信息
	trade := decoder.createBasicTradeInfo(
		poolStateAccount.String(),
		poolCreatorAccount.String(),
		types.TradeRaydiumConcentratedLiquidityCreatePool,
	)

	// 设置交易对信息
	trade.PairInfo = types.Pair{
		ChainId:       SolChainId,
		Addr:          poolStateAccount.String(),
		BaseTokenAddr: baseTokenAddr,
		TokenAddr:     tokenAddr,
		BlockTime:     decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:      decoder.dtx.BlockDb.Slot,
		Name:          constants.RaydiumConcentratedLiquidity,
	}

	// Create and populate ClmmPoolInfoV1
	accountMetas := slice.Map[int, solTypes.AccountMeta](decoder.compiledInstruction.Accounts, func(_ int, index int) solTypes.AccountMeta {
		return solTypes.AccountMeta{
			PubKey:     tx.AccountKeys[index],
			IsSigner:   false,
			IsWritable: false,
		}
	})

	// Initialize ClmmPoolInfoV1 with data from createPool
	clmmInfo := &types.CLMMPoolInfo{
		AmmConfig:         ammConfigAccount,
		PoolState:         solTypes.AccountMeta{PubKey: poolStateAccount},
		InputVault:        solTypes.AccountMeta{PubKey: inputVaultAccount},
		OutputVault:       solTypes.AccountMeta{PubKey: outputVaultAccount},
		ObservationState:  solTypes.AccountMeta{PubKey: observationStateAccount},
		TokenProgram:      common.TokenProgramID,
		TokenProgram2022:  common.Token2022ProgramID,
		MemoProgram:       common.MemoProgramID,
		InputVaultMint:    common.PublicKeyFromString(baseTokenAddr),
		OutputVaultMint:   common.PublicKeyFromString(tokenAddr),
		RemainingAccounts: accountMetas,
		TxHash:            decoder.dtx.TxHash,
		TickArray:         tickArrayBitmapAccount,
	}

	// Process AMM config to get the trade fee rate
	solClient := decoder.svcCtx.GetSolClient()
	if solClient != nil {
		accountInfo, err := solClient.GetAccountInfoWithConfig(decoder.ctx, clmmInfo.AmmConfig.String(), client.GetAccountInfoConfig{
			Commitment: rpc.CommitmentConfirmed,
		})
		if err == nil {
			ammConfig := amm_v3.AmmConfig{}
			if err := ammConfig.UnmarshalWithDecoder(bin.NewBorshDecoder(accountInfo.Data)); err == nil {
				clmmInfo.TradeFeeRate = ammConfig.TradeFeeRate
			}
		}
	}

	// Set the ClmmPoolInfoV1 field in the trade object
	trade.ClmmPoolInfoV1 = clmmInfo
	return trade, nil
}

func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquiditySwapV2() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	ammConfigAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]]        // ammConfigAccount
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]        // poolStateAccount
	inputTokenAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]       // inputTokenAccountAccount
	outputTokenAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[4]]      // outputTokenAccountAccount
	inputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[5]]       // inputVaultAccount
	outputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]      // outputVaultAccount
	observationStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]] // observationStateAccount

	tokenSwap, err := decoder.decodeTokenSwap(inputTokenAccount, outputTokenAccount)
	if err != nil {
		return nil, err
	}

	// 构建基础交易信息
	trade, err := decoder.buildBaseTrade(poolStateAccount, inputTokenAccount, tokenSwap)
	if err != nil {
		return nil, err
	}

	// 更新池中的代币数量
	decoder.updatePoolTokenAmounts(trade, inputVaultAccount, outputVaultAccount, tokenSwap)

	// 构建 V2 CLMM 信息
	accountMetas := decoder.buildAccountMetas()
	if len(accountMetas) > 12 {
		clmmInfo, err := decoder.buildCLMMInfoV2(
			ammConfigAccount,
			poolStateAccount,
			inputVaultAccount,
			outputVaultAccount,
			observationStateAccount,
			accountMetas[13:],
			tokenSwap,
		)
		if err != nil {
			return nil, err
		}
		trade.ClmmPoolInfoV2 = clmmInfo
	}

	return trade, nil
}

// decodeTokenSwap 统一的代币交换解析方法，适用于V1和V2
func (decoder *ConcentratedLiquidityDecoder) decodeTokenSwap(inputTokenAccount, outputTokenAccount common.PublicKey) (swap *Swap, err error) {
	var fromTransfer *token.TransferParam
	var toTransfer *token.TransferParam

	var (
		accountKeys = decoder.dtx.Tx.AccountKeys
	)

	swap = &Swap{}

	fromTokenAccountInfo := decoder.dtx.TokenAccountMap[inputTokenAccount.String()]
	if fromTokenAccountInfo == nil {
		err = fmt.Errorf("fromTokenAccountInfo not found,tx hash: %v", decoder.dtx.TxHash)
		return
	}
	toTokenAccountInfo := decoder.dtx.TokenAccountMap[outputTokenAccount.String()]
	if toTokenAccountInfo == nil {
		err = fmt.Errorf("toTokenAccountInfo not found,tx hash: %v", decoder.dtx.TxHash)
		return
	}

	if decoder.innerInstruction == nil {
		err = fmt.Errorf("innerInstruction not found,tx hash: %v", decoder.dtx.TxHash)
		return
	}

	for _, innerInstruction := range decoder.innerInstruction.Instructions {
		transfer, err := DecodeTokenTransfer(accountKeys, &innerInstruction)
		if err != nil {
			continue
		}
		if transfer.From.String() == inputTokenAccount.String() {
			fromTransfer = transfer
		} else if transfer.To.String() == outputTokenAccount.String() {
			toTransfer = transfer
		}
	}
	if fromTransfer == nil {
		err = errors.New("fromTransfer not found ")
		return
	}
	if toTransfer == nil {
		err = errors.New("toTransfer not found ")
		return
	}
	if !IsSwapTransfer(fromTransfer, toTransfer, decoder.dtx.TokenAccountMap) {
		err = errors.New("not swap transfer")
		return
	}
	logx.Infof("decodeTokenSwap: tx=%v, fromToken=%v, toToken=%v, TokenStrWrapSol=%v",
		decoder.dtx.TxHash, fromTokenAccountInfo.TokenAddress, toTokenAccountInfo.TokenAddress, TokenStrWrapSol)

	// 确定交易类型和代币角色
	decoder.populateSwapInfo(swap, fromTokenAccountInfo, toTokenAccountInfo, fromTransfer, toTransfer)
	return
}

// populateSwapInfo 填充交换信息
func (decoder *ConcentratedLiquidityDecoder) populateSwapInfo(
	swap *Swap,
	fromTokenInfo, toTokenInfo *TokenAccount,
	fromTransfer, toTransfer *token.TransferParam,
) {
	// 判断是买入还是卖出,并确定基础代币
	var isBuy bool
	var baseTokenInfo, tradeTokenInfo *TokenAccount
	var baseTransfer, tradeTransfer *token.TransferParam
	var ownerAddr string

	if fromTokenInfo.TokenAddress == TokenStrWrapSol {
		// WSOL -> Token (买入场景)
		isBuy = true
		baseTokenInfo, tradeTokenInfo = fromTokenInfo, toTokenInfo
		baseTransfer, tradeTransfer = fromTransfer, toTransfer
		ownerAddr = toTokenInfo.Owner
	} else if toTokenInfo.TokenAddress == TokenStrWrapSol {
		// Token -> WSOL (卖出场景)
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	} else if decoder.isStableCoin(fromTokenInfo.TokenAddress) {
		// from 是稳定币,to 是其他代币 -> 买入场景
		logx.Infof("非 WSOL 代币对交换(稳定币买入): tx=%v, fromToken=%v, toToken=%v",
			decoder.dtx.TxHash, fromTokenInfo.TokenAddress, toTokenInfo.TokenAddress)
		isBuy = true
		baseTokenInfo, tradeTokenInfo = fromTokenInfo, toTokenInfo
		baseTransfer, tradeTransfer = fromTransfer, toTransfer
		ownerAddr = toTokenInfo.Owner
	} else if decoder.isStableCoin(toTokenInfo.TokenAddress) {
		// to 是稳定币,from 是其他代币 -> 卖出场景
		logx.Infof("非 WSOL 代币对交换(稳定币卖出): tx=%v, fromToken=%v, toToken=%v",
			decoder.dtx.TxHash, fromTokenInfo.TokenAddress, toTokenInfo.TokenAddress)
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	} else {
		// 两个都不是稳定币,默认将 to 作为基础代币,按卖出处理
		logx.Infof("非 WSOL 代币对交换(默认): tx=%v, fromToken=%v, toToken=%v",
			decoder.dtx.TxHash, fromTokenInfo.TokenAddress, toTokenInfo.TokenAddress)
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	}

	// 填充交换信息
	swap.BaseTokenInfo = baseTokenInfo
	swap.TokenInfo = tradeTokenInfo
	if isBuy {
		swap.Type = types.TradeTypeBuy
	} else {
		swap.Type = types.TradeTypeSell
	}
	swap.BaseTokenAmountInt = int64(baseTransfer.Amount)
	swap.BaseTokenAmount = float64(baseTransfer.Amount) / math.Pow10(int(baseTokenInfo.TokenDecimal))
	swap.TokenAmountInt = int64(tradeTransfer.Amount)
	swap.TokenAmount = float64(tradeTransfer.Amount) / math.Pow10(int(tradeTokenInfo.TokenDecimal))
	swap.To = ownerAddr
}

// isStableCoin 判断是否为稳定币
func (decoder *ConcentratedLiquidityDecoder) isStableCoin(tokenAddress string) bool {
	return tokenAddress == TokenStrUSDC || tokenAddress == TokenStrUSDT
}

func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquiditySwap() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	ammConfigAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]]        // ammConfigAccount
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]        // poolStateAccount
	inputTokenAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]       // inputTokenAccountAccount
	outputTokenAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[4]]      // outputTokenAccountAccount
	inputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[5]]       // inputVaultAccount
	outputVaultAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]      // outputVaultAccount
	observationStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]] // observationStateAccount
	tickArrayAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[9]]        // tickArrayAccount

	tokenSwap, err := decoder.decodeTokenSwap(inputTokenAccount, outputTokenAccount)
	if err != nil {
		return nil, err
	}

	// 构建基础交易信息
	trade, err := decoder.buildBaseTrade(poolStateAccount, inputTokenAccount, tokenSwap)
	if err != nil {
		return nil, err
	}

	// 更新池中的代币数量 (V1使用inputTokenAccount和outputTokenAccount而不是vault accounts)
	decoder.updatePoolTokenAmounts(trade, inputTokenAccount, outputTokenAccount, tokenSwap)

	// 构建 V1 CLMM 信息
	accountMetas := decoder.buildAccountMetas()
	if len(accountMetas) > 9 {
		clmmInfo, err := decoder.buildCLMMInfoV1(
			ammConfigAccount,
			poolStateAccount,
			inputVaultAccount,
			outputVaultAccount,
			observationStateAccount,
			tickArrayAccount,
			accountMetas[10:],
			tokenSwap,
		)
		if err != nil {
			return nil, err
		}
		trade.ClmmPoolInfoV1 = clmmInfo
		logx.Infof("decoder clmmInfo v1 tx hash: %v, clmm id: %v", decoder.dtx.TxHash, clmmInfo.PoolState.PubKey.String())
	}

	return trade, nil
}

// buildBaseTrade 构建基础交易信息
func (decoder *ConcentratedLiquidityDecoder) buildBaseTrade(poolStateAccount, inputTokenAccount common.PublicKey, tokenSwap *Swap) (*types.TradeWithPair, error) {
	// 验证 TokenAmount 不能为零
	if tokenSwap.TokenAmount == 0 {
		return nil, fmt.Errorf("trade.TokenAmount is zero, tx:%v", decoder.dtx.TxHash)
	}

	trade := &types.TradeWithPair{}
	trade.ChainId = SolChainId
	trade.TxHash = decoder.dtx.TxHash
	trade.PairAddr = poolStateAccount.String()

	trade.PairInfo = types.Pair{
		ChainId:          SolChainId,
		Addr:             poolStateAccount.String(),
		BaseTokenAddr:    tokenSwap.BaseTokenInfo.TokenAddress,
		BaseTokenDecimal: tokenSwap.BaseTokenInfo.TokenDecimal,
		BaseTokenSymbol:  util.GetBaseToken(SolChainIdInt).Symbol,
		TokenAddr:        tokenSwap.TokenInfo.TokenAddress,
		TokenDecimal:     tokenSwap.TokenInfo.TokenDecimal,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:         decoder.dtx.BlockDb.Slot,
	}

	trade.Maker = inputTokenAccount.String()
	trade.Type = tokenSwap.Type
	trade.BaseTokenAmount = tokenSwap.BaseTokenAmount
	trade.TokenAmount = tokenSwap.TokenAmount
	trade.BaseTokenPriceUSD = decoder.dtx.SolPrice
	trade.TotalUSD = decimal.NewFromFloat(tokenSwap.BaseTokenAmount).Mul(decimal.NewFromFloat(decoder.dtx.SolPrice)).InexactFloat64()
	trade.TokenPriceUSD = decimal.NewFromFloat(trade.TotalUSD).Div(decimal.NewFromFloat(tokenSwap.TokenAmount)).InexactFloat64()

	trade.To = tokenSwap.To
	trade.Slot = decoder.dtx.BlockDb.Slot
	trade.BlockTime = decoder.dtx.BlockDb.BlockTime.Unix()
	trade.HashId = fmt.Sprintf("%v#%d", decoder.dtx.BlockDb.Slot, decoder.dtx.TxIndex)
	trade.TransactionIndex = decoder.dtx.TxIndex
	trade.SwapName = constants.RaydiumConcentratedLiquidity
	trade.PairInfo.Name = trade.SwapName
	trade.BaseTokenAccountAddress = tokenSwap.BaseTokenInfo.TokenAccountAddress
	trade.TokenAccountAddress = tokenSwap.TokenInfo.TokenAccountAddress
	trade.BaseTokenAmountInt = tokenSwap.BaseTokenAmountInt
	trade.TokenAmountInt = tokenSwap.TokenAmountInt

	return trade, nil
}

// updatePoolTokenAmounts 更新池中的代币数量
func (decoder *ConcentratedLiquidityDecoder) updatePoolTokenAmounts(trade *types.TradeWithPair, account1, account2 common.PublicKey, tokenSwap *Swap) {
	if account1 != (common.PublicKey{}) {
		poolTokenAccount := decoder.dtx.TokenAccountMap[account1.String()]
		if poolTokenAccount != nil {
			if poolTokenAccount.TokenAddress == tokenSwap.BaseTokenInfo.TokenAddress {
				trade.CurrentBaseTokenInPoolAmount = decimal.New(poolTokenAccount.PostValue, -int32(poolTokenAccount.TokenDecimal)).InexactFloat64()
			} else if poolTokenAccount.TokenAddress == tokenSwap.TokenInfo.TokenAddress {
				trade.CurrentTokenInPoolAmount = decimal.New(poolTokenAccount.PostValue, -int32(poolTokenAccount.TokenDecimal)).InexactFloat64()
			}
		}
	}

	if account2 != (common.PublicKey{}) {
		poolTokenAccount := decoder.dtx.TokenAccountMap[account2.String()]
		if poolTokenAccount != nil {
			if poolTokenAccount.TokenAddress == tokenSwap.BaseTokenInfo.TokenAddress {
				trade.CurrentBaseTokenInPoolAmount = decimal.New(poolTokenAccount.PostValue, -int32(poolTokenAccount.TokenDecimal)).InexactFloat64()
			} else if poolTokenAccount.TokenAddress == tokenSwap.TokenInfo.TokenAddress {
				trade.CurrentTokenInPoolAmount = decimal.New(poolTokenAccount.PostValue, -int32(poolTokenAccount.TokenDecimal)).InexactFloat64()
			}
		}
	}

	trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
	trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount
}

// buildAccountMetas 构建账户元数据列表
func (decoder *ConcentratedLiquidityDecoder) buildAccountMetas() []solTypes.AccountMeta {
	tx := decoder.dtx.Tx
	return slice.Map[int, solTypes.AccountMeta](decoder.compiledInstruction.Accounts, func(_ int, index int) solTypes.AccountMeta {
		return solTypes.AccountMeta{
			PubKey:     tx.AccountKeys[index],
			IsSigner:   false,
			IsWritable: false,
		}
	})
}

// parseTradeFeeRate 解析交易费率
func (decoder *ConcentratedLiquidityDecoder) parseTradeFeeRate(ammConfigAccount common.PublicKey) (uint32, error) {
	solClient := decoder.svcCtx.GetSolClient()
	accountInfo, err := solClient.GetAccountInfoWithConfig(decoder.ctx, ammConfigAccount.String(), client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return 0, err
	}
	ammConfig := amm_v3.AmmConfig{}
	if err := ammConfig.UnmarshalWithDecoder(bin.NewBorshDecoder(accountInfo.Data)); err != nil {
		return 0, err
	}
	return ammConfig.TradeFeeRate, nil
}

// buildCLMMInfoV1 构建 V1 版本的 CLMM 池信息
func (decoder *ConcentratedLiquidityDecoder) buildCLMMInfoV1(
	ammConfigAccount, poolStateAccount, inputVaultAccount, outputVaultAccount, observationStateAccount, tickArrayAccount common.PublicKey,
	remainingAccounts []solTypes.AccountMeta,
	tokenSwap *Swap,
) (*types.CLMMPoolInfo, error) {
	clmmInfo := &types.CLMMPoolInfo{
		AmmConfig:         ammConfigAccount,
		PoolState:         solTypes.AccountMeta{PubKey: poolStateAccount},
		InputVault:        solTypes.AccountMeta{PubKey: inputVaultAccount},
		OutputVault:       solTypes.AccountMeta{PubKey: outputVaultAccount},
		ObservationState:  solTypes.AccountMeta{PubKey: observationStateAccount},
		TokenProgram:      common.TokenProgramID,
		TokenProgram2022:  common.Token2022ProgramID,
		MemoProgram:       common.MemoProgramID,
		TickArray:         tickArrayAccount,
		InputVaultMint:    common.PublicKeyFromString(tokenSwap.BaseTokenInfo.TokenAddress),
		OutputVaultMint:   common.PublicKeyFromString(tokenSwap.TokenInfo.TokenAddress),
		RemainingAccounts: remainingAccounts,
		TxHash:            decoder.dtx.TxHash,
	}

	// 如果是卖单 数据库默认解析是买单
	if tokenSwap.Type == types.TradeTypeSell {
		clmmInfo.InputVault, clmmInfo.OutputVault = clmmInfo.OutputVault, clmmInfo.InputVault
	}

	// 解析费率
	tradeFeeRate, err := decoder.parseTradeFeeRate(ammConfigAccount)
	if err != nil {
		return nil, err
	}
	clmmInfo.TradeFeeRate = tradeFeeRate

	return clmmInfo, nil
}

// buildCLMMInfoV2 构建 V2 版本的 CLMM 池信息
func (decoder *ConcentratedLiquidityDecoder) buildCLMMInfoV2(
	ammConfigAccount, poolStateAccount, inputVaultAccount, outputVaultAccount, observationStateAccount common.PublicKey,
	remainingAccounts []solTypes.AccountMeta,
	tokenSwap *Swap,
) (*types.CLMMPoolInfo, error) {
	clmmInfo := &types.CLMMPoolInfo{
		AmmConfig:         ammConfigAccount,
		PoolState:         solTypes.AccountMeta{PubKey: poolStateAccount},
		InputVault:        solTypes.AccountMeta{PubKey: inputVaultAccount},
		OutputVault:       solTypes.AccountMeta{PubKey: outputVaultAccount},
		ObservationState:  solTypes.AccountMeta{PubKey: observationStateAccount},
		TokenProgram:      common.TokenProgramID,
		TokenProgram2022:  common.Token2022ProgramID,
		MemoProgram:       common.MemoProgramID,
		InputVaultMint:    common.PublicKeyFromString(tokenSwap.BaseTokenInfo.TokenAddress),
		OutputVaultMint:   common.PublicKeyFromString(tokenSwap.TokenInfo.TokenAddress),
		RemainingAccounts: remainingAccounts,
		TxHash:            decoder.dtx.TxHash,
	}

	// 如果是卖单 数据库默认解析是买单
	if tokenSwap.Type == types.TradeTypeSell {
		clmmInfo.InputVault, clmmInfo.OutputVault = clmmInfo.OutputVault, clmmInfo.InputVault
		clmmInfo.InputVaultMint, clmmInfo.OutputVaultMint = clmmInfo.OutputVaultMint, clmmInfo.InputVaultMint
	}

	// 解析费率
	tradeFeeRate, err := decoder.parseTradeFeeRate(ammConfigAccount)
	if err != nil {
		return nil, err
	}
	clmmInfo.TradeFeeRate = tradeFeeRate

	return clmmInfo, nil
}
