// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: beacon_test.go
// ═══════════════════════════════════════════════════════════════

package beacon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCollector struct {
	collectCount int32
	metrics      *HeartbeatMetrics
}

func (m *mockCollector) Collect(_ context.Context, input CollectInput) (*HeartbeatMetrics, error) {
	atomic.AddInt32(&m.collectCount, 1)
	if m.metrics != nil {
		return m.metrics, nil
	}
	return &HeartbeatMetrics{
		ScanDrix: ScanDrixInfo{
			Version:     "1.2.3",
			Deployment:  DeploymentBare,
			UptimeHours: 42,
		},
		Runtime: RuntimeInfo{
			GoVersion: "go1.24.0",
			OS:        "linux",
			Arch:      "amd64",
			CPUCount:  8,
			DBType:    "postgres",
			DBVersion: "PostgreSQL 16.2",
		},
		Usage7d: Usage7d{
			ActiveUsers:    10,
			Organizations:  2,
			Teams:          3,
			ReposConnected: 5,
			PRsReviewed:    15,
		},
		Config: ConfigSummary{
			DrixyRulesEnabled:   true,
			AgentReviewReposPct: 0,
			Integrations:        []string{"github", "gitlab"},
		},
	}, nil
}

type mockTransport struct {
	mu        sync.Mutex
	disabled  bool
	sendCount int32
	lastPayload map[string]interface{}
	lastVersion string
	returnOK  bool
}

func (m *mockTransport) IsDisabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disabled
}

func (m *mockTransport) Send(_ context.Context, payload map[string]interface{}, version string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt32(&m.sendCount, 1)
	m.lastPayload = payload
	m.lastVersion = version
	return m.returnOK, nil
}

func TestBeaconHTTPProvider_OptOut(t *testing.T) {
	provider := NewBeaconHTTPProvider(nil)

	// Clean env
	_ = os.Unsetenv("SCANDRIX_TELEMETRY_DISABLED")
	_ = os.Unsetenv("DO_NOT_TRACK")
	assert.False(t, provider.IsDisabled())

	// Test variations of SCANDRIX_TELEMETRY_DISABLED
	for _, val := range []string{"1", "true", "True", "YES", "on"} {
		_ = os.Setenv("SCANDRIX_TELEMETRY_DISABLED", val)
		assert.True(t, provider.IsDisabled(), "expected disabled for %s", val)
	}

	_ = os.Unsetenv("SCANDRIX_TELEMETRY_DISABLED")
	_ = os.Setenv("DO_NOT_TRACK", "1")
	assert.True(t, provider.IsDisabled())

	_ = os.Unsetenv("DO_NOT_TRACK")
}

func TestBeaconHTTPProvider_Send(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := NewBeaconHTTPProvider(nil)
	provider.endpointFn = func() string { return server.URL }

	payload := map[string]interface{}{
		"test_key": "test_value",
		"counter":  42,
	}

	ok, err := provider.Send(context.Background(), payload, "2.0.0")
	require.NoError(t, err)
	assert.True(t, ok)

	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "scandrix-self-hosted/2.0.0", receivedHeaders.Get("User-Agent"))

	var decoded map[string]interface{}
	err = json.Unmarshal(receivedBody, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "test_value", decoded["test_key"])
}

func TestHeartbeatCollector_SafeFallback(t *testing.T) {
	collector := NewHeartbeatCollectorService(nil, nil)

	input := CollectInput{
		FirstSeenAt: time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
	}

	metrics, err := collector.Collect(context.Background(), input)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(5), metrics.ScanDrix.UptimeHours)
	assert.Equal(t, "unknown", metrics.Runtime.DBVersion)
	assert.Equal(t, int64(0), metrics.Usage7d.ActiveUsers)
	assert.Equal(t, int64(0), metrics.Usage7d.Organizations)
	assert.Empty(t, metrics.Config.Integrations)
	assert.False(t, metrics.Config.DrixyRulesEnabled)
}

