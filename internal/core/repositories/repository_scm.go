package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgTrackedRepositoryRepository implements domain.TrackedRepositoryRepository.
type PgTrackedRepositoryRepository struct {
	pool *pgxpool.Pool
}

// NewTrackedRepositoryRepository instantiates a new PgTrackedRepositoryRepository.
func NewTrackedRepositoryRepository(pool *pgxpool.Pool) *PgTrackedRepositoryRepository {
	return &PgTrackedRepositoryRepository{pool: pool}
}

// FindByID retrieves a repository by its UUID within a workspace.
func (r *PgTrackedRepositoryRepository) FindByID(ctx context.Context, wsID, repoID uuid.UUID) (*domain.TrackedRepository, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, scm_provider,
		       external_repo_id, full_name, default_branch, is_private, is_active,
		       webhooks_enabled, last_scanned_at, config_file_path, custom_prompt, settings
		FROM tracked_repositories
		WHERE workspace_id = $1 AND id = $2
	`
	repo := &domain.TrackedRepository{}
	err := r.pool.QueryRow(ctx, query, wsID, repoID).Scan(
		&repo.ID, &repo.CreatedAt, &repo.UpdatedAt, &repo.WorkspaceID, &repo.OrganizationID,
		&repo.SCMProvider, &repo.ExternalRepoID, &repo.FullName, &repo.DefaultBranch,
		&repo.IsPrivate, &repo.IsActive, &repo.WebhooksEnabled, &repo.LastScannedAt,
		&repo.ConfigFilePath, &repo.CustomPrompt, &repo.Settings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying tracked repository: %w", err)
	}
	return repo, nil
}

// FindByFullName retrieves a repository by full name within a workspace.
func (r *PgTrackedRepositoryRepository) FindByFullName(ctx context.Context, wsID uuid.UUID, provider, fullName string) (*domain.TrackedRepository, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, scm_provider,
		       external_repo_id, full_name, default_branch, is_private, is_active,
		       webhooks_enabled, last_scanned_at, config_file_path, custom_prompt, settings
		FROM tracked_repositories
		WHERE workspace_id = $1 AND scm_provider = $2 AND full_name = $3
		LIMIT 1
	`
	repo := &domain.TrackedRepository{}
	err := r.pool.QueryRow(ctx, query, wsID, provider, fullName).Scan(
		&repo.ID, &repo.CreatedAt, &repo.UpdatedAt, &repo.WorkspaceID, &repo.OrganizationID,
		&repo.SCMProvider, &repo.ExternalRepoID, &repo.FullName, &repo.DefaultBranch,
		&repo.IsPrivate, &repo.IsActive, &repo.WebhooksEnabled, &repo.LastScannedAt,
		&repo.ConfigFilePath, &repo.CustomPrompt, &repo.Settings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying repository by full name: %w", err)
	}
	return repo, nil
}

