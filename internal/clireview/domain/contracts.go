package domain

import (
	"context"
	"time"
)

// ITrialRateLimiterService provides rate limiting for anonymous or trial CLI reviews.
type ITrialRateLimiterService interface {
	CheckRateLimit(ctx context.Context, fingerprint string) (*RateLimitResult, error)
}

// IAuthenticatedRateLimiterService provides rate limiting for authenticated team CLI reviews.
type IAuthenticatedRateLimiterService interface {
	CheckRateLimit(ctx context.Context, teamID string) (*AuthenticatedRateLimitResult, error)
}

// RateLimitResult conveys trial rate-limiting decisions.
type RateLimitResult struct {
	Allowed   bool       `json:"allowed"`
	Remaining int        `json:"remaining"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

// AuthenticatedRateLimitResult conveys team rate-limiting decisions.
type AuthenticatedRateLimitResult struct {
	Allowed   bool       `json:"allowed"`
	Remaining int        `json:"remaining"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

// ReadTraceDecisionBranchInput defines input for fetching recorded branch decisions.
type ReadTraceDecisionBranchInput struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	RepositoryID   string `json:"repositoryId"`
	RepositoryName string `json:"repositoryName"`
	Branch         string `json:"branch"`
}

// ITraceDecisionBranchReader reads decision shard branches from code host repositories.
type ITraceDecisionBranchReader interface {
	Read(ctx context.Context, input ReadTraceDecisionBranchInput) (*TraceDecisionBranchRecord, error)
}

// ILLMDecisionClient provides structured LLM extraction for decisions.
type ILLMDecisionClient interface {
	ExtractDecisions(ctx context.Context, systemPrompt string, userPayload string) ([]CliSessionClassifiedDecision, error)
}

// IGitHubPublicPrService fetches pull request metadata and unified diffs from GitHub.
type IGitHubPublicPrService interface {
	Fetch(ctx context.Context, prURL string) (*PublicPrMetadata, error)
	FetchDiff(ctx context.Context, owner, repo string, prNumber int) (string, error)
}

// IPublicPrAiSummaryService generates high-level summaries for public PR demos.
type IPublicPrAiSummaryService interface {
	Generate(ctx context.Context, pr *PublicPrMetadata, diff string) (string, error)
}

// IPublicPrGroupingService clusters changed files by developer intent.
type IPublicPrGroupingService interface {
	Generate(ctx context.Context, pr *PublicPrMetadata, diff string, changedFiles []string) ([]PublicPrGrouping, error)
}

// IFeaturedPublicReviewRepository stores and queries showcase reviews.
type IFeaturedPublicReviewRepository interface {
	UpsertBySlug(ctx context.Context, slug string, review *FeaturedPublicReview) (*FeaturedPublicReview, error)
	FindBySlug(ctx context.Context, slug string) (*FeaturedPublicReview, error)
	ListPublished(ctx context.Context) ([]FeaturedPublicReviewListItem, error)
}

// ISessionEventRepository persists agent turn events and captures.
type ISessionEventRepository interface {
	Create(ctx context.Context, event *SessionEvent) (*SessionEvent, error)
	FindByUUID(ctx context.Context, uuid string) (*SessionEvent, error)
	FindBySessionID(ctx context.Context, sessionID, orgID string) ([]*SessionEvent, error)
	MarkClassificationProcessing(ctx context.Context, uuid string) error
	MarkClassificationCompleted(ctx context.Context, uuid string, decisions []CliSessionClassifiedDecision, source string) error
	MarkClassificationFailed(ctx context.Context, uuid string, errorMessage string) error
	MarkClassificationSkipped(ctx context.Context, uuid string, reason string) error
	FindOrphanedSessions(ctx context.Context, inactivityMinutes int, limit int) ([]OrphanedSessionRef, error)
}

// OrphanedSessionRef references an unclosed CLI session.
type OrphanedSessionRef struct {
	SessionID      string    `json:"sessionId"`
	OrganizationID string    `json:"organizationId"`
	TeamID         string    `json:"teamId"`
	Branch         string    `json:"branch"`
	LastEventAt    time.Time `json:"lastEventAt"`
}

// ICliSessionCaptureRepository manages classified CLI agent sessions.
type ICliSessionCaptureRepository interface {
	Create(ctx context.Context, capture *CliSessionCapture) (*CliSessionCapture, error)
	FindByDedupKey(ctx context.Context, dedupKey string) (*CliSessionCapture, error)
	FindByCaptureID(ctx context.Context, captureID string) (*CliSessionCapture, error)
	MarkProcessing(ctx context.Context, captureID string) error
	MarkCompleted(ctx context.Context, captureID string, decisions []CliSessionClassifiedDecision, source string) error
	MarkSkipped(ctx context.Context, captureID string, reason string) error
	MarkFailed(ctx context.Context, captureID string, errorMsg string) error
}

// IAutomationExecutionService defines persistence for review automation execution telemetry.
type IAutomationExecutionService interface {
	Create(ctx context.Context, record *AutomationExecutionRecord) (*AutomationExecutionRecord, error)
	Update(ctx context.Context, uuid string, status string, data map[string]any, repositoryID string, errorMsg string) error
	FindByID(ctx context.Context, uuid string) (*AutomationExecutionRecord, error)
}

// IDrixyRulesService manages custom organizational rules and plan-limit reconciliation.
type IDrixyRulesService interface {
	FindByOrganizationID(ctx context.Context, orgID string) ([]DrixyRule, error)
	SyncRulesWithPlanLimit(ctx context.Context, orgAndTeam OrganizationAndTeamData, rules []DrixyRule) ([]DrixyRule, error)
}

// IParametersService retrieves org parameters and code review configuration.
type IParametersService interface {
	GetCodeReviewConfig(ctx context.Context, orgAndTeam OrganizationAndTeamData) (*CodeReviewConfig, []RepositoryRef, error)
}

// ICodeManagementService provides platform integrations resolution.
type ICodeManagementService interface {
	GetTypeIntegration(ctx context.Context, orgAndTeam OrganizationAndTeamData) (string, error)
}

