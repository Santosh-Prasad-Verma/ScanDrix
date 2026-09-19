package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// TransactionalWorkflowWriter supports executing writes within an explicit database transaction.
type TransactionalWorkflowWriter interface {
	CreateJobTx(ctx context.Context, tx pgx.Tx, job *WorkflowJobModel) error
	CreateOutboxMessageTx(ctx context.Context, tx pgx.Tx, msg *OutboxMessageModel) error
}

// WorkflowJobQueueService mirrors ScanDrix WorkflowJobQueueService.
// Implements the transactional outbox pattern to guarantee at-least-once job delivery.
type WorkflowJobQueueService struct {
	pool   *pgxpool.Pool
	writer TransactionalWorkflowWriter
	repo   JobRepository
}

// NewWorkflowJobQueueService instantiates a workflow job queue service.
func NewWorkflowJobQueueService(
	pool *pgxpool.Pool,
	writer TransactionalWorkflowWriter,
	repo JobRepository,
) *WorkflowJobQueueService {
	return &WorkflowJobQueueService{
		pool:   pool,
		writer: writer,
		repo:   repo,
	}
}

// Enqueue transactionally creates a WorkflowJob and an OutboxMessage in PostgreSQL.
func (s *WorkflowJobQueueService) Enqueue(
	ctx context.Context,
	job *WorkflowJobModel,
) (uuid.UUID, error) {
	if job.UUID == uuid.Nil {
		job.UUID = uuid.New()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = domain.JobStatusPending
	}
	if job.MaxRetries <= 0 {
		job.MaxRetries = 5
	}

	exchange := "workflow.exchange"
	routingKey := fmt.Sprintf("workflow.jobs.created.%s", job.WorkflowType)

	var orgIDStr, teamIDStr string
	if job.OrganizationID != nil {
		orgIDStr = job.OrganizationID.String()
	}
	if job.TeamID != nil {
		teamIDStr = job.TeamID.String()
	}

	payload := map[string]any{
		"jobId":          job.UUID.String(),
		"correlationId":  job.CorrelationID,
		"workflowType":   string(job.WorkflowType),
		"handlerType":    string(job.HandlerType),
		"organizationId": orgIDStr,
		"teamId":         teamIDStr,
	}

	outboxMsg := &OutboxMessageModel{
		UUID:          uuid.New(),
		CreatedAt:     now,
		UpdatedAt:     now,
		JobID:         &job.UUID,
		Exchange:      exchange,
		RoutingKey:    routingKey,
		Payload:       payload,
		Status:        domain.OutboxStatusReady,
		Attempts:      0,
		NextAttemptAt: now,
	}

	if s.pool != nil && s.writer != nil {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed starting database transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if err := s.writer.CreateJobTx(ctx, tx, job); err != nil {
			return uuid.Nil, fmt.Errorf("failed creating workflow job in tx: %w", err)
		}
		if err := s.writer.CreateOutboxMessageTx(ctx, tx, outboxMsg); err != nil {
			return uuid.Nil, fmt.Errorf("failed creating outbox message in tx: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return uuid.Nil, fmt.Errorf("failed committing workflow job tx: %w", err)
		}
	}

	return job.UUID, nil
}

// Schedule enqueues a job scheduled to run in the future.
func (s *WorkflowJobQueueService) Schedule(
	ctx context.Context,
	job *WorkflowJobModel,
	scheduledAt time.Time,
) (uuid.UUID, error) {
	job.ScheduledAt = &scheduledAt
	return s.Enqueue(ctx, job)
}

// Cancel cancels a pending or waiting job.
func (s *WorkflowJobQueueService) Cancel(ctx context.Context, jobID uuid.UUID) error {
	updates := map[string]any{
		"status":       domain.JobStatusCancelled,
		"completed_at": time.Now().UTC(),
		"updated_at":   time.Now().UTC(),
	}
	return s.repo.Update(ctx, jobID, updates)
}

// GetJobStatus retrieves the current job state.
func (s *WorkflowJobQueueService) GetJobStatus(ctx context.Context, jobID uuid.UUID) (*WorkflowJobModel, error) {
	return s.repo.FindOne(ctx, jobID)
}
