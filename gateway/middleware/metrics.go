package middleware

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsOnce sync.Once
	reqCounter  *prometheus.CounterVec
	reqLatency  *prometheus.HistogramVec
)

func initMetrics() {
	reqCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cpmm_gateway_requests_total",
			Help: "Total CPMM liquidity gateway requests",
		},
		[]string{"path", "status"},
	)
	reqLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cpmm_gateway_request_duration_seconds",
			Help:    "Latency of CPMM liquidity gateway requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "status"},
	)
	prometheus.MustRegister(reqCounter, reqLatency)
}

// RecordLiquidityMetric records gateway level metrics for CPMM endpoints.
func RecordLiquidityMetric(path, status string, duration time.Duration) {
	metricsOnce.Do(initMetrics)
	reqCounter.WithLabelValues(path, status).Inc()
	reqLatency.WithLabelValues(path, status).Observe(duration.Seconds())
}
