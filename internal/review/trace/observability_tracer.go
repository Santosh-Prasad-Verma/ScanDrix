// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SpanStatus categorizes the final execution status of an observability span.
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "OK"
	SpanStatusError SpanStatus = "ERROR"
)

// TokenUsage tracks prompt, completion, and cache tokens consumed during LLM calls.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SpanEvent records an instantaneous event or log within a span lifecycle.
type SpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// ObservabilitySpan represents an individual trace span in the review pipeline.
type ObservabilitySpan struct {
	SpanID        string                 `json:"span_id"`
	TraceID       string                 `json:"trace_id"`
	ParentSpanID  string                 `json:"parent_span_id,omitempty"`
	Name          string                 `json:"name"`
	SessionID     string                 `json:"session_id"`
	OrgID         string                 `json:"org_id"`
	RepoID        string                 `json:"repo_id"`
	PRNumber      int                    `json:"pr_number"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration_ms"`
	Status        SpanStatus             `json:"status"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Attributes    map[string]interface{} `json:"attributes"`
	Events        []SpanEvent            `json:"events"`
	TokenUsage    TokenUsage             `json:"token_usage"`
	EstimatedCost float64                `json:"estimated_cost_usd"`
	mu            sync.Mutex
}

// End finishes the span, calculates duration, and sends it to the tracer exporter.
func (s *ObservabilitySpan) End(status SpanStatus, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.EndTime = time.Now().UTC()
	s.Duration = s.EndTime.Sub(s.StartTime)
	s.Status = status
	s.ErrorMessage = errMsg
}

// SetAttribute safely attaches a key-value attribute to the span.
func (s *ObservabilitySpan) SetAttribute(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Attributes == nil {
		s.Attributes = make(map[string]interface{})
	}
	s.Attributes[key] = val
}

// RecordEvent appends a timestamped event into the span.
func (s *ObservabilitySpan) RecordEvent(name string, attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Events = append(s.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now().UTC(),
		Attributes: attrs,
	})
}

// AddTokenUsage aggregates tokens and calculates approximate USD inference cost.
func (s *ObservabilitySpan) AddTokenUsage(prompt, completion, cached int, costPerMToken float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TokenUsage.PromptTokens += prompt
	s.TokenUsage.CompletionTokens += completion
	s.TokenUsage.CachedTokens += cached
	s.TokenUsage.TotalTokens = s.TokenUsage.PromptTokens + s.TokenUsage.CompletionTokens

	// Estimate cost: (prompt + completion) / 1,000,000 * costPerMToken
	if costPerMToken > 0 {
		s.EstimatedCost += (float64(prompt+completion) / 1000000.0) * costPerMToken
	}
}

// TraceContextKey is the context key for extracting the active span.
type traceContextKey struct{}

// Tracer manages trace generation, parent-child span linking, and telemetry buffering.
type Tracer struct {
	spansBuffer []*ObservabilitySpan
	maxBuffer   int
	mu          sync.RWMutex
}

// NewTracer constructs a thread-safe tracer with a bounded ring buffer.
func NewTracer(bufferSize int) *Tracer {
	if bufferSize <= 0 {
		bufferSize = 2048
	}
	return &Tracer{
		spansBuffer: make([]*ObservabilitySpan, 0, bufferSize),
		maxBuffer:   bufferSize,
	}
}

// StartSpan creates a new root or child span and injects it into context.
func (t *Tracer) StartSpan(
	ctx context.Context,
	name, orgID, repoID string,
	prNumber int,
) (*ObservabilitySpan, context.Context) {
	traceID := uuid.New().String()
	parentID := ""

	// Check if parent span exists in context
	if parentSpan, ok := ctx.Value(traceContextKey{}).(*ObservabilitySpan); ok && parentSpan != nil {
		traceID = parentSpan.TraceID
		parentID = parentSpan.SpanID
		if orgID == "" {
			orgID = parentSpan.OrgID
		}
		if repoID == "" {
			repoID = parentSpan.RepoID
		}
		if prNumber == 0 {
			prNumber = parentSpan.PRNumber
		}
	}

	sessionID := fmt.Sprintf("%s:%s:%d", orgID, repoID, prNumber)

	span := &ObservabilitySpan{
		SpanID:       uuid.New().String(),
		TraceID:      traceID,
		ParentSpanID: parentID,
		Name:         name,
		SessionID:    sessionID,
		OrgID:        orgID,
		RepoID:       repoID,
		PRNumber:     prNumber,
		StartTime:    time.Now().UTC(),
		Attributes:   make(map[string]interface{}),
		Events:       make([]SpanEvent, 0),
	}

	t.mu.Lock()
	if len(t.spansBuffer) >= t.maxBuffer {
		// Evict oldest span
		t.spansBuffer = t.spansBuffer[1:]
	}
	t.spansBuffer = append(t.spansBuffer, span)
	t.mu.Unlock()

	newCtx := context.WithValue(ctx, traceContextKey{}, span)
	return span, newCtx
}

// GetSpanFromContext retrieves the active span from context if present.
func GetSpanFromContext(ctx context.Context) *ObservabilitySpan {
	if span, ok := ctx.Value(traceContextKey{}).(*ObservabilitySpan); ok {
		return span
	}
	return nil
}

// GetSpansByTraceID retrieves all spans belonging to a given trace.
func (t *Tracer) GetSpansByTraceID(traceID string) []*ObservabilitySpan {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matched []*ObservabilitySpan
	for _, s := range t.spansBuffer {
		if s.TraceID == traceID {
			matched = append(matched, s)
		}
	}
	return matched
}

// GetSpansBySessionID retrieves all spans belonging to a pull request review session.
func (t *Tracer) GetSpansBySessionID(sessionID string) []*ObservabilitySpan {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matched []*ObservabilitySpan
	for _, s := range t.spansBuffer {
		if s.SessionID == sessionID {
			matched = append(matched, s)
		}
	}
	return matched
}

// TotalTokenUsageForSession aggregates token consumption and cost for an entire PR session.
func (t *Tracer) TotalTokenUsageForSession(sessionID string) (TokenUsage, float64) {
	spans := t.GetSpansBySessionID(sessionID)
	var total TokenUsage
	var cost float64

	for _, s := range spans {
		s.mu.Lock()
		total.PromptTokens += s.TokenUsage.PromptTokens
		total.CompletionTokens += s.TokenUsage.CompletionTokens
		total.CachedTokens += s.TokenUsage.CachedTokens
		total.TotalTokens += s.TokenUsage.TotalTokens
		cost += s.EstimatedCost
		s.mu.Unlock()
	}

	return total, cost
}
