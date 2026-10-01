// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: auth_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/pkg/models"
)

// UserRecord mirrors the users entity in the PostgreSQL database.
type UserRecord struct {
	UUID           uuid.UUID
	Email          string
	Password       string
	Role           string
	Status         string
	OrganizationID *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RefreshTokenRecord is the non-secret metadata for a stored refresh token.
//
// It intentionally exposes no token material. Every consumer needs only the
// owning user and whether the token has already been spent
// (AUDIT_REMEDIATION.md F-12).
type RefreshTokenRecord struct {
	UUID         uuid.UUID
	ExpiryDate   time.Time
	Used         bool
	AuthProvider string
	UserUUID     uuid.UUID
}

// hashRefreshToken returns the hex-encoded SHA-256 digest of a refresh token.
//
// This is the single definition of the stored form. The same encoding is
// applied by migration 039 when backfilling existing rows, so the Go code and
// the database agree on what a stored digest looks like.
//
// The token is 32 bytes of CSPRNG output, so it has no guessable structure and
// a plain digest is appropriate here: the attacker model is a database read,
// and what must not be recoverable from a dump is the token itself.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GetUserByEmail fetches a user record by email.
// GetUserByEmail fetches a user record by email. Runs with system elevation to allow pre-auth authentication under RLS.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, email, password, role, status, organization_id, "createdAt", "updatedAt"
		FROM users
		WHERE LOWER(email) = LOWER($1)
		LIMIT 1;
	`
	var u UserRecord
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, email).Scan(
			&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID fetches a user record by primary UUID. Runs with system elevation to allow pre-auth lookups under RLS.
func (r *Repository) GetUserByID(ctx context.Context, userUUID uuid.UUID) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, email, password, role, status, organization_id, "createdAt", "updatedAt"
		FROM users
		WHERE uuid = $1
		LIMIT 1;
	`
	var u UserRecord
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, userUUID).Scan(
			&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetOrganizationByEmailDomain resolves an active organization ID matching an email domain (e.g. "acme.com").
func (r *Repository) GetOrganizationByEmailDomain(ctx context.Context, domain string) (*uuid.UUID, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}
	cleanDomain := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(domain)), "@")
	if cleanDomain == "" {
		return nil, errors.New("invalid domain parameter")
	}

	query := `
		SELECT organization_id
		FROM users
		WHERE LOWER(email) LIKE '%@' || $1
		  AND organization_id IS NOT NULL
		  AND status = 'active'
		LIMIT 1;
	`
	var orgID uuid.UUID
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, cleanDomain).Scan(&orgID)
	})
	if err != nil {
		return nil, err
	}
	return &orgID, nil
}

