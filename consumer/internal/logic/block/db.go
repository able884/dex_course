package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	set "github.com/duke-git/lancet/v2/datastructure/set"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constrants"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/types"
)

// NewTradeModel 将链上成交数据转换为数据库模型，便于后续批量写入。
func (s *BlockService) NewTradeModel(trade *types.TradeWithPair) (tradeDb *solmodel.Trade) {
	if trade == nil {
		s.Errorf("NewTradeModel: trade is nil, returning nil")
		return
	}

	s.Infof("NewTradeModel: Converting trade - TxHash: %s, PairAddr: %s, Type: %s",
		trade.TxHash, trade.PairAddr, trade.Type)

	now := time.Now()
	tradeDb = &solmodel.Trade{
		HashId:            trade.HashId,
		ChainId:           SolChainIdInt,
		PairAddr:          trade.PairAddr,
		TxHash:            trade.TxHash,
		Maker:             trade.Maker,
		TradeType:         trade.Type,
		BaseTokenAmount:   trade.BaseTokenAmount,
		TokenAmount:       trade.TokenAmount,
		BaseTokenPriceUsd: trade.BaseTokenPriceUSD,
		TotalUsd:          trade.TotalUSD,
		TokenPriceUsd:     trade.TokenPriceUSD,
		To:                trade.To,
		BlockNum:          trade.BlockNum,
		BlockTime:         time.Unix(trade.BlockTime, 0),
		BlockTimeStamp:    trade.BlockTime,
		SwapName:          trade.SwapName,

		CreatedAt: now,
		UpdatedAt: now,
	}

	s.Infof("NewTradeModel: Successfully created trade model - HashId: %s, ChainId: %d",
		tradeDb.HashId, tradeDb.ChainId)
	return
}

// SaveTrades 按交易对并行落库成交数据，同时过滤掉无效记录。
func (s *BlockService) SaveTrades(ctx context.Context, chainId int64, tradeMap map[string][]*types.TradeWithPair) {
	s.Infof("SaveTrades: Starting with %d pair addresses", len(tradeMap))

	group := threading.NewRoutineGroup()
	for key, trade := range tradeMap {
		s.Infof("SaveTrades: Processing pair %s with %d trades", key, len(trade))

		// 1. 预过滤：仅保留有效的买卖行为，避免写入无价格或类型异常的记录
		trade = slice.Filter[*types.TradeWithPair](trade, func(index int, item *types.TradeWithPair) bool {
			if item == nil {
				s.Infof("SaveTrades: Filtered out nil trade at index %d", index)
				return false
			}

			// Normal filtering for buy/sell trades
			if item.Type != types.TradeTypeBuy && item.Type != types.TradeTypeSell {
				s.Infof("SaveTrades: Filtered out trade with invalid type %s at index %d", item.Type, index)
				return false
			}
			if item.TokenPriceUSD == 0 {
				s.Infof("SaveTrades: Filtered out trade with zero price at index %d", index)
				return false
			}
			return true
		})

		s.Infof("SaveTrades: After filtering, pair %s has %d valid trades", key, len(trade))

		group.RunSafe(func(key string, trade []*types.TradeWithPair) func() {
			return func() {
				// 2. 并行写库：每个交易对独立处理，避免互相阻塞
				txhashes := slice.Map[*types.TradeWithPair, string](trade, func(_ int, item *types.TradeWithPair) string {
					return item.TxHash
				})
				s.Infof("SaveTrades: will BatchSaveByTrade key: %v, tx hashes: %v", key, txhashes)

				err := s.BatchSaveByTrade(ctx, chainId, key, trade)
				if err != nil {
					s.Errorf("SaveTrades: BatchSaveByTrade err:%v, key:%v, tx hashes: %v", err, key, txhashes)
				} else {
					s.Infof("SaveTrades: Successfully processed pair %s with %d trades", key, len(trade))
				}
			}
		}(key, trade))
	}
	s.Infof("SaveTrades: Waiting for all goroutines to complete")
	group.Wait()
	s.Infof("SaveTrades: All trades processed successfully")
}

