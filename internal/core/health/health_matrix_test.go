package health_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Matrix Tests: Service Endpoints & Status Codes
// ============================================================================

func TestHealthMatrix_EndpointsStatusAndPayloadContract(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "2.5.0-matrix")

	t.Run("Check Endpoint with Nil DB returns 503 and ok app", func(t *testing.T) {
		code, resp := svc.Check(context.Background())
		assert.Equal(t, http.StatusServiceUnavailable, code)
		assert.Equal(t, "error", resp.Status)
		assert.Equal(t, "2.5.0-matrix", resp.Version)
		assert.NotEmpty(t, resp.Timestamp)

		// Application is up
		assert.Equal(t, "up", resp.Details.Application.Status)
		assert.NotEmpty(t, resp.Details.Application.Uptime)
		assert.True(t, resp.Details.Application.MemoryHeap > 0)

		// Database is down due to nil pool
		assert.Equal(t, "down", resp.Details.Database.Status)
		assert.Equal(t, "down", resp.Details.Database.Postgres.Status)
		assert.Contains(t, resp.Details.Database.Postgres.Message, "database pool not initialized")
	})

	t.Run("ReadyCheck is an exact alias for Check", func(t *testing.T) {
		codeCheck, respCheck := svc.Check(context.Background())
		codeReady, respReady := svc.ReadyCheck(context.Background())

		assert.Equal(t, codeCheck, codeReady)
		assert.Equal(t, respCheck.Status, respReady.Status)
		assert.Equal(t, respCheck.Version, respReady.Version)
		assert.Equal(t, respCheck.Details.Database.Status, respReady.Details.Database.Status)
	})

	t.Run("SimpleCheck returns 200 OK without database query", func(t *testing.T) {
		code, resp := svc.SimpleCheck()
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "2.5.0-matrix", resp.Version)
		assert.Equal(t, "API is running", resp.Message)
		assert.True(t, resp.Uptime >= 0)
		assert.NotEmpty(t, resp.Timestamp)
	})

	t.Run("LiveCheck is an exact alias for SimpleCheck", func(t *testing.T) {
		codeSimple, respSimple := svc.SimpleCheck()
		codeLive, respLive := svc.LiveCheck()

		assert.Equal(t, codeSimple, codeLive)
		assert.Equal(t, respSimple.Status, respLive.Status)
		assert.Equal(t, respSimple.Version, respLive.Version)
		assert.Equal(t, respSimple.Message, respLive.Message)
	})

	t.Run("ServingCheck evaluates Postgres connection pool", func(t *testing.T) {
		code, resp := svc.ServingCheck(context.Background())
		assert.Equal(t, http.StatusServiceUnavailable, code)
		assert.Equal(t, "error", resp.Status)
		assert.Equal(t, "2.5.0-matrix", resp.Version)
		assert.NotNil(t, resp.Details["postgres"])

		pgStatus, ok := resp.Details["postgres"].(health.PostgresStatusDetails)
		require.True(t, ok)
		assert.Equal(t, "down", pgStatus.Status)
	})
}

// ============================================================================
// Matrix Tests: Version and Environment Precedence Matrix
// ============================================================================

func TestHealthMatrix_VersionAndEnvironmentPrecedence(t *testing.T) {
	originalEnv := os.Getenv("API_NODE_ENV")
	originalScandrixEnv := os.Getenv("SCANDRIX_ENV")
	originalRelVer := os.Getenv("RELEASE_VERSION")

	defer func() {
		os.Setenv("API_NODE_ENV", originalEnv)
		os.Setenv("SCANDRIX_ENV", originalScandrixEnv)
		os.Setenv("RELEASE_VERSION", originalRelVer)
	}()

	testCases := []struct {
		name            string
		paramVersion    string
		envReleaseVer   string
		expectedVersion string
		apiNodeEnv      string
		scandrixEnv     string
		expectedEnv     string
	}{
		{
			name:            "Explicit Version Overrides All Envs",
			paramVersion:    "3.0.0-custom",
			envReleaseVer:   "2.0.0",
			expectedVersion: "3.0.0-custom",
			apiNodeEnv:      "staging",
			scandrixEnv:     "development",
			expectedEnv:     "staging",
		},
		{
			name:            "RELEASE_VERSION Fallback when Param Empty",
			paramVersion:    "",
			envReleaseVer:   "2.4.1",
			expectedVersion: "2.4.1",
			apiNodeEnv:      "",
			scandrixEnv:     "sandbox",
			expectedEnv:     "sandbox",
		},
		{
			name:            "Default 1.0.0 and Production Fallbacks",
			paramVersion:    "",
			envReleaseVer:   "",
			expectedVersion: "1.0.0",
			apiNodeEnv:      "",
			scandrixEnv:     "",
			expectedEnv:     "production",
		},
		{
			name:            "API_NODE_ENV Takes Precedence over SCANDRIX_ENV",
			paramVersion:    "1.2.3",
			envReleaseVer:   "",
			expectedVersion: "1.2.3",
			apiNodeEnv:      "test-env-node",
			scandrixEnv:     "test-env-scandrix",
			expectedEnv:     "test-env-node",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("RELEASE_VERSION", tc.envReleaseVer)
			os.Setenv("API_NODE_ENV", tc.apiNodeEnv)
			os.Setenv("SCANDRIX_ENV", tc.scandrixEnv)

			svc := health.NewService(nil, nil, nil, tc.paramVersion)
			code, resp := svc.Check(context.Background())
			assert.Equal(t, http.StatusServiceUnavailable, code)
			assert.Equal(t, tc.expectedVersion, resp.Version)
			assert.Equal(t, tc.expectedEnv, resp.Details.Application.Environment)
		})
	}
}

