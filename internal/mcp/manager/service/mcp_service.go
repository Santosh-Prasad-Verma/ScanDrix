// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/oauth"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/scandrixmcp"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
)

const IntegrationsCatalogTTL = 5 * time.Minute

type catalogCacheEntry struct {
	expires time.Time
	value   []models.MCPIntegration
}

// MCPService coordinates connections, integrations catalog, and OAuth lifecycles.
type MCPService struct {
	repo                *repository.MCPRepository
	providerFactory     *providers.ProviderFactory
	integrationsService *IntegrationsService
	httpClient          *http.Client
	redirectURI         string

	cacheMu sync.RWMutex
	cache   map[string]catalogCacheEntry
}

// NewMCPService initializes the core MCP manager business service.
func NewMCPService(
	repo *repository.MCPRepository,
	providerFactory *providers.ProviderFactory,
	integrationsService *IntegrationsService,
	redirectURI string,
) *MCPService {
	return &MCPService{
		repo:                repo,
		providerFactory:     providerFactory,
		integrationsService: integrationsService,
		httpClient:          &http.Client{Timeout: 10 * time.Second},
		redirectURI:         redirectURI,
		cache:               make(map[string]catalogCacheEntry),
	}
}

// ═══════════════════════════════════════════════════════════════
// CONNECTIONS
// ═══════════════════════════════════════════════════════════════

// GetConnections retrieves paginated connections for an organization with category stamping.
func (s *MCPService) GetConnections(ctx context.Context, query models.QueryDTO, orgID string) (*models.ConnectionsResponseDTO, error) {
	items, total, err := s.repo.GetConnections(ctx, orgID, query.Page, query.PageSize, query.AppName, query.Provider, query.IntegrationID, query.Status)
	if err != nil {
		return nil, err
	}

	for i := range items {
		if cat, ok := scandrixmcp.ManagedCategoryByID[items[i].IntegrationID]; ok {
			items[i].Category = &cat
		}
	}

	return &models.ConnectionsResponseDTO{
		Items: items,
		Total: total,
	}, nil
}

// GetConnection returns a connection by ID strictly scoped to the organization.
func (s *MCPService) GetConnection(ctx context.Context, connectionID, orgID string) (*models.MCPConnectionEntity, error) {
	conn, err := s.repo.GetConnectionByID(ctx, orgID, connectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection not found")
	}
	if cat, ok := scandrixmcp.ManagedCategoryByID[conn.IntegrationID]; ok {
		conn.Category = &cat
	}
	return conn, nil
}

// UpdateConnection modifies connection status or metadata.
func (s *MCPService) UpdateConnection(ctx context.Context, dto models.UpdateConnectionDTO, orgID string) (*models.MCPConnectionEntity, error) {
	conn, err := s.repo.GetConnectionByIntegrationID(ctx, orgID, dto.IntegrationID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection not found")
	}

	if dto.Status != "" {
		conn.Status = models.MCPConnectionStatus(dto.Status)
	}
	if dto.Metadata != nil {
		if conn.Metadata == nil {
			conn.Metadata = make(map[string]any)
		}
		for k, v := range dto.Metadata {
			conn.Metadata[k] = v
		}
	}

	return s.repo.SaveConnection(ctx, conn)
}

// DeleteConnection disconnects an integration, purging both connection and OAuth credentials.
func (s *MCPService) DeleteConnection(ctx context.Context, ref, orgID string) error {
	conn, _ := s.repo.GetConnectionByIDOrIntegrationID(ctx, orgID, ref)

	integrationID := ref
	if conn != nil {
		integrationID = conn.IntegrationID
	}

	// 1. Purge OAuth / static-token credential
	_ = s.integrationsService.DeleteOAuthState(ctx, orgID, integrationID)

	// 2. Delete connection row if present
	if conn != nil {
		p, err := s.providerFactory.GetProvider(conn.Provider)
		if err == nil {
			_ = p.DeleteConnection(ctx, conn.ID)
		}
		_ = s.repo.DeleteConnection(ctx, orgID, conn.ID)
	}

	return nil
}

// UpdateAllowedTools updates the permitted tool slugs for a connection.
func (s *MCPService) UpdateAllowedTools(ctx context.Context, integrationID string, allowedTools []string, orgID string) (*models.MCPConnectionEntity, error) {
	return s.repo.UpdateAllowedTools(ctx, orgID, integrationID, allowedTools)
}

// ═══════════════════════════════════════════════════════════════
// INTEGRATIONS CATALOG
// ═══════════════════════════════════════════════════════════════

