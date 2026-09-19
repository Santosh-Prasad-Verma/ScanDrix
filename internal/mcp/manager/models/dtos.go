// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package models

// QueryDTO provides pagination and filtering options for MCP endpoints.
type QueryDTO struct {
	Page          int    `json:"page"`
	PageSize      int    `json:"pageSize"`
	Provider      string `json:"provider,omitempty"`
	AppName       string `json:"appName,omitempty"`
	IntegrationID string `json:"integrationId,omitempty"`
	Status        string `json:"status,omitempty"`
}

// InitiateConnectionDTO defines payload for initiating an integration connection.
type InitiateConnectionDTO struct {
	IntegrationID string         `json:"integrationId"`
	AuthParams    map[string]any `json:"authParams,omitempty"`
	AllowedTools  []string       `json:"allowedTools,omitempty"`
}

// UpdateConnectionDTO defines payload for updating an existing connection.
type UpdateConnectionDTO struct {
	IntegrationID string         `json:"integrationId"`
	Status        string         `json:"status"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// UpdateAllowedToolsDTO defines payload for modifying permitted tool slugs.
type UpdateAllowedToolsDTO struct {
	AllowedTools []string `json:"allowedTools"`
}

// StringRecordDTO represents a key-value header pair.
type StringRecordDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CreateIntegrationDTO defines payload for registering a custom or managed integration.
type CreateIntegrationDTO struct {
	IntegrationID       string                 `json:"integrationId,omitempty"`
	BaseURL             string                 `json:"baseUrl"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	LogoURL             string                 `json:"logoUrl,omitempty"`
	Protocol            MCPIntegrationProtocol `json:"protocol"`
	AuthType            MCPIntegrationAuthType `json:"authType"`
	BearerToken         string                 `json:"bearerToken,omitempty"`
	APIKey              string                 `json:"apiKey,omitempty"`
	APIKeyHeader        string                 `json:"apiKeyHeader,omitempty"`
	BasicUser           string                 `json:"basicUser,omitempty"`
	BasicPassword       string                 `json:"basicPassword,omitempty"`
	ClientID            string                 `json:"clientId,omitempty"`
	ClientSecret        string                 `json:"clientSecret,omitempty"`
	OAuthScopes         []string               `json:"oauthScopes,omitempty"`
	DynamicRegistration bool                   `json:"dynamicRegistration,omitempty"`
	Headers             []StringRecordDTO      `json:"headers,omitempty"`
}

// ConnectTokenDTO defines payload for the Bring-Your-Own-Token onboarding flow.
type ConnectTokenDTO struct {
	AuthMethod   string            `json:"authMethod"`
	Secret       string            `json:"secret"`
	Fields       map[string]string `json:"fields,omitempty"`
	AllowedTools []string          `json:"allowedTools,omitempty"`
}

// InitiateOAuthDTO defines payload for beginning an OAuth flow.
type InitiateOAuthDTO struct {
	IntegrationID string `json:"integrationId"`
	AuthMethod    string `json:"authMethod,omitempty"`
}

// FinishOAuthDTO defines payload for exchanging an OAuth callback authorization code.
type FinishOAuthDTO struct {
	IntegrationID string `json:"integrationId"`
	Code          string `json:"code"`
	State         string `json:"state"`
}

// ConnectionsResponseDTO wraps paginated connection listings.
type ConnectionsResponseDTO struct {
	Items []MCPConnectionEntity `json:"items"`
	Total int64                 `json:"total"`
}

// MessageResponseDTO represents a generic status confirmation response.
type MessageResponseDTO struct {
	Message    string         `json:"message"`
	Connection map[string]any `json:"connection,omitempty"`
}

// OAuthInitResponseDTO returns the constructed OAuth authorization URL.
type OAuthInitResponseDTO struct {
	AuthURL string `json:"authUrl"`
}

// ErrorResponseDTO provides a standard RFC 7807-compliant error format.
type ErrorResponseDTO struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
}