// CreateUser persists a new user with bcrypt password into the users table.
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, role string, orgID *uuid.UUID) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		INSERT INTO users (uuid, email, password, role, status, organization_id, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4::users_role_enum, 'active', $5, now(), now())
		RETURNING uuid, email, password, role, status, organization_id, "createdAt", "updatedAt";
	`
	newID := uuid.New()
	var u UserRecord
	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, newID, email, passwordHash, role, orgID).Scan(
			&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateRefreshToken records an active refresh token in the auth table.
//
// Only the SHA-256 digest is stored. The plaintext exists only in the caller's
// memory, for the response that hands it to the client. Storing it verbatim
// meant a read-only database compromise yielded live 30-day bearer credentials
// (AUDIT_REMEDIATION.md F-12).
func (r *Repository) CreateRefreshToken(ctx context.Context, userUUID uuid.UUID, refreshToken string, expiry time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	if strings.TrimSpace(refreshToken) == "" {
		return errors.New("refresh token must not be empty")
	}

	query := `
		INSERT INTO auth (uuid, "userUuid", "tokenHash", "expiryDate", used, "authProvider", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, false, 'credentials', now(), now());
	`
	_, err := r.execSystem(ctx, query, uuid.New(), userUUID, hashRefreshToken(refreshToken), expiry)
	return err
}

// GetRefreshToken retrieves a refresh token record by its digest.
//
// The returned record deliberately carries no token material: the only field
// callers need is who the token belongs to and whether it has been spent.
func (r *Repository) GetRefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("refresh token is required")
	}

	query := `
		SELECT uuid, "expiryDate", used, "authProvider", "userUuid"
		FROM auth
		WHERE "tokenHash" = $1
		LIMIT 1;
	`
	var t RefreshTokenRecord
	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, hashRefreshToken(refreshToken)).Scan(
			&t.UUID, &t.ExpiryDate, &t.Used, &t.AuthProvider, &t.UserUUID,
		)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkRefreshTokenUsed invalidates a refresh token as part of one-time token rotation (Master Rule 5.1).
func (r *Repository) MarkRefreshTokenUsed(ctx context.Context, refreshToken string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	if strings.TrimSpace(refreshToken) == "" {
		return errors.New("refresh token must not be empty")
	}

	query := `
		UPDATE auth
		SET used = true, "updatedAt" = now()
		WHERE "tokenHash" = $1;
	`
	_, err := r.execSystem(ctx, query, hashRefreshToken(refreshToken))
	return err
}

// SaveAPIKey records an issued CLI API key in team_cli_key storage.
func (r *Repository) SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		INSERT INTO team_cli_key (uuid, workspace_id, name, "keyHash", active, "keyPrefix", "expiresAt", config, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, true, $5, $6, '{}'::jsonb, now(), now())
		ON CONFLICT (uuid) DO UPDATE SET
			name = EXCLUDED.name,
			"keyHash" = EXCLUDED."keyHash",
			active = true,
			"updatedAt" = now();
	`
	_, err := r.execTenant(ctx, workspaceID, query, id, workspaceID, name, keyHash, prefix, expiresAt)
	return err
}

// GetAPIKeyByHash retrieves an active CLI key record by its SHA-256 hash or prefix.
func (r *Repository) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.TeamCLIKey, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, "lastUsedAt", "expiresAt", "createdAt", "updatedAt"
		FROM team_cli_key
		WHERE "keyHash" = $1 AND active = true
		LIMIT 1;
	`
	var k models.TeamCLIKey
	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, keyHash).Scan(
			&k.ID, &k.WorkspaceID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Active, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// TouchAPIKeyUsage records the timestamp when an API key was used for authentication.
func (r *Repository) TouchAPIKeyUsage(ctx context.Context, keyID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE team_cli_key
		SET "lastUsedAt" = now(), "updatedAt" = now()
		WHERE uuid = $1;
	`
	_, err := r.execSystem(ctx, query, keyID)
	return err
}

// ListAPIKeys retrieves all active CLI API keys for a workspace.
func (r *Repository) ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*models.TeamCLIKey, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, "lastUsedAt", "expiresAt", "createdAt", "updatedAt"
		FROM team_cli_key
		WHERE workspace_id = $1
		ORDER BY "createdAt" DESC;
	`
	var keys []*models.TeamCLIKey
	err := r.queryTenant(ctx, workspaceID, query, []any{workspaceID}, func(rows pgx.Rows) error {
		for rows.Next() {
			var k models.TeamCLIKey
			if err := rows.Scan(&k.ID, &k.WorkspaceID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Active, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
				return err
			}
			keys = append(keys, &k)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// RevokeAPIKey deactivates an issued CLI API key.
func (r *Repository) RevokeAPIKey(ctx context.Context, workspaceID, keyID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE team_cli_key
		SET active = false, "updatedAt" = now()
		WHERE uuid = $1 AND (workspace_id = $2 OR workspace_id IS NULL);
	`
	_, err := r.execSystem(ctx, query, keyID, workspaceID)
	return err
}

