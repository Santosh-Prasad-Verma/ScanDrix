package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PostgresWorkflowJobRepository provides production TypeORM-compatible PostgreSQL storage for workflow_jobs.
type PostgresWorkflowJobRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresWorkflowJobRepository instantiates a new workflow job repository.
func NewPostgresWorkflowJobRepository(pool *pgxpool.Pool) *PostgresWorkflowJobRepository {
	return &PostgresWorkflowJobRepository{pool: pool}
}

// CreateJobTx inserts a new workflow job record within a transaction.
func (r *PostgresWorkflowJobRepository) CreateJobTx(ctx context.Context, tx pgx.Tx, job *WorkflowJobModel) error {
	payloadJSON, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("failed marshaling job payload: %w", err)
	}

	metadataJSON, err := json.Marshal(job.Metadata)
	if err != nil {
		return fmt.Errorf("failed marshaling job metadata: %w", err)
	}

	var waitingJSON []byte
	if job.WaitingForEvent != nil {
		waitingJSON, _ = json.Marshal(job.WaitingForEvent)
	}

	query := `
		INSERT INTO workflow_jobs (
			uuid, created_at, updated_at, correlation_id, workflow_type,
			handler_type, payload, status, priority, retry_count,
			max_retries, organization_id, team_id, scheduled_at,
			started_at, completed_at, current_stage, metadata, waiting_for_event
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18, $19
		)
	`

	_, err = tx.Exec(ctx, query,
		job.UUID, job.CreatedAt, job.UpdatedAt, job.CorrelationID, string(job.WorkflowType),
		string(job.HandlerType), payloadJSON, string(job.Status), job.Priority, job.RetryCount,
		job.MaxRetries, job.OrganizationID, job.TeamID, job.ScheduledAt,
		job.StartedAt, job.CompletedAt, job.CurrentStage, metadataJSON, waitingJSON,
	)
	if err != nil {
		return fmt.Errorf("failed inserting workflow_job in tx: %w", err)
	}
	return nil
}

// FindOne retrieves a workflow job by UUID.
func (r *PostgresWorkflowJobRepository) FindOne(ctx context.Context, id uuid.UUID) (*WorkflowJobModel, error) {
	query := `
		SELECT uuid, created_at, updated_at, correlation_id, workflow_type,
		       handler_type, payload, status, priority, retry_count,
		       max_retries, organization_id, team_id, error_classification,
		       last_error, scheduled_at, started_at, completed_at, current_stage,
		       metadata, waiting_for_event
		FROM workflow_jobs
		WHERE uuid = $1
	`

	row := r.pool.QueryRow(ctx, query, id)

	var job WorkflowJobModel
	var wType, hType, status string
	var errClass, lastErr, stage *string
	var payloadBytes, metaBytes, waitBytes []byte

	err := row.Scan(
		&job.UUID, &job.CreatedAt, &job.UpdatedAt, &job.CorrelationID, &wType,
		&hType, &payloadBytes, &status, &job.Priority, &job.RetryCount,
		&job.MaxRetries, &job.OrganizationID, &job.TeamID, &errClass,
		&lastErr, &job.ScheduledAt, &job.StartedAt, &job.CompletedAt, &stage,
		&metaBytes, &waitBytes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed scanning workflow_job %s: %w", id, err)
	}

	job.WorkflowType = domain.WorkflowType(wType)
	job.HandlerType = domain.HandlerType(hType)
	job.Status = domain.JobStatus(status)
	job.LastError = lastErr
	job.CurrentStage = stage
	if errClass != nil {
		ec := domain.ErrorClassification(*errClass)
		job.ErrorClassification = &ec
	}

	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &job.Payload)
	}
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &job.Metadata)
	}
	if len(waitBytes) > 0 {
		var spec WaitingForEventSpec
		if err := json.Unmarshal(waitBytes, &spec); err == nil {
			job.WaitingForEvent = &spec
		}
	}

	return &job, nil
}

