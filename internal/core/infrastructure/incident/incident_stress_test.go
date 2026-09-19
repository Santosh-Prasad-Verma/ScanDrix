package incident_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/infrastructure/incident"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIncidentStress_CircuitBreakerConcurrency tests thread safety of CircuitBreaker
// under concurrent rapid transitions between CLOSED, OPEN, and HALF_OPEN.
func TestIncidentStress_CircuitBreakerConcurrency(t *testing.T) {
	cb := incident.NewCircuitBreaker(5, 20*time.Millisecond)
	const numGoroutines = 40
	const cycles = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < cycles; i++ {
				if workerID%2 == 0 {
					cb.RecordFailure()
				} else {
					cb.RecordSuccess()
				}
				_ = cb.Allow()
				_ = cb.State()
			}
		}(g)
	}

	wg.Wait()

	// Verify consistent state
	state := cb.State()
	assert.True(t, state == incident.CircuitClosed || state == incident.CircuitOpen || state == incident.CircuitHalfOpen)
}

// TestIncidentStress_DeduplicationConcurrency verifies that out of 100 concurrent
// identical incident reports, exactly 1 incident is dispatched and 99 are deduplicated.
func TestIncidentStress_DeduplicationConcurrency(t *testing.T) {
	var dispatchedIncidents atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatchedIncidents.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"inc_123","attributes":{"name":"Critical DB Error"}}}`))
	}))
	defer server.Close()

	client := incident.NewBetterStackClient("test-token", server.URL, server.Client())
	manager := incident.NewIncidentManager(client, "production", "scanner-service", 5*time.Second)

	const concurrency = 100
	var wg sync.WaitGroup
	var firedCount atomic.Int32
	var suppressedCount atomic.Int32

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			payload := incident.CreateIncidentPayload{
				Name:      "Database Connection Pool Exhausted",
				Summary:   "All 100 pool connections in use",
				Requester: "monitoring@scandrix.dev",
				Severity:  "critical",
			}

			created, err := manager.ReportIncident(context.Background(), "db_pool_exhausted_key", payload)
			assert.NoError(t, err)
			if created {
				firedCount.Add(1)
			} else {
				suppressedCount.Add(1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(1), firedCount.Load(), "Exactly 1 incident should have been fired")
	assert.Equal(t, int32(concurrency-1), suppressedCount.Load(), "Remaining should have been suppressed")
	assert.Equal(t, int32(1), dispatchedIncidents.Load())
}

// TestIncidentStress_CircuitBreakerTripsAndSkipsNetwork verifies that after threshold
// failures, the client skips network calls immediately with ErrCircuitBreakerOpen.
func TestIncidentStress_CircuitBreakerTripsAndSkipsNetwork(t *testing.T) {
	var serverRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverRequests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := incident.NewBetterStackClient("test-token", server.URL, server.Client())

	// Threshold is 5 failures
	for i := 0; i < 5; i++ {
		err := client.PingHeartbeat(context.Background(), server.URL)
		assert.Error(t, err)
	}

	assert.Equal(t, int32(5), serverRequests.Load())

	// 6th call should be rejected locally by circuit breaker without reaching server
	err := client.PingHeartbeat(context.Background(), server.URL)
	assert.ErrorIs(t, err, incident.ErrCircuitBreakerOpen)
	assert.Equal(t, int32(5), serverRequests.Load(), "Server should NOT have received 6th request")
}

// TestIncidentStress_SafeURLSanitization verifies redaction of tokens and query params in URLs.
func TestIncidentStress_SafeURLSanitization(t *testing.T) {
	client := incident.NewBetterStackClient("test-token", "", nil)

	testURLs := []struct {
		raw      string
		expected string
	}{
		{
			raw:      "https://uptime.betterstack.com/api/v2/heartbeats/0123456789abcdef012345",
			expected: "https://uptime.betterstack.com/api/v2/heartbeats/[REDACTED]",
		},
		{
			raw:      "https://uptime.betterstack.com/ping?secret_token=abc12345",
			expected: "https://uptime.betterstack.com/ping",
		},
		{
			raw:      "https://uptime.betterstack.com/api/v2/plain",
			expected: "https://uptime.betterstack.com/api/v2/plain",
		},
	}

	for _, tc := range testURLs {
		clean := client.RedactURL(tc.raw)
		assert.Equal(t, tc.expected, clean)
	}
}

// TestIncidentStress_ManagerHeartbeatSuccessAndFailure validates manager heartbeat pings.
func TestIncidentStress_ManagerHeartbeatSuccessAndFailure(t *testing.T) {
	var pingCount atomic.Int32
	var failCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			pingCount.Add(1)
			w.WriteHeader(http.StatusOK)
		} else if r.Method == http.MethodPost {
			failCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := incident.NewBetterStackClient("test-token", server.URL, server.Client())
	mgr := incident.NewIncidentManager(client, "production", "worker", time.Minute)

	// 1. Success ping
	err := mgr.PingHeartbeat(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, int32(1), pingCount.Load())

	// 2. Empty heartbeat URL is no-op
	err = mgr.PingHeartbeat(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, int32(1), pingCount.Load())

	// 3. Failure ping
	err = mgr.FailHeartbeat(context.Background(), server.URL, "Kafka Lag High", map[string]any{"lag": 1500})
	require.NoError(t, err)
	assert.Equal(t, int32(1), failCount.Load())
}