// GetIntegrations returns all integrations across providers, combining with connection status.
func (s *MCPService) GetIntegrations(ctx context.Context, query models.QueryDTO, orgID string) ([]models.MCPIntegration, error) {
	cacheKey := fmt.Sprintf("%s:%d:%d:%s", orgID, query.Page, query.PageSize, query.AppName)

	integrations := s.getCachedCatalog(cacheKey)
	if integrations == nil {
		allProviders := s.providerFactory.GetProviders()
		results := make([]models.MCPIntegration, 0)

		for _, p := range allProviders {
			list, err := p.GetIntegrations(ctx, query.Page, query.PageSize, map[string]any{
				"appName":        query.AppName,
				"organizationId": orgID,
			})
			if err == nil {
				results = append(results, list...)
			}
		}

		integrations = results
		s.setCachedCatalog(cacheKey, integrations)
	}

	// Fetch active connections to determine connection state
	connections, _ := s.repo.ListAllConnectionsForOrg(ctx, orgID)
	connMap := make(map[string]models.MCPConnectionEntity)
	for _, c := range connections {
		connMap[c.IntegrationID] = c
	}

	output := make([]models.MCPIntegration, 0, len(integrations))
	for _, item := range integrations {
		conn, hasConn := connMap[item.ID]
		hasCred := s.integrationsService.HasManagedCredential(ctx, orgID, item.ID)

		isConnected := hasConn || hasCred
		status := models.ConnectionStatusInactive
		if hasConn {
			status = conn.Status
		} else if hasCred {
			status = models.ConnectionStatusActive
		}

		// Defaults for default built-in integration and none-auth items like docs
		if item.IsDefault || (item.Provider == models.ProviderScandrixMCP && item.AuthScheme == string(models.AuthTypeNone)) {
			isConnected = true
			status = models.ConnectionStatusActive
		}

		item.IsConnected = isConnected
		item.ConnectionStatus = &status
		output = append(output, item)
	}

	return output, nil
}

// GetIntegration returns details for a specific integration.
func (s *MCPService) GetIntegration(ctx context.Context, integrationID, providerType, orgID string) (*models.MCPIntegration, error) {
	p, err := s.providerFactory.GetProvider(providerType)
	if err != nil {
		return nil, err
	}

	integration, err := p.GetIntegration(ctx, integrationID, orgID)
	if err != nil {
		return nil, err
	}

	requiredParams, _ := p.GetIntegrationRequiredParams(ctx, integrationID)
	integration.RequiredParams = requiredParams

	conn, _ := s.repo.GetConnectionByIntegrationID(ctx, orgID, integrationID)
	hasCred := s.integrationsService.HasManagedCredential(ctx, orgID, integrationID)

	isConnected := conn != nil || hasCred
	status := models.ConnectionStatusInactive
	if conn != nil {
		status = conn.Status
	} else if hasCred {
		status = models.ConnectionStatusActive
	}

	integration.IsConnected = isConnected
	integration.ConnectionStatus = &status

	return integration, nil
}

// GetIntegrationRequiredParams returns required parameters for configuring an integration.
func (s *MCPService) GetIntegrationRequiredParams(ctx context.Context, integrationID, providerType string) ([]models.MCPRequiredParam, error) {
	p, err := s.providerFactory.GetProvider(providerType)
	if err != nil {
		return nil, err
	}
	return p.GetIntegrationRequiredParams(ctx, integrationID)
}

// GetIntegrationTools discovers tools exposed by an integration.
func (s *MCPService) GetIntegrationTools(ctx context.Context, integrationID, providerType, orgID string) ([]models.MCPTool, error) {
	p, err := s.providerFactory.GetProvider(providerType)
	if err != nil {
		return nil, err
	}
	return p.GetIntegrationTools(ctx, integrationID, orgID)
}

// ═══════════════════════════════════════════════════════════════
// CONNECT & ONBOARDING
// ═══════════════════════════════════════════════════════════════

// InitiateConnection links an integration to an organization connection.
func (s *MCPService) InitiateConnection(ctx context.Context, orgID, providerType string, dto models.InitiateConnectionDTO) (*models.MCPConnectionEntity, error) {
	p, err := s.providerFactory.GetProvider(providerType)
	if err != nil {
		return nil, err
	}

	conn, err := p.InitiateConnection(ctx, orgID, dto)
	if err != nil {
		return nil, err
	}

	existing, _ := s.repo.GetConnectionByIntegrationID(ctx, orgID, dto.IntegrationID)
	if existing != nil {
		conn.ID = existing.ID
	}

	return s.repo.SaveConnection(ctx, conn)
}

