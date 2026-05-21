package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"

	solClient "github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	solTypes "github.com/blocto/solana-go-sdk/types"
	bin "github.com/gagliardetto/binary"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
	"richcode.cc/dex/pkg/types"
)

const cpmmDefaultTokenDecimal = 6

type CpmmDecoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction
	innerInstruction    *solClient.InnerInstruction
}

func (decoder *CpmmDecoder) DecodeRaydiumCPMMInstruction() (*types.TradeWithPair, error) {
	discriminator := GetInstructionDiscriminator(decoder.compiledInstruction.Data)

	fmt.Println("cpmm decoder discriminator: ", string(discriminator), decoder.dtx.TxHash)

	switch {
	case bytes.Equal(discriminator, raydium_cp_swap.Instruction_Initialize[:]):
		return decoder.decodeInitialize()
	case bytes.Equal(discriminator, raydium_cp_swap.Instruction_Deposit[:]):
		return decoder.decodeDeposit()
	case bytes.Equal(discriminator, raydium_cp_swap.Instruction_Withdraw[:]):
		return decoder.decodeWithdraw()
	case bytes.Equal(discriminator, raydium_cp_swap.Instruction_SwapBaseInput[:]):
		return decoder.decodeSwap()
	case bytes.Equal(discriminator, raydium_cp_swap.Instruction_SwapBaseOutput[:]):
		return decoder.decodeSwap()
	default:
		return nil, ErrNotSupportInstruction
	}
}

func (decoder *CpmmDecoder) decodeInitialize() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	accounts := decoder.compiledInstruction.Accounts
	if len(accounts) < 14 {
		return nil, errors.New("initialize accounts length mismatch")
	}

	poolCreator := tx.AccountKeys[accounts[0]]
	ammConfig := tx.AccountKeys[accounts[1]]
	poolState := tx.AccountKeys[accounts[3]]
	token0Mint := tx.AccountKeys[accounts[4]]
	token1Mint := tx.AccountKeys[accounts[5]]
	lpMint := tx.AccountKeys[accounts[6]]
	token0Vault := tx.AccountKeys[accounts[10]]
	token1Vault := tx.AccountKeys[accounts[11]]
	observationState := tx.AccountKeys[accounts[13]]
	token0Program := tx.AccountKeys[accounts[15]]
	token1Program := tx.AccountKeys[accounts[16]]

	poolStateData, _ := decoder.fetchPoolState(poolState.String())
	ammConfigData, _ := decoder.fetchAmmConfig(ammConfig.String())

	// Raydium CPMM pools enforce token0/token1 ordering; prefer on-chain pool state over heuristics.
	baseTokenAddr := token0Mint.String()
	tokenAddr := token1Mint.String()
	baseIsToken0 := true
	if poolStateData != nil {
		baseTokenAddr = poolStateData.Token0Mint.String()
		tokenAddr = poolStateData.Token1Mint.String()
		baseIsToken0 = true
		decoder.logger().Infof("cpmm init: using pool state ordering token0=%s token1=%s tx=%s", baseTokenAddr, tokenAddr, decoder.dtx.TxHash)
	} else {
		// 如果是稳定币或者WSOL，则优先将其作为base token，
		baseTokenAddr, tokenAddr = decoder.determineBaseAndTokenMints(token0Mint.String(), token1Mint.String())
		baseIsToken0 = baseTokenAddr == token0Mint.String()
		decoder.logger().Infof("cpmm init: fallback heuristic base=%s token=%s baseIsToken0=%v tx=%s", baseTokenAddr, tokenAddr, baseIsToken0, decoder.dtx.TxHash)
	}

	// Prefer reading decimals directly from mint accounts to avoid corrupted values from state decoding.
	baseDecimal := decoder.fetchMintDecimal(baseTokenAddr)
	tokenDecimal := decoder.fetchMintDecimal(tokenAddr)
	if baseDecimal == 0 && poolStateData != nil {
		if baseIsToken0 {
			baseDecimal = poolStateData.Mint0Decimals
		} else {
			baseDecimal = poolStateData.Mint1Decimals
		}
	}
	if tokenDecimal == 0 && poolStateData != nil {
		if baseIsToken0 {
			tokenDecimal = poolStateData.Mint1Decimals
		} else {
			tokenDecimal = poolStateData.Mint0Decimals
		}
	}
	decoder.logger().Infof("cpmm init decimals: base=%s(%d) token=%s(%d) baseIsToken0=%v tx=%s", baseTokenAddr, baseDecimal, tokenAddr, tokenDecimal, baseIsToken0, decoder.dtx.TxHash)

	trade := &types.TradeWithPair{
		ChainId:          SolChainId,
		TxHash:           decoder.dtx.TxHash,
		PairAddr:         poolState.String(),
		Maker:            poolCreator.String(),
		Type:             types.TradeRaydiumCPMMCreatePool,
		Slot:             decoder.dtx.BlockDb.Slot,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		HashId:           fmt.Sprintf("%v#%d", decoder.dtx.BlockDb.Slot, decoder.dtx.TxIndex),
		TransactionIndex: decoder.dtx.TxIndex,
		SwapName:         constants.RaydiumCPMM,
		PairInfo: types.Pair{
			ChainId:                SolChainId,
			Addr:                   poolState.String(),
			BaseTokenAddr:          baseTokenAddr,
			TokenAddr:              tokenAddr,
			BaseTokenDecimal:       baseDecimal,
			TokenDecimal:           tokenDecimal,
			BaseTokenIsNativeToken: baseTokenAddr == TokenStrWrapSol,
			BaseTokenIsToken0:      baseIsToken0,
			BlockNum:               decoder.dtx.BlockDb.Slot,
			BlockTime:              decoder.dtx.BlockDb.BlockTime.Unix(),
			Name:                   constants.RaydiumCPMM,
		},
	}

	trade.CpmmPoolInfo = decoder.buildCpmmPoolInfo(&cpmmPoolMeta{
		AmmConfig:          ammConfig.String(),
		PoolState:          poolState.String(),
		Token0Vault:        token0Vault.String(),
		Token1Vault:        token1Vault.String(),
		Token0Mint:         token0Mint.String(),
		Token1Mint:         token1Mint.String(),
		ObservationState:   observationState.String(),
		Token0Program:      token0Program.String(),
		Token1Program:      token1Program.String(),
		LpMint:             lpMint.String(),
		Authority:          tx.AccountKeys[accounts[2]].String(),
		BaseToken:          baseTokenAddr,
		AmmConfigData:      ammConfigData,
		PoolStateData:      poolStateData,
		BaseIsToken0:       baseIsToken0,
		ExistingBasePrice:  0,
		ExistingTokenPrice: 0,
	})

	// 更新token信息，并根据金库账户的余额来更新trade和pair中的池子token余额 = 金库token的变动之后的余额 / token的精度
	decoder.updateVaultAmounts(trade, token0Vault, token1Vault, baseTokenAddr, poolStateData)
	return trade, nil
}

