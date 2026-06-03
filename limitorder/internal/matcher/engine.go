package matcher

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
)

// Fill represents a match between maker and taker orders
type Fill struct {
	MakerOrderPDA string
	TakerOrderPDA string
	Price         int64 // Fill price (maker's price)
	Quantity      int64 // Fill quantity in lots
	MakerOwner    string
	TakerOwner    string
}

// MatchResult contains the result of a matching operation
type MatchResult struct {
	Fills       []*Fill
	TakerFilled bool // Whether taker order is fully filled
}

// Engine is the core matching engine
type Engine struct {
	mu sync.RWMutex

	// Order books for each market
	orderBooks map[string]*OrderBook // marketPDA -> orderbook

	// Database connection
	db *sql.DB

	// Executor for on-chain execution
	executor  *Executor
	rpcClient *ag_rpc.Client

	// Worker pool
	workerPoolSize int
	matchQueue     chan *MatchTask
	stopChan       chan struct{}
	wg             sync.WaitGroup

	// Configuration
	config EngineConfig
}

// EngineConfig holds engine configuration
type EngineConfig struct {
	WorkerPoolSize    int
	MatchInterval     time.Duration
	EventBufferSize   int
	RPCURL            string
	MatcherPrivateKey string
	ProgramID         string
}

// MatchTask represents a matching task
type MatchTask struct {
	MarketPDA string
	Order     *Order
}

// NewEngine creates a new matching engine
func NewEngine(db *sql.DB, config EngineConfig) *Engine {
	// 创建 executor（如果配置了 matcher 私钥）
	var executor *Executor
	var rpcClient *ag_rpc.Client
	if config.MatcherPrivateKey != "" && config.RPCURL != "" && config.ProgramID != "" {
		var err error
		executor, err = NewExecutor(config.RPCURL, config.MatcherPrivateKey, config.ProgramID)
		if err != nil {
			logx.Errorf("Failed to create executor: %v", err)
			logx.Infof("Matching engine will run in simulation mode (no on-chain execution)")
		} else {
			logx.Info("Executor initialized successfully")
		}
	} else {
		logx.Infof("Matcher private key not configured, running in simulation mode")
	}

	if config.RPCURL != "" {
		rpcClient = ag_rpc.New(config.RPCURL)
	}

	return &Engine{
		orderBooks:     make(map[string]*OrderBook),
		db:             db,
		executor:       executor,
		rpcClient:      rpcClient,
		workerPoolSize: config.WorkerPoolSize,
		matchQueue:     make(chan *MatchTask, config.EventBufferSize),
		stopChan:       make(chan struct{}),
		config:         config,
	}
}

// Start starts the matching engine
func (e *Engine) Start(ctx context.Context) error {
	logx.Info("Starting matching engine...")

	// Load active orders from database
	if err := e.LoadOrdersFromDatabase(ctx); err != nil {
		logx.Errorf("Failed to load orders from database: %v", err)
		// Don't fail startup, just log the error
	}

	// Start worker pool
	for i := 0; i < e.workerPoolSize; i++ {
		e.wg.Add(1)
		go e.worker(ctx, i)
	}

	// Start periodic matching
	e.wg.Add(1)
	go e.periodicMatcher(ctx)

	logx.Infof("Matching engine started with %d workers", e.workerPoolSize)
	return nil
}

// Stop stops the matching engine
func (e *Engine) Stop() {
	logx.Info("Stopping matching engine...")
	close(e.stopChan)
	e.wg.Wait()
	logx.Info("Matching engine stopped")
}

// worker processes matching tasks
func (e *Engine) worker(ctx context.Context, id int) {
	defer e.wg.Done()

	logx.Infof("Matcher worker %d started", id)

	for {
		select {
		case <-e.stopChan:
			logx.Infof("Matcher worker %d stopping", id)
			return

		case task := <-e.matchQueue:
			e.processMatchTask(ctx, task)
		}
	}
}

// periodicMatcher performs periodic matching for all markets
func (e *Engine) periodicMatcher(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.MatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return

		case <-ticker.C:
			e.matchAllMarkets(ctx)
		}
	}
}

// matchAllMarkets attempts to match orders in all markets
func (e *Engine) matchAllMarkets(ctx context.Context) {
	e.mu.RLock()
	markets := make([]string, 0, len(e.orderBooks))
	for marketPDA := range e.orderBooks {
		markets = append(markets, marketPDA)
	}
	e.mu.RUnlock()

	for _, marketPDA := range markets {
		e.matchMarket(ctx, marketPDA)
	}
}

