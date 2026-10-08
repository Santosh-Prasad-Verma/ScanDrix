package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// Repository and connection lookups the bridge needs. Both are workspace-scoped.
type SCMSourceRepository interface {
	GetTrackedRepositoryByID(ctx context.Context, wsID, repositoryID uuid.UUID) (*models.TrackedRepository, error)
	GetIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (*models.IntegrationConnection, error)
}

// Errors the bridge returns. Callers map these to status codes and must not
// substitute a plausible file list for any of them.
var (
	// ErrNoSCMProvider means the repository's provider has no registered adapter.
	ErrNoSCMProvider = errors.New("no code management adapter for this repository provider")
	// ErrNotConnected means the workspace has no stored credential for the
	// repository's provider.
	ErrNotConnected = errors.New("code management provider is not connected for this workspace")
	// ErrRepositoryNotFound means the repository id is unknown in this workspace.
	ErrRepositoryNotFound = errors.New("repository not found in this workspace")
)

// PullRequestFilesProvider is the narrow slice of a code-management adapter that
// this bridge needs.
//
// It is deliberately not contracts.ICodeManagementService: that interface carries
// roughly fifty methods, and depending on all of them would make the bridge
// impossible to exercise without a full adapter double. The production adapters
// satisfy this interface structurally.
type PullRequestFilesProvider interface {
	GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error)
}

// SCMPullRequestFileFetcher reads real pull request diffs from the SCM provider.
//
// PullRequestController serves GET /pull-requests/files. The diff itself is only
// known to the provider: the stored review findings record which paths were
// reviewed, not how they changed. Before this existed the endpoint returned a
// fixed +10/-2 and a hardcoded "modified" status for every file, which made an
// unmeasured diff look measured. The provider services are stateless and take
// credentials per request, so one adapter instance serves every workspace.
type SCMPullRequestFileFetcher struct {
	source SCMSourceRepository
	// byProvider holds one adapter per provider. A nil entry means that provider
	// is not wired in this deployment.
	byProvider map[models.SCMProvider]PullRequestFilesProvider
}

// NewSCMPullRequestFileFetcher builds the bridge.
func NewSCMPullRequestFileFetcher(
	source SCMSourceRepository,
	byProvider map[models.SCMProvider]PullRequestFilesProvider,
) *SCMPullRequestFileFetcher {
	return &SCMPullRequestFileFetcher{source: source, byProvider: byProvider}
}

// GetPullRequestFiles returns the provider's own view of a pull request's files,
// including the unified patch, change status and line counts.
//
// Every failure is returned. The caller must report it rather than degrading to
// an empty or synthesised list: an empty list and an unreadable diff are
// different facts, and a rule dry run over a silent empty list reports a clean
// pass over zero files.
func (f *SCMPullRequestFileFetcher) GetPullRequestFiles(
	ctx context.Context,
	wsID, repositoryID uuid.UUID,
	prNumber int,
) ([]*types.PullRequestFile, error) {
	if f == nil || f.source == nil {
		return nil, fmt.Errorf("scm file fetcher unavailable")
	}

	tracked, err := f.source.GetTrackedRepositoryByID(ctx, wsID, repositoryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepositoryNotFound
		}
		return nil, fmt.Errorf("could not resolve repository: %w", err)
	}
	if tracked == nil {
		return nil, ErrRepositoryNotFound
	}
	// A deactivated repository may still be listed in historical reviews, but its
	// credentials are no longer trusted for live API calls.
	if !tracked.IsActive {
		return nil, fmt.Errorf("repository %s is not active: %w", tracked.NamespacePath, ErrNotConnected)
	}

	adapter, ok := f.byProvider[tracked.Provider]
	if !ok || adapter == nil {
		return nil, fmt.Errorf("provider %s: %w", tracked.Provider, ErrNoSCMProvider)
	}

	connection, err := f.source.GetIntegrationConnection(ctx, wsID, tracked.Provider)
	if err != nil {
		return nil, fmt.Errorf("could not read %s credentials: %w", tracked.Provider, err)
	}
	if connection == nil || !connection.IsConnected {
		return nil, fmt.Errorf("%s: %w", tracked.Provider, ErrNotConnected)
	}
	token := strings.TrimSpace(connection.AccessTokenEnc)
	if token == "" {
		return nil, fmt.Errorf("%s credential is empty: %w", tracked.Provider, ErrNotConnected)
	}

	owner, name := splitNamespace(tracked.NamespacePath)
	orgData := types.OrganizationAndTeamData{
		// OrganizationID is the workspace in this deployment; providers only use
		// it for correlation, and credentials come from IntegrationCredentials.
		OrganizationID: wsID.String(),
		WorkspaceID:    wsID,
		Provider:       string(tracked.Provider),
		ProviderID:     tracked.ExternalID,
		IntegrationCredentials: map[string]any{
			"token": token,
			"org":   connection.AccountName,
		},
	}
	repoDescriptor := types.RepositoryDescriptor{
		ID:       tracked.ExternalID,
		Name:     name,
		Owner:    owner,
		FullName: tracked.NamespacePath,
	}

	return adapter.GetFilesByPullRequestId(ctx, orgData, &repoDescriptor, prNumber)
}

// splitNamespace splits "owner/repo" into its parts. A namespace with no slash
// is treated as a bare repository name with no owner, which is what single-path
// self-hosted deployments use.
func splitNamespace(namespacePath string) (owner, name string) {
	trimmed := strings.Trim(strings.TrimSpace(namespacePath), "/")
	if trimmed == "" {
		return "", ""
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[:idx], trimmed[idx+1:]
	}
	return "", trimmed
}

// ErrNoRowsSentinel mirrors the driver's no-rows error so this package does not
// import a database driver. It is matched with errors.Is.
var ErrNoRowsSentinel = errors.New("no rows in result set")