// Create inserts a new tracked repository.
func (r *PgTrackedRepositoryRepository) Create(ctx context.Context, repo *domain.TrackedRepository) error {
	query := `
		INSERT INTO tracked_repositories (
			id, created_at, updated_at, workspace_id, organization_id, scm_provider,
			external_repo_id, full_name, default_branch, is_private, is_active,
			webhooks_enabled, last_scanned_at, config_file_path, custom_prompt, settings
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	now := time.Now().UTC()
	if repo.ID == uuid.Nil {
		repo.ID = uuid.New()
	}
	repo.CreatedAt = now
	repo.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		repo.ID, repo.CreatedAt, repo.UpdatedAt, repo.WorkspaceID, repo.OrganizationID,
		repo.SCMProvider, repo.ExternalRepoID, repo.FullName, repo.DefaultBranch,
		repo.IsPrivate, repo.IsActive, repo.WebhooksEnabled, repo.LastScannedAt,
		repo.ConfigFilePath, repo.CustomPrompt, repo.Settings,
	)
	if err != nil {
		return fmt.Errorf("failed creating tracked repository: %w", err)
	}
	return nil
}

// Update persists changes to repository metadata or configuration settings.
func (r *PgTrackedRepositoryRepository) Update(ctx context.Context, repo *domain.TrackedRepository) error {
	query := `
		UPDATE tracked_repositories
		SET updated_at = $3, default_branch = $4, is_private = $5, is_active = $6,
		    webhooks_enabled = $7, last_scanned_at = $8, config_file_path = $9,
		    custom_prompt = $10, settings = $11
		WHERE workspace_id = $1 AND id = $2
	`
	repo.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		repo.WorkspaceID, repo.ID, repo.UpdatedAt, repo.DefaultBranch, repo.IsPrivate,
		repo.IsActive, repo.WebhooksEnabled, repo.LastScannedAt, repo.ConfigFilePath,
		repo.CustomPrompt, repo.Settings,
	)
	if err != nil {
		return fmt.Errorf("failed updating tracked repository: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByOrganization lists all repositories belonging to an organization.
func (r *PgTrackedRepositoryRepository) ListByOrganization(ctx context.Context, wsID, orgID uuid.UUID) ([]*domain.TrackedRepository, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, scm_provider,
		       external_repo_id, full_name, default_branch, is_private, is_active,
		       webhooks_enabled, last_scanned_at, config_file_path, custom_prompt, settings
		FROM tracked_repositories
		WHERE workspace_id = $1 AND organization_id = $2
		ORDER BY full_name ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed querying organization repositories: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.TrackedRepository, 0)
	for rows.Next() {
		repo := &domain.TrackedRepository{}
		if err := rows.Scan(
			&repo.ID, &repo.CreatedAt, &repo.UpdatedAt, &repo.WorkspaceID, &repo.OrganizationID,
			&repo.SCMProvider, &repo.ExternalRepoID, &repo.FullName, &repo.DefaultBranch,
			&repo.IsPrivate, &repo.IsActive, &repo.WebhooksEnabled, &repo.LastScannedAt,
			&repo.ConfigFilePath, &repo.CustomPrompt, &repo.Settings,
		); err != nil {
			return nil, fmt.Errorf("failed scanning repository row: %w", err)
		}
		items = append(items, repo)
	}
	return items, nil
}

// PgPullRequestReviewRepository implements domain.PullRequestReviewRepository.
type PgPullRequestReviewRepository struct {
	pool *pgxpool.Pool
}

// NewPullRequestReviewRepository instantiates a new PgPullRequestReviewRepository.
func NewPullRequestReviewRepository(pool *pgxpool.Pool) *PgPullRequestReviewRepository {
	return &PgPullRequestReviewRepository{pool: pool}
}

// FindByID retrieves a pull request review execution run.
func (r *PgPullRequestReviewRepository) FindByID(ctx context.Context, wsID, reviewID uuid.UUID) (*domain.PullRequestReview, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, repository_id, pull_number,
		       title, head_sha, base_sha, author_username, state, findings_count,
		       duration_ms, prompt_tokens, completion_tokens, model_id,
		       estimated_cost_usd, check_run_id, summary_markdown, failure_reason
		FROM pull_request_reviews
		WHERE workspace_id = $1 AND id = $2
	`
	rev := &domain.PullRequestReview{}
	err := r.pool.QueryRow(ctx, query, wsID, reviewID).Scan(
		&rev.ID, &rev.CreatedAt, &rev.UpdatedAt, &rev.WorkspaceID, &rev.RepositoryID, &rev.PullNumber,
		&rev.Title, &rev.HeadSHA, &rev.BaseSHA, &rev.AuthorUsername, &rev.State, &rev.FindingsCount,
		&rev.DurationMs, &rev.PromptTokens, &rev.CompletionTokens, &rev.ModelID,
		&rev.EstimatedCostUSD, &rev.CheckRunID, &rev.SummaryMarkdown, &rev.FailureReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying pull request review: %w", err)
	}
	return rev, nil
}