func (decoder *CpmmDecoder) decodeDeposit() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	accounts := decoder.compiledInstruction.Accounts
	if len(accounts) < 13 {
		return nil, errors.New("deposit accounts length mismatch")
	}

	owner := tx.AccountKeys[accounts[0]]
	poolState := tx.AccountKeys[accounts[2]]
	token0Account := tx.AccountKeys[accounts[4]]
	token1Account := tx.AccountKeys[accounts[5]]
	token0Vault := tx.AccountKeys[accounts[6]]
	token1Vault := tx.AccountKeys[accounts[7]]
	ammConfig := tx.AccountKeys[accounts[1]]
	lpMint := tx.AccountKeys[accounts[12]]
	token0Mint := tx.AccountKeys[accounts[10]]
	token1Mint := tx.AccountKeys[accounts[11]]
	token0Program := tx.AccountKeys[accounts[8]]
	token1Program := tx.AccountKeys[accounts[9]]

	// 如果是稳定币或者WSOL，则优先将其作为base token，
	baseTokenAddr, _ := decoder.determineBaseAndTokenMints(token0Mint.String(), token1Mint.String())
	baseIsToken0 := baseTokenAddr == token0Mint.String()

	// 解析token2022的转账指令，返回swap对象，
	// 里面有baseToken对象，token对象，买卖方向，本次交易的2种金额，以及转账的来源和去向（to字段），
	swap := decoder.decodeLiquidityTransfers(token0Account, token1Account, token0Vault, token1Vault, baseTokenAddr)
	trade, err := decoder.buildBaseTrade(poolState, owner, swap)
	if err != nil {
		return nil, err
	}
	trade.Type = types.TradeRaydiumCPMMIncreaseLiquidity
	trade.PairInfo.Name = constants.RaydiumCPMM
	trade.SwapName = constants.RaydiumCPMM

	poolStateData, _ := decoder.fetchPoolState(poolState.String())
	ammConfigData, _ := decoder.fetchAmmConfig(ammConfig.String())
	trade.CpmmPoolInfo = decoder.buildCpmmPoolInfo(&cpmmPoolMeta{
		AmmConfig:          ammConfig.String(),
		PoolState:          poolState.String(),
		Token0Vault:        token0Vault.String(),
		Token1Vault:        token1Vault.String(),
		Token0Mint:         token0Mint.String(),
		Token1Mint:         token1Mint.String(),
		ObservationState:   "",
		Token0Program:      token0Program.String(),
		Token1Program:      token1Program.String(),
		LpMint:             lpMint.String(),
		Authority:          tx.AccountKeys[accounts[1]].String(),
		BaseToken:          baseTokenAddr,
		AmmConfigData:      ammConfigData,
		PoolStateData:      poolStateData,
		BaseIsToken0:       baseIsToken0,
		ExistingBasePrice:  trade.BaseTokenPriceUSD,
		ExistingTokenPrice: trade.TokenPriceUSD,
	})

	// 更新trade和pair中的池子token余额 = 金库token的变动之后的余额 / token的精度
	decoder.updatePoolTokenAmounts(trade, token0Vault, token1Vault, swap)
	return trade, nil
}

