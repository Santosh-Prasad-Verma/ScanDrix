package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

// InsertOutboxEvent records an event inside the transactional boundary for guaranteed delivery (Outbox Pattern).
func (r *Repository) InsertOutboxEvent(ctx context.Context, event *models.OutboxRecord) error {
	if r == nil || r.client == nil {
		return nil
	}
	query := `
		INSERT INTO outbox_events (id, workspace_id, event_type, payload, status, retry_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`
	now := time.Now().UTC()
	event.CreatedAt = now
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	return r.client.ExecWithTenant(ctx, event.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			event.ID, event.WorkspaceID, event.EventType, event.Payload,
			models.OutboxPending, 0, event.CreatedAt,
		)
		return err
	})
}


// IngestWebhookEventTx atomically creates the incoming pull request review record and the outbox event within a single tenant transaction.
func (r *Repository) IngestWebhookEventTx(ctx context.Context, rev *models.PullRequestReview, event *models.OutboxRecord) error {
	if r == nil || r.client == nil {
		return errors.New("repository or database client is nil")
	}
	wsID := event.WorkspaceID
	if wsID == uuid.Nil && rev != nil {
		wsID = rev.WorkspaceID
	}

	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		if rev != nil {
			queryReview := `
				INSERT INTO pull_request_reviews (
					id, workspace_id, repository_id, pull_number, title, head_sha, base_sha, author_username, state, findings_count, created_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (id) DO NOTHING
			`
			if rev.ID == uuid.Nil {
				rev.ID = uuid.New()
			}
			if rev.CreatedAt.IsZero() {
				rev.CreatedAt = time.Now().UTC()
			}
			if _, err := tx.Exec(ctx, queryReview,
				rev.ID, rev.WorkspaceID, rev.RepositoryID, rev.PullNumber, rev.Title,
				rev.HeadSHA, rev.BaseSHA, rev.AuthorUsername, rev.State, rev.FindingsCount, rev.CreatedAt,
			); err != nil {
				return fmt.Errorf("failed inserting pull_request_review inside transaction: %w", err)
			}
		}

		if event != nil {
			queryOutbox := `
				INSERT INTO outbox_events (id, workspace_id, event_type, payload, status, retry_count, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (id) DO NOTHING
			`
			if event.ID == uuid.Nil {
				event.ID = uuid.New()
			}
			if event.CreatedAt.IsZero() {
				event.CreatedAt = time.Now().UTC()
			}
			if _, err := tx.Exec(ctx, queryOutbox,
				event.ID, event.WorkspaceID, event.EventType, event.Payload,
				models.OutboxPending, 0, event.CreatedAt,
			); err != nil {
				return fmt.Errorf("failed inserting outbox_event inside transaction: %w", err)
			}
		}
		return nil
	})
}