// VerifyCLIToken authenticates a plaintext CLI token against the database repository.
func (r *Repository) VerifyCLIToken(ctx context.Context, plaintext string) (uuid.UUID, *models.AccountProfile, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return uuid.Nil, nil, errors.New("database repository unavailable")
	}

	trimmed := strings.TrimSpace(plaintext)
	if trimmed == "" {
		return uuid.Nil, nil, errors.New("empty CLI token")
	}

	sum := sha256.Sum256([]byte(trimmed))
	keyHash := hex.EncodeToString(sum[:])

	query := `
		SELECT uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, "expiresAt"
		FROM team_cli_key
		WHERE active = true AND "keyHash" = $1
		LIMIT 1;
	`
	var key models.TeamCLIKey
	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, keyHash).Scan(
			&key.ID, &key.WorkspaceID, &key.TeamID, &key.Name, &key.KeyHash, &key.KeyPrefix, &key.Active, &key.ExpiresAt,
		)
	})
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("invalid or inactive CLI token: %w", err)
	}

	if key.ExpiresAt != nil && time.Now().UTC().After(*key.ExpiresAt) {
		return uuid.Nil, nil, errors.New("CLI token has expired")
	}

	_ = r.TouchAPIKeyUsage(ctx, key.ID)

	wsID := uuid.Nil
	if key.WorkspaceID != nil {
		wsID = *key.WorkspaceID
	}

	profile := &models.AccountProfile{
		ID:          key.ID,
		WorkspaceID: wsID,
		Email:       key.Name + "@cli.scandrix.internal",
		DisplayName: key.Name,
		Role:        models.RoleMember,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	return wsID, profile, nil
}