// 添加订单到内存订单簿，并将订单加入匹配队列，等待匹配任务执行
func (e *Engine) AddOrder(order *Order) {
	e.mu.Lock()

	// 获取或者创建一个订单簿
	ob, exists := e.orderBooks[order.MarketPDA]
	if !exists {
		ob = NewOrderBook(order.MarketPDA)
		e.orderBooks[order.MarketPDA] = ob
	}

	e.mu.Unlock()

	// 添加订单到内存订单簿
	ob.AddOrder(order)

	// 将订单加入队列，等待匹配任务执行
	e.matchQueue <- &MatchTask{
		MarketPDA: order.MarketPDA,
		Order:     order,
	}

	logx.Infof("Order added: %s, market: %s, side: %d, price: %d, qty: %d",
		order.OrderPDA, order.MarketPDA, order.Side, order.PriceLots, order.RemainingLots)
}

// CancelOrder removes an order from the order book
func (e *Engine) CancelOrder(marketPDA string, orderPDA string) bool {
	e.mu.RLock()
	ob, exists := e.orderBooks[marketPDA]
	e.mu.RUnlock()

	if !exists {
		return false
	}

	order := ob.RemoveOrder(orderPDA)
	if order != nil {
		logx.Infof("Order cancelled: %s", orderPDA)
		return true
	}

	return false
}

// processMatchTask processes a matching task
func (e *Engine) processMatchTask(ctx context.Context, task *MatchTask) {
	e.matchMarket(ctx, task.MarketPDA)
}

// matchMarket performs matching for a specific market
func (e *Engine) matchMarket(ctx context.Context, marketPDA string) {
	e.mu.RLock()
	// 查询内存订单簿
	ob, exists := e.orderBooks[marketPDA]
	e.mu.RUnlock()

	if !exists {
		return
	}

	currentSlot := e.getCurrentSlot(ctx)

	// 循环吃盘，获取所有成交明细Fills
	fills := e.match(ctx, ob, currentSlot)

	if len(fills) > 0 {
		logx.Infof("Market %s: matched %d fills", marketPDA, len(fills))

		// 链上执行所有成交明细
		e.executeFills(ctx, marketPDA, fills)
	}
}

