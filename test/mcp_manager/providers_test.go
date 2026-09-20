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
)

func TestProviderFactory(t *testing.T) {
	factory := providers.NewProviderFactory()
	scandrixProv := scandrixmcp.NewScandrixMCPProvider("http://localhost:3001")
	factory.Register(string(models.ProviderScandrixMCP), scandrixProv)

	// Direct lookup
	p1, err := factory.GetProvider("scandrixmcp")
	if err != nil || p1 == nil {
		t.Fatalf("expected scandrixmcp provider, got %v, err: %v", p1, err)
	}

	// Case-insensitive lookup
	p2, err := factory.GetProvider("SCANDRIXMCP")
	if err != nil || p2 == nil {
		t.Fatalf("expected case-insensitive scandrixmcp, got %v, err: %v", p2, err)
	}

	// Unknown lookup
	_, err = factory.GetProvider("unknown_provider")
	if err == nil {
		t.Fatalf("expected error for unknown provider, got nil")
	}
}

func TestScandrixMCPProviderCatalog(t *testing.T) {
	prov := scandrixmcp.NewScandrixMCPProvider("https://api.scandrix.io")
	ctx := context.Background()

	integrations, err := prov.GetIntegrations(ctx, 1, 50, map[string]any{"organizationId": "org-1"})
	if err != nil {
		t.Fatalf("failed listing integrations: %v", err)
	}

	if len(integrations) == 0 {
		t.Fatalf("expected non-empty integration list")
	}

	// Verify linear-default
	linear, err := prov.GetIntegration(ctx, "linear-default", "org-1")
	if err != nil || linear == nil {
		t.Fatalf("expected linear-default integration, got %v, err: %v", linear, err)
	}

	if linear.Name != "Linear" {
		t.Errorf("expected Linear, got %s", linear.Name)
	}

	// Verify tools call succeeds (degrades gracefully if remote server unreachable)
	tools, err := prov.GetIntegrationTools(ctx, "linear-default", "org-1")
	if err != nil {
		t.Fatalf("failed getting tools: %v", err)
	}
	if tools == nil {
		t.Errorf("expected non-nil tools slice")
	}

	// Verify Non-existent
	_, err = prov.GetIntegration(ctx, "non-existent", "org-1")
	if err == nil {
		t.Fatalf("expected error for non-existent integration, got nil")
	}
}

func TestCustomProvider(t *testing.T) {
	repo := repository.NewMCPRepository(nil)
	cryptoKey := generateRandomTestKey()
	enc, err := crypto.NewEncryptor(cryptoKey)
	if err != nil {
		t.Fatalf("failed creating encryptor: %v", err)
	}
	customProv := custom.NewCustomProvider(repo, enc)
	ctx := context.Background()

	// List on nil pool returns empty list safely
	list, err := customProv.GetIntegrations(ctx, 1, 50, map[string]any{"organizationId": "org-1"})
	if err != nil {
		t.Fatalf("unexpected error listing custom integrations: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 custom integrations on nil repo, got %d", len(list))
	}

	// Missing organizationId returns error
	_, err = customProv.GetIntegrations(ctx, 1, 50, map[string]any{})
	if err == nil {
		t.Fatalf("expected error for missing organizationId, got nil")
	}
}