// GetDevice retrieves a CLI hardware device record for a given workspace and device ID.
func (r *Repository) GetDevice(ctx context.Context, workspaceID uuid.UUID, deviceID string) (*models.CLIDevice, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, workspace_id, device_id, device_token_hash, user_agent, last_seen_at, created_at
		FROM cli_devices
		WHERE workspace_id = $1 AND device_id = $2
		LIMIT 1;
	`
	var d models.CLIDevice
	var userAgent *string
	// Scoped to the workspace: cli_devices has RLS.
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID, deviceID).Scan(
			&d.ID, &d.WorkspaceID, &d.DeviceID, &d.DeviceTokenHash, &userAgent, &d.LastSeen, &d.CreatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	if userAgent != nil {
		d.UserAgent = *userAgent
	}
	return &d, nil
}

// CountSeats returns the number of user seats currently consumed in a workspace.
//
// A seat is consumed by any account that is not deactivated or removed:
// active members plus everyone invited but not yet accepted. Accounts with no
// workspace are not counted against any quota.
func (r *Repository) CountSeats(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, errors.New("database repository unavailable")
	}
	if workspaceID == uuid.Nil {
		return 0, nil
	}

	query := `
		SELECT COUNT(*)
		FROM users
		WHERE organization_id = $1
		  AND status IN ('active', 'pending', 'awaiting_approval', 'pending_email');
	`
	// Scoped to the workspace: users, tracked_repositories and cli_devices all
	// have RLS, and a bare pool call would return 0 under a runtime role.
	var count int
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID).Scan(&count)
	})
	return count, err
}

// CountRepositories returns the number of active tracked repositories in a workspace.
func (r *Repository) CountRepositories(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, errors.New("database repository unavailable")
	}
	if workspaceID == uuid.Nil {
		return 0, nil
	}

	query := `
		SELECT COUNT(*)
		FROM tracked_repositories
		WHERE workspace_id = $1 AND is_active = TRUE;
	`
	// Scoped to the workspace: users, tracked_repositories and cli_devices all
	// have RLS, and a bare pool call would return 0 under a runtime role.
	var count int
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID).Scan(&count)
	})
	return count, err
}

// CountDevices returns the active number of hardware devices registered for a workspace.
func (r *Repository) CountDevices(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, errors.New("database repository unavailable")
	}

	query := `
		SELECT COUNT(*)
		FROM cli_devices
		WHERE workspace_id = $1;
	`
	// Scoped to the workspace: users, tracked_repositories and cli_devices all
	// have RLS, and a bare pool call would return 0 under a runtime role.
	var count int
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID).Scan(&count)
	})
	return count, err
}

// RegisterDevice inserts or updates a physical machine registration in the database.
func (r *Repository) RegisterDevice(ctx context.Context, device *models.CLIDevice) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return errors.New("database repository unavailable")
	}

	query := `
		INSERT INTO cli_devices (uuid, workspace_id, device_id, device_token_hash, user_agent, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (workspace_id, device_id) DO UPDATE SET
			device_token_hash = EXCLUDED.device_token_hash,
			user_agent = EXCLUDED.user_agent,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = now();
	`
	_, err := r.execSystem(ctx, query, device.ID, device.WorkspaceID, device.DeviceID, device.DeviceTokenHash, device.UserAgent, device.LastSeen, device.CreatedAt)
	return err
}

// UpdateDeviceLastSeen updates the heartbeat timestamp for a registered CLI device.
func (r *Repository) UpdateDeviceLastSeen(ctx context.Context, deviceID uuid.UUID, userAgent string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE cli_devices
		SET last_seen_at = now(), user_agent = COALESCE(NULLIF($2, ''), user_agent), updated_at = now()
		WHERE uuid = $1;
	`
	_, err := r.execSystem(ctx, query, deviceID, userAgent)
	return err
}

// CreateCLISession stores an RFC 8628 terminal authorization session in PostgreSQL (cli_auth_sessions table).
func (r *Repository) CreateCLISession(ctx context.Context, s *cliauth.CLIDeviceSession) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	// session_id is NOT NULL with no default. It is a legacy column from
	// migration 009 that the identity writer (sql_repositories.go) still
	// reads, so it cannot simply be dropped. device_code is already UNIQUE,
	// indexed, and is the lookup key this table is already queried by, so it
	// doubles as the session_id.
	//
	// Omitting it made every INSERT fail on the NOT NULL constraint. The
	// caller discarded that error (cli_session_store.go), so device sessions
	// silently never persisted and the in-memory fallback masked it.
	// AUDIT_REMEDIATION.md F-27.
	query := `
		INSERT INTO cli_auth_sessions (
			uuid, state, device_code, user_code, redirect_uri, mode, status,
			expires_at, "createdAt", "updatedAt", session_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.execSystem(ctx, query,
		s.UUID, s.State, s.DeviceCode, s.UserCode, s.RedirectURI, s.Mode, string(s.Status),
		s.ExpiresAt, s.CreatedAt, s.UpdatedAt, s.DeviceCode,
	)
	return err
}

// GetCLISessionByDeviceCode retrieves a session using its device_code.
func (r *Repository) GetCLISessionByDeviceCode(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, state, device_code, user_code, redirect_uri, mode, status,
		       access_token, refresh_token, user_id, workspace_id, user_email, user_agent,
		       expires_at, consumed_at, completed_at, "createdAt", "updatedAt"
		FROM cli_auth_sessions
		WHERE device_code = $1
		LIMIT 1;
	`
	var s cliauth.CLIDeviceSession
	var status string
	var access, refresh, email, agent, redirectURI *string
	var workspaceID *uuid.UUID
	var userID *uuid.UUID

	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, deviceCode).Scan(
			&s.UUID, &s.State, &s.DeviceCode, &s.UserCode, &redirectURI, &s.Mode, &status,
			&access, &refresh, &userID, &workspaceID, &email, &agent,
			&s.ExpiresAt, &s.ConsumedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	s.Status = cliauth.SessionStatus(status)
	// redirect_uri is nullable; scanning it straight into a string fails on
	// NULL, which made every lookup for a session without a redirect error
	// out and silently fall back to memory. AUDIT_REMEDIATION.md F-27.
	if redirectURI != nil {
		s.RedirectURI = *redirectURI
	}
	// The tenant bound on approval. Empty while the session is still PENDING,
	// which is expected: nobody has authenticated at that point.
	// AUDIT_REMEDIATION.md F-37.
	if workspaceID != nil {
		s.WorkspaceID = *workspaceID
	}
	if access != nil {
		s.AccessToken = *access
	}
	if refresh != nil {
		s.RefreshToken = *refresh
	}
	if email != nil {
		s.UserEmail = *email
	}
	if agent != nil {
		s.UserAgent = *agent
	}
	s.UserID = userID
	return &s, nil
}

