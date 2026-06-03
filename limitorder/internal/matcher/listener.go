package matcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// EventType represents the type of blockchain event
type EventType int

const (
	EventOrderCreated EventType = iota
	EventOrderCancelled
	EventOrderFilled
	EventOrderExpired
)

// Event represents a blockchain event
type Event struct {
	Type      EventType
	Timestamp int64
	Data      interface{}
}

// OrderCreatedEvent represents an order creation event
type OrderCreatedEvent struct {
	OrderPDA      string
	MarketPDA     string
	Owner         string
	Side          Side
	PriceLots     int64
	QtyLots       int64
	ExpirySlot    int64
	Timestamp     int64
}

// OrderCancelledEvent represents an order cancellation event
type OrderCancelledEvent struct {
	OrderPDA   string
	MarketPDA  string
	Owner      string
	Timestamp  int64
}

// OrderFilledEvent represents an order fill event
type OrderFilledEvent struct {
	MakerOrderPDA string
	TakerOrderPDA string
	MarketPDA     string
	Price         int64
	Quantity      int64
	MakerFilled   bool
	TakerFilled   bool
	Timestamp     int64
}

// EventListener listens to Solana blockchain events
type EventListener struct {
	mu sync.RWMutex

	// Event channel
	eventChan chan *Event

	// Matching engine reference
	engine *Engine

	// WebSocket connection (placeholder)
	// wsConn *websocket.Conn

	// Stop channel
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Configuration
	config ListenerConfig
}

// ListenerConfig holds listener configuration
type ListenerConfig struct {
	WebSocketURL      string
	ReconnectInterval time.Duration
	EventBufferSize   int
}

// NewEventListener creates a new event listener
func NewEventListener(engine *Engine, config ListenerConfig) *EventListener {
	return &EventListener{
		eventChan: make(chan *Event, config.EventBufferSize),
		engine:    engine,
		stopChan:  make(chan struct{}),
		config:    config,
	}
}

// Start starts the event listener
func (l *EventListener) Start(ctx context.Context) error {
	logx.Info("Starting event listener...")

	// Start event processor
	l.wg.Add(1)
	go l.processEvents(ctx)

	// Start WebSocket connection (placeholder)
	l.wg.Add(1)
	go l.connectAndListen(ctx)

	logx.Info("Event listener started")
	return nil
}

// Stop stops the event listener
func (l *EventListener) Stop() {
	logx.Info("Stopping event listener...")
	close(l.stopChan)
	l.wg.Wait()
	logx.Info("Event listener stopped")
}

// connectAndListen connects to Solana WebSocket and listens for events
func (l *EventListener) connectAndListen(ctx context.Context) {
	defer l.wg.Done()

	for {
		select {
		case <-l.stopChan:
			return

		default:
			// TODO: Implement WebSocket connection to Solana
			// This would involve:
			// 1. Connect to Solana WebSocket endpoint
			// 2. Subscribe to program logs for limit order program
			// 3. Parse event logs
			// 4. Send events to event channel

			// Placeholder implementation
			logx.Info("WebSocket listener running (placeholder)...")
			time.Sleep(l.config.ReconnectInterval)

			// Example of how events would be sent:
			// event := &Event{
			//     Type: EventOrderCreated,
			//     Timestamp: time.Now().Unix(),
			//     Data: &OrderCreatedEvent{...},
			// }
			// l.eventChan <- event
		}
	}
}

// processEvents processes incoming events
func (l *EventListener) processEvents(ctx context.Context) {
	defer l.wg.Done()

	for {
		select {
		case <-l.stopChan:
			return

		case event := <-l.eventChan:
			l.handleEvent(ctx, event)
		}
	}
}

// handleEvent handles a single event
func (l *EventListener) handleEvent(ctx context.Context, event *Event) {
	switch event.Type {
	case EventOrderCreated:
		l.handleOrderCreated(ctx, event.Data.(*OrderCreatedEvent))

	case EventOrderCancelled:
		l.handleOrderCancelled(ctx, event.Data.(*OrderCancelledEvent))

	case EventOrderFilled:
		l.handleOrderFilled(ctx, event.Data.(*OrderFilledEvent))

	case EventOrderExpired:
		// Similar to cancelled
		l.handleOrderCancelled(ctx, event.Data.(*OrderCancelledEvent))

	default:
		logx.Errorf("Unknown event type: %d", event.Type)
	}
}

