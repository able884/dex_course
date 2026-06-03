package block

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"richcode.cc/dex/consumer/internal/svc"
	"richcode.cc/dex/model/solmodel"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/cpmm"
	"richcode.cc/dex/pkg/types"

	"richcode.cc/dex/consumer/internal/config"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gorilla/websocket"
	"github.com/klen-ygs/gorm-zero/gormc"
	"github.com/mr-tron/base58"
	"github.com/panjf2000/ants/v2"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/threading"
)

var ErrServiceStop = errors.New("service stop")

var ErrUnknowProgram = errors.New("unknow program")

type BlockService struct {
	Name string
	sc   *svc.ServiceContext
	c    *client.Client
	logx.Logger
	workerPool *ants.Pool
	slotChan   chan uint64
	slot       uint64
	Conn       *websocket.Conn
	solPrice   float64
	ctx        context.Context
	cancel     func(err error)
	name       string
	batchQueue *pairBatchQueue

	metadataCache           metadataCache
	metadataTTL             time.Duration
	metadataFailureCooldown time.Duration
	metadataReqMu           sync.Mutex
	metadataReq             map[string]*metadataCacheResult

	priceRangeCache *PriceRangeCache
}

func (s *BlockService) Stop() {
	s.cancel(ErrServiceStop)
	if s.batchQueue != nil {
		s.batchQueue.stop()
	}
	if s.Conn != nil {
		err := s.Conn.WriteMessage(websocket.TextMessage, []byte("{\"id\":1,\"jsonrpc\":\"2.0\",\"method\": \"blockUnsubscribe\", \"params\": [0]}\n"))
		if err != nil {
			s.Error("programUnsubscribe", err)
		}
		_ = s.Conn.Close()
	}
}

func (s *BlockService) Start() {
	s.GetBlockFromHttp()
}

func NewBlockService(sc *svc.ServiceContext, name string, slotChan chan uint64, index int) *BlockService {
	ctx, cancel := context.WithCancelCause(context.Background())
	pool, _ := ants.NewPool(5)
	concurrency := sc.BlockPipelineSettings.PairBatchConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
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
		logx.Errorf("NewBlockService: failed to load price ranges from redis: %v", err)
	}

	// 启动定期清理任务
	go func() {
		ticker := time.NewTicker(1 * time.Hour) // 每小时清理一次
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				priceRangeCache.CleanExpiredRanges(ctx)
			}
		}
	}()

	solService := &BlockService{
		c: client.New(rpc.WithEndpoint(config.FindChainRpcByChainId(constants.SolChainIdInt)), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
		})),
		sc:                      sc,
		Logger:                  logx.WithContext(context.Background()).WithFields(logx.Field("service", fmt.Sprintf("%s-%v", name, index))),
		slotChan:                slotChan,
		workerPool:              pool,
		ctx:                     ctx,
		cancel:                  cancel,
		name:                    name,
		batchQueue:              newPairBatchQueue(concurrency),
		metadataCache:           cache,
		metadataTTL:             metadataTTL,
		metadataFailureCooldown: metadataCooldown,
		metadataReq:             make(map[string]*metadataCacheResult),
		priceRangeCache:         priceRangeCache,
	}
	return solService
}

func (s *BlockService) GetBlockFromHttp() {
	ctx := s.ctx
	for {
		select {
		case <-s.ctx.Done():
			fmt.Print("block service is stopped")
			return
		case slot, ok := <-s.slotChan:
			if !ok {
				fmt.Print("slotChan is closed")
				return
			}
			//打印当前最新slot
			// fmt.Println("consume slot is:", slot)
			threading.RunSafe(func() {
				s.ProcessBlock(ctx, int64(slot))
			})
		}
	}
}

func (s *BlockService) FillTradeWithPairInfo(trade *types.TradeWithPair, slot int64) {
	trade.Slot = slot
	trade.BlockNum = slot
	trade.ChainIdInt = constants.SolChainIdInt
	trade.ChainId = constants.SolChainId
}

