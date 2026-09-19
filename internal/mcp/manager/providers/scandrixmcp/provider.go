// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scandrixmcp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/client"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/services"
)

// ScandrixMCPProvider manages built-in and enterprise cloud integrations.
type ScandrixMCPProvider struct {
	mu                  sync.RWMutex
	managedIntegrations map[string]RawManagedConfig
	serverBaseURL       string
}

// NewScandrixMCPProvider initializes the provider with default catalog and resolved URLs.
func NewScandrixMCPProvider(serverBaseURL string) *ScandrixMCPProvider {
	if serverBaseURL == "" {
		serverBaseURL = os.Getenv("SCANDRIX_SERVER_URL")
		if serverBaseURL == "" {
			serverBaseURL = os.Getenv("API_SCANDRIX_MCP_SERVER_URL")
		}
		if serverBaseURL == "" {
			serverBaseURL = os.Getenv("SCANDRIX_MCP_SERVER_URL")
		}
		if serverBaseURL == "" {
			serverBaseURL = "http://localhost:3001"
		}
	}

	p := &ScandrixMCPProvider{
		managedIntegrations: make(map[string]RawManagedConfig),
		serverBaseURL:       strings.TrimRight(serverBaseURL, "/"),
	}

	for _, cfg := range DefaultManagedServersCatalog() {
		cfg.AuthMethods = NormalizeAuthMethods(cfg)
		cfg.BaseURL = p.resolveBaseURL(cfg.BaseURL)
		p.managedIntegrations[cfg.ID] = cfg
	}

	return p
}

func (p *ScandrixMCPProvider) resolveBaseURL(rawURL string) string {
	if !strings.HasPrefix(rawURL, "/") {
		return rawURL
	}
	u, err := url.Parse(p.serverBaseURL)
	if err != nil {
		return fmt.Sprintf("%s%s", p.serverBaseURL, rawURL)
	}
	origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	return fmt.Sprintf("%s%s", origin, rawURL)
}

// GetManagedConfig returns static descriptor for an integration ID.
func (p *ScandrixMCPProvider) GetManagedConfig(integrationID string) (*RawManagedConfig, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cfg, ok := p.managedIntegrations[integrationID]
	if !ok {
		return nil, fmt.Errorf("managed integration '%s' not found", integrationID)
	}
	return &cfg, nil
}

// GetAuthMethods returns all configured auth methods for an integration.
func (p *ScandrixMCPProvider) GetAuthMethods(integrationID string) ([]models.PublicAuthMethod, error) {
	cfg, err := p.GetManagedConfig(integrationID)
	if err != nil {
		return nil, err
	}
	return cfg.AuthMethods, nil
}

// GetIntegrations returns all managed integrations including the primary built-in ScanDrix MCP.
func (p *ScandrixMCPProvider) GetIntegrations(ctx context.Context, page, pageSize int, filters map[string]any) ([]models.MCPIntegration, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	descService := services.GetIntegrationDescriptionService()
	integrations := make([]models.MCPIntegration, 0, len(p.managedIntegrations)+1)

	// Built-in default ScanDrix MCP integration at root
	integrations = append(integrations, DefaultScandrixMCPIntegration())

	for _, cfg := range p.managedIntegrations {
		category := ManagedCategoryByID[cfg.ID]
		desc := descService.GetDescription("scandrixmcp", cfg.ID)
		if desc == "" {
			desc = fmt.Sprintf("Enterprise %s integration for automated code review context.", cfg.Name)
		}

		integrations = append(integrations, models.MCPIntegration{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Description: desc,
			AuthScheme:  string(cfg.AuthMethods[0].Type),
			AppName:     cfg.Name,
			Logo:        cfg.LogoURL,
			Provider:    models.ProviderScandrixMCP,
			BaseURL:     cfg.BaseURL,
			Protocol:    cfg.Protocol,
			AuthMethods: cfg.AuthMethods,
			Category:    category,
		})
	}

	return integrations, nil
}