// 循环吃盘，买一价 ≥ 卖一价就交叉撮合，按挂单时间先后判定 Maker/Taker、成交价取 Maker 报价，
// 生成成交记录 Fill，更新盘口剩余量，订单吃完自动撤掉
// 返回[]*Fill：本轮所有成交明细，用于下发链上调用settle_match
func (e *Engine) match(ctx context.Context, ob *OrderBook, currentSlot uint64) []*Fill {
	fills := []*Fill{}

	for {
		bestBid := ob.GetBestBid()
		bestAsk := ob.GetBestAsk()

		// 缺买盘/缺卖盘，撮合终止
		if bestBid == nil || bestAsk == nil {
			break
		}

		// Get first order from each side (FIFO)
		// 获取买一和卖一订单
		if len(bestBid.Orders) == 0 || len(bestAsk.Orders) == 0 {
			break
		}

		bidOrder := bestBid.Orders[0]
		if currentSlot > 0 && e.isOrderExpired(bidOrder, currentSlot) {
			e.expireOrder(ctx, ob, bidOrder)
			// 删掉过期单，重头新一轮循环重新拿盘口
			continue
		}

		askOrder := bestAsk.Orders[0]
		if currentSlot > 0 && e.isOrderExpired(askOrder, currentSlot) {
			e.expireOrder(ctx, ob, askOrder)
			// 删掉过期单，重头新一轮循环重新拿盘口
			continue
		}

		// 买价 < 卖价无交叉，停止撮合
		if bestBid.Price < bestAsk.Price {
			break
		}

		// 撮合成交数量：取两边剩余更小值（部分成交 / 全吃）
		// 例：买单剩 100，卖单剩 60 → 本次成交 60；卖单吃光，买单剩 40 留在盘口。
		fillQty := bidOrder.RemainingLots
		if askOrder.RemainingLots < fillQty {
			fillQty = askOrder.RemainingLots
		}

		// Determine fill price (maker price has priority)
		// 临时默认：Ask是Maker
		// 规则：先挂单 = Maker，后挂单 = Taker；成交价固定使用 Maker 的挂单价（主流订单簿规则）
		fillPrice := bestAsk.Price
		if askOrder.Timestamp < bidOrder.Timestamp {
			// Ask order is maker
			fillPrice = bestAsk.Price
		} else {
			// Bid order is maker
			fillPrice = bestBid.Price
		}

		// 创建成交记录 Fill
		fill := &Fill{
			MakerOrderPDA: askOrder.OrderPDA,
			TakerOrderPDA: bidOrder.OrderPDA,
			Price:         fillPrice,
			Quantity:      fillQty,
			MakerOwner:    askOrder.Owner,
			TakerOwner:    bidOrder.Owner,
		}

		// Determine actual maker/taker based on timestamp
		// 修正：如果Bid挂单更早，则Bid变Maker
		if bidOrder.Timestamp < askOrder.Timestamp {
			fill.MakerOrderPDA = bidOrder.OrderPDA
			fill.TakerOrderPDA = askOrder.OrderPDA
			fill.MakerOwner = bidOrder.Owner
			fill.TakerOwner = askOrder.Owner
		}

		fills = append(fills, fill)

		// 扣减订单剩余量
		bidOrder.RemainingLots -= fillQty
		askOrder.RemainingLots -= fillQty

		// 剩余=0：完全成交，从订单簿彻底移除；剩余>0：部分成交，更新订单簿剩余量
		if bidOrder.RemainingLots == 0 {
			ob.RemoveOrder(bidOrder.OrderPDA)
		} else {
			ob.UpdateOrderQuantity(bidOrder.OrderPDA, bidOrder.RemainingLots)
		}

		// 剩余=0：完全成交，从订单簿彻底移除；剩余>0：部分成交，更新订单簿剩余量
		if askOrder.RemainingLots == 0 {
			ob.RemoveOrder(askOrder.OrderPDA)
		} else {
			ob.UpdateOrderQuantity(askOrder.OrderPDA, askOrder.RemainingLots)
		}

		logx.Infof("Matched: maker=%s, taker=%s, price=%d, qty=%d",
			fill.MakerOrderPDA, fill.TakerOrderPDA, fill.Price, fill.Quantity)
	}

	return fills
}

// 执行每个成交：构建并发送交易，调用链上 match_orders 指令；失败重试，直到成功或订单过期；成功后保存成交记录到数据库
func (e *Engine) executeFills(ctx context.Context, marketPDA string, fills []*Fill) {
	if e.executor == nil {
		logx.Infof("Executor not configured, skipping on-chain execution")
		for _, fill := range fills {
			logx.Infof("[SIMULATION] Match: market=%s, maker=%s, taker=%s, price=%d, qty=%d",
				marketPDA, fill.MakerOrderPDA, fill.TakerOrderPDA, fill.Price, fill.Quantity)
		}
		return
	}

	currentSlot := e.getCurrentSlot(ctx)

	// 获取市场信息
	marketInfo, err := e.getMarketInfo(ctx, marketPDA)
	if err != nil {
		logx.Errorf("Failed to get market info for %s: %v", marketPDA, err)
		return
	}

	// 执行每个 fill
	for _, fill := range fills {
		if currentSlot > 0 && e.expireFillOrders(ctx, marketPDA, fill, currentSlot) {
			continue
		}

		// 执行单个Fill成交：构建并发送交易，调用链上 match_orders 指令
		if err := e.executor.ExecuteFill(ctx, fill, marketInfo); err != nil {
			if e.isOrderExpiredError(err) {
				slot := currentSlot
				if slot == 0 {
					slot = e.getCurrentSlot(ctx)
				}
				if slot > 0 {
					e.expireFillOrders(ctx, marketPDA, fill, slot)
				}
				logx.Infof("Skip fill: order expired on-chain, market=%s, maker=%s, taker=%s",
					marketPDA, fill.MakerOrderPDA, fill.TakerOrderPDA)
				continue
			}
			logx.Errorf("Failed to execute fill: %v", err)
			// TODO: 回滚内存中的订单簿状态
			continue
		}

		// 成功后保存成交记录到数据库
		if err := e.saveFillToDatabase(ctx, fill, marketPDA); err != nil {
			logx.Errorf("Failed to save fill to database: %v", err)
		}
	}
}

