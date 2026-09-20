package telemetry

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered Prometheus instrumentation collectors.
type Metrics struct {
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	ReviewsProcessedTotal *prometheus.CounterVec
	ReviewDurationSeconds *prometheus.HistogramVec
	ActiveWorkerCount     prometheus.Gauge
	FindingsDiscovered    *prometheus.CounterVec
	AuthAttemptsTotal     *prometheus.CounterVec
}

var globalMetrics *Metrics

// InitMetrics initializes and registers production Prometheus metrics collectors.
func InitMetrics() *Metrics {
	if globalMetrics != nil {
		return globalMetrics
	}

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "scandrix_http_requests_total",
				Help: "Total number of HTTP requests received",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "scandrix_http_request_duration_seconds",
				Help:    "Latency histogram of HTTP requests",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"method", "path"},
		),
		ReviewsProcessedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "scandrix_reviews_processed_total",
				Help: "Total count of pull request reviews processed by the engine",
			},
			[]string{"status", "provider"},
		),
		ReviewDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "scandrix_review_duration_seconds",
				Help:    "Execution time of complete review lifecycle in seconds",
				Buckets: []float64{1.0, 2.5, 5.0, 10.0, 20.0, 30.0, 60.0, 120.0},
			},
			[]string{"provider"},
		),
		ActiveWorkerCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "scandrix_active_workers",
				Help: "Current number of worker goroutines executing reviews",
			},
		),
		FindingsDiscovered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "scandrix_findings_discovered_total",
				Help: "Count of security, bug, and quality findings flagged",
			},
			[]string{"severity", "category"},
		),
		AuthAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "scandrix_auth_attempts_total",
				Help: "Total count of authentication attempts by status and failure reason",
			},
			[]string{"status", "reason"},
		),
	}

	prometheus.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.ReviewsProcessedTotal,
		m.ReviewDurationSeconds,
		m.ActiveWorkerCount,
		m.FindingsDiscovered,
		m.AuthAttemptsTotal,
	)

	globalMetrics = m
	return m
}

// GetMetrics returns the global singleton metrics instance.
func GetMetrics() *Metrics {
	if globalMetrics == nil {
		return InitMetrics()
	}
	return globalMetrics
}

// Handler returns the HTTP handler for Prometheus scraping (/metrics).
func Handler() http.Handler {
	return promhttp.Handler()
}

// MeasureHTTP is a Chi middleware that records request latency and status codes.
func MeasureHTTP(next http.Handler) http.Handler {
	m := GetMetrics()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriterDelegator{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		statusStr := http.StatusText(rw.statusCode)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

type responseWriterDelegator struct {
	http.ResponseWriter
	statusCode int
}

func (d *responseWriterDelegator) WriteHeader(code int) {
	d.statusCode = code
	d.ResponseWriter.WriteHeader(code)
}
