package observability

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	globalTracerMu sync.RWMutex
	globalTracer   Tracer = NewTracer("scandrix.default")
)

// SetGlobalTracer configures the default global tracer.
func SetGlobalTracer(t Tracer) {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()
	if t != nil {
		globalTracer = t
	}
}

// GetGlobalTracer retrieves the active global tracer.
func GetGlobalTracer() Tracer {
	globalTracerMu.RLock()
	defer globalTracerMu.RUnlock()
	return globalTracer
}

// SimpleTracer implements Tracer with processor fanout and active span lifecycle management.
type SimpleTracer struct {
	name           string
	version        string
	resource       map[string]string
	processors     []SpanProcessor
	activeSpans    map[string]Span
	maxActiveSpans int
	mu             sync.RWMutex
}

// TracerConfig configures a SimpleTracer instance.
type TracerConfig struct {
	Version        string
	Resource       map[string]string
	Processors     []SpanProcessor
	MaxActiveSpans int
}

// NewTracer instantiates a SimpleTracer.
func NewTracer(name string, opts ...func(*TracerConfig)) *SimpleTracer {
	cfg := TracerConfig{
		Version:        "1.0.0",
		MaxActiveSpans: 10000,
		Resource: map[string]string{
			"service.name":        "scandrix-core",
			"service.version":     "1.0.0",
			"telemetry.sdk.name":  "scandrix-go",
			"telemetry.sdk.lang":  "go",
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &SimpleTracer{
		name:           name,
		version:        cfg.Version,
		resource:       cfg.Resource,
		processors:     cfg.Processors,
		activeSpans:    make(map[string]Span),
		maxActiveSpans: cfg.MaxActiveSpans,
	}
}

// AddProcessor registers an additional SpanProcessor.
func (t *SimpleTracer) AddProcessor(p SpanProcessor) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p != nil {
		t.processors = append(t.processors, p)
	}
}

// Start creates and activates a new child span.
func (t *SimpleTracer) Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	if name == "" {
		name = "unnamed_operation"
	}

	var opt SpanOptions
	for _, fn := range opts {
		fn(&opt)
	}

	// Resolve parent span context
	var traceID string
	var parentSpanID string
	var traceFlags byte = 0x01 // Sampled by default

	if opt.Parent != nil && opt.Parent.IsValid() {
		traceID = opt.Parent.TraceID
		parentSpanID = opt.Parent.SpanID
		traceFlags = opt.Parent.TraceFlags
	} else if parentSC, ok := SpanContextFromContext(ctx); ok && parentSC.IsValid() {
		traceID = parentSC.TraceID
		parentSpanID = parentSC.SpanID
		traceFlags = parentSC.TraceFlags
	} else {
		traceID = GenerateTraceID()
	}

	spanID := GenerateSpanID()
	sc := SpanContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: traceFlags,
		Remote:     false,
	}

	// Enforce active span memory bound
	t.mu.Lock()
	if len(t.activeSpans) >= t.maxActiveSpans {
		t.pruneCompletedLocked()
	}
	t.mu.Unlock()

	span := newSpan(sc, parentSpanID, name, opt, t.resource, func(data *SpanData) {
		t.removeSpan(data.Context.SpanID)
		t.dispatchOnEnd(data)
	})

	t.mu.Lock()
	t.activeSpans[spanID] = span
	t.mu.Unlock()

	// Notify processors OnStart
	t.dispatchOnStart(ctx, span)

	// Propagate in context
	childCtx := ContextWithSpan(ctx, span)
	return childCtx, span
}

func (t *SimpleTracer) removeSpan(spanID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.activeSpans, spanID)
}

func (t *SimpleTracer) pruneCompletedLocked() {
	for id, s := range t.activeSpans {
		if !s.IsRecording() {
			delete(t.activeSpans, id)
		}
	}
}

func (t *SimpleTracer) dispatchOnStart(ctx context.Context, s Span) {
	t.mu.RLock()
	processors := make([]SpanProcessor, len(t.processors))
	copy(processors, t.processors)
	t.mu.RUnlock()

	for _, p := range processors {
		p.OnStart(ctx, s)
	}
}

func (t *SimpleTracer) dispatchOnEnd(data *SpanData) {
	t.mu.RLock()
	processors := make([]SpanProcessor, len(t.processors))
	copy(processors, t.processors)
	t.mu.RUnlock()

	for _, p := range processors {
		p.OnEnd(data)
	}
}

// ActiveSpanCount returns the number of in-flight spans.
func (t *SimpleTracer) ActiveSpanCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.activeSpans)
}

// ForceFlush flushes all registered processors.
func (t *SimpleTracer) ForceFlush(ctx context.Context) error {
	t.mu.RLock()
	processors := make([]SpanProcessor, len(t.processors))
	copy(processors, t.processors)
	t.mu.RUnlock()

	var errs []error
	for _, p := range processors {
		if err := p.ForceFlush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("tracer flush encountered %d errors: %v", len(errs), errs[0])
	}
	return nil
}

// Shutdown gracefully terminates all processors.
func (t *SimpleTracer) Shutdown(ctx context.Context) error {
	t.mu.RLock()
	processors := make([]SpanProcessor, len(t.processors))
	copy(processors, t.processors)
	t.mu.RUnlock()

	var errs []error
	for _, p := range processors {
		if err := p.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
