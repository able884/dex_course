package block

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/pumpfun/generated/pump_amm"
	"richcode.cc/dex/pkg/types"
	"richcode.cc/dex/pkg/util"
)

const eventLogPrefix = "Program data: "

const InitPumpTokenAmount = 873000000
const InitSolTokenAmount = 0.015
const VirtualInitPumpTokenAmount = 1073000191

type PumpAmmDecoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction // 指向当前指令，包含账户索引等上下文
}

// DecodePumpFunAMMInstruction 根据指令判别码分发到具体的买入或卖出解析逻辑。
func (decoder *PumpAmmDecoder) DecodePumpFunAMMInstruction() (*types.TradeWithPair, error) {
	discriminator := GetInstructionDiscriminator(decoder.compiledInstruction.Data)

	if bytes.Equal(discriminator, pump_amm.Instruction_Buy[:]) {
		return decoder.DecodePumpFunAMMBuyInstruction()
	} else if bytes.Equal(discriminator, pump_amm.Instruction_Sell[:]) {
		return decoder.DecodePumpFunAMMSellInstruction()
	} else if bytes.Equal(discriminator, pump_amm.Instruction_CreatePool[:]) {
		return decoder.DecodeCreatePoolInstruction(decoder.dtx.Tx.Meta.LogMessages)
	}
	return nil, errors.New("unknown instruction discriminator")
}

// DecodePumpFunAMMBuyInstruction 解析 PumpFun AMM 的买入事件并构造成交结构。
func (decoder *PumpAmmDecoder) DecodePumpFunAMMBuyInstruction() (*types.TradeWithPair, error) {
	logger := decoder.logger()
	logger.Infof("pump.fun AMM buy instruction tx=%s", decoder.dtx.TxHash)

	// 第一步：解析日志获取事件明细，只要识别到买入事件才继续后续逻辑
	events, err := decoder.parsePumpAmmEvents(decoder.dtx.Tx.Meta.LogMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to parse buy events: %w", err)
	}

	// 处理解析到的 BuyEvent
	var buyEvent *pump_amm.BuyEvent
	for _, rawEvent := range events {
		event, ok := rawEvent.(*pump_amm.BuyEvent)
		if !ok {
			continue
		}
		buyEvent = event
		break
	}
	if buyEvent == nil {
		return nil, errors.New("buy event not found in logs")
	}

	// 第二步：补齐账户上下文，若任一账户缺失则说明上游未记录 Token 变化，直接跳过本次交易
	baseTokenAccountInfo, err := decoder.getTokenAccount(buyEvent.UserBaseTokenAccount.String())
	if err != nil {
		logger.Infof("skip pump.fun buy: %v", err)
		return nil, nil
	}
	quoteTokenAccountInfo, err := decoder.getTokenAccount(buyEvent.UserQuoteTokenAccount.String())
	if err != nil {
		logger.Infof("skip pump.fun buy: %v", err)
		return nil, nil
	}

	trade, err := decoder.newPumpTrade(buyEvent.Pool.String(), buyEvent.User.String(), baseTokenAccountInfo, quoteTokenAccountInfo)
	if err != nil {
		return nil, err
	}

	// 第三步：组装业务侧需要的成交结构
	trade.Type = types.TradeTypeBuy
	trade.BaseTokenAmount = uiAmount(buyEvent.QuoteAmountInWithLpFee, quoteTokenAccountInfo.TokenDecimal)
	trade.TokenAmount = uiAmount(buyEvent.BaseAmountOut, baseTokenAccountInfo.TokenDecimal)
	trade.BaseTokenAmountInt = int64(buyEvent.QuoteAmountInWithLpFee)
	trade.TokenAmountInt = int64(buyEvent.BaseAmountOut)
	trade.To = buyEvent.UserQuoteTokenAccount.String()
	trade.TokenAmount1 = buyEvent.BaseAmountOut
	trade.TokenAmount2 = buyEvent.MaxQuoteAmountIn
	trade.PoolBaseTokenReserves = buyEvent.PoolBaseTokenReserves
	trade.PoolQuoteTokenReserves = buyEvent.PoolQuoteTokenReserves
	trade.CurrentBaseTokenInPoolAmount = float64(buyEvent.PoolQuoteTokenReserves)
	trade.CurrentTokenInPoolAmount = float64(buyEvent.PoolBaseTokenReserves)
	trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
	trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount

	// 第四步：计算价格指标，基于 SOL 单价推导出代币价格
	// Buy: 用户用SOL买token，QuoteAmountIn是实际支付的SOL数量（不包含手续费）
	solAmount := uiAmount(buyEvent.QuoteAmountIn, quoteTokenAccountInfo.TokenDecimal)
	solPriceUSD := decoder.solPriceUSD()
	totalUSD := decimal.NewFromFloat(solAmount).Mul(decimal.NewFromFloat(solPriceUSD)).InexactFloat64()
	tokenAmount := uiAmount(buyEvent.BaseAmountOut, baseTokenAccountInfo.TokenDecimal)
	if tokenAmount == 0 {
		logger.Infof("skip pump.fun buy: token amount is zero baseAccount=%s", baseTokenAccountInfo.TokenAccountAddress)
		return nil, nil
	}

	trade.BaseTokenPriceUSD = solPriceUSD
	trade.TotalUSD = totalUSD
	trade.TokenPriceUSD = decimal.NewFromFloat(totalUSD).Div(decimal.NewFromFloat(tokenAmount)).InexactFloat64()

	tx := decoder.dtx.Tx
	// 第五步：解析指令账户映射，持久化 Pump AMM 元信息
	info, infoErr := decoder.newPumpAmmInfo(
		buyEvent.Pool.String(),
		tx,
		buyEvent.ProtocolFeeRecipient.String(),
		buyEvent.ProtocolFeeRecipientTokenAccount.String(),
	)
	if infoErr != nil {
		logger.Infof("pump.fun buy: build PumpAmmInfo failed: %v", infoErr)
	} else {
		trade.PumpAmmInfo = info
	}

	// 第六步：同步 Pump 相关指标，决定是否进入迁移阶段
	trade.PumpPoint = calculatePumpPoint(trade.CurrentTokenInPoolAmount)
	trade.PumpMarketCap = decimal.NewFromFloat(trade.TokenPriceUSD).Mul(decimal.NewFromFloat(trade.PairInfo.TokenTotalSupply)).InexactFloat64()
	trade.PumpPairAddr = trade.PairAddr
	trade.PumpStatus = PumpStatusTrading
	if trade.PumpPoint >= 0.999 {
		trade.PumpStatus = PumpStatusMigrating
		trade.PumpPoint = 1
	}

	logger.Infof("decoded pump.fun AMM buy pool=%s maker=%s tokenOut=%d quoteInWithFee=%d priceUSD=%.8f",
		trade.PairAddr,
		trade.Maker,
		buyEvent.BaseAmountOut,
		buyEvent.QuoteAmountInWithLpFee,
		trade.TokenPriceUSD,
	)

	return trade, nil
}

