package block

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	set "github.com/duke-git/lancet/v2/datastructure/set"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/ethereum/go-ethereum/log"
	bin "github.com/gagliardetto/binary"
	"github.com/zeromicro/go-zero/core/threading"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/pkg/sol"
	"richcode.cc/dex/pkg/transfer"
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

	pairOwners := make(map[string]struct{})
	for _, trade := range trades {
		if trade == nil || len(trade.PairAddr) == 0 {
			continue
		}
		pairOwners[strings.ToLower(trade.PairAddr)] = struct{}{}
	}

	processedAccounts := make(map[string]struct{})

	for _, tokenAccount := range tokenAccountMap {
		if tokenAccount == nil {
			continue
		}
		ownerNormalized := strings.ToLower(tokenAccount.Owner)
		if _, ok := pairOwners[ownerNormalized]; ok {
			continue
		}

		status := 0
		if tokenAccount.Closed {
			status = 1
		}

		if tokenAccount.TokenAddress == constants.TokenStrWrapSol {
			continue
		}

		accountKey := fmt.Sprintf("%s_%s", ownerNormalized, strings.ToLower(tokenAccount.TokenAccountAddress))
		if _, ok := processedAccounts[accountKey]; ok {
			continue
		}
		processedAccounts[accountKey] = struct{}{}

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

	// 检查每个账户是否已存在，存在则更新，不存在则插入
	log.Info("本次保存TOKEN_ACCOUNT数量:", len(tokenAccounts))
	err := s.sc.SolTokenAccountModel.BatchUpsertTokenAccounts(ctx, tokenAccounts)
	if err != nil {
		s.Error("tokenAccountModel.BatchUpsertTokenAccounts err:", err)
		return
	}

	// 保存成功后，将数据缓存到Redis，设置24小时过期时间
	s.cacheTokenAccountsToRedis(ctx, tokenAccounts)
}

// cacheTokenAccountsToRedis 将TokenAccount数据缓存到Redis，设置24小时过期时间
func (s *BlockService) cacheTokenAccountsToRedis(ctx context.Context, tokenAccounts []*solmodel.SolTokenAccount) {
	if s.sc.Redis == nil {
		s.Infof("cacheTokenAccountsToRedis: Redis is not configured, skipping cache")
		return
	}

	// 设置24小时过期时间
	expireTime := 24 * time.Hour

	for _, account := range tokenAccounts {
		// 使用 owner_address 作为缓存key
		cacheKey := s.getTokenAccountCacheKey(account.OwnerAddress, account.TokenAccountAddress)

		// 将账户信息序列化为JSON字符串
		accountData, err := transfer.Struct2String(account)
		if err != nil {
			s.Errorf("cacheTokenAccountsToRedis: failed to serialize account, owner: %s, err: %v", account.OwnerAddress, err)
			continue
		}

		// 存入Redis
		err = s.sc.Redis.Setex(cacheKey, accountData, int(expireTime.Seconds()))
		if err != nil {
			s.Errorf("cacheTokenAccountsToRedis: failed to cache account, owner: %s, err: %v", account.OwnerAddress, err)
		} else {
			s.Infof("cacheTokenAccountsToRedis: cached account, owner: %s, key: %s", account.OwnerAddress, cacheKey)
		}
	}
}

// getTokenAccountCacheKey 生成TokenAccount的Redis缓存key
func (s *BlockService) getTokenAccountCacheKey(ownerAddress, tokenAccountAddress string) string {
	return fmt.Sprintf("token_account:%s:%s", ownerAddress, tokenAccountAddress)
}

// checkTokenAccountExistsWithCache 检查TokenAccount是否存在，先查Redis缓存，查不到再查数据库
func (s *BlockService) checkTokenAccountExistsWithCache(ctx context.Context, ownerAddress, tokenAccountAddress string) (exists bool, account *solmodel.SolTokenAccount) {
	if s.sc.Redis != nil {
		cacheKey := s.getTokenAccountCacheKey(ownerAddress, tokenAccountAddress)
		accountData, err := s.sc.Redis.Get(cacheKey)

		if err == nil && accountData != "" {
			// 缓存命中
			cachedAccount, err := transfer.String2Struct[solmodel.SolTokenAccount](accountData)
			if err == nil {
				s.Infof("checkTokenAccountExistsWithCache: cache hit for owner: %s", ownerAddress)
				return true, &cachedAccount
			}
			s.Errorf("checkTokenAccountExistsWithCache: failed to deserialize cached account, owner: %s, err: %v", ownerAddress, err)
		}
	}

	return false, nil
}