// FetchPendingOutboxEvents retrieves un-dispatched events for the background publisher relay.
func (r *Repository) FetchPendingOutboxEvents(ctx context.Context, limit int) ([]models.OutboxRecord, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		UPDATE outbox_events
		SET status = 'PROCESSING'
		WHERE id IN (
			SELECT id
			FROM outbox_events
			WHERE status = 'PENDING'
			   OR (status = 'PROCESSING' AND created_at < NOW() - INTERVAL '5 minutes')
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, workspace_id, event_type, payload, status, retry_count, created_at;
	`
	var records []models.OutboxRecord
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, limit)
		if err != nil {
			return fmt.Errorf("failed to fetch pending outbox events: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var rec models.OutboxRecord
			if err := rows.Scan(
				&rec.ID, &rec.WorkspaceID, &rec.EventType, &rec.Payload,
				&rec.Status, &rec.RetryCount, &rec.CreatedAt,
			); err != nil {
				return fmt.Errorf("error scanning outbox row: %w", err)
			}
			records = append(records, rec)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}


// MarkOutboxEventPublished marks an event as successfully published to the message broker.
func (r *Repository) MarkOutboxEventPublished(ctx context.Context, eventID uuid.UUID) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}
	query := `
		UPDATE outbox_events
		SET status = $1, published_at = $2
		WHERE id = $3
	`
	now := time.Now().UTC()
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, models.OutboxPublished, now, eventID)
		return err
	})
}


// MarkOutboxEventFailed marks an event as failed, incrementing retries and resetting to PENDING or FAILED.
func (r *Repository) MarkOutboxEventFailed(ctx context.Context, eventID uuid.UUID, errMsg string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}
	query := `
		UPDATE outbox_events
		SET status = CASE WHEN retry_count >= 5 THEN 'FAILED' ELSE 'PENDING' END,
		    retry_count = retry_count + 1,
		    last_error = $1
		WHERE id = $2
	`
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, errMsg, eventID)
		return err
	})
}


// RetryDeadLetterOutboxEvents resets failed or DLQ outbox events back to PENDING status.
func (r *Repository) RetryDeadLetterOutboxEvents(ctx context.Context, limit int) (int, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}
	query := `
		UPDATE outbox_events
		SET status = 'PENDING', retry_count = 0, last_error = NULL
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE status IN ('FAILED', 'DEAD_LETTER', 'RETRYING')
			LIMIT $1
		)
	`
	var count int
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, limit)
		if err != nil {
			return err
		}
		count = int(cmdTag.RowsAffected())
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}


// GetOutboxMetrics queries delivery stats from outbox_events.
func (r *Repository) GetOutboxMetrics(ctx context.Context, wsID uuid.UUID) (totalDelivered, pending, retrying, dlq int64, lastEventAt *time.Time, err error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, 0, nil, nil
	}

	query := `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE status = 'PUBLISHED'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'PENDING'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'PENDING' AND retry_count > 0), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'FAILED'), 0),
			MAX(created_at)
		FROM outbox_events
		WHERE workspace_id = $1;
	`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&totalDelivered, &pending, &retrying, &dlq, &lastEventAt)
	})
	return totalDelivered, pending, retrying, dlq, lastEventAt, err
}


// ClaimInboxMessage atomically claims a message in the database for processing.
func (r *Repository) ClaimInboxMessage(ctx context.Context, messageID, consumerID string) (bool, error) {
	if r == nil || r.client == nil {
		return true, nil
	}

	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	now := time.Now().UTC()

	var claimed bool
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		query := `
			INSERT INTO inbox_records (id, message_id, consumer_id, status, attempt_count, processed_at)
			VALUES ($1, $2, $3, 'PROCESSING', 0, $4)
			ON CONFLICT (id) DO UPDATE
			SET status = CASE
				WHEN inbox_records.status IN ('COMPLETED', 'PROCESSING') THEN inbox_records.status
				ELSE 'PROCESSING'
			END,
			processed_at = CASE
				WHEN inbox_records.status IN ('COMPLETED', 'PROCESSING') THEN inbox_records.processed_at
				ELSE $4
			END
			RETURNING (status = 'PROCESSING' AND xmax = 0) OR (status = 'PROCESSING' AND processed_at = $4);
		`
		var success bool
		err := tx.QueryRow(ctx, query, key, messageID, consumerID, now).Scan(&success)
		if err != nil {
			return err
		}
		claimed = success
		return nil
	})

	return claimed, err
}


// GetInboxAttemptCount retrieves the attempt count for an inbox record.
func (r *Repository) GetInboxAttemptCount(ctx context.Context, messageID, consumerID string) int {
	if r == nil || r.client == nil {
		return 0
	}
	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	var count int
	_ = r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT attempt_count FROM inbox_records WHERE id = $1", key).Scan(&count)
	})
	return count
}


// MarkInboxCompleted marks the inbox message as completed.
func (r *Repository) MarkInboxCompleted(ctx context.Context, messageID, consumerID string) error {
	if r == nil || r.client == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE inbox_records SET status = 'COMPLETED', processed_at = NOW() WHERE id = $1", key)
		return err
	})
}


// MarkInboxFailed marks the inbox message as failed.
func (r *Repository) MarkInboxFailed(ctx context.Context, messageID, consumerID string, failErr error) error {
	if r == nil || r.client == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	errMsg := ""
	if failErr != nil {
		errMsg = failErr.Error()
	}
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE inbox_records SET status = 'FAILED', last_error = $2, processed_at = NOW() WHERE id = $1", key, errMsg)
		return err
	})
}


// ReleaseInboxMessage resets the status so the message can be reclaimed.
func (r *Repository) ReleaseInboxMessage(ctx context.Context, messageID, consumerID string, attemptCount int) error {
	if r == nil || r.client == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%s", messageID, consumerID)
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE inbox_records SET status = 'RETRYING', attempt_count = $2, processed_at = NOW() WHERE id = $1", key, attemptCount)
		return err
	})
}

