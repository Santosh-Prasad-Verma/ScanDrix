// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package models

import (
	"time"
)

// MCPIntegrationOAuthStatus represents the authorization status of an OAuth/token integration.
type MCPIntegrationOAuthStatus string

const (
	OAuthStatusActive   MCPIntegrationOAuthStatus = "ACTIVE"
	OAuthStatusInactive MCPIntegrationOAuthStatus = "INACTIVE"
	OAuthStatusPending  MCPIntegrationOAuthStatus = "PENDING"
	OAuthStatusFailed   MCPIntegrationOAuthStatus = "FAILED"
)

// MCPIntegrationOAuthEntity maps to "mcp-manager"."mcp_integration_oauth" in PostgreSQL.
type MCPIntegrationOAuthEntity struct {
	ID             string                    `json:"id"`
	Status         MCPIntegrationOAuthStatus `json:"status"`
	OrganizationID string                    `json:"organizationId"`
	IntegrationID  string                    `json:"integrationId"`
	Auth           *string                   `json:"-"` // Encrypted ciphertext
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
}

// ManagedTokenCredential stores a bring-your-own-token credential.
type ManagedTokenCredential struct {
	Kind         string                 `json:"kind"` // "managed-token"
	AuthMethodID string                 `json:"authMethodId"`
	AuthType     MCPIntegrationAuthType `json:"authType"`
	Secret       string                 `json:"secret"`
	Fields       map[string]string      `json:"fields,omitempty"`
}

// OAuthTokenData represents exchanged tokens returned by an OAuth 2.0 token endpoint.
type OAuthTokenData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"` // Unix timestamp in seconds or ms
	TokenType    string `json:"tokenType,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthAuthorizationServerMetadata represents RFC 8414 metadata.
type OAuthAuthorizationServerMetadata struct {
	Issuer                string   `json:"issuer,omitempty"`
	AuthorizationEndpoint string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string   `json:"token_endpoint,omitempty"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported []string `json:"response_types_supported,omitempty"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`
}

// OAuthProtectedResourceMetadata represents resource server discovery metadata.
type OAuthProtectedResourceMetadata struct {
	Resource             string   `json:"resource,omitempty"`
	AuthorizationServers []string `json:"authorization_servers,omitempty"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

// IntegrationOAuthState captures the transient or active OAuth state for an integration.
type IntegrationOAuthState struct {
	ClientID            string                            `json:"clientId,omitempty"`
	ClientSecret        string                            `json:"clientSecret,omitempty"`
	OAuthScopes         []string                          `json:"oauthScopes,omitempty"`
	DynamicRegistration bool                              `json:"dynamicRegistration,omitempty"`
	ASMetadata          *OAuthAuthorizationServerMetadata `json:"asMetadata,omitempty"`
	RSMetadata          *OAuthProtectedResourceMetadata   `json:"rsMetadata,omitempty"`
	RedirectURI         string                            `json:"redirectUri,omitempty"`
	CodeChallenge       string                            `json:"codeChallenge,omitempty"`
	CodeVerifier        string                            `json:"codeVerifier,omitempty"`
	State               string                            `json:"state,omitempty"`
	Tokens              *OAuthTokenData                   `json:"tokens,omitempty"`
}
