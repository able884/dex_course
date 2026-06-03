package cranker

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// Scanner scans database for expired orders
type Scanner struct {
	db     sqlx.SqlConn
	config ScannerConfig
}

// ScannerConfig holds scanner configuration
type ScannerConfig struct {
	BatchSize         int // Orders per batch
	ExpiredOrderLimit int // Max orders per scan
}

// NewScanner creates a new scanner
func NewScanner(db sqlx.SqlConn, config ScannerConfig) *Scanner {
	return &Scanner{
		db:     db,
		config: config,
	}
}

// ScanExpiredOrders scans for expired orders
func (s *Scanner) ScanExpiredOrders(ctx context.Context, currentSlot int64) ([]*ExpiredOrder, error) {
	// Query active orders that have expired
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE status = 1
		  AND expiry_slot > 0
		  AND expiry_slot < ?
		ORDER BY expiry_slot ASC, created_at ASC
		LIMIT ?
	`

	var orders []*ExpiredOrder

	err := s.db.QueryRowsCtx(ctx, &orders, query, currentSlot, s.config.ExpiredOrderLimit)
	if err != nil {
		return nil, fmt.Errorf("query expired orders failed: %w", err)
	}

	logx.Infof("Scanned for expired orders (current slot: %d, limit: %d), found: %d",
		currentSlot, s.config.ExpiredOrderLimit, len(orders))

	return orders, nil
}

// ScanExpiredOrdersByMarket scans for expired orders in a specific market
func (s *Scanner) ScanExpiredOrdersByMarket(ctx context.Context, marketPDA string, currentSlot int64) ([]*ExpiredOrder, error) {
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE status = 1
		  AND expiry_slot > 0
		  AND expiry_slot < ?
		  AND market_pda = ?
		ORDER BY expiry_slot ASC, created_at ASC
		LIMIT ?
	`

	var orders []*ExpiredOrder

	err := s.db.QueryRowsCtx(ctx, &orders, query, currentSlot, marketPDA, s.config.ExpiredOrderLimit)
	if err != nil {
		return nil, fmt.Errorf("query expired orders by market failed: %w", err)
	}

	logx.Infof("Scanned for expired orders in market %s (current slot: %d), found: %d",
		marketPDA, currentSlot, len(orders))

	return orders, nil
}

// ScanStaleOrders scans for orders that haven't been updated in a long time
// This can help detect orders that are stuck or have sync issues
func (s *Scanner) ScanStaleOrders(ctx context.Context, staleDuration int64) ([]*ExpiredOrder, error) {
	// Find active orders that haven't been updated for staleDuration seconds
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE status = 1
		  AND UNIX_TIMESTAMP(updated_at) < UNIX_TIMESTAMP() - ?
		ORDER BY updated_at ASC
		LIMIT ?
	`

	var orders []*ExpiredOrder

	err := s.db.QueryRowsCtx(ctx, &orders, query, staleDuration, s.config.ExpiredOrderLimit)
	if err != nil {
		return nil, fmt.Errorf("query stale orders failed: %w", err)
	}

	logx.Infof("Scanned for stale orders (stale duration: %d seconds), found: %d",
		staleDuration, len(orders))

	return orders, nil
}

// CountExpiredOrders returns the count of expired orders
func (s *Scanner) CountExpiredOrders(ctx context.Context, currentSlot int64) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM limit_order
		WHERE status = 1
		  AND expiry_slot > 0
		  AND expiry_slot < ?
	`

	var count int64

	err := s.db.QueryRowCtx(ctx, &count, query, currentSlot)
	if err != nil {
		return 0, fmt.Errorf("count expired orders failed: %w", err)
	}

	return count, nil
}

