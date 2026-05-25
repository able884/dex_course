package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/types"
)

// SaveRequest 保存请求
type SaveRequest struct {
	Slot       uint64
	Protocol   ProtocolType
	Data       interface{}
	RetryCount int
}

// SaveResult 保存结果
type SaveResult struct {
	Slot         uint64
	Protocol     ProtocolType
	Success      bool
	Error        error
	RowsAffected int64
	SaveTime     time.Duration
}

// BatchSaver 批量保存器
type BatchSaver struct {
	sc           *svc.ServiceContext
	logger       logx.Logger
	ctx          context.Context
	blockService *BlockService // 用于调用现有的保存方法
}

// NewBatchSaver 创建批量保存器
func NewBatchSaver(sc *svc.ServiceContext, ctx context.Context) *BatchSaver {
	// 初始化并发度
	concurrency := sc.BlockPipelineSettings.PairBatchConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// 初始化元数据缓存配置
	metadataTTL := time.Duration(sc.BlockPipelineSettings.MetadataCacheTTLHours) * time.Hour
	if metadataTTL <= 0 {
		metadataTTL = 24 * time.Hour
	}
	metadataCooldown := 10 * time.Minute
	var cache metadataCache
	if sc.MetadataCache != nil {
		cache = &redisMetadataCache{client: sc.MetadataCache}
	}

	// 初始化价格范围缓存
	priceRangeCache := NewPriceRangeCache(sc.Redis, constants.SolChainIdInt)
	if err := priceRangeCache.LoadFromRedis(ctx); err != nil {
		logx.Errorf("NewBatchSaver: failed to load price ranges from redis: %v", err)
	}

	// 创建一个 BlockService 实例用于调用保存方法（完整初始化）
	blockService := &BlockService{
		sc:                      sc,
		Logger:                  logx.WithContext(ctx).WithFields(logx.Field("service", "batch_saver_helper")),
		ctx:                     ctx,
		batchQueue:              newPairBatchQueue(concurrency),
		metadataCache:           cache,
		metadataTTL:             metadataTTL,
		metadataFailureCooldown: metadataCooldown,
		metadataReq:             make(map[string]*metadataCacheResult),
		priceRangeCache:         priceRangeCache,
	}

	return &BatchSaver{
		sc:           sc,
		logger:       logx.WithContext(ctx).WithFields(logx.Field("service", "batch_saver")),
		ctx:          ctx,
		blockService: blockService,
	}
}

// SaveResults 批量保存处理结果
func (s *BatchSaver) SaveResults(ctx context.Context, slot uint64, results []*ProcessResult) ([]*SaveResult, error) {
	if len(results) == 0 {
		return nil, nil
	}

	// 1. 收集所有交易和公共数据
	allTrades := make([]*types.TradeWithPair, 0)
	var tokenAccountMap map[string]*TokenAccount

	for _, result := range results {
		if result.Success && result.DataToSave != nil {
			if processData, ok := result.DataToSave.(*ProtocolProcessData); ok {
				allTrades = append(allTrades, processData.Trades...)
				if tokenAccountMap == nil {
					tokenAccountMap = processData.TokenAccountMap
				}
			}
		}
	}

	// 2. 并发保存：交易数据、TokenAccount、各协议特定数据
	// 使用 error channel 收集错误
	var wg sync.WaitGroup
	errChan := make(chan error, len(results)+2) // +2 for SaveTrades and Mint/Burn

	// 2.1 保存交易信息和 TokenAccount
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("SaveTrades panic: %v", r)
				s.logger.Error(err)
				errChan <- err
			}
		}()

		// 将交易按 Pair 分组
		tradeMap := make(map[string][]*types.TradeWithPair)
		for _, trade := range allTrades {
			if len(trade.PairAddr) > 0 {
				tradeMap[trade.PairAddr] = append(tradeMap[trade.PairAddr], trade)
			}
		}

		s.blockService.SaveTrades(ctx, constants.SolChainIdInt, tradeMap)
		s.blockService.SaveTokenAccounts(ctx, allTrades, tokenAccountMap)
	}()

	// 2.2 处理 Token Mint/Burn
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("UpdateTokenMints/Burns panic: %v", r)
				s.logger.Error(err)
				errChan <- err
			}
		}()

		tokenMints := make([]*types.TradeWithPair, 0)
		tokenBurns := make([]*types.TradeWithPair, 0)

		for _, trade := range allTrades {
			if trade.Type == types.TradeTokenMint {
				tokenMints = append(tokenMints, trade)
			} else if trade.Type == types.TradeTokenBurn {
				tokenBurns = append(tokenBurns, trade)
			}
		}

		if len(tokenMints) > 0 {
			s.blockService.UpdateTokenMints(ctx, tokenMints)
		}
		if len(tokenBurns) > 0 {
			s.blockService.UpdateTokenBurns(ctx, tokenBurns)
		}
	}()

	// 2.3 并发保存各协议特定数据
	var saveResultsMu sync.Mutex
	saveResults := make([]*SaveResult, 0, len(results))

	for _, result := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					err := fmt.Errorf("saveProtocolData panic for %s: %v", result.Protocol, r)
					s.logger.Error(err)
					errChan <- err
				}
			}()

			// 跳过失败的处理结果
			if !result.Success || result.DataToSave == nil {
				s.logger.Infof("Skipping failed result: protocol=%s, slot=%d", result.Protocol, slot)
				return
			}

			// 保存单个协议的特定数据
			saveResult := s.saveProtocolData(ctx, slot, result)

			saveResultsMu.Lock()
			saveResults = append(saveResults, saveResult)
			saveResultsMu.Unlock()

			// 如果保存失败，记录错误
			if !saveResult.Success && saveResult.Error != nil {
				errChan <- fmt.Errorf("protocol %s save failed: %w", result.Protocol, saveResult.Error)
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误发生
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// 如果有任何 panic 错误，返回失败
	if len(errs) > 0 {
		return saveResults, fmt.Errorf("batch save encountered %d error(s): %v", len(errs), errs[0])
	}

	return saveResults, nil
}