// FindByCorrelationID retrieves a workflow job by correlation ID.
func (r *PostgresWorkflowJobRepository) FindByCorrelationID(ctx context.Context, correlationID string) (*WorkflowJobModel, error) {
	query := `
		SELECT uuid
		FROM workflow_jobs
		WHERE correlation_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, query, correlationID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.FindOne(ctx, id)
}

// Update modifies dynamic fields of a workflow job.
func (r *PostgresWorkflowJobRepository) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+2)
	argIdx := 1

	for k, v := range updates {
		val := v
		if k == "payload" || k == "metadata" || k == "waiting_for_event" || k == "pipeline_state" {
			if v != nil {
				bytes, err := json.Marshal(v)
				if err == nil {
					val = bytes
				}
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, argIdx))
		args = append(args, val)
		argIdx++
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now().UTC())
	argIdx++

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE workflow_jobs
		SET %s
		WHERE uuid = $%d
	`, joinStrings(setClauses, ", "), argIdx)

	_, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed updating workflow_job %s: %w", id, err)
	}
	return nil
}

// FindWaitingJobsByEvent finds jobs waiting for a specific event type and key.
func (r *PostgresWorkflowJobRepository) FindWaitingJobsByEvent(ctx context.Context, eventType, eventKey string) ([]*WorkflowJobModel, error) {
	query := `
		SELECT uuid
		FROM workflow_jobs
		WHERE status = 'WAITING_FOR_EVENT'
		  AND waiting_for_event->>'eventType' = $1
		  AND waiting_for_event->>'eventKey' = $2
	`

	rows, err := r.pool.Query(ctx, query, eventType, eventKey)
	if err != nil {
		return nil, fmt.Errorf("failed querying waiting jobs for event: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}

	jobs := make([]*WorkflowJobModel, 0, len(ids))
	for _, id := range ids {
		job, err := r.FindOne(ctx, id)
		if err == nil && job != nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// ReapStaleJobs marks abandoned PROCESSING jobs as FAILED.
func (r *PostgresWorkflowJobRepository) ReapStaleJobs(ctx context.Context, staleThreshold time.Duration) (int, error) {
	thresholdTime := time.Now().UTC().Add(-staleThreshold)

	query := `
		UPDATE workflow_jobs
		SET status = 'FAILED',
		    error_classification = 'PERMANENT',
		    last_error = 'Job timed out and was abandoned in PROCESSING status',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE status = 'PROCESSING'
		  AND updated_at < $1
	`

	tag, err := r.pool.Exec(ctx, query, thresholdTime)
	if err != nil {
		return 0, fmt.Errorf("failed reaping stale workflow jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// PostgresOutboxRepository provides transactional outbox storage in PostgreSQL.
type PostgresOutboxRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresOutboxRepository instantiates a new outbox repository.
func NewPostgresOutboxRepository(pool *pgxpool.Pool) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{pool: pool}
}

// CreateOutboxMessageTx inserts an outbox message within a transaction.
func (r *PostgresOutboxRepository) CreateOutboxMessageTx(ctx context.Context, tx pgx.Tx, msg *OutboxMessageModel) error {
	payloadJSON, err := json.Marshal(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed marshaling outbox payload: %w", err)
	}

	query := `
		INSERT INTO outbox_messages (
			uuid, created_at, updated_at, job_id, exchange,
			routing_key, payload, status, attempts, next_attempt_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10
		)
	`

	_, err = tx.Exec(ctx, query,
		msg.UUID, msg.CreatedAt, msg.UpdatedAt, msg.JobID, msg.Exchange,
		msg.RoutingKey, payloadJSON, string(msg.Status), msg.Attempts, msg.NextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf("failed inserting outbox_message in tx: %w", err)
	}
	return nil
}

// FindReadyMessages finds batch of ready outbox messages with SKIP LOCKED.
func (r *PostgresOutboxRepository) FindReadyMessages(ctx context.Context, batchSize int) ([]*OutboxMessageModel, error) {
	now := time.Now().UTC()
	query := `
		SELECT uuid, created_at, updated_at, job_id, exchange,
		       routing_key, payload, status, attempts, next_attempt_at,
		       locked_at, locked_by, last_error, processed_at
		FROM outbox_messages
		WHERE status = 'READY'
		  AND next_attempt_at <= $1
		ORDER BY next_attempt_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.pool.Query(ctx, query, now, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed querying ready outbox messages: %w", err)
	}
	defer rows.Close()

	var messages []*OutboxMessageModel
	for rows.Next() {
		var msg OutboxMessageModel
		var status string
		var payloadBytes []byte

		err := rows.Scan(
			&msg.UUID, &msg.CreatedAt, &msg.UpdatedAt, &msg.JobID, &msg.Exchange,
			&msg.RoutingKey, &payloadBytes, &status, &msg.Attempts, &msg.NextAttemptAt,
			&msg.LockedAt, &msg.LockedBy, &msg.LastError, &msg.ProcessedAt,
		)
		if err == nil {
			msg.Status = domain.OutboxStatus(status)
			if len(payloadBytes) > 0 {
				_ = json.Unmarshal(payloadBytes, &msg.Payload)
			}
			messages = append(messages, &msg)
		}
	}

	return messages, nil
}

// MarkSent marks an outbox message as sent.
func (r *PostgresOutboxRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE outbox_messages
		SET status = 'SENT',
		    processed_at = $1,
		    updated_at = $1
		WHERE uuid = $2
	`
	_, err := r.pool.Exec(ctx, query, now, id)
	return err
}

// MarkFailed marks an outbox message as permanently failed.
func (r *PostgresOutboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	now := time.Now().UTC()
	query := `
		UPDATE outbox_messages
		SET status = 'FAILED',
		    last_error = $1,
		    updated_at = $2
		WHERE uuid = $3
	`
	_, err := r.pool.Exec(ctx, query, lastError, now, id)
	return err
}

// ScheduleRetry updates next attempt time and error on failure.
func (r *PostgresOutboxRepository) ScheduleRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, attempts int, lastError string) error {
	now := time.Now().UTC()
	query := `
		UPDATE outbox_messages
		SET next_attempt_at = $1,
		    attempts = $2,
		    last_error = $3,
		    updated_at = $4
		WHERE uuid = $5
	`
	_, err := r.pool.Exec(ctx, query, nextAttemptAt, attempts, lastError, now, id)
	return err
}

// PostgresInboxRepository provides idempotency and deduplication storage in PostgreSQL.
type PostgresInboxRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresInboxRepository instantiates a new inbox repository.
func NewPostgresInboxRepository(pool *pgxpool.Pool) *PostgresInboxRepository {
	return &PostgresInboxRepository{pool: pool}
}

// Claim attempts to claim a message atomically using ON CONFLICT DO UPDATE.
func (r *PostgresInboxRepository) Claim(ctx context.Context, messageID, consumerID string, jobID *uuid.UUID) (bool, error) {
	now := time.Now().UTC()
	query := `
		INSERT INTO inbox_messages (
			uuid, created_at, updated_at, message_id, consumer_id,
			job_id, status, attempts, next_attempt_at, locked_at
		) VALUES (
			gen_random_uuid(), $1, $1, $2, $3,
			$4, 'PROCESSING', 1, $1, $1
		)
		ON CONFLICT (consumer_id, message_id) DO UPDATE
		SET attempts = inbox_messages.attempts + 1,
		    updated_at = $1
		WHERE inbox_messages.status != 'PROCESSED'
		  AND (inbox_messages.locked_at IS NULL OR inbox_messages.locked_at < $1 - INTERVAL '15 minutes')
		RETURNING (inbox_messages.status = 'PROCESSING')
	`

	var claimed bool
	err := r.pool.QueryRow(ctx, query, now, messageID, consumerID, jobID).Scan(&claimed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row exists and condition was not met (already PROCESSED or currently locked)
			return false, nil
		}
		return false, fmt.Errorf("inbox claim error for message %s: %w", messageID, err)
	}

	return claimed, nil
}

// Complete marks an inbox message as PROCESSED.
func (r *PostgresInboxRepository) Complete(ctx context.Context, messageID, consumerID string) error {
	now := time.Now().UTC()
	query := `
		UPDATE inbox_messages
		SET status = 'PROCESSED',
		    processed_at = $1,
		    locked_at = NULL,
		    updated_at = $1
		WHERE consumer_id = $2
		  AND message_id = $3
	`
	_, err := r.pool.Exec(ctx, query, now, consumerID, messageID)
	return err
}

// Release releases a locked inbox message on failure for retry.
func (r *PostgresInboxRepository) Release(ctx context.Context, messageID, consumerID string, lastError error) error {
	now := time.Now().UTC()
	var errMsg *string
	if lastError != nil {
		s := lastError.Error()
		errMsg = &s
	}

	query := `
		UPDATE inbox_messages
		SET status = 'READY',
		    locked_at = NULL,
		    last_error = $1,
		    updated_at = $2
		WHERE consumer_id = $3
		  AND message_id = $4
	`
	_, err := r.pool.Exec(ctx, query, errMsg, now, consumerID, messageID)
	return err
}

func joinStrings(slice []string, sep string) string {
	if len(slice) == 0 {
		return ""
	}
	res := slice[0]
	for i := 1; i < len(slice); i++ {
		res += sep + slice[i]
	}
	return res
}
