package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/core/domain"
)

// CheckRunName is the single stable check-run name for every pipeline execution.
const DrixyCheckRunName = "Drixy Code Review"

// CheckStatus mirrors ScanDrix CheckStatus.
type CheckStatus string

const (
	CheckStatusQueued     CheckStatus = "queued"
	CheckStatusInProgress CheckStatus = "in_progress"
	CheckStatusCompleted  CheckStatus = "completed"
)

// CheckConclusion mirrors ScanDrix CheckConclusion.
type CheckConclusion string

const (
	CheckConclusionSuccess        CheckConclusion = "success"
	CheckConclusionFailure        CheckConclusion = "failure"
	CheckConclusionNeutral        CheckConclusion = "neutral"
	CheckConclusionCancelled      CheckConclusion = "cancelled"
	CheckConclusionTimedOut       CheckConclusion = "timed_out"
	CheckConclusionActionRequired CheckConclusion = "action_required"
	CheckConclusionSkipped        CheckConclusion = "skipped"
)

// CheckStageInfo mirrors checkStageMap from ScanDrix pipeline-checks.service.ts.
type CheckStageInfo struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

var CheckStageMap = map[string]CheckStageInfo{
	"_pipelineStart": {
		Title:   "Code Review Starting",
		Summary: "Drixy is analyzing your code changes...",
	},
	"PRLevelReviewStage": {
		Title:   "Code Review In Progress",
		Summary: "Reviewing PR-level changes: analyzing overall intent, descriptions, and cross-file impacts.",
	},
	"FileAnalysisStage": {
		Title:   "Code Review In Progress",
		Summary: "Reviewing file-level changes: analyzing each modified file for issues and improvement suggestions.",
	},
	"_pipelineEndSuccess": {
		Title:   "Code Review Complete",
		Summary: "Review finished successfully. Suggestions (if any) were posted as PR/file comments.",
	},
	"_pipelineEndFailure": {
		Title:   "Code Review Failed",
		Summary: "An error occurred during the review. Please check the logs for details.",
	},
	"_pipelineEndPartial": {
		Title:   "Code Review Completed with Warnings",
		Summary: "Review finished, but one or more non-critical stages failed. See details below.",
	},
	"_pipelineEndSkipped": {
		Title:   "Code Review Skipped",
		Summary: "Review skipped.",
	},
}