// saveProtocolData 保存单个协议的数据
func (s *BatchSaver) saveProtocolData(ctx context.Context, slot uint64, result *ProcessResult) *SaveResult {
	startTime := time.Now()

	saveResult := &SaveResult{
		Slot:     slot,
		Protocol: result.Protocol,
	}

	var err error
	var rowsAffected int64

	// 根据协议类型选择不同的保存逻辑
	switch result.Protocol {
	case ProtocolPumpFun:
		rowsAffected, err = s.savePumpFunData(result.DataToSave)

	case ProtocolPumpSwap:
		rowsAffected, err = s.savePumpFunData(result.DataToSave)

	case ProtocolRaydiumCLMM:
		rowsAffected, err = s.saveRaydiumCLMMData(result.DataToSave)

	case ProtocolRaydiumCPMM:
		rowsAffected, err = s.saveRaydiumCPMMData(result.DataToSave)

	case ProtocolRaydiumCPMMDevNet:
		rowsAffected, err = s.saveRaydiumCPMMData(result.DataToSave)

	case ProtocolSPLToken, ProtocolSPLToken2022:
		processData, ok := result.DataToSave.(*ProtocolProcessData)
		if ok && processData != nil {
			rowsAffected = int64(len(processData.Trades))
		}

	default:
		s.logger.Errorf("unsupported protocol: %s", result.Protocol)
	}

	elapsed := time.Since(startTime)
	saveResult.SaveTime = elapsed

	if err != nil {
		saveResult.Success = false
		saveResult.Error = err
		return saveResult
	}

	saveResult.Success = true
	saveResult.RowsAffected = rowsAffected

	return saveResult
}

// savePumpFunData 保存 PumpFun/PumpSwap 数据
func (s *BatchSaver) savePumpFunData(data interface{}) (int64, error) {
	processData, ok := data.(*ProtocolProcessData)
	if !ok {
		return 0, fmt.Errorf("invalid data type for PumpFun")
	}

	if len(processData.Trades) == 0 {
		return 0, nil
	}

	// 保存 PumpSwap 池子信息
	savedCount := int64(0)
	for _, trade := range processData.Trades {
		if trade.SwapName == constants.PumpSwap || trade.SwapName == "PumpSwap" {
			if err := s.blockService.SavePumpSwapPoolInfo(s.ctx, trade); err != nil {
				s.logger.Errorf("SavePumpSwapPoolInfo error: %v, trade: %+v", err, trade)
			} else {
				savedCount++
			}
		}
	}

	return savedCount, nil
}

// saveRaydiumCLMMData 保存 Raydium CLMM 数据
func (s *BatchSaver) saveRaydiumCLMMData(data interface{}) (int64, error) {
	processData, ok := data.(*ProtocolProcessData)
	if !ok {
		return 0, fmt.Errorf("invalid data type for Raydium CLMM")
	}

	if len(processData.Trades) == 0 {
		return 0, nil
	}

	// 保存 CLMM 池子信息和持仓信息
	savedCount := int64(0)
	for _, trade := range processData.Trades {
		// 保存池子信息
		if err := s.blockService.SaveRaydiumCLMMPoolInfo(s.ctx, trade); err != nil {
			s.logger.Errorf("SaveRaydiumCLMMPoolInfo error: %v, trade: %+v", err, trade)
		} else {
			savedCount++
		}

		// 保存 CLMM 持仓信息（仅对 open_position 类型）
		if trade.Type == "open_position" {
			if err := s.blockService.SaveClmmPosition(s.ctx, trade); err != nil {
				s.logger.Errorf("SaveClmmPosition error: %v, txHash: %s", err, trade.TxHash)
			} else {
				// 创建持仓后，更新持仓价值和手续费
				if trade.CLMMOpenPositionInfo != nil {
					_ = s.blockService.UpdateClmmPositionValueAndFees(s.ctx, trade.CLMMOpenPositionInfo.PositionNftMint, trade.PairAddr)
				}
			}
		}

		// 更新持仓价值和手续费（对于影响持仓的指令）
		if trade.Type == types.TradeRaydiumConcentratedLiquidityIncreaseLiquidity ||
			trade.Type == types.TradeRaydiumConcentratedLiquidityDecreaseLiquidity {
			if trade.CLMMOpenPositionInfo != nil && trade.CLMMOpenPositionInfo.PositionNftMint != "" {
				_ = s.blockService.UpdateClmmPositionValueAndFees(s.ctx, trade.CLMMOpenPositionInfo.PositionNftMint, trade.PairAddr)
			}
		}
	}

	return savedCount, nil
}

// saveRaydiumCPMMData 保存 Raydium CPMM 数据
func (s *BatchSaver) saveRaydiumCPMMData(data interface{}) (int64, error) {
	processData, ok := data.(*ProtocolProcessData)
	if !ok {
		return 0, fmt.Errorf("invalid data type for Raydium CPMM")
	}

	if len(processData.Trades) == 0 {
		return 0, nil
	}

	// 保存 CPMM 池子信息
	savedCount := int64(0)
	for _, trade := range processData.Trades {
		if err := s.blockService.SaveRaydiumCPMMPoolInfo(s.ctx, trade); err != nil {
			s.logger.Errorf("SaveRaydiumCPMMPoolInfo error: %v, trade: %+v", err, trade)
		} else {
			savedCount++
		}
	}

	return savedCount, nil
}
