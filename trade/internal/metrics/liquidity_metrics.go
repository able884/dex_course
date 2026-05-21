package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"richcode.cc/dex/trade/internal/types"
)

var (
	initOnce     sync.Once
	tokenLatency *prometheus.HistogramVec
	feeLatency   *prometheus.HistogramVec
)

type promMetrics struct{}

// NewLiquidityMetrics returns a Prometheus-backed metrics collector.
func NewLiquidityMetrics() types.LiquidityMetrics {
	initOnce.Do(func() {
		tokenLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cpmm_token_fetch_duration_seconds",
				Help:    "Duration of cpmm token fetch operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"status"},
		)
		feeLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cpmm_fee_tier_fetch_duration_seconds",
				Help:    "Duration of cpmm fee tier fetch operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"status"},
		)
		prometheus.MustRegister(tokenLatency, feeLatency)
	})
	return &promMetrics{}
}

func (p *promMetrics) RecordTokenFetch(status string, duration time.Duration) {
	if tokenLatency == nil {
		return
	}
	tokenLatency.WithLabelValues(status).Observe(duration.Seconds())
}

func (p *promMetrics) RecordFeeFetch(status string, duration time.Duration) {
	if feeLatency == nil {
		return
	}
	feeLatency.WithLabelValues(status).Observe(duration.Seconds())
}