// parsePumpAmmEvents 从日志中提取并反序列化 AMM 事件。
func (decoder *PumpAmmDecoder) parsePumpAmmEvents(logMessages []string) (events []any, err error) {
	for _, logMessage := range logMessages {
		if !strings.HasPrefix(logMessage, eventLogPrefix) {
			continue
		}

		dataStr := strings.TrimPrefix(logMessage, eventLogPrefix)
		eventData, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			continue
		}

		if len(eventData) < 8 {
			continue
		}

		var discriminator [8]byte
		copy(discriminator[:], eventData[:8])

		switch discriminator {
		case pump_amm.Event_BuyEvent:
			event, parseErr := pump_amm.ParseEvent_BuyEvent(eventData)
			if parseErr != nil {
				continue
			}
			events = append(events, event)

		case pump_amm.Event_SellEvent:
			event, parseErr := pump_amm.ParseEvent_SellEvent(eventData)
			if parseErr != nil {
				continue
			}
			events = append(events, event)

		case pump_amm.Event_CreatePoolEvent:
			event, parseErr := pump_amm.ParseEvent_CreatePoolEvent(eventData)
			if parseErr != nil {
				continue
			}
			events = append(events, event)

		default:
			continue
		}
	}

	return events, nil
}

// DecodePumpFunAMMSellInstruction 解析 PumpFun AMM 的卖出事件并构造成交结构。
func (decoder *PumpAmmDecoder) DecodePumpFunAMMSellInstruction() (*types.TradeWithPair, error) {
	logger := decoder.logger()
	logger.Infof("pump.fun AMM sell instruction tx=%s", decoder.dtx.TxHash)

	// 解析账户数据: 回顾交易解析（账户存储结构、指令数据结构）
	if len(decoder.compiledInstruction.Accounts) != 21 {
		return nil, fmt.Errorf("invalid accounts length: %d", len(decoder.compiledInstruction.Accounts))
	}

	// 第一步：解析日志获取事件明细，只要识别出卖出事件才继续后续逻辑
	events, err := decoder.parsePumpAmmEvents(decoder.dtx.Tx.Meta.LogMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sell events: %w", err)
	}

	// 处理解析到的 SellEvent
	var sellEvent *pump_amm.SellEvent
	for _, rawEvent := range events {
		event, ok := rawEvent.(*pump_amm.SellEvent)
		if !ok {
			continue
		}
		sellEvent = event
		break
	}
	if sellEvent == nil {
		return nil, errors.New("sell event not found in logs")
	}

	// 第二步：补齐账户上下文，若任一账户缺失则说明上游未记录 Token 变化，直接跳过本次交易
	baseTokenAccountInfo, err := decoder.getTokenAccount(sellEvent.UserBaseTokenAccount.String())
	if err != nil {
		logger.Infof("skip pump.fun sell: %v", err)
		return nil, nil
	}
	quoteTokenAccountInfo, err := decoder.getTokenAccount(sellEvent.UserQuoteTokenAccount.String())
	if err != nil {
		logger.Infof("skip pump.fun sell: %v", err)
		return nil, nil
	}

	trade, err := decoder.newPumpTrade(sellEvent.Pool.String(), sellEvent.User.String(), baseTokenAccountInfo, quoteTokenAccountInfo)
	if err != nil {
		return nil, err
	}

	// 第三步：组装业务侧需要的成交结构
	trade.Type = types.TradeTypeSell
	trade.BaseTokenAmount = uiAmount(sellEvent.QuoteAmountOut, quoteTokenAccountInfo.TokenDecimal)
	trade.TokenAmount = uiAmount(sellEvent.BaseAmountIn, baseTokenAccountInfo.TokenDecimal)
	trade.BaseTokenAmountInt = int64(sellEvent.QuoteAmountOut)
	trade.TokenAmountInt = int64(sellEvent.BaseAmountIn)
	trade.To = sellEvent.UserQuoteTokenAccount.String()
	trade.TokenAmount1 = sellEvent.BaseAmountIn
	trade.TokenAmount2 = sellEvent.MinQuoteAmountOut
	trade.PoolBaseTokenReserves = sellEvent.PoolBaseTokenReserves
	trade.PoolQuoteTokenReserves = sellEvent.PoolQuoteTokenReserves
	trade.CurrentBaseTokenInPoolAmount = float64(sellEvent.PoolQuoteTokenReserves)
	trade.CurrentTokenInPoolAmount = float64(sellEvent.PoolBaseTokenReserves)
	trade.PairInfo.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
	trade.PairInfo.CurrentTokenAmount = trade.CurrentTokenInPoolAmount

	// 第四步：计算价格指标，评估卖出所得与代币单价
	solAmount := uiAmount(sellEvent.QuoteAmountOut, quoteTokenAccountInfo.TokenDecimal)
	solPriceUSD := decoder.solPriceUSD()
	totalUSD := decimal.NewFromFloat(solAmount).Mul(decimal.NewFromFloat(solPriceUSD)).InexactFloat64()
	tokenAmount := uiAmount(sellEvent.BaseAmountIn, baseTokenAccountInfo.TokenDecimal)
	if tokenAmount == 0 {
		logger.Infof("skip pump.fun sell: token amount is zero baseAccount=%s", baseTokenAccountInfo.TokenAccountAddress)
		return nil, nil
	}

	trade.BaseTokenPriceUSD = solPriceUSD
	trade.TotalUSD = totalUSD
	trade.TokenPriceUSD = decimal.NewFromFloat(totalUSD).Div(decimal.NewFromFloat(tokenAmount)).InexactFloat64()

	// 第五步：解析指令账户映射，持久化 Pump AMM 元信息
	info, infoErr := decoder.newPumpAmmInfo(
		sellEvent.Pool.String(),
		decoder.dtx.Tx,
		sellEvent.ProtocolFeeRecipient.String(),
		sellEvent.ProtocolFeeRecipientTokenAccount.String(),
	)
	if infoErr != nil {
		logger.Infof("pump.fun sell: build PumpAmmInfo failed: %v", infoErr)
	} else {
		trade.PumpAmmInfo = info
	}

	// 第六步：更新 Pump 指标，评估是否达到上线或迁移阈值
	trade.PumpPoint = calculatePumpPoint(trade.CurrentTokenInPoolAmount)
	trade.PumpMarketCap = decimal.NewFromFloat(trade.TokenPriceUSD).Mul(decimal.NewFromFloat(trade.PairInfo.TokenTotalSupply)).InexactFloat64()
	if trade.PumpPoint >= 0.999 {
		trade.PumpStatus = PumpStatusMigrating
		trade.PumpPoint = 1
	}

	logger.Infof("decoded pump.fun AMM sell pool=%s maker=%s tokenIn=%d quoteOut=%d priceUSD=%.8f",
		trade.PairAddr,
		trade.Maker,
		sellEvent.BaseAmountIn,
		sellEvent.QuoteAmountOut,
		trade.TokenPriceUSD,
	)

	return trade, nil
}

