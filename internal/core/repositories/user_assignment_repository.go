package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// UserRepositoryAssignment links a user to a specific repository with custom review roles.
type UserRepositoryAssignment struct {
	domain.TenantScopedEntity
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	RepositoryID uuid.UUID `json:"repository_id" db:"repository_id"`
	Role         string    `json:"role" db:"role"` // "ADMIN", "REVIEWER", "OBSERVER"
	IsActive     bool      `json:"is_active" db:"is_active"`
}

// UserAssignmentRepository defines operations for repo-specific user permission assignments.
type UserAssignmentRepository interface {
	Assign(ctx context.Context, assignment *UserRepositoryAssignment) error
	Revoke(ctx context.Context, wsID, userID, repoID uuid.UUID) error
	ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*UserRepositoryAssignment, error)
	ListByRepository(ctx context.Context, wsID, repoID uuid.UUID) ([]*UserRepositoryAssignment, error)
}

// PostgresUserAssignmentRepository implements UserAssignmentRepository.
type PostgresUserAssignmentRepository struct {
	db *sql.DB
}

// NewPostgresUserAssignmentRepository instantiates a user assignment repository.
func NewPostgresUserAssignmentRepository(db *sql.DB) *PostgresUserAssignmentRepository {
	return &PostgresUserAssignmentRepository{db: db}
}

// Assign creates or reactivates a user repository permission assignment.
func (r *PostgresUserAssignmentRepository) Assign(ctx context.Context, a *UserRepositoryAssignment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	query := `
		INSERT INTO user_repository_assignments (
			id, workspace_id, user_id, repository_id, role, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (workspace_id, user_id, repository_id) DO UPDATE SET
			role = EXCLUDED.role,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		a.ID, a.WorkspaceID, a.UserID, a.RepositoryID, a.Role, a.IsActive, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed assigning user to repo: %w", err)
	}
	return nil
}

// Revoke disables the user repository assignment.
func (r *PostgresUserAssignmentRepository) Revoke(ctx context.Context, wsID, userID, repoID uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE user_repository_assignments
		SET is_active = false, updated_at = $1
		WHERE workspace_id = $2 AND user_id = $3 AND repository_id = $4
	`
	_, err := r.db.ExecContext(ctx, query, now, wsID, userID, repoID)
	return err
}

// ListByUser retrieves all repository assignments for an engineer.
func (r *PostgresUserAssignmentRepository) ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*UserRepositoryAssignment, error) {
	query := `
		SELECT id, workspace_id, user_id, repository_id, role, is_active, created_at, updated_at
		FROM user_repository_assignments
		WHERE workspace_id = $1 AND user_id = $2 AND is_active = true
	`
	rows, err := r.db.QueryContext(ctx, query, wsID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*UserRepositoryAssignment
	for rows.Next() {
		var a UserRepositoryAssignment
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.UserID, &a.RepositoryID, &a.Role, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}

// ListByRepository retrieves all users assigned to a repository.
func (r *PostgresUserAssignmentRepository) ListByRepository(ctx context.Context, wsID, repoID uuid.UUID) ([]*UserRepositoryAssignment, error) {
	query := `
		SELECT id, workspace_id, user_id, repository_id, role, is_active, created_at, updated_at
		FROM user_repository_assignments
		WHERE workspace_id = $1 AND repository_id = $2 AND is_active = true
	`
	rows, err := r.db.QueryContext(ctx, query, wsID, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*UserRepositoryAssignment
	for rows.Next() {
		var a UserRepositoryAssignment
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.UserID, &a.RepositoryID, &a.Role, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}
