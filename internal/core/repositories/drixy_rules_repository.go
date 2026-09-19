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

// PgDrixyRulesRepository implements domain.DrixyRulesRepository.
type PgDrixyRulesRepository struct {
	pool *pgxpool.Pool
}

// NewDrixyRulesRepository instantiates a new PgDrixyRulesRepository.
func NewDrixyRulesRepository(pool *pgxpool.Pool) *PgDrixyRulesRepository {
	return &PgDrixyRulesRepository{pool: pool}
}

// FindByID retrieves a custom rule by its UUID.
func (r *PgDrixyRulesRepository) FindByID(ctx context.Context, wsID, ruleID uuid.UUID) (*domain.DrixyRules, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, rule_key, name, category,
		       severity, description, pattern, prompt_instructions, bad_example,
		       good_example, applicable_languages, is_active, is_system_default,
		       weight_multiplier, likes_count
		FROM drixy_rules
		WHERE workspace_id = $1 AND id = $2
	`
	rule := &domain.DrixyRules{}
	err := r.pool.QueryRow(ctx, query, wsID, ruleID).Scan(
		&rule.ID, &rule.CreatedAt, &rule.UpdatedAt, &rule.WorkspaceID, &rule.RuleKey,
		&rule.Name, &rule.Category, &rule.Severity, &rule.Description, &rule.Pattern,
		&rule.PromptInstructions, &rule.BadExample, &rule.GoodExample,
		&rule.ApplicableLanguages, &rule.IsActive, &rule.IsSystemDefault,
		&rule.WeightMultiplier, &rule.LikesCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying rule: %w", err)
	}
	return rule, nil
}

// FindByKey retrieves a rule by its unique rule key.
func (r *PgDrixyRulesRepository) FindByKey(ctx context.Context, wsID uuid.UUID, ruleKey string) (*domain.DrixyRules, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, rule_key, name, category,
		       severity, description, pattern, prompt_instructions, bad_example,
		       good_example, applicable_languages, is_active, is_system_default,
		       weight_multiplier, likes_count
		FROM drixy_rules
		WHERE workspace_id = $1 AND rule_key = $2
		LIMIT 1
	`
	rule := &domain.DrixyRules{}
	err := r.pool.QueryRow(ctx, query, wsID, ruleKey).Scan(
		&rule.ID, &rule.CreatedAt, &rule.UpdatedAt, &rule.WorkspaceID, &rule.RuleKey,
		&rule.Name, &rule.Category, &rule.Severity, &rule.Description, &rule.Pattern,
		&rule.PromptInstructions, &rule.BadExample, &rule.GoodExample,
		&rule.ApplicableLanguages, &rule.IsActive, &rule.IsSystemDefault,
		&rule.WeightMultiplier, &rule.LikesCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying rule by key: %w", err)
	}
	return rule, nil
}

// Create inserts a custom rule definition.
func (r *PgDrixyRulesRepository) Create(ctx context.Context, rule *domain.DrixyRules) error {
	query := `
		INSERT INTO drixy_rules (
			id, created_at, updated_at, workspace_id, rule_key, name, category,
			severity, description, pattern, prompt_instructions, bad_example,
			good_example, applicable_languages, is_active, is_system_default,
			weight_multiplier, likes_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`
	now := time.Now().UTC()
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		rule.ID, rule.CreatedAt, rule.UpdatedAt, rule.WorkspaceID, rule.RuleKey,
		rule.Name, rule.Category, rule.Severity, rule.Description, rule.Pattern,
		rule.PromptInstructions, rule.BadExample, rule.GoodExample,
		rule.ApplicableLanguages, rule.IsActive, rule.IsSystemDefault,
		rule.WeightMultiplier, rule.LikesCount,
	)
	if err != nil {
		return fmt.Errorf("failed creating rule: %w", err)
	}
	return nil
}

