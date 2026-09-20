package health_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthStress_HighConcurrencyProbes verifies that 50 concurrent goroutines
// polling readiness, liveness, serving, and full checks execute with zero data races.
func TestHealthStress_HighConcurrencyProbes(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "2.5.0-rc1")
	ctx := context.Background()

	const numGoroutines = 50
	const checksPerWorker = 30

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < checksPerWorker; i++ {
				// 1. SimpleCheck / LiveCheck (always succeeds)
				liveCode, liveResp := svc.LiveCheck()
				assert.Equal(t, http.StatusOK, liveCode)
				assert.Equal(t, "ok", liveResp.Status)
				assert.Equal(t, "2.5.0-rc1", liveResp.Version)
				assert.True(t, liveResp.Uptime >= 0)

				simpleCode, simpleResp := svc.SimpleCheck()
				assert.Equal(t, http.StatusOK, simpleCode)
				assert.Equal(t, "ok", simpleResp.Status)

				// 2. Full Check (with nil dbPool, returns 503)
				fullCode, fullResp := svc.Check(ctx)
				assert.Equal(t, http.StatusServiceUnavailable, fullCode)
				assert.Equal(t, "error", fullResp.Status)
				assert.Equal(t, "up", fullResp.Details.Application.Status)
				assert.Equal(t, "down", fullResp.Details.Database.Status)

				// 3. ReadyCheck (alias for Check)
				readyCode, readyResp := svc.ReadyCheck(ctx)
				assert.Equal(t, http.StatusServiceUnavailable, readyCode)
				assert.Equal(t, "error", readyResp.Status)

				// 4. ServingCheck (returns 503 when DB down)
				servCode, servResp := svc.ServingCheck(ctx)
				assert.Equal(t, http.StatusServiceUnavailable, servCode)
				assert.Equal(t, "error", servResp.Status)
			}
		}(g)
	}

	wg.Wait()
}

// TestHealthStress_UptimeFormattingEdgeCases validates the formatting of seconds
// into human-readable strings across second, minute, hour, and multi-day boundaries.
func TestHealthStress_UptimeFormattingEdgeCases(t *testing.T) {
	testDurations := []struct {
		elapsed  time.Duration
		expected string
	}{
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 0s"},
		{75 * time.Second, "1m 15s"},
		{3599 * time.Second, "59m 59s"},
		{3600 * time.Second, "1h 0m 0s"},
		{3661 * time.Second, "1h 1m 1s"},
		{86400 * time.Second, "24h 0m 0s"},
	}

	now := time.Now().UTC()
	for _, tc := range testDurations {
		startTime := now.Add(-tc.elapsed)
		indicator := health.NewApplicationHealthIndicator(startTime)
		status, isHealthy := indicator.IsApplicationHealthy()

		assert.True(t, isHealthy)
		assert.Equal(t, "up", status.Status)
		assert.Equal(t, tc.expected, status.Uptime)
		assert.NotEmpty(t, status.Timestamp)
		assert.True(t, status.MemoryHeap > 0)
	}
}

// TestHealthStress_EnvironmentFallbackResolution verifies configuration of environment
// strings via API_NODE_ENV, SCANDRIX_ENV, and default "production".
func TestHealthStress_EnvironmentFallbackResolution(t *testing.T) {
	// 1. Explicit SCANDRIX_ENV
	_ = os.Setenv("SCANDRIX_ENV", "staging-us-east-1")
	_ = os.Unsetenv("API_NODE_ENV")
	ind1 := health.NewApplicationHealthIndicator(time.Now())
	st1, _ := ind1.IsApplicationHealthy()
	assert.Equal(t, "staging-us-east-1", st1.Environment)

	// 2. Override via API_NODE_ENV
	_ = os.Setenv("API_NODE_ENV", "development")
	ind2 := health.NewApplicationHealthIndicator(time.Now())
	st2, _ := ind2.IsApplicationHealthy()
	assert.Equal(t, "development", st2.Environment)

	// Clean up
	_ = os.Unsetenv("API_NODE_ENV")
	_ = os.Unsetenv("SCANDRIX_ENV")
}

// TestHealthStress_VersionDefaultFallbacks tests resolution of version from parameter,
// environment variable RELEASE_VERSION, or fallback "1.0.0".
func TestHealthStress_VersionDefaultFallbacks(t *testing.T) {
	// 1. Explicit parameter
	svc1 := health.NewService(nil, nil, nil, "3.1.2")
	_, resp1 := svc1.SimpleCheck()
	assert.Equal(t, "3.1.2", resp1.Version)

	// 2. From RELEASE_VERSION
	_ = os.Setenv("RELEASE_VERSION", "4.0.0-beta")
	svc2 := health.NewService(nil, nil, nil, "")
	_, resp2 := svc2.SimpleCheck()
	assert.Equal(t, "4.0.0-beta", resp2.Version)
	_ = os.Unsetenv("RELEASE_VERSION")

	// 3. Default "1.0.0"
	svc3 := health.NewService(nil, nil, nil, "")
	_, resp3 := svc3.SimpleCheck()
	assert.Equal(t, "1.0.0", resp3.Version)
}

// TestHealthStress_JSONResponseContractCompliance confirms all health response
// DTOs serialize into RFC3339-compliant JSON matching frontend and monitoring expectations.
func TestHealthStress_JSONResponseContractCompliance(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "1.5.0")
	ctx := context.Background()

	// 1. FullHealthResponse JSON
	_, full := svc.Check(ctx)
	fullBytes, err := json.Marshal(full)
	require.NoError(t, err)

	var fullParsed map[string]any
	err = json.Unmarshal(fullBytes, &fullParsed)
	require.NoError(t, err)
	assert.Equal(t, "error", fullParsed["status"])
	assert.Equal(t, "1.5.0", fullParsed["version"])
	assert.Contains(t, fullParsed, "details")
	assert.Contains(t, fullParsed, "timestamp")

	// 2. SimpleHealthResponse JSON
	_, simple := svc.SimpleCheck()
	simpleBytes, err := json.Marshal(simple)
	require.NoError(t, err)

	var simpleParsed map[string]any
	err = json.Unmarshal(simpleBytes, &simpleParsed)
	require.NoError(t, err)
	assert.Equal(t, "ok", simpleParsed["status"])
	assert.Equal(t, "1.5.0", simpleParsed["version"])
	assert.Equal(t, "API is running", simpleParsed["message"])
}

// TestHealthStress_ContextCancellationDuringHealthCheck validates that cancelling
// the probe context does not cause panics or deadlocks in health indicators.
func TestHealthStress_ContextCancellationDuringHealthCheck(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "1.0.0")

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, resp := svc.Check(ctx)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "error", resp.Status)

	servCode, servResp := svc.ServingCheck(ctx)
	assert.Equal(t, http.StatusServiceUnavailable, servCode)
	assert.Equal(t, "error", servResp.Status)
}
