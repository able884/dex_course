package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"time"

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
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/pkg/types"
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
		BaseTokenSymbol:  baseAccount.TokenSymbol, // 使用从数据库获取的 base token symbol
		TokenAddr:        tokenAccount.TokenAddress,
		TokenSymbol:      tokenAccount.TokenSymbol, // 使用从数据库获取的 token symbol
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
		baseTokenSymbol = token0Info.TokenSymbol
		baseTokenDecimal = token0Info.TokenDecimal
		if token1Info != nil {
			tokenAddr = token1Info.TokenAddress
			tokenSymbol = token1Info.TokenSymbol
			tokenDecimal = token1Info.TokenDecimal
		} else {
			tokenAddr = tokenAccount1
			tokenSymbol = ""
			tokenDecimal = defaultTokenDecimal
		}
	} else if token1Info != nil && token1Info.TokenAddress == TokenStrWrapSol {
		// token1 是 WSOL,作为基础代币
		baseTokenAddr = token1Info.TokenAddress
		baseTokenSymbol = token1Info.TokenSymbol
		baseTokenDecimal = token1Info.TokenDecimal
		if token0Info != nil {
			tokenAddr = token0Info.TokenAddress
			tokenSymbol = token0Info.TokenSymbol
			tokenDecimal = token0Info.TokenDecimal
		} else {
			tokenAddr = tokenAccount0
			tokenSymbol = ""
			tokenDecimal = defaultTokenDecimal
		}
	} else if token0Info != nil && token1Info != nil {
		// 两个都不是 WSOL,默认 token0 作为基础代币
		baseTokenAddr = token0Info.TokenAddress
		baseTokenSymbol = token0Info.TokenSymbol
		baseTokenDecimal = token0Info.TokenDecimal
		tokenAddr = token1Info.TokenAddress
		tokenSymbol = token1Info.TokenSymbol
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
	} else if bytes.Equal(discriminator, amm_v3.Instruction_OpenPosition[:]) || bytes.Equal(discriminator, amm_v3.Instruction_OpenPositionWithToken22Nft[:]) {
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
	nftAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]] // NFT TokenAccount
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]
	tokenAccount0 := tx.AccountKeys[decoder.compiledInstruction.Accounts[7]]
	tokenAccount1 := tx.AccountKeys[decoder.compiledInstruction.Accounts[8]]

	// 从 NFT 账户获取 position_nft_mint
	positionNftMint := ""
	nftAccountInfo, err := decoder.getTokenAccountInfo(nftAccount.String())
	if err == nil && nftAccountInfo != nil {
		positionNftMint = nftAccountInfo.TokenAddress
	}

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

	// 将 position_nft_mint 存储到扩展字段中（如果 TradeWithPair 有相关字段）
	// 或者创建一个新的 CLMMLiquidityChangeInfo 结构
	if positionNftMint != "" {
		// 使用 CLMMOpenPositionInfo 作为临时存储（虽然字段不完全匹配，但可以存储 position_nft_mint）
		if trade.CLMMOpenPositionInfo == nil {
			trade.CLMMOpenPositionInfo = &types.CLMMOpenPositionInfo{}
		}
		trade.CLMMOpenPositionInfo.PositionNftMint = positionNftMint
	}

	return trade, nil
}

// DecodeRaydiumConcentratedLiquidityDecreaseLiquidityV2 解码减少流动性V2指令
func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityDecreaseLiquidityV2() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	nftOwnerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]
	nftAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]] // NFT TokenAccount
	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]]
	tokenVault0 := tx.AccountKeys[decoder.compiledInstruction.Accounts[5]]
	tokenVault1 := tx.AccountKeys[decoder.compiledInstruction.Accounts[6]]

	// 从 NFT 账户获取 position_nft_mint
	positionNftMint := ""
	nftAccountInfo, err := decoder.getTokenAccountInfo(nftAccount.String())
	if err == nil && nftAccountInfo != nil {
		positionNftMint = nftAccountInfo.TokenAddress
	}

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

	// 将 position_nft_mint 存储到扩展字段中
	if positionNftMint != "" {
		if trade.CLMMOpenPositionInfo == nil {
			trade.CLMMOpenPositionInfo = &types.CLMMOpenPositionInfo{}
		}
		trade.CLMMOpenPositionInfo.PositionNftMint = positionNftMint
	}

	return trade, nil
}

