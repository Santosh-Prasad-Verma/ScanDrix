package zoho

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveZohoConnection(t *testing.T) {
	if os.Getenv("RUN_LIVE_TEST") == "" {
		t.Skip("skipping live Zoho CRM test; set RUN_LIVE_TEST=1 to run")
	}
	_ = godotenv.Load("../../../.env")
	_ = godotenv.Load(".env")
	client := NewClientFromEnv()
	require.True(t, client.IsEnabled())

	id, err := client.UpsertLead(context.Background(), Lead{
		FirstName:   "ScanDrix",
		LastName:    "Test Lead",
		Email:       "test-integration@scandrix.dev",
		Company:     "ScanDrix AI",
		LeadStatus:  "New",
		Description: "Live integration test from ScanDrix Backend",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	t.Logf("SUCCESS! Created Lead in Zoho CRM with ID: %s", id)
}

func TestZohoClient_IsEnabled(t *testing.T) {
	client := NewClient(Config{})
	assert.False(t, client.IsEnabled())

	clientConfigured := NewClient(Config{
		ClientID:     "mock_client_id",
		ClientSecret: "mock_client_secret",
		RefreshToken: "mock_refresh_token",
	})
	assert.True(t, clientConfigured.IsEnabled())
}

func TestZohoClient_UpsertLead(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/oauth/v2/token", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		_ = r.ParseForm()
		assert.Equal(t, "mock_refresh_token", r.Form.Get("refresh_token"))
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "zoho_test_access_token_123",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/crm/v6/Leads/upsert", r.URL.Path)
		assert.Equal(t, "Zoho-oauthtoken zoho_test_access_token_123", r.Header.Get("Authorization"))

		var body map[string]any
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		data, ok := body["data"].([]any)
		require.True(t, ok)
		require.Len(t, data, 1)

		leadData, ok := data[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "dev@example.com", leadData["Email"])
		assert.Equal(t, "John Doe", leadData["Last_Name"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"code":    "SUCCESS",
					"message": "record added",
					"details": map[string]any{
						"id": "zoho_lead_987654321",
					},
				},
			},
		})
	}))
	defer apiServer.Close()

	client := NewClient(Config{
		ClientID:     "mock_client_id",
		ClientSecret: "mock_client_secret",
		RefreshToken: "mock_refresh_token",
		AccountsURL:  tokenServer.URL,
		APIDomain:    apiServer.URL,
		LeadSource:   "Test Suite",
	})

	id, err := client.UpsertLead(context.Background(), Lead{
		Email:    "dev@example.com",
		LastName: "John Doe",
	})

	require.NoError(t, err)
	assert.Equal(t, "zoho_lead_987654321", id)
}
