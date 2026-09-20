// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

func TestConnectionSerialization(t *testing.T) {
	now := time.Now().UTC()
	mcpURL := "https://mcp.linear.app/mcp"
	conn := models.MCPConnectionEntity{
		ID:             "550e8400-e29b-41d4-a716-446655440000",
		OrganizationID: "org-123",
		IntegrationID:  "linear-default",
		Provider:       string(models.ProviderScandrixMCP),
		Status:         models.ConnectionStatusActive,
		AppName:        "Linear",
		MCPURL:         &mcpURL,
		AllowedTools:   []string{"read_issue", "search_issues"},
		Metadata: map[string]any{
			"installedBy": "user-456",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(conn)
	if err != nil {
		t.Fatalf("failed marshaling MCPConnectionEntity: %v", err)
	}

	var parsed models.MCPConnectionEntity
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed unmarshaling MCPConnectionEntity: %v", err)
	}

	if parsed.ID != conn.ID || parsed.OrganizationID != conn.OrganizationID {
		t.Errorf("expected ID %s and Org %s, got %s and %s", conn.ID, conn.OrganizationID, parsed.ID, parsed.OrganizationID)
	}
	if parsed.Provider != string(models.ProviderScandrixMCP) {
		t.Errorf("expected provider %s, got %s", models.ProviderScandrixMCP, parsed.Provider)
	}
	if len(parsed.AllowedTools) != 2 || parsed.AllowedTools[0] != "read_issue" {
		t.Errorf("allowed tools not parsed properly: %v", parsed.AllowedTools)
	}
}

func TestCustomIntegrationSerialization(t *testing.T) {
	desc := "Internal microservice MCP"
	encAuth := "encrypted-secret"
	headers := `{"X-Custom-Header":"encrypted-value"}`

	integ := models.MCPIntegrationEntity{
		ID:             "cust-123",
		Active:         true,
		OrganizationID: "org-xyz",
		Protocol:       models.ProtocolSSE,
		BaseURL:        "https://example.com/sse",
		Name:           "Internal AI Helper",
		Description:    &desc,
		AuthType:       models.AuthTypeBearerToken,
		Auth:           &encAuth,
		Headers:        &headers,
	}

	data, err := json.Marshal(integ)
	if err != nil {
		t.Fatalf("failed marshaling MCPIntegrationEntity: %v", err)
	}

	var parsed models.MCPIntegrationEntity
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed unmarshaling MCPIntegrationEntity: %v", err)
	}

	if parsed.Protocol != models.ProtocolSSE {
		t.Errorf("expected protocol %s, got %s", models.ProtocolSSE, parsed.Protocol)
	}
	if parsed.AuthType != models.AuthTypeBearerToken {
		t.Errorf("expected authType %s, got %s", models.AuthTypeBearerToken, parsed.AuthType)
	}
}

func TestOAuthEntitySerialization(t *testing.T) {
	encAuth := `{"encryptedToken":"abc"}`
	oauth := models.MCPIntegrationOAuthEntity{
		ID:             "oauth-uuid-1",
		Status:         models.OAuthStatusActive,
		OrganizationID: "org-alpha",
		IntegrationID:  "sentry-default",
		Auth:           &encAuth,
	}

	data, err := json.Marshal(oauth)
	if err != nil {
		t.Fatalf("failed marshaling MCPIntegrationOAuthEntity: %v", err)
	}

	var parsed models.MCPIntegrationOAuthEntity
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed unmarshaling MCPIntegrationOAuthEntity: %v", err)
	}

	if parsed.Status != models.OAuthStatusActive {
		t.Errorf("expected status %s, got %s", models.OAuthStatusActive, parsed.Status)
	}
}

func TestDTOValidation(t *testing.T) {
	req := models.InitiateConnectionDTO{
		IntegrationID: "scandrix-docs-default",
		AllowedTools:  []string{"lookup_docs"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed marshaling InitiateConnectionDTO: %v", err)
	}

	var parsed models.InitiateConnectionDTO
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed unmarshaling InitiateConnectionDTO: %v", err)
	}

	if parsed.IntegrationID != "scandrix-docs-default" {
		t.Errorf("expected scandrix-docs-default, got %s", parsed.IntegrationID)
	}
}
