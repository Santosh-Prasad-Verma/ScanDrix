// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/oauth"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
)

const ManagedTokenKind = "managed-token"

// IntegrationsService manages custom integrations and OAuth credentials.
type IntegrationsService struct {
	repo        *repository.MCPRepository
	encryptor   *crypto.Encryptor
	httpClient  *http.Client
	redirectURI string
}

// NewIntegrationsService initializes integration and OAuth credential services.
func NewIntegrationsService(repo *repository.MCPRepository, encryptor *crypto.Encryptor, redirectURI string) *IntegrationsService {
	return &IntegrationsService{
		repo:        repo,
		encryptor:   encryptor,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		redirectURI: redirectURI,
	}
}

// CreateCustomIntegration validates and persists a new custom MCP integration.
func (s *IntegrationsService) CreateCustomIntegration(ctx context.Context, orgID string, dto models.CreateIntegrationDTO) (*models.MCPIntegrationEntity, error) {
	if dto.Name == "" || dto.BaseURL == "" || dto.Protocol == "" || dto.AuthType == "" {
		return nil, errors.New("name, baseUrl, protocol, and authType are required")
	}

	authPayload := make(map[string]any)
	switch dto.AuthType {
	case models.AuthTypeBearerToken:
		if dto.BearerToken == "" {
			return nil, errors.New("bearerToken is required for BEARER_TOKEN auth type")
		}
		authPayload["bearerToken"] = dto.BearerToken
	case models.AuthTypeAPIKey:
		if dto.APIKey == "" {
			return nil, errors.New("apiKey is required for API_KEY auth type")
		}
		authPayload["apiKey"] = dto.APIKey
		authPayload["apiKeyHeader"] = dto.APIKeyHeader
	case models.AuthTypeBasic:
		if dto.BasicUser == "" {
			return nil, errors.New("basicUser is required for BASIC auth type")
		}
		authPayload["basicUser"] = dto.BasicUser
		authPayload["basicPassword"] = dto.BasicPassword
	case models.AuthTypeOAuth2:
		authPayload["clientId"] = dto.ClientID
		authPayload["clientSecret"] = dto.ClientSecret
		authPayload["oauthScopes"] = dto.OAuthScopes
		authPayload["dynamicRegistration"] = dto.DynamicRegistration
	case models.AuthTypeNone:
		// No credentials needed
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", dto.AuthType)
	}

	authBytes, _ := json.Marshal(authPayload)
	encryptedAuth, err := s.encryptor.Encrypt(string(authBytes))
	if err != nil {
		return nil, fmt.Errorf("failed encrypting auth credentials: %w", err)
	}

	headersMap := make(map[string]string)
	for _, h := range dto.Headers {
		if h.Key != "" {
			headersMap[h.Key] = h.Value
		}
	}
	headersBytes, _ := json.Marshal(headersMap)
	encryptedHeaders, err := s.encryptor.Encrypt(string(headersBytes))
	if err != nil {
		return nil, fmt.Errorf("failed encrypting custom headers: %w", err)
	}

	entity := &models.MCPIntegrationEntity{
		Active:         true,
		OrganizationID: orgID,
		Protocol:       dto.Protocol,
		BaseURL:        dto.BaseURL,
		Name:           dto.Name,
		Description:    &dto.Description,
		LogoURL:        &dto.LogoURL,
		AuthType:       dto.AuthType,
		Auth:           &encryptedAuth,
		Headers:        &encryptedHeaders,
	}

	return s.repo.SaveCustomIntegration(ctx, entity)
}

// EditCustomIntegration updates an existing custom integration.
func (s *IntegrationsService) EditCustomIntegration(ctx context.Context, orgID, integrationID string, dto models.CreateIntegrationDTO) (*models.MCPIntegrationEntity, error) {
	existing, err := s.repo.GetCustomIntegrationByID(ctx, orgID, integrationID, false)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("integration not found")
	}

	if dto.Name != "" {
		existing.Name = dto.Name
	}
	if dto.BaseURL != "" {
		existing.BaseURL = dto.BaseURL
	}
	if dto.Protocol != "" {
		existing.Protocol = dto.Protocol
	}
	if dto.Description != "" {
		existing.Description = &dto.Description
	}
	if dto.LogoURL != "" {
		existing.LogoURL = &dto.LogoURL
	}

	return s.repo.SaveCustomIntegration(ctx, existing)
}

// DeleteCustomIntegration deletes a custom integration if no active connections exist.
func (s *IntegrationsService) DeleteCustomIntegration(ctx context.Context, orgID, integrationID string) error {
	conn, _ := s.repo.GetConnectionByIntegrationID(ctx, orgID, integrationID)
	if conn != nil {
		return errors.New("cannot delete integration with active connections; please disconnect it first")
	}
	return s.repo.DeleteCustomIntegration(ctx, orgID, integrationID)
}

// SaveTokenCredential saves an encrypted Bring-Your-Own-Token credential in mcp_integration_oauth.
func (s *IntegrationsService) SaveTokenCredential(ctx context.Context, orgID, integrationID string, cred *models.ManagedTokenCredential) error {
	payload, err := json.Marshal(cred)
	if err != nil {
		return err
	}

	encrypted, err := s.encryptor.Encrypt(string(payload))
	if err != nil {
		return err
	}

	entity := &models.MCPIntegrationOAuthEntity{
		Status:         models.OAuthStatusActive,
		OrganizationID: orgID,
		IntegrationID:  integrationID,
		Auth:           &encrypted,
	}
	return s.repo.SaveOAuthEntity(ctx, entity)
}

