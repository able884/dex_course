package block

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/trade/trade"
)

const defaultCpmmConfigIndex = int32(2) // 0.50% fee tier by default

// PumpMigrationWorker listens to migration jobs and calls trade RPC to build Raydium CPMM pools.
type PumpMigrationWorker struct {
	sc   *svc.ServiceContext
	stop chan struct{}
}

func NewPumpMigrationWorker(sc *svc.ServiceContext) *PumpMigrationWorker {
	return &PumpMigrationWorker{
		sc:   sc,
		stop: make(chan struct{}),
	}
}

func (w *PumpMigrationWorker) Start() {
	logger := logx.WithContext(context.Background()).WithFields(logx.Field("service", "pump-migration"))
	if w.sc == nil || w.sc.PumpMigrationChan == nil {
		logger.Info("pump migration worker disabled: missing service context or channel")
		return
	}
	logger.Infof("pump migration worker started")
	for {
		select {
		case job := <-w.sc.PumpMigrationChan:
			w.handle(job, logger)
		case <-w.stop:
			logger.Infof("pump migration worker stopped")
			return
		}
	}
}

func (w *PumpMigrationWorker) Stop() {
	select {
	case <-w.stop:
		return
	default:
		close(w.stop)
	}
}

func (w *PumpMigrationWorker) handle(job svc.PumpMigrationJob, logger logx.Logger) {
	wallet := w.sc.Config.Consumer.MigrationWallet
	if wallet == "" {
		logger.Errorf("skip migration for pair %s: missing migration wallet (configure Consumer.MigrationWallet)", job.PairAddr)
		return
	}

	baseAmt := decimal.NewFromFloat(job.BaseAmount)
	tokenAmt := decimal.NewFromFloat(job.TokenAmount)
	if baseAmt.Sign() <= 0 {
		baseAmt = decimal.NewFromFloat(0.01)
	}
	if tokenAmt.Sign() <= 0 {
		tokenAmt = decimal.NewFromFloat(0.01)
	}
	price := decimal.NewFromFloat(1)
	if tokenAmt.Sign() > 0 {
		price = baseAmt.Div(tokenAmt)
	}

	cfgIdx := w.sc.Config.Consumer.MigrationConfigIndex
	if cfgIdx < 0 {
		cfgIdx = defaultCpmmConfigIndex
	}

	logger.Infof("trigger migration: pair=%s token=%s pump_point=%.4f base=%.6f token=%.6f wallet=%s target=%s",
		job.PairAddr, job.TokenMint, job.PumpPoint, job.BaseAmount, job.TokenAmount, wallet, w.sc.Config.Consumer.MigrationTarget)

	switch w.sc.Config.Consumer.MigrationTarget {
	case "raydium_cpmm", "":
		if w.sc.TradeService == nil {
			logger.Errorf("skip migration for pair %s: trade service not configured", job.PairAddr)
			return
		}
		req := &trade.CreateCpmmPoolRequest{
			ChainId:           int32(w.sc.Config.Sol.ChainId),
			PoolType:          "cpmm",
			BaseTokenMint:     constants.TokenStrWrapSol,
			QuoteTokenMint:    job.TokenMint,
			BaseAmount:        baseAmt.String(),
			QuoteAmount:       tokenAmt.String(),
			InitialPrice:      price.String(),
			ConfigIndex:       cfgIdx,
			StartTime:         time.Now().Unix(),
			UserWalletAddress: wallet,
		}
		if _, err := w.sc.TradeService.CreateCpmmPool(context.Background(), req); err != nil {
			logger.Errorf("create cpmm pool failed for pair %s: %v", job.PairAddr, err)
		}
	case "pump_amm":
		logger.Errorf("pump_amm migration not implemented yet, pair=%s token=%s", job.PairAddr, job.TokenMint)
	default:
		logger.Errorf("unsupported migration target %s, pair=%s", w.sc.Config.Consumer.MigrationTarget, job.PairAddr)
	}
}