// 解析提现指令，Raydium CPMM 的提现在指令上与 Swap 类似，
// 都是涉及两个转账：资金池vault -> 用户账户，但增加流动性是用户账户 -> vault，
// 移除流动性是 vault -> 用户账户，因此可以复用 swap 的解析逻辑，并通过转账的方向来区分是增加还是移除流动性。
func (decoder *CpmmDecoder) decodeWithdraw() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	accounts := decoder.compiledInstruction.Accounts
	if len(accounts) < 13 {
		return nil, errors.New("withdraw accounts length mismatch")
	}

	owner := tx.AccountKeys[accounts[0]]
	poolState := tx.AccountKeys[accounts[2]]
	token0Account := tx.AccountKeys[accounts[4]]
	token1Account := tx.AccountKeys[accounts[5]]
	token0Vault := tx.AccountKeys[accounts[6]]
	token1Vault := tx.AccountKeys[accounts[7]]
	ammConfig := tx.AccountKeys[accounts[1]]
	lpMint := tx.AccountKeys[accounts[12]]
	token0Mint := tx.AccountKeys[accounts[10]]
	token1Mint := tx.AccountKeys[accounts[11]]
	token0Program := tx.AccountKeys[accounts[8]]
	token1Program := tx.AccountKeys[accounts[9]]

	// 如果是稳定币或者WSOL，则优先将其作为base token，
	baseTokenAddr, _ := decoder.determineBaseAndTokenMints(token0Mint.String(), token1Mint.String())
	baseIsToken0 := baseTokenAddr == token0Mint.String()

	// 解析token2022的转账指令，返回swap对象，
	// 里面有baseToken对象，token对象，买卖方向，本次交易的2种金额，以及转账的来源和去向（to字段），
	swap := decoder.decodeLiquidityTransfers(token0Vault, token1Vault, token0Account, token1Account, baseTokenAddr)
	trade, err := decoder.buildBaseTrade(poolState, owner, swap)
	if err != nil {
		return nil, err
	}
	trade.Type = types.TradeRaydiumCPMMDecreaseLiquidity
	trade.PairInfo.Name = constants.RaydiumCPMM
	trade.SwapName = constants.RaydiumCPMM

	poolStateData, _ := decoder.fetchPoolState(poolState.String())
	ammConfigData, _ := decoder.fetchAmmConfig(ammConfig.String())
	trade.CpmmPoolInfo = decoder.buildCpmmPoolInfo(&cpmmPoolMeta{
		AmmConfig:          ammConfig.String(),
		PoolState:          poolState.String(),
		Token0Vault:        token0Vault.String(),
		Token1Vault:        token1Vault.String(),
		Token0Mint:         token0Mint.String(),
		Token1Mint:         token1Mint.String(),
		ObservationState:   "",
		Token0Program:      token0Program.String(),
		Token1Program:      token1Program.String(),
		LpMint:             lpMint.String(),
		Authority:          tx.AccountKeys[accounts[1]].String(),
		BaseToken:          baseTokenAddr,
		AmmConfigData:      ammConfigData,
		PoolStateData:      poolStateData,
		BaseIsToken0:       baseIsToken0,
		ExistingBasePrice:  trade.BaseTokenPriceUSD,
		ExistingTokenPrice: trade.TokenPriceUSD,
	})

	// 更新trade和pair中的池子token余额 = 金库token的变动之后的余额 / token的精度
	decoder.updatePoolTokenAmounts(trade, token0Vault, token1Vault, swap)
	return trade, nil
}

func (decoder *CpmmDecoder) decodeSwap() (*types.TradeWithPair, error) {
	tx := decoder.dtx.Tx
	accounts := decoder.compiledInstruction.Accounts
	if len(accounts) < 13 {
		return nil, errors.New("swap accounts length mismatch")
	}

	payer := tx.AccountKeys[accounts[0]]
	ammConfig := tx.AccountKeys[accounts[2]]
	poolState := tx.AccountKeys[accounts[3]]
	inputTokenAccount := tx.AccountKeys[accounts[4]]
	outputTokenAccount := tx.AccountKeys[accounts[5]]
	inputVault := tx.AccountKeys[accounts[6]]
	outputVault := tx.AccountKeys[accounts[7]]
	inputTokenProgram := tx.AccountKeys[accounts[8]]
	outputTokenProgram := tx.AccountKeys[accounts[9]]
	inputTokenMint := tx.AccountKeys[accounts[10]]
	outputTokenMint := tx.AccountKeys[accounts[11]]
	observationState := tx.AccountKeys[accounts[12]]

	tokenSwap, err := decoder.decodeTokenSwap(inputTokenAccount, outputTokenAccount)
	if err != nil {
		return nil, err
	}

	trade, err := decoder.buildBaseTrade(poolState, payer, tokenSwap)
	if err != nil {
		return nil, err
	}

	trade.SwapName = constants.RaydiumCPMM
	trade.PairInfo.Name = constants.RaydiumCPMM

	decoder.updatePoolTokenAmounts(trade, inputVault, outputVault, tokenSwap)

	poolStateData, _ := decoder.fetchPoolState(poolState.String())
	token0MintStr := inputTokenMint.String()
	token1MintStr := outputTokenMint.String()
	token0VaultStr := inputVault.String()
	token1VaultStr := outputVault.String()
	token0ProgramStr := inputTokenProgram.String()
	token1ProgramStr := outputTokenProgram.String()
	if poolStateData != nil {
		token0MintStr = poolStateData.Token0Mint.String()
		token1MintStr = poolStateData.Token1Mint.String()
		token0VaultStr = poolStateData.Token0Vault.String()
		token1VaultStr = poolStateData.Token1Vault.String()
		token0ProgramStr = poolStateData.Token0Program.String()
		token1ProgramStr = poolStateData.Token1Program.String()
	}
	baseTokenAddr := tokenSwap.BaseTokenInfo.TokenAddress
	if baseTokenAddr == "" {
		// 如果是稳定币或者WSOL，则优先将其作为base token，
		baseTokenAddr, _ = decoder.determineBaseAndTokenMints(token0MintStr, token1MintStr)
	}
	baseIsToken0 := baseTokenAddr == token0MintStr
	ammConfigData, _ := decoder.fetchAmmConfig(ammConfig.String())
	trade.CpmmPoolInfo = decoder.buildCpmmPoolInfo(&cpmmPoolMeta{
		AmmConfig:          ammConfig.String(),
		PoolState:          poolState.String(),
		Token0Vault:        token0VaultStr,
		Token1Vault:        token1VaultStr,
		Token0Mint:         token0MintStr,
		Token1Mint:         token1MintStr,
		ObservationState:   observationState.String(),
		Token0Program:      token0ProgramStr,
		Token1Program:      token1ProgramStr,
		LpMint:             "",
		Authority:          tx.AccountKeys[accounts[1]].String(),
		BaseToken:          baseTokenAddr,
		AmmConfigData:      ammConfigData,
		PoolStateData:      poolStateData,
		BaseIsToken0:       baseIsToken0,
		ExistingBasePrice:  trade.BaseTokenPriceUSD,
		ExistingTokenPrice: trade.TokenPriceUSD,
	})

	return trade, nil
}