// FindLatestByPR returns the most recent review executed for a PR number.
func (r *PgPullRequestReviewRepository) FindLatestByPR(ctx context.Context, wsID, repoID uuid.UUID, pullNumber int) (*domain.PullRequestReview, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, repository_id, pull_number,
		       title, head_sha, base_sha, author_username, state, findings_count,
		       duration_ms, prompt_tokens, completion_tokens, model_id,
		       estimated_cost_usd, check_run_id, summary_markdown, failure_reason
		FROM pull_request_reviews
		WHERE workspace_id = $1 AND repository_id = $2 AND pull_number = $3
		ORDER BY created_at DESC
		LIMIT 1
	`
	rev := &domain.PullRequestReview{}
	err := r.pool.QueryRow(ctx, query, wsID, repoID, pullNumber).Scan(
		&rev.ID, &rev.CreatedAt, &rev.UpdatedAt, &rev.WorkspaceID, &rev.RepositoryID, &rev.PullNumber,
		&rev.Title, &rev.HeadSHA, &rev.BaseSHA, &rev.AuthorUsername, &rev.State, &rev.FindingsCount,
		&rev.DurationMs, &rev.PromptTokens, &rev.CompletionTokens, &rev.ModelID,
		&rev.EstimatedCostUSD, &rev.CheckRunID, &rev.SummaryMarkdown, &rev.FailureReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying latest PR review: %w", err)
	}
	return rev, nil
}

// Create inserts a new review execution record.
func (r *PgPullRequestReviewRepository) Create(ctx context.Context, review *domain.PullRequestReview) error {
	query := `
		INSERT INTO pull_request_reviews (
			id, created_at, updated_at, workspace_id, repository_id, pull_number,
			title, head_sha, base_sha, author_username, state, findings_count,
			duration_ms, prompt_tokens, completion_tokens, model_id,
			estimated_cost_usd, check_run_id, summary_markdown, failure_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`
	now := time.Now().UTC()
	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}
	review.CreatedAt = now
	review.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		review.ID, review.CreatedAt, review.UpdatedAt, review.WorkspaceID, review.RepositoryID, review.PullNumber,
		review.Title, review.HeadSHA, review.BaseSHA, review.AuthorUsername, review.State, review.FindingsCount,
		review.DurationMs, review.PromptTokens, review.CompletionTokens, review.ModelID,
		review.EstimatedCostUSD, review.CheckRunID, review.SummaryMarkdown, review.FailureReason,
	)
	if err != nil {
		return fmt.Errorf("failed creating review record: %w", err)
	}
	return nil
}

// Update persists review progression (token counts, summary markdown, status transition).
func (r *PgPullRequestReviewRepository) Update(ctx context.Context, review *domain.PullRequestReview) error {
	query := `
		UPDATE pull_request_reviews
		SET updated_at = $3, state = $4, findings_count = $5, duration_ms = $6,
		    prompt_tokens = $7, completion_tokens = $8, estimated_cost_usd = $9,
		    check_run_id = $10, summary_markdown = $11, failure_reason = $12
		WHERE workspace_id = $1 AND id = $2
	`
	review.UpdatedAt = time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, query,
		review.WorkspaceID, review.ID, review.UpdatedAt, review.State, review.FindingsCount, review.DurationMs,
		review.PromptTokens, review.CompletionTokens, review.EstimatedCostUSD,
		review.CheckRunID, review.SummaryMarkdown, review.FailureReason,
	)
	if err != nil {
		return fmt.Errorf("failed updating review: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Query performs multi-dimensional search over pull request reviews.
func (r *PgPullRequestReviewRepository) Query(ctx context.Context, filter domain.PullRequestReviewFilter) (*domain.PaginatedResult[*domain.PullRequestReview], error) {
	filter.EnsureDefaults()

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("workspace_id = $%d", argIdx))
	args = append(args, filter.WorkspaceID)
	argIdx++

	if filter.RepositoryID != nil {
		conditions = append(conditions, fmt.Sprintf("repository_id = $%d", argIdx))
		args = append(args, *filter.RepositoryID)
		argIdx++
	}
	if filter.State != nil {
		conditions = append(conditions, fmt.Sprintf("state = $%d", argIdx))
		args = append(args, *filter.State)
		argIdx++
	}
	if filter.AuthorUsername != nil {
		conditions = append(conditions, fmt.Sprintf("author_username = $%d", argIdx))
		args = append(args, *filter.AuthorUsername)
		argIdx++
	}
	if filter.MinFindings != nil {
		conditions = append(conditions, fmt.Sprintf("findings_count >= $%d", argIdx))
		args = append(args, *filter.MinFindings)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	var total int64
	countSQL := fmt.Sprintf("SELECT count(*) FROM pull_request_reviews WHERE %s", whereClause)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed counting reviews: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	allowedReviewSortColumns := map[string]string{
		"created_at":     "created_at",
		"updated_at":     "updated_at",
		"pull_number":    "pull_number",
		"state":          "state",
		"findings_count": "findings_count",
		"duration_ms":    "duration_ms",
	}
	sortCol, ok := allowedReviewSortColumns[strings.ToLower(filter.OrderBy)]
	if !ok {
		sortCol = "created_at"
	}
	sortDir := "DESC"
	if strings.ToUpper(filter.OrderDir) == "ASC" {
		sortDir = "ASC"
	}

	querySQL := fmt.Sprintf(`
		SELECT id, created_at, updated_at, workspace_id, repository_id, pull_number,
		       title, head_sha, base_sha, author_username, state, findings_count,
		       duration_ms, prompt_tokens, completion_tokens, model_id,
		       estimated_cost_usd, check_run_id, summary_markdown, failure_reason
		FROM pull_request_reviews
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortCol, sortDir, argIdx, argIdx+1)

	args = append(args, filter.PageSize, offset)
	rows, err := r.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed querying reviews: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.PullRequestReview, 0, filter.PageSize)
	for rows.Next() {
		rev := &domain.PullRequestReview{}
		if err := rows.Scan(
			&rev.ID, &rev.CreatedAt, &rev.UpdatedAt, &rev.WorkspaceID, &rev.RepositoryID, &rev.PullNumber,
			&rev.Title, &rev.HeadSHA, &rev.BaseSHA, &rev.AuthorUsername, &rev.State, &rev.FindingsCount,
			&rev.DurationMs, &rev.PromptTokens, &rev.CompletionTokens, &rev.ModelID,
			&rev.EstimatedCostUSD, &rev.CheckRunID, &rev.SummaryMarkdown, &rev.FailureReason,
		); err != nil {
			return nil, fmt.Errorf("failed scanning review row: %w", err)
		}
		items = append(items, rev)
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &domain.PaginatedResult[*domain.PullRequestReview]{
		Items:      items,
		TotalCount: total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
		HasNext:    filter.Page < totalPages,
	}, nil
}

