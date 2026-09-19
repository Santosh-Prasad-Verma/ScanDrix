// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
)

func TestRepositoryNilPoolSafety(t *testing.T) {
	repo := repository.NewMCPRepository(nil)
	ctx := context.Background()

	// 1. GetConnections
	conns, total, err := repo.GetConnections(ctx, "org-1", 0, 0, "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error on nil pool GetConnections: %v", err)
	}
	if len(conns) != 0 || total != 0 {
		t.Errorf("expected empty conns and 0 total, got %d and %d", len(conns), total)
	}

	// 2. GetConnectionByID
	conn, err := repo.GetConnectionByID(ctx, "org-1", "some-uuid")
	if err != nil || conn != nil {
		t.Errorf("expected nil conn and nil error, got %v and %v", conn, err)
	}

	// 3. GetConnectionByIntegrationID
	conn, err = repo.GetConnectionByIntegrationID(ctx, "org-1", "linear-default")
	if err != nil || conn != nil {
		t.Errorf("expected nil conn and nil error, got %v and %v", conn, err)
	}

	// 4. ListAllConnectionsForOrg
	list, err := repo.ListAllConnectionsForOrg(ctx, "org-1")
	if err != nil || len(list) != 0 {
		t.Errorf("expected empty list, got len %d, err %v", len(list), err)
	}

	// 5. SaveConnection
	sample := &models.MCPConnectionEntity{OrganizationID: "org-1", IntegrationID: "int-1"}
	saved, err := repo.SaveConnection(ctx, sample)
	if err != nil || saved != sample {
		t.Errorf("expected saved sample, got %v, err %v", saved, err)
	}

	// 6. DeleteConnection
	if err := repo.DeleteConnection(ctx, "org-1", "some-uuid"); err != nil {
		t.Errorf("expected nil error on DeleteConnection, got %v", err)
	}

	// 7. Custom Integrations
	customs, err := repo.GetCustomIntegrations(ctx, "org-1", true)
	if err != nil || len(customs) != 0 {
		t.Errorf("expected empty custom list, got %v, err %v", customs, err)
	}

	custom, err := repo.GetCustomIntegrationByID(ctx, "org-1", "id-1", false)
	if err != nil || custom != nil {
		t.Errorf("expected nil custom, got %v, err %v", custom, err)
	}

	custEntity := &models.MCPIntegrationEntity{OrganizationID: "org-1", Name: "custom"}
	savedCust, err := repo.SaveCustomIntegration(ctx, custEntity)
	if err != nil || savedCust != custEntity {
		t.Errorf("expected saved custom entity, got %v, err %v", savedCust, err)
	}

	if err := repo.DeleteCustomIntegration(ctx, "org-1", "id-1"); err != nil {
		t.Errorf("expected nil error on DeleteCustomIntegration, got %v", err)
	}

	// 8. OAuth
	oauth, err := repo.GetOAuthEntity(ctx, "org-1", "int-1")
	if err != nil || oauth != nil {
		t.Errorf("expected nil oauth entity, got %v, err %v", oauth, err)
	}

	if err := repo.SaveOAuthEntity(ctx, &models.MCPIntegrationOAuthEntity{}); err != nil {
		t.Errorf("expected nil error on SaveOAuthEntity, got %v", err)
	}

	if err := repo.DeleteOAuthEntity(ctx, "org-1", "int-1"); err != nil {
		t.Errorf("expected nil error on DeleteOAuthEntity, got %v", err)
	}
}

func TestRepositoryIDOrIntegrationIDRouting(t *testing.T) {
	repo := repository.NewMCPRepository(nil)
	ctx := context.Background()

	// UUID format should route without panic
	conn, err := repo.GetConnectionByIDOrIntegrationID(ctx, "org-1", "550e8400-e29b-41d4-a716-446655440000")
	if err != nil || conn != nil {
		t.Errorf("expected nil, got %v, err: %v", conn, err)
	}

	// Slug format should route without panic
	conn, err = repo.GetConnectionByIDOrIntegrationID(ctx, "org-1", "scandrix-docs-default")
	if err != nil || conn != nil {
		t.Errorf("expected nil, got %v, err: %v", conn, err)
	}
}