// 解析创建池子指令
func (decoder *PumpAmmDecoder) DecodeCreatePoolInstruction(logMessages []string) (*types.TradeWithPair, error) {
	logger := decoder.logger()
	logger.Infof("pump.fun AMM create pool instruction tx=%s", decoder.dtx.TxHash)

	events, err := decoder.parsePumpAmmEvents(decoder.dtx.Tx.Meta.LogMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to parse create pool events: %w", err)
	}

	// 处理解析到的 CreatePoolEvent
	var createPoolEvent *pump_amm.CreatePoolEvent
	for _, rawEvent := range events {
		event, ok := rawEvent.(*pump_amm.CreatePoolEvent)
		if !ok {
			continue
		}
		createPoolEvent = event
		break
	}
	if createPoolEvent == nil {
		return nil, errors.New("create pool event not found in logs")
	}
	trade := &types.TradeWithPair{}
	trade.ChainId = SolChainId
	trade.TxHash = decoder.dtx.TxHash
	trade.PairAddr = createPoolEvent.Pool.String()
	trade.PairInfo = types.Pair{
		ChainId:          SolChainId,
		Addr:             createPoolEvent.Pool.String(),
		BaseTokenAddr:    createPoolEvent.BaseMint.String(),
		BaseTokenDecimal: createPoolEvent.BaseMintDecimals,
		BaseTokenSymbol:  util.GetBaseToken(SolChainIdInt).Symbol,
		TokenAddr:        createPoolEvent.QuoteMint.String(),
		TokenDecimal:     createPoolEvent.QuoteMintDecimals,
		BlockTime:        decoder.dtx.BlockDb.BlockTime.Unix(),
		BlockNum:         decoder.dtx.BlockDb.Slot,
	}
	trade.Maker = createPoolEvent.Creator.String()
	trade.Type = types.TradePumpAmmCreatePool
	trade.LpMintAddress = createPoolEvent.LpMint.String()
	trade.PoolBaseTokenReserves = createPoolEvent.PoolBaseAmount
	trade.PoolQuoteTokenReserves = createPoolEvent.PoolQuoteAmount
	return trade, nil
}

