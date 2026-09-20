// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExceptionsFilter_RecoversPanicWithoutLeakingInternals(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection string postgres://secret:password@10.0.0.1/db failed")
	})

	handler := ExceptionsFilter(logger)(panicHandler)

	req := httptest.NewRequest("GET", "/api/v1/sensitive", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify HTTP 500 response
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var resp map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Verify client does NOT receive the panic or secret text
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, "An internal server error occurred", resp["error"])
	assert.NotContains(t, rr.Body.String(), "postgres://secret:password")
	assert.NotContains(t, rr.Body.String(), "panic")

	// Verify server log DID record the panic details for debugging
	logStr := logBuf.String()
	assert.Contains(t, logStr, "Unhandled panic intercepted in HTTP pipeline")
	assert.Contains(t, logStr, "postgres://secret:password")
}

func TestLoggingInterceptor_RecordsRequestMetrics(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok response"))
	})

	handler := LoggingInterceptor(logger)(okHandler)

	req := httptest.NewRequest("POST", "/api/v1/reviews", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	logStr := logBuf.String()
	assert.Contains(t, logStr, "HTTP request processed")
	assert.Contains(t, logStr, `"method":"POST"`)
	assert.Contains(t, logStr, `"path":"/api/v1/reviews"`)
	assert.Contains(t, logStr, `"status":200`)
}

func TestRespondJSON_EnvelopeFormatting(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	data := map[string]string{"id": "item-123"}
	meta := map[string]int{"page": 1}

	RespondJSON(rr, req, http.StatusOK, data, meta)

	assert.Equal(t, http.StatusOK, rr.Code)

	var env ResponseEnvelope
	err := json.Unmarshal(rr.Body.Bytes(), &env)
	require.NoError(t, err)

	assert.True(t, env.Success)
	assert.Empty(t, env.Error)

	dataMap, ok := env.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "item-123", dataMap["id"])

	metaMap, ok := env.Meta.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), metaMap["page"])
}