func (s *BlockService) SaveRaydiumCLMMPoolInfo(ctx context.Context, pair *types.TradeWithPair) (err error) {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			var txHash string
			if pair != nil {
				txHash = pair.TxHash
			} else {
				txHash = "unknown"
			}
			s.Errorf("SaveRaydiumCLMMPoolInfo: panic occurred: %v, tx hash: %v", r, txHash)
			err = fmt.Errorf("SaveRaydiumCLMMPoolInfo: panic occurred: %v", r)
		}
	}()

	if pair.SwapName != constants.RaydiumConcentratedLiquidity {
		s.Infof("SaveRaydiumCLMMPoolInfo: Skipping - not a CLMM pool. SwapName: %s", pair.SwapName)
		return nil
	}

	if pair.ClmmPoolInfoV1 == nil && pair.ClmmPoolInfoV2 == nil {
		s.Infof("SaveRaydiumCLMMPoolInfo: v1 and v2 are nil, Skipping - no CLMM pool info. SwapName: %s", pair.SwapName)
		return nil
	}

	if pair.ClmmPoolInfoV1 != nil {
		// Get pool state string safely
		var poolStateStr string
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Errorf("SaveRaydiumCLMMPoolInfo: panic getting poolState string: %v, tx hash: %v", r, pair.TxHash)
					poolStateStr = ""
				}
			}()
			poolStateStr = pair.ClmmPoolInfoV1.PoolState.PubKey.String()
		}()

		if poolStateStr == "" {
			return fmt.Errorf("SaveRaydiumCLMMPoolInfo: failed to get pool state string")
		}

		// Process remaining accounts safely
		var remainingAccountsStr string
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Errorf("SaveRaydiumCLMMPoolInfo: panic processing remaining accounts: %v, tx hash: %v", r, pair.TxHash)
					remainingAccountsStr = "[]"
				}
			}()

			remainingAccounts := make([]string, 0)
			for _, account := range pair.ClmmPoolInfoV1.RemainingAccounts {
				remainingAccounts = append(remainingAccounts, account.PubKey.String())
			}
			remainingAccountsStr, _ = transfer.Struct2String(remainingAccounts)
		}()

		now := time.Now()

		// Find existing pool
		dbPool, err := s.sc.SolRaydiumCLMMPoolV1Model.FindOneByPoolState(ctx, poolStateStr)

		fmt.Println("pair.ClmmPoolInfoV1.TickArray is:", pair.ClmmPoolInfoV1.TickArray)
		if pair.ClmmPoolInfoV1 != nil && dbPool != nil {
			dbPool.TickArray = pair.ClmmPoolInfoV1.TickArray.String()
		}

		// Instead, always use the JSON array string
		if dbPool != nil {
			dbPool.RemainingAccounts = remainingAccountsStr
		}

		switch {
		case err == nil:
			// Update existing pool
			if dbPool != nil {
				dbPool.UpdatedAt = now
				func() {
					defer func() {
						if r := recover(); r != nil {
							s.Errorf("SaveRaydiumCLMMPoolInfo: panic in Update: %v, tx hash: %v", r, pair.TxHash)
						}
					}()
					_ = s.sc.SolRaydiumCLMMPoolV1Model.Update(ctx, dbPool)
				}()
				s.Infof("SaveRaydiumCLMMPoolInfo:FindOneByPoolState v1 update success, id: %v, hash: %v", poolStateStr, pair.TxHash)
			}
			return nil
		case errors.Is(err, solmodel.ErrNotFound) || strings.Contains(err.Error(), "record not found") || err.Error() == "sql: no rows in result set":
			// Create new pool record
			var ammConfigStr, inputVaultStr, outputVaultStr, observationStateStr, tokenProgramStr, tokenProgram2022Str, memoProgramStr, inputVaultMintStr, outputVaultMintStr, tickArrayStr string

			// Safely get all the PubKey strings
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.Errorf("SaveRaydiumCLMMPoolInfo: panic getting ClmmPoolInfoV1 PubKey strings: %v, tx hash: %v", r, pair.TxHash)
					}
				}()
				ammConfigStr = pair.ClmmPoolInfoV1.AmmConfig.String()
				inputVaultStr = pair.ClmmPoolInfoV1.InputVault.PubKey.String()
				outputVaultStr = pair.ClmmPoolInfoV1.OutputVault.PubKey.String()
				observationStateStr = pair.ClmmPoolInfoV1.ObservationState.PubKey.String()
				tokenProgramStr = pair.ClmmPoolInfoV1.TokenProgram.String()
				tokenProgram2022Str = pair.ClmmPoolInfoV1.TokenProgram2022.String()
				memoProgramStr = pair.ClmmPoolInfoV1.MemoProgram.String()
				inputVaultMintStr = pair.ClmmPoolInfoV1.InputVaultMint.String()
				outputVaultMintStr = pair.ClmmPoolInfoV1.OutputVaultMint.String()
				tickArrayStr = pair.ClmmPoolInfoV1.TickArray.String()

				s.Infof("SaveRaydiumCLMMPoolInfo: ClmmPoolInfoV1 details - InputVaultMint: %s, OutputVaultMint: %s, PoolState: %s",
					inputVaultMintStr, outputVaultMintStr, poolStateStr)
			}()

			info := &solmodel.ClmmPoolInfoV1{
				AmmConfig:         ammConfigStr,
				PoolState:         poolStateStr,
				InputVault:        inputVaultStr,
				OutputVault:       outputVaultStr,
				ObservationState:  observationStateStr,
				TokenProgram:      tokenProgramStr,
				TokenProgram2022:  tokenProgram2022Str,
				MemoProgram:       memoProgramStr,
				InputVaultMint:    inputVaultMintStr,
				OutputVaultMint:   outputVaultMintStr,
				TickArray:         tickArrayStr,
				RemainingAccounts: remainingAccountsStr,
				TxHash:            pair.TxHash,
				TradeFeeRate:      int64(pair.ClmmPoolInfoV1.TradeFeeRate),
				CreatedAt:         now,
				UpdatedAt:         now,
			}

			// Safely insert record
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("SaveRaydiumCLMMPoolInfo: panic in Insert: %v", r)
					}
				}()
				err = s.sc.SolRaydiumCLMMPoolV1Model.Insert(ctx, info)
			}()

			if err != nil {
				if !strings.Contains(err.Error(), "Duplicate entry") {
					s.Errorf("SaveRaydiumCLMMPoolInfo: Failed to insert CLMM V1 pool: %v, txHash: %s, poolState: %s",
						err, pair.TxHash, poolStateStr)
					return fmt.Errorf("SaveRaydiumCLMMPoolInfo:SolRaydiumCLMMPoolV1Model.Insert %#v err:%v", info, err)
				}
				s.Infof("SaveRaydiumCLMMPoolInfo: Duplicate CLMM V1 pool entry, skipping: %s", poolStateStr)
				return nil
			}
			s.Infof("SaveRaydiumCLMMPoolInfo: Successfully inserted CLMM V1 pool: %s, txHash: %s, InputMint: %s, OutputMint: %s",
				poolStateStr, pair.TxHash, info.InputVaultMint, info.OutputVaultMint)
		default:
			s.Errorf("SaveRaydiumCLMMPoolInfo: Failed to find CLMM V1 pool: %v, txHash: %s, poolState: %s",
				err, pair.TxHash, poolStateStr)
			return fmt.Errorf("SaveRaydiumCLMMPoolInfo:SolRaydiumCLMMPoolV1Model.FindOneByPoolState err:%w", err)
		}
	}

	if pair.ClmmPoolInfoV2 != nil {
		// Safely print ClmmPoolInfoV2 without causing panics
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Errorf("SaveRaydiumCLMMPoolInfo: panic printing ClmmPoolInfoV2: %v, tx hash: %v", r, pair.TxHash)
				}
			}()
			s.Infof("SaveRaydiumCLMMPoolInfo: Processing CLMM V2 pool for txHash: %s", pair.TxHash)
		}()

		// Get pool state string safely
		var poolStateStr string
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Errorf("SaveRaydiumCLMMPoolInfo: panic getting poolState string: %v, tx hash: %v", r, pair.TxHash)
					poolStateStr = ""
				}
			}()
			poolStateStr = pair.ClmmPoolInfoV2.PoolState.PubKey.String()
		}()

		if poolStateStr == "" {
			s.Errorf("SaveRaydiumCLMMPoolInfo: Failed - empty poolState for CLMM V2, txHash: %s", pair.TxHash)
			return fmt.Errorf("SaveRaydiumCLMMPoolInfo: failed to get pool state string")
		}

		// Process remaining accounts safely
		var remainingAccountsStr string
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Errorf("SaveRaydiumCLMMPoolInfo: panic processing remaining accounts: %v, tx hash: %v", r, pair.TxHash)
					remainingAccountsStr = "[]"
				}
			}()

			remainingAccounts := make([]string, 0)
			for _, account := range pair.ClmmPoolInfoV2.RemainingAccounts {
				remainingAccounts = append(remainingAccounts, account.PubKey.String())
			}
			remainingAccountsStr, _ = transfer.Struct2String(remainingAccounts)
		}()

		now := time.Now()

		// Find existing pool
		dbPool, err := s.sc.SolRaydiumCLMMPoolV2Model.FindOneByPoolState(ctx, poolStateStr)

		switch {
		case err == nil:
			// Update existing pool
			if dbPool != nil {
				dbPool.UpdatedAt = now
				func() {
					defer func() {
						if r := recover(); r != nil {
							s.Errorf("SaveRaydiumCLMMPoolInfo: panic in Update: %v, tx hash: %v", r, pair.TxHash)
						}
					}()
					_ = s.sc.SolRaydiumCLMMPoolV2Model.Update(ctx, dbPool)
				}()
				s.Infof("SaveRaydiumCLMMPoolInfo:FindOneByPoolState v2 update success, id: %v, hash: %v", poolStateStr, pair.TxHash)
			}
			return nil
		case errors.Is(err, solmodel.ErrNotFound) || strings.Contains(err.Error(), "record not found") || err.Error() == "sql: no rows in result set":
			// Create new pool record
			var ammConfigStr, inputVaultStr, outputVaultStr, observationStateStr, tokenProgramStr, tokenProgram2022Str, memoProgramStr, inputVaultMintStr, outputVaultMintStr string

			// Safely get all the PubKey strings
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.Errorf("SaveRaydiumCLMMPoolInfo: panic getting ClmmPoolInfoV2 PubKey strings: %v, tx hash: %v", r, pair.TxHash)
					}
				}()
				ammConfigStr = pair.ClmmPoolInfoV2.AmmConfig.String()
				inputVaultStr = pair.ClmmPoolInfoV2.InputVault.PubKey.String()
				outputVaultStr = pair.ClmmPoolInfoV2.OutputVault.PubKey.String()
				observationStateStr = pair.ClmmPoolInfoV2.ObservationState.PubKey.String()
				tokenProgramStr = pair.ClmmPoolInfoV2.TokenProgram.String()
				tokenProgram2022Str = pair.ClmmPoolInfoV2.TokenProgram2022.String()
				memoProgramStr = pair.ClmmPoolInfoV2.MemoProgram.String()
				inputVaultMintStr = pair.ClmmPoolInfoV2.InputVaultMint.String()
				outputVaultMintStr = pair.ClmmPoolInfoV2.OutputVaultMint.String()

				s.Infof("SaveRaydiumCLMMPoolInfo: ClmmPoolInfoV2 details - InputVaultMint: %s, OutputVaultMint: %s, PoolState: %s",
					inputVaultMintStr, outputVaultMintStr, poolStateStr)
			}()

			info := &solmodel.ClmmPoolInfoV2{
				AmmConfig:         ammConfigStr,
				PoolState:         poolStateStr,
				InputVault:        inputVaultStr,
				OutputVault:       outputVaultStr,
				ObservationState:  observationStateStr,
				TokenProgram:      tokenProgramStr,
				TokenProgram2022:  tokenProgram2022Str,
				MemoProgram:       memoProgramStr,
				InputVaultMint:    inputVaultMintStr,
				OutputVaultMint:   outputVaultMintStr,
				RemainingAccounts: remainingAccountsStr,
				TxHash:            pair.TxHash,
				TradeFeeRate:      int64(pair.ClmmPoolInfoV2.TradeFeeRate),
				CreatedAt:         now,
				UpdatedAt:         now,
			}

			// Safely insert record
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("SaveRaydiumCLMMPoolInfo: panic in Insert: %v", r)
					}
				}()
				err = s.sc.SolRaydiumCLMMPoolV2Model.Insert(ctx, info)
			}()

			if err != nil {
				if !strings.Contains(err.Error(), "Duplicate entry") {
					s.Errorf("SaveRaydiumCLMMPoolInfo: Failed to insert CLMM V2 pool: %v, txHash: %s, poolState: %s",
						err, pair.TxHash, poolStateStr)
					return fmt.Errorf("SaveRaydiumCLMMPoolInfo:SolRaydiumCLMMPoolV2Model.Insert %#v err:%v", info, err)
				}
				s.Infof("SaveRaydiumCLMMPoolInfo: Duplicate CLMM V2 pool entry, skipping: %s", poolStateStr)
				return nil
			}
			s.Infof("SaveRaydiumCLMMPoolInfo: Successfully inserted CLMM V2 pool: %s, txHash: %s, InputMint: %s, OutputMint: %s",
				poolStateStr, pair.TxHash, info.InputVaultMint, info.OutputVaultMint)
		default:
			s.Errorf("SaveRaydiumCLMMPoolInfo: Failed to find CLMM V2 pool: %v, txHash: %s, poolState: %s",
				err, pair.TxHash, poolStateStr)
			return fmt.Errorf("SaveRaydiumCLMMPoolInfo:SolRaydiumCLMMPoolV2Model.FindOneByPoolState err:%w", err)
		}
	}

	return nil
}

