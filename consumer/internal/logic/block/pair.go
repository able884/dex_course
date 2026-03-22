package block

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constrants"
	"richcode.cc/dex/pkg/types"
	"richcode.cc/dex/pkg/util"
)

// SavePair 负责写入或更新交易对基础信息，并同步 Pump 相关指标。
func (s *BlockService) SavePair(ctx context.Context, trade *types.TradeWithPair, tokenDb *solmodel.Token) (pairAtDB *solmodel.Pair, err error) {
	chainId := SolChainIdInt
	var tokenTotalSupply float64
	var tokenSymbol = trade.PairInfo.TokenSymbol
	if tokenDb != nil {
		tokenTotalSupply = tokenDb.TotalSupply
		tokenSymbol = tokenDb.Symbol
	}

	// 第一步：尝试读取现有交易对信息，判断是新增还是增量更新
	pairAtDB, err = s.sc.PairModel.FindOneByChainIdAddress(ctx, int64(chainId), trade.PairAddr)
	if err != nil {
		s.Errorf("SavePair:FindOneByChainIdAddress err: %v, pair address: %v", err, trade.PairAddr)
	}
	// 根据成交信息估算即时流动性，后续用于更新市值、流动性指标
	liq := trade.CurrentBaseTokenInPoolAmount*trade.BaseTokenPriceUSD + trade.CurrentTokenInPoolAmount*trade.TokenPriceUSD
	if liq == 0 {
		if trade.CLMMOpenPositionInfo != nil && trade.CLMMOpenPositionInfo.Liquidity != nil {

			u := trade.CLMMOpenPositionInfo.Liquidity
			bi := new(big.Int).Lsh(new(big.Int).SetUint64(u.Hi), 64)
			bi = bi.Add(bi, new(big.Int).SetUint64(u.Lo))
			f, _ := new(big.Float).SetInt(bi).Float64()
			liq = f

			fmt.Println("liq is:", liq)
		}
	}

	if trade.SwapName == constants.PumpFun {
		// PumpFun 池子的流动性以双倍基础 Token 估算，贴合官方前端展示
		liq = trade.CurrentBaseTokenInPoolAmount * trade.BaseTokenPriceUSD * 2
	}

	baseTokenPrice := trade.BaseTokenPriceUSD
	tokenPrice := trade.TokenPriceUSD
	if baseTokenPrice == 0 {
		baseTokenPrice = 161.876662583626140000
	}
	if tokenPrice == 0 {
		tokenPrice = 0.000004522833952587
	}

	switch {
	case errors.Is(err, solmodel.ErrNotFound) || (err != nil && strings.Contains(err.Error(), "record not found")):
		// 分支一：库中不存在该交易对，需要插入基础信息
		var baseTokenIsNativeToken, baseTokenIsToken0 int64
		if trade.PairInfo.BaseTokenIsNativeToken {
			baseTokenIsNativeToken = 1
		}

		if trade.PairInfo.BaseTokenIsToken0 {
			baseTokenIsToken0 = 1
		}

		fmt.Println("trade.BaseTokenPriceUSD is:", baseTokenPrice)
		fmt.Println("trade.TokenPriceUSD is:", tokenPrice)

		pairAtDB = &solmodel.Pair{
			ChainId:                      int64(chainId),
			Address:                      trade.PairAddr,
			Name:                         trade.SwapName,
			FactoryAddress:               "",
			BaseTokenAddress:             trade.PairInfo.BaseTokenAddr,
			TokenAddress:                 trade.PairInfo.TokenAddr,
			BaseTokenSymbol:              util.GetBaseToken(SolChainIdInt).Symbol,
			TokenSymbol:                  tokenSymbol,
			BaseTokenDecimal:             int64(trade.PairInfo.BaseTokenDecimal),
			TokenDecimal:                 int64(trade.PairInfo.TokenDecimal),
			BaseTokenIsNativeToken:       baseTokenIsNativeToken,
			BaseTokenIsToken0:            baseTokenIsToken0,
			CurrentBaseTokenAmount:       trade.CurrentBaseTokenInPoolAmount,
			CurrentTokenAmount:           trade.CurrentTokenInPoolAmount,
			Fdv:                          liq,
			MktCap:                       liq,
			Liquidity:                    liq,
			BlockNum:                     trade.PairInfo.BlockNum,
			BlockTime:                    time.Unix(trade.BlockTime, 0),
			Slot:                         trade.Slot,
			PumpPoint:                    trade.PumpPoint,
			PumpLaunched:                 util.BoolToInt64(trade.PumpLaunched),
			PumpMarketCap:                trade.PumpMarketCap,
			PumpOwner:                    trade.PumpOwner,
			PumpSwapPairAddr:             trade.PumpSwapPairAddr,
			PumpVirtualBaseTokenReserves: trade.PumpVirtualBaseTokenReserves,
			PumpVirtualTokenReserves:     trade.PumpVirtualTokenReserves,
			PumpStatus:                   int64(trade.PumpStatus),
			PumpPairAddr:                 trade.PumpPairAddr,
			LatestTradeTime:              time.Unix(trade.BlockTime, 0),
			BaseTokenPrice:               baseTokenPrice,
			TokenPrice:                   tokenPrice,
		}

		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv

		if trade.PairInfo.InitBaseTokenAmount > 0 && trade.PairInfo.InitTokenAmount > 0 {
			pairAtDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
			pairAtDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
		}

		//you should push here
		// if pairAtDB.Name =PumpFun and PumpPoint==0 ,这就是新token

		// Push new pump.fun token creation to WebSocket
		if pairAtDB.Name == constants.PumpFun || pairAtDB.Name == "PumpFun" && pairAtDB.PumpPoint == 0 {
			fmt.Println("new token created")
		}

		err = s.sc.PairModel.Insert(ctx, pairAtDB)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				// db already exists
				pairAtDB, err = s.sc.PairModel.FindOneByChainIdAddress(ctx, int64(chainId), trade.PairAddr)
				if err != nil {
					return nil, err
				}
				return pairAtDB, nil
			}
			err = fmt.Errorf("PairModel.Insert err:%w", err)
			return
		}

	case err == nil:
		// 分支二：交易对已存在，直接更新核心指标
		pairAtDB.CurrentBaseTokenAmount = trade.CurrentBaseTokenInPoolAmount
		pairAtDB.CurrentTokenAmount = trade.CurrentTokenInPoolAmount
		pairAtDB.Fdv = liq
		if trade.SwapName == constants.PumpFun {
			pairAtDB.Liquidity = trade.CurrentBaseTokenInPoolAmount * trade.BaseTokenPriceUSD * 2
		}
		pairAtDB.BaseTokenPrice = baseTokenPrice
		pairAtDB.TokenPrice = tokenPrice
		pairAtDB.Slot = trade.Slot
		pairAtDB.BlockTime = time.Unix(trade.BlockTime, 0)
		pairAtDB.Liquidity = liq

		// 其它可选字段也可同步更新
		// 保存到数据库
		err = s.sc.PairModel.Update(ctx, pairAtDB)
		if err != nil {
			err = fmt.Errorf("PairModel.Update err:%w", err)
			return
		}

		// 同步 trade 的市值等
		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv
		return
	default:
		err = fmt.Errorf("PairModel.FindOneByChainIdAddress err:%w", err)
		return
	}
	// logx.Infof("SavePair:%v db token price: %v, trade token price: %v", trade.PairAddr, pairAtDB.TokenPrice, trade.TokenPriceUSD)

	// 默认值
	trade.Mcap = pairAtDB.MktCap
	trade.Fdv = pairAtDB.Fdv

	if trade.Slot > pairAtDB.Slot {
		// 只有出现更高的 Slot 才刷新历史指标，避免旧数据覆盖新状态
		// s.Infof("SavePair will UpdatePairDBPoint slot: %v, db slot: %v, hash: %v, pair address: %v", trade.Slot, pairAtDB.Slot, trade.TxHash, trade.PairAddr)

		if pairAtDB.InitBaseTokenAmount == 0 || pairAtDB.InitTokenAmount == 0 {
			if trade.PairInfo.InitBaseTokenAmount > 0 && trade.PairInfo.InitTokenAmount > 0 {
				pairAtDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
				pairAtDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
			}
		}

		// s.initAmount(pairAtDB)

		pairAtDB.TokenSymbol = tokenSymbol
		pairAtDB.Slot = trade.Slot
		pairAtDB.Liquidity = liq
		err = UpdatePairDBPoint(trade, pairAtDB, tokenTotalSupply)
		if err != nil {
			err = fmt.Errorf("UpdatePairDBPoint err:%w", err)
			return
		}
		pairAtDB.BaseTokenPrice = baseTokenPrice
		pairAtDB.TokenPrice = tokenPrice

		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv

		err = s.sc.PairModel.Update(ctx, pairAtDB)
		if err != nil {
			err = fmt.Errorf("PairModel.Update err:%w", err)
			return
		}
	}

	return
}

