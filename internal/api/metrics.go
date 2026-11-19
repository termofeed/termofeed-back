package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	HttpRequestsTotal    *prometheus.CounterVec
	HttpRequestDuration  *prometheus.HistogramVec
	ActiveConnections    prometheus.Gauge
	ActiveRooms          prometheus.Gauge
	ReactionsTotal       prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		HttpRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "termofeed_http_requests_total",
				Help: "Total number of HTTP requests processed, partitioned by status code, method and path.",
			},
			[]string{"method", "path", "status"},
		),
		HttpRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "termofeed_http_request_duration_seconds",
				Help:    "Histogram of response latency for HTTP requests.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		ActiveConnections: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "termofeed_active_websocket_connections",
				Help: "Current number of active WebSocket connections.",
			},
		),
		ActiveRooms: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "termofeed_active_rooms",
				Help: "Current number of active rooms.",
			},
		),
		ReactionsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "termofeed_reactions_total",
				Help: "Total number of reactions submitted.",
			},
		),
	}
}