func TestSelfHostedBeaconService_DailyDeduplication(t *testing.T) {
	store := NewInMemoryTelemetryStateStore()
	collector := &mockCollector{}
	transport := &mockTransport{returnOK: true}

	service := NewSelfHostedBeaconService(store, collector, transport, nil)

	ctx := context.Background()

	// First execution should collect and send
	err := service.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&collector.collectCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&transport.sendCount))

	// State should have recorded today's date
	state, err := store.GetState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.NotNil(t, state.LastSentDay)
	today := time.Now().UTC().Format("2006-01-02")
	assert.Equal(t, today, *state.LastSentDay)
	assert.Nil(t, state.InFlightDay)

	// Second execution on the same day should short-circuit
	err = service.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&collector.collectCount), "should not collect again today")
	assert.Equal(t, int32(1), atomic.LoadInt32(&transport.sendCount), "should not send again today")
}

func TestSelfHostedBeaconService_InFlightClaimLock(t *testing.T) {
	store := NewInMemoryTelemetryStateStore()
	collector := &mockCollector{}
	transport := &mockTransport{returnOK: true}

	service := NewSelfHostedBeaconService(store, collector, transport, nil)

	ctx := context.Background()
	today := time.Now().UTC().Format("2006-01-02")

	// Pre-seed an active in-flight claim 5 minutes ago
	recent := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	err := store.SetState(ctx, &TelemetryStateValue{
		InstanceID:        "test-uuid",
		FirstSeenAt:       time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano),
		InFlightDay:       &today,
		InFlightStartedAt: &recent,
	})
	require.NoError(t, err)

	// Run should be locked out by the fresh in-flight claim
	err = service.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&collector.collectCount))
	assert.Equal(t, int32(0), atomic.LoadInt32(&transport.sendCount))

	// Now simulate stale in-flight claim (35 minutes ago)
	stale := time.Now().UTC().Add(-35 * time.Minute).Format(time.RFC3339Nano)
	err = store.SetState(ctx, &TelemetryStateValue{
		InstanceID:        "test-uuid",
		FirstSeenAt:       time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano),
		InFlightDay:       &today,
		InFlightStartedAt: &stale,
	})
	require.NoError(t, err)

	// Stale claim should be bypassed, allowing run to proceed
	err = service.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&collector.collectCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&transport.sendCount))
}

func TestSelfHostedBeaconService_Preview(t *testing.T) {
	store := NewInMemoryTelemetryStateStore()
	collector := &mockCollector{}
	transport := &mockTransport{returnOK: true}

	service := NewSelfHostedBeaconService(store, collector, transport, nil)

	ctx := context.Background()
	preview, err := service.Preview(ctx)
	require.NoError(t, err)
	require.NotNil(t, preview)

	assert.Equal(t, 1, preview["schema_version"])
	assert.NotEmpty(t, preview["instance_id"])
	assert.NotEmpty(t, preview["sent_at"])
	assert.Contains(t, preview, "scandrix")
	assert.Contains(t, preview, "runtime")
	assert.Contains(t, preview, "usage_7d")
	assert.Contains(t, preview, "config")

	// Transport should NOT have been called
	assert.Equal(t, int32(0), atomic.LoadInt32(&transport.sendCount))

	// LastSentDay should NOT be populated
	state, err := store.GetState(ctx)
	require.NoError(t, err)
	assert.Nil(t, state.LastSentDay)
}

func TestBeaconCronJob_Interface(t *testing.T) {
	store := NewInMemoryTelemetryStateStore()
	collector := &mockCollector{}
	transport := &mockTransport{returnOK: true}
	service := NewSelfHostedBeaconService(store, collector, transport, nil)

	job := NewBeaconCronJob(service, nil)
	assert.Equal(t, "self-hosted-beacon", job.Name())
	assert.Equal(t, 24*time.Hour, job.Interval())

	err := job.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&collector.collectCount))
}
