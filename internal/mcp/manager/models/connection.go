// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package models

import (
	"time"
)

// MCPConnectionStatus represents the lifecycle state of an MCP connection.
type MCPConnectionStatus string

const (
	ConnectionStatusActive   MCPConnectionStatus = "ACTIVE"
	ConnectionStatusInactive MCPConnectionStatus = "INACTIVE"
	ConnectionStatusExpired  MCPConnectionStatus = "EXPIRED"
	ConnectionStatusPending  MCPConnectionStatus = "PENDING"
	ConnectionStatusFailed   MCPConnectionStatus = "FAILED"
)

// MCPConnectionEntity maps directly to "mcp-manager"."mcp_connections" table in PostgreSQL.
type MCPConnectionEntity struct {
	ID             string              `json:"id"`
	OrganizationID string              `json:"organizationId"`
	IntegrationID  string              `json:"integrationId"`
	Provider       string              `json:"provider"`
	Status         MCPConnectionStatus `json:"status"`
	AppName        string              `json:"appName"`
	MCPURL         *string             `json:"mcpUrl,omitempty"`
	AllowedTools   []string            `json:"allowedTools"`
	Metadata       map[string]any      `json:"metadata,omitempty"`
	Category       *string             `json:"category,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	DeletedAt      *time.Time          `json:"deletedAt,omitempty"`
}