// BatchSaveByTrade 针对单个交易对执行配对信息与成交数据的落库。
func (s *BlockService) BatchSaveByTrade(ctx context.Context, chainId int64, pairAddress string, trades []*types.TradeWithPair) (err error) {
	if err = s.SavePairInfo(ctx, chainId, pairAddress, trades); err != nil {
		s.Error(fmt.Errorf("batchSaveByTrade:savePairInfo err:%v", err))
	}
	if err = s.BatchSaveTrade(ctx, trades); err != nil {
		s.Error(fmt.Errorf("batchSaveByTrade:saveTrade err:%w", err))
	}
	return
}

// SavePairInfo 负责同步交易对的基础信息以及关联 Token 信息。
func (s *BlockService) SavePairInfo(ctx context.Context, chainId int64, pairAddress string, trades []*types.TradeWithPair) (err error) {
	fmt.Println("SavePairInfo: 开始保存pair信息")

	if trades == nil {
		// 理论上不会出现，做保护避免 panic
		fmt.Println("trades is:", trades[0].TxHash)
		return
	}

	if len(trades) == 0 {
		s.Errorf("SavePairInfo: trades is empty, returning early")
		return nil
	}

	// 1. 选择最新成交作为基准，并同步 Token 信息（总量、符号等可能被刷新）
	trade := trades[len(trades)-1]
	var tokenDb *solmodel.Token
	tokenDb, err = s.SaveToken(ctx, trade)
	if err != nil || tokenDb == nil {
		s.Error("SavePairInfo:SaveToken err:", err)
		return err
	}

	if tokenDb.TotalSupply == 0 {
		s.Errorf("savePairInfo token totalSupply is 0, tokenDb: %#v", tokenDb)
	}

	// 2. 将最新的 Token 总量透传给同批次的所有成交（用于估算市值）
	for _, tradeInfo := range trades {
		tradeInfo.PairInfo.TokenTotalSupply = tokenDb.TotalSupply
	}

	// 3. 同步或创建交易对信息，并记录 Pump 指标
	_, err = s.SavePair(ctx, trade, tokenDb)
	if err != nil {
		fmt.Println("SavePair err: %v", err)
	}

	// 4. 统一回写市值结果，确保 MQ / 存储端数据一致
	for _, tradeInfo := range trades {
		tradeInfo.Mcap = trade.Mcap
		tradeInfo.Fdv = trade.Fdv
	}
	return
}

// BatchSaveTrade 批量写入成交记录，写入前会生成对应的数据库结构。
func (s *BlockService) BatchSaveTrade(ctx context.Context, trades []*types.TradeWithPair) error {
	s.Infof("BatchSaveTrade: Starting with %d trades", len(trades))

	if trades == nil {
		s.Infof("BatchSaveTrade: trades is nil, returning early")
		return nil
	}

	if len(trades) == 0 {
		s.Infof("BatchSaveTrade: trades slice is empty, returning early")
		return nil
	}

	// Log some trade details for debugging
	for i, trade := range trades {
		if i < 3 { // Only log first 3 trades to avoid spam
			s.Infof("BatchSaveTrade: Trade[%d] - TxHash: %s, PairAddr: %s, Type: %s, TokenAmount: %f",
				i, trade.TxHash, trade.PairAddr, trade.Type, trade.TokenAmount)
		}
	}

	s.Infof("BatchSaveTrade: Converting trades to database models")
	// 2. 数据转换：将业务对象映射为 ORM 结构，方便批量插入
	tradeDbs := slice.Map[*types.TradeWithPair, *solmodel.Trade](trades, func(_ int, trade *types.TradeWithPair) *solmodel.Trade {
		return s.NewTradeModel(trade)
	})

	s.Infof("BatchSaveTrade: Converted %d trades to database models", len(tradeDbs))

	// Log some database model details
	for i, tradeDb := range tradeDbs {
		if i < 3 { // Only log first 3 to avoid spam
			s.Infof("BatchSaveTrade: TradeDb[%d] - TxHash: %s, PairAddr: %s, TradeType: %s",
				i, tradeDb.TxHash, tradeDb.PairAddr, tradeDb.TradeType)
		}
	}

	s.Infof("BatchSaveTrade: Calling BatchInsertTrades with %d trades", len(tradeDbs))
	err := s.sc.TradeModel.BatchInsertTrades(ctx, tradeDbs)
	if err != nil {
		s.Errorf("BatchSaveTrade: BatchInsertTrades failed with error: %v", err)
		return err
	}

	s.Infof("BatchSaveTrade: Successfully inserted %d trades", len(tradeDbs))
	return nil
}

