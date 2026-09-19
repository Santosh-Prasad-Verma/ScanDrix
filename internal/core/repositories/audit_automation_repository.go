package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgAuditLogRepository implements domain.AuditLogRepository.
type PgAuditLogRepository struct {
	pool *pgxpool.Pool
}

// NewAuditLogRepository instantiates a new PgAuditLogRepository.
func NewAuditLogRepository(pool *pgxpool.Pool) *PgAuditLogRepository {
	return &PgAuditLogRepository{pool: pool}
}

// Record persists an enterprise compliance audit log record.
func (r *PgAuditLogRepository) Record(ctx context.Context, log *domain.AuditLogRecord) error {
	query := `
		INSERT INTO audit_logs (
			id, created_at, updated_at, workspace_id, actor_user_id, action,
			target_resource, target_id, ip_address, user_agent, previous_state, new_state, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	now := time.Now().UTC()
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	log.CreatedAt = now
	log.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		log.ID, log.CreatedAt, log.UpdatedAt, log.WorkspaceID, log.ActorUserID, log.Action,
		log.TargetResource, log.TargetID, log.IPAddress, log.UserAgent, log.PreviousState,
		log.NewState, log.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed recording audit log: %w", err)
	}
	return nil
}

// Query retrieves audit records with pagination.
func (r *PgAuditLogRepository) Query(
	ctx context.Context,
	wsID uuid.UUID,
	targetResource string,
	pq domain.PaginationQuery,
) (*domain.PaginatedResult[*domain.AuditLogRecord], error) {
	pq.EnsureDefaults()

	var total int64
	countSQL := `SELECT count(*) FROM audit_logs WHERE workspace_id = $1`
	args := []any{wsID}
	if targetResource != "" {
		countSQL += ` AND target_resource = $2`
		args = append(args, targetResource)
	}
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed counting audit logs: %w", err)
	}

	offset := (pq.Page - 1) * pq.PageSize
	querySQL := fmt.Sprintf(`
		SELECT id, created_at, updated_at, workspace_id, actor_user_id, action,
		       target_resource, target_id, ip_address, user_agent, previous_state, new_state, metadata
		FROM audit_logs
		WHERE workspace_id = $1
	`)
	if targetResource != "" {
		querySQL += ` AND target_resource = $2`
		querySQL += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $3 OFFSET $4`)
		args = []any{wsID, targetResource, pq.PageSize, offset}
	} else {
		querySQL += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $2 OFFSET $3`)
		args = []any{wsID, pq.PageSize, offset}
	}

	rows, err := r.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed querying audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.AuditLogRecord, 0, pq.PageSize)
	for rows.Next() {
		l := &domain.AuditLogRecord{}
		if err := rows.Scan(
			&l.ID, &l.CreatedAt, &l.UpdatedAt, &l.WorkspaceID, &l.ActorUserID, &l.Action,
			&l.TargetResource, &l.TargetID, &l.IPAddress, &l.UserAgent, &l.PreviousState,
			&l.NewState, &l.Metadata,
		); err != nil {
			return nil, fmt.Errorf("failed scanning audit log row: %w", err)
		}
		items = append(items, l)
	}

	totalPages := int(total) / pq.PageSize
	if int(total)%pq.PageSize != 0 {
		totalPages++
	}

	return &domain.PaginatedResult[*domain.AuditLogRecord]{
		Items:      items,
		TotalCount: total,
		Page:       pq.Page,
		PageSize:   pq.PageSize,
		TotalPages: totalPages,
		HasNext:    pq.Page < totalPages,
	}, nil
}

// PgAutomationRepository manages custom automations and executions.
type PgAutomationRepository struct {
	pool *pgxpool.Pool
}

// NewAutomationRepository instantiates a new PgAutomationRepository.
func NewAutomationRepository(pool *pgxpool.Pool) *PgAutomationRepository {
	return &PgAutomationRepository{pool: pool}
}

// Create inserts a custom automation workflow rule.
func (r *PgAutomationRepository) Create(ctx context.Context, auto *domain.Automation) error {
	query := `
		INSERT INTO automations (
			id, created_at, updated_at, workspace_id, organization_id, name,
			trigger_event, action_type, filter_conditions, action_configuration, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	if auto.ID == uuid.Nil {
		auto.ID = uuid.New()
	}
	auto.CreatedAt = now
	auto.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		auto.ID, auto.CreatedAt, auto.UpdatedAt, auto.WorkspaceID, auto.OrganizationID, auto.Name,
		auto.TriggerEvent, auto.ActionType, auto.FilterConditions, auto.ActionConfiguration, auto.IsActive,
	)
	if err != nil {
		return fmt.Errorf("failed inserting automation: %w", err)
	}
	return nil
}

