package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/scandrix/backend/internal/api/services"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

/**
 * The bridge turns a database repository id into a live SCM call. Two properties
 * matter: the credentials and repository identity handed to the provider must come
 * from stored, workspace-scoped records (never from the request), and every
 * failure must surface rather than degrade into an empty file list.
 */

type fakeSource struct {
	tracked     *models.TrackedRepository
	trackedErr  error
	connection  *models.IntegrationConnection
	connectionE error
	lastWSID    uuid.UUID
	lastRepoID  uuid.UUID
}

func (f *fakeSource) GetTrackedRepositoryByID(_ context.Context, wsID, repoID uuid.UUID) (*models.TrackedRepository, error) {
	f.lastWSID, f.lastRepoID = wsID, repoID
	return f.tracked, f.trackedErr
}

func (f *fakeSource) GetIntegrationConnection(_ context.Context, _ uuid.UUID, _ models.SCMProvider) (*models.IntegrationConnection, error) {
	return f.connection, f.connectionE
}

// fakeAdapter records what the bridge passed down.
type fakeAdapter struct {
	provider   models.SCMProvider
	gotOrgData types.OrganizationAndTeamData
	gotRepo    types.RepositoryDescriptor
	gotPR      int
	files      []*types.PullRequestFile
	err        error
}

// GetFilesByPullRequestId satisfies the contract for the fields this test uses.
func (f *fakeAdapter) GetFilesByPullRequestId(_ context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	f.gotOrgData = orgData
	if repo != nil {
		f.gotRepo = *repo
	}
	f.gotPR = prNumber
	return f.files, f.err
}

var _ services.PullRequestFilesProvider = (*fakeAdapter)(nil)

func newBridge(source *fakeSource, adapter *fakeAdapter) *services.SCMPullRequestFileFetcher {
	byProvider := map[models.SCMProvider]services.PullRequestFilesProvider{}
	if adapter != nil {
		byProvider[adapter.provider] = adapter
	}
	return services.NewSCMPullRequestFileFetcher(source, byProvider)
}

