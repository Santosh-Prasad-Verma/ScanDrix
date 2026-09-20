package factory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// PlatformCapabilities defines feature support flags for a VCS provider.
type PlatformCapabilities struct {
	Provider             models.SCMProvider `json:"provider"`
	SupportsInlineReview bool               `json:"supports_inline_review"`
	SupportsReactions    bool               `json:"supports_reactions"`
	SupportsIssues       bool               `json:"supports_issues"`
	SupportsCheckRuns    bool               `json:"supports_check_runs"`
	SupportsDraftReviews bool               `json:"supports_draft_reviews"`
	SupportsDiscussions  bool               `json:"supports_discussions"`
}

// IntegrationHealthStatus reports the connection and token health of a VCS integration.
type IntegrationHealthStatus struct {
	Provider    models.SCMProvider `json:"provider"`
	IsConnected bool               `json:"is_connected"`
	LatencyMs   int64              `json:"latency_ms"`
	LastError   string             `json:"last_error,omitempty"`
	CheckedAt   time.Time          `json:"checked_at"`
}

// RepositorySyncSummary models the outcome of a repository synchronization across providers.
type RepositorySyncSummary struct {
	Provider       models.SCMProvider `json:"provider"`
	TotalFetched   int                `json:"total_fetched"`
	ActiveRepos    int                `json:"active_repos"`
	ArchivedRepos  int                `json:"archived_repos"`
	SyncDurationMs int64              `json:"sync_duration_ms"`
}

// GetPlatformCapabilities returns the capability matrix for a specific VCS provider.
func (s *CodeManagementService) GetPlatformCapabilities(provider models.SCMProvider) PlatformCapabilities {
	switch provider {
	case models.ProviderGitHub:
		return PlatformCapabilities{
			Provider:             models.ProviderGitHub,
			SupportsInlineReview: true,
			SupportsReactions:    true,
			SupportsIssues:       true,
			SupportsCheckRuns:    true,
			SupportsDraftReviews: true,
			SupportsDiscussions:  true,
		}
	case models.ProviderGitLab:
		return PlatformCapabilities{
			Provider:             models.ProviderGitLab,
			SupportsInlineReview: true,
			SupportsReactions:    true,
			SupportsIssues:       true,
			SupportsCheckRuns:    true,
			SupportsDraftReviews: true,
			SupportsDiscussions:  true,
		}
	case models.ProviderBitbucket:
		return PlatformCapabilities{
			Provider:             models.ProviderBitbucket,
			SupportsInlineReview: true,
			SupportsReactions:    false,
			SupportsIssues:       false,
			SupportsCheckRuns:    true,
			SupportsDraftReviews: false,
			SupportsDiscussions:  true,
		}
	case models.ProviderAzure:
		return PlatformCapabilities{
			Provider:             models.ProviderAzure,
			SupportsInlineReview: true,
			SupportsReactions:    false,
			SupportsIssues:       true,
			SupportsCheckRuns:    true,
			SupportsDraftReviews: false,
			SupportsDiscussions:  true,
		}
	case models.ProviderForgejo:
		return PlatformCapabilities{
			Provider:             models.ProviderForgejo,
			SupportsInlineReview: true,
			SupportsReactions:    true,
			SupportsIssues:       true,
			SupportsCheckRuns:    true,
			SupportsDraftReviews: true,
			SupportsDiscussions:  false,
		}
	default:
		return PlatformCapabilities{
			Provider: provider,
		}
	}
}

// HealthCheckIntegration verifies live connectivity and measures round-trip latency to a VCS provider.
func (s *CodeManagementService) HealthCheckIntegration(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*IntegrationHealthStatus, error) {
	provider, err := s.ResolveProvider(orgData)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}

	adapter, err := s.factory.GetCodeManagementService(provider)
	if err != nil {
		return nil, fmt.Errorf("get adapter for %s: %w", provider, err)
	}

	start := time.Now()
	connStatus, err := adapter.VerifyConnection(ctx, orgData)
	latency := time.Since(start).Milliseconds()

	isConnected := false
	if connStatus != nil {
		isConnected = connStatus.HasConnection || connStatus.IsConnected
	}

	status := &IntegrationHealthStatus{
		Provider:    provider,
		IsConnected: isConnected,
		LatencyMs:   latency,
		CheckedAt:   time.Now().UTC(),
	}
	if err != nil {
		status.LastError = err.Error()
	}
	return status, nil
}

// BatchHealthCheckIntegrations checks multiple integrations in parallel.
func (s *CodeManagementService) BatchHealthCheckIntegrations(
	ctx context.Context,
	orgDataList []types.OrganizationAndTeamData,
) []IntegrationHealthStatus {
	results := make([]IntegrationHealthStatus, len(orgDataList))
	var wg sync.WaitGroup

	for i, od := range orgDataList {
		wg.Add(1)
		go func(idx int, data types.OrganizationAndTeamData) {
			defer wg.Done()
			status, err := s.HealthCheckIntegration(ctx, data)
			if err != nil {
				results[idx] = IntegrationHealthStatus{
					Provider:    models.SCMProvider(data.Provider),
					IsConnected: false,
					LastError:   err.Error(),
					CheckedAt:   time.Now().UTC(),
				}
				return
			}
			results[idx] = *status
		}(i, od)
	}

	wg.Wait()
	return results
}

// SynchronizeRepositories fetches all available repositories for a provider and tallies statistics.
func (s *CodeManagementService) SynchronizeRepositories(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*RepositorySyncSummary, error) {
	provider, err := s.ResolveProvider(orgData)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}

	adapter, err := s.factory.GetCodeManagementService(provider)
	if err != nil {
		return nil, fmt.Errorf("get adapter: %w", err)
	}

	start := time.Now()
	repos, err := adapter.GetRepositories(ctx, orgData, nil, "", "")
	duration := time.Since(start).Milliseconds()

	if err != nil {
		return nil, fmt.Errorf("fetch repositories: %w", err)
	}

	summary := &RepositorySyncSummary{
		Provider:       provider,
		TotalFetched:   len(repos),
		SyncDurationMs: duration,
	}

	for _, r := range repos {
		if r != nil && r.Archived {
			summary.ArchivedRepos++
		} else {
			summary.ActiveRepos++
		}
	}
	return summary, nil
}