func (s *BlockService) ProcessBlock(ctx context.Context, slot int64) {
	if slot == 0 {
		return
	}

	s.slot = uint64(slot)
	s.resetMetadataAggregator(s.slot)

	block := &solmodel.Block{
		Slot:      slot,
		Status:    constants.BlockFailed,
		BlockTime: time.Now(), // 设置默认时间，避免 '0000-00-00' 错误
	}

	blockInfo, err := GetSolBlockInfoDelay(s.sc.GetSolClient(), ctx, uint64(slot))
	if err != nil || blockInfo == nil {
		fmt.Println("get block info error: ", err)

		// Anchor 会返回 was skipped 的 slot，直接标记为跳槽并落库
		if strings.Contains(err.Error(), "was skipped") {
			block.Status = constants.BlockSkipped
		}

		if saveErr := s.saveOrUpdateBlock(ctx, block); saveErr != nil {
			s.Errorf("save block slot:%d failed: %v", slot, saveErr)
		}
		return
	}

	// Step1: 同步 blockTime、blockHeight 等基础信息
	if blockInfo.BlockTime != nil {
		block.BlockTime = *blockInfo.BlockTime
	}

	if blockInfo.BlockHeight != nil {
		block.BlockHeight = *blockInfo.BlockHeight
	}
	block.Status = constants.BlockProcessed

	// Step2: 计算当期 SOL 价格，为后续成交估值提供基准
	var tokenAccountMap = make(map[string]*TokenAccount)
	solPrice := s.GetBlockSolPrice(ctx, blockInfo, tokenAccountMap)
	if solPrice == 0 {
		solPrice = s.solPrice
	}
	// 区块 -> 交易 -> 转账（Transfer）SOL-(USDT/USDC) -> (USDT|USDC) / SOL = 价格
	block.SolPrice = solPrice

	trades := make([]*types.TradeWithPair, 0, 1000)
	slice.ForEach(blockInfo.Transactions, func(index int, tx client.BlockTransaction) {

		// Step3: 构造解码上下文，逐笔解码链上交易
		decodeTx := &DecodedTx{
			BlockDb:         block,
			Tx:              &tx,
			TxIndex:         index,
			TokenAccountMap: tokenAccountMap,
			SolPrice:        solPrice,
		}

		trade, err := DecodeTx(ctx, s.sc, decodeTx)
		if err != nil {
			if strings.Contains(err.Error(), "unknow program") {
				return
			}
			return
		}

		trade = slice.Filter(trade, func(index int, item *types.TradeWithPair) bool {
			if item == nil {
				return false
			}
			s.FillTradeWithPairInfo(item, slot)
			return true
		})

		trades = append(trades, trade...)
	})

	// Step4: 将成交按 Pair 归类，方便后续批量写入
	tradeMap := make(map[string][]*types.TradeWithPair)

	pumpSwapCount := 0
	pumpFunCount := 0

	for _, trade := range trades {
		if len(trade.PairAddr) > 0 {
			tradeMap[trade.PairAddr] = append(tradeMap[trade.PairAddr], trade)
		}
	}

	for _, value := range tradeMap {
		// 简单统计不同 Swap 的成交数量，方便监控
		if value[0].SwapName == constants.PumpFun {
			pumpFunCount++
			continue
		}
		if value[0].SwapName == constants.PumpSwap {
			pumpSwapCount++
			continue
		}
	}

	{
		// 额外挑出 Mint 行为，触发 Token 总量刷新
		tokenMints := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenMint {
				return true
			}
			return false
		})

		s.UpdateTokenMints(ctx, tokenMints)
	}

	{
		// 额外挑出 Burn 行为，触发 Token 总量刷新
		tokenBurns := slice.Filter[*types.TradeWithPair](trades, func(_ int, item *types.TradeWithPair) bool {
			if item != nil && item.Type == types.TradeTokenBurn {
				return true
			}
			return false
		})

		s.UpdateTokenBurns(ctx, tokenBurns)
	}

	//并发处理： 保存交易信息，保存token账户信息
	group := threading.NewRoutineGroup()
	group.RunSafe(func() {
		// Step5: 写入成交信息 & TokenAccount 快照
		s.SaveTrades(ctx, constants.SolChainIdInt, tradeMap)

		s.SaveTokenAccounts(ctx, trades, tokenAccountMap)
	})

	// pump swap
	group.RunSafe(func() {
		// Step6: 针对 PumpSwap 交易补充池子元数据
		slice.ForEach(trades, func(_ int, trade *types.TradeWithPair) {
			if trade.SwapName == constants.PumpSwap || trade.SwapName == "PumpSwap" {
				if trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell {
					if err = s.SavePumpSwapPoolInfo(ctx, trade); err != nil {
						s.Errorf("processBlock:%v SavePumpSwapPoolInfo err: %v", slot, err)
					}
				}

			}
		})
	})

	group.RunSafe(func() {
		slice.ForEach(trades, func(_ int, trade *types.TradeWithPair) {
			if trade.SwapName == constants.RaydiumConcentratedLiquidity || trade.SwapName == "RaydiumClmm" {
				s.Infof("CLMM Processing: Found CLMM trade with type: %s, txHash: %s, pair: %s",
					trade.Type, trade.TxHash, trade.PairAddr)

				// 更新价格范围缓存（仅对买卖交易）
				if (trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell) && trade.TokenPriceUSD > 0 && s.priceRangeCache != nil {
					blockTime := time.Unix(trade.BlockTime, 0)
					s.priceRangeCache.UpdatePrice(ctx, trade.PairAddr, trade.TokenPriceUSD, blockTime)
				}

				// Save all CLMM trades, not just buy/sell
				if err = s.SaveRaydiumCLMMPoolInfo(ctx, trade); err != nil {
					s.Errorf("processBlock:%v saveRaydiumCLMMPoolInfo err: %v, trade.Type: %s", slot, err, trade.Type)
				} else {
					s.Infof("CLMM Success: Saved pool info for txHash: %s, trade.Type: %s", trade.TxHash, trade.Type)
				}

				// 保存 CLMM 持仓信息（仅对 open_position 类型）
				if trade.Type == "open_position" {
					if err = s.SaveClmmPosition(ctx, trade); err != nil {
						s.Errorf("processBlock:%v SaveClmmPosition err: %v, txHash: %s", slot, err, trade.TxHash)
					} else {
						s.Infof("CLMM Position Success: Saved position for txHash: %s", trade.TxHash)
						// 创建持仓后，更新持仓价值和手续费
						if trade.CLMMOpenPositionInfo != nil {
							_ = s.UpdateClmmPositionValueAndFees(ctx, trade.CLMMOpenPositionInfo.PositionNftMint, trade.PairAddr)
						}
					}
				}

				// 更新持仓价值和手续费（对于影响持仓的指令）
				if trade.Type == types.TradeRaydiumConcentratedLiquidityIncreaseLiquidity ||
					trade.Type == types.TradeRaydiumConcentratedLiquidityDecreaseLiquidity {
					// 从 CLMMOpenPositionInfo 中获取 position_nft_mint
					if trade.CLMMOpenPositionInfo != nil && trade.CLMMOpenPositionInfo.PositionNftMint != "" {
						_ = s.UpdateClmmPositionValueAndFees(ctx, trade.CLMMOpenPositionInfo.PositionNftMint, trade.PairAddr)
					}
				} else if trade.Type == types.TradeTypeBuy || trade.Type == types.TradeTypeSell {
					// Swap 交易会影响池子中所有持仓的手续费
					// 为了性能，异步批量更新该池子的所有持仓
					go func(poolState string) {
						positions, err := s.sc.ClmmPositionModel.FindByUserWalletAndPool(context.Background(), "", poolState)
						if err == nil {
							for _, pos := range positions {
								_ = s.UpdateClmmPositionValueAndFees(context.Background(), pos.PositionNftMint, pos.PoolState)
							}
						}
					}(trade.PairAddr)
				}
			}
		})
	})

	group.RunSafe(func() {
		slice.ForEach(trades, func(_ int, trade *types.TradeWithPair) {
			if trade.SwapName == constants.RaydiumCPMM {
				if err = s.SaveRaydiumCPMMPoolInfo(ctx, trade); err != nil {
					s.Errorf("processBlock:%v SaveRaydiumCPMMPoolInfo err: %v", slot, err)
				}
			}
		})
	})

	group.Wait()

	// Step7: 区块落库，标识处理完成
	if err = s.saveOrUpdateBlock(ctx, block); err != nil {
		s.Errorf("save block slot:%d err:%v", slot, err)
	}
}