func (decoder *ConcentratedLiquidityDecoder) DecodeRaydiumConcentratedLiquidityOpenPosition() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	discriminator := GetInstructionDiscriminator(decoder.compiledInstruction.Data)

	// 判断是指令类型：OpenPosition 还是 OpenPositionWithToken22Nft
	isToken22Nft := bytes.Equal(discriminator, amm_v3.Instruction_OpenPositionWithToken22Nft[:])

	// 根据指令类型确定账户索引
	// OpenPosition: 有 metadata_account (索引4), 共19个账户
	// OpenPositionWithToken22Nft: 没有 metadata_account, 共20个账户，索引前移
	var (
		poolStateIdx, protocolPositionIdx, tickArrayLowerIdx, tickArrayUpperIdx int
		personalPositionIdx, tokenAccount0Idx, tokenAccount1Idx                 int
		tokenVault0Idx, tokenVault1Idx, rentIdx, systemProgramIdx               int
		tokenProgramIdx, associatedTokenProgramIdx                              int
		metadataAccountIdx, metadataProgramIdx                                  int
	)

	if isToken22Nft {
		// OpenPositionWithToken22Nft 账户索引（没有 metadata_account）
		// 账户顺序: [0]payer, [1]position_nft_owner, [2]position_nft_mint, [3]position_nft_account,
		// [4]pool_state, [5]protocol_position, [6]tick_array_lower, [7]tick_array_upper,
		// [8]personal_position, [9]token_account_0, [10]token_account_1, [11]token_vault_0,
		// [12]token_vault_1, [13]rent, [14]system_program, [15]token_program,
		// [16]associated_token_program, [17]token_program_2022, [18]vault_0_mint, [19]vault_1_mint
		poolStateIdx = 4
		protocolPositionIdx = 5
		tickArrayLowerIdx = 6
		tickArrayUpperIdx = 7
		personalPositionIdx = 8
		tokenAccount0Idx = 9
		tokenAccount1Idx = 10
		tokenVault0Idx = 11
		tokenVault1Idx = 12
		rentIdx = 13
		systemProgramIdx = 14
		tokenProgramIdx = 15
		associatedTokenProgramIdx = 16
		// Token22Nft 没有 metadata 相关账户
		metadataAccountIdx = -1
		metadataProgramIdx = -1
	} else {
		// OpenPosition 账户索引（有 metadata_account）
		// 账户顺序: [0]payer, [1]position_nft_owner, [2]position_nft_mint, [3]position_nft_account,
		// [4]metadata_account, [5]pool_state, [6]protocol_position, [7]tick_array_lower,
		// [8]tick_array_upper, [9]personal_position, [10]token_account_0, [11]token_account_1,
		// [12]token_vault_0, [13]token_vault_1, [14]rent, [15]system_program, [16]token_program,
		// [17]associated_token_program, [18]metadata_program
		poolStateIdx = 5
		protocolPositionIdx = 6
		tickArrayLowerIdx = 7
		tickArrayUpperIdx = 8
		personalPositionIdx = 9
		tokenAccount0Idx = 10
		tokenAccount1Idx = 11
		tokenVault0Idx = 12
		tokenVault1Idx = 13
		rentIdx = 14
		systemProgramIdx = 15
		tokenProgramIdx = 16
		associatedTokenProgramIdx = 17
		metadataAccountIdx = 4
		metadataProgramIdx = 18
	}

	// 公共账户（两种指令类型都相同）
	payerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[0]]              // payerAccount
	positionNftOwnerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[1]]   // positionNftOwnerAccount
	positionNftMintAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[2]]    // positionNftMintAccount
	positionNftAccountAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[3]] // positionNftAccountAccount

	// 根据指令类型获取账户
	var metadataAccountAccount common.PublicKey
	if metadataAccountIdx >= 0 {
		metadataAccountAccount = tx.AccountKeys[decoder.compiledInstruction.Accounts[metadataAccountIdx]]
	}

	poolStateAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[poolStateIdx]]
	protocolPositionAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[protocolPositionIdx]]
	tickArrayLowerAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[tickArrayLowerIdx]]
	tickArrayUpperAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[tickArrayUpperIdx]]
	personalPositionAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[personalPositionIdx]]
	tokenAccount0account := tx.AccountKeys[decoder.compiledInstruction.Accounts[tokenAccount0Idx]]
	tokenAccount1account := tx.AccountKeys[decoder.compiledInstruction.Accounts[tokenAccount1Idx]]
	tokenVault0account := tx.AccountKeys[decoder.compiledInstruction.Accounts[tokenVault0Idx]]
	tokenVault1account := tx.AccountKeys[decoder.compiledInstruction.Accounts[tokenVault1Idx]]
	rentAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[rentIdx]]
	systemProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[systemProgramIdx]]
	tokenProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[tokenProgramIdx]]
	associatedTokenProgramAccount := tx.AccountKeys[decoder.compiledInstruction.Accounts[associatedTokenProgramIdx]]

	var metadataProgramAccount common.PublicKey
	if metadataProgramIdx >= 0 {
		metadataProgramAccount = tx.AccountKeys[decoder.compiledInstruction.Accounts[metadataProgramIdx]]
	}

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
	}

	// 根据指令类型设置可选字段
	if metadataAccountIdx >= 0 {
		trade.CLMMOpenPositionInfo.MetadataAccount = metadataAccountAccount.String()
	}
	if metadataProgramIdx >= 0 {
		trade.CLMMOpenPositionInfo.MetadataProgram = metadataProgramAccount.String()
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
	if tradeFeeRate, err := decoder.parseTradeFeeRate(clmmInfo.AmmConfig); err == nil {
		clmmInfo.TradeFeeRate = tradeFeeRate
	} else {
		logx.Errorf("decode createPool clmm: parseTradeFeeRate err: %v, ammConfig=%s", err, clmmInfo.AmmConfig.String())
	}

	// Set the ClmmPoolInfoV1 field in the trade object
	trade.ClmmPoolInfoV1 = clmmInfo

	// 根据基础代币和交易代币地址，正确填充池中代币数量和代币精度
	decoder.fillVaultBalances(trade, inputVaultAccount.String(), outputVaultAccount.String(), baseTokenAddr, tokenAddr)

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

	// 构造swap对象，包含输入输出token信息和本次交易的amount
	tokenSwap, err := decoder.decodeTokenSwap(inputTokenAccount, outputTokenAccount)
	if err != nil {
		return nil, err
	}

	// 构建基础trade，这里面会设置tokenUsdPrice
	trade, err := decoder.buildBaseTrade(poolStateAccount, inputTokenAccount, tokenSwap)
	if err != nil {
		return nil, err
	}

	// 更新池中的基础代币和交易代币数量
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

// 构造swap对象，包含输入输出token信息和本次交易的amount
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
		// 解析代币转账指令，找到 fromTransfer 和 toTransfer
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
	// 验证是否为交换指令（确保 fromTransfer 和 toTransfer 是对应的交换关系）
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

	// 构造swap对象，包含输入输出token信息和本次交易的amount
	tokenSwap, err := decoder.decodeTokenSwap(inputTokenAccount, outputTokenAccount)
	if err != nil {
		return nil, err
	}

	// 构建基础trade，这里面会设置tokenUsdPrice
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

// fetchClmmPriceFromChain 从链上获取 CLMM 池子的价格信息
// 使用 price = (sqrt_price_x64 / 2^64)^2 * (10^decimals0) / (10^decimals1) 计算价格
// 如果 inputMint 是 token1 则反转价格
func (decoder *ConcentratedLiquidityDecoder) fetchClmmPriceFromChain(poolState common.PublicKey, inputMint, outputMint string) (float64, error) {
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		return 0, errors.New("solana rpc client not configured")
	}

	ctx, cancel := context.WithTimeout(decoder.ctx, 5*time.Second)
	defer cancel()

	// 从链上获取 PoolState 账户数据
	accountInfo, err := cli.GetAccountInfo(ctx, poolState.String())
	if err != nil {
		return 0, fmt.Errorf("failed to get pool state account: %w", err)
	}
	if len(accountInfo.Data) == 0 {
		return 0, errors.New("pool state account not found or empty")
	}

	// 解析 PoolState 账户数据
	data := accountInfo.Data
	dec := bin.NewBorshDecoder(data)
	var poolStateAccount amm_v3.PoolStateAccount
	if err := poolStateAccount.UnmarshalWithDecoder(dec); err != nil {
		return 0, fmt.Errorf("failed to decode pool state: %w", err)
	}

	// 获取 sqrt_price_x64
	sqrtPriceX64 := poolStateAccount.SqrtPriceX64
	poolTokenMint0 := poolStateAccount.TokenMint0.String()
	poolTokenMint1 := poolStateAccount.TokenMint1.String()
	decimals0 := int64(poolStateAccount.MintDecimals0)
	decimals1 := int64(poolStateAccount.MintDecimals1)

	// 使用公共函数计算价格 price = (sqrt_price_x64 / 2^64)^2 * (10^decimals0) / (10^decimals1)
	// 如果 inputMint 是 token1 则反转价格
	price, err := clmm.CalculatePriceFromSqrtPriceX64(
		clmm.Uint128{
			Lo: sqrtPriceX64.Lo,
			Hi: sqrtPriceX64.Hi,
		},
		decimals0,
		decimals1,
		poolTokenMint0,
		poolTokenMint1,
		inputMint,
		outputMint,
	)
	if err != nil {
		return 0, err
	}

	// 如果 mint 地址不匹配，记录警告
	if inputMint != poolTokenMint0 && inputMint != poolTokenMint1 {
		logx.Errorf("fetchClmmPriceFromChain: mint addresses don't match. inputMint=%s, outputMint=%s, poolTokenMint0=%s, poolTokenMint1=%s",
			inputMint, outputMint, poolTokenMint0, poolTokenMint1)
	}

	return price, nil
}

