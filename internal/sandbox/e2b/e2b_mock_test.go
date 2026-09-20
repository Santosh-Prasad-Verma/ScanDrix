package e2b_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/e2b"
	"github.com/scandrix/backend/pkg/models"
)

func TestE2BProviderWithMockServer(t *testing.T) {
	var createdSandboxID string
	var deletedSandboxID string

	// Create mock E2B API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API Key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "e2b_valid_key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body["templateID"] != "scandrix-sandbox" {
				t.Errorf("expected templateID scandrix-sandbox, got %v", body["templateID"])
			}

			createdSandboxID = "sbx-998877"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sandboxID":       createdSandboxID,
				"domain":          "e2b.dev",
				"envdAccessToken": "token-test-secret",
				"envdVersion":     "0.1.0",
			})
			return
		}

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/commands") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stdout":    "e2b remote output",
				"stderr":    "",
				"exit_code": 0,
			})
			return
		}

		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sandboxes/") {
			deletedSandboxID = strings.TrimPrefix(r.URL.Path, "/sandboxes/")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	provider := e2b.NewE2BProvider(e2b.Config{
		APIKey:     "e2b_valid_key",
		Endpoint:   server.URL,
		TemplateID: "scandrix-sandbox",
		HTTPClient: server.Client(),
	})

	if !provider.IsAvailable() {
		t.Fatal("expected provider with key to be available")
	}

	inst, err := provider.CreateSandboxWithRepo(ctx, contracts.CreateSandboxParams{
		CloneURL:   "https://github.com/example/repo.git",
		Branch:     "main",
		Platform:   models.ProviderGitHub,
		AuthToken:  "ghp_dummy123",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateSandboxWithRepo failed: %v", err)
	}

	if createdSandboxID != "sbx-998877" {
		t.Errorf("expected sandboxID sbx-998877 to be created, got %s", createdSandboxID)
	}

	if inst.GetTier() != contracts.TierMicroVM {
		t.Errorf("expected TierMicroVM, got %s", inst.GetTier())
	}

	// Test remote command execution
	res, err := inst.Run(ctx, "echo 'hello from e2b'", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("Run command failed: %v", err)
	}
	if res.ExitCode != 0 || res.Stdout != "e2b remote output" {
		t.Errorf("unexpected run result: %+v", res)
	}

	// Test cleanup
	err = inst.Cleanup(ctx)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if deletedSandboxID != "sbx-998877" {
		t.Errorf("expected sandbox %s to be deleted via DELETE /sandboxes/{id}, got %s", createdSandboxID, deletedSandboxID)
	}
}
