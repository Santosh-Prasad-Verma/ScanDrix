package enterprise

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type traceCtxKey struct{}

// Tracer manages distributed traces and W3C context propagation.
type Tracer struct {
	mu    sync.RWMutex
	spans []*Span
}

// NewTracer initializes the tracer.
func NewTracer() *Tracer {
	return &Tracer{
		spans: make([]*Span, 0),
	}
}

// ExtractW3C parses a standard W3C traceparent header.
// Format: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
func ExtractW3C(traceparent string) (*TraceContext, error) {
	parts := strings.Split(strings.TrimSpace(traceparent), "-")
	if len(parts) != 4 {
		return nil, errors.New("invalid w3c traceparent header format")
	}

	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if version != "00" {
		return nil, fmt.Errorf("unsupported traceparent version: %s", version)
	}
	if len(traceID) != 32 || len(spanID) != 16 || len(flags) != 2 {
		return nil, errors.New("malformed traceparent lengths")
	}

	return &TraceContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
	}, nil
}

// FormatW3C constructs the traceparent header string.
func FormatW3C(tc *TraceContext) string {
	flags := tc.TraceFlags
	if flags == "" {
		flags = "01" // default sampled
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.SpanID, flags)
}

// StartSpan creates an execution span, inheriting from incoming W3C context if present.
func (t *Tracer) StartSpan(ctx context.Context, name string, attrs map[string]any) (context.Context, *Span) {
	var traceID, parentSpanID string

	if parent, ok := ctx.Value(traceCtxKey{}).(*TraceContext); ok && parent != nil {
		traceID = parent.TraceID
		parentSpanID = parent.SpanID
	} else {
		traceID = randomHex(16) // 32 hex chars
	}

	spanID := randomHex(8) // 16 hex chars
	if attrs == nil {
		attrs = make(map[string]any)
	}

	span := &Span{
		ID:           spanID,
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		Name:         name,
		StartTime:    time.Now().UTC(),
		Attributes:   attrs,
		Status:       "OK",
	}

	currentTC := &TraceContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: "01",
	}

	newCtx := context.WithValue(ctx, traceCtxKey{}, currentTC)

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return newCtx, span
}

// EndSpan finalizes span execution.
func (t *Tracer) EndSpan(span *Span, status, errMsg string) {
	span.EndTime = time.Now().UTC()
	if status != "" {
		span.Status = status
	}
	if errMsg != "" {
		span.ErrorMessage = errMsg
		span.Status = "ERROR"
	}
}

// InjectHTTP writes the active trace context into outbound HTTP request headers.
func InjectHTTP(ctx context.Context, req *http.Request) {
	if tc, ok := ctx.Value(traceCtxKey{}).(*TraceContext); ok && tc != nil {
		req.Header.Set("traceparent", FormatW3C(tc))
		if tc.TraceState != "" {
			req.Header.Set("tracestate", tc.TraceState)
		}
	}
}

// GetRecordedSpans returns all completed spans for assertion or exporting.
func (t *Tracer) GetRecordedSpans() []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()
	res := make([]*Span, len(t.spans))
	copy(res, t.spans)
	return res
}

func randomHex(bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