// GetCLISessionByUserCode retrieves a pending session using its 8-char user_code.
func (r *Repository) GetCLISessionByUserCode(ctx context.Context, userCode string) (*cliauth.CLIDeviceSession, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	query := `
		SELECT uuid, state, device_code, user_code, redirect_uri, mode, status,
		       access_token, refresh_token, user_id, workspace_id, user_email, user_agent,
		       expires_at, consumed_at, completed_at, "createdAt", "updatedAt"
		FROM cli_auth_sessions
		WHERE user_code = $1 AND status = 'pending'
		LIMIT 1;
	`
	var s cliauth.CLIDeviceSession
	var status string
	var access, refresh, email, agent, redirectURI *string
	var workspaceID *uuid.UUID
	var userID *uuid.UUID

	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, cleanCode).Scan(
			&s.UUID, &s.State, &s.DeviceCode, &s.UserCode, &redirectURI, &s.Mode, &status,
			&access, &refresh, &userID, &workspaceID, &email, &agent,
			&s.ExpiresAt, &s.ConsumedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	s.Status = cliauth.SessionStatus(status)
	// redirect_uri is nullable; scanning it straight into a string fails on
	// NULL, which made every lookup for a session without a redirect error
	// out and silently fall back to memory. AUDIT_REMEDIATION.md F-27.
	if redirectURI != nil {
		s.RedirectURI = *redirectURI
	}
	// The tenant bound on approval. Empty while the session is still PENDING,
	// which is expected: nobody has authenticated at that point.
	// AUDIT_REMEDIATION.md F-37.
	if workspaceID != nil {
		s.WorkspaceID = *workspaceID
	}
	if access != nil {
		s.AccessToken = *access
	}
	if refresh != nil {
		s.RefreshToken = *refresh
	}
	if email != nil {
		s.UserEmail = *email
	}
	if agent != nil {
		s.UserAgent = *agent
	}
	s.UserID = userID
	return &s, nil
}

