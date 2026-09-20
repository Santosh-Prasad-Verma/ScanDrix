// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Infrastructure Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedConfig_CascadingAndValidation(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	envLocalPath := filepath.Join(tmpDir, ".env.local")

	// .env: baseline
	err := os.WriteFile(envPath, []byte("PORT=3000\nDATABASE_URL=postgres://baseline:5432/db\nFEATURE_A=off\n"), 0600)
	require.NoError(t, err)

	// .env.local: override
	err = os.WriteFile(envLocalPath, []byte("PORT=8080\nFEATURE_A=on\n"), 0600)
	require.NoError(t, err)

	cfg, err := LoadCascadingConfig(envLocalPath, envPath)
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Get("PORT"))                                        // Overridden by .env.local
	assert.Equal(t, "on", cfg.Get("FEATURE_A"))                                     // Overridden by .env.local
	assert.Equal(t, "postgres://baseline:5432/db", cfg.Get("DATABASE_URL"))         // Retained from .env
	assert.Equal(t, "default-val", cfg.Get("NON_EXISTENT", "default-val"))          // Fallback

	assert.True(t, cfg.Has("PORT"))
	assert.False(t, cfg.Has("NON_EXISTENT"))

	require.NoError(t, cfg.ValidateRequired("PORT", "DATABASE_URL"))
	require.Error(t, cfg.ValidateRequired("PORT", "MISSING_VAR"))
}

func TestAuthGuard_Middleware(t *testing.T) {
	guard := NewAuthGuard("secret-key", nil)

	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(AuthContextKey{}).(*AuthenticatedClaims)
		require.True(t, ok)
		assert.Equal(t, "authenticated-user", claims.UserID)
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Missing header
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rrNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(rrNoAuth, reqNoAuth)
	assert.Equal(t, http.StatusUnauthorized, rrNoAuth.Code)

	// 2. Malformed header
	reqBadAuth := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqBadAuth.Header.Set("Authorization", "Basic 1234")
	rrBadAuth := httptest.NewRecorder()
	handler.ServeHTTP(rrBadAuth, reqBadAuth)
	assert.Equal(t, http.StatusUnauthorized, rrBadAuth.Code)

	// 3. Valid Bearer token
	reqValid := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqValid.Header.Set("Authorization", "Bearer valid-token-123")
	rrValid := httptest.NewRecorder()
	handler.ServeHTTP(rrValid, reqValid)
	assert.Equal(t, http.StatusOK, rrValid.Code)
}

func TestExceptionsFilter_PanicRecovery(t *testing.T) {
	filter := ExceptionsFilter(nil)
	panickingHandler := filter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection blew up")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/crash", nil)
	rr := httptest.NewRecorder()
	panickingHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Internal server error")
}

func TestLoggerWrapperService_WithContext(t *testing.T) {
	logger := NewLoggerWrapperService("test_service")
	require.NotNil(t, logger)

	claims := &AuthenticatedClaims{
		UserID:         "usr-1",
		OrganizationID: "org-1",
	}
	ctx := context.WithValue(context.Background(), AuthContextKey{}, claims)
	ctxLogger := logger.WithContext(ctx)
	require.NotNil(t, ctxLogger)

	// Verify logging does not panic
	ctxLogger.Info("test info", "key", "val")
	ctxLogger.Warn("test warn")
	ctxLogger.Error("test error")
	ctxLogger.Debug("test debug")
}

func TestSharedObservabilityService_Recording(t *testing.T) {
	obs := NewSharedObservabilityService()
	require.NotNil(t, obs)

	obs.RecordStepExecution("step-1", "deterministic", "success", 15*time.Millisecond)
	obs.RecordLLMUsage("anthropic", "claude-3-7-sonnet", 100, 250, 800*time.Millisecond)
	obs.RecordReviewOutcome("github", "success", 2*time.Second)

	ctx, spanEnd := obs.StartSpan(context.Background(), "test_op")
	require.NotNil(t, ctx)
	spanEnd()
}

func TestPoolErrorHandlerService(t *testing.T) {
	handler := NewPoolErrorHandlerService(nil)
	require.NotNil(t, handler)
	handler.HandlePoolError(nil, "noop")
	handler.HandlePoolError(assert.AnError, "test_query")
}