// handleOrderCreated handles order creation events
func (l *EventListener) handleOrderCreated(ctx context.Context, data *OrderCreatedEvent) {
	logx.Infof("Order created event: %s", data.OrderPDA)

	// Create order object
	order := &Order{
		OrderPDA:      data.OrderPDA,
		MarketPDA:     data.MarketPDA,
		Owner:         data.Owner,
		Side:          data.Side,
		PriceLots:     data.PriceLots,
		QtyLots:       data.QtyLots,
		RemainingLots: data.QtyLots,
		ExpirySlot:    data.ExpirySlot,
		Timestamp:     data.Timestamp,
	}

	// Add to matching engine
	l.engine.AddOrder(order)

	// Save order to database
	err := l.saveOrderToDatabase(ctx, data)
	if err != nil {
		logx.Errorf("Failed to save order to database: %s, error: %v", data.OrderPDA, err)
		// Continue processing even if DB save fails - order is already in memory
	}
}

// handleOrderCancelled handles order cancellation events
func (l *EventListener) handleOrderCancelled(ctx context.Context, data *OrderCancelledEvent) {
	logx.Infof("Order cancelled event: %s", data.OrderPDA)

	// Remove from matching engine
	l.engine.CancelOrder(data.MarketPDA, data.OrderPDA)

	// Update database
	err := l.updateOrderCancelled(ctx, data)
	if err != nil {
		logx.Errorf("Failed to update cancelled order in database: %s, error: %v", data.OrderPDA, err)
	}
}

// handleOrderFilled handles order fill events
func (l *EventListener) handleOrderFilled(ctx context.Context, data *OrderFilledEvent) {
	logx.Infof("Order filled event: maker=%s, taker=%s, qty=%d",
		data.MakerOrderPDA, data.TakerOrderPDA, data.Quantity)

	// Update maker order
	if !data.MakerFilled {
		// Partially filled, update quantity
		ob := l.engine.GetOrderBook(data.MarketPDA)
		if ob != nil {
			// Calculate new remaining quantity
			// This should come from the event data in production
			ob.mu.RLock()
			makerOrder, exists := ob.Orders[data.MakerOrderPDA]
			ob.mu.RUnlock()

			if exists {
				newRemaining := makerOrder.RemainingLots - data.Quantity
				if newRemaining < 0 {
					newRemaining = 0
				}
				ob.UpdateOrderQuantity(data.MakerOrderPDA, newRemaining)
			}
		}
	} else {
		// Fully filled, remove order
		l.engine.CancelOrder(data.MarketPDA, data.MakerOrderPDA)
	}

	// Update taker order
	if !data.TakerFilled {
		// Partially filled, update quantity
		ob := l.engine.GetOrderBook(data.MarketPDA)
		if ob != nil {
			ob.mu.RLock()
			takerOrder, exists := ob.Orders[data.TakerOrderPDA]
			ob.mu.RUnlock()

			if exists {
				newRemaining := takerOrder.RemainingLots - data.Quantity
				if newRemaining < 0 {
					newRemaining = 0
				}
				ob.UpdateOrderQuantity(data.TakerOrderPDA, newRemaining)
			}
		}
	} else {
		// Fully filled, remove order
		l.engine.CancelOrder(data.MarketPDA, data.TakerOrderPDA)
	}

	// Save fill to database
	err := l.saveFillToDatabase(ctx, data)
	if err != nil {
		logx.Errorf("Failed to save fill to database: maker=%s, taker=%s, error: %v",
			data.MakerOrderPDA, data.TakerOrderPDA, err)
	}
}

// SubscribeToMarket subscribes to events for a specific market
func (l *EventListener) SubscribeToMarket(marketPDA string) error {
	// TODO: Implement market-specific subscription
	// This would involve subscribing to Solana program accounts
	// filtered by market PDA

	logx.Infof("Subscribed to market: %s", marketPDA)
	return nil
}

// UnsubscribeFromMarket unsubscribes from events for a specific market
func (l *EventListener) UnsubscribeFromMarket(marketPDA string) error {
	// TODO: Implement market-specific unsubscription

	logx.Infof("Unsubscribed from market: %s", marketPDA)
	return nil
}

// Database operation methods