// SaveClmmPosition 保存 CLMM 持仓信息
func (s *BlockService) SaveClmmPosition(ctx context.Context, trade *types.TradeWithPair) error {
	// 只处理 open_position 类型的交易
	if trade == nil || trade.Type != "open_position" {
		return nil
	}

	// 检查是否有 CLMMOpenPositionInfo
	if trade.CLMMOpenPositionInfo == nil {
		s.Infof("SaveClmmPosition: CLMMOpenPositionInfo is nil, skipping. txHash: %s", trade.TxHash)
		return nil
	}

	info := trade.CLMMOpenPositionInfo

	// 检查是否已存在该持仓（通过 position_nft_mint）
	existing, err := s.sc.ClmmPositionModel.FindOneByPositionNftMint(ctx, info.PositionNftMint)
	if err == nil && existing != nil {
		// 持仓已存在，跳过
		s.Infof("SaveClmmPosition: Position already exists, skipping. positionNftMint: %s, txHash: %s", info.PositionNftMint, trade.TxHash)
		return nil
	}

	// 准备流动性字符串
	liquidityStr := "0"
	if info.Liquidity != nil {
		// Uint128 转换为字符串，格式为 "hi,lo"
		liquidityStr = fmt.Sprintf("%d,%d", info.Liquidity.Hi, info.Liquidity.Lo)
	}

	if liquidityStr == "0" || (info.Liquidity != nil && info.Liquidity.Hi == 0 && info.Liquidity.Lo == 0) {
		if info.PersonalPosition != "" {
			// 从链上获取 PersonalPositionState 账户中的流动性
			personalPositionState, err := s.fetchPersonalPositionStateByAddress(ctx, info.PersonalPosition)
			if err == nil && personalPositionState != nil {
				// 从 PersonalPositionState 中获取流动性
				liquidityStr = fmt.Sprintf("%d,%d", personalPositionState.Liquidity.Hi, personalPositionState.Liquidity.Lo)
				s.Infof("SaveClmmPosition: Fetched liquidity from chain. positionNftMint: %s, liquidity: %s", info.PositionNftMint, liquidityStr)
			} else {
				s.Infof("SaveClmmPosition: Failed to fetch liquidity from chain (will retry later). positionNftMint: %s, personalPosition: %s, err: %v",
					info.PositionNftMint, info.PersonalPosition, err)
			}
		}
	}

	// 准备 tick 索引值
	tickLowerIndex := int32(0)
	tickUpperIndex := int32(0)
	tickArrayLowerStartIndex := int32(0)
	tickArrayUpperStartIndex := int32(0)
	if info.TickLowerIndex != nil {
		tickLowerIndex = *info.TickLowerIndex
	}
	if info.TickUpperIndex != nil {
		tickUpperIndex = *info.TickUpperIndex
	}
	if info.TickArrayLowerStartIndex != nil {
		tickArrayLowerStartIndex = *info.TickArrayLowerStartIndex
	}
	if info.TickArrayUpperStartIndex != nil {
		tickArrayUpperStartIndex = *info.TickArrayUpperStartIndex
	}

	// 准备金额值
	amount0Max := int64(0)
	amount1Max := int64(0)
	if info.Amount0Max != nil {
		amount0Max = int64(*info.Amount0Max)
	}
	if info.Amount1Max != nil {
		amount1Max = int64(*info.Amount1Max)
	}

	// 获取区块时间
	blockTime := time.Unix(trade.BlockTime, 0)

	// 创建持仓记录
	position := &solmodel.ClmmPosition{
		ChainId:                  int64(trade.ChainIdInt),
		UserWalletAddress:        info.Payer,
		PoolState:                info.PoolState,
		PositionNftMint:          info.PositionNftMint,
		PositionNftAccount:       info.PositionNftAccount,
		PersonalPosition:         info.PersonalPosition,
		TickLowerIndex:           tickLowerIndex,
		TickUpperIndex:           tickUpperIndex,
		TickArrayLowerStartIndex: tickArrayLowerStartIndex,
		TickArrayUpperStartIndex: tickArrayUpperStartIndex,
		Liquidity:                liquidityStr,
		Amount0Max:               amount0Max,
		Amount1Max:               amount1Max,
		TokenAccount0:            info.TokenAccount0,
		TokenAccount1:            info.TokenAccount1,
		TokenVault0:              info.TokenVault0,
		TokenVault1:              info.TokenVault1,
		TxHash:                   trade.TxHash,
		Slot:                     trade.Slot,
		BlockTime:                blockTime,
		BlockTimeStamp:           trade.BlockTime,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	// 保存到数据库
	err = s.sc.ClmmPositionModel.Insert(ctx, position)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			s.Infof("SaveClmmPosition: Duplicate position entry, skipping. positionNftMint: %s, txHash: %s", info.PositionNftMint, trade.TxHash)
			return nil
		}
		s.Errorf("SaveClmmPosition: Failed to insert position: %v, txHash: %s, positionNftMint: %s", err, trade.TxHash, info.PositionNftMint)
		return fmt.Errorf("SaveClmmPosition: ClmmPositionModel.Insert err: %w", err)
	}

	s.Infof("SaveClmmPosition: Successfully saved position. positionNftMint: %s, userWallet: %s, poolState: %s, txHash: %s",
		info.PositionNftMint, info.Payer, info.PoolState, trade.TxHash)

	return nil
}

