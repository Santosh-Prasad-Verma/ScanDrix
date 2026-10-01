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
	"reflect"
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

	assert.Equal(t, "8080", cfg.Get("PORT"))                                // Overridden by .env.local
	assert.Equal(t, "on", cfg.Get("FEATURE_A"))                             // Overridden by .env.local
	assert.Equal(t, "postgres://baseline:5432/db", cfg.Get("DATABASE_URL")) // Retained from .env
	assert.Equal(t, "default-val", cfg.Get("NON_EXISTENT", "default-val"))  // Fallback

	assert.True(t, cfg.Has("PORT"))
	assert.False(t, cfg.Has("NON_EXISTENT"))

	require.NoError(t, cfg.ValidateRequired("PORT", "DATABASE_URL"))
	require.Error(t, cfg.ValidateRequired("PORT", "MISSING_VAR"))
}

// TestAuthGuardHasNoMiddleware pins AUDIT_REMEDIATION.md F-14.
//
// The guard previously accepted ANY non-empty Authorization header, never
// validated the token, and injected a hardcoded claims object with
// UserID "authenticated-user". The test at this location asserted that
// `Bearer valid-token-123` was accepted, i.e. it certified the bypass.
//
// AuthGuard now deliberately exposes no Middleware method, so wiring it fails
// at compile time rather than granting a session to anyone who sends a header.
// The real guards are auth.Authenticator.Middleware and the MCP transport
// middleware.
func TestAuthGuardHasNoMiddleware(t *testing.T) {
	guard := NewAuthGuard("secret-key", nil)
	require.NotNil(t, guard, "the constructor is retained so call sites fail to compile loudly")

	// Compile-time proof, expressed as a reflection check so the assertion
	// survives a future refactor that tries to re-add the method.
	mt := reflect.TypeOf(guard)
	_, hasMiddleware := mt.MethodByName("Middleware")
	assert.False(t, hasMiddleware,
		"AuthGuard must not expose Middleware: it cannot validate tokens and "+
			"would authenticate anyone. Use auth.Authenticator.Middleware instead.")
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