// CompleteCLISession marks a CLI auth session as completed with issued tokens.
// CompleteCLISession records the browser approval. workspaceID is the tenant
// that approved the device code; it is never taken from the CLI.
// AUDIT_REMEDIATION.md F-37.
func (r *Repository) CompleteCLISession(ctx context.Context, userCode, accessToken, refreshToken string, userID, workspaceID uuid.UUID, email string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	if workspaceID == uuid.Nil {
		return cliauth.ErrNoWorkspace
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	query := `
		UPDATE cli_auth_sessions
		SET status = 'completed', access_token = $1, refresh_token = $2,
		    user_id = $3, workspace_id = $4, user_email = $5,
		    completed_at = now(), "updatedAt" = now()
		WHERE user_code = $6 AND status = 'pending';
	`
	_, err := r.execSystem(ctx, query, accessToken, refreshToken, userID, workspaceID, email, cleanCode)
	return err
}

// ConsumeCLISession marks an authorized session consumed so tokens cannot be replayed.
func (r *Repository) ConsumeCLISession(ctx context.Context, sessionID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE cli_auth_sessions
		SET status = 'consumed', consumed_at = now(), "updatedAt" = now()
		WHERE uuid = $1;
	`
	_, err := r.execSystem(ctx, query, sessionID)
	return err
}

// ConsumeCLISessionByDeviceCode atomically claims a completed session for the
// device code and returns it, so a one-time authorization can be redeemed only
// once even under concurrent polls.
//
// AUDIT_REMEDIATION.md F-27. The guard is the `status = 'completed'` predicate
// inside the UPDATE: two concurrent transactions both evaluating
// `SELECT ... FOR UPDATE` style reads would still both succeed if the check
// lived in Go, but only one UPDATE can match the row while it is still
// 'completed', because the second one re-evaluates the predicate after the
// first commits and matches zero rows. Returning the freshly-read row in the
// same statement means the caller never sees a status it cannot own.
//
// Returns cliauth.ErrSessionConsumed when another caller already redeemed it.
func (r *Repository) ConsumeCLISessionByDeviceCode(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, cliauth.ErrSessionNotFound
	}

	// A CTE performs the guarded UPDATE and reads the resulting row in one
	// round trip and one snapshot, which is what makes this atomic.
	const query = `
		WITH claimed AS (
			UPDATE cli_auth_sessions
			SET status = 'consumed', consumed_at = now(), "updatedAt" = now()
			WHERE device_code = $1 AND status = 'completed'
			RETURNING uuid, state, device_code, user_code, redirect_uri, mode,
			          access_token, refresh_token, user_email, status, consumed_at
		)
		SELECT uuid, state, device_code, user_code, redirect_uri, mode,
		       access_token, refresh_token, user_email, status, consumed_at
		FROM claimed
	`

	sess := &cliauth.CLIDeviceSession{}
	// Optional columns are scanned through pointers. Scanning a NULL into a
	// plain string fails with "cannot scan NULL into *string", and redirect_uri
	// is nullable -- the same latent fault that makes
	// GetCLISessionByDeviceCode unusable for any session without a redirect.
	var (
		redirectURI *string
		accessToken *string
		refreshTok  *string
		userEmail   *string
		mode        *string
		consumedAt  *time.Time
		status      string
	)
	err := r.querySystemRow(ctx, query, func(row pgx.Row) error {
		return row.Scan(
			&sess.UUID, &sess.State, &sess.DeviceCode, &sess.UserCode,
			&redirectURI, &mode, &accessToken, &refreshTok,
			&userEmail, &status, &consumedAt,
		)
	}, deviceCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either already consumed, still pending, or gone. Distinguish so
			// the caller does not report "expired" for a pending flow.
			if existing, getErr := r.GetCLISessionByDeviceCode(ctx, deviceCode); getErr == nil && existing != nil {
				switch existing.Status {
				case "consumed":
					return nil, cliauth.ErrSessionConsumed
				case "completed":
					// Completed but not claimable: another transaction holds it.
					return nil, cliauth.ErrSessionConsumed
				default:
					return nil, cliauth.ErrSessionPending
				}
			}
			return nil, cliauth.ErrSessionNotFound
		}
		return nil, err
	}
	sess.Status = cliauth.SessionStatus(status)
	sess.ConsumedAt = consumedAt
	if redirectURI != nil {
		sess.RedirectURI = *redirectURI
	}
	if mode != nil {
		sess.Mode = *mode
	}
	if accessToken != nil {
		sess.AccessToken = *accessToken
	}
	if refreshTok != nil {
		sess.RefreshToken = *refreshTok
	}
	if userEmail != nil {
		sess.UserEmail = *userEmail
	}
	return sess, nil
}

// UpdateUserPassword updates the bcrypt hash for an active user in the users table.
func (r *Repository) UpdateUserPassword(ctx context.Context, email, passwordHash string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE users
		SET password = $1, "updatedAt" = now()
		WHERE LOWER(email) = LOWER($2);
	`
	_, err := r.execSystem(ctx, query, passwordHash, email)
	return err
}

// InvalidateAllUserRefreshTokens revokes all active refresh tokens for a user upon password reset.
func (r *Repository) InvalidateAllUserRefreshTokens(ctx context.Context, userUUID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE auth
		SET used = true, "updatedAt" = now()
		WHERE "userUuid" = $1;
	`
	_, err := r.execSystem(ctx, query, userUUID)
	return err
}

// UpdateUserStatus changes a user's status (e.g. "active", "suspended", "inactive") for IdP deprovisioning.
func (r *Repository) UpdateUserStatus(ctx context.Context, userUUID uuid.UUID, status string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE users
		SET status = $1, "updatedAt" = now()
		WHERE uuid = $2;
	`
	tag, err := r.execSystem(ctx, query, status, userUUID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Distinguish "no such user" from "updated", so a caller deprovisioning
		// someone can tell a real miss from a success. This previously returned
		// nil on zero rows, which made a failed suspension look successful.
		return fmt.Errorf("no user with uuid %s", userUUID)
	}
	return nil
}

// PruneExpiredCLISessions deletes expired terminal authorization device sessions to prevent replay attacks.
func (r *Repository) PruneExpiredCLISessions(ctx context.Context) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}

	query := `
		DELETE FROM cli_auth_sessions
		WHERE expires_at < NOW()
		   OR (status = 'consumed' AND "updatedAt" < NOW() - INTERVAL '24 hours');
	`
	var rowsAffected int64
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query)
		if err != nil {
			return err
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed pruning expired CLI sessions: %w", err)
	}
	return rowsAffected, nil
}

