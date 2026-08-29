package settings_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/usecases/settings"
)

func TestSSRFFirewallAndRegionGuards(t *testing.T) {
	// 1. HTTP is rejected (must be HTTPS)
	err := settings.AssertSafeEndpoint("http://api.openai.com/v1")
	if err == nil {
		t.Fatal("expected non-HTTPS URL to be rejected")
	}

	// 2. Loopback/Private IP rejected
	err = settings.AssertSafeEndpoint("https://127.0.0.1:8080/v1")
	if err == nil {
		t.Fatal("expected 127.0.0.1 loopback IP to be rejected")
	}

	err = settings.AssertSafeEndpoint("https://169.254.169.254/latest/meta-data")
	if err == nil {
		t.Fatal("expected AWS metadata link-local IP to be rejected")
	}

	// 3. Region regex guard
	if err := settings.AssertSafeRegion("us-east-1"); err != nil {
		t.Fatalf("valid region rejected: %v", err)
	}
	if err := settings.AssertSafeRegion("evil.com/?"); err == nil {
		t.Fatal("expected malicious region to be rejected")
	}
}

func TestModelOverrideHierarchy(t *testing.T) {
	mgr := settings.NewModelOverrideManager()
	wsID := uuid.New()
	defaultModel := "claude-3-5-sonnet-20241022"

	// 1. Without overrides -> uses default
	m := mgr.ResolveModel(wsID, "acme/backend", "cmd/api", defaultModel)
	if m != defaultModel {
		t.Fatalf("expected default %s, got %s", defaultModel, m)
	}

	// 2. Add Global Override
	mgr.SetOverride(settings.ModelOverride{
		Scope:       settings.ScopeGlobal,
		WorkspaceID: wsID,
		ModelID:     "gpt-4o",
	})
	m = mgr.ResolveModel(wsID, "acme/backend", "cmd/api", defaultModel)
	if m != "gpt-4o" {
		t.Fatalf("expected global override gpt-4o, got %s", m)
	}

	// 3. Add Repository Override -> Overrides Global
	mgr.SetOverride(settings.ModelOverride{
		Scope:          settings.ScopeRepository,
		WorkspaceID:    wsID,
		RepositoryPath: "acme/backend",
		ModelID:        "claude-3-7-sonnet",
	})
	m = mgr.ResolveModel(wsID, "acme/backend", "cmd/api", defaultModel)
	if m != "claude-3-7-sonnet" {
		t.Fatalf("expected repo override claude-3-7-sonnet, got %s", m)
	}

	// 4. Add Directory Override -> Overrides Repository
	mgr.SetOverride(settings.ModelOverride{
		Scope:          settings.ScopeDirectory,
		WorkspaceID:    wsID,
		RepositoryPath: "acme/backend",
		DirectoryPath:  "cmd/api",
		ModelID:        "o3-mini",
	})
	m = mgr.ResolveModel(wsID, "acme/backend", "cmd/api/main.go", defaultModel)
	if m != "o3-mini" {
		t.Fatalf("expected dir override o3-mini, got %s", m)
	}

	// Another directory in same repo still gets repo override
	mOther := mgr.ResolveModel(wsID, "acme/backend", "pkg/auth", defaultModel)
	if mOther != "claude-3-7-sonnet" {
		t.Fatalf("expected repo override claude-3-7-sonnet for other dir, got %s", mOther)
	}
}

func TestBotFilterService(t *testing.T) {
	botSvc := settings.NewBotFilterService()
	wsID := uuid.New()

	// Default known bots
	if !botSvc.IsBotAuthor(wsID, "dependabot[bot]") {
		t.Fatal("expected dependabot[bot] to be recognized as bot")
	}
	if !botSvc.IsBotAuthor(wsID, "renovate") {
		t.Fatal("expected renovate to be recognized as bot")
	}
	if botSvc.IsBotAuthor(wsID, "alice_developer") {
		t.Fatal("alice_developer should not be classified as bot")
	}

	// Custom bot configuration
	botSvc.SetConfig(settings.BotIgnoreConfig{
		WorkspaceID:     wsID,
		IgnoreKnownBots: true,
		CustomBotNames:  []string{"internal-ci-runner"},
		IgnoredUserIDs:  []string{"service-account-42"},
	})

	if !botSvc.IsBotAuthor(wsID, "internal-ci-runner") {
		t.Fatal("expected custom bot internal-ci-runner to be recognized")
	}
	if !botSvc.IsBotAuthor(wsID, "service-account-42") {
		t.Fatal("expected ignored user service-account-42 to be recognized")
	}
}

func TestBYOKConnectionTesterMock(t *testing.T) {
	// Mock OpenAI API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-sk-test" {
			http.Error(w, `{"error":"invalid_api_key"}`, http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer mockServer.Close()

	tester := settings.NewBYOKConnectionTester(5 * time.Second)
	ctx := context.Background()

	// 1. Missing API Key
	resNoKey := tester.TestConnection(ctx, settings.ByokTestInput{
		Provider: "openai",
		APIKey:   "",
	})
	if resNoKey.Success || resNoKey.Code != settings.ByokResultAuth {
		t.Fatalf("expected auth error for empty key, got %+v", resNoKey)
	}

	// 2. Invalid Region
	resBadRegion := tester.TestConnection(ctx, settings.ByokTestInput{
		Provider: "openai",
		APIKey:   "valid-sk-test",
		Region:   "bad/region/path",
	})
	if resBadRegion.Success || resBadRegion.Code != settings.ByokResultBadRequest {
		t.Fatalf("expected bad request for malicious region, got %+v", resBadRegion)
	}
}
