package block

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"

	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/shopspring/decimal"
	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/pkg/pumpfun/generated/pump_amm"
)

const eventLogPrefix = "Program data: "

type PumpAmmDecoder struct {
	ctx                 context.Context
	svcCtx              *svc.ServiceContext
	dtx                 *DecodedTx
	compiledInstruction *solTypes.CompiledInstruction
}

func (decoder *PumpAmmDecoder) DecodePumpFunAMMInstruction() (err error) {
	discriminator := GetInstructionDiscriminator(decoder.compiledInstruction.Data)

	if bytes.Equal(discriminator, pump_amm.Instruction_Buy[:]) {
		return decoder.DecodePumpFunAMMBuyInstruction()
	} else if bytes.Equal(discriminator, pump_amm.Instruction_Sell[:]) {
		return decoder.DecodePumpFunAMMSellInstruction()
	} else {
		return errors.New("unknown instruction discriminator")
	}
}

func (decoder *PumpAmmDecoder) DecodePumpFunAMMBuyInstruction() (err error) {
	fmt.Println("pump.fun AMM Buy instruction", decoder.dtx.TxHash)

	// 解析事件数据
	events, err := decoder.parsePumpAmmEvents(decoder.dtx.Tx.Meta.LogMessages)
	if err != nil {
		return fmt.Errorf("failed to parse buy events: %w", err)
	}

	// 处理解析到的 BuyEvent
	for _, event := range events {
		if buyEvent, ok := event.(*pump_amm.BuyEvent); ok {
			fmt.Printf("Buy Event - Pool: %s, User: %s, BaseAmountOut: %d, QuoteAmountIn: %d\n",
				buyEvent.Pool.String(),
				buyEvent.User.String(),
				buyEvent.BaseAmountOut,
				buyEvent.QuoteAmountIn)

			// token info
			baseTokenAccountInfo := decoder.dtx.TokenAccountMap[buyEvent.UserBaseTokenAccount.String()]
			if baseTokenAccountInfo == nil {
				fmt.Printf("baseTokenAccountInfo is nil, userBaseTokenAccount: %s\n", buyEvent.UserBaseTokenAccount.String())
				continue
			}
			quoteTokenAccountInfo := decoder.dtx.TokenAccountMap[buyEvent.UserQuoteTokenAccount.String()]
			if quoteTokenAccountInfo == nil {
				fmt.Printf("quoteTokenAccountInfo is nil, userQuoteTokenAccount: %s\n", buyEvent.UserQuoteTokenAccount.String())
				continue
			}

			// 计算token price
			// Buy: 用户用SOL买token，QuoteAmountIn是实际支付的SOL数量（不包含手续费）
			solAmount := decimal.NewFromInt(int64(buyEvent.QuoteAmountIn)).Div(decimal.NewFromFloat(math.Pow10(int(quoteTokenAccountInfo.TokenDecimal)))).InexactFloat64()
			totalUSD := decimal.NewFromFloat(solAmount).Mul(decimal.NewFromFloat(decoder.dtx.BlockDb.SolPrice)).InexactFloat64()
			tokenAmount := decimal.NewFromInt(int64(buyEvent.BaseAmountOut)).Div(decimal.NewFromFloat(math.Pow10(int(baseTokenAccountInfo.TokenDecimal)))).InexactFloat64()
			if tokenAmount == 0 {
				fmt.Printf("TokenAmount is 0, baseTokenAccount: %s\n", baseTokenAccountInfo.TokenAccountAddress)
				continue
			}
			tokenPriceUSD := decimal.NewFromFloat(totalUSD).Div(decimal.NewFromFloat(tokenAmount)).InexactFloat64()
			fmt.Printf("Buy - TokenPriceUSD: %f, SOLAmount: %f, TotalUSD: %f, TokenAmount: %f\n", tokenPriceUSD, solAmount, totalUSD, tokenAmount)
		}
	}

	return nil
}

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

func (decoder *PumpAmmDecoder) DecodePumpFunAMMSellInstruction() (err error) {
	fmt.Println("pump.fun AMM Sell instruction", decoder.dtx.TxHash)

	// 解析账户数据: 回顾交易解析（账户存储结构、指令数据结构）
	if len(decoder.compiledInstruction.Accounts) != 21 {
		return fmt.Errorf("invalid accounts length: %d", len(decoder.compiledInstruction.Accounts))
	}

	// 解析事件数据
	events, err := decoder.parsePumpAmmEvents(decoder.dtx.Tx.Meta.LogMessages)
	if err != nil {
		return fmt.Errorf("failed to parse sell events: %w", err)
	}

	// 处理解析到的 SellEvent
	for _, event := range events {
		if sellEvent, ok := event.(*pump_amm.SellEvent); ok {
			fmt.Printf("Sell Event - Pool: %s, User: %s, BaseAmountIn: %d, QuoteAmountOut: %d\n",
				sellEvent.Pool.String(),
				sellEvent.User.String(),
				sellEvent.BaseAmountIn,
				sellEvent.QuoteAmountOut)

			// token info
			baseTokenAccountInfo := decoder.dtx.TokenAccountMap[sellEvent.UserBaseTokenAccount.String()]
			if baseTokenAccountInfo == nil {
				fmt.Printf("baseTokenAccountInfo is nil, userBaseTokenAccount: %s\n", sellEvent.UserBaseTokenAccount.String())
				continue
			}
			quoteTokenAccountInfo := decoder.dtx.TokenAccountMap[sellEvent.UserQuoteTokenAccount.String()]
			if quoteTokenAccountInfo == nil {
				fmt.Printf("quoteTokenAccountInfo is nil, userQuoteTokenAccount: %s\n", sellEvent.UserQuoteTokenAccount.String())
				continue
			}

			// 计算token price
			// Sell: 用户卖token换SOL，QuoteAmountOut是实际获得的SOL数量（包含手续费扣除后的净收入）
			solAmount := decimal.NewFromInt(int64(sellEvent.QuoteAmountOut)).Div(decimal.NewFromFloat(math.Pow10(int(quoteTokenAccountInfo.TokenDecimal)))).InexactFloat64()
			totalUSD := decimal.NewFromFloat(solAmount).Mul(decimal.NewFromFloat(decoder.dtx.BlockDb.SolPrice)).InexactFloat64()
			tokenAmount := decimal.NewFromInt(int64(sellEvent.BaseAmountIn)).Div(decimal.NewFromFloat(math.Pow10(int(baseTokenAccountInfo.TokenDecimal)))).InexactFloat64()
			if tokenAmount == 0 {
				fmt.Printf("TokenAmount is 0, baseTokenAccount: %s\n", baseTokenAccountInfo.TokenAccountAddress)
				continue
			}
			tokenPriceUSD := decimal.NewFromFloat(totalUSD).Div(decimal.NewFromFloat(tokenAmount)).InexactFloat64()
			fmt.Printf("Sell - TokenPriceUSD: %f, SOLAmount: %f, TotalUSD: %f, TokenAmount: %f\n", tokenPriceUSD, solAmount, totalUSD, tokenAmount)
		}
	}

	return nil
}