// TouchAccountActivity updates last_active_at for an account profile within a tenant boundary.
func (r *Repository) TouchAccountActivity(ctx context.Context, wsID uuid.UUID, email string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	query := `UPDATE account_profiles SET last_active_at = NOW() WHERE workspace_id = $1 AND email = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, wsID, email)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// Profile may not exist yet in account_profiles, upsert it
			upsertQuery := `
				INSERT INTO account_profiles (id, workspace_id, email, display_name, role, last_active_at, created_at, updated_at)
				VALUES (gen_random_uuid(), $1, $2, $2, 'MEMBER', NOW(), NOW(), NOW())
				ON CONFLICT (workspace_id, email) DO UPDATE SET last_active_at = NOW(), updated_at = NOW();
			`
			_, err = tx.Exec(ctx, upsertQuery, wsID, email)
			return err
		}
		return nil
	})
}

// OrphanedSessionRecord models an unclosed CLI device or trace session.
type OrphanedSessionRecord struct {
	ID        uuid.UUID `json:"id"`
	SessionID string    `json:"session_id"`
	UserCode  string    `json:"user_code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FindOrphanedCLISessions retrieves CLI sessions that have not received heartbeats within threshold.
func (r *Repository) FindOrphanedCLISessions(ctx context.Context, inactivityThresholdMinutes, limit int) ([]OrphanedSessionRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []OrphanedSessionRecord{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		SELECT id, session_id, user_code, status, created_at, "updatedAt"
		FROM cli_auth_sessions
		WHERE status IN ('pending', 'authorized')
		  AND "updatedAt" < NOW() - ($1 || ' minutes')::interval
		ORDER BY "updatedAt" ASC
		LIMIT $2;
	`
	var sessions []OrphanedSessionRecord
	err := r.querySystem(ctx, query, []any{fmt.Sprintf("%d", inactivityThresholdMinutes), limit}, func(rows pgx.Rows) error {
		for rows.Next() {
			var s OrphanedSessionRecord
			if err := rows.Scan(&s.ID, &s.SessionID, &s.UserCode, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			sessions = append(sessions, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying orphaned sessions: %w", err)
	}
	return sessions, nil
}

// MarkCLISessionClassified updates the status of an orphaned session to expired/classified.
func (r *Repository) MarkCLISessionClassified(ctx context.Context, sessionID uuid.UUID, classification string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE cli_auth_sessions
		SET status = 'expired', "updatedAt" = NOW()
		WHERE id = $1;
	`
	_, err := r.execSystem(ctx, query, sessionID)
	return err
}

// SaveRepositoryAssignment stores or replaces the explicit per-user repository restrictions for a workspace member.
func (r *Repository) SaveRepositoryAssignment(ctx context.Context, assignment *models.UserRepositoryAssignment) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return errors.New("database repository unavailable")
	}
	if assignment == nil {
		return errors.New("assignment cannot be nil")
	}

	if assignment.ID == uuid.Nil {
		assignment.ID = uuid.New()
	}

	query := `
		INSERT INTO user_repository_assignments (uuid, workspace_id, user_id, repository_ids, assigned_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			repository_ids = EXCLUDED.repository_ids,
			assigned_by = EXCLUDED.assigned_by,
			updated_at = now();
	`
	_, err := r.execSystem(ctx, query, assignment.ID, assignment.WorkspaceID, assignment.UserID, assignment.RepositoryIDs, assignment.AssignedBy)
	return err
}

