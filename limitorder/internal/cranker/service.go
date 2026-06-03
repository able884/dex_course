package cranker

import (
	"context"
	"fmt"
	"sync"
	"time"

	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// Service is the order cleanup/cranking service
type Service struct {
	mu sync.RWMutex

	// Database connection
	db sqlx.SqlConn

	// Scanner for finding expired orders
	scanner *Scanner

	// Executor for on-chain sweep
	executor *Executor

	// Solana RPC client (for querying current slot)
	rpcClient *ag_rpc.Client

	// Stop channel
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Configuration
	config ServiceConfig

	// Statistics
	stats Stats
}

// ServiceConfig holds service configuration
type ServiceConfig struct {
	ScanInterval      time.Duration // How often to scan for expired orders
	BatchSize         int           // How many orders to process per batch
	ExpiredOrderLimit int           // Max orders to process per scan
	Enable            bool          // Enable/disable service
	SignerPrivateKey  string        // Base58 private key for sweep_expired signer
	ProgramID         string        // Program ID for limit order program
}

// Stats holds service statistics
type Stats struct {
	mu sync.RWMutex

	TotalScans       int64 // Total number of scans performed
	TotalExpired     int64 // Total expired orders found
	TotalCleaned     int64 // Total orders successfully cleaned
	TotalFailed      int64 // Total orders that failed to clean
	LastScanTime     time.Time
	LastCleanedCount int
}

// ExpiredOrder represents an expired order to be cleaned
type ExpiredOrder struct {
	OrderPDA      string
	OrderID       int64
	MarketPDA     string
	Owner         string
	Side          int
	PriceLots     int64
	RemainingLots int64
	ExpirySlot    int64
	CreatedAt     time.Time
}

// NewService creates a new cranker service
func NewService(db sqlx.SqlConn, config ServiceConfig, rpcClient *ag_rpc.Client) *Service {
	scanner := NewScanner(db, ScannerConfig{
		BatchSize:         config.BatchSize,
		ExpiredOrderLimit: config.ExpiredOrderLimit,
	})

	var executor *Executor
	if rpcClient != nil && config.SignerPrivateKey != "" && config.ProgramID != "" {
		var err error
		executor, err = NewExecutor(rpcClient, config.SignerPrivateKey, config.ProgramID)
		if err != nil {
			logx.Errorf("Failed to create cranker executor: %v", err)
		}
	} else {
		logx.Infof("Cranker signer not configured, sweep_expired will be disabled")
	}

	return &Service{
		db:        db,
		scanner:   scanner,
		executor:  executor,
		rpcClient: rpcClient,
		stopChan:  make(chan struct{}),
		config:    config,
	}
}

// Start starts the cranker service
func (s *Service) Start(ctx context.Context) error {
	if !s.config.Enable {
		logx.Info("Cranker service is disabled")
		return nil
	}

	logx.Info("Starting cranker service...")

	// Start periodic scanning
	s.wg.Add(1)
	go s.periodicScanner(ctx)

	logx.Infof("Cranker service started (scan interval: %v)", s.config.ScanInterval)
	return nil
}

// Stop stops the cranker service
func (s *Service) Stop() {
	if !s.config.Enable {
		return
	}

	logx.Info("Stopping cranker service...")
	close(s.stopChan)
	s.wg.Wait()
	logx.Info("Cranker service stopped")
}

// periodicScanner performs periodic scanning for expired orders
func (s *Service) periodicScanner(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ScanInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.scanAndClean(ctx)

	for {
		select {
		case <-s.stopChan:
			return

		case <-ticker.C:
			s.scanAndClean(ctx)
		}
	}
}

// scanAndClean scans for expired orders and cleans them
func (s *Service) scanAndClean(ctx context.Context) {
	startTime := time.Now()

	// Update stats
	s.stats.mu.Lock()
	s.stats.TotalScans++
	s.stats.LastScanTime = startTime
	s.stats.mu.Unlock()

	// Get current slot (in production, query from Solana)
	currentSlot := s.getCurrentSlot(ctx)

	// Scan for expired orders
	expiredOrders, err := s.scanner.ScanExpiredOrders(ctx, currentSlot)
	if err != nil {
		logx.Errorf("Failed to scan expired orders: %v", err)
		return
	}

	if len(expiredOrders) == 0 {
		logx.Infof("No expired orders found")
		return
	}

	logx.Infof("Found %d expired orders", len(expiredOrders))

	// Update stats
	s.stats.mu.Lock()
	s.stats.TotalExpired += int64(len(expiredOrders))
	s.stats.mu.Unlock()

	// Clean orders in batches
	cleaned := 0
	failed := 0

	for i := 0; i < len(expiredOrders); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(expiredOrders) {
			end = len(expiredOrders)
		}

		batch := expiredOrders[i:end]
		c, f := s.cleanBatch(ctx, batch)
		cleaned += c
		failed += f
	}

	// Update stats
	s.stats.mu.Lock()
	s.stats.TotalCleaned += int64(cleaned)
	s.stats.TotalFailed += int64(failed)
	s.stats.LastCleanedCount = cleaned
	s.stats.mu.Unlock()

	duration := time.Since(startTime)
	logx.Infof("Scan completed: cleaned=%d, failed=%d, duration=%v",
		cleaned, failed, duration)
}

