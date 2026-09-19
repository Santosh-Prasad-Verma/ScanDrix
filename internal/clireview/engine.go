package clireview

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

// JobRecord tracks asynchronous CLI review jobs.
type JobRecord struct {
	ID            uuid.UUID
	CorrelationID string
	Status        string // "PENDING", "PROCESSING", "COMPLETED", "FAILED"
	Result        *CliReviewResponse
	Error         string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	Input         EnqueueCliReviewInput
}

// Engine coordinates CLI review execution and asynchronous job queues.
type Engine struct {
	rulesEvaluator *rules.Evaluator
	authLimiter    *AuthenticatedRateLimiter
	jobs           map[uuid.UUID]*JobRecord
	mu             sync.RWMutex
}

// NewEngine creates an initialized CLI review engine.
func NewEngine(evaluator *rules.Evaluator, authLimiter *AuthenticatedRateLimiter) *Engine {
	if authLimiter == nil {
		authLimiter = NewAuthenticatedRateLimiter(10)
	}
	return &Engine{
		rulesEvaluator: evaluator,
		authLimiter:    authLimiter,
		jobs:           make(map[uuid.UUID]*JobRecord),
	}
}

// ExecuteReview synchronously processes a CLI review request.
func (e *Engine) ExecuteReview(ctx context.Context, input CliReviewInput) (*CliReviewResponse, error) {
	startTime := time.Now().UTC()

	// 1. Prepare files from diff / file list
	files := PrepareCliFiles(input)
	if len(files) == 0 {
		return &CliReviewResponse{
			Summary:       "ScanDrix: No files or changes detected to review.",
			Issues:        nil,
			FilesAnalyzed: 0,
			Duration:      time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 2. Evaluate AST security & quality rules
	issues := EvaluateRulesAgainstFiles(e.rulesEvaluator, files, input.Config)

	// 3. Format CLI response
	res := FormatCliOutput(issues, len(files), startTime)
	return &res, nil
}

// EnqueueReview submits a CLI review for background execution.
func (e *Engine) EnqueueReview(ctx context.Context, input EnqueueCliReviewInput) (*EnqueueCliReviewResult, error) {
	if !input.IsTrialMode && input.TeamID != "" {
		if !e.authLimiter.Acquire(input.TeamID) {
			return nil, errors.New("rate_limited: team maximum concurrent reviews reached")
		}
	}

	jobID := uuid.New()
	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	record := &JobRecord{
		ID:            jobID,
		CorrelationID: correlationID,
		Status:        "PENDING",
		CreatedAt:     time.Now().UTC(),
		Input:         input,
	}

	e.mu.Lock()
	e.jobs[jobID] = record
	e.mu.Unlock()

	// Execute asynchronously
	go func() {
		defer func() {
			if !input.IsTrialMode && input.TeamID != "" {
				e.authLimiter.Release(input.TeamID)
			}
		}()

		now := time.Now().UTC()
		e.mu.Lock()
		record.Status = "PROCESSING"
		record.StartedAt = &now
		e.mu.Unlock()

		// Execute review
		res, err := e.ExecuteReview(context.Background(), input.Input)

		finishTime := time.Now().UTC()
		e.mu.Lock()
		defer e.mu.Unlock()

		record.CompletedAt = &finishTime
		if err != nil {
			record.Status = "FAILED"
			record.Error = err.Error()
		} else {
			record.Status = "COMPLETED"
			record.Result = res
		}
	}()

	return &EnqueueCliReviewResult{
		JobID:         jobID,
		CorrelationID: correlationID,
	}, nil
}

// GetJobStatus retrieves current status and results for a job.
func (e *Engine) GetJobStatus(jobID uuid.UUID) (*try.JobStatusResponse, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rec, ok := e.jobs[jobID]
	if !ok {
		return nil, false
	}

	resp := &try.JobStatusResponse{
		JobID:     rec.ID.String(),
		Status:    rec.Status,
		Error:     rec.Error,
		CreatedAt: rec.CreatedAt.Format(time.RFC3339),
	}

	if rec.StartedAt != nil {
		s := rec.StartedAt.Format(time.RFC3339)
		resp.StartedAt = &s
	}
	if rec.CompletedAt != nil {
		c := rec.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &c
	}

	if rec.Result != nil {
		var mappedIssues []try.ReviewIssue
		for _, i := range rec.Result.Issues {
			endLine := 0
			if i.EndLine != nil {
				endLine = *i.EndLine
			}
			mappedIssues = append(mappedIssues, try.ReviewIssue{
				File:           i.File,
				Line:           i.Line,
				EndLine:        endLine,
				Severity:       i.Severity,
				Category:       i.Category,
				Message:        i.Message,
				Suggestion:     i.Suggestion,
				Recommendation: i.Recommendation,
				RuleID:         i.RuleID,
			})
		}
		resp.Result = &try.ReviewResult{
			Summary:       rec.Result.Summary,
			Issues:        mappedIssues,
			FilesAnalyzed: rec.Result.FilesAnalyzed,
			Duration:      rec.Result.Duration,
		}
	}

	if rec.Input.PublicPR != nil {
		resp.PublicPR = rec.Input.PublicPR
		resp.PublicDiff = rec.Input.PublicDiff
	}

	return resp, true
}

// WaitForJob polls job status until completion, failure, or context cancellation.
func (e *Engine) WaitForJob(ctx context.Context, jobID uuid.UUID, pollInterval time.Duration) (*CliReviewResponse, error) {
	if pollInterval <= 0 {
		pollInterval = 50 * time.Millisecond
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			e.mu.RLock()
			rec, ok := e.jobs[jobID]
			e.mu.RUnlock()

			if !ok {
				return nil, errors.New("job not found")
			}

			if rec.Status == "COMPLETED" {
				return rec.Result, nil
			}
			if rec.Status == "FAILED" {
				return nil, errors.New(rec.Error)
			}
		}
	}
}
