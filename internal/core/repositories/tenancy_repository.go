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

// Common database query errors.
var (
	ErrNotFound = errors.New("requested resource not found")
)

// PgWorkspaceRepository implements domain.WorkspaceRepository using PostgreSQL 17.
type PgWorkspaceRepository struct {
	pool *pgxpool.Pool
}

// NewWorkspaceRepository instantiates a new PgWorkspaceRepository.
func NewWorkspaceRepository(pool *pgxpool.Pool) *PgWorkspaceRepository {
	return &PgWorkspaceRepository{pool: pool}
}

// FindByID retrieves a workspace by its primary UUID.
func (r *PgWorkspaceRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	query := `
		SELECT id, created_at, updated_at, slug, name, status, tier,
		       spend_limit_usd, current_month_spend_usd, enforce_spend_limit,
		       trial_ends_at, custom_domain, saml_required, allowed_email_domains, metadata
		FROM workspaces
		WHERE id = $1
	`
	ws := &domain.Workspace{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&ws.ID, &ws.CreatedAt, &ws.UpdatedAt, &ws.Slug, &ws.Name, &ws.Status, &ws.Tier,
		&ws.SpendLimitUSD, &ws.CurrentMonthSpendUSD, &ws.EnforceSpendLimit,
		&ws.TrialEndsAt, &ws.CustomDomain, &ws.SAMLRequired, &ws.AllowedEmailDomains, &ws.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying workspace by ID: %w", err)
	}
	return ws, nil
}

// FindBySlug retrieves a workspace by unique slug.
func (r *PgWorkspaceRepository) FindBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	query := `
		SELECT id, created_at, updated_at, slug, name, status, tier,
		       spend_limit_usd, current_month_spend_usd, enforce_spend_limit,
		       trial_ends_at, custom_domain, saml_required, allowed_email_domains, metadata
		FROM workspaces
		WHERE slug = $1
	`
	ws := &domain.Workspace{}
	err := r.pool.QueryRow(ctx, query, slug).Scan(
		&ws.ID, &ws.CreatedAt, &ws.UpdatedAt, &ws.Slug, &ws.Name, &ws.Status, &ws.Tier,
		&ws.SpendLimitUSD, &ws.CurrentMonthSpendUSD, &ws.EnforceSpendLimit,
		&ws.TrialEndsAt, &ws.CustomDomain, &ws.SAMLRequired, &ws.AllowedEmailDomains, &ws.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying workspace by slug: %w", err)
	}
	return ws, nil
}