// GetOrdersExpiringInRange returns orders expiring within a slot range
func (s *Scanner) GetOrdersExpiringInRange(ctx context.Context, startSlot, endSlot int64) ([]*ExpiredOrder, error) {
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE status = 1
		  AND expiry_slot >= ?
		  AND expiry_slot < ?
		ORDER BY expiry_slot ASC, created_at ASC
		LIMIT ?
	`

	var orders []*ExpiredOrder

	err := s.db.QueryRowsCtx(ctx, &orders, query, startSlot, endSlot, s.config.ExpiredOrderLimit)
	if err != nil {
		return nil, fmt.Errorf("query orders expiring in range failed: %w", err)
	}

	logx.Infof("Scanned for orders expiring between slot %d and %d, found: %d",
		startSlot, endSlot, len(orders))

	return orders, nil
}

// ScanOrdersByOwner scans for expired orders by owner
func (s *Scanner) ScanOrdersByOwner(ctx context.Context, owner string, currentSlot int64) ([]*ExpiredOrder, error) {
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE status = 1
		  AND expiry_slot > 0
		  AND expiry_slot < ?
		  AND owner = ?
		ORDER BY expiry_slot ASC, created_at ASC
		LIMIT ?
	`

	var orders []*ExpiredOrder

	err := s.db.QueryRowsCtx(ctx, &orders, query, currentSlot, owner, s.config.ExpiredOrderLimit)
	if err != nil {
		return nil, fmt.Errorf("query expired orders by owner failed: %w", err)
	}

	logx.Infof("Scanned for expired orders by owner %s, found: %d", owner, len(orders))

	return orders, nil
}

// GetExpiredOrderDetails gets detailed information about an expired order
func (s *Scanner) GetExpiredOrderDetails(ctx context.Context, orderPDA string) (*ExpiredOrder, error) {
	query := `
		SELECT
			order_pda,
			order_id,
			market_pda,
			owner,
			side,
			price_lots,
			remaining_lots,
			expiry_slot,
			created_at
		FROM limit_order
		WHERE order_pda = ?
	`

	var order ExpiredOrder

	err := s.db.QueryRowCtx(ctx, &order, query, orderPDA)
	if err != nil {
		return nil, fmt.Errorf("query order details failed: %w", err)
	}

	return &order, nil
}

// ValidateOrderExpiry checks if an order should be expired
func (s *Scanner) ValidateOrderExpiry(order *ExpiredOrder, currentSlot int64) bool {
	// No expiry set (expiry_slot = 0 means never expires)
	if order.ExpirySlot == 0 {
		return false
	}

	// Order has expired
	if order.ExpirySlot < currentSlot {
		return true
	}

	return false
}

// BuildOrderBatches splits orders into batches for processing
func (s *Scanner) BuildOrderBatches(orders []*ExpiredOrder) [][]*ExpiredOrder {
	batches := [][]*ExpiredOrder{}

	for i := 0; i < len(orders); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(orders) {
			end = len(orders)
		}
		batches = append(batches, orders[i:end])
	}

	return batches
}

// Example queries for production implementation:
//
// 1. Query with proper struct mapping:
// type dbExpiredOrder struct {
//     OrderPDA      string    `db:"order_pda"`
//     OrderID       int64     `db:"order_id"`
//     MarketPDA     string    `db:"market_pda"`
//     Owner         string    `db:"owner"`
//     Side          int       `db:"side"`
//     PriceLots     int64     `db:"price_lots"`
//     RemainingLots int64     `db:"remaining_lots"`
//     ExpirySlot    int64     `db:"expiry_slot"`
//     CreatedAt     time.Time `db:"created_at"`
// }
//
// var dbOrders []dbExpiredOrder
// err := s.db.QueryRowsCtx(ctx, &dbOrders, query, params...)
//
// 2. Convert to domain model:
// orders := make([]*ExpiredOrder, len(dbOrders))
// for i, dbOrder := range dbOrders {
//     orders[i] = &ExpiredOrder{
//         OrderPDA:      dbOrder.OrderPDA,
//         OrderID:       dbOrder.OrderID,
//         MarketPDA:     dbOrder.MarketPDA,
//         Owner:         dbOrder.Owner,
//         Side:          dbOrder.Side,
//         PriceLots:     dbOrder.PriceLots,
//         RemainingLots: dbOrder.RemainingLots,
//         ExpirySlot:    dbOrder.ExpirySlot,
//         CreatedAt:     dbOrder.CreatedAt,
//     }
// }
