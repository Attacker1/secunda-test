// Package metrics регистрирует и экспонирует метрики Prometheus.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	RequestsTotal *prometheus.CounterVec
	ErrorsTotal   *prometheus.CounterVec
	Duration      *prometheus.HistogramVec
}

func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		RequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path", "status"}),
		ErrorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total number of HTTP responses with status >= 500.",
		}, []string{"method", "path"}),
		Duration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
	}
}