// Create inserts a new workspace record.
func (r *PgWorkspaceRepository) Create(ctx context.Context, ws *domain.Workspace) error {
	query := `
		INSERT INTO workspaces (
			id, created_at, updated_at, slug, name, status, tier,
			spend_limit_usd, current_month_spend_usd, enforce_spend_limit,
			trial_ends_at, custom_domain, saml_required, allowed_email_domains, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	now := time.Now().UTC()
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	ws.CreatedAt = now
	ws.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		ws.ID, ws.CreatedAt, ws.UpdatedAt, ws.Slug, ws.Name, ws.Status, ws.Tier,
		ws.SpendLimitUSD, ws.CurrentMonthSpendUSD, ws.EnforceSpendLimit,
		ws.TrialEndsAt, ws.CustomDomain, ws.SAMLRequired, ws.AllowedEmailDomains, ws.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed creating workspace: %w", err)
	}
	return nil
}

// Update persists modifications to an existing workspace.
func (r *PgWorkspaceRepository) Update(ctx context.Context, ws *domain.Workspace) error {
	query := `
		UPDATE workspaces
		SET updated_at = $2, name = $3, status = $4, tier = $5,
		    spend_limit_usd = $6, current_month_spend_usd = $7, enforce_spend_limit = $8,
		    custom_domain = $9, saml_required = $10, allowed_email_domains = $11, metadata = $12
		WHERE id = $1
	`
	ws.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		ws.ID, ws.UpdatedAt, ws.Name, ws.Status, ws.Tier,
		ws.SpendLimitUSD, ws.CurrentMonthSpendUSD, ws.EnforceSpendLimit,
		ws.CustomDomain, ws.SAMLRequired, ws.AllowedEmailDomains, ws.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed updating workspace: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns a paginated view of workspaces for administrative operations.
func (r *PgWorkspaceRepository) List(ctx context.Context, pq domain.PaginationQuery) (*domain.PaginatedResult[*domain.Workspace], error) {
	pq.EnsureDefaults()

	var total int64
	countQuery := `SELECT count(*) FROM workspaces`
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed counting workspaces: %w", err)
	}

	offset := (pq.Page - 1) * pq.PageSize
	query := fmt.Sprintf(`
		SELECT id, created_at, updated_at, slug, name, status, tier,
		       spend_limit_usd, current_month_spend_usd, enforce_spend_limit,
		       trial_ends_at, custom_domain, saml_required, allowed_email_domains, metadata
		FROM workspaces
		ORDER BY %s %s
		LIMIT $1 OFFSET $2
	`, pq.OrderBy, pq.OrderDir)

	rows, err := r.pool.Query(ctx, query, pq.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed listing workspaces: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.Workspace, 0, pq.PageSize)
	for rows.Next() {
		ws := &domain.Workspace{}
		if err := rows.Scan(
			&ws.ID, &ws.CreatedAt, &ws.UpdatedAt, &ws.Slug, &ws.Name, &ws.Status, &ws.Tier,
			&ws.SpendLimitUSD, &ws.CurrentMonthSpendUSD, &ws.EnforceSpendLimit,
			&ws.TrialEndsAt, &ws.CustomDomain, &ws.SAMLRequired, &ws.AllowedEmailDomains, &ws.Metadata,
		); err != nil {
			return nil, fmt.Errorf("failed scanning workspace row: %w", err)
		}
		items = append(items, ws)
	}

	totalPages := int(total) / pq.PageSize
	if int(total)%pq.PageSize != 0 {
		totalPages++
	}

	return &domain.PaginatedResult[*domain.Workspace]{
		Items:      items,
		TotalCount: total,
		Page:       pq.Page,
		PageSize:   pq.PageSize,
		TotalPages: totalPages,
		HasNext:    pq.Page < totalPages,
	}, nil
}

// PgOrganizationRepository implements domain.OrganizationRepository.
type PgOrganizationRepository struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepository instantiates a new PgOrganizationRepository.
func NewOrganizationRepository(pool *pgxpool.Pool) *PgOrganizationRepository {
	return &PgOrganizationRepository{pool: pool}
}

// FindByID retrieves an organization within a workspace boundary.
func (r *PgOrganizationRepository) FindByID(ctx context.Context, wsID, orgID uuid.UUID) (*domain.Organization, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, external_id, name,
		       avatar_url, billing_email, scm_provider, installation_id, is_active, settings
		FROM organizations
		WHERE workspace_id = $1 AND id = $2
	`
	org := &domain.Organization{}
	err := r.pool.QueryRow(ctx, query, wsID, orgID).Scan(
		&org.ID, &org.CreatedAt, &org.UpdatedAt, &org.WorkspaceID, &org.ExternalID, &org.Name,
		&org.AvatarURL, &org.BillingEmail, &org.SCMProvider, &org.InstallationID, &org.IsActive, &org.Settings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying organization: %w", err)
	}
	return org, nil
}

// FindByExternalID locates an organization across SCM installations.
func (r *PgOrganizationRepository) FindByExternalID(ctx context.Context, provider, externalID string) (*domain.Organization, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, external_id, name,
		       avatar_url, billing_email, scm_provider, installation_id, is_active, settings
		FROM organizations
		WHERE scm_provider = $1 AND external_id = $2
		LIMIT 1
	`
	org := &domain.Organization{}
	err := r.pool.QueryRow(ctx, query, provider, externalID).Scan(
		&org.ID, &org.CreatedAt, &org.UpdatedAt, &org.WorkspaceID, &org.ExternalID, &org.Name,
		&org.AvatarURL, &org.BillingEmail, &org.SCMProvider, &org.InstallationID, &org.IsActive, &org.Settings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying organization by external ID: %w", err)
	}
	return org, nil
}

// Create inserts a new organization record.
func (r *PgOrganizationRepository) Create(ctx context.Context, org *domain.Organization) error {
	query := `
		INSERT INTO organizations (
			id, created_at, updated_at, workspace_id, external_id, name,
			avatar_url, billing_email, scm_provider, installation_id, is_active, settings
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	now := time.Now().UTC()
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	org.CreatedAt = now
	org.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		org.ID, org.CreatedAt, org.UpdatedAt, org.WorkspaceID, org.ExternalID, org.Name,
		org.AvatarURL, org.BillingEmail, org.SCMProvider, org.InstallationID, org.IsActive, org.Settings,
	)
	if err != nil {
		return fmt.Errorf("failed creating organization: %w", err)
	}
	return nil
}