// UpdateTokenMints 扫描 Mint 交易并刷新 Token 的最新总发行量。
func (s *BlockService) UpdateTokenMints(ctx context.Context, tokenMints []*types.TradeWithPair) {
	client := s.sc.GetSolClient()

	hashSet := set.New[string]()

	slice.ForEach(tokenMints, func(_ int, item *types.TradeWithPair) {
		if item != nil && item.Type == types.TradeTokenMint {
			mintTo := item.InstructionMintTo
			// 同一 Mint 在单批次内只需更新一次，避免重复请求链上数据
			if hashSet.Contain(mintTo.Mint.String()) {
				return
			}
			token, err := s.sc.TokenModel.FindOneByChainIdAddress(s.ctx, int64(item.ChainIdInt), mintTo.Mint.String())
			if err == nil && token != nil {
				totalSupply, err := sol.GetTokenTotalSupply(client, s.ctx, mintTo.Mint.String())
				if err == nil && totalSupply.IsPositive() {
					token.TotalSupply = totalSupply.InexactFloat64()
					s.Infof("UpdateTokenMints: update totalSupply, token address: %v, total supply: %v, tx hash: %v", token.Address, token.TotalSupply, item.TxHash)
					hashSet.Add(mintTo.Mint.String())
					_ = s.sc.TokenModel.Update(ctx, token)
				}
			}
		}
	})
}

// UpdateTokenBurns 扫描 Burn 交易并更新 Token 的发行量缓存。
func (s *BlockService) UpdateTokenBurns(ctx context.Context, tokenBurns []*types.TradeWithPair) {
	client := s.sc.GetSolClient()

	hashSet := set.New[string]()

	slice.ForEach(tokenBurns, func(_ int, item *types.TradeWithPair) {
		if item == nil {
			return
		}
		if item != nil && (item.Type == types.TradeTokenBurn || item.Type == "token_burn") {
			burn := item.InstructionBurn
			// 过滤同一 Mint 的重复更新，减轻数据库与链上读取压力
			if hashSet.Contain(burn.Mint.String()) {
				return
			}
			token, err := s.sc.TokenModel.FindOneByChainIdAddress(s.ctx, int64(item.ChainIdInt), burn.Mint.String())
			if err == nil && token != nil {
				totalSupply, err := sol.GetTokenTotalSupply(client, s.ctx, burn.Mint.String())
				if err == nil && totalSupply.IsPositive() {
					token.TotalSupply = totalSupply.InexactFloat64()
					s.Infof("UpdateTokenBurns: update totalSupply, token address: %v, total supply: %v, tx hash: %v", token.Address, token.TotalSupply, item.TxHash)
					hashSet.Add(burn.Mint.String())
					_ = s.sc.TokenModel.Update(ctx, token)
				}
			}
		}
	})
}