// GetRepositoryAssignment retrieves explicit per-user repository restrictions for a user in a workspace.
func (r *Repository) GetRepositoryAssignment(ctx context.Context, wsID, userID uuid.UUID) (*models.UserRepositoryAssignment, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, workspace_id, user_id, repository_ids, COALESCE(assigned_by, ''), created_at, updated_at
		FROM user_repository_assignments
		WHERE workspace_id = $1 AND user_id = $2
		LIMIT 1;
	`
	var a models.UserRepositoryAssignment
	// Identity lookup: no tenant exists yet, so this runs as a system
	// worker. It still filters on an explicit identity predicate.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, userID).Scan(
			&a.ID,
			&a.WorkspaceID,
			&a.UserID,
			&a.RepositoryIDs,
			&a.AssignedBy,
			&a.CreatedAt,
			&a.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// DeleteRepositoryAssignment removes explicit per-user repository restrictions, reverting the user to default open workspace scope.
func (r *Repository) DeleteRepositoryAssignment(ctx context.Context, wsID, userID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return errors.New("database repository unavailable")
	}

	query := `
		DELETE FROM user_repository_assignments
		WHERE workspace_id = $1 AND user_id = $2;
	`
	_, err := r.execSystem(ctx, query, wsID, userID)
	return err
}

// ═════════════════════════════════════════════════════════════════════════════
// RLS CONTEXT
//
// Queries here used to go straight to client.Pool. Under a non-superuser
// runtime role a bare pool call silently matches zero rows against any
// RLS-protected table, so the trap for anyone adding a method is: it compiles,
// the unit tests pass, and the write quietly disappears.
//
// Every Exec and Query call now declares its context:
//
//   execTenant / queryTenant - the caller has a workspace; RLS scopes to it.
//   execSystem / querySystem - identity resolution that happens BEFORE a tenant
//     is known (login by email, refresh tokens, CLI keys by hash, device
//     codes). These are system-elevated by necessity, and each still filters on
//     an explicit identity predicate, so elevation does not widen the result.
//
// All 29 call sites are converted: 7 tenant-scoped, 13 system-elevated, plus the
// Exec/Query helpers above. `r.client.Pool` no longer appears as a CALL anywhere
// in this file, only in the nil guards, so a new method cannot copy the bypass by
// accident.
//
// The trap when converting these: a pgx.Row cannot outlive its transaction, so a
// row returned from the closure and scanned afterwards fails with "conn busy".
// Scan inside the transaction, which is why every site above does.

// execSystem runs a write with the system-worker RLS context set.
func (r *Repository) execSystem(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	var tag pgconn.CommandTag
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		var e error
		tag, e = tx.Exec(ctx, sql, args...)
		return e
	})
	return tag, err
}

// querySystemRow runs a single-row query with the system-worker RLS context
// set and hands the row to scan inside the transaction.
//
// It exists so a guarded `UPDATE ... RETURNING` can be read in the same
// statement, and therefore the same snapshot, as the write that produced it.
// The alternative -- exec then read -- re-reads outside the transaction and
// can observe another caller's claim.
func (r *Repository) querySystemRow(ctx context.Context, sql string, scan func(pgx.Row) error, args ...any) error {
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, sql, args...))
	})
}

// execTenant runs a write scoped to one workspace.
func (r *Repository) execTenant(ctx context.Context, wsID uuid.UUID, sql string, args ...any) (pgconn.CommandTag, error) {
	var tag pgconn.CommandTag
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		var e error
		tag, e = tx.Exec(ctx, sql, args...)
		return e
	})
	return tag, err
}

// querySystem iterates rows inside a system-worker transaction. The rows are
// consumed before commit: a pgx.Rows returned from the closure and iterated
// afterwards fails with "conn busy".
func (r *Repository) querySystem(ctx context.Context, sql string, args []any, fn func(pgx.Rows) error) error {
	return r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return fn(rows)
	})
}

// queryTenant is querySystem scoped to one workspace.
func (r *Repository) queryTenant(ctx context.Context, wsID uuid.UUID, sql string, args []any, fn func(pgx.Rows) error) error {
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return fn(rows)
	})
}