// PgCodeFindingRepository implements domain.CodeFindingRepository.
type PgCodeFindingRepository struct {
	pool *pgxpool.Pool
}

// NewCodeFindingRepository instantiates a new PgCodeFindingRepository.
func NewCodeFindingRepository(pool *pgxpool.Pool) *PgCodeFindingRepository {
	return &PgCodeFindingRepository{pool: pool}
}

// FindByID retrieves a code finding by its UUID.
func (r *PgCodeFindingRepository) FindByID(ctx context.Context, wsID, findingID uuid.UUID) (*domain.CodeFinding, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, review_id, repository_id,
		       rule_id, category, severity, file_path, line_start, line_end,
		       message, code_snippet, suggested_fix, diff_hunk, cwe, owasp,
		       confidence_score, is_resolved, comment_id
		FROM code_findings
		WHERE workspace_id = $1 AND id = $2
	`
	f := &domain.CodeFinding{}
	err := r.pool.QueryRow(ctx, query, wsID, findingID).Scan(
		&f.ID, &f.CreatedAt, &f.UpdatedAt, &f.WorkspaceID, &f.ReviewID, &f.RepositoryID,
		&f.RuleID, &f.Category, &f.Severity, &f.FilePath, &f.LineStart, &f.LineEnd,
		&f.Message, &f.CodeSnippet, &f.SuggestedFix, &f.DiffHunk, &f.CWE, &f.OWASP,
		&f.ConfidenceScore, &f.IsResolved, &f.CommentID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying finding: %w", err)
	}
	return f, nil
}

// CreateBatch performs high-speed batch insertion of review findings.
func (r *PgCodeFindingRepository) CreateBatch(ctx context.Context, findings []*domain.CodeFinding) error {
	if len(findings) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}

	query := `
		INSERT INTO code_findings (
			id, created_at, updated_at, workspace_id, review_id, repository_id,
			rule_id, category, severity, file_path, line_start, line_end,
			message, code_snippet, suggested_fix, diff_hunk, cwe, owasp,
			confidence_score, is_resolved, comment_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`
	for _, f := range findings {
		if f.ID == uuid.Nil {
			f.ID = uuid.New()
		}
		f.CreatedAt = now
		f.UpdatedAt = now

		batch.Queue(query,
			f.ID, f.CreatedAt, f.UpdatedAt, f.WorkspaceID, f.ReviewID, f.RepositoryID,
			f.RuleID, f.Category, f.Severity, f.FilePath, f.LineStart, f.LineEnd,
			f.Message, f.CodeSnippet, f.SuggestedFix, f.DiffHunk, f.CWE, f.OWASP,
			f.ConfidenceScore, f.IsResolved, f.CommentID,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range findings {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert finding failed: %w", err)
		}
	}
	return nil
}

// Query retrieves code findings with filtering on category, severity, OWASP, and resolution status.
func (r *PgCodeFindingRepository) Query(ctx context.Context, filter domain.CodeFindingFilter) (*domain.PaginatedResult[*domain.CodeFinding], error) {
	filter.EnsureDefaults()

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("workspace_id = $%d", argIdx))
	args = append(args, filter.WorkspaceID)
	argIdx++

	if filter.ReviewID != nil {
		conditions = append(conditions, fmt.Sprintf("review_id = $%d", argIdx))
		args = append(args, *filter.ReviewID)
		argIdx++
	}
	if filter.RepositoryID != nil {
		conditions = append(conditions, fmt.Sprintf("repository_id = $%d", argIdx))
		args = append(args, *filter.RepositoryID)
		argIdx++
	}
	if filter.Severity != nil {
		conditions = append(conditions, fmt.Sprintf("severity = $%d", argIdx))
		args = append(args, *filter.Severity)
		argIdx++
	}
	if filter.Category != nil {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *filter.Category)
		argIdx++
	}
	if filter.IsResolved != nil {
		conditions = append(conditions, fmt.Sprintf("is_resolved = $%d", argIdx))
		args = append(args, *filter.IsResolved)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	var total int64
	countSQL := fmt.Sprintf("SELECT count(*) FROM code_findings WHERE %s", whereClause)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed counting findings: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	allowedFindingSortColumns := map[string]string{
		"created_at":       "created_at",
		"updated_at":       "updated_at",
		"severity":         "severity",
		"category":         "category",
		"file_path":        "file_path",
		"confidence_score": "confidence_score",
	}
	sortCol, ok := allowedFindingSortColumns[strings.ToLower(filter.OrderBy)]
	if !ok {
		sortCol = "created_at"
	}
	sortDir := "DESC"
	if strings.ToUpper(filter.OrderDir) == "ASC" {
		sortDir = "ASC"
	}

	querySQL := fmt.Sprintf(`
		SELECT id, created_at, updated_at, workspace_id, review_id, repository_id,
		       rule_id, category, severity, file_path, line_start, line_end,
		       message, code_snippet, suggested_fix, diff_hunk, cwe, owasp,
		       confidence_score, is_resolved, comment_id
		FROM code_findings
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortCol, sortDir, argIdx, argIdx+1)

	args = append(args, filter.PageSize, offset)
	rows, err := r.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed querying findings: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.CodeFinding, 0, filter.PageSize)
	for rows.Next() {
		f := &domain.CodeFinding{}
		if err := rows.Scan(
			&f.ID, &f.CreatedAt, &f.UpdatedAt, &f.WorkspaceID, &f.ReviewID, &f.RepositoryID,
			&f.RuleID, &f.Category, &f.Severity, &f.FilePath, &f.LineStart, &f.LineEnd,
			&f.Message, &f.CodeSnippet, &f.SuggestedFix, &f.DiffHunk, &f.CWE, &f.OWASP,
			&f.ConfidenceScore, &f.IsResolved, &f.CommentID,
		); err != nil {
			return nil, fmt.Errorf("failed scanning finding row: %w", err)
		}
		items = append(items, f)
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &domain.PaginatedResult[*domain.CodeFinding]{
		Items:      items,
		TotalCount: total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
		HasNext:    filter.Page < totalPages,
	}, nil
}

