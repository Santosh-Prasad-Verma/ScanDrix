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

// BillingTransaction models an enterprise billing charge or credit.
type BillingTransaction struct {
	domain.TenantScopedEntity
	AmountCents       int64     `json:"amount_cents" db:"amount_cents"`
	Currency          string    `json:"currency" db:"currency"`
	Provider          string    `json:"provider" db:"provider"` // "STRIPE", "ZOHO", "INVOICE"
	TransactionType   string    `json:"transaction_type" db:"transaction_type"` // "SUBSCRIPTION", "SEATS_ADDON", "USAGE_OVERAGE"
	Status            string    `json:"status" db:"status"`
	InvoiceID         *string   `json:"invoice_id,omitempty" db:"invoice_id"`
	PaymentMethodID   *string   `json:"payment_method_id,omitempty" db:"payment_method_id"`
}

// PlanConfiguration holds enterprise subscription plan limits and feature bundles.
type PlanConfiguration struct {
	domain.BaseEntity
	Tier                    string  `json:"tier" db:"tier"`
	Name                    string  `json:"name" db:"name"`
	SeatPriceCents          int64   `json:"seat_price_cents" db:"seat_price_cents"`
	IncludedSeats           int     `json:"included_seats" db:"included_seats"`
	IncludedReviewsPerMonth int     `json:"included_reviews_per_month" db:"included_reviews_per_month"`
	MaxRepos                int     `json:"max_repos" db:"max_repos"`
	SAMLIncluded            bool    `json:"saml_included" db:"saml_included"`
	CustomRulesIncluded     bool    `json:"custom_rules_included" db:"custom_rules_included"`
	BYOKIncluded            bool    `json:"byok_included" db:"byok_included"`
	IsActive                bool    `json:"is_active" db:"is_active"`
}

// OrganizationLicense manages cryptographically signed enterprise license keys.
type OrganizationLicense struct {
	domain.TenantScopedEntity
	LicenseKey     string    `json:"license_key" db:"license_key"`
	Tier           string    `json:"tier" db:"tier"`
	SeatsAllocated int       `json:"seats_allocated" db:"seats_allocated"`
	SeatsUsed      int       `json:"seats_used" db:"seats_used"`
	ValidFrom      time.Time `json:"valid_from" db:"valid_from"`
	ValidUntil     time.Time `json:"valid_until" db:"valid_until"`
	IsActive       bool      `json:"is_active" db:"is_active"`
}

// PgBillingRepository manages billing transactions and plan configurations.
type PgBillingRepository struct {
	pool *pgxpool.Pool
}

// NewBillingRepository instantiates a new PgBillingRepository.
func NewBillingRepository(pool *pgxpool.Pool) *PgBillingRepository {
	return &PgBillingRepository{pool: pool}
}

// RecordTransaction persists a new billing transaction.
func (r *PgBillingRepository) RecordTransaction(ctx context.Context, tx *BillingTransaction) error {
	query := `
		INSERT INTO billing_transactions (
			id, created_at, updated_at, workspace_id, amount_cents, currency,
			provider, transaction_type, status, invoice_id, payment_method_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}
	tx.CreatedAt = now
	tx.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		tx.ID, tx.CreatedAt, tx.UpdatedAt, tx.WorkspaceID, tx.AmountCents, tx.Currency,
		tx.Provider, tx.TransactionType, tx.Status, tx.InvoiceID, tx.PaymentMethodID,
	)
	if err != nil {
		return fmt.Errorf("failed recording billing transaction: %w", err)
	}
	return nil
}

// ListTransactionsByWorkspace retrieves billing history for an enterprise tenant.
func (r *PgBillingRepository) ListTransactionsByWorkspace(ctx context.Context, wsID uuid.UUID, limit int) ([]*BillingTransaction, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, created_at, updated_at, workspace_id, amount_cents, currency,
		       provider, transaction_type, status, invoice_id, payment_method_id
		FROM billing_transactions
		WHERE workspace_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, wsID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying billing transactions: %w", err)
	}
	defer rows.Close()

	items := make([]*BillingTransaction, 0, limit)
	for rows.Next() {
		tx := &BillingTransaction{}
		if err := rows.Scan(
			&tx.ID, &tx.CreatedAt, &tx.UpdatedAt, &tx.WorkspaceID, &tx.AmountCents, &tx.Currency,
			&tx.Provider, &tx.TransactionType, &tx.Status, &tx.InvoiceID, &tx.PaymentMethodID,
		); err != nil {
			return nil, fmt.Errorf("failed scanning billing transaction: %w", err)
		}
		items = append(items, tx)
	}
	return items, nil
}

// GetPlanConfig retrieves plan quotas and features by tier.
func (r *PgBillingRepository) GetPlanConfig(ctx context.Context, tier string) (*PlanConfiguration, error) {
	query := `
		SELECT id, created_at, updated_at, tier, name, seat_price_cents, included_seats,
		       included_reviews_per_month, max_repos, saml_included, custom_rules_included,
		       byok_included, is_active
		FROM plan_configurations
		WHERE tier = $1
		LIMIT 1
	`
	plan := &PlanConfiguration{}
	err := r.pool.QueryRow(ctx, query, tier).Scan(
		&plan.ID, &plan.CreatedAt, &plan.UpdatedAt, &plan.Tier, &plan.Name,
		&plan.SeatPriceCents, &plan.IncludedSeats, &plan.IncludedReviewsPerMonth,
		&plan.MaxRepos, &plan.SAMLIncluded, &plan.CustomRulesIncluded,
		&plan.BYOKIncluded, &plan.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying plan configuration: %w", err)
	}
	return plan, nil
}

// PgLicenseRepository manages enterprise license keys.
type PgLicenseRepository struct {
	pool *pgxpool.Pool
}

// NewLicenseRepository instantiates a new PgLicenseRepository.
func NewLicenseRepository(pool *pgxpool.Pool) *PgLicenseRepository {
	return &PgLicenseRepository{pool: pool}
}

// FindByWorkspace retrieves active license data for a workspace.
func (r *PgLicenseRepository) FindByWorkspace(ctx context.Context, wsID uuid.UUID) (*OrganizationLicense, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, license_key, tier,
		       seats_allocated, seats_used, valid_from, valid_until, is_active
		FROM organization_licenses
		WHERE workspace_id = $1 AND is_active = true
		LIMIT 1
	`
	lic := &OrganizationLicense{}
	err := r.pool.QueryRow(ctx, query, wsID).Scan(
		&lic.ID, &lic.CreatedAt, &lic.UpdatedAt, &lic.WorkspaceID, &lic.LicenseKey, &lic.Tier,
		&lic.SeatsAllocated, &lic.SeatsUsed, &lic.ValidFrom, &lic.ValidUntil, &lic.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying license: %w", err)
	}
	return lic, nil
}

