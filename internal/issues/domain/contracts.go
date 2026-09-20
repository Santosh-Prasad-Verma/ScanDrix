package domain

import (
	"context"
	"time"
)

// IssuesRepository provides persistent storage operations for tracked issues.
type IssuesRepository interface {
	Create(ctx context.Context, issue *Issue) (*Issue, error)
	FindByID(ctx context.Context, uuid string) (*Issue, error)
	FindOne(ctx context.Context, filter map[string]any) (*Issue, error)
	FindByFileAndStatus(ctx context.Context, orgID, repoID, filePath string, status IssueStatus) ([]*Issue, error)
	Find(ctx context.Context, orgID string) ([]*Issue, error)
	FindByFilters(ctx context.Context, filter map[string]any) ([]*Issue, error)
	Count(ctx context.Context, filter map[string]any) (int64, error)
	Update(ctx context.Context, uuid string, updateData map[string]any) (*Issue, error)
	UpdateLabel(ctx context.Context, uuid, label string) (*Issue, error)
	UpdateSeverity(ctx context.Context, uuid string, severity SeverityLevel) (*Issue, error)
	UpdateStatus(ctx context.Context, uuid string, status IssueStatus) (*Issue, error)
	UpdateStatusByIds(ctx context.Context, uuids []string, status IssueStatus) ([]*Issue, error)
	AddSuggestionIDs(ctx context.Context, uuid string, suggestionIDs []string) (*Issue, error)
}

// IssuesService defines business operations for tracked issues.
type IssuesService interface {
	IssuesRepository
}

// DrixyIssuesManagementService coordinates analysis, deduplication, and lifecycle updates.
type DrixyIssuesManagementService interface {
	ProcessClosedPR(ctx context.Context, params ContextToGenerateIssues) error
	MergeSuggestionsIntoIssues(ctx context.Context, context ContextToGenerateIssues, filePath string, newSuggestions []SuggestionItem) error
	CreateNewIssues(ctx context.Context, context ContextToGenerateIssues, unmatchedSuggestions []SuggestionItem) error
	ResolveExistingIssues(ctx context.Context, context ContextToGenerateIssues, files []PRFileInfo) error
	AgeCalculation(createdAt time.Time) string
	BuildFilter(ctx context.Context, filters GetIssuesFilter) (map[string]any, error)
	EnrichContributingSuggestions(ctx context.Context, suggestions []ContributingSuggestion, orgID string) ([]ContributingSuggestion, error)
	ClearIssuesCache(ctx context.Context, orgID string) error
}

// DrixyIssuesAnalysisEngine abstracts LLM-based clustering, deduplication, and code resolution.
type DrixyIssuesAnalysisEngine interface {
	MergeSuggestionsIntoIssues(ctx context.Context, orgTeam OrganizationAndTeamData, pr PullRequestInfo, promptData any) (*MergeResult, error)
	ResolveExistingIssues(ctx context.Context, orgTeam OrganizationAndTeamData, pr PullRequestInfo, promptData any) (*ResolveResult, error)
}

// MergeResult holds output from AI suggestion deduplication against existing issues.
type MergeResult struct {
	Matches []SuggestionMatch `json:"matches"`
}

// SuggestionMatch links an incoming suggestion to an existing issue or flags it as new.
type SuggestionMatch struct {
	SuggestionID    string `json:"suggestionId"`
	ExistingIssueID string `json:"existingIssueId,omitempty"`
}

// ResolveResult holds the AI code verification verdicts for open issues.
type ResolveResult struct {
	IssueVerificationResults []IssueVerificationResult `json:"issueVerificationResults"`
}

// IssueVerificationResult indicates whether an issue's defect remains present in code.
type IssueVerificationResult struct {
	IssueID              string `json:"issueId"`
	IsIssuePresentInCode bool   `json:"isIssuePresentInCode"`
}

// ExternalIssueTracker defines operations for syncing issues to external trackers (Jira, Linear, GitHub).
type ExternalIssueTracker interface {
	TrackerType() IssueTrackerType
	CreateIssue(ctx context.Context, issue *Issue) (*ExternalIssueRef, error)
	UpdateIssueStatus(ctx context.Context, externalID string, status IssueStatus) error
	GetIssue(ctx context.Context, externalID string) (*ExternalIssueRef, error)
}

// FeedbackItem provides sentiment reactions for a suggestion.
type FeedbackItem struct {
	SuggestionID string        `json:"suggestionId"`
	Reactions    ReactionStats `json:"reactions"`
}

// CodeReviewFeedbackReader provides read access to feedback reactions.
type CodeReviewFeedbackReader interface {
	GetByOrganizationID(ctx context.Context, orgID string) ([]FeedbackItem, error)
}

// IssuesAuthorizationService manages role permissions and repository scoping.
type IssuesAuthorizationService interface {
	Ensure(ctx context.Context, user UserRef, action, resource string, repoIDs []string) error
	GetRepositoryScope(ctx context.Context, user UserRef, action, resource string) ([]string, error)
}

// IssueCacheService provides caching for issue queries.
type IssueCacheService interface {
	Get(ctx context.Context, key string) ([]*Issue, bool, error)
	Set(ctx context.Context, key string, issues []*Issue, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// PullRequestsIssuesService accesses pull requests and updates synchronization state.
type PullRequestsIssuesService interface {
	FindByNumberAndRepositoryName(ctx context.Context, number int, repoName string, orgTeam OrganizationAndTeamData) (*PullRequestInfo, []PRFileInfo, error)
	UpdateSyncedWithIssuesFlag(ctx context.Context, prNumber int, repoID, orgID string, synced bool) error
	FindSuggestionsByPR(ctx context.Context, orgID string, prNumber int, deliveryStatus string) ([]SuggestionItem, error)
}

// ParametersService retrieves organization-level configuration parameters.
type ParametersService interface {
	GetIssueCreationConfig(ctx context.Context, orgTeam OrganizationAndTeamData) (*IssueCreationConfig, error)
}