// Update persists rule revisions and weight multiplier adjustments.
func (r *PgDrixyRulesRepository) Update(ctx context.Context, rule *domain.DrixyRules) error {
	query := `
		UPDATE drixy_rules
		SET updated_at = $3, name = $4, category = $5, severity = $6, description = $7,
		    pattern = $8, prompt_instructions = $9, bad_example = $10, good_example = $11,
		    applicable_languages = $12, is_active = $13, weight_multiplier = $14
		WHERE workspace_id = $1 AND id = $2
	`
	rule.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		rule.WorkspaceID, rule.ID, rule.UpdatedAt, rule.Name, rule.Category, rule.Severity,
		rule.Description, rule.Pattern, rule.PromptInstructions, rule.BadExample, rule.GoodExample,
		rule.ApplicableLanguages, rule.IsActive, rule.WeightMultiplier,
	)
	if err != nil {
		return fmt.Errorf("failed updating rule: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListActive returns all active rules applicable to a workspace.
func (r *PgDrixyRulesRepository) ListActive(ctx context.Context, wsID uuid.UUID) ([]*domain.DrixyRules, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, rule_key, name, category,
		       severity, description, pattern, prompt_instructions, bad_example,
		       good_example, applicable_languages, is_active, is_system_default,
		       weight_multiplier, likes_count
		FROM drixy_rules
		WHERE workspace_id = $1 AND is_active = true
		ORDER BY category ASC, rule_key ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed listing active rules: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.DrixyRules, 0)
	for rows.Next() {
		rule := &domain.DrixyRules{}
		if err := rows.Scan(
			&rule.ID, &rule.CreatedAt, &rule.UpdatedAt, &rule.WorkspaceID, &rule.RuleKey,
			&rule.Name, &rule.Category, &rule.Severity, &rule.Description, &rule.Pattern,
			&rule.PromptInstructions, &rule.BadExample, &rule.GoodExample,
			&rule.ApplicableLanguages, &rule.IsActive, &rule.IsSystemDefault,
			&rule.WeightMultiplier, &rule.LikesCount,
		); err != nil {
			return nil, fmt.Errorf("failed scanning rule: %w", err)
		}
		items = append(items, rule)
	}
	return items, nil
}

// IncrementLikes increments developer community reactions for fine-tuning weight.
func (r *PgDrixyRulesRepository) IncrementLikes(ctx context.Context, wsID, ruleID uuid.UUID) error {
	query := `
		UPDATE drixy_rules
		SET likes_count = likes_count + 1, updated_at = $3
		WHERE workspace_id = $1 AND id = $2
	`
	cmd, err := r.pool.Exec(ctx, query, wsID, ruleID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed incrementing rule likes: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PgOrganizationParametersRepository manages tenant-level review toggles.
type PgOrganizationParametersRepository struct {
	pool *pgxpool.Pool
}

// NewOrganizationParametersRepository instantiates a new repository.
func NewOrganizationParametersRepository(pool *pgxpool.Pool) *PgOrganizationParametersRepository {
	return &PgOrganizationParametersRepository{pool: pool}
}

// FindByOrganization retrieves parameters configured for an organization.
func (r *PgOrganizationParametersRepository) FindByOrganization(ctx context.Context, wsID, orgID uuid.UUID) (*domain.OrganizationParameters, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, strictness_level,
		       auto_review_drafts, min_severity, ignore_draft_prs, max_files_per_review,
		       max_diff_bytes, enabled_rules, excluded_paths, custom_prompt,
		       telemetry_enabled, notification_channels
		FROM organization_parameters
		WHERE workspace_id = $1 AND organization_id = $2
		LIMIT 1
	`
	p := &domain.OrganizationParameters{}
	err := r.pool.QueryRow(ctx, query, wsID, orgID).Scan(
		&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.WorkspaceID, &p.OrganizationID, &p.StrictnessLevel,
		&p.AutoReviewDrafts, &p.MinSeverity, &p.IgnoreDraftPRs, &p.MaxFilesPerReview,
		&p.MaxDiffBytes, &p.EnabledRules, &p.ExcludedPaths, &p.CustomPrompt,
		&p.TelemetryEnabled, &p.NotificationChannels,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying organization parameters: %w", err)
	}
	return p, nil
}

// Upsert creates or updates organization parameters atomically.
func (r *PgOrganizationParametersRepository) Upsert(ctx context.Context, p *domain.OrganizationParameters) error {
	query := `
		INSERT INTO organization_parameters (
			id, created_at, updated_at, workspace_id, organization_id, strictness_level,
			auto_review_drafts, min_severity, ignore_draft_prs, max_files_per_review,
			max_diff_bytes, enabled_rules, excluded_paths, custom_prompt,
			telemetry_enabled, notification_channels
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (workspace_id, organization_id) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    strictness_level = EXCLUDED.strictness_level,
		    auto_review_drafts = EXCLUDED.auto_review_drafts,
		    min_severity = EXCLUDED.min_severity,
		    ignore_draft_prs = EXCLUDED.ignore_draft_prs,
		    max_files_per_review = EXCLUDED.max_files_per_review,
		    max_diff_bytes = EXCLUDED.max_diff_bytes,
		    enabled_rules = EXCLUDED.enabled_rules,
		    excluded_paths = EXCLUDED.excluded_paths,
		    custom_prompt = EXCLUDED.custom_prompt,
		    telemetry_enabled = EXCLUDED.telemetry_enabled,
		    notification_channels = EXCLUDED.notification_channels
	`
	now := time.Now().UTC()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		p.ID, p.CreatedAt, p.UpdatedAt, p.WorkspaceID, p.OrganizationID, p.StrictnessLevel,
		p.AutoReviewDrafts, p.MinSeverity, p.IgnoreDraftPRs, p.MaxFilesPerReview,
		p.MaxDiffBytes, p.EnabledRules, p.ExcludedPaths, p.CustomPrompt,
		p.TelemetryEnabled, p.NotificationChannels,
	)
	if err != nil {
		return fmt.Errorf("failed upserting organization parameters: %w", err)
	}
	return nil
}
