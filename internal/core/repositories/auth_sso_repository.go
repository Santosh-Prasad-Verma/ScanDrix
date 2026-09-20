package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgAuthRepository manages password hashes, MFA credentials, and lockout mechanisms.
type PgAuthRepository struct {
	pool *pgxpool.Pool
}

// NewAuthRepository instantiates a new PgAuthRepository.
func NewAuthRepository(pool *pgxpool.Pool) *PgAuthRepository {
	return &PgAuthRepository{pool: pool}
}

// FindByUserID retrieves local authentication credentials for a user.
func (r *PgAuthRepository) FindByUserID(ctx context.Context, wsID, userID uuid.UUID) (*domain.Auth, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, user_id, password_hash,
		       salt, mfa_secret, mfa_enabled, failed_login_attempts, locked_until, password_changed_at
		FROM auth
		WHERE workspace_id = $1 AND user_id = $2
	`
	a := &domain.Auth{}
	err := r.pool.QueryRow(ctx, query, wsID, userID).Scan(
		&a.ID, &a.CreatedAt, &a.UpdatedAt, &a.WorkspaceID, &a.UserID, &a.PasswordHash,
		&a.Salt, &a.MFASecret, &a.MFAEnabled, &a.FailedLoginAttempts, &a.LockedUntil, &a.PasswordChangedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying auth record: %w", err)
	}
	return a, nil
}

// Create inserts a new local credentials record.
func (r *PgAuthRepository) Create(ctx context.Context, a *domain.Auth) error {
	query := `
		INSERT INTO auth (
			id, created_at, updated_at, workspace_id, user_id, password_hash,
			salt, mfa_secret, mfa_enabled, failed_login_attempts, locked_until, password_changed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	now := time.Now().UTC()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.PasswordChangedAt.IsZero() {
		a.PasswordChangedAt = now
	}

	_, err := r.pool.Exec(ctx, query,
		a.ID, a.CreatedAt, a.UpdatedAt, a.WorkspaceID, a.UserID, a.PasswordHash,
		a.Salt, a.MFASecret, a.MFAEnabled, a.FailedLoginAttempts, a.LockedUntil, a.PasswordChangedAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating auth record: %w", err)
	}
	return nil
}

// UpdatePassword updates password credentials and resets failed login counters.
func (r *PgAuthRepository) UpdatePassword(ctx context.Context, wsID, userID uuid.UUID, hash, salt string) error {
	now := time.Now().UTC()
	query := `
		UPDATE auth
		SET password_hash = $3, salt = $4, password_changed_at = $5,
		    failed_login_attempts = 0, locked_until = NULL, updated_at = $5
		WHERE workspace_id = $1 AND user_id = $2
	`
	cmd, err := r.pool.Exec(ctx, query, wsID, userID, hash, salt, now)
	if err != nil {
		return fmt.Errorf("failed updating password: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordFailedAttempt increments the failed login attempt counter and applies lockout thresholds.
func (r *PgAuthRepository) RecordFailedAttempt(ctx context.Context, wsID, userID uuid.UUID, maxAttempts int, lockDuration time.Duration) (isLocked bool, err error) {
	now := time.Now().UTC()
	query := `
		UPDATE auth
		SET failed_login_attempts = failed_login_attempts + 1,
		    locked_until = CASE
		        WHEN failed_login_attempts + 1 >= $3 THEN $4::timestamptz
		        ELSE locked_until
		    END,
		    updated_at = $5
		WHERE workspace_id = $1 AND user_id = $2
		RETURNING failed_login_attempts, locked_until
	`
	lockUntil := now.Add(lockDuration)
	var currentAttempts int
	var currentLockedUntil *time.Time

	err = r.pool.QueryRow(ctx, query, wsID, userID, maxAttempts, lockUntil, now).Scan(&currentAttempts, &currentLockedUntil)
	if err != nil {
		return false, fmt.Errorf("failed recording login failure: %w", err)
	}
	return currentLockedUntil != nil && currentLockedUntil.After(now), nil
}

// ResetFailedAttempts clears the failed login counter upon successful authentication.
func (r *PgAuthRepository) ResetFailedAttempts(ctx context.Context, wsID, userID uuid.UUID) error {
	query := `
		UPDATE auth
		SET failed_login_attempts = 0, locked_until = NULL, updated_at = $3
		WHERE workspace_id = $1 AND user_id = $2
	`
	_, err := r.pool.Exec(ctx, query, wsID, userID, time.Now().UTC())
	return err
}

// PgAuthIntegrationRepository stores third-party OAuth provider connections.
type PgAuthIntegrationRepository struct {
	pool *pgxpool.Pool
}

// NewAuthIntegrationRepository instantiates a new repository.
func NewAuthIntegrationRepository(pool *pgxpool.Pool) *PgAuthIntegrationRepository {
	return &PgAuthIntegrationRepository{pool: pool}
}

// FindByProvider locates an OAuth identity link for a user.
func (r *PgAuthIntegrationRepository) FindByProvider(ctx context.Context, wsID, userID uuid.UUID, provider string) (*domain.AuthIntegration, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, user_id, provider,
		       provider_account_id, provider_username, encrypted_access_token,
		       encrypted_refresh_token, token_expires_at, scopes, profile_data
		FROM auth_integrations
		WHERE workspace_id = $1 AND user_id = $2 AND provider = $3
	`
	ai := &domain.AuthIntegration{}
	err := r.pool.QueryRow(ctx, query, wsID, userID, provider).Scan(
		&ai.ID, &ai.CreatedAt, &ai.UpdatedAt, &ai.WorkspaceID, &ai.UserID, &ai.Provider,
		&ai.ProviderAccountID, &ai.ProviderUsername, &ai.EncryptedAccessToken,
		&ai.EncryptedRefreshToken, &ai.TokenExpiresAt, &ai.Scopes, &ai.ProfileData,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying auth integration: %w", err)
	}
	return ai, nil
}

// Upsert creates or refreshes an OAuth provider identity link.
func (r *PgAuthIntegrationRepository) Upsert(ctx context.Context, ai *domain.AuthIntegration) error {
	query := `
		INSERT INTO auth_integrations (
			id, created_at, updated_at, workspace_id, user_id, provider,
			provider_account_id, provider_username, encrypted_access_token,
			encrypted_refresh_token, token_expires_at, scopes, profile_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (workspace_id, user_id, provider) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    provider_account_id = EXCLUDED.provider_account_id,
		    provider_username = EXCLUDED.provider_username,
		    encrypted_access_token = EXCLUDED.encrypted_access_token,
		    encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
		    token_expires_at = EXCLUDED.token_expires_at,
		    scopes = EXCLUDED.scopes,
		    profile_data = EXCLUDED.profile_data
	`
	now := time.Now().UTC()
	if ai.ID == uuid.Nil {
		ai.ID = uuid.New()
	}
	ai.CreatedAt = now
	ai.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		ai.ID, ai.CreatedAt, ai.UpdatedAt, ai.WorkspaceID, ai.UserID, ai.Provider,
		ai.ProviderAccountID, ai.ProviderUsername, ai.EncryptedAccessToken,
		ai.EncryptedRefreshToken, ai.TokenExpiresAt, ai.Scopes, ai.ProfileData,
	)
	if err != nil {
		return fmt.Errorf("failed upserting auth integration: %w", err)
	}
	return nil
}

// PgSSOConfigRepository stores enterprise SAML 2.0 and OIDC settings.
type PgSSOConfigRepository struct {
	pool *pgxpool.Pool
}

// NewSSOConfigRepository instantiates a new PgSSOConfigRepository.
func NewSSOConfigRepository(pool *pgxpool.Pool) *PgSSOConfigRepository {
	return &PgSSOConfigRepository{pool: pool}
}

// FindByOrganization retrieves SSO configuration for an enterprise tenant.
func (r *PgSSOConfigRepository) FindByOrganization(ctx context.Context, wsID, orgID uuid.UUID) (*domain.SSOConfig, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, provider_type,
		       idp_entity_id, idp_sso_url, encrypted_certificate, attribute_mapping,
		       allow_idp_initiated, enforce_sso, is_active
		FROM sso_configs
		WHERE workspace_id = $1 AND organization_id = $2
	`
	sso := &domain.SSOConfig{}
	err := r.pool.QueryRow(ctx, query, wsID, orgID).Scan(
		&sso.ID, &sso.CreatedAt, &sso.UpdatedAt, &sso.WorkspaceID, &sso.OrganizationID, &sso.ProviderType,
		&sso.IdpEntityID, &sso.IdpSSOURL, &sso.EncryptedCertificate, &sso.AttributeMapping,
		&sso.AllowIdpInitiated, &sso.EnforceSSO, &sso.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying sso config: %w", err)
	}
	return sso, nil
}

// Upsert creates or modifies SSO settings for an enterprise organization.
func (r *PgSSOConfigRepository) Upsert(ctx context.Context, sso *domain.SSOConfig) error {
	query := `
		INSERT INTO sso_configs (
			id, created_at, updated_at, workspace_id, organization_id, provider_type,
			idp_entity_id, idp_sso_url, encrypted_certificate, attribute_mapping,
			allow_idp_initiated, enforce_sso, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (workspace_id, organization_id) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    provider_type = EXCLUDED.provider_type,
		    idp_entity_id = EXCLUDED.idp_entity_id,
		    idp_sso_url = EXCLUDED.idp_sso_url,
		    encrypted_certificate = EXCLUDED.encrypted_certificate,
		    attribute_mapping = EXCLUDED.attribute_mapping,
		    allow_idp_initiated = EXCLUDED.allow_idp_initiated,
		    enforce_sso = EXCLUDED.enforce_sso,
		    is_active = EXCLUDED.is_active
	`
	now := time.Now().UTC()
	if sso.ID == uuid.Nil {
		sso.ID = uuid.New()
	}
	sso.CreatedAt = now
	sso.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		sso.ID, sso.CreatedAt, sso.UpdatedAt, sso.WorkspaceID, sso.OrganizationID, sso.ProviderType,
		sso.IdpEntityID, sso.IdpSSOURL, sso.EncryptedCertificate, sso.AttributeMapping,
		sso.AllowIdpInitiated, sso.EnforceSSO, sso.IsActive,
	)
	if err != nil {
		return fmt.Errorf("failed upserting sso config: %w", err)
	}
	return nil
}
