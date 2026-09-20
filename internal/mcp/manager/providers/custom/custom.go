// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package custom

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/mcp/manager/client"
	"github.com/scandrix/backend/internal/mcp/manager/crypto"
	"github.com/scandrix/backend/internal/mcp/manager/models"
	"github.com/scandrix/backend/internal/mcp/manager/providers"
	"github.com/scandrix/backend/internal/mcp/manager/providers/services"
	"github.com/scandrix/backend/internal/mcp/manager/repository"
)

// CustomProvider manages user-registered MCP servers.
type CustomProvider struct {
	repo      *repository.MCPRepository
	encryptor *crypto.Encryptor
}

// NewCustomProvider initializes custom server management.
func NewCustomProvider(repo *repository.MCPRepository, encryptor *crypto.Encryptor) *CustomProvider {
	return &CustomProvider{
		repo:      repo,
		encryptor: encryptor,
	}
}

// GetIntegrations returns custom integrations configured for an organization.
func (p *CustomProvider) GetIntegrations(ctx context.Context, page, pageSize int, filters map[string]any) ([]models.MCPIntegration, error) {
	orgID, _ := filters["organizationId"].(string)
	if orgID == "" {
		return nil, errors.New("organizationId is required")
	}

	entities, err := p.repo.GetCustomIntegrations(ctx, orgID, true)
	if err != nil {
		return nil, err
	}

	descService := services.GetIntegrationDescriptionService()
	integrations := make([]models.MCPIntegration, 0, len(entities))
	for _, entity := range entities {
		desc := stringValue(entity.Description)
		if desc == "" {
			desc = descService.GetDescription("custom", entity.Name)
		}
		integrations = append(integrations, models.MCPIntegration{
			ID:          entity.ID,
			Name:        entity.Name,
			Description: desc,
			AuthScheme:  string(entity.AuthType),
			AppName:     entity.Name,
			Logo:        stringValue(entity.LogoURL),
			Provider:    models.ProviderCustom,
			BaseURL:     entity.BaseURL,
			Protocol:    entity.Protocol,
			Active:      entity.Active,
		})
	}

	return integrations, nil
}

// GetIntegration retrieves a specific custom integration for an organization.
func (p *CustomProvider) GetIntegration(ctx context.Context, integrationID, organizationID string) (*models.MCPIntegration, error) {
	entity, err := p.repo.GetCustomIntegrationByID(ctx, organizationID, integrationID, false)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("custom integration '%s' not found", integrationID)
	}

	desc := stringValue(entity.Description)
	if desc == "" {
		desc = services.GetIntegrationDescriptionService().GetDescription("custom", entity.Name)
	}

	return &models.MCPIntegration{
		ID:          entity.ID,
		Name:        entity.Name,
		Description: desc,
		AuthScheme:  string(entity.AuthType),
		AppName:     entity.Name,
		Logo:        stringValue(entity.LogoURL),
		Provider:    models.ProviderCustom,
		BaseURL:     entity.BaseURL,
		Protocol:    entity.Protocol,
		Active:      entity.Active,
	}, nil
}

// GetIntegrationRequiredParams returns required params for custom integrations.
func (p *CustomProvider) GetIntegrationRequiredParams(ctx context.Context, integrationID string) ([]models.MCPRequiredParam, error) {
	return []models.MCPRequiredParam{}, nil
}

// BuildClient creates an MCPClient for a custom integration, decrypting headers and credentials.
func (p *CustomProvider) BuildClient(ctx context.Context, orgID, integrationID string) (*client.MCPClient, error) {
	entity, err := p.repo.GetCustomIntegrationByID(ctx, orgID, integrationID, false)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("custom integration '%s' not found", integrationID)
	}

	headers := make(map[string]string)

	// Decrypt custom headers
	if entity.Headers != nil && *entity.Headers != "" {
		decrypted, dErr := p.encryptor.Decrypt(*entity.Headers)
		if dErr == nil {
			var customHeaders map[string]string
			if err := json.Unmarshal([]byte(decrypted), &customHeaders); err == nil {
				for k, v := range customHeaders {
					headers[k] = v
				}
			}
		}
	}

	// Decrypt auth credentials
	if entity.Auth != nil && *entity.Auth != "" {
		decrypted, dErr := p.encryptor.Decrypt(*entity.Auth)
		if dErr == nil {
			var authPayload map[string]any
			if err := json.Unmarshal([]byte(decrypted), &authPayload); err == nil {
				switch entity.AuthType {
				case models.AuthTypeBearerToken:
					if token, ok := authPayload["bearerToken"].(string); ok && token != "" {
						headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
					}
				case models.AuthTypeAPIKey:
					headerName, _ := authPayload["apiKeyHeader"].(string)
					keyVal, _ := authPayload["apiKey"].(string)
					if headerName == "" {
						headerName = "X-Api-Key"
					}
					if keyVal != "" {
						headers[headerName] = keyVal
					}
				case models.AuthTypeBasic:
					user, _ := authPayload["basicUser"].(string)
					pass, _ := authPayload["basicPassword"].(string)
					creds := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, pass)))
					headers["Authorization"] = fmt.Sprintf("Basic %s", creds)
				}
			}
		}
	}

	return client.NewMCPClient(entity.BaseURL, headers, entity.Name, models.ProviderCustom), nil
}

// GetIntegrationTools discovers tools exposed by a custom server.
func (p *CustomProvider) GetIntegrationTools(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error) {
	c, err := p.BuildClient(ctx, organizationID, integrationID)
	if err != nil {
		return nil, err
	}

	toolsCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	tools, err := c.GetTools(toolsCtx)
	if err != nil {
		return []models.MCPTool{}, nil
	}

	for i := range tools {
		tools[i].Warning = providers.HasWarning(tools[i].Name)
	}

	return tools, nil
}

// VerifyManagedConnection executes tools check throwing if credentials fail.
func (p *CustomProvider) VerifyManagedConnection(ctx context.Context, integrationID, organizationID string) ([]models.MCPTool, error) {
	c, err := p.BuildClient(ctx, organizationID, integrationID)
	if err != nil {
		return nil, err
	}
	return c.GetTools(ctx)
}

// InitiateConnection links a custom integration to an organization connection.
func (p *CustomProvider) InitiateConnection(ctx context.Context, orgID string, dto models.InitiateConnectionDTO) (*models.MCPConnectionEntity, error) {
	entity, err := p.repo.GetCustomIntegrationByID(ctx, orgID, dto.IntegrationID, false)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("custom integration '%s' not found", dto.IntegrationID)
	}

	tools, _ := p.GetIntegrationTools(ctx, dto.IntegrationID, orgID)
	allowedTools := dto.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = providers.DefaultReadOnlyToolSlugs(tools)
	}

	conn := &models.MCPConnectionEntity{
		OrganizationID: orgID,
		IntegrationID:  dto.IntegrationID,
		Provider:       string(models.ProviderCustom),
		Status:         models.ConnectionStatusActive,
		AppName:        entity.Name,
		MCPURL:         &entity.BaseURL,
		AllowedTools:   allowedTools,
		Metadata: map[string]any{
			"customIntegration": true,
		},
	}

	return conn, nil
}

// DeleteConnection handles connection removal.
func (p *CustomProvider) DeleteConnection(ctx context.Context, connectionID string) error {
	return nil
}

func stringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}
