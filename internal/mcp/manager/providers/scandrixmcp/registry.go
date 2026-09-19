// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package scandrixmcp

import (
	"github.com/scandrix/backend/internal/mcp/manager/models"
)

// RawManagedConfig represents the declarative JSON descriptor for a managed integration.
type RawManagedConfig struct {
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	Category    string                        `json:"category"`
	BaseURL     string                        `json:"baseUrl"`
	Protocol    models.MCPIntegrationProtocol `json:"protocol"`
	LogoURL     string                        `json:"logoUrl"`
	Headers     map[string]string             `json:"headers"`
	AuthMethods []models.PublicAuthMethod     `json:"authMethods"`
}

// ManagedCategoryByID maps integration IDs to canonical capability categories.
var ManagedCategoryByID = map[string]string{
	"scandrix-docs-default":   "documentation",
	"scandrix-issues-default": "task-management",
	"sentry-default":          "observability",
	"exa-default":             "search",
	"scandrix-osv-default":    "security",
	"linear-default":          "task-management",
	"atlassian-rovo-default":  "task-management",
	"notion-default":          "task-management",
	"fireflies-default":       "meetings",
}

// DefaultManagedServersCatalog returns the production registry of preconfigured integrations.
func DefaultManagedServersCatalog() []RawManagedConfig {
	return []RawManagedConfig{
		{
			ID:       "scandrix-docs-default",
			Name:     "ScanDrix Docs",
			Category: "documentation",
			BaseURL:  "https://docs.scandrix.io/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://scandrix.io/assets/logo.png",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "none", Type: models.AuthTypeNone, Default: true},
			},
		},
		{
			ID:       "scandrix-issues-default",
			Name:     "Git Issues",
			Category: "task-management",
			BaseURL:  "/mcp/issues",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://scandrix.io/assets/logo.png",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "none", Type: models.AuthTypeNone, Default: true},
			},
		},
		{
			ID:       "sentry-default",
			Name:     "Sentry",
			Category: "observability",
			BaseURL:  "https://mcp.sentry.dev/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://cdn.jsdelivr.net/gh/devicons/devicon/icons/sentry/sentry-original.svg",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "oauth", Label: "OAuth", Type: models.AuthTypeOAuth2, Default: true},
			},
		},
		{
			ID:       "exa-default",
			Name:     "Exa Search",
			Category: "search",
			BaseURL:  "https://mcp.exa.ai/mcp?tools=web_search_exa,get_code_context_exa,crawling_exa,company_research_exa,linkedin_search_exa,deep_researcher_start,deep_researcher_check",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://exa.ai/images/favicon-32x32.png",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "none", Type: models.AuthTypeNone, Default: true},
			},
		},
		{
			ID:       "scandrix-osv-default",
			Name:     "ScanDrix OSV",
			Category: "security",
			BaseURL:  "https://mcp-osv.scandrix.io/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://scandrix.io/assets/logo.png",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "none", Type: models.AuthTypeNone, Default: true},
			},
		},
		{
			ID:       "linear-default",
			Name:     "Linear",
			Category: "task-management",
			BaseURL:  "https://mcp.linear.app/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://static.linear.app/client/assets/newAppIcon-256x256-full.NvjGn2GI.png",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "oauth", Label: "OAuth", Type: models.AuthTypeOAuth2, Default: true},
				{
					ID:    "token",
					Label: "API key",
					Type:  models.AuthTypeBearerToken,
					UserFields: []models.ManagedAuthUserField{
						{Name: "apiKey", Label: "Linear API key", Required: true, Secret: true},
					},
				},
			},
		},
		{
			ID:       "atlassian-rovo-default",
			Name:     "Atlassian Rovo",
			Category: "task-management",
			BaseURL:  "https://mcp.atlassian.com/v1/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://avatars.githubusercontent.com/u/21215?s=200&v=4",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "oauth", Label: "OAuth", Type: models.AuthTypeOAuth2, Default: true},
				{
					ID:    "token",
					Label: "API token",
					Type:  models.AuthTypeBasic,
					UserFields: []models.ManagedAuthUserField{
						{Name: "email", Label: "Atlassian account email", Required: true},
						{Name: "apiToken", Label: "API token", Required: true, Secret: true},
					},
				},
			},
		},
		{
			ID:       "notion-default",
			Name:     "Notion",
			Category: "task-management",
			BaseURL:  "https://mcp.notion.com/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://www.notion.so/images/favicon.ico",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{ID: "oauth", Label: "OAuth", Type: models.AuthTypeOAuth2, Default: true},
			},
		},
		{
			ID:       "fireflies-default",
			Name:     "Fireflies",
			Category: "meetings",
			BaseURL:  "https://api.fireflies.ai/mcp",
			Protocol: models.ProtocolHTTP,
			LogoURL:  "https://fireflies.ai/favicon.ico",
			Headers:  map[string]string{},
			AuthMethods: []models.PublicAuthMethod{
				{
					ID:      "token",
					Label:   "API key",
					Type:    models.AuthTypeBearerToken,
					Default: true,
					UserFields: []models.ManagedAuthUserField{
						{Name: "apiKey", Label: "Fireflies API key", Required: true, Secret: true},
					},
				},
			},
		},
	}
}