// WorkflowPausedError mirrors ScanDrix WorkflowPausedError.
type WorkflowPausedError struct {
	EventType string         `json:"event_type"`
	EventKey  string         `json:"event_key"`
	StageName string         `json:"stage_name"`
	TaskID    string         `json:"task_id,omitempty"`
	TimeoutMs int64          `json:"timeout_ms"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Context   any            `json:"context,omitempty"`
}

func (e *WorkflowPausedError) Error() string {
	return fmt.Sprintf("Workflow paused at stage %s waiting for event %s (key=%s, timeout=%dms)",
		e.StageName, e.EventType, e.EventKey, e.TimeoutMs)
}

// CheckRunRecord encapsulates external SCM check run status.
type CheckRunRecord struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	HeadSHA     string          `json:"head_sha"`
	Status      CheckStatus     `json:"status"`
	Conclusion  CheckConclusion `json:"conclusion,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary"`
	Text        string          `json:"text,omitempty"`
	DetailsURL  string          `json:"details_url,omitempty"`
}

// ChecksAdapter defines interface for creating/updating SCM checks (GitHub, GitLab, Azure, Bitbucket).
type ChecksAdapter interface {
	CreateOrUpdateCheckRun(ctx context.Context, check *CheckRunRecord) (*CheckRunRecord, error)
}

// NullChecksAdapter provides a no-op implementation when SCM check reporting is disabled.
type NullChecksAdapter struct{}

func (a *NullChecksAdapter) CreateOrUpdateCheckRun(ctx context.Context, check *CheckRunRecord) (*CheckRunRecord, error) {
	return check, nil
}

// ChecksAdapterFactory manages provider-specific checks adapters with fallback to NullChecksAdapter.
type ChecksAdapterFactory struct {
	adapters map[string]ChecksAdapter
}

// NewChecksAdapterFactory constructs a new ChecksAdapterFactory.
func NewChecksAdapterFactory() *ChecksAdapterFactory {
	return &ChecksAdapterFactory{
		adapters: make(map[string]ChecksAdapter),
	}
}

// RegisterAdapter registers a provider adapter.
func (f *ChecksAdapterFactory) RegisterAdapter(providerKey string, adapter ChecksAdapter) {
	f.adapters[providerKey] = adapter
}

// GetAdapter returns the checks adapter for a provider key, falling back to NullChecksAdapter.
func (f *ChecksAdapterFactory) GetAdapter(providerKey string) ChecksAdapter {
	if adapter, ok := f.adapters[providerKey]; ok {
		return adapter
	}
	return &NullChecksAdapter{}
}

// PipelineChecksService mirrors ScanDrix PipelineChecksService.
type PipelineChecksService struct {
	adapter ChecksAdapter
}

// NewPipelineChecksService instantiates a pipeline checks service.
func NewPipelineChecksService(adapter ChecksAdapter) *PipelineChecksService {
	return &PipelineChecksService{adapter: adapter}
}

// UpdatePipelineStart creates or updates the check run when pipeline starts.
func (s *PipelineChecksService) UpdatePipelineStart(
	ctx context.Context,
	headSHA string,
	detailsURL string,
) (*CheckRunRecord, error) {
	if s.adapter == nil {
		return nil, nil
	}

	info := CheckStageMap["_pipelineStart"]
	check := &CheckRunRecord{
		Name:       DrixyCheckRunName,
		HeadSHA:    headSHA,
		Status:     CheckStatusInProgress,
		StartedAt:  time.Now().UTC(),
		Title:      info.Title,
		Summary:    info.Summary,
		DetailsURL: detailsURL,
	}

	return s.adapter.CreateOrUpdateCheckRun(ctx, check)
}

// UpdateStageProgress updates the check run for an active pipeline stage.
func (s *PipelineChecksService) UpdateStageProgress(
	ctx context.Context,
	checkID string,
	headSHA string,
	stageName string,
) (*CheckRunRecord, error) {
	if s.adapter == nil {
		return nil, nil
	}

	info, exists := CheckStageMap[stageName]
	if !exists {
		info = CheckStageInfo{
			Title:   "Code Review In Progress",
			Summary: fmt.Sprintf("Executing stage: %s", stageName),
		}
	}

	check := &CheckRunRecord{
		ID:        checkID,
		Name:      DrixyCheckRunName,
		HeadSHA:   headSHA,
		Status:    CheckStatusInProgress,
		Title:     info.Title,
		Summary:   info.Summary,
		StartedAt: time.Now().UTC(),
	}

	return s.adapter.CreateOrUpdateCheckRun(ctx, check)
}

// UpdatePipelineEnd finalizes the check run with final status and conclusion.
func (s *PipelineChecksService) UpdatePipelineEnd(
	ctx context.Context,
	checkID string,
	headSHA string,
	status domain.AutomationStatus,
	errors []PipelineError,
) (*CheckRunRecord, error) {
	if s.adapter == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	var conclusion CheckConclusion
	var stageKey string

	switch status {
	case domain.AutomationStatusSuccess:
		conclusion = CheckConclusionSuccess
		stageKey = "_pipelineEndSuccess"
	case domain.AutomationStatusSkipped:
		conclusion = CheckConclusionSkipped
		stageKey = "_pipelineEndSkipped"
	case domain.AutomationStatusFailure:
		conclusion = CheckConclusionFailure
		stageKey = "_pipelineEndFailure"
	default:
		if len(errors) > 0 {
			conclusion = CheckConclusionNeutral
			stageKey = "_pipelineEndPartial"
		} else {
			conclusion = CheckConclusionSuccess
			stageKey = "_pipelineEndSuccess"
		}
	}

	info := CheckStageMap[stageKey]
	var textSummary strings.Builder
	textSummary.WriteString(info.Summary)

	if len(errors) > 0 {
		textSummary.WriteString("\n\n### Pipeline Diagnostics\n")
		for _, e := range errors {
			textSummary.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", e.Stage, e.Severity, e.Error))
		}
	}

	check := &CheckRunRecord{
		ID:          checkID,
		Name:        DrixyCheckRunName,
		HeadSHA:     headSHA,
		Status:      CheckStatusCompleted,
		Conclusion:  conclusion,
		CompletedAt: &now,
		Title:       info.Title,
		Summary:     info.Summary,
		Text:        textSummary.String(),
	}

	return s.adapter.CreateOrUpdateCheckRun(ctx, check)
}