func (s *BlockService) saveOrUpdateBlock(ctx context.Context, block *solmodel.Block) error {
	if block == nil {
		return errors.New("block is nil")
	}

	existing, err := s.sc.BlockModel.FindOneBySlot(ctx, block.Slot)
	switch {
	case err == nil && existing != nil:
		block.Id = existing.Id
		if !existing.CreatedAt.IsZero() {
			block.CreatedAt = existing.CreatedAt
		}
		block.DeletedAt = existing.DeletedAt
		return s.sc.BlockModel.Update(ctx, block)
	case errors.Is(err, gormc.ErrNotFound) || errors.Is(err, solmodel.ErrNotFound):
		return s.sc.BlockModel.Insert(ctx, block)
	case err != nil:
		return err
	default:
		return s.sc.BlockModel.Insert(ctx, block)
	}
}

type pairBatchJob struct {
	pair string
	work func()
}

type pairBatchQueue struct {
	jobs      chan pairBatchJob
	pairLocks sync.Map
	once      sync.Once
}

func newPairBatchQueue(concurrency int) *pairBatchQueue {
	if concurrency <= 0 {
		concurrency = 1
	}
	queue := &pairBatchQueue{
		jobs: make(chan pairBatchJob, concurrency*2),
	}
	for i := 0; i < concurrency; i++ {
		go queue.worker()
	}
	return queue
}