// ConnectManagedToken onboard an integration with user-supplied static token (BYOT flow).
func (s *MCPService) ConnectManagedToken(ctx context.Context, orgID, integrationID string, dto models.ConnectTokenDTO) (*models.MCPConnectionEntity, error) {
	p, err := s.providerFactory.GetProvider(string(models.ProviderScandrixMCP))
	if err != nil {
		return nil, err
	}

	managedProvider, ok := p.(*scandrixmcp.ScandrixMCPProvider)
	if !ok {
		return nil, errors.New("invalid managed provider")
	}

	methods, err := managedProvider.GetAuthMethods(integrationID)
	if err != nil {
		return nil, err
	}

	method := scandrixmcp.GetAuthMethod(methods, dto.AuthMethod)
	if method == nil {
		return nil, fmt.Errorf("unknown auth method '%s' for integration %s", dto.AuthMethod, integrationID)
	}

	cred, err := scandrixmcp.ValidateTokenSubmission(method, dto)
	if err != nil {
		return nil, err
	}

	// Persist encrypted credential
	if err := s.integrationsService.SaveTokenCredential(ctx, orgID, integrationID, cred); err != nil {
		return nil, fmt.Errorf("failed persisting token credential: %w", err)
	}

	// Verify the credential actually connects to the remote server
	tools, verifyErr := managedProvider.VerifyManagedConnection(ctx, integrationID, orgID)
	if verifyErr != nil || len(tools) == 0 {
		// Roll back invalid credential
		_ = s.integrationsService.DeleteOAuthState(ctx, orgID, integrationID)
		if verifyErr != nil {
			return nil, fmt.Errorf("could not connect with provided credentials: %w", verifyErr)
		}
		return nil, errors.New("could not connect with provided credentials: no tools returned")
	}

	allowedTools := dto.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = providers.DefaultReadOnlyToolSlugs(tools)
	}

	cfg, _ := managedProvider.GetManagedConfig(integrationID)
	appName := integrationID
	baseURL := ""
	if cfg != nil {
		appName = cfg.Name
		baseURL = cfg.BaseURL
	}

	existing, _ := s.repo.GetConnectionByIntegrationID(ctx, orgID, integrationID)
	conn := &models.MCPConnectionEntity{
		OrganizationID: orgID,
		IntegrationID:  integrationID,
		Provider:       string(models.ProviderScandrixMCP),
		Status:         models.ConnectionStatusActive,
		AppName:        appName,
		MCPURL:         &baseURL,
		AllowedTools:   allowedTools,
		Metadata: map[string]any{
			"authMethod": method.ID,
		},
	}
	if existing != nil {
		conn.ID = existing.ID
	}

	return s.repo.SaveConnection(ctx, conn)
}

// ═══════════════════════════════════════════════════════════════
// OAUTH FLOWS
// ═══════════════════════════════════════════════════════════════

// InitiateOAuthIntegration begins RFC 7636 PKCE OAuth flow for managed or custom provider.
func (s *MCPService) InitiateOAuthIntegration(ctx context.Context, orgID, providerType string, dto models.InitiateOAuthDTO) (string, error) {
	p, err := s.providerFactory.GetProvider(providerType)
	if err != nil {
		return "", err
	}

	var baseURL string
	var scopes []string
	var dynamicReg bool
	var clientID, clientSecret string

	if providerType == string(models.ProviderScandrixMCP) {
		mp := p.(*scandrixmcp.ScandrixMCPProvider)
		cfg, err := mp.GetManagedConfig(dto.IntegrationID)
		if err != nil {
			return "", err
		}
		method := scandrixmcp.GetAuthMethod(cfg.AuthMethods, dto.AuthMethod)
		if method == nil || method.Type != models.AuthTypeOAuth2 {
			return "", errors.New("integration does not support OAuth2")
		}
		baseURL = cfg.BaseURL
		dynamicReg = true
	} else {
		customInt, err := s.repo.GetCustomIntegrationByID(ctx, orgID, dto.IntegrationID, false)
		if err != nil || customInt == nil {
			return "", errors.New("custom integration not found")
		}
		baseURL = customInt.BaseURL
	}

	// 1. Discover OAuth Metadata
	rs, as, err := oauth.DiscoverOAuth(ctx, s.httpClient, baseURL)
	if err != nil {
		return "", fmt.Errorf("oauth discovery failed: %w", err)
	}

	// 2. Dynamic client registration if needed
	redirectURI := s.redirectURI
	if dynamicReg && as.RegistrationEndpoint != "" {
		regID, regSecret, rErr := oauth.RegisterOAuthClient(ctx, s.httpClient, as.RegistrationEndpoint, redirectURI, scopes)
		if rErr == nil {
			clientID = regID
			clientSecret = regSecret
		}
	}

	if clientID == "" {
		return "", errors.New("client_id required and dynamic registration failed or not supported")
	}

	// 3. Generate PKCE & State
	verifier, challenge, err := oauth.GeneratePKCE()
	if err != nil {
		return "", err
	}
	state := oauth.GenerateState()

	// 4. Build authorization URL
	authURL, err := oauth.BuildAuthorizationURL(as.AuthorizationEndpoint, clientID, redirectURI, challenge, state, baseURL, scopes)
	if err != nil {
		return "", err
	}

	// 5. Save pending state
	oauthState := &models.IntegrationOAuthState{
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		OAuthScopes:         scopes,
		DynamicRegistration: dynamicReg,
		ASMetadata:          as,
		RSMetadata:          rs,
		RedirectURI:         redirectURI,
		CodeChallenge:       challenge,
		CodeVerifier:        verifier,
		State:               state,
	}

	if err := s.integrationsService.SaveOAuthState(ctx, orgID, dto.IntegrationID, models.OAuthStatusPending, oauthState); err != nil {
		return "", err
	}

	return authURL, nil
}

