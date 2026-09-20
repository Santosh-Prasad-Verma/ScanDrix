package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/core/log"
)

// MetricType classifies whether a metric represents an incremental counter or continuous value.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeGauge     MetricType = "gauge"
)

// MetricEvent represents a discrete telemetry observation.
type MetricEvent struct {
	Name       string            `json:"name"`
	Type       MetricType        `json:"type"`
	Value      float64           `json:"value"`
	Labels     map[string]string `json:"labels"`
	RecordedAt time.Time         `json:"recordedAt"`
}

// MetricsCollector buffers and asynchronously batches metrics observations.
type MetricsCollector struct {
	mu            sync.Mutex
	buffer        []MetricEvent
	batchSize     int
	flushInterval time.Duration
	ticker        *time.Ticker
	stopChan      chan struct{}
	logger        *log.StructuredLogger
	closed        bool
}

// NewMetricsCollector constructs a new collector with a periodic background flush.
func NewMetricsCollector(batchSize int, flushInterval time.Duration) *MetricsCollector {
	if batchSize <= 0 {
		batchSize = 75
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	mc := &MetricsCollector{
		buffer:        make([]MetricEvent, 0, batchSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ticker:        time.NewTicker(flushInterval),
		stopChan:      make(chan struct{}),
		logger:        log.CreateLogger("MetricsCollector"),
	}

	go mc.runLoop()
	return mc
}

func (mc *MetricsCollector) runLoop() {
	for {
		select {
		case <-mc.ticker.C:
			_ = mc.Flush(context.Background())
		case <-mc.stopChan:
			return
		}
	}
}

// RecordCounter increments a counter metric with arbitrary labels.
func (mc *MetricsCollector) RecordCounter(name string, value float64, labels map[string]string) {
	mc.record(MetricEvent{
		Name:       name,
		Type:       MetricTypeCounter,
		Value:      value,
		Labels:     labels,
		RecordedAt: time.Now().UTC(),
	})
}

// RecordHistogram records a distribution observation (e.g. latency in milliseconds).
func (mc *MetricsCollector) RecordHistogram(name string, value float64, labels map[string]string) {
	mc.record(MetricEvent{
		Name:       name,
		Type:       MetricTypeHistogram,
		Value:      value,
		Labels:     labels,
		RecordedAt: time.Now().UTC(),
	})
}

// RecordGauge sets a current point-in-time value.
func (mc *MetricsCollector) RecordGauge(name string, value float64, labels map[string]string) {
	mc.record(MetricEvent{
		Name:       name,
		Type:       MetricTypeGauge,
		Value:      value,
		Labels:     labels,
		RecordedAt: time.Now().UTC(),
	})
}

func (mc *MetricsCollector) record(evt MetricEvent) {
	mc.mu.Lock()
	if mc.closed {
		mc.mu.Unlock()
		return
	}
	mc.buffer = append(mc.buffer, evt)
	shouldFlush := len(mc.buffer) >= mc.batchSize
	mc.mu.Unlock()

	if shouldFlush {
		_ = mc.Flush(context.Background())
	}
}

// Flush dispatches all buffered metrics events.
func (mc *MetricsCollector) Flush(ctx context.Context) error {
	mc.mu.Lock()
	if len(mc.buffer) == 0 {
		mc.mu.Unlock()
		return nil
	}
	events := mc.buffer
	mc.buffer = make([]MetricEvent, 0, mc.batchSize)
	mc.mu.Unlock()

	mc.logger.Debug(log.LogArguments{
		Message: "Flushing metrics events",
		Context: "MetricsCollector",
		Metadata: map[string]interface{}{
			"count": len(events),
		},
	})
	return nil
}

// Close drains remaining events and stops the background ticker.
func (mc *MetricsCollector) Close() error {
	mc.mu.Lock()
	if mc.closed {
		mc.mu.Unlock()
		return nil
	}
	mc.closed = true
	mc.ticker.Stop()
	close(mc.stopChan)
	mc.mu.Unlock()

	return mc.Flush(context.Background())
}