// ============================================================================
// Matrix Tests: Application Health Indicator & Uptime Formatting
// ============================================================================

func TestHealthMatrix_ApplicationHealthIndicatorAndUptime(t *testing.T) {
	t.Run("Uptime Formatting Across Distinct Time Horizons", func(t *testing.T) {
		horizons := []struct {
			name        string
			offsetPast  time.Duration
			expectedFmt string
		}{
			{
				name:        "Just started (seconds only)",
				offsetPast:  25 * time.Second,
				expectedFmt: "25s",
			},
			{
				name:        "Minutes and seconds",
				offsetPast:  145 * time.Second, // 2m 25s
				expectedFmt: "2m 25s",
			},
			{
				name:        "Hours, minutes and seconds",
				offsetPast:  3665 * time.Second, // 1h 1m 5s
				expectedFmt: "1h 1m 5s",
			},
			{
				name:        "Long running process (48 hours)",
				offsetPast:  48 * time.Hour,
				expectedFmt: "48h 0m",
			},
		}

		for _, h := range horizons {
			t.Run(h.name, func(t *testing.T) {
				startTime := time.Now().Add(-h.offsetPast)
				appIndicator := health.NewApplicationHealthIndicator(startTime)
				status, healthy := appIndicator.IsApplicationHealthy()

				assert.True(t, healthy)
				assert.Equal(t, "up", status.Status)
				assert.Contains(t, status.Uptime, h.expectedFmt)
			})
		}
	})

	t.Run("RFC3339 Timestamp Parsing Validity", func(t *testing.T) {
		appIndicator := health.NewApplicationHealthIndicator(time.Now())
		status, _ := appIndicator.IsApplicationHealthy()

		parsed, err := time.Parse(time.RFC3339, status.Timestamp)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().UTC(), parsed, 5*time.Second)
	})
}

// ============================================================================
// Matrix Tests: Database Health Indicator
// ============================================================================

func TestHealthMatrix_DatabaseHealthIndicator(t *testing.T) {
	t.Run("Nil Database Connection Pool", func(t *testing.T) {
		dbIndicator := health.NewDatabaseHealthIndicator(nil)
		ctx := context.Background()

		details, healthy := dbIndicator.IsPostgresHealthy(ctx, 1000)
		assert.False(t, healthy)
		assert.Equal(t, "down", details.Status)
		assert.Equal(t, "database pool not initialized", details.Message)

		dbStatus, overallHealthy := dbIndicator.IsDatabaseHealthy(ctx)
		assert.False(t, overallHealthy)
		assert.Equal(t, "down", dbStatus.Status)
		assert.Equal(t, "down", dbStatus.Postgres.Status)
	})

	t.Run("Pre-Cancelled Context Handling", func(t *testing.T) {
		dbIndicator := health.NewDatabaseHealthIndicator(nil)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		details, healthy := dbIndicator.IsPostgresHealthy(cancelledCtx, 500)
		assert.False(t, healthy)
		assert.Equal(t, "down", details.Status)
	})
}

// ============================================================================
// Matrix Tests: JSON Schema and Serialization Contracts
// ============================================================================