// RecordExecution stores execution traces and duration of an automation trigger.
func (r *PgAutomationRepository) RecordExecution(ctx context.Context, exec *domain.AutomationExecution) error {
	query := `
		INSERT INTO automation_executions (
			id, created_at, updated_at, workspace_id, automation_id, trigger_payload,
			status, logs, duration_ms, executed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	now := time.Now().UTC()
	if exec.ID == uuid.Nil {
		exec.ID = uuid.New()
	}
	exec.CreatedAt = now
	exec.UpdatedAt = now
	if exec.ExecutedAt.IsZero() {
		exec.ExecutedAt = now
	}

	_, err := r.pool.Exec(ctx, query,
		exec.ID, exec.CreatedAt, exec.UpdatedAt, exec.WorkspaceID, exec.AutomationID,
		exec.TriggerPayload, exec.Status, exec.Logs, exec.DurationMs, exec.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("failed recording automation execution: %w", err)
	}
	return nil
}

// PgNotificationRepository manages in-app and outgoing multi-channel notifications.
type PgNotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository instantiates a new PgNotificationRepository.
func NewNotificationRepository(pool *pgxpool.Pool) *PgNotificationRepository {
	return &PgNotificationRepository{pool: pool}
}

// CreateUserNotification adds an in-app notification to a user's dashboard feed.
func (r *PgNotificationRepository) CreateUserNotification(ctx context.Context, notif *domain.UserNotification) error {
	query := `
		INSERT INTO user_notifications (
			id, created_at, updated_at, workspace_id, user_id, title, message, category, link_url, is_read, read_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	if notif.ID == uuid.Nil {
		notif.ID = uuid.New()
	}
	notif.CreatedAt = now
	notif.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		notif.ID, notif.CreatedAt, notif.UpdatedAt, notif.WorkspaceID, notif.UserID,
		notif.Title, notif.Message, notif.Category, notif.LinkURL, notif.IsRead, notif.ReadAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating user notification: %w", err)
	}
	return nil
}

// ListUnreadUserNotifications retrieves unread alerts for a developer.
func (r *PgNotificationRepository) ListUnreadUserNotifications(ctx context.Context, wsID, userID uuid.UUID, limit int) ([]*domain.UserNotification, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, created_at, updated_at, workspace_id, user_id, title, message,
		       category, link_url, is_read, read_at
		FROM user_notifications
		WHERE workspace_id = $1 AND user_id = $2 AND is_read = false
		ORDER BY created_at DESC
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, query, wsID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying user notifications: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.UserNotification, 0, limit)
	for rows.Next() {
		n := &domain.UserNotification{}
		if err := rows.Scan(
			&n.ID, &n.CreatedAt, &n.UpdatedAt, &n.WorkspaceID, &n.UserID, &n.Title,
			&n.Message, &n.Category, &n.LinkURL, &n.IsRead, &n.ReadAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning user notification: %w", err)
		}
		items = append(items, n)
	}
	return items, nil
}

// MarkRead flags a user notification as read.
func (r *PgNotificationRepository) MarkRead(ctx context.Context, wsID, notifID uuid.UUID) error {
	query := `
		UPDATE user_notifications
		SET is_read = true, read_at = $3, updated_at = $3
		WHERE workspace_id = $1 AND id = $2
	`
	now := time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query, wsID, notifID, now)
	if err != nil {
		return fmt.Errorf("failed marking notification read: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
