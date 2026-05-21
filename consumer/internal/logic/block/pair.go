package block

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constants"
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
	if tokenTotalSupply == 0 && trade.PairInfo.TokenTotalSupply > 0 {
		tokenTotalSupply = trade.PairInfo.TokenTotalSupply
	}

	// 第一步：尝试读取现有交易对信息，判断是新增还是增量更新
	pairAtDB, err = s.sc.PairModel.FindOneByChainIdAddress(ctx, int64(chainId), trade.PairAddr)
	if err != nil {
		s.Errorf("SavePair:FindOneByChainIdAddress err: %v, pair address: %v", err, trade.PairAddr)
	}
	// 根据成交信息估算即时流动性，后续用于更新市值、流动性指标
	// 注意：这个值只是初始值，最终会在 UpdatePairDBPoint 中重新计算
	liq := trade.CurrentBaseTokenInPoolAmount*trade.BaseTokenPriceUSD + trade.CurrentTokenInPoolAmount*trade.TokenPriceUSD
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
			BaseTokenSymbol:              trade.PairInfo.BaseTokenSymbol, // 使用从交易对信息中获取的 base token symbol
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
			go func() {
				fmt.Printf("🆕 [NEW PUMP TOKEN] Broadcasting: %s (%s)\n", pairAtDB.TokenSymbol, pairAtDB.TokenAddress)

				pushReq := &market.PushTokenInfoRequest{
					ChainId:      pairAtDB.ChainId,
					TokenAddress: pairAtDB.TokenAddress,
					PairAddress:  pairAtDB.Address,
					TokenPrice:   pairAtDB.TokenPrice,
					MktCap:       pairAtDB.MktCap,
					TokenName:    "", // Will be populated by market service from token database
					TokenSymbol:  pairAtDB.TokenSymbol,
					TokenIcon:    "", // Will be populated by market service from token database
					LaunchTime:   pairAtDB.BlockTime.Unix(),
					HoldCount:    0,   // Will be calculated by market service
					Change_24:    0.0, // Will be calculated by market service
					Txs_24H:      0,   // Will be calculated by market service
					PumpStatus:   int32(pairAtDB.PumpStatus),
				}

				_, err := s.sc.MarketService.PushTokenInfo(ctx, pushReq)
				if err != nil {
					fmt.Printf("❌ [NEW PUMP TOKEN] Failed to push: %v\n", err)
				} else {
					fmt.Printf("✅ [NEW PUMP TOKEN] Successfully pushed: %s\n", pairAtDB.TokenSymbol)
				}
			}()
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
		// 交易对已存在，后续根据 slot 判断是否需要增量更新
		break
	default:
		err = fmt.Errorf("PairModel.FindOneByChainIdAddress err:%w", err)
		return
	}
	// logx.Infof("SavePair:%v db token price: %v, trade token price: %v", trade.PairAddr, pairAtDB.TokenPrice, trade.TokenPriceUSD)

	prevSlot := pairAtDB.Slot
	if trade.Slot <= prevSlot {
		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv
		return
	}

	// 默认值
	trade.Mcap = pairAtDB.MktCap
	trade.Fdv = pairAtDB.Fdv

	if pairAtDB.InitBaseTokenAmount == 0 || pairAtDB.InitTokenAmount == 0 {
		if trade.PairInfo.InitBaseTokenAmount > 0 && trade.PairInfo.InitTokenAmount > 0 {
			pairAtDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
			pairAtDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
		}
	}

	pairAtDB.TokenSymbol = tokenSymbol
	pairAtDB.BlockTime = time.Unix(trade.BlockTime, 0)
	pairAtDB.Slot = trade.Slot
	pairAtDB.Liquidity = liq
	err = s.UpdatePairDBPoint(ctx, trade, pairAtDB, tokenTotalSupply)
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

	return
}