// logger 返回带上下文的日志对象，统一日志格式。
func (decoder *PumpAmmDecoder) logger() logx.Logger {
	if decoder.ctx != nil {
		return logx.WithContext(decoder.ctx)
	}
	return logx.WithContext(context.Background())
}

// solPriceUSD 获取当前交易关联的 SOL 美元价格，优先使用区块缓存。
func (decoder *PumpAmmDecoder) solPriceUSD() float64 {
	if decoder.dtx != nil && decoder.dtx.BlockDb != nil && decoder.dtx.BlockDb.SolPrice > 0 {
		return decoder.dtx.BlockDb.SolPrice
	}
	if decoder.dtx != nil && decoder.dtx.SolPrice > 0 {
		return decoder.dtx.SolPrice
	}
	return 0
}

// getTokenAccount 从缓存中拉取具体 TokenAccount 信息。
func (decoder *PumpAmmDecoder) getTokenAccount(address string) (*TokenAccount, error) {
	if decoder.dtx == nil || decoder.dtx.TokenAccountMap == nil {
		return nil, fmt.Errorf("token account map unavailable for %s", address)
	}
	info, ok := decoder.dtx.TokenAccountMap[address]
	if !ok || info == nil {
		return nil, fmt.Errorf("token account info missing for %s", address)
	}
	return info, nil
}

