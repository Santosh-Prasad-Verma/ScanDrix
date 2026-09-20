// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package models

import (
	"time"
)

// MCPIntegrationProtocol defines the wire protocol for an MCP integration.
type MCPIntegrationProtocol string

const (
	ProtocolHTTP  MCPIntegrationProtocol = "http"
	ProtocolSSE   MCPIntegrationProtocol = "sse"
	ProtocolStdio MCPIntegrationProtocol = "stdio"
)

// MCPIntegrationAuthType defines the authentication scheme for connecting to an MCP integration.
type MCPIntegrationAuthType string

const (
	AuthTypeNone        MCPIntegrationAuthType = "none"
	AuthTypeBearerToken MCPIntegrationAuthType = "bearer_token"
	AuthTypeAPIKey      MCPIntegrationAuthType = "api_key"
	AuthTypeBasic       MCPIntegrationAuthType = "basic"
	AuthTypeOAuth2      MCPIntegrationAuthType = "oauth2"
)

// MCPProviderType defines the provider backend.
type MCPProviderType string

const (
	ProviderScandrixMCP MCPProviderType = "scandrixmcp"
	ProviderCustom      MCPProviderType = "custom"
)

// MCPIntegrationEntity maps to "mcp-manager"."mcp_integrations" in PostgreSQL.
type MCPIntegrationEntity struct {
	ID             string                 `json:"id"`
	Active         bool                   `json:"active"`
	OrganizationID string                 `json:"organizationId"`
	Protocol       MCPIntegrationProtocol `json:"protocol"`
	BaseURL        string                 `json:"baseUrl"`
	Name           string                 `json:"name"`
	Description    *string                `json:"description,omitempty"`
	LogoURL        *string                `json:"logoUrl,omitempty"`
	AuthType       MCPIntegrationAuthType `json:"authType"`
	Auth           *string                `json:"-"` // Encrypted ciphertext
	Headers        *string                `json:"-"` // Encrypted ciphertext
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
	DeletedAt      *time.Time             `json:"deletedAt,omitempty"`
}

// MCPTool represents an invokable capability of an MCP server.
type MCPTool struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Provider    MCPProviderType `json:"provider"`
	Warning     bool            `json:"warning"`
	ReadOnly    bool            `json:"readOnly"`
	InputSchema map[string]any  `json:"inputSchema,omitempty"`
}

// MCPIntegration represents a high-level integration descriptor across providers.
type MCPIntegration struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	AuthScheme       string                 `json:"authScheme"`
	AppName          string                 `json:"appName"`
	Logo             string                 `json:"logo"`
	Provider         MCPProviderType        `json:"provider"`
	IsConnected      bool                   `json:"isConnected"`
	ConnectionStatus *MCPConnectionStatus   `json:"connectionStatus,omitempty"`
	IsDefault        bool                   `json:"isDefault,omitempty"`
	Active           bool                   `json:"active,omitempty"`
	BaseURL          string                 `json:"baseUrl,omitempty"`
	Protocol         MCPIntegrationProtocol `json:"protocol,omitempty"`
	AllowedTools     []string               `json:"allowedTools,omitempty"`
	AuthMethods      []PublicAuthMethod     `json:"authMethods,omitempty"`
	RequiredParams   []MCPRequiredParam     `json:"requiredParams,omitempty"`
	Category         string                 `json:"category,omitempty"`
}

// MCPRequiredParam represents a parameter needed to configure an integration.
type MCPRequiredParam struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Description string `json:"description,omitempty"`
}

// PublicAuthMethod represents an exposed auth method in the integrations catalog.
type PublicAuthMethod struct {
	ID         string                 `json:"id"`
	Label      string                 `json:"label,omitempty"`
	Type       MCPIntegrationAuthType `json:"type"`
	Default    bool                   `json:"default,omitempty"`
	UserFields []ManagedAuthUserField `json:"userFields,omitempty"`
}

// ManagedAuthUserField represents a form field required from the user during BYOT onboarding.
type ManagedAuthUserField struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Required bool   `json:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
}
