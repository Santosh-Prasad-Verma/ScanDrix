// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Infrastructure Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package infrastructure

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/telemetry"
)

// SharedObservabilityService provides a unified interface for recording operational telemetry.
type SharedObservabilityService struct {
	obs *telemetry.ObservabilityService
}

// NewSharedObservabilityService constructs the service wrapping the core telemetry package.
func NewSharedObservabilityService() *SharedObservabilityService {
	return &SharedObservabilityService{
		obs: telemetry.GetObservabilityService(),
	}
}

// RecordStepExecution logs blueprint or pipeline step execution metrics.
func (s *SharedObservabilityService) RecordStepExecution(stepName, stepType, status string, duration time.Duration) {
	if s.obs != nil {
		s.obs.RecordStepExecution(stepName, stepType, status, duration)
	}
}

// RecordLLMUsage logs token consumption and model inference latency.
func (s *SharedObservabilityService) RecordLLMUsage(provider, model string, promptTokens, completionTokens int, latency time.Duration) {
	if s.obs != nil {
		s.obs.RecordLLMUsage(provider, model, promptTokens, completionTokens, latency)
	}
}

// RecordReviewOutcome logs pull request review outcome and execution time.
func (s *SharedObservabilityService) RecordReviewOutcome(provider, status string, duration time.Duration) {
	if s.obs != nil {
		s.obs.RecordReviewOutcome(provider, status, duration)
	}
}

// StartSpan starts a lightweight tracing span.
func (s *SharedObservabilityService) StartSpan(ctx context.Context, operationName string) (context.Context, func()) {
	if s.obs != nil {
		return s.obs.StartSpan(ctx, operationName)
	}
	return ctx, func() {}
}
