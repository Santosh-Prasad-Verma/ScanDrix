// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package telemetry

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

const (
	ContextKeyRequestID   contextKey = "request_id"
	ContextKeyWorkspaceID contextKey = "workspace_id"
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyTraceID     contextKey = "trace_id"
)

// LoggerWrapper provides structured logging with contextual enrichment.
// It matches the enterprise LoggingService architecture and implements the blueprint.Logger interface.
type LoggerWrapper struct {
	logger *slog.Logger
}

// NewLoggerWrapper creates a new LoggerWrapper using slog.
func NewLoggerWrapper(base *slog.Logger) *LoggerWrapper {
	if base == nil {
		base = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}
	return &LoggerWrapper{logger: base}
}

// DefaultLogger returns a singleton JSON LoggerWrapper.
var defaultLogger = NewLoggerWrapper(nil)

func DefaultLogger() *LoggerWrapper {
	return defaultLogger
}

// WithContext returns an enriched LoggerWrapper containing contextual attributes (request_id, workspace_id, etc.).
func (l *LoggerWrapper) WithContext(ctx context.Context) *LoggerWrapper {
	if ctx == nil {
		return l
	}

	var attrs []any
	if reqID, ok := ctx.Value(ContextKeyRequestID).(string); ok && reqID != "" {
		attrs = append(attrs, "request_id", reqID)
	}
	if wsID, ok := ctx.Value(ContextKeyWorkspaceID).(string); ok && wsID != "" {
		attrs = append(attrs, "workspace_id", wsID)
	}
	if userID, ok := ctx.Value(ContextKeyUserID).(string); ok && userID != "" {
		attrs = append(attrs, "user_id", userID)
	}
	if traceID, ok := ctx.Value(ContextKeyTraceID).(string); ok && traceID != "" {
		attrs = append(attrs, "trace_id", traceID)
	}

	if len(attrs) == 0 {
		return l
	}

	return &LoggerWrapper{
		logger: l.logger.With(attrs...),
	}
}

// With returns a new LoggerWrapper with additional attributes.
func (l *LoggerWrapper) With(args ...any) *LoggerWrapper {
	return &LoggerWrapper{
		logger: l.logger.With(args...),
	}
}

// Log logs an informational message (satisfies blueprint.Logger).
func (l *LoggerWrapper) Log(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Info logs an informational message.
func (l *LoggerWrapper) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs a warning message.
func (l *LoggerWrapper) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Debug logs a debug message.
func (l *LoggerWrapper) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Error logs an error message with error details (satisfies blueprint.Logger).
func (l *LoggerWrapper) Error(msg string, err error, args ...any) {
	if err != nil {
		allArgs := append([]any{"error", err.Error()}, args...)
		l.logger.Error(msg, allArgs...)
	} else {
		l.logger.Error(msg, args...)
	}
}

// Underlying returns the raw *slog.Logger.
func (l *LoggerWrapper) Underlying() *slog.Logger {
	return l.logger
}
