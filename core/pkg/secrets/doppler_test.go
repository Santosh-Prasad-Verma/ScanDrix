package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDopplerGetSecretWithMockServer(t *testing.T) {
	// Setup mock Doppler server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dp.st.mocktoken123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/configs/config/secret" {
			name := r.URL.Query().Get("name")
			if name == "DATABASE_URL" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "DATABASE_URL",
					"value": map[string]string{
						"raw":      "postgresql://user:pass@localhost:5433/db",
						"computed": "postgresql://user:pass@localhost:5433/db",
					},
				})
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewCustomDopplerClient("dp.st.mocktoken123", "codehound", "dev", server.URL, 1*time.Minute)

	ctx := context.Background()
	secret, err := client.GetSecret(ctx, "DATABASE_URL")
	if err != nil {
		t.Fatalf("expected secret, got error: %v", err)
	}

	expected := "postgresql://user:pass@localhost:5433/db"
	if secret != expected {
		t.Errorf("expected '%s', got '%s'", expected, secret)
	}

	// Verify caching by calling again
	secretCached, err := client.GetSecret(ctx, "DATABASE_URL")
	if err != nil || secretCached != expected {
		t.Errorf("expected cached secret '%s', got '%s'", expected, secretCached)
	}
}

func TestDopplerFallbackToEnvironment(t *testing.T) {
	// When token is empty, it should fall back to os.Getenv
	_ = os.Setenv("TEST_LOCAL_SECRET", "super_secret_value_123")
	defer os.Unsetenv("TEST_LOCAL_SECRET")

	client := NewCustomDopplerClient("", "codehound", "dev", "", 1*time.Minute)

	ctx := context.Background()
	val, err := client.GetSecret(ctx, "TEST_LOCAL_SECRET")
	if err != nil {
		t.Fatalf("expected fallback to env, got error: %v", err)
	}

	if val != "super_secret_value_123" {
		t.Errorf("expected 'super_secret_value_123', got '%s'", val)
	}
}

func TestDopplerGetAllSecretsWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/configs/config/secrets" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"secrets": map[string]map[string]string{
					"DB_HOST": {
						"raw":      "localhost",
						"computed": "localhost",
					},
					"DB_PORT": {
						"raw":      "5433",
						"computed": "5433",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewCustomDopplerClient("dp.st.test", "codehound", "dev", server.URL, 1*time.Minute)

	ctx := context.Background()
	allSecrets, err := client.GetAllSecrets(ctx)
	if err != nil {
		t.Fatalf("failed to fetch all secrets: %v", err)
	}

	if len(allSecrets) != 2 {
		t.Errorf("expected 2 secrets, got %d", len(allSecrets))
	}
	if allSecrets["DB_HOST"] != "localhost" || allSecrets["DB_PORT"] != "5433" {
		t.Errorf("unexpected secret values: %+v", allSecrets)
	}
}