func (e *Engine) getCurrentSlot(ctx context.Context) uint64 {
	if e.rpcClient == nil {
		return 0
	}
	slot, err := e.rpcClient.GetSlot(ctx, ag_rpc.CommitmentConfirmed)
	if err != nil {
		logx.Errorf("Failed to get current slot: %v", err)
		return 0
	}
	return slot
}

func (e *Engine) isOrderExpired(order *Order, currentSlot uint64) bool {
	if order.ExpirySlot <= 0 {
		return false
	}
	return currentSlot >= uint64(order.ExpirySlot)
}

func (e *Engine) expireOrder(ctx context.Context, ob *OrderBook, order *Order) {
	ob.RemoveOrder(order.OrderPDA)
	if err := e.markOrderExpired(ctx, order.OrderPDA); err != nil {
		logx.Errorf("Failed to mark order expired: %s, error: %v", order.OrderPDA, err)
		return
	}
	logx.Infof("Order expired: %s", order.OrderPDA)
}

func (e *Engine) expireFillOrders(ctx context.Context, marketPDA string, fill *Fill, currentSlot uint64) bool {
	ob := e.GetOrderBook(marketPDA)
	if ob == nil {
		return false
	}

	expired := false

	ob.mu.RLock()
	maker := ob.Orders[fill.MakerOrderPDA]
	taker := ob.Orders[fill.TakerOrderPDA]
	ob.mu.RUnlock()

	if maker != nil && e.isOrderExpired(maker, currentSlot) {
		e.expireOrder(ctx, ob, maker)
		expired = true
	}
	if taker != nil && e.isOrderExpired(taker, currentSlot) {
		e.expireOrder(ctx, ob, taker)
		expired = true
	}

	return expired
}

func (e *Engine) isOrderExpiredError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "OrderExpired") || strings.Contains(msg, "6010")
}

// getMarketInfo retrieves market information from database
func (e *Engine) getMarketInfo(ctx context.Context, marketPDA string) (*MarketInfo, error) {
	query := `
		SELECT market_pda, base_mint, quote_mint, pyth_price_feed
		FROM limit_order_market
		WHERE market_pda = ? AND status = 1
	`

	var info MarketInfo
	var pythPriceFeed sql.NullString

	err := e.db.QueryRowContext(ctx, query, marketPDA).Scan(
		&info.MarketPDA,
		&info.BaseMint,
		&info.QuoteMint,
		&pythPriceFeed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query market info: %w", err)
	}

	if pythPriceFeed.Valid {
		info.PythPriceFeed = pythPriceFeed.String
	}

	return &info, nil
}

// saveFillToDatabase saves a fill record to database
func (e *Engine) saveFillToDatabase(ctx context.Context, fill *Fill, marketPDA string) error {
	// First, get market_id and decimals from market table
	var marketID int
	var baseDecimals, quoteDecimals int
	var takerFeeBps int
	err := e.db.QueryRowContext(ctx, `
		SELECT id, base_decimals, quote_decimals, taker_fee_bps
		FROM limit_order_market
		WHERE market_pda = ?
	`, marketPDA).Scan(&marketID, &baseDecimals, &quoteDecimals, &takerFeeBps)
	if err != nil {
		return fmt.Errorf("failed to get market info: %w", err)
	}

	// Get maker and taker order IDs from order table
	var makerOrderID, takerOrderID int64
	err = e.db.QueryRowContext(ctx, `
		SELECT id FROM limit_order WHERE order_pda = ?
	`, fill.MakerOrderPDA).Scan(&makerOrderID)
	if err != nil {
		return fmt.Errorf("failed to get maker order ID: %w", err)
	}

	err = e.db.QueryRowContext(ctx, `
		SELECT id FROM limit_order WHERE order_pda = ?
	`, fill.TakerOrderPDA).Scan(&takerOrderID)
	if err != nil {
		return fmt.Errorf("failed to get taker order ID: %w", err)
	}

	// Calculate display values
	// Note: This is a simplified calculation. In production, you'd need proper lot size conversion
	qtyDisplay := float64(fill.Quantity) / float64(1e9) // Assuming 9 decimals for base
	priceDisplay := float64(fill.Price) / float64(1e6)  // Assuming 6 decimals for quote
	totalValueDisplay := qtyDisplay * priceDisplay

	// Calculate fee (taker fee in quote token)
	fee := (fill.Quantity * fill.Price * int64(takerFeeBps)) / 10000
	feeDisplay := float64(fee) / float64(1e6)

	// Insert fill record
	// Note: tx_hash and slot should come from the actual transaction, but we'll use placeholders for now
	query := `
		INSERT INTO limit_order_fill
		(maker_order_id, taker_order_id, market_id, market_pda, maker, taker,
		 qty_lots, price_lots, fee, qty_display, price_display, fee_display,
		 total_value_display, tx_hash, slot, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
	`

	_, err = e.db.ExecContext(ctx, query,
		makerOrderID,
		takerOrderID,
		marketID,
		marketPDA,
		fill.MakerOwner,
		fill.TakerOwner,
		fill.Quantity,
		fill.Price,
		fee,
		qtyDisplay,
		priceDisplay,
		feeDisplay,
		totalValueDisplay,
		"pending", // Placeholder for tx_hash - should be updated after transaction confirms
		0,         // Placeholder for slot - should be updated after transaction confirms
	)

	return err
}

