package matcher

import (
	"container/heap"
	"sync"
)

// Side represents order side
type Side int

const (
	SideBid Side = 1 // Buy
	SideAsk Side = 2 // Sell
)

// Order represents an order in the order book
type Order struct {
	OrderPDA      string // Order PDA address
	OrderID       int64  // Order sequence number
	MarketPDA     string // Market PDA
	Owner         string // Owner address
	Side          Side   // Bid or Ask
	PriceLots     int64  // Price in lots
	QtyLots       int64  // Original quantity in lots
	RemainingLots int64  // Remaining quantity in lots
	ExpirySlot    int64  // Expiry slot (0 = no expiry)
	Timestamp     int64  // Creation timestamp (for time priority)
}

// PriceLevel represents all orders at a specific price level
type PriceLevel struct {
	Price  int64    // Price in lots
	Orders []*Order // Orders at this price (FIFO queue)
	TotalQty int64  // Total quantity at this price level
	index  int      // Index in heap (for heap.Interface)
}

// BidHeap is a max-heap for bid orders (higher price has higher priority)
type BidHeap []*PriceLevel

func (h BidHeap) Len() int           { return len(h) }
func (h BidHeap) Less(i, j int) bool { return h[i].Price > h[j].Price } // Max heap
func (h BidHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *BidHeap) Push(x interface{}) {
	n := len(*h)
	level := x.(*PriceLevel)
	level.index = n
	*h = append(*h, level)
}

func (h *BidHeap) Pop() interface{} {
	old := *h
	n := len(old)
	level := old[n-1]
	old[n-1] = nil
	level.index = -1
	*h = old[0 : n-1]
	return level
}

// AskHeap is a min-heap for ask orders (lower price has higher priority)
type AskHeap []*PriceLevel

func (h AskHeap) Len() int           { return len(h) }
func (h AskHeap) Less(i, j int) bool { return h[i].Price < h[j].Price } // Min heap
func (h AskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *AskHeap) Push(x interface{}) {
	n := len(*h)
	level := x.(*PriceLevel)
	level.index = n
	*h = append(*h, level)
}

func (h *AskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	level := old[n-1]
	old[n-1] = nil
	level.index = -1
	*h = old[0 : n-1]
	return level
}

// OrderBook manages orders for a specific market
type OrderBook struct {
	mu sync.RWMutex

	MarketPDA string // Market identifier

	// Heaps for price levels
	Bids *BidHeap
	Asks *AskHeap

	// Price level lookup maps
	BidLevels map[int64]*PriceLevel // price -> level
	AskLevels map[int64]*PriceLevel // price -> level

	// Order lookup map
	Orders map[string]*Order // orderPDA -> order
}

// NewOrderBook creates a new order book
func NewOrderBook(marketPDA string) *OrderBook {
	bids := &BidHeap{}
	asks := &AskHeap{}
	heap.Init(bids)
	heap.Init(asks)

	return &OrderBook{
		MarketPDA: marketPDA,
		Bids:      bids,
		Asks:      asks,
		BidLevels: make(map[int64]*PriceLevel),
		AskLevels: make(map[int64]*PriceLevel),
		Orders:    make(map[string]*Order),
	}
}

// AddOrder adds an order to the order book
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Store order in lookup map
	ob.Orders[order.OrderPDA] = order

	// Add to appropriate side
	if order.Side == SideBid {
		ob.addBidOrder(order)
	} else {
		ob.addAskOrder(order)
	}
}

// addBidOrder adds a bid order to the bid side
func (ob *OrderBook) addBidOrder(order *Order) {
	price := order.PriceLots

	// Get or create price level
	level, exists := ob.BidLevels[price]
	if !exists {
		level = &PriceLevel{
			Price:  price,
			Orders: []*Order{},
		}
		ob.BidLevels[price] = level
		heap.Push(ob.Bids, level)
	}

	// Append order to level (FIFO)
	level.Orders = append(level.Orders, order)
	level.TotalQty += order.RemainingLots

	// Update heap
	heap.Fix(ob.Bids, level.index)
}

