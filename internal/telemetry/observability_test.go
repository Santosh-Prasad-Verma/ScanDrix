// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerWrapper_ContextEnrichment(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	rawLogger := slog.New(handler)

	wrapper := NewLoggerWrapper(rawLogger)

	ctx := context.WithValue(context.Background(), ContextKeyRequestID, "req-12345")
	ctx = context.WithValue(ctx, ContextKeyWorkspaceID, "ws-67890")
	ctx = context.WithValue(ctx, ContextKeyUserID, "user-abc")

	ctxLogger := wrapper.WithContext(ctx)
	ctxLogger.Info("test message with context")

	output := buf.String()
	assert.Contains(t, output, `"msg":"test message with context"`)
	assert.Contains(t, output, `"request_id":"req-12345"`)
	assert.Contains(t, output, `"workspace_id":"ws-67890"`)
	assert.Contains(t, output, `"user_id":"user-abc"`)

	// Test Error method
	buf.Reset()
	ctxLogger.Error("operation failed", errors.New("db connection timed out"))
	errOutput := buf.String()
	assert.Contains(t, errOutput, `"msg":"operation failed"`)
	assert.Contains(t, errOutput, `"error":"db connection timed out"`)
}

func TestObservabilityService_RecordOperations(t *testing.T) {
	obs := GetObservabilityService()
	require.NotNil(t, obs)

	// Verify RecordStepExecution does not panic
	obs.RecordStepExecution("security-gate", "gate", "success", 150*time.Millisecond)

	// Verify RecordLLMUsage does not panic
	obs.RecordLLMUsage("anthropic", "claude-3-7-sonnet", 1200, 450, 2*time.Second)

	// Verify RecordReviewOutcome does not panic
	obs.RecordReviewOutcome("github", "completed", 5*time.Second)

	// Verify StartSpan does not panic
	ctx, finish := obs.StartSpan(context.Background(), "test-span")
	require.NotNil(t, ctx)
	require.NotNil(t, finish)
	finish()
}