// saveOrderToDatabase saves a new order to the database
func (l *EventListener) saveOrderToDatabase(ctx context.Context, data *OrderCreatedEvent) error {
	query := `
		INSERT INTO limit_order (
			order_pda, market_pda, owner, side,
			price_lots, qty_lots, remaining_lots,
			expiry_slot, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, NOW(), NOW())
	`

	_, err := l.engine.db.ExecContext(ctx, query,
		data.OrderPDA,
		data.MarketPDA,
		data.Owner,
		int(data.Side),
		data.PriceLots,
		data.QtyLots,
		data.QtyLots, // Initial remaining = qty
		data.ExpirySlot,
	)

	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}

	logx.Infof("Saved order to database: %s", data.OrderPDA)
	return nil
}

// updateOrderCancelled updates a cancelled order in the database
func (l *EventListener) updateOrderCancelled(ctx context.Context, data *OrderCancelledEvent) error {
	// Status: 2 = Cancelled
	query := `
		UPDATE limit_order
		SET status = 2,
		    cancelled_at = NOW(),
		    updated_at = NOW()
		WHERE order_pda = ?
	`

	_, err := l.engine.db.ExecContext(ctx, query, data.OrderPDA)
	if err != nil {
		return fmt.Errorf("failed to update cancelled order: %w", err)
	}

	logx.Infof("Updated cancelled order in database: %s", data.OrderPDA)
	return nil
}

// saveFillToDatabase saves a fill record and updates order statuses
func (l *EventListener) saveFillToDatabase(ctx context.Context, data *OrderFilledEvent) error {
	// Start a transaction to ensure atomicity
	// Note: go-zero's sqlx doesn't have built-in transaction support,
	// so we'll execute updates sequentially and rely on database constraints

	// 1. Insert fill record
	fillQuery := `
		INSERT INTO limit_order_fill (
			market_pda, maker_order_pda, taker_order_pda,
			price_lots, qty_lots, fill_time, created_at
		) VALUES (?, ?, ?, ?, ?, NOW(), NOW())
	`

	_, err := l.engine.db.ExecContext(ctx, fillQuery,
		data.MarketPDA,
		data.MakerOrderPDA,
		data.TakerOrderPDA,
		data.Price,
		data.Quantity,
	)

	if err != nil {
		return fmt.Errorf("failed to insert fill record: %w", err)
	}

	// 2. Update maker order
	if data.MakerFilled {
		// Fully filled - set status to Filled (3)
		_, err = l.engine.db.ExecContext(ctx, `
			UPDATE limit_order
			SET status = 3,
			    remaining_lots = 0,
			    filled_at = NOW(),
			    updated_at = NOW()
			WHERE order_pda = ?
		`, data.MakerOrderPDA)
	} else {
		// Partially filled - update remaining quantity
		_, err = l.engine.db.ExecContext(ctx, `
			UPDATE limit_order
			SET remaining_lots = remaining_lots - ?,
			    updated_at = NOW()
			WHERE order_pda = ?
		`, data.Quantity, data.MakerOrderPDA)
	}

	if err != nil {
		return fmt.Errorf("failed to update maker order: %w", err)
	}

	// 3. Update taker order
	if data.TakerFilled {
		// Fully filled - set status to Filled (3)
		_, err = l.engine.db.ExecContext(ctx, `
			UPDATE limit_order
			SET status = 3,
			    remaining_lots = 0,
			    filled_at = NOW(),
			    updated_at = NOW()
			WHERE order_pda = ?
		`, data.TakerOrderPDA)
	} else {
		// Partially filled - update remaining quantity
		_, err = l.engine.db.ExecContext(ctx, `
			UPDATE limit_order
			SET remaining_lots = remaining_lots - ?,
			    updated_at = NOW()
			WHERE order_pda = ?
		`, data.Quantity, data.TakerOrderPDA)
	}

	if err != nil {
		return fmt.Errorf("failed to update taker order: %w", err)
	}

	logx.Infof("Saved fill to database: maker=%s, taker=%s, qty=%d",
		data.MakerOrderPDA, data.TakerOrderPDA, data.Quantity)
	return nil
}

// Example WebSocket message parsing (placeholder):
//
// func parseWebSocketMessage(msg []byte) (*Event, error) {
//     // Parse Solana log messages
//     // Extract event type and data
//     // Example log format:
//     // "Program log: OrderCreated { order_pda: ..., price: ..., qty: ... }"
//
//     // Parse and create event
//     return &Event{
//         Type: EventOrderCreated,
//         Timestamp: time.Now().Unix(),
//         Data: &OrderCreatedEvent{...},
//     }, nil
// }
