package observability

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MemoryExporter retains exported spans in memory, primarily for unit testing and local inspection.
type MemoryExporter struct {
	mu    sync.RWMutex
	spans []*SpanData
}

// NewMemoryExporter instantiates an in-memory exporter.
func NewMemoryExporter() *MemoryExporter {
	return &MemoryExporter{
		spans: make([]*SpanData, 0, 64),
	}
}

func (m *MemoryExporter) Export(ctx context.Context, spans []*SpanData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range spans {
		m.spans = append(m.spans, s)
	}
	return nil
}

func (m *MemoryExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (m *MemoryExporter) Shutdown(ctx context.Context) error {
	return nil
}

// GetSpans returns a defensive copy of all captured spans.
func (m *MemoryExporter) GetSpans() []*SpanData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*SpanData, len(m.spans))
	copy(out, m.spans)
	return out
}

// Clear resets the accumulated spans.
func (m *MemoryExporter) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spans = m.spans[:0]
}

// OTLPExporter sends spans to an OpenTelemetry Collector HTTP endpoint (e.g., /v1/traces).
type OTLPExporter struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

// OTLPConfig configures the OTLPExporter.
type OTLPConfig struct {
	Endpoint string
	Headers  map[string]string
	Timeout  time.Duration
}

// NewOTLPExporter instantiates an OTLP HTTP exporter.
func NewOTLPExporter(cfg OTLPConfig) *OTLPExporter {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:4318/v1/traces"
	}
	return &OTLPExporter{
		endpoint: cfg.Endpoint,
		headers:  cfg.Headers,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (e *OTLPExporter) Export(ctx context.Context, spans []*SpanData) error {
	if len(spans) == 0 {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"resourceSpans": []map[string]any{
			{
				"scopeSpans": []map[string]any{
					{
						"spans": spans,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal otlp spans: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create otlp http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp http post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("otlp collector returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

func (e *OTLPExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	return nil
}

// PostgresTelemetryExporter persists span telemetry into Supabase PostgreSQL JSONB tables.
type PostgresTelemetryExporter struct {
	db *sql.DB
}

// NewPostgresTelemetryExporter creates a Postgres JSONB telemetry exporter.
func NewPostgresTelemetryExporter(db *sql.DB) *PostgresTelemetryExporter {
	return &PostgresTelemetryExporter{db: db}
}

func (p *PostgresTelemetryExporter) Export(ctx context.Context, spans []*SpanData) error {
	if p.db == nil || len(spans) == 0 {
		return nil
	}

	const query = `
		INSERT INTO telemetry_spans (
			trace_id, span_id, parent_span_id, name, kind,
			start_time, end_time, duration_ms, status_code,
			status_description, attributes, events, resource
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) ON CONFLICT (trace_id, span_id) DO NOTHING;
	`

	for _, s := range spans {
		attrsJSON, _ := json.Marshal(s.Attributes)
		eventsJSON, _ := json.Marshal(s.Events)
		resJSON, _ := json.Marshal(s.Resource)

		_, err := p.db.ExecContext(
			ctx, query,
			s.Context.TraceID,
			s.Context.SpanID,
			s.ParentSpanID,
			s.Name,
			string(s.Kind),
			s.StartTime,
			s.EndTime,
			s.DurationMs,
			string(s.Status.Code),
			s.Status.Description,
			attrsJSON,
			eventsJSON,
			resJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to persist span %s: %w", s.Context.SpanID, err)
		}
	}

	return nil
}

func (p *PostgresTelemetryExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (p *PostgresTelemetryExporter) Shutdown(ctx context.Context) error {
	return nil
}

// BatchSpanProcessor implements SpanProcessor with buffering and background periodic flushing.
type BatchSpanProcessor struct {
	exporter      SpanExporter
	queue         chan *SpanData
	maxBatchSize  int
	flushInterval time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
	mu            sync.Mutex
	stopped       bool
}

// BatchProcessorConfig configures the BatchSpanProcessor.
type BatchProcessorConfig struct {
	MaxQueueSize  int
	MaxBatchSize  int
	FlushInterval time.Duration
}

// NewBatchSpanProcessor instantiates a concurrent background batch span processor.
func NewBatchSpanProcessor(exporter SpanExporter, cfg ...func(*BatchProcessorConfig)) *BatchSpanProcessor {
	c := BatchProcessorConfig{
		MaxQueueSize:  2048,
		MaxBatchSize:  512,
		FlushInterval: 3 * time.Second,
	}
	for _, fn := range cfg {
		fn(&c)
	}

	b := &BatchSpanProcessor{
		exporter:      exporter,
		queue:         make(chan *SpanData, c.MaxQueueSize),
		maxBatchSize:  c.MaxBatchSize,
		flushInterval: c.FlushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	go b.worker()
	return b
}

func (b *BatchSpanProcessor) OnStart(parent context.Context, span Span) {}

func (b *BatchSpanProcessor) OnEnd(span *SpanData) {
	if span == nil {
		return
	}
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	select {
	case b.queue <- span:
	default:
		// Queue full, drop span to prevent memory bloat
	}
}

func (b *BatchSpanProcessor) worker() {
	defer close(b.doneCh)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	batch := make([]*SpanData, 0, b.maxBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		toExport := make([]*SpanData, len(batch))
		copy(toExport, batch)
		batch = batch[:0]

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = b.exporter.Export(ctx, toExport)
		cancel()
	}

	for {
		select {
		case s := <-b.queue:
			batch = append(batch, s)
			if len(batch) >= b.maxBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stopCh:
			// Drain remaining in queue
			for {
				select {
				case s := <-b.queue:
					batch = append(batch, s)
					if len(batch) >= b.maxBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (b *BatchSpanProcessor) ForceFlush(ctx context.Context) error {
	return b.exporter.ForceFlush(ctx)
}

func (b *BatchSpanProcessor) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return nil
	}
	b.stopped = true
	close(b.stopCh)
	b.mu.Unlock()

	select {
	case <-b.doneCh:
		return b.exporter.Shutdown(ctx)
	case <-ctx.Done():
		return errors.New("timeout waiting for batch processor shutdown")
	}
}
