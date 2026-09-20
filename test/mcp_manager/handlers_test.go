// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/mcp/manager/api"
	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/custom"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
	"github.com/scandrix/backend/internal/mcp/manager/service"
)

func setupTestMCPHandler() *api.MCPHandler {
	repo := repository.NewMCPRepository(nil)
	secret := generateRandomTestKey()
	enc, _ := crypto.NewEncryptor(secret)
	integService := service.NewIntegrationsService(repo, enc, "http://localhost:3000/setup/mcp/oauth")

	factory := providers.NewProviderFactory()
	scandrixProv := scandrixmcp.NewScandrixMCPProvider("http://localhost:3001")
	customProv := custom.NewCustomProvider(repo, enc)
	factory.Register(string(models.ProviderScandrixMCP), scandrixProv)
	factory.Register(string(models.ProviderCustom), customProv)

	mcpSvc := service.NewMCPService(repo, factory, integService, "http://localhost:3000/setup/mcp/oauth")

	return api.NewMCPHandler(mcpSvc, integService)
}

func withOrgID(req *http.Request, orgID string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), api.OrgIDContextKey, orgID))
}

func TestHandlerGetSingleIntegration(t *testing.T) {
	h := setupTestMCPHandler()

	r := chi.NewRouter()
	r.Get("/mcp/{provider}/integrations/{integrationId}", h.GetIntegration)

	// 1. Valid ID -> 200
	req := withOrgID(httptest.NewRequest("GET", "/mcp/scandrixmcp/integrations/linear-default", nil), "org-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for linear-default, got %d", rr.Code)
	}

	// 2. Unknown ID -> 404
	req = withOrgID(httptest.NewRequest("GET", "/mcp/scandrixmcp/integrations/unknown-integration-id", nil), "org-1")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown integration, got %d", rr.Code)
	}
}

func TestHandlerCreateConnection(t *testing.T) {
	h := setupTestMCPHandler()

	r := chi.NewRouter()
	r.Post("/mcp/{provider}/connect", h.InitiateConnection)

	// 1. Valid Body -> 200 or 201
	validBody, _ := json.Marshal(models.InitiateConnectionDTO{
		IntegrationID: "scandrix-docs-default",
		AllowedTools:  []string{"lookup"},
	})
	req := withOrgID(httptest.NewRequest("POST", "/mcp/scandrixmcp/connect", bytes.NewReader(validBody)), "org-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Errorf("expected 200/201, got %d body: %s", rr.Code, rr.Body.String())
	}

	// 2. Invalid JSON -> 400
	req = withOrgID(httptest.NewRequest("POST", "/mcp/scandrixmcp/connect", bytes.NewReader([]byte("{invalid-json"))), "org-1")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad json, got %d", rr.Code)
	}

	// 3. Missing IntegrationID -> 400
	missingID, _ := json.Marshal(models.InitiateConnectionDTO{})
	req = withOrgID(httptest.NewRequest("POST", "/mcp/scandrixmcp/connect", bytes.NewReader(missingID)), "org-1")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing integration ID, got %d", rr.Code)
	}
}

func TestHandlerAllowedTools(t *testing.T) {
	h := setupTestMCPHandler()

	r := chi.NewRouter()
	r.Put("/mcp/connections/{integrationId}/allowed-tools", h.UpdateAllowedTools)

	// Invalid JSON -> 400
	req := withOrgID(httptest.NewRequest("PUT", "/mcp/connections/test-id/allowed-tools", bytes.NewReader([]byte("{invalid"))), "org-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad json, got %d", rr.Code)
	}
}

func TestHandlerOAuthEndpoints(t *testing.T) {
	h := setupTestMCPHandler()

	r := chi.NewRouter()
	r.Post("/mcp/integration/{provider}/oauth/initialize", h.InitializeOAuthIntegration)
	r.Post("/mcp/integration/{provider}/oauth/finalize", h.FinalizeOAuthIntegration)

	// 1. OAuth start for integration that does NOT support oauth -> 400
	nonOAuthBody, _ := json.Marshal(models.InitiateOAuthDTO{
		IntegrationID: "scandrix-docs-default",
	})
	req := withOrgID(httptest.NewRequest("POST", "/mcp/integration/scandrixmcp/oauth/initialize", bytes.NewReader(nonOAuthBody)), "org-1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-oauth integration, got %d", rr.Code)
	}

	// 2. OAuth start for unknown integration -> 400
	unknownBody, _ := json.Marshal(models.InitiateOAuthDTO{
		IntegrationID: "unknown-id",
	})
	req = withOrgID(httptest.NewRequest("POST", "/mcp/integration/scandrixmcp/oauth/initialize", bytes.NewReader(unknownBody)), "org-1")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown integration, got %d", rr.Code)
	}

	// 3. OAuth finalize missing code and state -> 400
	emptyBody, _ := json.Marshal(models.FinishOAuthDTO{})
	req = withOrgID(httptest.NewRequest("POST", "/mcp/integration/scandrixmcp/oauth/finalize", bytes.NewReader(emptyBody)), "org-1")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing params, got %d", rr.Code)
	}
}
