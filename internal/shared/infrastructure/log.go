// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Infrastructure Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package infrastructure

import (
	"context"
	"log/slog"
	"os"
)

// LoggerWrapperService wraps structured slog.Logger with contextual metadata helpers.
type LoggerWrapperService struct {
	logger *slog.Logger
}

// NewLoggerWrapperService creates an enterprise structured logging wrapper.
func NewLoggerWrapperService(contextName string) *LoggerWrapperService {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler).With("context", contextName)
	return &LoggerWrapperService{
		logger: logger,
	}
}

// WithContext returns a new logger enriched with context key-values.
func (l *LoggerWrapperService) WithContext(ctx context.Context) *LoggerWrapperService {
	enriched := l.logger
	if claims, ok := ctx.Value(AuthContextKey{}).(*AuthenticatedClaims); ok && claims != nil {
		enriched = enriched.With(
			"user_id", claims.UserID,
			"organization_id", claims.OrganizationID,
		)
	}
	return &LoggerWrapperService{logger: enriched}
}

// Info logs informational messages.
func (l *LoggerWrapperService) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs warning messages.
func (l *LoggerWrapperService) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs error messages.
func (l *LoggerWrapperService) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// Debug logs debug messages.
func (l *LoggerWrapperService) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}