// cleanBatch cleans a batch of expired orders
func (s *Service) cleanBatch(ctx context.Context, orders []*ExpiredOrder) (cleaned, failed int) {
	for _, order := range orders {
		success := s.cleanOrder(ctx, order)
		if success {
			cleaned++
		} else {
			failed++
		}
	}

	return
}

// cleanOrder cleans a single expired order
func (s *Service) cleanOrder(ctx context.Context, order *ExpiredOrder) bool {
	logx.Infof("Cleaning expired order: %s (expired at slot %d)",
		order.OrderPDA, order.ExpirySlot)

	// Step 1: Cancel order on-chain
	txHash, err := s.cancelOrderOnChain(ctx, order)
	if err != nil {
		logx.Errorf("Failed to cancel order on-chain: %s, error: %v",
			order.OrderPDA, err)
		return false
	}

	logx.Infof("Order cancelled on-chain: %s, tx: %s", order.OrderPDA, txHash)

	// Step 2: Update database
	err = s.updateOrderStatus(ctx, order.OrderPDA, txHash)
	if err != nil {
		logx.Errorf("Failed to update order status in database: %s, error: %v",
			order.OrderPDA, err)
		// Even if DB update fails, consider it cleaned since on-chain succeeded
		return true
	}

	logx.Infof("Order cleaned successfully: %s", order.OrderPDA)
	return true
}

// cancelOrderOnChain cancels an expired order on Solana blockchain using sweep_expired instruction
func (s *Service) cancelOrderOnChain(ctx context.Context, order *ExpiredOrder) (string, error) {
	if s.executor == nil {
		return "", fmt.Errorf("cranker executor not initialized")
	}

	return s.executor.SweepExpired(ctx, order)
}

// updateOrderStatus updates order status in database
func (s *Service) updateOrderStatus(ctx context.Context, orderPDA string, txHash string) error {
	// Update limit_order table
	// Status: 4 = Expired
	query := `
		UPDATE limit_order
		SET status = 4,
		    expired_at = NOW(),
		    cancel_tx_hash = ?,
		    updated_at = NOW()
		WHERE order_pda = ?
	`

	_, err := s.db.ExecCtx(ctx, query, txHash, orderPDA)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	logx.Infof("Updated order status in database: %s", orderPDA)
	return nil
}

// getCurrentSlot gets current Solana slot number
func (s *Service) getCurrentSlot(ctx context.Context) int64 {
	// If no RPC client configured, use mock slot based on time
	if s.rpcClient == nil {
		slot := time.Now().UnixNano() / int64(400*time.Millisecond)
		return slot
	}

	// Query current slot from Solana RPC
	// Using finalized commitment for safety
	slot, err := s.rpcClient.GetSlot(ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		logx.Errorf("Failed to get current slot from RPC: %v, using time-based estimate", err)
		slotFallback := time.Now().UnixNano() / int64(400*time.Millisecond)
		return slotFallback
	}

	return int64(slot)
}

// GetStats returns service statistics
func (s *Service) GetStats() Stats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	return Stats{
		TotalScans:       s.stats.TotalScans,
		TotalExpired:     s.stats.TotalExpired,
		TotalCleaned:     s.stats.TotalCleaned,
		TotalFailed:      s.stats.TotalFailed,
		LastScanTime:     s.stats.LastScanTime,
		LastCleanedCount: s.stats.LastCleanedCount,
	}
}

// CleanNow forces an immediate scan and clean
func (s *Service) CleanNow(ctx context.Context) {
	logx.Info("Manual clean triggered")
	s.scanAndClean(ctx)
}

// Example usage:
// service := cranker.NewService(db, cranker.ServiceConfig{
//     ScanInterval:      60 * time.Second,
//     BatchSize:         50,
//     ExpiredOrderLimit: 100,
//     Enable:            true,
// })
// service.Start(ctx)
// defer service.Stop()