func (q *pairBatchQueue) enqueue(pair string, fn func()) {
	q.jobs <- pairBatchJob{
		pair: pair,
		work: fn,
	}
}

func (q *pairBatchQueue) stop() {
	q.once.Do(func() {
		close(q.jobs)
	})
}

func (q *pairBatchQueue) worker() {
	for job := range q.jobs {
		lock := q.getLock(job.pair)
		lock.Lock()
		job.work()
		lock.Unlock()
	}
}

func (q *pairBatchQueue) getLock(pair string) *sync.Mutex {
	lock, _ := q.pairLocks.LoadOrStore(pair, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type metadataCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

type metadataCacheResult struct {
	payload *tokenMetadataCachePayload
	err     error
}

func (s *BlockService) resetMetadataAggregator(slot uint64) {
	s.metadataReqMu.Lock()
	defer s.metadataReqMu.Unlock()
	s.metadataReq = make(map[string]*metadataCacheResult)
}

func (s *BlockService) metadataCacheGet(key string) (*metadataCacheResult, bool) {
	s.metadataReqMu.Lock()
	defer s.metadataReqMu.Unlock()
	res, ok := s.metadataReq[key]
	return res, ok
}

func (s *BlockService) metadataCacheSetResult(key string, res *metadataCacheResult) {
	s.metadataReqMu.Lock()
	defer s.metadataReqMu.Unlock()
	if s.metadataReq == nil {
		s.metadataReq = make(map[string]*metadataCacheResult)
	}
	s.metadataReq[key] = res
}

type redisMetadataCache struct {
	client *redis.Redis
}

func (r *redisMetadataCache) Get(ctx context.Context, key string) (string, error) {
	if r == nil || r.client == nil {
		return "", errors.New("metadata cache not configured")
	}
	return r.client.Get(key)
}

func (r *redisMetadataCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if r == nil || r.client == nil {
		return errors.New("metadata cache not configured")
	}
	seconds := int(ttl.Seconds())
	if seconds <= 0 {
		return r.client.Set(key, value)
	}
	return r.client.Setex(key, value, seconds)
}

func DecodeTx(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx) (trades []*types.TradeWithPair, err error) {
	if dtx.Tx == nil || dtx.BlockDb == nil {
		return
	}

	tx := dtx.Tx
	dtx.TxHash = base58.Encode(tx.Transaction.Signatures[0])

	if tx.Meta.Err != nil {
		return
	}

	dtx.InnerInstructionMap = GetInnerInstructionMap(tx)

	// 遍历交易内所有指令，挨个解析生成业务侧的成交结构
	for i := range tx.Transaction.Message.Instructions {
		inst := &tx.Transaction.Message.Instructions[i]
		var trade *types.TradeWithPair
		trade, err = DecodeInstruction(ctx, sc, dtx, inst, i)
		if err != nil {
			if strings.Contains(err.Error(), "unknow program") || strings.Contains(err.Error(), "not support instruction") {
				continue
			}
			logx.Errorf("decode instruction error: %v, hash: %v, index: %d", err, dtx.TxHash, i)
			continue
		}
		trades = append(trades, trade)
	}
	return
}

func DecodeInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, index int) (trade *types.TradeWithPair, err error) {
	if len(dtx.Tx.AccountKeys) == 0 {
		return nil, errors.New("account keys is empty")
	}

	if int(instruction.ProgramIDIndex) >= len(dtx.Tx.AccountKeys) {
		return nil, fmt.Errorf("program ID index %d out of bounds for account keys length %d", instruction.ProgramIDIndex, len(dtx.Tx.AccountKeys))
	}

	tx := dtx.Tx
	innerInstructions := dtx.InnerInstructionMap[index]
	program := tx.AccountKeys[instruction.ProgramIDIndex].String()

	switch program {
	case ProgramStrPumpFun:
		trade, err = DecodePumpFunInstruction(ctx, sc, dtx, instruction, index)
		return
	case ProgramStrPumpFunAMM:
		decoder := &PumpAmmDecoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
		}
		trade, err = decoder.DecodePumpFunAMMInstruction()
		return
	case common.TokenProgramID.String():
		trade, err = DecodeTokenProgramInstruction(ctx, sc, dtx, instruction, index)
		return trade, err
	case common.Token2022ProgramID.String():
		decoder := &Token2022Decoder{
			ctx:                 ctx,
			svcCtx:              sc,
			dtx:                 dtx,
			compiledInstruction: instruction,
			innerInstruction:    innerInstructions,
		}
		trade, err = decoder.DecodeToken2022DecoderInstruction()
		if err != nil {
			return nil, err
		}
		return trade, err
	case clmm.ProgramClMMDevNet.String():
		return DecodeRaydiumCLMMInstruction(ctx, sc, dtx, instruction, innerInstructions)
	case clmm.ProgramRaydiumConcentratedLiquidity.String():
		return DecodeRaydiumCLMMInstruction(ctx, sc, dtx, instruction, innerInstructions)
	case cpmm.ProgramRaydiumCPMMProgramDevNet.String():
		return DecodeRaydiumCPMMInstruction(ctx, sc, dtx, instruction, innerInstructions)
	case cpmm.ProgramRaydiumCPMMProgram.String():
		return DecodeRaydiumCPMMInstruction(ctx, sc, dtx, instruction, innerInstructions)
	case constants.ProgramStrLimitOrder: // Limit Order Program
		return DecodeLimitOrderInstruction(ctx, sc, dtx, instruction, innerInstructions)
	default:
		return nil, ErrUnknowProgram
	}
}

func DecodeRaydiumCLMMInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, innerInstructions *client.InnerInstruction) (trade *types.TradeWithPair, err error) {
	decoder := &ConcentratedLiquidityDecoder{
		ctx:                 ctx,
		svcCtx:              sc,
		dtx:                 dtx,
		compiledInstruction: instruction,
		innerInstruction:    innerInstructions,
	}
	trade, err = decoder.DecodeRaydiumConcentratedLiquidityInstruction()
	if err != nil {
		logx.Errorf("error find inner clmm tx: %v, err : %v", dtx.TxHash, err)
		return nil, err
	}
	logx.Infof("find inner clmm tx: %v, pairInfo: %#v", dtx.TxHash, trade.PairInfo)
	return trade, nil
}

func DecodeRaydiumCPMMInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, innerInstructions *client.InnerInstruction) (trade *types.TradeWithPair, err error) {
	decoder := &CpmmDecoder{
		ctx:                 ctx,
		svcCtx:              sc,
		dtx:                 dtx,
		compiledInstruction: instruction,
		innerInstruction:    innerInstructions,
	}
	trade, err = decoder.DecodeRaydiumCPMMInstruction()
	if err != nil {
		logx.Errorf("error decoding cpmm tx: %v, err : %v", dtx.TxHash, err)
		return nil, err
	}
	return trade, nil
}

func GetInstructionDiscriminator(data []byte) []byte {
	if len(data) < 8 || data == nil {
		return nil
	}
	return data[:8]
}

