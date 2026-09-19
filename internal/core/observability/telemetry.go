package observability

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ReviewStage defines the canonical lifecycle stages of a ScanDrix automated review.
type ReviewStage string

const (
	StageValidatePayload   ReviewStage = "validate_payload"
	StageFetchGitDiff      ReviewStage = "fetch_git_diff"
	StageResolveContext    ReviewStage = "resolve_context"
	StageASTParse          ReviewStage = "ast_parse"
	StageSecurityAnalysis  ReviewStage = "security_analysis"
	StageSynthesizeReview  ReviewStage = "synthesize_review"
	StagePublishFeedback   ReviewStage = "publish_feedback"
)

// StageExecutionRecord captures execution telemetry for a specific pipeline stage.
type StageExecutionRecord struct {
	Stage       ReviewStage    `json:"stage"`
	StartTime   time.Time      `json:"startTime"`
	EndTime     time.Time      `json:"endTime"`
	DurationMs  int64          `json:"durationMs"`
	Status      StatusCode     `json:"status"`
	InputTokens int64          `json:"inputTokens,omitempty"`
	OutputTokens int64         `json:"outputTokens,omitempty"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ReviewFlowDAG records the end-to-end directed acyclic graph of a code review execution.
type ReviewFlowDAG struct {
	ReviewID      string                 `json:"reviewId"`
	TenantID      string                 `json:"tenantId"`
	WorkspaceID   string                 `json:"workspaceId"`
	RepositoryID  string                 `json:"repositoryId"`
	PRNumber      int                    `json:"prNumber"`
	CorrelationID string                 `json:"correlationId"`
	StartTime     time.Time              `json:"startTime"`
	EndTime       time.Time              `json:"endTime"`
	TotalDuration int64                  `json:"totalDurationMs"`
	Stages        []StageExecutionRecord `json:"stages"`
	CriticalPath  []ReviewStage          `json:"criticalPath"`
	TotalTokens   int64                  `json:"totalTokens"`
	Success       bool                   `json:"success"`
	mu            sync.RWMutex
}

// ReviewFlowTracker manages flow tracking across distributed goroutines.
type ReviewFlowTracker struct {
	mu    sync.RWMutex
	flows map[string]*ReviewFlowDAG
}

// NewReviewFlowTracker creates a thread-safe flow tracker.
func NewReviewFlowTracker() *ReviewFlowTracker {
	return &ReviewFlowTracker{
		flows: make(map[string]*ReviewFlowDAG),
	}
}

// StartFlow begins recording a review execution DAG.
func (t *ReviewFlowTracker) StartFlow(
	reviewID string,
	tenantID string,
	workspaceID string,
	repositoryID string,
	prNumber int,
	correlationID string,
) *ReviewFlowDAG {
	flow := &ReviewFlowDAG{
		ReviewID:      reviewID,
		TenantID:      tenantID,
		WorkspaceID:   workspaceID,
		RepositoryID:  repositoryID,
		PRNumber:      prNumber,
		CorrelationID: correlationID,
		StartTime:     time.Now().UTC(),
		Stages:        make([]StageExecutionRecord, 0, 8),
		CriticalPath:  make([]ReviewStage, 0, 8),
		Success:       true,
	}

	t.mu.Lock()
	t.flows[reviewID] = flow
	t.mu.Unlock()

	return flow
}

// RecordStage records the completion of a specific review stage.
func (f *ReviewFlowDAG) RecordStage(
	stage ReviewStage,
	startTime time.Time,
	status StatusCode,
	inputTokens int64,
	outputTokens int64,
	err error,
	meta map[string]any,
) {
	endTime := time.Now().UTC()
	durationMs := endTime.Sub(startTime).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	record := StageExecutionRecord{
		Stage:        stage,
		StartTime:    startTime,
		EndTime:      endTime,
		DurationMs:   durationMs,
		Status:       status,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Metadata:     meta,
	}
	if err != nil {
		record.Error = err.Error()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.Stages = append(f.Stages, record)
	f.CriticalPath = append(f.CriticalPath, stage)
	f.TotalTokens += (inputTokens + outputTokens)
	if status == StatusError {
		f.Success = false
	}
}

// Complete finalizes the review DAG and calculates aggregate metrics.
func (f *ReviewFlowDAG) Complete() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EndTime = time.Now().UTC()
	f.TotalDuration = f.EndTime.Sub(f.StartTime).Milliseconds()
}

// FlowMetrics summarizes multi-tenant review efficiency.
type FlowMetrics struct {
	TotalReviews       int64   `json:"totalReviews"`
	SuccessfulReviews  int64   `json:"successfulReviews"`
	FailedReviews      int64   `json:"failedReviews"`
	AverageDurationMs  float64 `json:"averageDurationMs"`
	TotalTokensSpent   int64   `json:"totalTokensSpent"`
	StageBottleneck    ReviewStage `json:"stageBottleneck"`
}

// GetAggregateMetrics computes operational metrics across tracked flows.
func (t *ReviewFlowTracker) GetAggregateMetrics() FlowMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var metrics FlowMetrics
	metrics.TotalReviews = int64(len(t.flows))
	if metrics.TotalReviews == 0 {
		return metrics
	}

	var totalDuration float64
	stageDurations := make(map[ReviewStage]int64)

	for _, flow := range t.flows {
		flow.mu.RLock()
		if flow.Success {
			metrics.SuccessfulReviews++
		} else {
			metrics.FailedReviews++
		}
		totalDuration += float64(flow.TotalDuration)
		metrics.TotalTokensSpent += flow.TotalTokens

		for _, st := range flow.Stages {
			stageDurations[st.Stage] += st.DurationMs
		}
		flow.mu.RUnlock()
	}

	metrics.AverageDurationMs = totalDuration / float64(metrics.TotalReviews)

	// Identify highest aggregate latency bottleneck
	var maxStage ReviewStage
	var maxDuration int64
	for st, dur := range stageDurations {
		if dur > maxDuration {
			maxDuration = dur
			maxStage = st
		}
	}
	metrics.StageBottleneck = maxStage

	return metrics
}

// TraceSpanWithContext creates a span pre-populated with multi-tenant attribution tags.
func TraceSpanWithContext(
	ctx context.Context,
	tracer Tracer,
	name string,
	tenantID string,
	workspaceID string,
	repoID string,
	prNumber int,
) (context.Context, Span) {
	childCtx, span := tracer.Start(ctx, name, WithAttributes(map[string]any{
		TenantID:          tenantID,
		WorkspaceID:       workspaceID,
		RepositoryID:      repoID,
		PullRequestNumber: prNumber,
	}))
	return childCtx, span
}

// FormatTraceID returns a standardized logging identifier.
func FormatTraceID(sc SpanContext) string {
	if !sc.IsValid() {
		return "trace_uninitialized"
	}
	return fmt.Sprintf("trace_%s_%s", sc.TraceID[:8], sc.SpanID[:8])
}