// GetManagedCredential decrypts and reads a static token credential if present.
func (s *IntegrationsService) GetManagedCredential(ctx context.Context, orgID, integrationID string) (*models.ManagedTokenCredential, error) {
	entity, err := s.repo.GetOAuthEntity(ctx, orgID, integrationID)
	if err != nil || entity == nil || entity.Auth == nil {
		return nil, err
	}

	decrypted, err := s.encryptor.Decrypt(*entity.Auth)
	if err != nil {
		return nil, err
	}

	var cred models.ManagedTokenCredential
	if err := json.Unmarshal([]byte(decrypted), &cred); err != nil {
		return nil, err
	}

	if cred.Kind != ManagedTokenKind {
		return nil, nil // Not a static token (likely OAuth state)
	}

	return &cred, nil
}

// SaveOAuthState stores encrypted OAuth metadata in mcp_integration_oauth.
func (s *IntegrationsService) SaveOAuthState(ctx context.Context, orgID, integrationID string, status models.MCPIntegrationOAuthStatus, state *models.IntegrationOAuthState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	encrypted, err := s.encryptor.Encrypt(string(payload))
	if err != nil {
		return err
	}

	entity := &models.MCPIntegrationOAuthEntity{
		Status:         status,
		OrganizationID: orgID,
		IntegrationID:  integrationID,
		Auth:           &encrypted,
	}
	return s.repo.SaveOAuthEntity(ctx, entity)
}

// GetOAuthState retrieves and decrypts OAuth state for an integration.
func (s *IntegrationsService) GetOAuthState(ctx context.Context, orgID, integrationID string) (*models.IntegrationOAuthState, error) {
	entity, err := s.repo.GetOAuthEntity(ctx, orgID, integrationID)
	if err != nil || entity == nil || entity.Auth == nil {
		return nil, err
	}

	decrypted, err := s.encryptor.Decrypt(*entity.Auth)
	if err != nil {
		return nil, err
	}

	var state models.IntegrationOAuthState
	if err := json.Unmarshal([]byte(decrypted), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// RefreshOAuthStateIfNeeded refreshes access token if expired or nearing expiry within 60s.
func (s *IntegrationsService) RefreshOAuthStateIfNeeded(ctx context.Context, orgID, integrationID string, state *models.IntegrationOAuthState) (*models.IntegrationOAuthState, error) {
	if state == nil || state.Tokens == nil || state.Tokens.RefreshToken == "" || state.ASMetadata == nil {
		return state, nil
	}

	now := time.Now().Unix()
	// If token expires in more than 60 seconds, no refresh required
	if state.Tokens.ExpiresAt > now+60 {
		return state, nil
	}

	tokenEndpoint := state.ASMetadata.TokenEndpoint
	newTokens, err := oauth.RefreshToken(ctx, s.httpClient, tokenEndpoint, state.ClientID, state.ClientSecret, state.Tokens.RefreshToken)
	if err != nil {
		return state, err
	}

	state.Tokens = newTokens
	_ = s.SaveOAuthState(ctx, orgID, integrationID, models.OAuthStatusActive, state)
	return state, nil
}

// ResolveManagedAuthHeaders resolves authorization headers ready for agent runtime invocation.
func (s *IntegrationsService) ResolveManagedAuthHeaders(ctx context.Context, orgID, integrationID string) (map[string]string, error) {
	// 1. Check static token credential
	cred, err := s.GetManagedCredential(ctx, orgID, integrationID)
	if err == nil && cred != nil {
		switch cred.AuthType {
		case models.AuthTypeBearerToken:
			return map[string]string{"Authorization": fmt.Sprintf("Bearer %s", cred.Secret)}, nil
		case models.AuthTypeBasic:
			user := cred.Fields["email"]
			if user == "" {
				user = cred.Fields["user"]
			}
			encoded := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, cred.Secret)))
			return map[string]string{"Authorization": fmt.Sprintf("Basic %s", encoded)}, nil
		case models.AuthTypeAPIKey:
			header := cred.Fields["apiKeyHeader"]
			if header == "" {
				header = "X-Api-Key"
			}
			return map[string]string{header: cred.Secret}, nil
		}
	}

	// 2. Check OAuth state
	oauthState, err := s.GetOAuthState(ctx, orgID, integrationID)
	if err == nil && oauthState != nil && oauthState.Tokens != nil {
		oauthState, _ = s.RefreshOAuthStateIfNeeded(ctx, orgID, integrationID, oauthState)
		if oauthState.Tokens.AccessToken != "" {
			return map[string]string{"Authorization": fmt.Sprintf("Bearer %s", oauthState.Tokens.AccessToken)}, nil
		}
	}

	return map[string]string{}, nil
}

// HasManagedCredential verifies if an organization has configured credentials for an integration.
func (s *IntegrationsService) HasManagedCredential(ctx context.Context, orgID, integrationID string) bool {
	cred, _ := s.GetManagedCredential(ctx, orgID, integrationID)
	if cred != nil {
		return true
	}
	entity, _ := s.repo.GetOAuthEntity(ctx, orgID, integrationID)
	return entity != nil && entity.Status == models.OAuthStatusActive
}

// DeleteOAuthState purges credentials and state.
func (s *IntegrationsService) DeleteOAuthState(ctx context.Context, orgID, integrationID string) error {
	return s.repo.DeleteOAuthEntity(ctx, orgID, integrationID)
}
