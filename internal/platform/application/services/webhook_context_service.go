// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"net/url"
	"strings"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// WebhookDisambiguator provides provider-specific network disambiguation (e.g., self-hosted host).
type WebhookDisambiguator struct {
	Host string `json:"host,omitempty"`
}

// IntegrationConfigRecord represents a repository-to-team integration mapping.
type IntegrationConfigRecord struct {
	ID             string
	Key            string
	Value          string
	Platform       models.SCMProvider
	OrganizationID string
	TeamID         string
	Host           string
	BotUsername    string
}

// TeamAutomationRecord tracks team-level active automation rules.
type TeamAutomationRecord struct {
	ID             string
	TeamID         string
	AutomationType string // e.g. "AUTOMATION_CODE_REVIEW"
	Status         bool   // true = active
}

// IIntegrationConfigReader queries integration configurations.
type IIntegrationConfigReader interface {
	FindConfigsByRepository(ctx context.Context, platform models.SCMProvider, repoID string) ([]IntegrationConfigRecord, error)
}

// ITeamAutomationReader queries team automation activation status.
type ITeamAutomationReader interface {
	IsCodeReviewActive(ctx context.Context, teamID string) (string, bool, error)
}

// WebhookContextResult contains the resolved routing context for incoming webhooks.
type WebhookContextResult struct {
	OrganizationAndTeamData types.OrganizationAndTeamData `json:"organizationAndTeamData"`
	TeamAutomationID        string                        `json:"teamAutomationId"`
	BotUsername             string                        `json:"botUsername,omitempty"`
}

// WebhookContextService resolves organization, team, and active automation context.
type WebhookContextService struct {
	configReader     IIntegrationConfigReader
	automationReader ITeamAutomationReader
}

// NewWebhookContextService creates a new WebhookContextService.
func NewWebhookContextService(
	configReader IIntegrationConfigReader,
	automationReader ITeamAutomationReader,
) *WebhookContextService {
	return &WebhookContextService{
		configReader:     configReader,
		automationReader: automationReader,
	}
}

// GetContext retrieves the organization, team, and active automation context for a webhook event.
func (s *WebhookContextService) GetContext(
	ctx context.Context,
	platform models.SCMProvider,
	repositoryID string,
	disambiguator *WebhookDisambiguator,
) (*WebhookContextResult, error) {
	configs, err := s.configReader.FindConfigsByRepository(ctx, platform, repositoryID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}

	candidates := s.disambiguateConfigs(configs, disambiguator)

	for _, config := range candidates {
		if config.OrganizationID == "" || config.TeamID == "" {
			continue
		}

		autoID, active, err := s.automationReader.IsCodeReviewActive(ctx, config.TeamID)
		if err != nil {
			continue
		}

		if active {
			return &WebhookContextResult{
				OrganizationAndTeamData: types.OrganizationAndTeamData{
					OrganizationID: config.OrganizationID,
					TeamID:         config.TeamID,
				},
				TeamAutomationID: autoID,
				BotUsername:      config.BotUsername,
			}, nil
		}
	}

	return nil, nil
}

func (s *WebhookContextService) disambiguateConfigs(
	configs []IntegrationConfigRecord,
	disambiguator *WebhookDisambiguator,
) []IntegrationConfigRecord {
	if len(configs) <= 1 || disambiguator == nil || disambiguator.Host == "" {
		return configs
	}

	targetHost := normalizeHost(disambiguator.Host)
	if targetHost == "" {
		return configs
	}

	// If any candidate has no host recorded, we cannot safely filter
	for _, c := range configs {
		if normalizeHost(c.Host) == "" {
			return configs
		}
	}

	var matched []IntegrationConfigRecord
	for _, c := range configs {
		if normalizeHost(c.Host) == targetHost {
			matched = append(matched, c)
		}
	}

	if len(matched) == 1 {
		return matched
	}
	return configs
}

func normalizeHost(value string) string {
	val := strings.TrimSpace(value)
	if val == "" {
		return ""
	}
	if !strings.Contains(val, "://") {
		val = "https://" + val
	}
	parsed, err := url.Parse(val)
	if err != nil {
		return strings.ToLower(strings.Split(value, "/")[0])
	}
	return strings.ToLower(parsed.Hostname())
}
