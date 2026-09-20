// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package product

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostHogClient_CaptureAndIdentify(t *testing.T) {
	var capturedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/capture", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPostHogClientWithConfig("test-key", server.URL, server.Client())
	assert.True(t, client.IsEnabled())

	ctx := context.Background()
	err := client.Capture(ctx, "user-123", "test_event", map[string]any{"source": "cli"}, map[string]string{"organization": "org-456"})
	require.NoError(t, err)

	assert.Equal(t, "test_event", capturedPayload["event"])
	assert.Equal(t, "user-123", capturedPayload["distinct_id"])
	props := capturedPayload["properties"].(map[string]any)
	assert.Equal(t, "cli", props["source"])
	groups := props["$groups"].(map[string]any)
	assert.Equal(t, "org-456", groups["organization"])
}

func TestResendClient_Send(t *testing.T) {
	var resendBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer resend-key-123", r.Header.Get("Authorization"))
		_ = json.NewDecoder(r.Body).Decode(&resendBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewResendClientWithConfig("resend-key-123", server.URL, server.Client())
	assert.True(t, client.IsEnabled())

	err := client.Send(context.Background(), "user.signed_up", "user@example.com", map[string]any{"name": "Alice"})
	require.NoError(t, err)

	assert.Equal(t, "user.signed_up", resendBody["event"])
	assert.Equal(t, "user@example.com", resendBody["email"])
}

func TestN8nClient_RetryOnFailure(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewN8nClientWithConfig(server.URL, server.Client())
	assert.True(t, client.IsEnabled())

	err := client.Notify(context.Background(), "first_review.completed", map[string]any{"pr": 12})
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

type mockPostHog struct {
	capturedEvents []string
	identified     []string
	fail           bool
}

func (m *mockPostHog) IsEnabled() bool { return true }
func (m *mockPostHog) Capture(ctx context.Context, distinctID string, event string, properties map[string]any, groups map[string]string) error {
	if m.fail {
		return errors.New("posthog down")
	}
	m.capturedEvents = append(m.capturedEvents, event)
	return nil
}
func (m *mockPostHog) Identify(ctx context.Context, distinctID string, properties map[string]any) error {
	if m.fail {
		return errors.New("posthog down")
	}
	m.identified = append(m.identified, distinctID)
	return nil
}
func (m *mockPostHog) GroupIdentify(ctx context.Context, groupType string, groupKey string, properties map[string]any) error {
	if m.fail {
		return errors.New("posthog down")
	}
	return nil
}
func (m *mockPostHog) IsFeatureEnabled(ctx context.Context, featureName string, identifier string, orgTeamData any, evalCtx *FeatureEvaluationContext) (bool, error) {
	if m.fail {
		return false, errors.New("posthog down")
	}
	return true, nil
}

type mockResend struct {
	sentEvents []string
	fail       bool
}

func (m *mockResend) IsEnabled() bool { return true }
func (m *mockResend) Send(ctx context.Context, event string, email string, payload map[string]any) error {
	if m.fail {
		return errors.New("resend down")
	}
	m.sentEvents = append(m.sentEvents, event)
	return nil
}

type mockN8n struct {
	notifiedEvents []string
	fail           bool
}

func (m *mockN8n) IsEnabled() bool { return true }
func (m *mockN8n) Notify(ctx context.Context, eventID string, props map[string]any) error {
	if m.fail {
		return errors.New("n8n down")
	}
	m.notifiedEvents = append(m.notifiedEvents, eventID)
	return nil
}

func TestTelemetryService_FlowAndFailSafeSwallowing(t *testing.T) {
	ctx := context.Background()

	// 1. Success Flow
	posthog := &mockPostHog{}
	resend := &mockResend{}
	n8n := &mockN8n{}
	svc := NewTelemetryService(posthog, resend, n8n, nil)

	svc.UserSignedUp(ctx, UserSignedUpParams{
		UserID:           "u1",
		Email:            "u1@scandrix.dev",
		OrganizationID:   "org1",
		OrganizationName: "ScanDrix Inc",
		TeamID:           "team1",
	})

	assert.Contains(t, posthog.identified, "u1")
	assert.Contains(t, posthog.capturedEvents, "user_signed_up")
	assert.Contains(t, resend.sentEvents, "user.signed_up")
	assert.Contains(t, n8n.notifiedEvents, "user.signed_up")

	// 2. Failure Flow — Telemetry must NEVER throw or panic (fail-safe)
	failingPosthog := &mockPostHog{fail: true}
	failingResend := &mockResend{fail: true}
	failingN8n := &mockN8n{fail: true}
	failingSvc := NewTelemetryService(failingPosthog, failingResend, failingN8n, nil)

	// None of these should panic or return error
	failingSvc.UserSignedUp(ctx, UserSignedUpParams{UserID: "u2", Email: "u2@scandrix.dev", OrganizationID: "org2"})
	failingSvc.OnboardingCompleted(ctx, OnboardingCompletedParams{UserID: "u2", OrganizationID: "org2", TeamID: "t2", ReviewedPR: true})
	failingSvc.FirstReviewCompleted(ctx, FirstReviewCompletedParams{OrganizationID: "org2", PullRequestNumber: 1})
}

func TestPostHogClient_IsFeatureEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/decide?v=3", r.URL.RequestURI())
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		assert.Equal(t, "user-abc", reqBody["distinct_id"])

		resp := map[string]any{
			"featureFlags": map[string]any{
				"heavy-review": true,
				"beta-test":    false,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewPostHogClientWithConfig("key-123", server.URL, server.Client())
	ctx := context.Background()

	// 1. Enabled flag
	enabled, err := client.IsFeatureEnabled(ctx, "heavy-review", "user-abc", map[string]any{"organizationId": "org-1"}, &FeatureEvaluationContext{
		Groups: map[string]string{"team": "team-1"},
	})
	require.NoError(t, err)
	assert.True(t, enabled)

	// 2. Disabled flag
	enabled, err = client.IsFeatureEnabled(ctx, "beta-test", "user-abc", nil, nil)
	require.NoError(t, err)
	assert.False(t, enabled)

	// 3. Non-existent flag
	enabled, err = client.IsFeatureEnabled(ctx, "unknown-flag", "user-abc", nil, nil)
	require.NoError(t, err)
	assert.False(t, enabled)

	// 4. Disabled client (no api key)
	disabledClient := NewPostHogClientWithConfig("", server.URL, server.Client())
	enabled, err = disabledClient.IsFeatureEnabled(ctx, "heavy-review", "user-abc", nil, nil)
	require.NoError(t, err)
	assert.True(t, enabled) // Permissive fallback
}

