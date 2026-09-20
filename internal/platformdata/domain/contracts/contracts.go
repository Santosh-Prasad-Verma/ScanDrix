// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: contracts
// File: contracts.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

// DeliveredFilterOpts specifies query criteria for counting delivered pull requests.
type DeliveredFilterOpts struct {
	Severities     []string
	AuthorEmail    string
	UnresolvedOnly bool
	OpenOnly       bool
}

// IPullRequestsRepository defines the persistent data access contract for rich PR documents.
type IPullRequestsRepository interface {
	Create(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error)
	FindByID(ctx context.Context, uuid string) (*models.PullRequest, error)
	FindOne(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error)
	FindByNumberAndRepositoryID(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error)
	Find(ctx context.Context, orgID string, repoID string, limit int, offset int) ([]*models.PullRequest, error)

	FindPRNumbersByTitleAndOrganization(ctx context.Context, title string, orgID string, repoIDs []string) ([]struct {
		Number       int
		RepositoryID string
	}, error)

	FindManyByNumbersAndRepositoryIDs(ctx context.Context, criteria []struct {
		Number       int
		RepositoryID string
	}, orgID string) ([]*models.PullRequest, error)

	FindManyByNumbers(ctx context.Context, prNumbers []int, orgID string) ([]models.PullRequestUserMapping, error)
	FindNumbersByRepositoryID(ctx context.Context, orgID string, repoID string, until *time.Time) ([]int, error)

	FindSuggestionCountsByNumbersAndRepositoryIds(ctx context.Context, criteria []struct {
		Number       int
		RepositoryID string
	}, orgID string) (map[string]models.SuggestionCountsBySeverity, error)

	FindOpenPullRequestKeysOpenedSince(ctx context.Context, since time.Time, orgID string, repoIDs []string) ([]struct {
		Number       int
		RepositoryID string
	}, error)

	FindDistinctAuthorsByRepositoryIds(ctx context.Context, orgID string, repoIDs []string, search string, limit int) ([]models.PullRequestAuthorSuggestion, error)
	CountDeliveredPullRequests(ctx context.Context, orgID string, repoIDs []string, opts DeliveredFilterOpts) (int, error)

	FindFileWithSuggestions(ctx context.Context, orgID string, repoID string, prNumber int, filePath string) (*models.File, error)
	FindSuggestionsByPR(ctx context.Context, orgID string, repoID string, prNumber int, deliveryStatus enums.DeliveryStatus) ([]models.Suggestion, error)

	BulkApplyFileChanges(ctx context.Context, prUUID string, orgID string, ops []models.FileBulkOp) (*models.BulkApplyResult, error)
	ComputeFileTotals(ctx context.Context, prUUID string, orgID string) (added int, deleted int, changes int, err error)

	Update(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error)
	UpdateSuggestion(ctx context.Context, orgID string, suggestionID string, updateData map[string]interface{}) error
	UpdateSyncedSuggestionsFlag(ctx context.Context, prNumbers []int, repoID string, orgID string, synced bool) error
	UpdateSyncedWithIssuesFlag(ctx context.Context, prNumber int, repoID string, orgID string, synced bool) error
}

// IPullRequestsService orchestrates business rules, author extraction, and suggestion aggregation.
type IPullRequestsService interface {
	IPullRequestsRepository

	AggregateAndSaveDataStructure(
		ctx context.Context,
		pr *models.PullRequest,
		changedFiles []models.File,
		prioritizedSuggestions []models.Suggestion,
		unusedSuggestions []models.Suggestion,
		commits []models.Commit,
	) (*models.PullRequest, error)

	ExtractUser(payload map[string]interface{}, platformType string) (*models.PullRequestUser, error)
	ExtractUsers(payload map[string]interface{}, platformType string) ([]models.PullRequestUser, error)

	AddPRLevelSuggestions(
		ctx context.Context,
		orgID string,
		repoID string,
		prNumber int,
		suggestions []models.SuggestionByPR,
	) error
}