// buildBaseTrade 构建基础交易信息，设置tokenUsdPrice
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
		BaseTokenSymbol:  tokenSwap.BaseTokenInfo.TokenSymbol, // 使用从数据库获取的 base token symbol
		TokenAddr:        tokenSwap.TokenInfo.TokenAddress,
		TokenSymbol:      tokenSwap.TokenInfo.TokenSymbol, // 使用从数据库获取的 token symbol
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

	// 对于 CLMM 池子，从链上读取 sqrt_price_x64 计算价格
	// 如果从链上读取失败，回退到使用交易金额计算价格
	clmmPrice, err := decoder.fetchClmmPriceFromChain(poolStateAccount, tokenSwap.BaseTokenInfo.TokenAddress, tokenSwap.TokenInfo.TokenAddress)
	if err != nil {
		logx.Errorf("buildBaseTrade: fetchClmmPriceFromChain failed, poolState:%s err:%v, falling back to trade amount calculation", poolStateAccount.String(), err)
		// 回退到使用交易金额计算价格
		if tokenSwap.TokenAmount != 0 {
			trade.TokenPriceUSD = decimal.NewFromFloat(trade.TotalUSD).Div(decimal.NewFromFloat(tokenSwap.TokenAmount)).InexactFloat64()
		} else {
			trade.TokenPriceUSD = 0
		}
	} else {
		// 使用从链上读取的价格计算 TokenPriceUSD
		// clmmPrice 是 outputMint/inputMint 的价格，即 token/base 的价格
		// TokenPriceUSD = clmmPrice * BaseTokenPriceUSD
		trade.TokenPriceUSD = clmmPrice * decoder.dtx.SolPrice
	}

	logx.Infof("buildBaseTrade CLMM: tx=%v, type=%s, baseAmount=%f, tokenAmount=%f, solPrice=%f, totalUSD=%f, tokenPriceUSD=%f",
		decoder.dtx.TxHash, trade.Type, tokenSwap.BaseTokenAmount, tokenSwap.TokenAmount, decoder.dtx.SolPrice, trade.TotalUSD, trade.TokenPriceUSD)

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

// 更新池子中的代币数量
// CurrentBaseTokenInPoolAmount = 基础代币账户的余额 / 10^基础代币精度
// CurrentTokenInPoolAmount数量 = 代币账户的余额 / 10^代币精度
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
	if len(accountInfo.Data) == 0 {
		return 0, fmt.Errorf("empty amm_config account data: %s", ammConfigAccount.String())
	}
	data := accountInfo.Data
	if len(data) >= 8 {
		data = data[8:] // skip Anchor discriminator
	}
	ammConfig := amm_v3.AmmConfig{}
	if err := ammConfig.UnmarshalWithDecoder(bin.NewBorshDecoder(data)); err != nil {
		return 0, err
	}
	tradeFee := ammConfig.TradeFeeRate
	if tradeFee > 1_000_000 { // trade fee is in 1e-6 units; anything larger is likely bad data
		logx.Errorf("clmm parseTradeFeeRate: suspicious trade_fee_rate=%d for amm_config=%s tx=%s, clamping to sane range",
			tradeFee, ammConfigAccount.String(), decoder.dtx.TxHash)
		tradeFee = tradeFee % 1_000_000
	}
	return tradeFee, nil
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