func (e *Engine) markOrderExpired(ctx context.Context, orderPDA string) error {
	query := `
		UPDATE limit_order
		SET status = 4,
		    expired_at = NOW(),
		    updated_at = NOW()
		WHERE order_pda = ? AND status = 1
	`
	_, err := e.db.ExecContext(ctx, query, orderPDA)
	if err != nil {
		return fmt.Errorf("failed to update expired order: %w", err)
	}
	return nil
}

// GetOrderBook returns the order book for a market
func (e *Engine) GetOrderBook(marketPDA string) *OrderBook {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.orderBooks[marketPDA]
}

// GetOrCreateOrderBook gets or creates an order book for a market
func (e *Engine) GetOrCreateOrderBook(marketPDA string) *OrderBook {
	e.mu.Lock()
	defer e.mu.Unlock()

	ob, exists := e.orderBooks[marketPDA]
	if !exists {
		ob = NewOrderBook(marketPDA)
		e.orderBooks[marketPDA] = ob
	}

	return ob
}

// LoadOrdersFromDatabase loads active orders from database
func (e *Engine) LoadOrdersFromDatabase(ctx context.Context) error {
	logx.Info("Loading active orders from database...")

	// 1. 查询所有活跃订单 (status=1)
	query := `
		SELECT order_pda, market_pda, owner, side, price_lots, remaining_lots, expiry_slot, created_at
		FROM limit_order
		WHERE status = 1
		ORDER BY market_pda, created_at
	`

	rows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query active orders: %w", err)
	}
	defer rows.Close()

	// 2. 按市场分组
	ordersByMarket := make(map[string][]*Order)
	for rows.Next() {
		var dbOrder struct {
			OrderPda      string
			MarketPda     string
			Owner         string
			Side          int64
			PriceLots     int64
			RemainingLots int64
			ExpirySlot    int64
			CreatedAt     time.Time
		}

		if err := rows.Scan(
			&dbOrder.OrderPda,
			&dbOrder.MarketPda,
			&dbOrder.Owner,
			&dbOrder.Side,
			&dbOrder.PriceLots,
			&dbOrder.RemainingLots,
			&dbOrder.ExpirySlot,
			&dbOrder.CreatedAt,
		); err != nil {
			return fmt.Errorf("failed to scan order: %w", err)
		}

		order := &Order{
			OrderPDA:      dbOrder.OrderPda,
			MarketPDA:     dbOrder.MarketPda,
			Owner:         dbOrder.Owner,
			Side:          Side(dbOrder.Side),
			PriceLots:     dbOrder.PriceLots,
			QtyLots:       dbOrder.RemainingLots,
			RemainingLots: dbOrder.RemainingLots,
			ExpirySlot:    dbOrder.ExpirySlot,
			Timestamp:     dbOrder.CreatedAt.Unix(),
		}
		ordersByMarket[dbOrder.MarketPda] = append(ordersByMarket[dbOrder.MarketPda], order)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// 3. 添加到各个订单簿
	totalOrders := 0
	for marketPDA, marketOrders := range ordersByMarket {
		book := e.GetOrCreateOrderBook(marketPDA)
		for _, order := range marketOrders {
			book.AddOrder(order)
			totalOrders++
		}
	}

	logx.Infof("Loaded %d active orders from %d markets", totalOrders, len(ordersByMarket))
	return nil
}

