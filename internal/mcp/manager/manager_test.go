// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package manager_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/api"
	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/oauth"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/providers/services"
)

func TestEncryption(t *testing.T) {
	enc, err := crypto.NewEncryptor("test-enterprise-secure-secret-key-12345")
	if err != nil {
		t.Fatalf("failed initializing encryptor: %v", err)
	}

	plaintext := "sk_live_very_secret_token_value_abc"
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("expected plaintext '%s', got '%s'", plaintext, decrypted)
	}
}

func TestPKCEAndState(t *testing.T) {
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}
	if len(verifier) == 0 || len(challenge) == 0 {
		t.Fatalf("empty verifier or challenge")
	}

	state := oauth.GenerateState()
	if len(state) != 32 { // 16 bytes hex-encoded
		t.Fatalf("unexpected state length: %d", len(state))
	}
}

func TestCanonicalResourceURI(t *testing.T) {
	raw := "HTTPS://API.EXA.AI:443/mcp/"
	canonical, err := oauth.GetCanonicalResourceURI(raw)
	if err != nil {
		t.Fatalf("failed canonicalizing: %v", err)
	}
	if canonical != "https://api.exa.ai:443/mcp" {
		t.Fatalf("unexpected canonical URI: %s", canonical)
	}
}

func TestIntegrationDescriptionService(t *testing.T) {
	svc := services.GetIntegrationDescriptionService()
	desc := svc.GetDescription("scandrixmcp", "sentry-default")
	if len(desc) == 0 {
		t.Fatalf("expected curated description for sentry-default, got empty")
	}

	fallback := svc.GetDescription("custom", "jira")
	if fallback != "Integration with Jira for automation and task management." {
		t.Fatalf("unexpected fallback description: %s", fallback)
	}
}

func TestDefaultScandrixMCPIntegrationAndTools(t *testing.T) {
	primary := scandrixmcp.DefaultScandrixMCPIntegration()
	if primary.ID != scandrixmcp.DefaultPrimaryIntegrationID {
		t.Fatalf("unexpected primary ID: %s", primary.ID)
	}
	if !primary.IsDefault || !primary.IsConnected {
		t.Fatalf("primary integration should be default and connected")
	}

	tools := scandrixmcp.DefaultScandrixToolsCatalog()
	if len(tools) != 23 {
		t.Fatalf("expected 23 native tools in default catalog, got %d", len(tools))
	}

	foundRules := false
	for _, tool := range tools {
		if tool.Slug == "SCANDRIX_GET_DRIXY_RULES" {
			foundRules = true
			break
		}
	}
	if !foundRules {
		t.Fatalf("did not find SCANDRIX_GET_DRIXY_RULES in tool catalog")
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	api.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp api.HealthResponseDTO
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding health response: %v", err)
	}

	if resp.Status != "ok" {
		t.Fatalf("expected status 'ok', got '%s'", resp.Status)
	}
	if resp.Database != "disconnected" { // nil pool passed
		t.Fatalf("expected db status 'disconnected', got '%s'", resp.Database)
	}
	if resp.Memory.Total == 0 {
		t.Fatalf("expected non-zero memory total")
	}
}

func TestTokenValidation(t *testing.T) {
	method := &models.PublicAuthMethod{
		ID:   "token",
		Type: models.AuthTypeBearerToken,
		UserFields: []models.ManagedAuthUserField{
			{Name: "apiKey", Required: true, Secret: true},
		},
	}

	dto := models.ConnectTokenDTO{
		AuthMethod: "token",
		Secret:     "token_12345",
	}

	cred, err := scandrixmcp.ValidateTokenSubmission(method, dto)
	if err != nil {
		t.Fatalf("ValidateTokenSubmission failed: %v", err)
	}
	if cred.Secret != "token_12345" {
		t.Fatalf("expected secret token_12345, got %s", cred.Secret)
	}
}

func TestScandrixMCPProviderListing(t *testing.T) {
	provider := scandrixmcp.NewScandrixMCPProvider("http://localhost:3001")
	integrations, err := provider.GetIntegrations(context.Background(), 1, 50, nil)
	if err != nil {
		t.Fatalf("GetIntegrations failed: %v", err)
	}

	if len(integrations) == 0 {
		t.Fatalf("expected integrations, got 0")
	}

	// First entry must be primary built-in ScanDrix MCP
	if !integrations[0].IsDefault {
		t.Fatalf("first integration must be the default built-in ScanDrix MCP")
	}
	if integrations[0].Name != "ScanDrix MCP" {
		t.Fatalf("expected name ScanDrix MCP, got %s", integrations[0].Name)
	}

	// Tools query for default primary integration
	tools, err := provider.GetIntegrationTools(context.Background(), scandrixmcp.DefaultPrimaryIntegrationID, "org_123")
	if err != nil {
		t.Fatalf("GetIntegrationTools failed: %v", err)
	}
	if len(tools) != 23 {
		t.Fatalf("expected 23 tools for primary integration, got %d", len(tools))
	}
}