// MarkResolved flags a code finding as resolved.
func (r *PgCodeFindingRepository) MarkResolved(ctx context.Context, wsID, findingID uuid.UUID) error {
	query := `
		UPDATE code_findings
		SET is_resolved = true, updated_at = $3
		WHERE workspace_id = $1 AND id = $2
	`
	cmd, err := r.pool.Exec(ctx, query, wsID, findingID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed marking finding resolved: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PgFindingFeedbackRepository implements domain.FindingFeedbackRepository.
type PgFindingFeedbackRepository struct {
	pool *pgxpool.Pool
}

// NewFindingFeedbackRepository instantiates a new PgFindingFeedbackRepository.
func NewFindingFeedbackRepository(pool *pgxpool.Pool) *PgFindingFeedbackRepository {
	return &PgFindingFeedbackRepository{pool: pool}
}

// Create records engineer feedback on an automated finding.
func (r *PgFindingFeedbackRepository) Create(ctx context.Context, fb *domain.FindingFeedback) error {
	query := `
		INSERT INTO finding_feedbacks (
			id, created_at, updated_at, workspace_id, finding_id, user_id, reaction, comment, is_actioned
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now().UTC()
	if fb.ID == uuid.Nil {
		fb.ID = uuid.New()
	}
	fb.CreatedAt = now
	fb.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		fb.ID, fb.CreatedAt, fb.UpdatedAt, fb.WorkspaceID, fb.FindingID, fb.UserID,
		fb.Reaction, fb.Comment, fb.IsActioned,
	)
	if err != nil {
		return fmt.Errorf("failed inserting feedback: %w", err)
	}
	return nil
}

// ListByFinding retrieves all feedback recorded for a finding.
func (r *PgFindingFeedbackRepository) ListByFinding(ctx context.Context, wsID, findingID uuid.UUID) ([]*domain.FindingFeedback, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, finding_id, user_id, reaction, comment, is_actioned
		FROM finding_feedbacks
		WHERE workspace_id = $1 AND finding_id = $2
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID, findingID)
	if err != nil {
		return nil, fmt.Errorf("failed querying finding feedback: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.FindingFeedback, 0)
	for rows.Next() {
		fb := &domain.FindingFeedback{}
		if err := rows.Scan(
			&fb.ID, &fb.CreatedAt, &fb.UpdatedAt, &fb.WorkspaceID, &fb.FindingID, &fb.UserID,
			&fb.Reaction, &fb.Comment, &fb.IsActioned,
		); err != nil {
			return nil, fmt.Errorf("failed scanning feedback row: %w", err)
		}
		items = append(items, fb)
	}
	return items, nil
}