// SyncOrderFromDB refreshes a single order in the in-memory order book.
func (e *Engine) SyncOrderFromDB(ctx context.Context, marketPDA, orderPDA string) error {
	if orderPDA == "" {
		return nil
	}

	query := `
		SELECT order_pda, market_pda, owner, side, price_lots, remaining_lots, expiry_slot, created_at, status
		FROM limit_order
		WHERE order_pda = ?
	`

	var dbOrder struct {
		OrderPda      string
		MarketPda     string
		Owner         string
		Side          int64
		PriceLots     int64
		RemainingLots int64
		ExpirySlot    int64
		CreatedAt     time.Time
		Status        int64
	}

	err := e.db.QueryRowContext(ctx, query, orderPDA).Scan(
		&dbOrder.OrderPda,
		&dbOrder.MarketPda,
		&dbOrder.Owner,
		&dbOrder.Side,
		&dbOrder.PriceLots,
		&dbOrder.RemainingLots,
		&dbOrder.ExpirySlot,
		&dbOrder.CreatedAt,
		&dbOrder.Status,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			if marketPDA != "" {
				e.CancelOrder(marketPDA, orderPDA)
			} else {
				e.removeOrderFromAllBooks(orderPDA)
			}
			return nil
		}
		return fmt.Errorf("failed to query order: %w", err)
	}

	if marketPDA == "" {
		marketPDA = dbOrder.MarketPda
	}

	if dbOrder.Status != 1 {
		e.CancelOrder(marketPDA, orderPDA)
		return nil
	}

	side := Side(dbOrder.Side)
	if side != SideBid && side != SideAsk {
		return fmt.Errorf("invalid order side %d for %s", dbOrder.Side, orderPDA)
	}

	order := &Order{
		OrderPDA:      dbOrder.OrderPda,
		MarketPDA:     dbOrder.MarketPda,
		Owner:         dbOrder.Owner,
		Side:          side,
		PriceLots:     dbOrder.PriceLots,
		QtyLots:       dbOrder.RemainingLots,
		RemainingLots: dbOrder.RemainingLots,
		ExpirySlot:    dbOrder.ExpirySlot,
		Timestamp:     dbOrder.CreatedAt.Unix(),
	}

	ob := e.GetOrCreateOrderBook(marketPDA)
	ob.mu.RLock()
	existing := ob.Orders[orderPDA]
	ob.mu.RUnlock()

	if existing == nil {
		e.AddOrder(order)
		return nil
	}

	if existing.Side != order.Side || existing.PriceLots != order.PriceLots {
		e.CancelOrder(marketPDA, orderPDA)
		e.AddOrder(order)
		return nil
	}

	ob.UpdateOrderQuantity(orderPDA, order.RemainingLots)
	return nil
}

func (e *Engine) removeOrderFromAllBooks(orderPDA string) {
	e.mu.RLock()
	markets := make([]string, 0, len(e.orderBooks))
	for marketPDA := range e.orderBooks {
		markets = append(markets, marketPDA)
	}
	e.mu.RUnlock()

	for _, marketPDA := range markets {
		e.CancelOrder(marketPDA, orderPDA)
	}
}

// GetOrderBookDepth returns order book depth data
func (e *Engine) GetOrderBookDepth(marketPDA string, depth int) (bids, asks []*PriceLevel) {
	e.mu.RLock()
	ob, exists := e.orderBooks[marketPDA]
	e.mu.RUnlock()

	if !exists {
		return nil, nil
	}

	return ob.GetTopLevels(depth)
}

// GetOrderBookStats returns statistics for an order book
func (e *Engine) GetOrderBookStats(marketPDA string) (bidCount, askCount, totalOrders int) {
	e.mu.RLock()
	ob, exists := e.orderBooks[marketPDA]
	e.mu.RUnlock()

	if !exists {
		return 0, 0, 0
	}

	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bidCount = ob.Bids.Len()
	askCount = ob.Asks.Len()
	totalOrders = len(ob.Orders)

	return
}

// Example usage:
// engine := NewEngine(db, EngineConfig{
//     WorkerPoolSize:  10,
//     MatchInterval:   1 * time.Second,
//     EventBufferSize: 1000,
// })
// engine.Start(ctx)
// defer engine.Stop()
