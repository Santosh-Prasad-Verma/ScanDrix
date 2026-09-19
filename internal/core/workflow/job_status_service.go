package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// JobStatusService mirrors ScanDrix JobStatusService for operational workflow introspection and telemetry.
type JobStatusService struct {
	pool    *pgxpool.Pool
	jobRepo JobRepository
}

// NewJobStatusService instantiates a new job status service.
func NewJobStatusService(pool *pgxpool.Pool, jobRepo JobRepository) *JobStatusService {
	return &JobStatusService{
		pool:    pool,
		jobRepo: jobRepo,
	}
}

// GetJobStatus retrieves the current job model by UUID.
func (s *JobStatusService) GetJobStatus(ctx context.Context, jobID uuid.UUID) (*WorkflowJobModel, error) {
	return s.jobRepo.FindOne(ctx, jobID)
}

// GetJobDetail returns detailed job state, execution timeline, and completion percentage.
func (s *JobStatusService) GetJobDetail(ctx context.Context, jobID uuid.UUID) (*JobDetailResponse, error) {
	job, err := s.jobRepo.FindOne(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving job status: %w", err)
	}
	if job == nil {
		return nil, nil
	}

	history, err := s.getExecutionHistory(ctx, jobID)
	if err != nil {
		history = []JobExecutionHistoryEntry{}
	}

	progress := s.calculateProgress(job)

	return &JobDetailResponse{
		Job:              job,
		ExecutionHistory: history,
		ProgressPercent:  progress,
	}, nil
}

func (s *JobStatusService) calculateProgress(job *WorkflowJobModel) int {
	if job.Status == domain.JobStatusCompleted {
		return 100
	}
	if job.Status == domain.JobStatusFailed || job.Status == domain.JobStatusCancelled {
		return 0
	}

	// Approximate progress based on standard review pipeline stages
	if job.CurrentStage != nil {
		switch *job.CurrentStage {
		case "InitStage", "_pipelineStart":
			return 10
		case "FetchPRFilesStage":
			return 25
		case "FilterFilesStage":
			return 40
		case "AnalyzeChangesStage", "ASTGraphStage":
			return 55
		case "PRLevelReviewStage":
			return 70
		case "FileAnalysisStage":
			return 85
		case "SummaryGenerationStage":
			return 95
		}
	}

	if job.Status == domain.JobStatusProcessing {
		return 50
	}
	return 5
}

func (s *JobStatusService) getExecutionHistory(ctx context.Context, jobID uuid.UUID) ([]JobExecutionHistoryEntry, error) {
	if s.pool == nil {
		return nil, nil
	}

	query := `
		SELECT metadata->'executionHistory'
		FROM workflow_jobs
		WHERE uuid = $1
	`

	var historyBytes []byte
	err := s.pool.QueryRow(ctx, query, jobID).Scan(&historyBytes)
	if err != nil || len(historyBytes) == 0 {
		return nil, err
	}

	var history []JobExecutionHistoryEntry
	if err := json.Unmarshal(historyBytes, &history); err != nil {
		return nil, err
	}
	return history, nil
}

// GetMetrics returns real-time workflow queue, processing, and error telemetry directly from PostgreSQL.
func (s *JobStatusService) GetMetrics(ctx context.Context) (*WorkflowMetrics, error) {
	metrics := &WorkflowMetrics{
		ByStatus: make(map[string]int),
	}

	if s.pool == nil {
		metrics.SuccessRate = 100.0
		return metrics, nil
	}

	// 1. Status aggregates
	statusQuery := `
		SELECT status, COUNT(*)
		FROM workflow_jobs
		GROUP BY status
	`
	rows, err := s.pool.Query(ctx, statusQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err == nil {
				metrics.ByStatus[status] = count
				if status == string(domain.JobStatusPending) {
					metrics.QueueSize = count
				} else if status == string(domain.JobStatusProcessing) {
					metrics.ProcessingCount = count
				}
			}
		}
	}

	// 2. Completed & Failed Today
	todayQuery := `
		SELECT 
			COALESCE(SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END), 0) AS completed_today,
			COALESCE(SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END), 0) AS failed_today,
			COALESCE(AVG(CASE WHEN status = 'COMPLETED' AND started_at IS NOT NULL AND completed_at IS NOT NULL 
			                 THEN EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000 ELSE NULL END), 0) AS avg_duration_ms
		FROM workflow_jobs
		WHERE completed_at >= CURRENT_DATE
	`
	_ = s.pool.QueryRow(ctx, todayQuery).Scan(&metrics.CompletedToday, &metrics.FailedToday, &metrics.AverageProcessingTime)

	totalToday := metrics.CompletedToday + metrics.FailedToday
	if totalToday > 0 {
		metrics.SuccessRate = (float64(metrics.CompletedToday) / float64(totalToday)) * 100.0
	} else {
		metrics.SuccessRate = 100.0
	}

	// 3. Stale Processing Jobs
	staleQuery := `
		SELECT COUNT(*)
		FROM workflow_jobs
		WHERE status = 'PROCESSING'
		  AND updated_at < NOW() - INTERVAL '3 hours'
	`
	_ = s.pool.QueryRow(ctx, staleQuery).Scan(&metrics.StaleProcessingJobs)

	// 4. Inbox & Outbox Lag
	inboxLagQuery := `SELECT COUNT(*) FROM inbox_messages WHERE status = 'PROCESSING'`
	_ = s.pool.QueryRow(ctx, inboxLagQuery).Scan(&metrics.InboxLag)

	outboxLagQuery := `SELECT COUNT(*) FROM outbox_messages WHERE status = 'READY'`
	_ = s.pool.QueryRow(ctx, outboxLagQuery).Scan(&metrics.OutboxLag)

	return metrics, nil
}