// SavePumpSwapPoolInfo 将 PumpSwap 的池子元信息保存至数据库，避免重复写入。
func (s *BlockService) SavePumpSwapPoolInfo(ctx context.Context, pair *types.TradeWithPair) (err error) {
	if pair.SwapName != constants.PumpSwap && pair.SwapName != "PumpSwap" {
		return nil
	}

	if pair.PumpAmmInfo != nil {
		// 查询是否已入库，避免重复写入同一池子的静态信息
		_, err := s.sc.PumpAmmInfoModel.FindOneByPoolAccount(ctx, pair.PumpAmmInfo.PoolAccount)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, solmodel.ErrNotFound) || err.Error() == "record not found":
			if err = s.sc.PumpAmmInfoModel.Insert(ctx, pair.PumpAmmInfo); err != nil {
				if !strings.Contains(err.Error(), "Duplicate entry") {
					return err
				} else {
					return nil
				}
			}
		default:
			err = fmt.Errorf("SavePumpSwapPoolInfo:777 PumpAmmInfoModel.FindOneByPoolAccount err:%w", err)
		}
	}

	return
}

// SaveTokenAccounts 批量同步交易涉及的 TokenAccount 信息，便于后续余额统计。
func (s *BlockService) SaveTokenAccounts(ctx context.Context, trades []*types.TradeWithPair, tokenAccountMap map[string]*TokenAccount) {
	var tokenAccounts []*solmodel.SolTokenAccount

	for _, tokenAccount := range tokenAccountMap {

		status := 0
		if tokenAccount.Closed {
			status = 1
		}

		if tokenAccount.TokenAddress == constants.TokenStrWrapSol {
			continue
		}
		solTokenAccount := &solmodel.SolTokenAccount{
			OwnerAddress:        tokenAccount.Owner,
			Status:              int64(status),
			ChainId:             SolChainIdInt,
			TokenAccountAddress: tokenAccount.TokenAccountAddress,
			TokenAddress:        tokenAccount.TokenAddress,        // token_address
			TokenDecimal:        int64(tokenAccount.TokenDecimal), // token_decimal
			Balance:             tokenAccount.PostValue,           // token balance
			Slot:                int64(s.slot),                    // 开启统计高度
		}
		tokenAccounts = append(tokenAccounts, solTokenAccount)
	}

	// remove dup
	slice.Reverse(tokenAccounts)
	tokenAccounts = slice.UniqueByComparator[*solmodel.SolTokenAccount](tokenAccounts, func(item *solmodel.SolTokenAccount, other *solmodel.SolTokenAccount) bool {
		if item.OwnerAddress == other.OwnerAddress && item.TokenAccountAddress == other.TokenAccountAddress {
			s.Errorf("SaveTokenAccounts:UniqueByComparator dup token address: %v, account1: %v, account2: %v", item.TokenAddress, item.Balance, other.Balance)
			return true
		}
		return false
	})
	slice.Reverse(tokenAccounts)

	m := make(map[string]time.Time)
	countMap := make(map[string]int)
	slice.ForEach[*solmodel.SolTokenAccount](tokenAccounts, func(_ int, sta *solmodel.SolTokenAccount) {
		if _, ok := m[fmt.Sprintf("%v_%v", sta.ChainId, sta.TokenAddress)]; ok {
			countMap[fmt.Sprintf("%v_%v", sta.ChainId, sta.TokenAddress)] += 1
			return
		}
		address, err := s.sc.TokenModel.FindOneByChainIdAddress(s.ctx, sta.ChainId, sta.TokenAddress)
		if err != nil || address == nil {
			return
		}
		m[fmt.Sprintf("%v_%v", sta.ChainId, sta.TokenAddress)] = address.CreatedAt
		countMap[fmt.Sprintf("%v_%v", sta.ChainId, sta.TokenAddress)] = 1
	})

	tokenAccounts = slice.Filter[*solmodel.SolTokenAccount](tokenAccounts, func(_ int, sta *solmodel.SolTokenAccount) bool {
		if value, ok := m[fmt.Sprintf("%v_%v", sta.ChainId, sta.TokenAddress)]; ok {
			sta.CreatedAt = value
			return true
		}
		return false
	})

	err := s.sc.SolTokenAccountModel.BatchInsertTokenAccounts(ctx, tokenAccounts)
	if err != nil {
		s.Error("tokenAccountModel.BatchSave err:", err)
	}
}
