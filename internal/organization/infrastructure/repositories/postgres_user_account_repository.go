// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	onboardingusecases "github.com/scandrix/backend/internal/organization/application/usecases/onboarding"
)

// PostgresUserAccountRepository persists user account data for organization onboarding.
type PostgresUserAccountRepository struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
	mem  map[uuid.UUID]*onboardingusecases.UserAccount
}

// NewPostgresUserAccountRepository instantiates a user account repository.
func NewPostgresUserAccountRepository(pool *pgxpool.Pool) *PostgresUserAccountRepository {
	return &PostgresUserAccountRepository{
		pool: pool,
		mem:  make(map[uuid.UUID]*onboardingusecases.UserAccount),
	}
}

// FindByID retrieves a user account by primary UUID.
func (r *PostgresUserAccountRepository) FindByID(ctx context.Context, id uuid.UUID) (*onboardingusecases.UserAccount, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		u, exists := r.mem[id]
		if !exists {
			return nil, errors.New("user not found")
		}
		cpy := *u
		return &cpy, nil
	}

	query := `
		SELECT uuid, COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid), email, role, status
		FROM users
		WHERE uuid = $1
		LIMIT 1;
	`
	var u onboardingusecases.UserAccount
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.WorkspaceID, &u.Email, &u.Role, &u.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed fetching user by id: %w", err)
	}
	u.DisplayName = u.Email
	return &u, nil
}

// FindByWorkspaceID retrieves all users associated with a specific workspace.
func (r *PostgresUserAccountRepository) FindByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*onboardingusecases.UserAccount, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*onboardingusecases.UserAccount
		for _, u := range r.mem {
			if u.WorkspaceID == wsID {
				cpy := *u
				res = append(res, &cpy)
			}
		}
		return res, nil
	}

	query := `
		SELECT uuid, organization_id, email, role, status
		FROM users
		WHERE organization_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching users by workspace id: %w", err)
	}
	defer rows.Close()

	var res []*onboardingusecases.UserAccount
	for rows.Next() {
		var u onboardingusecases.UserAccount
		if err := rows.Scan(&u.ID, &u.WorkspaceID, &u.Email, &u.Role, &u.Status); err != nil {
			return nil, fmt.Errorf("failed scanning user row: %w", err)
		}
		u.DisplayName = u.Email
		res = append(res, &u)
	}
	return res, nil
}

// UpdateWorkspaceAndRole migrates a user to a target workspace and updates their organization role.
func (r *PostgresUserAccountRepository) UpdateWorkspaceAndRole(ctx context.Context, userID, newWsID uuid.UUID, role, status string) (*onboardingusecases.UserAccount, error) {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		u, exists := r.mem[userID]
		if !exists {
			u = &onboardingusecases.UserAccount{ID: userID}
			r.mem[userID] = u
		}
		u.WorkspaceID = newWsID
		u.Role = role
		u.Status = status
		cpy := *u
		return &cpy, nil
	}

	query := `
		UPDATE users
		SET organization_id = $1, role = $2::public.users_role_enum, status = $3::public.users_status_enum, "updatedAt" = NOW()
		WHERE uuid = $4
		RETURNING uuid, organization_id, email, role, status;
	`
	var u onboardingusecases.UserAccount
	err := r.pool.QueryRow(ctx, query, newWsID, role, status, userID).Scan(
		&u.ID, &u.WorkspaceID, &u.Email, &u.Role, &u.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("failed updating user workspace and role: %w", err)
	}
	u.DisplayName = u.Email
	return &u, nil
}

// DeleteUser removes a user record from the database.
func (r *PostgresUserAccountRepository) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.mem, userID)
		return nil
	}

	query := `DELETE FROM users WHERE uuid = $1;`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

// CheckUserInOtherOrganization checks if an email belongs to a user registered in a different workspace.
func (r *PostgresUserAccountRepository) CheckUserInOtherOrganization(ctx context.Context, email string, wsID uuid.UUID) (bool, *uuid.UUID, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, u := range r.mem {
			if strings.ToLower(strings.TrimSpace(u.Email)) == normEmail {
				if u.WorkspaceID != uuid.Nil && u.WorkspaceID != wsID {
					targetWS := u.WorkspaceID
					return true, &targetWS, nil
				}
			}
		}
		return false, nil, nil
	}

	query := `
		SELECT organization_id
		FROM users
		WHERE LOWER(email) = $1 AND organization_id IS NOT NULL AND organization_id != '00000000-0000-0000-0000-000000000000'::uuid AND organization_id != $2
		LIMIT 1;
	`
	var existingWS uuid.UUID
	err := r.pool.QueryRow(ctx, query, normEmail, wsID).Scan(&existingWS)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed checking user cross-organization existence: %w", err)
	}
	return true, &existingWS, nil
}