func GetInnerInstructionByInner(instructions []solTypes.CompiledInstruction, startIndex, innerLen int) *client.InnerInstruction {
	if startIndex+innerLen+1 > len(instructions) {
		return nil
	}
	innerInstruction := &client.InnerInstruction{
		Index: uint64(instructions[startIndex].ProgramIDIndex),
	}
	for i := 0; i < innerLen; i++ {
		innerInstruction.Instructions = append(innerInstruction.Instructions, instructions[startIndex+i+1])
	}
	return innerInstruction
}

// FillTokenAccountMap 汇总交易前后 TokenAccount 余额，用于后续价格、持仓等计算。
func FillTokenAccountMap(tx *client.BlockTransaction, tokenAccountMapIn map[string]*TokenAccount) (tokenAccountMap map[string]*TokenAccount, hasTokenChange bool) {
	if tokenAccountMapIn == nil {
		tokenAccountMapIn = make(map[string]*TokenAccount)
	}
	tokenAccountMap = tokenAccountMapIn
	for _, pre := range tx.Meta.PreTokenBalances {
		var tokenAccount = tx.AccountKeys[pre.AccountIndex].String()
		preValue, _ := strconv.ParseInt(pre.UITokenAmount.Amount, 10, 64)
		tokenAccountMap[tokenAccount] = &TokenAccount{
			Owner:               pre.Owner,                  // owner address
			TokenAccountAddress: tokenAccount,               // token account address
			TokenAddress:        pre.Mint,                   // token address
			TokenSymbol:         "",                         // will be filled later
			TokenName:           "",                         // will be filled later
			TokenDecimal:        pre.UITokenAmount.Decimals, // token decimal
			PreValue:            preValue,
			Closed:              true,
			PreValueUIString:    pre.UITokenAmount.UIAmountString,
		}
	}
	for _, post := range tx.Meta.PostTokenBalances {
		var tokenAccount = tx.AccountKeys[post.AccountIndex].String()
		postValue, _ := strconv.ParseInt(post.UITokenAmount.Amount, 10, 64)
		if tokenAccountMap[tokenAccount] != nil {
			tokenAccountMap[tokenAccount].Closed = false
			tokenAccountMap[tokenAccount].PostValue = postValue
			if tokenAccountMap[tokenAccount].PostValue != tokenAccountMap[tokenAccount].PreValue {
				hasTokenChange = true
			}
		} else {
			hasTokenChange = true
			tokenAccountMap[tokenAccount] = &TokenAccount{
				Owner:               post.Owner,                  // owner address
				TokenAccountAddress: tokenAccount,                // token account address
				TokenAddress:        post.Mint,                   // token address
				TokenSymbol:         "",                          // will be filled later
				TokenName:           "",                          // will be filled later
				TokenDecimal:        post.UITokenAmount.Decimals, // token decimal
				PostValue:           postValue,                   // token balance
				Init:                true,
				PostValueUIString:   post.UITokenAmount.UIAmountString,
			}
		}
	}
	for i := range tx.Transaction.Message.Instructions {
		instruction := &tx.Transaction.Message.Instructions[i]
		program := tx.AccountKeys[instruction.ProgramIDIndex].String()
		if program == ProgramStrToken {
			DecodeInitAccountInstruction(tx, tokenAccountMap, instruction)
		}
	}
	for _, instructions := range tx.Meta.InnerInstructions {
		for i := range instructions.Instructions {
			instruction := instructions.Instructions[i]
			program := tx.AccountKeys[instruction.ProgramIDIndex].String()
			if program == ProgramStrToken {
				DecodeInitAccountInstruction(tx, tokenAccountMap, &instruction)
			}
		}
	}
	tokenDecimalMap := make(map[string]uint8)
	for _, v := range tokenAccountMap {
		if v.TokenDecimal != 0 {
			tokenDecimalMap[v.TokenAddress] = v.TokenDecimal
		}
	}
	for _, v := range tokenAccountMap {
		if v.TokenDecimal == 0 {
			v.TokenDecimal = tokenDecimalMap[v.TokenAddress]
		}
	}
	return
}

