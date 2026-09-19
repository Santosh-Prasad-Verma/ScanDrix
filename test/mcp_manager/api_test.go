// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/scandrix/backend/internal/mcp/manager/api"
	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/custom"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
	"github.com/scandrix/backend/internal/mcp/manager/service"
)

func createTestJWT(jwtSecret, orgID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"organizationId": orgID,
		"exp":            time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(jwtSecret))
	return tokenStr
}

func setupTestRouter(jwtSecret string) http.Handler {
	encryptor, _ := crypto.NewEncryptor("test-secret-key-32-bytes-long-12345")
	repo := repository.NewMCPRepository(nil) // Memory mode for unpersisted catalog routes
	integrationsSvc := service.NewIntegrationsService(repo, encryptor, "http://localhost:3000/callback")

	factory := providers.NewProviderFactory()
	scandrixProvider := scandrixmcp.NewScandrixMCPProvider("http://localhost:3001")
	customProvider := custom.NewCustomProvider(repo, encryptor)

	factory.Register(string(models.ProviderScandrixMCP), scandrixProvider)
	factory.Register(string(models.ProviderCustom), customProvider)

	mcpSvc := service.NewMCPService(repo, factory, integrationsSvc, "http://localhost:3000/callback")
	handler := api.NewMCPHandler(mcpSvc, integrationsSvc)

	return api.NewRouter(api.RouterConfig{
		Handler:      handler,
		JWTSecret:    jwtSecret,
		CORSOrigins:  []string{"*"},
		DocsUser:     "admin",
		DocsPass:     "secret123",
		DocsEnabled:  true,
		DocsPath:     "/docs",
		DocsSpecPath: "/openapi.json",
	})
}

func TestHealthEndpoint(t *testing.T) {
	router := setupTestRouter("my-test-secret")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed decoding health response: %v", err)
	}

	if resp["status"] != "ok" || resp["service"] != "scandrix-mcp-manager" {
		t.Fatalf("Unexpected health response: %v", resp)
	}
}

func TestDocsBasicAuth(t *testing.T) {
	router := setupTestRouter("my-test-secret")

	// 1. Unauthenticated request to /docs should be rejected with 401
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 Unauthorized for /docs without basic auth, got %d", rec.Code)
	}

	// 2. Valid basic auth to /openapi.json should return 200
	reqAuth := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	reqAuth.SetBasicAuth("admin", "secret123")
	recAuth := httptest.NewRecorder()
	router.ServeHTTP(recAuth, reqAuth)

	if recAuth.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for authenticated /openapi.json, got %d", recAuth.Code)
	}
}

func TestAuthGuardTenantIsolation(t *testing.T) {
	jwtSecret := "top-secret-jwt-key-for-testing-12345"
	router := setupTestRouter(jwtSecret)

	// 1. Unauthenticated request to /mcp/integrations should fail with 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/mcp/integrations", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)

	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 for unauthenticated /mcp/integrations, got %d", recUnauth.Code)
	}

	// 2. Request with invalid JWT secret should fail with 401
	badToken := createTestJWT("wrong-secret-key", "org_123")
	reqBad := httptest.NewRequest(http.MethodGet, "/mcp/integrations", nil)
	reqBad.Header.Set("Authorization", fmt.Sprintf("Bearer %s", badToken))
	recBad := httptest.NewRecorder()
	router.ServeHTTP(recBad, reqBad)

	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("Expected HTTP 401 for invalid JWT, got %d", recBad.Code)
	}

	// 3. Valid authenticated request should succeed with 200
	validToken := createTestJWT(jwtSecret, "org_acme_corp")
	reqValid := httptest.NewRequest(http.MethodGet, "/mcp/integrations", nil)
	reqValid.Header.Set("Authorization", fmt.Sprintf("Bearer %s", validToken))
	recValid := httptest.NewRecorder()
	router.ServeHTTP(recValid, reqValid)

	if recValid.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for authenticated /mcp/integrations, got %d: %s", recValid.Code, recValid.Body.String())
	}

	var integrations []models.MCPIntegration
	if err := json.NewDecoder(recValid.Body).Decode(&integrations); err != nil {
		t.Fatalf("Failed decoding integrations: %v", err)
	}

	if len(integrations) == 0 {
		t.Fatalf("Expected catalog integrations, got empty list")
	}

	// Verify catalog items contain canonical capability categories
	hasTaskManagement := false
	hasObservability := false
	for _, it := range integrations {
		if it.Category == "task-management" {
			hasTaskManagement = true
		}
		if it.Category == "observability" {
			hasObservability = true
		}
	}

	if !hasTaskManagement || !hasObservability {
		t.Fatalf("Catalog should contain task-management and observability integrations")
	}
}