// GetIntegration returns a single integration descriptor.
func (p *ScandrixMCPProvider) GetIntegration(ctx context.Context, integrationID, organizationID string) (*models.MCPIntegration, error) {
	if integrationID == DefaultPrimaryIntegrationID || integrationID == "scandrix-mcp-default" {
		primary := DefaultScandrixMCPIntegration()
		return &primary, nil
	}

	cfg, err := p.GetManagedConfig(integrationID)
	if err != nil {
		return nil, err
	}

	descService := services.GetIntegrationDescriptionService()
	desc := descService.GetDescription("scandrixmcp", cfg.ID)
	if desc == "" {
		desc = fmt.Sprintf("Enterprise %s integration for automated code review context.", cfg.Name)
	}

	category := ManagedCategoryByID[cfg.ID]
	return &models.MCPIntegration{
		ID:          cfg.ID,
		Name:        cfg.Name,
		Description: desc,
		AuthScheme:  string(cfg.AuthMethods[0].Type),
		AppName:     cfg.Name,
		Logo:        cfg.LogoURL,
		Provider:    models.ProviderScandrixMCP,
		BaseURL:     cfg.BaseURL,
		Protocol:    cfg.Protocol,
		AuthMethods: cfg.AuthMethods,
		Category:    category,
	}, nil
}

// GetIntegrationRequiredParams returns required parameters for an integration.
func (p *ScandrixMCPProvider) GetIntegrationRequiredParams(ctx context.Context, integrationID string) ([]models.MCPRequiredParam, error) {
	methods, err := p.GetAuthMethods(integrationID)
	if err != nil {
		return nil, err
	}

	params := make([]models.MCPRequiredParam, 0)
	for _, m := range methods {
		for _, f := range m.UserFields {
			params = append(params, models.MCPRequiredParam{
				Key:      f.Name,
				Label:    f.Label,
				Required: f.Required,
				Secret:   f.Secret,
			})
		}
	}
	return params, nil
}

// BuildClient creates an MCPClient configured for a managed server with resolved headers.
func (p *ScandrixMCPProvider) BuildClient(integrationID string, authHeaders map[string]string) (*client.MCPClient, error) {
	cfg, err := p.GetManagedConfig(integrationID)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	for k, v := range authHeaders {
		headers[k] = v
	}

	return client.NewMCPClient(cfg.BaseURL, headers, cfg.Name, models.ProviderScandrixMCP), nil
}

// GetIntegrationTools queries tools with an 8s timeout, returning empty slice on transient network failures.
func (p *ScandrixMCPProvider) GetIntegrationTools(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error) {
	if integrationID == DefaultPrimaryIntegrationID || integrationID == "scandrix-mcp-default" {
		return DefaultScandrixToolsCatalog(), nil
	}

	c, err := p.BuildClient(integrationID, nil)
	if err != nil {
		return nil, err
	}

	toolsCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	tools, err := c.GetTools(toolsCtx)
	if err != nil {
		// Circuit-breaker: degrade gracefully to empty list
		return []models.MCPTool{}, nil
	}

	for i := range tools {
		tools[i].Warning = providers.HasWarning(tools[i].Name)
	}

	return tools, nil
}

// VerifyManagedConnection fetches tools strictly requiring working credentials, throwing on error.
func (p *ScandrixMCPProvider) VerifyManagedConnection(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error) {
	c, err := p.BuildClient(integrationID, nil)
	if err != nil {
		return nil, err
	}

	tools, err := c.GetTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("credential verification failed: %w", err)
	}
	return tools, nil
}

// InitiateConnection creates a connection entity for a managed integration.
func (p *ScandrixMCPProvider) InitiateConnection(ctx context.Context, orgID string, dto models.InitiateConnectionDTO) (*models.MCPConnectionEntity, error) {
	cfg, err := p.GetManagedConfig(dto.IntegrationID)
	if err != nil {
		return nil, err
	}

	tools, _ := p.GetIntegrationTools(ctx, dto.IntegrationID, orgID)
	allowedTools := dto.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = providers.DefaultReadOnlyToolSlugs(tools)
	}

	conn := &models.MCPConnectionEntity{
		OrganizationID: orgID,
		IntegrationID:  dto.IntegrationID,
		Provider:       string(models.ProviderScandrixMCP),
		Status:         models.ConnectionStatusActive,
		AppName:        cfg.Name,
		MCPURL:         &cfg.BaseURL,
		AllowedTools:   allowedTools,
		Metadata: map[string]any{
			"autoCreated": true,
		},
	}
	return conn, nil
}

// DeleteConnection handles deletion hook.
func (p *ScandrixMCPProvider) DeleteConnection(ctx context.Context, connectionID string) error {
	return nil
}
