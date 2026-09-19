// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ObservabilityService provides unified recording of metrics, traces, and step executions,
// matching the enterprise ObservabilityModule architecture.
type ObservabilityService struct {
	metrics               *Metrics
	stepExecutionDuration *prometheus.HistogramVec
	stepExecutionsTotal   *prometheus.CounterVec
	llmTokensTotal        *prometheus.CounterVec
	llmLatencySeconds     *prometheus.HistogramVec
}

var globalObservability *ObservabilityService

// NewObservabilityService constructs an initialized ObservabilityService with registered collectors.
func NewObservabilityService(m *Metrics) *ObservabilityService {
	if m == nil {
		m = GetMetrics()
	}

	stepDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scandrix_blueprint_step_duration_seconds",
			Help:    "Execution duration of blueprint and pipeline steps in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
		},
		[]string{"step_name", "step_type", "status"},
	)

	stepTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scandrix_blueprint_step_executions_total",
			Help: "Total count of blueprint step executions partitioned by step name, type, and status",
		},
		[]string{"step_name", "step_type", "status"},
	)

	tokensTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "scandrix_llm_tokens_total",
			Help: "Total LLM tokens consumed partitioned by provider and model",
		},
		[]string{"provider", "model", "type"},
	)

	llmLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "scandrix_llm_request_duration_seconds",
			Help:    "Latency of downstream LLM inferences in seconds",
			Buckets: []float64{0.25, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 45.0, 90.0},
		},
		[]string{"provider", "model"},
	)

	prometheus.MustRegister(stepDuration, stepTotal, tokensTotal, llmLatency)

	return &ObservabilityService{
		metrics:               m,
		stepExecutionDuration: stepDuration,
		stepExecutionsTotal:   stepTotal,
		llmTokensTotal:        tokensTotal,
		llmLatencySeconds:     llmLatency,
	}
}

// GetObservabilityService retrieves the global singleton ObservabilityService.
func GetObservabilityService() *ObservabilityService {
	if globalObservability == nil {
		globalObservability = NewObservabilityService(GetMetrics())
	}
	return globalObservability
}

// RecordStepExecution logs a blueprint/stage execution outcome into Prometheus.
func (o *ObservabilityService) RecordStepExecution(stepName, stepType, status string, duration time.Duration) {
	if o == nil || o.stepExecutionDuration == nil {
		return
	}
	durationSec := duration.Seconds()
	o.stepExecutionDuration.WithLabelValues(stepName, stepType, status).Observe(durationSec)
	o.stepExecutionsTotal.WithLabelValues(stepName, stepType, status).Inc()
}

// RecordLLMUsage logs token consumption and inference latency.
func (o *ObservabilityService) RecordLLMUsage(provider, model string, promptTokens, completionTokens int, latency time.Duration) {
	if o == nil {
		return
	}
	if promptTokens > 0 && o.llmTokensTotal != nil {
		o.llmTokensTotal.WithLabelValues(provider, model, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 && o.llmTokensTotal != nil {
		o.llmTokensTotal.WithLabelValues(provider, model, "completion").Add(float64(completionTokens))
	}
	if latency > 0 && o.llmLatencySeconds != nil {
		o.llmLatencySeconds.WithLabelValues(provider, model).Observe(latency.Seconds())
	}
}

// RecordReviewOutcome logs pull request review completion metrics.
func (o *ObservabilityService) RecordReviewOutcome(provider, status string, duration time.Duration) {
	if o == nil || o.metrics == nil {
		return
	}
	o.metrics.ReviewsProcessedTotal.WithLabelValues(status, provider).Inc()
	o.metrics.ReviewDurationSeconds.WithLabelValues(provider).Observe(duration.Seconds())
}

// StartSpan creates a lightweight span or trace event for the operation.
func (o *ObservabilityService) StartSpan(ctx context.Context, operationName string) (context.Context, func()) {
	startedAt := time.Now()
	return ctx, func() {
		_ = time.Since(startedAt)
	}
}