// newPumpTrade 构建通用的 Pump 交易结构，填充基础元数据。
func (decoder *PumpAmmDecoder) newPumpTrade(poolAddr, maker string, baseAccount, quoteAccount *TokenAccount) (*types.TradeWithPair, error) {
	if baseAccount == nil {
		return nil, errors.New("base token account info is nil")
	}
	if quoteAccount == nil {
		return nil, errors.New("quote token account info is nil")
	}
	if decoder.dtx == nil || decoder.dtx.BlockDb == nil {
		return nil, errors.New("block info unavailable for trade")
	}

	block := decoder.dtx.BlockDb
	trade := &types.TradeWithPair{
		ChainId:           SolChainId,
		ChainIdInt:        SolChainIdInt,
		TxHash:            decoder.dtx.TxHash,
		PairAddr:          poolAddr,
		Maker:             maker,
		Slot:              block.Slot,
		BlockNum:          block.Slot,
		BlockTime:         block.BlockTime.Unix(),
		HashId:            fmt.Sprintf("%v#%d", block.Slot, decoder.dtx.TxIndex),
		TransactionIndex:  decoder.dtx.TxIndex,
		SwapName:          PumpSwap,
		PumpPairAddr:      poolAddr,
		PumpStatus:        PumpStatusTrading,
		BaseTokenPriceUSD: decoder.solPriceUSD(),
		PairInfo: types.Pair{
			ChainId:          SolChainId,
			Addr:             poolAddr,
			BaseTokenAddr:    baseAccount.TokenAddress,
			BaseTokenDecimal: baseAccount.TokenDecimal,
			BaseTokenSymbol:  util.GetBaseToken(SolChainIdInt).Symbol,
			TokenAddr:        quoteAccount.TokenAddress,
			TokenDecimal:     quoteAccount.TokenDecimal,
			BlockNum:         block.Slot,
			BlockTime:        block.BlockTime.Unix(),
		},
	}
	trade.PairInfo.InitTokenAmount = VirtualInitPumpTokenAmount
	trade.PairInfo.InitBaseTokenAmount = InitSolTokenAmount

	return trade, nil
}

// newPumpAmmInfo 基于指令账户索引生成 Pump AMM 的池子信息。
func (decoder *PumpAmmDecoder) newPumpAmmInfo(
	poolAddr string,
	tx *client.BlockTransaction,
	protocolFeeRecipient string,
	protocolFeeRecipientTokenAccount string,
) (*solmodel.PumpAmmInfo, error) {
	if tx == nil {
		return nil, errors.New("transaction data unavailable for pump AMM info")
	}
	accounts := decoder.compiledInstruction.Accounts
	if len(accounts) <= 15 {
		return nil, fmt.Errorf("invalid accounts length: %d", len(accounts))
	}
	now := time.Now()
	return &solmodel.PumpAmmInfo{
		PoolAccount:                      poolAddr,
		GlobalConfigAccount:              tx.AccountKeys[accounts[2]].String(),
		BaseMintAccount:                  tx.AccountKeys[accounts[3]].String(),
		QuoteMintAccount:                 tx.AccountKeys[accounts[4]].String(),
		PoolBaseTokenAccount:             tx.AccountKeys[accounts[7]].String(),
		PoolQuoteTokenAccount:            tx.AccountKeys[accounts[8]].String(),
		ProtocolFeeRecipientAccount:      protocolFeeRecipient,
		ProtocolFeeRecipientTokenAccount: protocolFeeRecipientTokenAccount,
		BaseTokenProgram:                 tx.AccountKeys[accounts[11]].String(),
		QuoteTokenProgram:                tx.AccountKeys[accounts[12]].String(),
		EventAuthorityAccount:            tx.AccountKeys[accounts[15]].String(),
		CreatedAt:                        now,
		UpdatedAt:                        now,
	}, nil
}

// calculatePumpPoint 根据池子剩余 Token 量计算 Pump 曲线进度。
func calculatePumpPoint(poolTokenReserves float64) float64 {
	if poolTokenReserves <= 0 {
		return 1
	}
	point := decimal.NewFromInt(1).
		Sub(decimal.NewFromFloat(poolTokenReserves).
			Div(decimal.NewFromInt(int64(InitPumpTokenAmount)))).
		InexactFloat64()
	return min(max(point, 0), 1)
}

// uiAmount 将区块链原始数值按精度转换为可读的浮点数。
func uiAmount(amount uint64, decimals uint8) float64 {
	if decimals == 0 {
		return float64(amount)
	}
	return decimal.NewFromInt(int64(amount)).Div(decimal.NewFromFloat(math.Pow10(int(decimals)))).InexactFloat64()
}