func TestHealthMatrix_JSONSerializationContracts(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "1.0.0")

	t.Run("FullHealthResponse Serializes Exact Field Names", func(t *testing.T) {
		_, resp := svc.Check(context.Background())
		bytes, err := json.Marshal(resp)
		require.NoError(t, err)

		rawJSON := string(bytes)
		assert.Contains(t, rawJSON, `"status":`)
		assert.Contains(t, rawJSON, `"version":`)
		assert.Contains(t, rawJSON, `"timestamp":`)
		assert.Contains(t, rawJSON, `"details":`)
		assert.Contains(t, rawJSON, `"application":`)
		assert.Contains(t, rawJSON, `"database":`)
		assert.Contains(t, rawJSON, `"postgres":`)
		assert.Contains(t, rawJSON, `"uptime":`)
		assert.Contains(t, rawJSON, `"environment":`)
		assert.Contains(t, rawJSON, `"memory_heap_bytes":`)
		assert.Contains(t, rawJSON, `"latency_ms":`)

		var parsed map[string]any
		err = json.Unmarshal(bytes, &parsed)
		require.NoError(t, err)
		assert.Equal(t, "error", parsed["status"])
	})

	t.Run("SimpleHealthResponse Serializes Exact Field Names", func(t *testing.T) {
		_, resp := svc.SimpleCheck()
		bytes, err := json.Marshal(resp)
		require.NoError(t, err)

		rawJSON := string(bytes)
		assert.Contains(t, rawJSON, `"status":"ok"`)
		assert.Contains(t, rawJSON, `"message":"API is running"`)
		assert.Contains(t, rawJSON, `"uptime":`)
		assert.Contains(t, rawJSON, `"version":"1.0.0"`)
	})

	t.Run("ServingCheck Serializes Nested Postgres Status", func(t *testing.T) {
		_, resp := svc.ServingCheck(context.Background())
		bytes, err := json.Marshal(resp)
		require.NoError(t, err)

		rawJSON := string(bytes)
		assert.Contains(t, rawJSON, `"details":`)
		assert.Contains(t, rawJSON, `"postgres":`)
		assert.Contains(t, rawJSON, `"status":"down"`)
	})
}

// ============================================================================
// Matrix Tests: High Concurrency Multithreaded Probing Stress
// ============================================================================

func TestHealthMatrix_HighConcurrencyMultiEndpointStress(t *testing.T) {
	svc := health.NewService(nil, nil, nil, "v1.9.9")
	const numGoroutines = 50
	const iterationsPerGoroutine = 30

	var totalCalls atomic.Int64
	var checkCalls atomic.Int64
	var simpleCalls atomic.Int64
	var servingCalls atomic.Int64
	var readyCalls atomic.Int64
	var liveCalls atomic.Int64

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	ctx := context.Background()

	for g := 0; g < numGoroutines; g++ {
		go func(routineID int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				op := (routineID*iterationsPerGoroutine + i) % 5
				switch op {
				case 0:
					code, resp := svc.Check(ctx)
					assert.Equal(t, http.StatusServiceUnavailable, code)
					assert.Equal(t, "error", resp.Status)
					checkCalls.Add(1)
				case 1:
					code, resp := svc.SimpleCheck()
					assert.Equal(t, http.StatusOK, code)
					assert.Equal(t, "ok", resp.Status)
					simpleCalls.Add(1)
				case 2:
					code, resp := svc.ServingCheck(ctx)
					assert.Equal(t, http.StatusServiceUnavailable, code)
					assert.Equal(t, "error", resp.Status)
					servingCalls.Add(1)
				case 3:
					code, resp := svc.ReadyCheck(ctx)
					assert.Equal(t, http.StatusServiceUnavailable, code)
					assert.Equal(t, "error", resp.Status)
					readyCalls.Add(1)
				case 4:
					code, resp := svc.LiveCheck()
					assert.Equal(t, http.StatusOK, code)
					assert.Equal(t, "ok", resp.Status)
					liveCalls.Add(1)
				}
				totalCalls.Add(1)
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := int64(numGoroutines * iterationsPerGoroutine)
	assert.Equal(t, expectedTotal, totalCalls.Load())
	assert.Equal(t, expectedTotal/5, checkCalls.Load())
	assert.Equal(t, expectedTotal/5, simpleCalls.Load())
	assert.Equal(t, expectedTotal/5, servingCalls.Load())
	assert.Equal(t, expectedTotal/5, readyCalls.Load())
	assert.Equal(t, expectedTotal/5, liveCalls.Load())
}
