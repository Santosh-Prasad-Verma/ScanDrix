package usecases

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// CliReviewGitSummary captures git remote and branch context.
type CliReviewGitSummary struct {
	Remote           string `json:"remote,omitempty"`
	Branch           string `json:"branch,omitempty"`
	CommitSHA        string `json:"commitSha,omitempty"`
	InferredPlatform string `json:"inferredPlatform,omitempty"`
}

// CliReviewAuthSummary captures authentication method without exposing secrets.
type CliReviewAuthSummary struct {
	Mode              string `json:"mode"` // "team-key" | "personal"
	TeamKeyName       string `json:"teamKeyName,omitempty"`
	LoggedInUserEmail string `json:"loggedInUserEmail,omitempty"`
}

// CliReviewSummaryItem represents a high-level review execution card on the dashboard.
type CliReviewSummaryItem struct {
	ExecutionUUID  string                `json:"executionUuid"`
	CorrelationID  string                `json:"correlationId,omitempty"`
	Status         string                `json:"status"` // "in_progress", "success", "error"
	ErrorMessage   string                `json:"errorMessage,omitempty"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
	FinishedAt     *time.Time            `json:"finishedAt,omitempty"`
	DurationMs     *int64                `json:"durationMs,omitempty"`
	UserEmail      string                `json:"userEmail,omitempty"`
	Git            *CliReviewGitSummary  `json:"git,omitempty"`
	CLIVersion     string                `json:"cliVersion,omitempty"`
	RepositoryID   string                `json:"repositoryId,omitempty"`
	RepositoryName string                `json:"repositoryName,omitempty"`
	FilesAnalyzed  int                   `json:"filesAnalyzed"`
	IssuesFound    int                   `json:"issuesFound"`
	CliAuth        *CliReviewAuthSummary `json:"cliAuth,omitempty"`
}

// CliReviewTimelineEntry records a specific pipeline execution stage milestone.
type CliReviewTimelineEntry struct {
	UUID       string         `json:"uuid"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	Status     string         `json:"status"`
	StageName  string         `json:"stageName,omitempty"`
	StageLabel string         `json:"stageLabel,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

// CliReviewDetailPayload provides deep inspection into a completed review.
type CliReviewDetailPayload struct {
	CliReviewSummaryItem
	Timeline []CliReviewTimelineEntry `json:"timeline"`
	Result   any                      `json:"result,omitempty"`
}

// PaginatedCliReviewSummaries wraps dashboard list results with pagination metadata.
type PaginatedCliReviewSummaries struct {
	Data     []CliReviewSummaryItem `json:"data"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	HasMore  bool                   `json:"hasMore"`
}