func (decoder *CpmmDecoder) decodeTokenSwap(inputTokenAccount, outputTokenAccount common.PublicKey) (swap *Swap, err error) {
	var fromTransfer *token.TransferParam
	var toTransfer *token.TransferParam
	accountKeys := decoder.dtx.Tx.AccountKeys

	swap = &Swap{}

	fromTokenAccountInfo := decoder.dtx.TokenAccountMap[inputTokenAccount.String()]
	if fromTokenAccountInfo == nil {
		err = fmt.Errorf("fromTokenAccountInfo not found, tx hash: %v", decoder.dtx.TxHash)
		return
	}
	toTokenAccountInfo := decoder.dtx.TokenAccountMap[outputTokenAccount.String()]
	if toTokenAccountInfo == nil {
		err = fmt.Errorf("toTokenAccountInfo not found, tx hash: %v", decoder.dtx.TxHash)
		return
	}

	if decoder.innerInstruction == nil {
		err = fmt.Errorf("innerInstruction not found, tx hash: %v", decoder.dtx.TxHash)
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

	if fromTransfer == nil || toTransfer == nil {
		return nil, errors.New("swap transfer not found")
	}
	if !IsSwapTransfer(fromTransfer, toTransfer, decoder.dtx.TokenAccountMap) {
		return nil, errors.New("not swap transfer")
	}

	decoder.populateSwapInfo(swap, fromTokenAccountInfo, toTokenAccountInfo, fromTransfer, toTransfer)
	return
}

// 解析token2022的转账指令，返回swap对象，
// 里面有baseToken对象，token对象，买卖方向，本次交易的2种金额，以及转账的来源和去向（to字段），
// 兼容增加/移除流动性的解析，
func (decoder *CpmmDecoder) decodeLiquidityTransfers(from0, from1, to0, to1 common.PublicKey, baseTokenAddr string) *Swap {
	accountKeys := decoder.dtx.Tx.AccountKeys
	swap := &Swap{}
	var transfer0, transfer1 *token.TransferParam
	if decoder.innerInstruction != nil {
		for _, innerInstruction := range decoder.innerInstruction.Instructions {
			transfer, err := DecodeTokenTransfer(accountKeys, &innerInstruction)
			if err != nil {
				continue
			}
			switch {
			case transfer.From.String() == from0.String() && transfer.To.String() == to0.String():
				transfer0 = transfer
			case transfer.From.String() == from1.String() && transfer.To.String() == to1.String():
				transfer1 = transfer
			case transfer.From.String() == to0.String() && transfer.To.String() == from0.String():
				transfer0 = transfer
			case transfer.From.String() == to1.String() && transfer.To.String() == from1.String():
				transfer1 = transfer
			}
		}
	}

	token0Info := decoder.dtx.TokenAccountMap[from0.String()]
	if token0Info == nil {
		// 提款
		token0Info = decoder.dtx.TokenAccountMap[to0.String()]
	}
	token1Info := decoder.dtx.TokenAccountMap[from1.String()]
	if token1Info == nil {
		// 提款
		token1Info = decoder.dtx.TokenAccountMap[to1.String()]
	}

	if token0Info == nil {
		token0Info = &TokenAccount{
			TokenAccountAddress: from0.String(),
			TokenAddress:        from0.String(),
			TokenDecimal:        cpmmDefaultTokenDecimal,
		}
	}
	if token1Info == nil {
		token1Info = &TokenAccount{
			TokenAccountAddress: from1.String(),
			TokenAddress:        from1.String(),
			TokenDecimal:        cpmmDefaultTokenDecimal,
		}
	}

	if token0Info == nil || token1Info == nil {
		return swap
	}

	isBuy := baseTokenAddr == token0Info.TokenAddress
	var baseInfo, tokenInfo *TokenAccount
	var baseTransfer, tokenTransfer *token.TransferParam
	if isBuy {
		baseInfo = token0Info
		tokenInfo = token1Info
		baseTransfer = transfer0
		tokenTransfer = transfer1
	} else {
		baseInfo = token1Info
		tokenInfo = token0Info
		baseTransfer = transfer1
		tokenTransfer = transfer0
	}

	swap.BaseTokenInfo = baseInfo
	swap.TokenInfo = tokenInfo
	if baseTransfer != nil {
		swap.BaseTokenAmountInt = int64(baseTransfer.Amount)
		swap.BaseTokenAmount = float64(baseTransfer.Amount) / math.Pow10(int(baseInfo.TokenDecimal))
	}
	if tokenTransfer != nil {
		swap.TokenAmountInt = int64(tokenTransfer.Amount)
		swap.TokenAmount = float64(tokenTransfer.Amount) / math.Pow10(int(tokenInfo.TokenDecimal))
	}
	if baseTransfer != nil && tokenTransfer != nil && baseTransfer.From.String() == baseInfo.TokenAccountAddress {
		swap.Type = types.TradeTypeBuy
		swap.To = tokenInfo.Owner
	} else {
		swap.Type = types.TradeTypeSell
		swap.To = baseInfo.Owner
	}
	return swap
}

func (decoder *CpmmDecoder) populateSwapInfo(
	swap *Swap,
	fromTokenInfo, toTokenInfo *TokenAccount,
	fromTransfer, toTransfer *token.TransferParam,
) {
	var isBuy bool
	var baseTokenInfo, tradeTokenInfo *TokenAccount
	var baseTransfer, tradeTransfer *token.TransferParam
	var ownerAddr string

	if fromTokenInfo.TokenAddress == TokenStrWrapSol {
		isBuy = true
		baseTokenInfo, tradeTokenInfo = fromTokenInfo, toTokenInfo
		baseTransfer, tradeTransfer = fromTransfer, toTransfer
		ownerAddr = toTokenInfo.Owner
	} else if toTokenInfo.TokenAddress == TokenStrWrapSol {
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	} else if decoder.isStableCoin(fromTokenInfo.TokenAddress) {
		isBuy = true
		baseTokenInfo, tradeTokenInfo = fromTokenInfo, toTokenInfo
		baseTransfer, tradeTransfer = fromTransfer, toTransfer
		ownerAddr = toTokenInfo.Owner
	} else if decoder.isStableCoin(toTokenInfo.TokenAddress) {
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	} else {
		isBuy = false
		baseTokenInfo, tradeTokenInfo = toTokenInfo, fromTokenInfo
		baseTransfer, tradeTransfer = toTransfer, fromTransfer
		ownerAddr = fromTokenInfo.Owner
	}

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

// 构建基础的trade对象，包含交易双方、交易金额、交易方向等基本信息
func (decoder *CpmmDecoder) buildBaseTrade(poolStateAccount common.PublicKey, maker common.PublicKey, tokenSwap *Swap) (*types.TradeWithPair, error) {
	if tokenSwap == nil || tokenSwap.TokenInfo == nil || tokenSwap.BaseTokenInfo == nil {
		return nil, errors.New("swap info incomplete")
	}

	trade := &types.TradeWithPair{}
	trade.ChainId = SolChainId
	trade.TxHash = decoder.dtx.TxHash
	trade.PairAddr = poolStateAccount.String()

	trade.PairInfo = types.Pair{
		ChainId:                SolChainId,
		Addr:                   poolStateAccount.String(),
		BaseTokenAddr:          tokenSwap.BaseTokenInfo.TokenAddress,
		BaseTokenDecimal:       tokenSwap.BaseTokenInfo.TokenDecimal,
		BaseTokenSymbol:        tokenSwap.BaseTokenInfo.TokenSymbol,
		TokenAddr:              tokenSwap.TokenInfo.TokenAddress,
		TokenSymbol:            tokenSwap.TokenInfo.TokenSymbol,
		TokenDecimal:           tokenSwap.TokenInfo.TokenDecimal,
		BaseTokenIsNativeToken: tokenSwap.BaseTokenInfo.TokenAddress == TokenStrWrapSol,
		BlockTime:              decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:               decoder.dtx.BlockDb.Slot,
		Name:                   constants.RaydiumCPMM,
	}

	trade.Maker = maker.String()
	trade.Type = tokenSwap.Type
	trade.BaseTokenAmount = tokenSwap.BaseTokenAmount
	trade.TokenAmount = tokenSwap.TokenAmount

	// 本次交易的baseTokenUsd价格
	// 如果是wsol价格 = 最开始计算的SolPrice，
	// 如果是其他usdt稳定币价格 = 1，其他则为0
	trade.BaseTokenPriceUSD = decoder.baseTokenPriceUSD(tokenSwap.BaseTokenInfo.TokenAddress)
	if trade.BaseTokenPriceUSD == 0 {
		// 根据交易中的pair地址，从已解析的交易数据中获取该pair的基础币和交易币的价格信息
		if basePrice, tokenPrice := decoder.fallbackPairPrices(poolStateAccount.String()); basePrice > 0 {
			trade.BaseTokenPriceUSD = basePrice
			if tokenPrice > 0 {
				trade.TokenPriceUSD = tokenPrice
			}
		}
	}

	// 本次交易总的usd金额 = 本次交易的baseToken数量 * baseToken的美元价格
	trade.TotalUSD = trade.BaseTokenAmount * trade.BaseTokenPriceUSD
	if trade.TokenPriceUSD == 0 && tokenSwap.TokenAmount != 0 && trade.TotalUSD > 0 {
		// 本次交易的token价格 = 本次交易的总美元金额 / token数量
		trade.TokenPriceUSD = trade.TotalUSD / tokenSwap.TokenAmount
	}
	if trade.TokenPriceUSD == 0 {
		// 根据交易中的pair地址，从已解析的交易数据中获取该pair的基础币和交易币的价格信息
		if _, tokenPrice := decoder.fallbackPairPrices(poolStateAccount.String()); tokenPrice > 0 {
			trade.TokenPriceUSD = tokenPrice
		}
	}

	trade.To = tokenSwap.To
	trade.Slot = decoder.dtx.BlockDb.Slot
	trade.BlockTime = decoder.dtx.BlockDb.BlockTime.Unix()
	trade.HashId = fmt.Sprintf("%v#%d", decoder.dtx.BlockDb.Slot, decoder.dtx.TxIndex)
	trade.TransactionIndex = decoder.dtx.TxIndex
	trade.SwapName = constants.RaydiumCPMM
	trade.PairInfo.Name = trade.SwapName
	trade.BaseTokenAccountAddress = tokenSwap.BaseTokenInfo.TokenAccountAddress
	trade.TokenAccountAddress = tokenSwap.TokenInfo.TokenAccountAddress
	trade.BaseTokenAmountInt = tokenSwap.BaseTokenAmountInt
	trade.TokenAmountInt = tokenSwap.TokenAmountInt

	return trade, nil
}

// 更新trade和pair中的池子token余额 = 金库token的变动之后的余额 / token的精度
func (decoder *CpmmDecoder) updatePoolTokenAmounts(trade *types.TradeWithPair, account1, account2 common.PublicKey, tokenSwap *Swap) {
	if account1 != (common.PublicKey{}) {
		poolTokenAccount := decoder.dtx.TokenAccountMap[account1.String()]
		if poolTokenAccount != nil {
			// 根据金库token0Vault的 当前baseTokenAmount = 变动之后的余额 / token0的精度
			if poolTokenAccount.TokenAddress == tokenSwap.BaseTokenInfo.TokenAddress {
				trade.CurrentBaseTokenInPoolAmount = float64(poolTokenAccount.PostValue) / math.Pow10(int(poolTokenAccount.TokenDecimal))
				trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
			} else if poolTokenAccount.TokenAddress == tokenSwap.TokenInfo.TokenAddress {
				trade.CurrentTokenInPoolAmount = float64(poolTokenAccount.PostValue) / math.Pow10(int(poolTokenAccount.TokenDecimal))
				trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount
			}
		}
	}

	if account2 != (common.PublicKey{}) {
		poolTokenAccount := decoder.dtx.TokenAccountMap[account2.String()]
		if poolTokenAccount != nil {
			if poolTokenAccount.TokenAddress == tokenSwap.BaseTokenInfo.TokenAddress {
				trade.CurrentBaseTokenInPoolAmount = float64(poolTokenAccount.PostValue) / math.Pow10(int(poolTokenAccount.TokenDecimal))
				trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
			} else if poolTokenAccount.TokenAddress == tokenSwap.TokenInfo.TokenAddress {
				trade.CurrentTokenInPoolAmount = float64(poolTokenAccount.PostValue) / math.Pow10(int(poolTokenAccount.TokenDecimal))
				trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount
			}
		}
	}
}

func (decoder *CpmmDecoder) updateVaultAmounts(trade *types.TradeWithPair, token0Vault, token1Vault common.PublicKey, baseTokenAddr string, poolState *raydium_cp_swap.PoolStateAccount) {
	swap := &Swap{
		BaseTokenInfo: &TokenAccount{TokenAddress: baseTokenAddr},
		TokenInfo:     &TokenAccount{},
	}

	trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
	trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount
	if baseTokenAddr == "" {
		return
	}

	token0Info := decoder.dtx.TokenAccountMap[token0Vault.String()]
	token1Info := decoder.dtx.TokenAccountMap[token1Vault.String()]

	// For freshly created vaults, fill missing mint/decimal info from pool state or on-chain fetch.
	if token0Info == nil {
		token0Info = &TokenAccount{
			TokenAccountAddress: token0Vault.String(),
			TokenAddress:        "",
			TokenDecimal:        0,
		}
	}
	if token1Info == nil {
		token1Info = &TokenAccount{
			TokenAccountAddress: token1Vault.String(),
			TokenAddress:        "",
			TokenDecimal:        0,
		}
	}

	if poolState != nil {
		if token0Info.TokenAddress == "" || token0Info.TokenAddress == token0Vault.String() {
			token0Info.TokenAddress = poolState.Token0Mint.String()
		}
		if token1Info.TokenAddress == "" || token1Info.TokenAddress == token1Vault.String() {
			token1Info.TokenAddress = poolState.Token1Mint.String()
		}
		if token0Info.TokenDecimal == 0 {
			token0Info.TokenDecimal = poolState.Mint0Decimals
		}
		if token1Info.TokenDecimal == 0 {
			token1Info.TokenDecimal = poolState.Mint1Decimals
		}
	}

	if token0Info.TokenDecimal == 0 && token0Info.TokenAddress != "" {
		token0Info.TokenDecimal = decoder.fetchMintDecimal(token0Info.TokenAddress)
	}
	if token1Info.TokenDecimal == 0 && token1Info.TokenAddress != "" {
		token1Info.TokenDecimal = decoder.fetchMintDecimal(token1Info.TokenAddress)
	}

	// 填充token账户的余额和小数位信息，当交易数据中缺失时（例如新创建的vault），
	// 通过RPC调用链上数据来获取并填充这些信息，避免解析失败导致整个交易解析失败。
	decoder.fillTokenAccountAmount(token0Vault, token0Info)
	decoder.fillTokenAccountAmount(token1Vault, token1Info)

	decoder.dtx.TokenAccountMap[token0Vault.String()] = token0Info
	decoder.dtx.TokenAccountMap[token1Vault.String()] = token1Info

	swap.TokenInfo.TokenAddress = token1Info.TokenAddress

	// 更新trade和pair中的baseToken和token余额 = 金库token的变动之后的余额 / token的精度
	decoder.updatePoolTokenAmounts(trade, token0Vault, token1Vault, swap)
}

// 如果是稳定币或者WSOL，则优先将其作为base token，以提高价格计算的稳定性和准确性，
// 因为Raydium CPMM的池子经常是新创建的，且池子中的token账户也可能是新创建的，
// 这时通过解析交易数据来判断哪个token是base token可能会不准确，
// 因此优先将稳定币或者WSOL作为base token，可以提高解析的准确性和稳定性。
func (decoder *CpmmDecoder) determineBaseAndTokenMints(tokenMint0, tokenMint1 string) (baseTokenAddr, tokenAddr string) {
	switch {
	case tokenMint0 == TokenStrWrapSol:
		return tokenMint0, tokenMint1
	case tokenMint1 == TokenStrWrapSol:
		return tokenMint1, tokenMint0
	case decoder.isStableCoin(tokenMint0):
		return tokenMint0, tokenMint1
	case decoder.isStableCoin(tokenMint1):
		return tokenMint1, tokenMint0
	default:
		return tokenMint0, tokenMint1
	}
}

func (decoder *CpmmDecoder) isStableCoin(tokenAddress string) bool {
	return tokenAddress == TokenStrUSDC || tokenAddress == TokenStrUSDT
}

func (decoder *CpmmDecoder) fetchPoolState(poolState string) (*raydium_cp_swap.PoolStateAccount, error) {
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		return nil, errors.New("sol client nil")
	}
	accountInfo, err := cli.GetAccountInfoWithConfig(decoder.ctx, poolState, solClient.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	state := &raydium_cp_swap.PoolStateAccount{}
	if err := state.UnmarshalWithDecoder(bin.NewBorshDecoder(accountInfo.Data)); err != nil {
		return nil, err
	}
	return state, nil
}

func (decoder *CpmmDecoder) fetchAmmConfig(ammConfig string) (*raydium_cp_swap.AmmConfigAccount, error) {
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		return nil, errors.New("sol client nil")
	}
	accountInfo, err := cli.GetAccountInfoWithConfig(decoder.ctx, ammConfig, solClient.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	config := &raydium_cp_swap.AmmConfigAccount{}
	if err := config.UnmarshalWithDecoder(bin.NewBorshDecoder(accountInfo.Data)); err != nil {
		return nil, err
	}
	return config, nil
}

// 根据mint账户的base58地址，获取该mint的decimal信息，用于解析交易中涉及的token数量，
// 由于Raydium CPMM的池子经常是新创建的，且池子中的token账户也可能是新创建的，
// 因此在解析过程中经常会遇到缺少token decimal信息的情况，
// 这个函数会尝试从已解析的交易数据中获取decimal信息，如果没有，则通过RPC调用链上数据来获取decimal信息，
// 获取失败时会默认返回0小数位，避免解析失败导致整个交易解析失败。
func (decoder *CpmmDecoder) fetchMintDecimal(mint string) uint8 {
	if mint == "" {
		return cpmmDefaultTokenDecimal
	}
	if decoder.dtx != nil && decoder.dtx.TokenDecimalMap != nil {
		if val, ok := decoder.dtx.TokenDecimalMap[mint]; ok {
			return val
		}
	}
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		return cpmmDefaultTokenDecimal
	}

	// Rpc调用获取mint账户信息，获取失败或数据异常时，默认返回0小数位，避免解析失败导致整个交易解析失败
	accountInfo, err := cli.GetAccountInfoWithConfig(decoder.ctx, mint, solClient.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		decoder.logger().Errorf("fetch mint decimal failed, mint: %s, err: %v", mint, err)
		return cpmmDefaultTokenDecimal
	}
	if len(accountInfo.Data) == 0 {
		decoder.logger().Errorf("fetch mint decimal empty account info, mint: %s", mint)
		return cpmmDefaultTokenDecimal
	}
	// 根据mint账户的data，解析出mint账户对象
	mintAccount, err := token.MintAccountFromData(accountInfo.Data)
	if err != nil {
		decoder.logger().Errorf("decode mint account failed, mint: %s, err: %v", mint, err)
		return cpmmDefaultTokenDecimal
	}

	if decoder.dtx != nil {
		if decoder.dtx.TokenDecimalMap == nil {
			decoder.dtx.TokenDecimalMap = make(map[string]uint8)
		}
		decoder.dtx.TokenDecimalMap[mint] = mintAccount.Decimals
	}
	return mintAccount.Decimals
}

// 填充token账户的余额和小数位信息，当交易数据中缺失时（例如新创建的vault），
// 通过RPC调用链上数据来获取并填充这些信息，避免解析失败导致整个交易解析失败。
func (decoder *CpmmDecoder) fillTokenAccountAmount(addr common.PublicKey, info *TokenAccount) {
	if info == nil || info.PostValue > 0 {
		return
	}
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		return
	}
	accountInfo, err := cli.GetAccountInfoWithConfig(decoder.ctx, addr.String(), solClient.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil || len(accountInfo.Data) == 0 {
		return
	}
	// 根据token账户的data，解析出token账户对象，获取余额和小数位信息，填充到token账户对象中
	tokenAcc, err := token.TokenAccountFromData(accountInfo.Data)
	if err != nil {
		return
	}
	info.PostValue = int64(tokenAcc.Amount)
	info.PreValue = int64(tokenAcc.Amount)
	if info.TokenAddress == "" {
		info.TokenAddress = tokenAcc.Mint.String()
	}
	if info.TokenDecimal == 0 {
		info.TokenDecimal = decoder.fetchMintDecimal(tokenAcc.Mint.String())
	}
}

type cpmmPoolMeta struct {
	AmmConfig          string
	PoolState          string
	Token0Vault        string
	Token1Vault        string
	Token0Mint         string
	Token1Mint         string
	Token0Program      string
	Token1Program      string
	LpMint             string
	ObservationState   string
	Authority          string
	BaseToken          string
	AmmConfigData      *raydium_cp_swap.AmmConfigAccount
	PoolStateData      *raydium_cp_swap.PoolStateAccount
	BaseIsToken0       bool
	ExistingBasePrice  float64
	ExistingTokenPrice float64
}

func (decoder *CpmmDecoder) buildCpmmPoolInfo(meta *cpmmPoolMeta) *solmodel.CpmmPoolInfo {
	if meta == nil {
		return nil
	}

	// 基础币默认是token0，但如果池子状态数据中显示基础币是token1，则覆盖默认值，确保解析结果正确。
	baseIsToken0 := meta.BaseIsToken0
	if meta.PoolStateData != nil && meta.BaseToken != "" {
		if meta.BaseToken == meta.PoolStateData.Token0Mint.String() {
			baseIsToken0 = true
		} else if meta.BaseToken == meta.PoolStateData.Token1Mint.String() {
			baseIsToken0 = false
		}
	}

	info := &solmodel.CpmmPoolInfo{
		AmmConfig:          meta.AmmConfig,
		PoolState:          meta.PoolState,
		InputVault:         meta.Token0Vault,
		OutputVault:        meta.Token1Vault,
		Authority:          meta.Authority,
		InputTokenProgram:  meta.Token0Program,
		OutputTokenProgram: meta.Token1Program,
		InputTokenMint:     meta.Token0Mint,
		OutputTokenMint:    meta.Token1Mint,
		ObservationState:   meta.ObservationState,
		TxHash:             decoder.dtx.TxHash,
	}

	if !baseIsToken0 {
		info.InputVault, info.OutputVault = info.OutputVault, info.InputVault
		info.InputTokenProgram, info.OutputTokenProgram = info.OutputTokenProgram, info.InputTokenProgram
		info.InputTokenMint, info.OutputTokenMint = info.OutputTokenMint, info.InputTokenMint
	}

	if meta.PoolStateData != nil {
		if info.Authority == "" {
			info.Authority = meta.PoolStateData.PoolCreator.String()
		}
		if obs := meta.PoolStateData.ObservationKey.String(); obs != "" && len(info.ObservationState) == 0 {
			info.ObservationState = obs
		}
		if info.InputVault == "" && baseIsToken0 {
			info.InputVault = meta.PoolStateData.Token0Vault.String()
			info.OutputVault = meta.PoolStateData.Token1Vault.String()
		} else if info.InputVault == "" {
			info.InputVault = meta.PoolStateData.Token1Vault.String()
			info.OutputVault = meta.PoolStateData.Token0Vault.String()
		}
	}

	if meta.AmmConfigData != nil {
		// TradeFeeRate is in hundredths of a bip (1e-6). Cap to 100% to avoid corrupted values blowing DB limits.
		rate := int64(meta.AmmConfigData.TradeFeeRate)
		const maxAllowed = int64(100_000_000) // 100% in 1e-6 units
		if rate > maxAllowed {
			decoder.logger().Errorf("cpmm trade fee rate too large (%d), capped to %d, ammConfig=%s", rate, maxAllowed, meta.AmmConfig)
			rate = maxAllowed
		}
		info.TradeFeeRate = rate
	}
	return info
}

// 获取基础币的usd价格，如果是wsol价格 = 最开始计算的SolPrice，
// 如果是其他usdt稳定币价格 = 1，
// 其他则为0
func (decoder *CpmmDecoder) baseTokenPriceUSD(tokenAddr string) float64 {
	switch tokenAddr {
	case TokenStrWrapSol:
		return decoder.dtx.SolPrice
	case TokenStrUSDC, TokenStrUSDT:
		return 1
	default:
		// Unknown token price; do not default to SOL to avoid misleading valuation.
		return 0
	}
}

// 根据交易中的pair地址，从已解析的交易数据中获取该pair的基础币和交易币的价格信息，
func (decoder *CpmmDecoder) fallbackPairPrices(pairAddr string) (basePrice, tokenPrice float64) {
	if decoder.svcCtx == nil || decoder.svcCtx.PairModel == nil {
		return 0, 0
	}
	pair, err := decoder.svcCtx.PairModel.FindOneByChainIdAddress(decoder.ctx, int64(SolChainIdInt), pairAddr)
	if err == nil && pair != nil {
		return pair.BaseTokenPrice, pair.TokenPrice
	}
	return 0, 0
}

func (decoder *CpmmDecoder) logger() logx.Logger {
	return logx.WithContext(decoder.ctx)
}
