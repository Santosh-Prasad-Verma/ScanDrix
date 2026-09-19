package factory

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

type mockAdvancedPlatformService struct {
	contracts.ICodeManagementService
	provider models.SCMProvider
}

func (m *mockAdvancedPlatformService) Provider() models.SCMProvider {
	return m.provider
}

func (m *mockAdvancedPlatformService) VerifyConnection(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*types.CodeManagementConnectionStatus, error) {
	return &types.CodeManagementConnectionStatus{
		HasConnection:   true,
		IsConnected:     true,
		IsSetupComplete: true,
		PlatformName:    string(m.provider),
	}, nil
}

func (m *mockAdvancedPlatformService) GetRepositories(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	archived *bool,
	search string,
	cursor string,
) ([]*types.Repositories, error) {
	return []*types.Repositories{
		{ID: "1", Name: "repo-active", FullName: "org/repo-active", Archived: false},
		{ID: "2", Name: "repo-archived", FullName: "org/repo-archived", Archived: true},
	}, nil
}

func TestCodeManagementService_AdvancedCapabilities(t *testing.T) {
	fac := NewPlatformIntegrationFactory()
	mockGitHub := &mockAdvancedPlatformService{provider: models.ProviderGitHub}
	mockGitLab := &mockAdvancedPlatformService{provider: models.ProviderGitLab}

	fac.RegisterCodeManagementService(models.ProviderGitHub, mockGitHub)
	fac.RegisterCodeManagementService(models.ProviderGitLab, mockGitLab)

	svc := NewCodeManagementService(fac, nil)

	// 1. Check capabilities
	caps := svc.GetPlatformCapabilities(models.ProviderGitHub)
	if !caps.SupportsInlineReview || !caps.SupportsDiscussions || !caps.SupportsReactions {
		t.Errorf("expected full capabilities for GitHub, got %+v", caps)
	}

	capsBB := svc.GetPlatformCapabilities(models.ProviderBitbucket)
	if capsBB.SupportsReactions {
		t.Errorf("Bitbucket should not support reactions")
	}

	// 2. Health check
	ctx := context.Background()
	status, err := svc.HealthCheckIntegration(ctx, types.OrganizationAndTeamData{
		Provider: string(models.ProviderGitHub),
	})
	if err != nil {
		t.Fatalf("HealthCheckIntegration failed: %v", err)
	}
	if !status.IsConnected || status.Provider != models.ProviderGitHub {
		t.Errorf("unexpected health status: %+v", status)
	}

	// 3. Batch health checks
	batch := svc.BatchHealthCheckIntegrations(ctx, []types.OrganizationAndTeamData{
		{Provider: string(models.ProviderGitHub)},
		{Provider: string(models.ProviderGitLab)},
	})
	if len(batch) != 2 {
		t.Fatalf("expected 2 results, got %d", len(batch))
	}
	if !batch[0].IsConnected || !batch[1].IsConnected {
		t.Errorf("expected both integrations connected, got %+v", batch)
	}

	// 4. Synchronize repositories
	summary, err := svc.SynchronizeRepositories(ctx, types.OrganizationAndTeamData{
		Provider: string(models.ProviderGitHub),
	})
	if err != nil {
		t.Fatalf("SynchronizeRepositories failed: %v", err)
	}
	if summary.TotalFetched != 2 || summary.ActiveRepos != 1 || summary.ArchivedRepos != 1 {
		t.Errorf("unexpected sync summary: %+v", summary)
	}
}