// addAskOrder adds an ask order to the ask side
func (ob *OrderBook) addAskOrder(order *Order) {
	price := order.PriceLots

	// Get or create price level
	level, exists := ob.AskLevels[price]
	if !exists {
		level = &PriceLevel{
			Price:  price,
			Orders: []*Order{},
		}
		ob.AskLevels[price] = level
		heap.Push(ob.Asks, level)
	}

	// Append order to level (FIFO)
	level.Orders = append(level.Orders, order)
	level.TotalQty += order.RemainingLots

	// Update heap
	heap.Fix(ob.Asks, level.index)
}

// RemoveOrder removes an order from the order book
func (ob *OrderBook) RemoveOrder(orderPDA string) *Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, exists := ob.Orders[orderPDA]
	if !exists {
		return nil
	}

	// Remove from orders map
	delete(ob.Orders, orderPDA)

	// Remove from price level
	if order.Side == SideBid {
		ob.removeBidOrder(order)
	} else {
		ob.removeAskOrder(order)
	}

	return order
}

// removeBidOrder removes a bid order from its price level
func (ob *OrderBook) removeBidOrder(order *Order) {
	level, exists := ob.BidLevels[order.PriceLots]
	if !exists {
		return
	}

	// Remove order from level
	for i, o := range level.Orders {
		if o.OrderPDA == order.OrderPDA {
			level.Orders = append(level.Orders[:i], level.Orders[i+1:]...)
			level.TotalQty -= order.RemainingLots
			break
		}
	}

	// If level is empty, remove it from heap
	if len(level.Orders) == 0 {
		delete(ob.BidLevels, order.PriceLots)
		heap.Remove(ob.Bids, level.index)
	} else {
		heap.Fix(ob.Bids, level.index)
	}
}

// removeAskOrder removes an ask order from its price level
func (ob *OrderBook) removeAskOrder(order *Order) {
	level, exists := ob.AskLevels[order.PriceLots]
	if !exists {
		return
	}

	// Remove order from level
	for i, o := range level.Orders {
		if o.OrderPDA == order.OrderPDA {
			level.Orders = append(level.Orders[:i], level.Orders[i+1:]...)
			level.TotalQty -= order.RemainingLots
			break
		}
	}

	// If level is empty, remove it from heap
	if len(level.Orders) == 0 {
		delete(ob.AskLevels, order.PriceLots)
		heap.Remove(ob.Asks, level.index)
	} else {
		heap.Fix(ob.Asks, level.index)
	}
}

// GetBestBid returns the best bid price level (highest price)
func (ob *OrderBook) GetBestBid() *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if ob.Bids.Len() == 0 {
		return nil
	}
	return (*ob.Bids)[0]
}

// GetBestAsk returns the best ask price level (lowest price)
func (ob *OrderBook) GetBestAsk() *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if ob.Asks.Len() == 0 {
		return nil
	}
	return (*ob.Asks)[0]
}

// GetTopLevels returns top N price levels for bids and asks
func (ob *OrderBook) GetTopLevels(depth int) (bids []*PriceLevel, asks []*PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	// Get top bid levels
	bidCount := depth
	if bidCount > ob.Bids.Len() {
		bidCount = ob.Bids.Len()
	}
	bids = make([]*PriceLevel, bidCount)
	for i := 0; i < bidCount; i++ {
		bids[i] = (*ob.Bids)[i]
	}

	// Get top ask levels
	askCount := depth
	if askCount > ob.Asks.Len() {
		askCount = ob.Asks.Len()
	}
	asks = make([]*PriceLevel, askCount)
	for i := 0; i < askCount; i++ {
		asks[i] = (*ob.Asks)[i]
	}

	return bids, asks
}

// UpdateOrderQuantity updates the remaining quantity of an order
func (ob *OrderBook) UpdateOrderQuantity(orderPDA string, newRemainingLots int64) bool {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, exists := ob.Orders[orderPDA]
	if !exists {
		return false
	}

	delta := newRemainingLots - order.RemainingLots
	order.RemainingLots = newRemainingLots

	// Update price level total quantity
	if order.Side == SideBid {
		if level, exists := ob.BidLevels[order.PriceLots]; exists {
			level.TotalQty += delta
			heap.Fix(ob.Bids, level.index)
		}
	} else {
		if level, exists := ob.AskLevels[order.PriceLots]; exists {
			level.TotalQty += delta
			heap.Fix(ob.Asks, level.index)
		}
	}

	// If fully filled, remove order
	if newRemainingLots == 0 {
		delete(ob.Orders, orderPDA)
	}

	return true
}
