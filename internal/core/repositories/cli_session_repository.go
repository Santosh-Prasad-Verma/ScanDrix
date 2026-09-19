package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// CliSessionRepository defines operations for interactive CLI browser login handshakes.
type CliSessionRepository interface {
	Create(ctx context.Context, session *domain.CliAuthSession) error
	FindBySessionCode(ctx context.Context, sessionCode string) (*domain.CliAuthSession, error)
	FindByUserCode(ctx context.Context, userCode string) (*domain.CliAuthSession, error)
	Authorize(ctx context.Context, userCode string, userID, wsID uuid.UUID, tokenPayload string) error
	Complete(ctx context.Context, sessionCode string) error
	PurgeExpired(ctx context.Context) (int64, error)
}

// PostgresCliSessionRepository implements CliSessionRepository.
type PostgresCliSessionRepository struct {
	db *sql.DB
}

// NewPostgresCliSessionRepository instantiates a CLI session repository.
func NewPostgresCliSessionRepository(db *sql.DB) *PostgresCliSessionRepository {
	return &PostgresCliSessionRepository{db: db}
}

// Create inserts a new auth handshake request.
func (r *PostgresCliSessionRepository) Create(ctx context.Context, session *domain.CliAuthSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	now := time.Now().UTC()
	session.CreatedAt = now
	session.UpdatedAt = now
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = now.Add(15 * time.Minute)
	}

	query := `
		INSERT INTO cli_auth_sessions (id, session_code, user_code, status, user_id, workspace_id, token_payload, client_ip, user_agent, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.SessionCode, session.UserCode, session.Status,
		session.UserID, session.WorkspaceID, session.TokenPayload,
		session.ClientIP, session.UserAgent, session.ExpiresAt,
		session.CreatedAt, session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating cli auth session: %w", err)
	}
	return nil
}

// FindBySessionCode retrieves session polling state for CLI client.
func (r *PostgresCliSessionRepository) FindBySessionCode(ctx context.Context, sessionCode string) (*domain.CliAuthSession, error) {
	query := `
		SELECT id, session_code, user_code, status, user_id, workspace_id, token_payload, client_ip, user_agent, expires_at, created_at, updated_at
		FROM cli_auth_sessions
		WHERE session_code = $1
	`
	row := r.db.QueryRowContext(ctx, query, sessionCode)
	var s domain.CliAuthSession
	err := row.Scan(&s.ID, &s.SessionCode, &s.UserCode, &s.Status, &s.UserID, &s.WorkspaceID, &s.TokenPayload, &s.ClientIP, &s.UserAgent, &s.ExpiresAt, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching cli session code %s: %w", sessionCode, err)
	}
	return &s, nil
}

// FindByUserCode retrieves session for browser approval page.
func (r *PostgresCliSessionRepository) FindByUserCode(ctx context.Context, userCode string) (*domain.CliAuthSession, error) {
	query := `
		SELECT id, session_code, user_code, status, user_id, workspace_id, token_payload, client_ip, user_agent, expires_at, created_at, updated_at
		FROM cli_auth_sessions
		WHERE user_code = $1
	`
	row := r.db.QueryRowContext(ctx, query, userCode)
	var s domain.CliAuthSession
	err := row.Scan(&s.ID, &s.SessionCode, &s.UserCode, &s.Status, &s.UserID, &s.WorkspaceID, &s.TokenPayload, &s.ClientIP, &s.UserAgent, &s.ExpiresAt, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching cli user code %s: %w", userCode, err)
	}
	return &s, nil
}

// Authorize binds an authenticated user identity and credentials to the pending session.
func (r *PostgresCliSessionRepository) Authorize(ctx context.Context, userCode string, userID, wsID uuid.UUID, tokenPayload string) error {
	now := time.Now().UTC()
	query := `
		UPDATE cli_auth_sessions
		SET status = 'AUTHORIZED', user_id = $1, workspace_id = $2, token_payload = $3, updated_at = $4
		WHERE user_code = $5 AND status = 'PENDING' AND expires_at > $4
	`
	res, err := r.db.ExecContext(ctx, query, userID, wsID, tokenPayload, now, userCode)
	if err != nil {
		return fmt.Errorf("failed authorizing cli session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pending valid session not found for code %s", userCode)
	}
	return nil
}

// Complete marks the session as claimed by the CLI client.
func (r *PostgresCliSessionRepository) Complete(ctx context.Context, sessionCode string) error {
	now := time.Now().UTC()
	query := `
		UPDATE cli_auth_sessions
		SET status = 'COMPLETED', updated_at = $1
		WHERE session_code = $2 AND status = 'AUTHORIZED'
	`
	_, err := r.db.ExecContext(ctx, query, now, sessionCode)
	return err
}

// PurgeExpired cleans up old unauthenticated sessions.
func (r *PostgresCliSessionRepository) PurgeExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	query := `DELETE FROM cli_auth_sessions WHERE expires_at < $1`
	res, err := r.db.ExecContext(ctx, query, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
