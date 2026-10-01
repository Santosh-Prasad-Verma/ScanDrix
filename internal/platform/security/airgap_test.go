package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockEgressReporter struct {
	blockedURLs []string
}

func (m *mockEgressReporter) ReportBlockedEgress(ctx context.Context, targetURL string, reason string) {
	m.blockedURLs = append(m.blockedURLs, targetURL)
}

func TestAirGapGate_AllowsLocalWhenAirGapped(t *testing.T) {
	reporter := &mockEgressReporter{blockedURLs: make([]string, 0)}
	gate := NewAirGapGate(AirGapConfig{
		Enabled:  true,
		Reporter: reporter,
	})

	// Local mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("local response"))
	}))
	defer server.Close()

	client := gate.WrapClient(server.Client())

	// Request to localhost must succeed
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	assert.NoError(t, err)

	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, reporter.blockedURLs)
}

func TestAirGapGate_BlocksExternalWhenAirGapped(t *testing.T) {
	reporter := &mockEgressReporter{blockedURLs: make([]string, 0)}
	gate := NewAirGapGate(AirGapConfig{
		Enabled:  true,
		Reporter: reporter,
	})

	client := gate.WrapClient(&http.Client{})

	// Request to public cloud LLM must be blocked fail-closed
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.openai.com/v1/chat/completions", nil)
	assert.NoError(t, err)

	resp, err := client.Do(req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrAirGapEgressBlocked)

	assert.Len(t, reporter.blockedURLs, 1)
	assert.Contains(t, reporter.blockedURLs[0], "api.openai.com")
}

func TestAirGapGate_PermitsConfiguredPrivateCIDRs(t *testing.T) {
	gate := NewAirGapGate(AirGapConfig{
		Enabled:      true,
		AllowedHosts: []string{"10.0.0.0/8", "custom-model-server.internal"},
	})

	assert.NoError(t, gate.CheckAddressValid("http://10.2.3.4:8080/v1/chat"))
	assert.NoError(t, gate.CheckAddressValid("http://custom-model-server.internal/v1/models"))
	assert.Error(t, gate.CheckAddressValid("https://api.anthropic.com/v1/messages"))
}

func TestAirGapGate_DisabledPermitsAll(t *testing.T) {
	gate := NewAirGapGate(AirGapConfig{
		Enabled: false,
	})

	assert.False(t, gate.IsAirGapped())
	assert.NoError(t, gate.CheckAddressValid("https://api.openai.com/v1/chat/completions"))
}