// MapExecutionToSummary maps raw execution records into sanitized dashboard summary items.
func MapExecutionToSummary(execution *domain.AutomationExecutionRecord) CliReviewSummaryItem {
	if execution == nil {
		return CliReviewSummaryItem{}
	}

	data := execution.DataExecution
	if data == nil {
		data = make(map[string]any)
	}

	corrID, _ := data["correlationId"].(string)
	userEmail, _ := data["userEmail"].(string)
	cliVersion, _ := data["cliVersion"].(string)

	var gitSummary *CliReviewGitSummary
	if gitMap, ok := data["git"].(map[string]any); ok {
		gitSummary = &CliReviewGitSummary{
			Remote:           stringVal(gitMap["remote"]),
			Branch:           stringVal(gitMap["branch"]),
			CommitSHA:        stringVal(gitMap["commitSha"]),
			InferredPlatform: stringVal(gitMap["inferredPlatform"]),
		}
	} else if gc, ok := data["git"].(*domain.GitContext); ok && gc != nil {
		gitSummary = &CliReviewGitSummary{
			Remote:           gc.Remote,
			Branch:           gc.Branch,
			CommitSHA:        gc.CommitSHA,
			InferredPlatform: gc.InferredPlatform,
		}
	}

	var authSummary *CliReviewAuthSummary
	if authMap, ok := data["cliAuth"].(map[string]any); ok {
		mode := stringVal(authMap["mode"])
		if mode != "" {
			authSummary = &CliReviewAuthSummary{
				Mode:              mode,
				TeamKeyName:       stringVal(authMap["teamKeyName"]),
				LoggedInUserEmail: stringVal(authMap["userEmail"]),
			}
		}
	} else if authCtx, ok := data["cliAuth"].(*domain.ExecutionAuthContext); ok && authCtx != nil {
		authSummary = &CliReviewAuthSummary{
			Mode:              authCtx.Mode,
			TeamKeyName:       authCtx.TeamKeyName,
			LoggedInUserEmail: authCtx.UserEmail,
		}
	}

	var repoID, repoName string
	if repoRes, ok := data["repositoryResolution"].(map[string]any); ok {
		repoID = stringVal(repoRes["resolvedRepositoryId"])
		repoName = stringVal(repoRes["resolvedRepositoryName"])
	}
	if repoID == "" {
		repoID = execution.RepositoryID
	}

	var filesAnalyzed, issuesFound int
	if fa, ok := data["filesAnalyzed"].(int); ok {
		filesAnalyzed = fa
	} else if faF, ok := data["filesAnalyzed"].(float64); ok {
		filesAnalyzed = int(faF)
	}

	if iff, ok := data["issuesFound"].(int); ok {
		issuesFound = iff
	} else if ifF, ok := data["issuesFound"].(float64); ok {
		issuesFound = int(ifF)
	}

	var finishedAt *time.Time
	var durationMs *int64
	if execution.Status != "in_progress" {
		finishedAt = &execution.UpdatedAt
		diff := execution.UpdatedAt.Sub(execution.CreatedAt).Milliseconds()
		durationMs = &diff
	}

	return CliReviewSummaryItem{
		ExecutionUUID:  execution.UUID,
		CorrelationID:  corrID,
		Status:         execution.Status,
		ErrorMessage:   execution.ErrorMessage,
		CreatedAt:      execution.CreatedAt,
		UpdatedAt:      execution.UpdatedAt,
		FinishedAt:     finishedAt,
		DurationMs:     durationMs,
		UserEmail:      userEmail,
		Git:            gitSummary,
		CLIVersion:     cliVersion,
		RepositoryID:   repoID,
		RepositoryName: repoName,
		FilesAnalyzed:  filesAnalyzed,
		IssuesFound:    issuesFound,
		CliAuth:        authSummary,
	}
}

// GetCliReviewsInput parameters to query dashboard reviews.
type GetCliReviewsInput struct {
	OrganizationAndTeamData domain.OrganizationAndTeamData `json:"organizationAndTeamData"`
	RepositoryID            string                         `json:"repositoryId,omitempty"`
	UserEmail               string                         `json:"userEmail,omitempty"`
	Since                   *time.Time                     `json:"since,omitempty"`
	Page                    int                            `json:"page"`
	PageSize                int                            `json:"pageSize"`
}

// IAutomationExecutionQueryService defines interface for querying executions.
type IAutomationExecutionQueryService interface {
	FindCliReviewExecutionsByOrganization(
		ctx context.Context,
		orgAndTeam domain.OrganizationAndTeamData,
		repoID string,
		userEmail string,
		since *time.Time,
		skip int,
		take int,
	) ([]*domain.AutomationExecutionRecord, int, error)
	FindTimelineByExecutionID(ctx context.Context, executionUUID string) ([]CliReviewTimelineEntry, error)
}

// GetCliReviewsUseCase queries paginated list of reviews for dashboard.
type GetCliReviewsUseCase struct {
	queryService IAutomationExecutionQueryService
}

// NewGetCliReviewsUseCase initializes the usecase.
func NewGetCliReviewsUseCase(queryService IAutomationExecutionQueryService) *GetCliReviewsUseCase {
	return &GetCliReviewsUseCase{
		queryService: queryService,
	}
}