// PgCliAuthSessionRepository handles OAuth device authorization flow for `scandrix login`.
type PgCliAuthSessionRepository struct {
	pool *pgxpool.Pool
}

// NewCliAuthSessionRepository instantiates a new repository.
func NewCliAuthSessionRepository(pool *pgxpool.Pool) *PgCliAuthSessionRepository {
	return &PgCliAuthSessionRepository{pool: pool}
}

// CreateSession initiates a new pending CLI login exchange.
func (r *PgCliAuthSessionRepository) CreateSession(ctx context.Context, sess *domain.CliAuthSession) error {
	query := `
		INSERT INTO cli_auth_sessions (
			id, created_at, updated_at, session_code, user_code, status,
			user_id, workspace_id, token_payload, client_ip, user_agent, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	now := time.Now().UTC()
	if sess.ID == uuid.Nil {
		sess.ID = uuid.New()
	}
	sess.CreatedAt = now
	sess.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		sess.ID, sess.CreatedAt, sess.UpdatedAt, sess.SessionCode, sess.UserCode, sess.Status,
		sess.UserID, sess.WorkspaceID, sess.TokenPayload, sess.ClientIP, sess.UserAgent, sess.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating cli auth session: %w", err)
	}
	return nil
}

// FindByUserCode looks up an active login session by the 8-character code shown to the developer.
func (r *PgCliAuthSessionRepository) FindByUserCode(ctx context.Context, userCode string) (*domain.CliAuthSession, error) {
	query := `
		SELECT id, created_at, updated_at, session_code, user_code, status,
		       user_id, workspace_id, token_payload, client_ip, user_agent, expires_at
		FROM cli_auth_sessions
		WHERE user_code = $1 AND expires_at > now()
		LIMIT 1
	`
	sess := &domain.CliAuthSession{}
	err := r.pool.QueryRow(ctx, query, userCode).Scan(
		&sess.ID, &sess.CreatedAt, &sess.UpdatedAt, &sess.SessionCode, &sess.UserCode, &sess.Status,
		&sess.UserID, &sess.WorkspaceID, &sess.TokenPayload, &sess.ClientIP, &sess.UserAgent, &sess.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying cli session: %w", err)
	}
	return sess, nil
}

// Authorize updates the session status to AUTHORIZED and attaches the encrypted token payload.
func (r *PgCliAuthSessionRepository) Authorize(ctx context.Context, sessionCode string, userID, wsID uuid.UUID, tokenPayload string) error {
	query := `
		UPDATE cli_auth_sessions
		SET status = 'AUTHORIZED', user_id = $2, workspace_id = $3,
		    token_payload = $4, updated_at = $5
		WHERE session_code = $1 AND status = 'PENDING' AND expires_at > now()
	`
	cmd, err := r.pool.Exec(ctx, query, sessionCode, userID, wsID, tokenPayload, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed authorizing cli session: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
