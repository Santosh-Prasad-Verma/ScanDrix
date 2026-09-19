// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/custom"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
	"github.com/scandrix/backend/internal/mcp/manager/service"
)

func TestIntegrationsServiceTokenCredential(t *testing.T) {
	repo := repository.NewMCPRepository(nil)
	secret := generateRandomTestKey()
	enc, err := crypto.NewEncryptor(secret)
	if err != nil {
		t.Fatalf("failed creating encryptor: %v", err)
	}
	integService := service.NewIntegrationsService(repo, enc, "http://localhost:3000/oauth")
	ctx := context.Background()

	// 1. Save and Get token credential on nil pool
	cred := &models.ManagedTokenCredential{
		Kind:         service.ManagedTokenKind,
		AuthMethodID: "token",
		AuthType:     models.AuthTypeBearerToken,
		Secret:       "test-linear-api-key-12345",
	}

	err = integService.SaveTokenCredential(ctx, "org-1", "linear-default", cred)
	if err != nil {
		t.Fatalf("failed saving token credential: %v", err)
	}

	// 2. HasManagedCredential check on nil repo returns false safely
	hasCred := integService.HasManagedCredential(ctx, "org-1", "linear-default")
	if hasCred {
		t.Errorf("expected false on nil repository, got true")
	}

	// 3. ResolveManagedAuthHeaders returns empty headers on nil repo
	headers, err := integService.ResolveManagedAuthHeaders(ctx, "org-1", "linear-default")
	if err != nil {
		t.Fatalf("unexpected error resolving headers: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %v", headers)
	}
}

func TestMCPServiceCatalogCaching(t *testing.T) {
	repo := repository.NewMCPRepository(nil)
	secret := generateRandomTestKey()
	enc, err := crypto.NewEncryptor(secret)
	if err != nil {
		t.Fatalf("failed creating encryptor: %v", err)
	}
	integService := service.NewIntegrationsService(repo, enc, "http://localhost:3000/oauth")

	factory := providers.NewProviderFactory()
	scandrixProv := scandrixmcp.NewScandrixMCPProvider("http://localhost:3001")
	customProv := custom.NewCustomProvider(repo, enc)
	factory.Register(string(models.ProviderScandrixMCP), scandrixProv)
	factory.Register(string(models.ProviderCustom), customProv)

	mcpSvc := service.NewMCPService(repo, factory, integService, "http://localhost:3000/oauth")
	ctx := context.Background()

	// 1. Initial catalog call populates cache
	catalog1, err := mcpSvc.GetIntegrations(ctx, models.QueryDTO{}, "org-1")
	if err != nil {
		t.Fatalf("failed getting available integrations: %v", err)
	}
	if len(catalog1) == 0 {
		t.Fatalf("expected non-empty catalog")
	}

	// 2. Second call returns cached result immediately
	catalog2, err := mcpSvc.GetIntegrations(ctx, models.QueryDTO{}, "org-1")
	if err != nil {
		t.Fatalf("failed getting cached integrations: %v", err)
	}
	if len(catalog2) != len(catalog1) {
		t.Errorf("cached catalog length mismatch: %d vs %d", len(catalog2), len(catalog1))
	}

	// 3. Single integration lookup
	item, err := mcpSvc.GetIntegration(ctx, "linear-default", "scandrixmcp", "org-1")
	if err != nil || item == nil {
		t.Fatalf("expected linear-default integration, got %v, err: %v", item, err)
	}
	if item.Name != "Linear" {
		t.Errorf("expected Linear, got %s", item.Name)
	}
}