// Execute retrieves paginated reviews and formats them for display.
func (uc *GetCliReviewsUseCase) Execute(ctx context.Context, input GetCliReviewsInput) (*PaginatedCliReviewSummaries, error) {
	page := input.Page
	if page < 1 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > 100 {
		pageSize = 100
	}
	skip := (page - 1) * pageSize

	if uc.queryService == nil {
		return &PaginatedCliReviewSummaries{
			Data:     []CliReviewSummaryItem{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
			HasMore:  false,
		}, nil
	}

	records, total, err := uc.queryService.FindCliReviewExecutionsByOrganization(
		ctx,
		input.OrganizationAndTeamData,
		input.RepositoryID,
		input.UserEmail,
		input.Since,
		skip,
		pageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query review executions: %w", err)
	}

	var summaries []CliReviewSummaryItem
	for _, rec := range records {
		summaries = append(summaries, MapExecutionToSummary(rec))
	}

	return &PaginatedCliReviewSummaries{
		Data:     summaries,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		HasMore:  skip+len(summaries) < total,
	}, nil
}

// GetCliReviewByIdInput parameters for fetching review detail.
type GetCliReviewByIdInput struct {
	ExecutionUUID  string `json:"executionUuid"`
	OrganizationID string `json:"organizationId"`
}

// GetCliReviewByIdUseCase retrieves single review detail and its timeline milestones.
type GetCliReviewByIdUseCase struct {
	executionService domain.IAutomationExecutionService
	queryService     IAutomationExecutionQueryService
}

// NewGetCliReviewByIdUseCase initializes the detail usecase.
func NewGetCliReviewByIdUseCase(
	executionService domain.IAutomationExecutionService,
	queryService IAutomationExecutionQueryService,
) *GetCliReviewByIdUseCase {
	return &GetCliReviewByIdUseCase{
		executionService: executionService,
		queryService:     queryService,
	}
}

// Execute looks up a review ensuring caller belongs to the tenant.
func (uc *GetCliReviewByIdUseCase) Execute(ctx context.Context, input GetCliReviewByIdInput) (*CliReviewDetailPayload, error) {
	if uc.executionService == nil {
		return nil, fmt.Errorf("CLI review %s not found", input.ExecutionUUID)
	}

	exec, err := uc.executionService.FindByID(ctx, input.ExecutionUUID)
	if err != nil || exec == nil || exec.Origin != "cli" {
		return nil, fmt.Errorf("CLI review %s not found", input.ExecutionUUID)
	}

	// Security: check tenant ownership
	if exec.OrganizationID != "" && input.OrganizationID != "" && exec.OrganizationID != input.OrganizationID {
		return nil, fmt.Errorf("CLI review %s not found", input.ExecutionUUID)
	}

	var timeline []CliReviewTimelineEntry
	if uc.queryService != nil {
		tl, _ := uc.queryService.FindTimelineByExecutionID(ctx, exec.UUID)
		timeline = tl
	}

	summary := MapExecutionToSummary(exec)

	var resultObj any
	if exec.DataExecution != nil {
		resultObj = exec.DataExecution["result"]
	}

	return &CliReviewDetailPayload{
		CliReviewSummaryItem: summary,
		Timeline:             timeline,
		Result:               resultObj,
	}, nil
}

// InMemoryAutomationQueryService supports testing and dev execution querying.
type InMemoryAutomationQueryService struct {
	mu         sync.RWMutex
	executions map[string]*domain.AutomationExecutionRecord
	timelines  map[string][]CliReviewTimelineEntry
}

// NewInMemoryAutomationQueryService creates a query service instance.
func NewInMemoryAutomationQueryService() *InMemoryAutomationQueryService {
	return &InMemoryAutomationQueryService{
		executions: make(map[string]*domain.AutomationExecutionRecord),
		timelines:  make(map[string][]CliReviewTimelineEntry),
	}
}

func (s *InMemoryAutomationQueryService) RecordExecution(rec *domain.AutomationExecutionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[rec.UUID] = rec
}

func (s *InMemoryAutomationQueryService) RecordTimeline(executionUUID string, item CliReviewTimelineEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timelines[executionUUID] = append(s.timelines[executionUUID], item)
}

func (s *InMemoryAutomationQueryService) FindCliReviewExecutionsByOrganization(
	ctx context.Context,
	orgAndTeam domain.OrganizationAndTeamData,
	repoID string,
	userEmail string,
	since *time.Time,
	skip int,
	take int,
) ([]*domain.AutomationExecutionRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matches []*domain.AutomationExecutionRecord
	for _, e := range s.executions {
		if e.OrganizationID != orgAndTeam.OrganizationID {
			continue
		}
		if repoID != "" && e.RepositoryID != repoID {
			continue
		}
		if userEmail != "" {
			email, _ := e.DataExecution["userEmail"].(string)
			if !strings.EqualFold(email, userEmail) {
				continue
			}
		}
		if since != nil && e.CreatedAt.Before(*since) {
			continue
		}
		matches = append(matches, e)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})

	total := len(matches)
	if skip >= total {
		return []*domain.AutomationExecutionRecord{}, total, nil
	}

	end := skip + take
	if end > total {
		end = total
	}

	return matches[skip:end], total, nil
}

func (s *InMemoryAutomationQueryService) FindTimelineByExecutionID(ctx context.Context, executionUUID string) ([]CliReviewTimelineEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.timelines[executionUUID], nil
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