// fetchPersonalPositionStateByAddress 从链上获取 PersonalPositionState（通过地址）
func (s *BlockService) fetchPersonalPositionStateByAddress(ctx context.Context, personalPositionAddr string) (*amm_v3.PersonalPositionStateAccount, error) {
	cli := s.sc.GetSolClient()
	if cli == nil {
		return nil, errors.New("solana rpc client not configured")
	}

	if personalPositionAddr == "" {
		return nil, errors.New("personal_position address is empty")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从链上获取账户数据
	accountInfo, err := cli.GetAccountInfo(ctxWithTimeout, personalPositionAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get personal position account: %w", err)
	}
	if len(accountInfo.Data) == 0 {
		return nil, errors.New("personal position account not found or empty")
	}

	data := accountInfo.Data
	dec := bin.NewBorshDecoder(data)
	var personalPositionState amm_v3.PersonalPositionStateAccount
	if err := personalPositionState.UnmarshalWithDecoder(dec); err != nil {
		return nil, fmt.Errorf("failed to decode personal position state: %w", err)
	}

	return &personalPositionState, nil
}

func (s *BlockService) SaveRaydiumCPMMPoolInfo(ctx context.Context, trade *types.TradeWithPair) error {
	if trade == nil || trade.SwapName != constants.RaydiumCPMM {
		return nil
	}
	if s.sc == nil || s.sc.SolRaydiumCPMMPoolModel == nil {
		return errors.New("cpmm pool model not initialized")
	}

	poolState := trade.PairAddr
	info := trade.CpmmPoolInfo
	existing, findErr := s.sc.SolRaydiumCPMMPoolModel.FindOneByPoolState(ctx, poolState)
	switch {
	case findErr == nil && existing != nil:
		info = mergeCpmmPoolInfo(existing, info)
	case errors.Is(findErr, solmodel.ErrNotFound):
		if info == nil {
			return fmt.Errorf("cpmm pool info is nil for new pool %s", poolState)
		}
	default:
		if findErr != nil {
			return fmt.Errorf("SaveRaydiumCPMMPoolInfo: find pool err: %w", findErr)
		}
	}

	if info == nil {
		return nil
	}

	s.applyCpmmMetrics(trade, info)
	if existing != nil && existing.Id > 0 {
		info.Id = existing.Id
		if !existing.CreatedAt.IsZero() {
			info.CreatedAt = existing.CreatedAt
		}
		return s.sc.SolRaydiumCPMMPoolModel.Update(ctx, info)
	}
	return s.sc.SolRaydiumCPMMPoolModel.Insert(ctx, info)
}

func mergeCpmmPoolInfo(existing *solmodel.CpmmPoolInfo, incoming *solmodel.CpmmPoolInfo) *solmodel.CpmmPoolInfo {
	if existing == nil {
		return incoming
	}
	if incoming == nil {
		return existing
	}
	if incoming.AmmConfig == "" {
		incoming.AmmConfig = existing.AmmConfig
	}
	if incoming.PoolState == "" {
		incoming.PoolState = existing.PoolState
	}
	if incoming.InputVault == "" {
		incoming.InputVault = existing.InputVault
	}
	if incoming.OutputVault == "" {
		incoming.OutputVault = existing.OutputVault
	}
	if incoming.Authority == "" {
		incoming.Authority = existing.Authority
	}
	if incoming.InputTokenProgram == "" {
		incoming.InputTokenProgram = existing.InputTokenProgram
	}
	if incoming.OutputTokenProgram == "" {
		incoming.OutputTokenProgram = existing.OutputTokenProgram
	}
	if incoming.InputTokenMint == "" {
		incoming.InputTokenMint = existing.InputTokenMint
	}
	if incoming.OutputTokenMint == "" {
		incoming.OutputTokenMint = existing.OutputTokenMint
	}
	if incoming.TradeFeeRate == 0 {
		incoming.TradeFeeRate = existing.TradeFeeRate
	}
	if incoming.ObservationState == "" {
		incoming.ObservationState = existing.ObservationState
	}
	if incoming.TxHash == "" {
		incoming.TxHash = existing.TxHash
	}
	if incoming.LpMint == "" {
		incoming.LpMint = existing.LpMint
	}
	incoming.Volume24h = existing.Volume24h
	incoming.Fees24h = existing.Fees24h
	incoming.Apr24h = existing.Apr24h
	incoming.Liquidity = existing.Liquidity
	return incoming
}

func (s *BlockService) applyCpmmMetrics(trade *types.TradeWithPair, info *solmodel.CpmmPoolInfo) {
	if trade == nil || info == nil {
		return
	}
	blockTime := time.Unix(trade.BlockTime, 0)
	if blockTime.Sub(info.UpdatedAt) > 24*time.Hour {
		info.Volume24h = 0
		info.Fees24h = 0
	}

	if trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell {
		volumeUSD := trade.TotalUSD
		info.Volume24h += volumeUSD
		if info.TradeFeeRate > 0 && volumeUSD > 0 {
			info.Fees24h += volumeUSD * float64(info.TradeFeeRate) / 1_000_000
		}
	}

	if trade.CurrentBaseTokenInPoolAmount > 0 || trade.CurrentTokenInPoolAmount > 0 {
		basePrice := trade.BaseTokenPriceUSD
		tokenPrice := trade.TokenPriceUSD

		// Derive missing price from pool ratio when one side has a known USD price.
		if basePrice > 0 && tokenPrice == 0 && trade.CurrentTokenInPoolAmount > 0 {
			tokenPrice = basePrice * (trade.CurrentBaseTokenInPoolAmount / trade.CurrentTokenInPoolAmount)
		} else if tokenPrice > 0 && basePrice == 0 && trade.CurrentBaseTokenInPoolAmount > 0 {
			basePrice = tokenPrice * (trade.CurrentTokenInPoolAmount / trade.CurrentBaseTokenInPoolAmount)
		}

		liq := trade.CurrentBaseTokenInPoolAmount*basePrice + trade.CurrentTokenInPoolAmount*tokenPrice
		if liq > 0 {
			info.Liquidity = liq
		} else {
			// Fallback to CPMM liquidity metric sqrt(x*y) when USD prices are unavailable.
			if trade.CurrentBaseTokenInPoolAmount > 0 && trade.CurrentTokenInPoolAmount > 0 {
				liq = math.Sqrt(trade.CurrentBaseTokenInPoolAmount * trade.CurrentTokenInPoolAmount)
				if liq > 0 {
					info.Liquidity = liq
				}
			}
		}
		s.Infof("cpmm metrics: pair=%s baseAmt=%.6f tokenAmt=%.6f basePrice=%.6f tokenPrice=%.6f liq=%.6f tx=%s",
			trade.PairAddr, trade.CurrentBaseTokenInPoolAmount, trade.CurrentTokenInPoolAmount, basePrice, tokenPrice, info.Liquidity, trade.TxHash)
	}
	if info.Liquidity > 0 && info.Fees24h > 0 {
		info.Apr24h = info.Fees24h * 365 / info.Liquidity * 100
	}
	info.UpdatedAt = blockTime
}

// fillPairTokenSymbols 填充单个交易对的 base token 和 quote token symbol
// 仅在保存 Pair 信息时调用，避免不必要的查询
func (s *BlockService) fillPairTokenSymbols(ctx context.Context, trade *types.TradeWithPair) {
	if trade == nil {
		return
	}

	solClient := s.sc.GetSolClient()

	// 填充 base token symbol (如果为空)
	if trade.PairInfo.BaseTokenAddr != "" && trade.PairInfo.BaseTokenSymbol == "" {
		baseTokenAddr := trade.PairInfo.BaseTokenAddr

		// 优先从数据库获取
		tokenDB, err := s.sc.TokenModel.FindOneByChainIdAddress(ctx, SolChainIdInt, baseTokenAddr)
		if err == nil && tokenDB != nil && tokenDB.Symbol != "" {
			trade.PairInfo.BaseTokenSymbol = tokenDB.Symbol
		} else {
			// 数据库没有，从 RPC 获取
			tokenInfo, rpcErr := sol.GetTokenInfo(solClient, ctx, baseTokenAddr)
			if rpcErr == nil && tokenInfo != nil && tokenInfo.Data.Symbol != "" {
				trade.PairInfo.BaseTokenSymbol = tokenInfo.Data.Symbol
			}
		}
	}

	// 填充 quote token symbol (如果为空)
	if trade.PairInfo.TokenAddr != "" && trade.PairInfo.TokenSymbol == "" && trade.PairInfo.TokenAddr != constants.TokenStrWrapSol {
		tokenAddr := trade.PairInfo.TokenAddr

		// 优先从数据库获取
		tokenDB, err := s.sc.TokenModel.FindOneByChainIdAddress(ctx, SolChainIdInt, tokenAddr)
		if err == nil && tokenDB != nil && tokenDB.Symbol != "" {
			trade.PairInfo.TokenSymbol = tokenDB.Symbol
		} else {
			// 数据库没有，从 RPC 获取
			tokenInfo, rpcErr := sol.GetTokenInfo(solClient, ctx, tokenAddr)
			if rpcErr == nil && tokenInfo != nil && tokenInfo.Data.Symbol != "" {
				trade.PairInfo.TokenSymbol = tokenInfo.Data.Symbol
			}
		}
	}
}