func TestBridgeReturnsTheProvidersRealDiff(t *testing.T) {
	wsID, repoID := uuid.New(), uuid.New()
	patch := "@@ -1,2 +1,3 @@\n-old\n+new\n"
	source := &fakeSource{
		tracked: &models.TrackedRepository{
			ID: repoID, WorkspaceID: wsID, Provider: models.ProviderGitHub,
			ExternalID: "R_1", NamespacePath: "acme/widgets", IsActive: true,
		},
		connection: &models.IntegrationConnection{
			ID: uuid.New(), Provider: models.ProviderGitHub, IsConnected: true,
			AccessTokenEnc: "secret-token", AccountName: "acme",
		},
	}
	adapter := &fakeAdapter{
		provider: models.ProviderGitHub,
		files: []*types.PullRequestFile{
			{Filename: "a.go", Status: "modified", Additions: 12, Deletions: 4, Patch: patch},
			// A binary file legitimately has no patch.
			{Filename: "logo.png", Status: "added", Additions: 0, Deletions: 0, Patch: ""},
		},
	}

	files, err := newBridge(source, adapter).GetPullRequestFiles(context.Background(), wsID, repoID, 42)
	if err != nil {
		t.Fatalf("expected the provider diff, got error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Patch != patch {
		t.Errorf("patch not passed through: %q", files[0].Patch)
	}

	// The repository identity must be derived from the stored row.
	if adapter.gotRepo.Owner != "acme" || adapter.gotRepo.Name != "widgets" {
		t.Errorf("namespace not split correctly: %+v", adapter.gotRepo)
	}
	if adapter.gotPR != 42 {
		t.Errorf("expected prNumber 42, got %d", adapter.gotPR)
	}
	// The stored token is forwarded; nothing is invented.
	if got, _ := adapter.gotOrgData.IntegrationCredentials["token"].(string); got != "secret-token" {
		t.Errorf("expected the stored credential to be forwarded, got %q", got)
	}
	if adapter.gotOrgData.WorkspaceID != wsID {
		t.Errorf("expected the workspace on orgData, got %v", adapter.gotOrgData.WorkspaceID)
	}
	// The lookup must be workspace-scoped so a guessed repository id cannot cross tenants.
	if source.lastWSID != wsID || source.lastRepoID != repoID {
		t.Errorf("lookup not scoped as requested: ws=%v repo=%v", source.lastWSID, source.lastRepoID)
	}
}

func TestBridgeRefusesRatherThanReturningAnEmptyList(t *testing.T) {
	wsID, repoID := uuid.New(), uuid.New()
	tracked := &models.TrackedRepository{
		ID: repoID, WorkspaceID: wsID, Provider: models.ProviderGitHub,
		ExternalID: "R_1", NamespacePath: "acme/widgets", IsActive: true,
	}
	connected := &models.IntegrationConnection{
		Provider: models.ProviderGitHub, IsConnected: true, AccessTokenEnc: "secret-token",
	}
	adapter := &fakeAdapter{provider: models.ProviderGitHub}

	cases := []struct {
		name    string
		source  *fakeSource
		adapter *fakeAdapter
		wantErr error
	}{
		{
			name:    "repository unknown in this workspace",
			source:  &fakeSource{trackedErr: pgx.ErrNoRows},
			adapter: adapter,
			wantErr: services.ErrRepositoryNotFound,
		},
		{
			name:    "repository row missing",
			source:  &fakeSource{},
			adapter: adapter,
			wantErr: services.ErrRepositoryNotFound,
		},
		{
			name:    "provider has no adapter",
			source:  &fakeSource{tracked: tracked, connection: connected},
			adapter: nil,
			wantErr: services.ErrNoSCMProvider,
		},
		{
			name:    "provider not connected",
			source:  &fakeSource{tracked: tracked, connection: &models.IntegrationConnection{Provider: models.ProviderGitHub}},
			adapter: adapter,
			wantErr: services.ErrNotConnected,
		},
		{
			name: "credential missing",
			source: &fakeSource{tracked: tracked, connection: &models.IntegrationConnection{
				Provider: models.ProviderGitHub, IsConnected: true, AccessTokenEnc: "  ",
			}},
			adapter: adapter,
			wantErr: services.ErrNotConnected,
		},
		{
			name: "repository deactivated",
			source: &fakeSource{
				tracked:    &models.TrackedRepository{ID: repoID, Provider: models.ProviderGitHub, NamespacePath: "acme/widgets"},
				connection: connected,
			},
			adapter: adapter,
			wantErr: services.ErrNotConnected,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := newBridge(tc.source, tc.adapter).GetPullRequestFiles(context.Background(), wsID, repoID, 7)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			// Critically: never a silent empty list, which would read downstream as
			// "this pull request changed no files".
			if files != nil {
				t.Errorf("expected no files alongside the error, got %d", len(files))
			}
		})
	}
}

func TestBridgePropagatesProviderFailure(t *testing.T) {
	wsID, repoID := uuid.New(), uuid.New()
	providerErr := errors.New("upstream 502")
	source := &fakeSource{
		tracked: &models.TrackedRepository{
			ID: repoID, WorkspaceID: wsID, Provider: models.ProviderGitHub,
			NamespacePath: "acme/widgets", IsActive: true,
		},
		connection: &models.IntegrationConnection{Provider: models.ProviderGitHub, IsConnected: true, AccessTokenEnc: "t"},
	}
	adapter := &fakeAdapter{provider: models.ProviderGitHub, err: providerErr}

	if _, err := newBridge(source, adapter).GetPullRequestFiles(context.Background(), wsID, repoID, 3); !errors.Is(err, providerErr) {
		t.Fatalf("expected the provider error to propagate, got %v", err)
	}
}