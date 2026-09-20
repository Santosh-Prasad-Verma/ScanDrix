package domain

import (
	"context"
	"time"
)

// PRQueryParams holds filtering parameters for pull request execution queries.
type PRQueryParams struct {
	OrganizationID string
	TeamID         string
	RepositoryID   string
	PlatformType   string
	Author         string
	Status         AutomationStatus
	Search         string
	Page           int
	PageSize       int
	StartDate      *time.Time
	EndDate        *time.Time
}

// PRKey identifies a pull request uniquely across repositories.
type PRKey struct {
	RepositoryID      string `json:"repository_id"`
	PullRequestNumber int    `json:"pull_request_number"`
}

// AwaitingReviewParams configures lookups for PRs waiting on analysis.
type AwaitingReviewParams struct {
	OrganizationID string
	TeamID         string
	RepositoryIDs  []string
	Since          time.Time
}

// DistinctReviewedParams configures lookups for unique PRs reviewed in a timeframe.
type DistinctReviewedParams struct {
	OrganizationID string
	TeamID         string
	RepositoryIDs  []string
	StartDate      time.Time
	EndDate        time.Time
}

// CliQueryParams configures searches for CLI-triggered code reviews.
type CliQueryParams struct {
	OrganizationID string
	TeamID         string
	Status         AutomationStatus
	Page           int
	PageSize       int
	StartDate      *time.Time
	EndDate        *time.Time
}

// PRRef references a pull request with timing and commit information.
type PRRef struct {
	RepositoryID      string    `json:"repository_id"`
	PullRequestNumber int       `json:"pull_request_number"`
	LastExecutionDate time.Time `json:"last_execution_date"`
	HeadCommitSHA     string    `json:"head_commit_sha"`
}

// CreateCodeReviewResult holds the created execution and initial stage log.
type CreateCodeReviewResult struct {
	Execution *AutomationExecutionEntity
	StageLog  *CodeReviewExecutionEntity
}

// AutomationRepository defines persistent storage operations for automations.
type AutomationRepository interface {
	FindOne(ctx context.Context, filter map[string]any) (*AutomationEntity, error)
	Find(ctx context.Context, filter map[string]any) ([]*AutomationEntity, error)
	FindByID(ctx context.Context, uuid string) (*AutomationEntity, error)
	Create(ctx context.Context, automation *AutomationEntity) (*AutomationEntity, error)
	Update(ctx context.Context, filter map[string]any, data map[string]any) (*AutomationEntity, error)
	Delete(ctx context.Context, uuid string) error
}

// AutomationService defines business operations for automations.
type AutomationService interface {
	AutomationRepository
}

// AutomationExecutionRepository provides data access for workflow execution records.
type AutomationExecutionRepository interface {
	Create(ctx context.Context, exec *AutomationExecutionEntity) (*AutomationExecutionEntity, error)
	Update(ctx context.Context, filter map[string]any, data map[string]any) (*AutomationExecutionEntity, error)
	FindStaleInProgress(ctx context.Context, cutoffDate time.Time, limit int) ([]*AutomationExecutionEntity, error)
	Delete(ctx context.Context, uuid string) error
	FindByID(ctx context.Context, uuid string) (*AutomationExecutionEntity, error)
	Find(ctx context.Context, filter map[string]any) ([]*AutomationExecutionEntity, error)
	FindPullRequestExecutionsByOrganizationAndTeam(ctx context.Context, params PRQueryParams) ([]*AutomationExecutionEntity, int64, error)
	GetAwaitingReviewPullRequestKeys(ctx context.Context, params AwaitingReviewParams) ([]PRKey, error)
	GetDistinctReviewedPullRequestKeys(ctx context.Context, params DistinctReviewedParams) ([]PRKey, error)
	FindCliReviewExecutionsByOrganization(ctx context.Context, params CliQueryParams) ([]*AutomationExecutionEntity, int64, error)
	FindLatestExecutionByFilters(ctx context.Context, filter map[string]any) (*AutomationExecutionEntity, error)
	FindByPeriodAndTeamAutomationID(ctx context.Context, teamAutomationID string, start, end time.Time) ([]*AutomationExecutionEntity, error)
	FindEligiblePullRequestRefsForApprovalByPeriodAndTeamAutomationID(ctx context.Context, teamAutomationID string, start, end time.Time) ([]PRRef, error)
}

// AutomationExecutionService provides lifecycle and stage tracking operations for executions.
type AutomationExecutionService interface {
	AutomationExecutionRepository
	CreateCodeReview(ctx context.Context, exec *AutomationExecutionEntity, message, stageName string) (*CreateCodeReviewResult, error)
	UpdateCodeReview(ctx context.Context, filter map[string]any, data map[string]any, message, stageName string) (*AutomationExecutionEntity, error)
	UpdateStageLog(ctx context.Context, stageLogUUID string, data map[string]any) error
}

// CodeReviewExecutionRepository tracks individual pipeline stages.
type CodeReviewExecutionRepository interface {
	Create(ctx context.Context, exec *CodeReviewExecutionEntity) (*CodeReviewExecutionEntity, error)
	Update(ctx context.Context, filter map[string]any, data map[string]any) (*CodeReviewExecutionEntity, error)
	Find(ctx context.Context, filter map[string]any) ([]*CodeReviewExecutionEntity, error)
	FindOne(ctx context.Context, filter map[string]any) (*CodeReviewExecutionEntity, error)
	FindManyByAutomationExecutionIDs(ctx context.Context, executionIDs []string) ([]*CodeReviewExecutionEntity, error)
	ExistsByAutomationExecutionAndStageStatus(ctx context.Context, executionID string, stageName string, status AutomationStatus) (bool, error)
	Delete(ctx context.Context, uuid string) error
}

// CodeReviewExecutionService provides business operations for stage logs.
type CodeReviewExecutionService interface {
	CodeReviewExecutionRepository
}

// TeamAutomationRepository manages team-level activation of automations.
type TeamAutomationRepository interface {
	Create(ctx context.Context, teamAuto *TeamAutomationEntity) (*TeamAutomationEntity, error)
	Update(ctx context.Context, filter map[string]any, data map[string]any) (*TeamAutomationEntity, error)
	Delete(ctx context.Context, uuid string) error
	FindByID(ctx context.Context, uuid string) (*TeamAutomationEntity, error)
	Find(ctx context.Context, filter map[string]any) ([]*TeamAutomationEntity, error)
}

// TeamAutomationService provides team automation registration and status management.
type TeamAutomationService interface {
	TeamAutomationRepository
	Register(ctx context.Context, teamAuto *TeamAutomationEntity) (*TeamAutomationEntity, error)
}

// AutomationStrategy defines routing for conversational or message-based automation.
type AutomationStrategy interface {
	Route(ctx context.Context, message, userID, channel, sessionID, userName string) (any, error)
}

// AutomationFactory abstracts lifecycle setup and execution of an automation type.
type AutomationFactory interface {
	GetAutomationType() AutomationType
	Setup(ctx context.Context, payload any) error
	Run(ctx context.Context, payload any) (any, error)
	Stop(ctx context.Context, payload any) error
}

// ExecuteAutomationService provides dispatching across registered automation strategies.
type ExecuteAutomationService interface {
	SetupStrategy(ctx context.Context, automationType AutomationType, data any) error
	ExecuteStrategy(ctx context.Context, automationType AutomationType, data any) (any, error)
	StopStrategy(ctx context.Context, automationType AutomationType, data any) error
	GetAutomationMethods(automationType AutomationType) (AutomationFactory, error)
}
