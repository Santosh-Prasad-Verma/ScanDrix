// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package providers

import (
	"context"

	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// Provider abstracts MCP catalog and connectivity operations across backends.
type Provider interface {
	GetIntegrations(ctx context.Context, page, pageSize int, filters map[string]any) ([]models.MCPIntegration, error)
	GetIntegration(ctx context.Context, integrationID, organizationID string) (*models.MCPIntegration, error)
	GetIntegrationRequiredParams(ctx context.Context, integrationID string) ([]models.MCPRequiredParam, error)
	GetIntegrationTools(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error)
	VerifyManagedConnection(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error)
	InitiateConnection(ctx context.Context, orgID string, dto models.InitiateConnectionDTO) (*models.MCPConnectionEntity, error)
	DeleteConnection(ctx context.Context, connectionID string) error
}