// UpdatePairDBPoint 根据最新成交刷新交易对的价格、流动性和 Pump 指标。
func (s *BlockService) UpdatePairDBPoint(ctx context.Context, trade *types.TradeWithPair, pairDB *solmodel.Pair, tokenTotalSupply float64) error {
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

	// Update token and base token prices only if valid.
	if tokenPriceUSD > 0 {
		pairDB.TokenPrice = tokenPriceUSD
	} else if pairDB.TokenPrice > 0 {
		tokenPriceUSD = pairDB.TokenPrice
	}

	if baseTokenPriceUSD > 0 {
		pairDB.BaseTokenPrice = baseTokenPriceUSD
	} else if pairDB.BaseTokenPrice > 0 {
		baseTokenPriceUSD = pairDB.BaseTokenPrice
	}

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
	} else if pairDB.Name == constants.RaydiumConcentratedLiquidity {
		// CLMM 池子的流动性计算
		// 如果 CurrentBaseTokenAmount 和 CurrentTokenAmount 都为 0，尝试从数据库获取 vault balances
		if pairDB.CurrentBaseTokenAmount == 0 && pairDB.CurrentTokenAmount == 0 {
			// 尝试从 clmm_pool_info 表获取 vault balances
			if poolInfoV1, err := s.sc.SolRaydiumCLMMPoolV1Model.FindOneByPoolState(ctx, trade.PairAddr); err == nil && poolInfoV1 != nil {
				// 从链上获取 vault balances，根据 mint 地址判断对应关系
				if cli := s.sc.GetSolClient(); cli != nil {
					// 获取 InputVault 余额
					if resp, err := cli.GetTokenAccountBalance(ctx, poolInfoV1.InputVault); err == nil && resp.Amount > 0 {
						amount := float64(resp.Amount) / math.Pow10(int(resp.Decimals))
						// 根据 mint 地址判断是 base token 还是 token
						if poolInfoV1.InputVaultMint == trade.PairInfo.BaseTokenAddr {
							pairDB.CurrentBaseTokenAmount = amount
						} else if poolInfoV1.InputVaultMint == trade.PairInfo.TokenAddr {
							pairDB.CurrentTokenAmount = amount
						}
					}
					// 获取 OutputVault 余额
					if resp, err := cli.GetTokenAccountBalance(ctx, poolInfoV1.OutputVault); err == nil && resp.Amount > 0 {
						amount := float64(resp.Amount) / math.Pow10(int(resp.Decimals))
						// 根据 mint 地址判断是 base token 还是 token
						if poolInfoV1.OutputVaultMint == trade.PairInfo.BaseTokenAddr {
							pairDB.CurrentBaseTokenAmount = amount
						} else if poolInfoV1.OutputVaultMint == trade.PairInfo.TokenAddr {
							pairDB.CurrentTokenAmount = amount
						}
					}
				}
			} else if poolInfoV2, err := s.sc.SolRaydiumCLMMPoolV2Model.FindOneByPoolState(ctx, trade.PairAddr); err == nil && poolInfoV2 != nil {
				// 从链上获取 vault balances，根据 mint 地址判断对应关系
				if cli := s.sc.GetSolClient(); cli != nil {
					// 获取 InputVault 余额
					if resp, err := cli.GetTokenAccountBalance(ctx, poolInfoV2.InputVault); err == nil && resp.Amount > 0 {
						amount := float64(resp.Amount) / math.Pow10(int(resp.Decimals))
						// 根据 mint 地址判断是 base token 还是 token
						if poolInfoV2.InputVaultMint == trade.PairInfo.BaseTokenAddr {
							pairDB.CurrentBaseTokenAmount = amount
						} else if poolInfoV2.InputVaultMint == trade.PairInfo.TokenAddr {
							pairDB.CurrentTokenAmount = amount
						}
					}
					// 获取 OutputVault 余额
					if resp, err := cli.GetTokenAccountBalance(ctx, poolInfoV2.OutputVault); err == nil && resp.Amount > 0 {
						amount := float64(resp.Amount) / math.Pow10(int(resp.Decimals))
						// 根据 mint 地址判断是 base token 还是 token
						if poolInfoV2.OutputVaultMint == trade.PairInfo.BaseTokenAddr {
							pairDB.CurrentBaseTokenAmount = amount
						} else if poolInfoV2.OutputVaultMint == trade.PairInfo.TokenAddr {
							pairDB.CurrentTokenAmount = amount
						}
					}
				}
			}
		}
		// 计算 CLMM 流动性
		pairDB.Liquidity = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentTokenAmount)).
			Add(decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount))).InexactFloat64()
	} else {
		pairDB.Liquidity = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentTokenAmount)).
			Add(decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount))).InexactFloat64()
	}

	// pairDB.MktCap = tokenPriceUSD*pairDB.CurrentTokenAmount + baseTokenPriceUSD*pairDB.CurrentBaseTokenAmount

	// TODO: Update pair cache.
	// pair.PairCache.Update(pairDB)
	return nil
}