// UpdatePairDBPoint 根据最新成交刷新交易对的价格、流动性和 Pump 指标。
func UpdatePairDBPoint(trade *types.TradeWithPair, pairDB *solmodel.Pair, tokenTotalSupply float64) error {
	currentTokenInPoolAmount := trade.CurrentTokenInPoolAmount
	currentBaseTokenInPoolAmount := trade.CurrentBaseTokenInPoolAmount
	baseTokenPriceUSD := trade.BaseTokenPriceUSD
	tokenPriceUSD := trade.TokenPriceUSD
	tradeTime := trade.BlockTime

	if pairDB.InitTokenAmount == 0 || pairDB.InitBaseTokenAmount == 0 {
		if trade.PairInfo.InitTokenAmount > 0 && trade.PairInfo.InitBaseTokenAmount > 0 {
			pairDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
			pairDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
			logx.Infof("UpdatePairDBPoint:update init token amount,swapName: %v, %v,%v", trade.SwapName, pairDB.InitTokenAmount, pairDB.InitBaseTokenAmount)
		}
	}

	pairDB.PumpPoint = trade.PumpPoint
	pairDB.PumpStatus = int64(trade.PumpStatus)
	pairDB.PumpVirtualBaseTokenReserves = trade.PumpVirtualBaseTokenReserves
	pairDB.PumpVirtualTokenReserves = trade.PumpVirtualTokenReserves
	// logx.Infof("UpdatePairDBPoint:update token address: %v pump ponit: %v", trade.PairInfo.TokenAddr, pairDB.PumpPoint)

	// Reset token price if base token liquidity is critically low, unless from specific swap types.
	// if trade.SwapName != util.SwapNamePump && currentBaseTokenInPoolAmount > 0 && currentBaseTokenInPoolAmount < 0.01 {
	// 	tokenPriceUSD = 0
	// }

	// Return early if the trade is older than the last update.
	// if tradeTime < pairDB.LatestTradeTime.Unix() {
	// 	return nil
	// }

	// Update token and base token prices only if valid.
	if tokenPriceUSD > 0 {
		pairDB.TokenPrice = tokenPriceUSD
		// logx.Infof("UpdatePairDBPoint %v db price:%v, trade price %v,", pairDB.Address, pairDB.TokenPrice, trade.TokenPriceUSD)
		// if trade.TokenPriceUSD != pairDB.TokenPrice {
		// 	logx.Infof("Diff UpdatePairDBPoint %v db price:%v, trade price %v,", pairDB.Address, pairDB.TokenPrice, trade.TokenPriceUSD)
		// }
	}
	pairDB.BaseTokenPrice = baseTokenPriceUSD

	// Update FDV (fully diluted valuation) based on token supply.
	if tokenTotalSupply > 0 {
		pairDB.Fdv = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(tokenTotalSupply)).InexactFloat64()
		pairDB.MktCap = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(tokenTotalSupply)).InexactFloat64()
	}

	// Update current liquidity only if both amounts are positive.
	if currentBaseTokenInPoolAmount > 0 && currentTokenInPoolAmount > 0 {
		pairDB.CurrentBaseTokenAmount = currentBaseTokenInPoolAmount
		pairDB.CurrentTokenAmount = currentTokenInPoolAmount
	}

	// Update the latest trade time.
	pairDB.LatestTradeTime = time.Unix(tradeTime, 0)

	// Calculate market cap based on the current liquidity and prices.
	if pairDB.Name == constants.PumpFun {
		// PumpFun 使用对半占比的方式估算池子总流动性
		pairDB.Liquidity = decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount)).Mul(decimal.NewFromFloat(2)).InexactFloat64()
	} else {
		pairDB.Liquidity = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentTokenAmount)).
			Add(decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount))).InexactFloat64()
	}

	// pairDB.MktCap = tokenPriceUSD*pairDB.CurrentTokenAmount + baseTokenPriceUSD*pairDB.CurrentBaseTokenAmount

	// TODO: Update pair cache.
	// pair.PairCache.Update(pairDB)
	return nil
}