// FinalizeOAuthIntegration completes the OAuth authorization code exchange.
func (s *MCPService) FinalizeOAuthIntegration(ctx context.Context, orgID, providerType string, dto models.FinishOAuthDTO) error {
	oauthState, err := s.integrationsService.GetOAuthState(ctx, orgID, dto.IntegrationID)
	if err != nil || oauthState == nil {
		return errors.New("oauth metadata missing for integration")
	}

	if dto.State != oauthState.State {
		return errors.New("invalid oauth state parameter")
	}

	if oauthState.ASMetadata == nil || oauthState.ASMetadata.TokenEndpoint == "" {
		return errors.New("token endpoint missing from oauth metadata")
	}

	tokens, err := oauth.ExchangeCodeForTokens(
		ctx,
		s.httpClient,
		oauthState.ASMetadata.TokenEndpoint,
		oauthState.ClientID,
		oauthState.ClientSecret,
		dto.Code,
		oauthState.CodeVerifier,
		oauthState.RedirectURI,
		"",
	)
	if err != nil {
		return fmt.Errorf("oauth token exchange failed: %w", err)
	}

	oauthState.Tokens = tokens
	if err := s.integrationsService.SaveOAuthState(ctx, orgID, dto.IntegrationID, models.OAuthStatusActive, oauthState); err != nil {
		return err
	}

	// Upsert active connection row
	existing, _ := s.repo.GetConnectionByIntegrationID(ctx, orgID, dto.IntegrationID)
	conn := &models.MCPConnectionEntity{
		OrganizationID: orgID,
		IntegrationID:  dto.IntegrationID,
		Provider:       providerType,
		Status:         models.ConnectionStatusActive,
		AppName:        dto.IntegrationID,
		AllowedTools:   []string{},
		Metadata: map[string]any{
			"oauth": true,
		},
	}
	if existing != nil {
		conn.ID = existing.ID
	}
	_, _ = s.repo.SaveConnection(ctx, conn)

	return nil
}

// ═══════════════════════════════════════════════════════════════
// INTERNAL CONNECTION CONFIG RESOLUTION
// ═══════════════════════════════════════════════════════════════

// GetCustomIntegrationConnectionConfig returns config including decrypted credentials for internal agent runtime.
func (s *MCPService) GetCustomIntegrationConnectionConfig(ctx context.Context, orgID, integrationID string) (*models.MCPIntegrationEntity, error) {
	return s.repo.GetCustomIntegrationByID(ctx, orgID, integrationID, true)
}

// GetScandrixMCPConnectionConfig resolves live auth headers for a managed connection.
func (s *MCPService) GetScandrixMCPConnectionConfig(ctx context.Context, orgID, integrationID string) (map[string]string, error) {
	return s.integrationsService.ResolveManagedAuthHeaders(ctx, orgID, integrationID)
}

// ═══════════════════════════════════════════════════════════════
// IN-MEMORY CATALOG CACHING
// ═══════════════════════════════════════════════════════════════

func (s *MCPService) getCachedCatalog(key string) []models.MCPIntegration {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.value
}

func (s *MCPService) setCachedCatalog(key string, value []models.MCPIntegration) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now := time.Now()
	// Evict expired entries
	for k, v := range s.cache {
		if now.After(v.expires) {
			delete(s.cache, k)
		}
	}

	s.cache[key] = catalogCacheEntry{
		expires: now.Add(IntegrationsCatalogTTL),
		value:   value,
	}
}