// Update modifies an existing organization.
func (r *PgOrganizationRepository) Update(ctx context.Context, org *domain.Organization) error {
	query := `
		UPDATE organizations
		SET updated_at = $3, name = $4, avatar_url = $5, billing_email = $6,
		    installation_id = $7, is_active = $8, settings = $9
		WHERE workspace_id = $1 AND id = $2
	`
	org.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		org.WorkspaceID, org.ID, org.UpdatedAt, org.Name, org.AvatarURL, org.BillingEmail,
		org.InstallationID, org.IsActive, org.Settings,
	)
	if err != nil {
		return fmt.Errorf("failed updating organization: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByWorkspace lists all organizations attached to a workspace.
func (r *PgOrganizationRepository) ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*domain.Organization, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, external_id, name,
		       avatar_url, billing_email, scm_provider, installation_id, is_active, settings
		FROM organizations
		WHERE workspace_id = $1
		ORDER BY name ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed listing organizations: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.Organization, 0)
	for rows.Next() {
		org := &domain.Organization{}
		if err := rows.Scan(
			&org.ID, &org.CreatedAt, &org.UpdatedAt, &org.WorkspaceID, &org.ExternalID, &org.Name,
			&org.AvatarURL, &org.BillingEmail, &org.SCMProvider, &org.InstallationID, &org.IsActive, &org.Settings,
		); err != nil {
			return nil, fmt.Errorf("failed scanning organization row: %w", err)
		}
		items = append(items, org)
	}
	return items, nil
}

// PgUserRepository implements domain.UserRepository.
type PgUserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository instantiates a new PgUserRepository.
func NewUserRepository(pool *pgxpool.Pool) *PgUserRepository {
	return &PgUserRepository{pool: pool}
}

// FindByID retrieves a user within a workspace boundary.
func (r *PgUserRepository) FindByID(ctx context.Context, wsID, userID uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, email, full_name,
		       avatar_url, is_active, is_super_admin, email_verified, last_login_at
		FROM users
		WHERE workspace_id = $1 AND id = $2
	`
	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, wsID, userID).Scan(
		&user.ID, &user.CreatedAt, &user.UpdatedAt, &user.WorkspaceID, &user.Email, &user.FullName,
		&user.AvatarURL, &user.IsActive, &user.IsSuperAdmin, &user.EmailVerified, &user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying user: %w", err)
	}
	return user, nil
}

// FindByEmail locates a user across workspaces by email.
func (r *PgUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, email, full_name,
		       avatar_url, is_active, is_super_admin, email_verified, last_login_at
		FROM users
		WHERE email = $1
		LIMIT 1
	`
	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.CreatedAt, &user.UpdatedAt, &user.WorkspaceID, &user.Email, &user.FullName,
		&user.AvatarURL, &user.IsActive, &user.IsSuperAdmin, &user.EmailVerified, &user.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying user by email: %w", err)
	}
	return user, nil
}

// Create inserts a new user record.
func (r *PgUserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (
			id, created_at, updated_at, workspace_id, email, full_name,
			avatar_url, is_active, is_super_admin, email_verified, last_login_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		user.ID, user.CreatedAt, user.UpdatedAt, user.WorkspaceID, user.Email, user.FullName,
		user.AvatarURL, user.IsActive, user.IsSuperAdmin, user.EmailVerified, user.LastLoginAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating user: %w", err)
	}
	return nil
}

// Update modifies an existing user record.
func (r *PgUserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET updated_at = $3, full_name = $4, avatar_url = $5, is_active = $6,
		    email_verified = $7, last_login_at = $8
		WHERE workspace_id = $1 AND id = $2
	`
	user.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		user.WorkspaceID, user.ID, user.UpdatedAt, user.FullName, user.AvatarURL,
		user.IsActive, user.EmailVerified, user.LastLoginAt,
	)
	if err != nil {
		return fmt.Errorf("failed updating user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete marks a user as inactive.
func (r *PgUserRepository) Delete(ctx context.Context, wsID, userID uuid.UUID) error {
	query := `
		UPDATE users
		SET is_active = false, updated_at = $3
		WHERE workspace_id = $1 AND id = $2
	`
	cmd, err := r.pool.Exec(ctx, query, wsID, userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed soft-deleting user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
