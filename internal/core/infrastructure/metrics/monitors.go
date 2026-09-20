package metrics

import (
	"sync"
	"time"

	"github.com/scandrix/backend/internal/core/log"
)

// ErrorRateMonitor tracks total requests vs 5xx errors to calculate error rates.
type ErrorRateMonitor struct {
	mu           sync.RWMutex
	totalReqs    int64
	totalErrors  int64
	thresholdPct float64
	logger       *log.StructuredLogger
}

// NewErrorRateMonitor constructs an ErrorRateMonitor with an alert threshold (e.g. 5.0%).
func NewErrorRateMonitor(thresholdPct float64) *ErrorRateMonitor {
	return &ErrorRateMonitor{
		thresholdPct: thresholdPct,
		logger:       log.CreateLogger("ErrorRateMonitor"),
	}
}

// RecordRequest increments the total request counter.
func (m *ErrorRateMonitor) RecordRequest() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalReqs++
}

// RecordError increments the 5xx error counter and alerts if threshold is exceeded.
func (m *ErrorRateMonitor) RecordError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalErrors++
	m.totalReqs++

	if m.totalReqs >= 20 {
		rate := (float64(m.totalErrors) / float64(m.totalReqs)) * 100.0
		if rate >= m.thresholdPct {
			m.logger.Warn(log.LogArguments{
				Message: "HTTP error rate threshold breached",
				Context: "ErrorRateMonitor",
				Metadata: map[string]interface{}{
					"errorRatePct": rate,
					"threshold":    m.thresholdPct,
					"totalErrors":  m.totalErrors,
					"totalReqs":    m.totalReqs,
				},
			})
		}
	}
}

// CurrentErrorRate returns the current error rate percentage.
func (m *ErrorRateMonitor) CurrentErrorRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.totalReqs == 0 {
		return 0.0
	}
	return (float64(m.totalErrors) / float64(m.totalReqs)) * 100.0
}

// ReviewResponseMonitor records review response latencies and monitors SLA deadlines.
type ReviewResponseMonitor struct {
	mu         sync.RWMutex
	durations  []time.Duration
	maxLatency time.Duration
	logger     *log.StructuredLogger
}

// NewReviewResponseMonitor creates a new ReviewResponseMonitor with maximum expected duration.
func NewReviewResponseMonitor(maxLatency time.Duration) *ReviewResponseMonitor {
	return &ReviewResponseMonitor{
		durations:  make([]time.Duration, 0, 100),
		maxLatency: maxLatency,
		logger:     log.CreateLogger("ReviewResponseMonitor"),
	}
}

// RecordReviewLatency records the duration of a code review run.
func (m *ReviewResponseMonitor) RecordReviewLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = append(m.durations, d)

	if d > m.maxLatency {
		m.logger.Warn(log.LogArguments{
			Message: "Review execution exceeded SLA threshold",
			Context: "ReviewResponseMonitor",
			Metadata: map[string]interface{}{
				"durationMs":   d.Milliseconds(),
				"maxLatencyMs": m.maxLatency.Milliseconds(),
			},
		})
	}
}

// WebhookFailureMonitor monitors incoming webhook delivery failures.
type WebhookFailureMonitor struct {
	mu           sync.RWMutex
	failures     int
	threshold    int
	lastFailure  time.Time
	logger       *log.StructuredLogger
}

// NewWebhookFailureMonitor constructs a new WebhookFailureMonitor.
func NewWebhookFailureMonitor(alertThreshold int) *WebhookFailureMonitor {
	return &WebhookFailureMonitor{
		threshold: alertThreshold,
		logger:    log.CreateLogger("WebhookFailureMonitor"),
	}
}

// RecordFailure records a webhook delivery or signature failure.
func (m *WebhookFailureMonitor) RecordFailure(source string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures++
	m.lastFailure = time.Now()

	if m.failures >= m.threshold {
		m.logger.Error(log.LogArguments{
			Message: "High volume of webhook failures detected",
			Context: "WebhookFailureMonitor",
			Metadata: map[string]interface{}{
				"failures": m.failures,
				"source":   source,
				"reason":   reason,
			},
		})
	}
}

// Reset resets the failure counter (e.g. after alert handling).
func (m *WebhookFailureMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = 0
}
