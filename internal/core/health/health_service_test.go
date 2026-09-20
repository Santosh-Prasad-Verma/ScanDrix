package health_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/health"
	"github.com/stretchr/testify/assert"
)

func TestHealthEndpoints(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "2.4.0")
	ctx := context.Background()

	// 1. SimpleCheck & LiveCheck
	code, simple := svc.SimpleCheck()
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", simple.Status)
	assert.Equal(t, "2.4.0", simple.Version)
	assert.Equal(t, "API is running", simple.Message)

	codeLive, live := svc.LiveCheck()
	assert.Equal(t, http.StatusOK, codeLive)
	assert.Equal(t, simple.Status, live.Status)

	// 2. Full Check (with nil DB pool returns 503)
	codeFull, full := svc.Check(ctx)
	assert.Equal(t, http.StatusServiceUnavailable, codeFull)
	assert.Equal(t, "error", full.Status)
	assert.Equal(t, "2.4.0", full.Version)
	assert.Equal(t, "up", full.Details.Application.Status)
	assert.Equal(t, "down", full.Details.Database.Status)

	// 3. ReadyCheck (alias)
	codeReady, _ := svc.ReadyCheck(ctx)
	assert.Equal(t, codeFull, codeReady)

	// 4. ServingCheck (with nil DB pool returns 503)
	codeServing, serving := svc.ServingCheck(ctx)
	assert.Equal(t, http.StatusServiceUnavailable, codeServing)
	assert.Equal(t, "error", serving.Status)
}

func TestApplicationHealthIndicator(t *testing.T) {
	indicator := health.NewApplicationHealthIndicator(time.Now().Add(-65 * time.Second))
	status, healthy := indicator.IsApplicationHealthy()

	assert.True(t, healthy)
	assert.Equal(t, "up", status.Status)
	assert.Contains(t, status.Uptime, "1m")
	assert.NotEmpty(t, status.Timestamp)
	assert.NotEmpty(t, status.Environment)
}