func DecodeTokenTransfer(accountKeys []common.PublicKey, instruction *solTypes.CompiledInstruction) (transfer *token.TransferParam, err error) {
	transfer = &token.TransferParam{}
	if accountKeys[instruction.ProgramIDIndex].String() == common.Token2022ProgramID.String() {
		if len(instruction.Accounts) < 3 {
			err = errors.New("not enough accounts")
			return
		}
		if len(instruction.Data) < 1 {
			err = errors.New("data len to0 small")
			return
		}
		if instruction.Data[0] == byte(token.InstructionTransfer) {
			if len(instruction.Data) != 9 {
				err = errors.New("data len not equal 9")
				return
			}
			if len(instruction.Accounts) < 3 {
				err = errors.New("account len too small")
				return
			}
			transfer.From = accountKeys[instruction.Accounts[0]]
			transfer.To = accountKeys[instruction.Accounts[1]]
			transfer.Auth = accountKeys[instruction.Accounts[2]]
			transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:])
		} else if instruction.Data[0] == byte(token.InstructionTransferChecked) {
			if len(instruction.Data) < 10 {
				err = errors.New("data len not equal 10")
				return
			}
			if len(instruction.Accounts) < 4 {
				err = errors.New("account len too small")
				return
			}
			transfer.From = accountKeys[instruction.Accounts[0]]
			// mint := accountKeys[instruction.Accounts[1]]
			transfer.To = accountKeys[instruction.Accounts[2]]
			transfer.Auth = accountKeys[instruction.Accounts[3]]
			transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:10])
			// decimal := instruction.Data[10]
		} else {
			err = errors.New("not transfer Instruction")
			return
		}
		return transfer, nil
	}

	if accountKeys[instruction.ProgramIDIndex].String() != ProgramStrToken {
		err = errors.New("not token program")
		return
	}
	if len(instruction.Accounts) < 3 {
		err = errors.New("not enough accounts")
		return
	}
	if len(instruction.Data) < 1 {
		err = errors.New("data len to0 small")
		return
	}
	if instruction.Data[0] == byte(token.InstructionTransfer) {
		if len(instruction.Data) != 9 {
			err = errors.New("data len not equal 9")
			return
		}
		if len(instruction.Accounts) < 3 {
			err = errors.New("account len too small")
			return
		}
		transfer.From = accountKeys[instruction.Accounts[0]]
		transfer.To = accountKeys[instruction.Accounts[1]]
		transfer.Auth = accountKeys[instruction.Accounts[2]]
		transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:])
	} else if instruction.Data[0] == byte(token.InstructionTransferChecked) {
		if len(instruction.Data) != 10 {
			err = errors.New("data len not equal 10")
			return
		}
		if len(instruction.Accounts) < 4 {
			err = errors.New("account len too small")
			return
		}
		transfer.From = accountKeys[instruction.Accounts[0]]
		// mint := accountKeys[instruction.Accounts[1]]
		transfer.To = accountKeys[instruction.Accounts[2]]
		transfer.Auth = accountKeys[instruction.Accounts[3]]
		transfer.Amount = binary.LittleEndian.Uint64(instruction.Data[1:10])
		// decimal := instruction.Data[10]
	} else {
		err = errors.New("not transfer Instruction")
		return
	}

	return
}

func DecodeInitAccountInstruction(tx *client.BlockTransaction, tokenAccountMap map[string]*TokenAccount, instruction *solTypes.CompiledInstruction) {
	if len(instruction.Data) == 0 {
		return
	}
	var mint, tokenAccount, owner string
	switch token.Instruction(instruction.Data[0]) {
	// init account
	case token.InstructionInitializeAccount:
		if len(instruction.Accounts) < 3 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = tx.AccountKeys[instruction.Accounts[2]].String()
	case token.InstructionInitializeAccount2:
		if len(instruction.Accounts) < 2 || len(instruction.Data) < 33 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(instruction.Data[1:]).String()
	case token.InstructionInitializeAccount3:
		if len(instruction.Accounts) < 2 || len(instruction.Data) < 33 {
			return
		}
		tokenAccount = tx.AccountKeys[instruction.Accounts[0]].String()
		mint = tx.AccountKeys[instruction.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(instruction.Data[1:]).String()
	default:
		return
	}
	if tokenAccountMap[tokenAccount] != nil && tokenAccountMap[tokenAccount].TokenAddress == mint {
		return
	} else {
		tokenAccountMap[tokenAccount] = &TokenAccount{
			Init:                true,
			Owner:               owner,
			TokenAddress:        mint,
			TokenAccountAddress: tokenAccount,
			TokenDecimal:        0,
			PreValue:            0,
			PostValue:           0,
		}
	}
}
