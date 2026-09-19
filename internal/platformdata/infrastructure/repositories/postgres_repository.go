// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: repositories
// File: postgres_repository.go
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

// PostgresPullRequestsRepository implements contracts.IPullRequestsRepository on PostgreSQL.
type PostgresPullRequestsRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresPullRequestsRepository initializes a PostgreSQL PR repository.
func NewPostgresPullRequestsRepository(pool *pgxpool.Pool) *PostgresPullRequestsRepository {
	return &PostgresPullRequestsRepository{pool: pool}
}

// Create persists a new pull request aggregate root with its sub-documents.
func (r *PostgresPullRequestsRepository) Create(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}
	if pr == nil {
		return nil, errors.New("pull request cannot be nil")
	}

	if pr.UUID == "" {
		pr.UUID = uuid.NewString()
	}
	now := time.Now().UTC()
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = now
	}
	pr.UpdatedAt = now

	userDataBytes, _ := json.Marshal(pr.User)
	repoDataBytes, _ := json.Marshal(pr.Repository)
	filesBytes, _ := json.Marshal(pr.Files)
	commitsBytes, _ := json.Marshal(pr.Commits)
	suggestionsByPRBytes, _ := json.Marshal(pr.SuggestionsByPR)
	prLevelSuggestionsBytes, _ := json.Marshal(pr.PRLevelSuggestions)

	wsUUID, err := uuid.Parse(pr.OrganizationID)
	if err != nil {
		wsUUID = uuid.Nil
	}

	query := `
		INSERT INTO platform_pull_requests (
			id, workspace_id, repository_id, number, title, status, merged, heavy, is_draft,
			provider, url, base_branch_ref, head_branch_ref, user_data, repository_data,
			files, commits, suggestions_by_pr, pr_level_suggestions, total_added, total_deleted,
			total_changes, synced_embedded_suggestions, synced_with_issues, opened_at, closed_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14::jsonb, $15::jsonb,
			$16::jsonb, $17::jsonb, $18::jsonb, $19::jsonb, $20, $21,
			$22, $23, $24, $25, $26,
			$27, $28
		)
		ON CONFLICT (workspace_id, repository_id, number) DO UPDATE
		SET title = EXCLUDED.title,
		    status = EXCLUDED.status,
		    merged = EXCLUDED.merged,
		    heavy = EXCLUDED.heavy,
		    is_draft = EXCLUDED.is_draft,
		    url = EXCLUDED.url,
		    base_branch_ref = EXCLUDED.base_branch_ref,
		    head_branch_ref = EXCLUDED.head_branch_ref,
		    user_data = EXCLUDED.user_data,
		    repository_data = EXCLUDED.repository_data,
		    files = EXCLUDED.files,
		    commits = EXCLUDED.commits,
		    suggestions_by_pr = EXCLUDED.suggestions_by_pr,
		    pr_level_suggestions = EXCLUDED.pr_level_suggestions,
		    total_added = EXCLUDED.total_added,
		    total_deleted = EXCLUDED.total_deleted,
		    total_changes = EXCLUDED.total_changes,
		    synced_embedded_suggestions = EXCLUDED.synced_embedded_suggestions,
		    synced_with_issues = EXCLUDED.synced_with_issues,
		    updated_at = now()
		RETURNING id, created_at, updated_at;
	`

	var returnedID uuid.UUID
	var returnedCreated, returnedUpdated time.Time

	var openedAt *time.Time
	if pr.OpenedAt != "" {
		if t, err := time.Parse(time.RFC3339, pr.OpenedAt); err == nil {
			openedAt = &t
		}
	}
	var closedAt *time.Time
	if pr.ClosedAt != "" {
		if t, err := time.Parse(time.RFC3339, pr.ClosedAt); err == nil {
			closedAt = &t
		}
	}

	err = r.pool.QueryRow(ctx, query,
		pr.UUID, wsUUID, pr.Repository.ID, pr.Number, pr.Title, pr.Status, pr.Merged, pr.Heavy, pr.IsDraft,
		pr.Provider, pr.URL, pr.BaseBranchRef, pr.HeadBranchRef, string(userDataBytes), string(repoDataBytes),
		string(filesBytes), string(commitsBytes), string(suggestionsByPRBytes), string(prLevelSuggestionsBytes),
		pr.TotalAdded, pr.TotalDeleted, pr.TotalChanges, pr.SyncedEmbeddedSuggestions, pr.SyncedWithIssues,
		openedAt, closedAt, pr.CreatedAt, pr.UpdatedAt,
	).Scan(&returnedID, &returnedCreated, &returnedUpdated)

	if err != nil {
		return nil, fmt.Errorf("failed inserting platform pull request: %w", err)
	}

	pr.UUID = returnedID.String()
	pr.CreatedAt = returnedCreated
	pr.UpdatedAt = returnedUpdated
	return pr, nil
}

// FindByID retrieves a pull request by its internal UUID.
func (r *PostgresPullRequestsRepository) FindByID(ctx context.Context, prUUID string) (*models.PullRequest, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT id, workspace_id, repository_id, number, title, status, merged, heavy, is_draft,
		       provider, url, base_branch_ref, head_branch_ref, user_data, repository_data,
		       files, commits, suggestions_by_pr, pr_level_suggestions, total_added, total_deleted,
		       total_changes, synced_embedded_suggestions, synced_with_issues, opened_at, closed_at,
		       created_at, updated_at
		FROM platform_pull_requests
		WHERE id = $1;
	`
	row := r.pool.QueryRow(ctx, query, prUUID)
	return r.scanPullRequest(row)
}

// FindOne finds a pull request by organization, repository id, and PR number.
func (r *PostgresPullRequestsRepository) FindOne(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error) {
	return r.FindByNumberAndRepositoryID(ctx, orgID, repoID, number)
}

// FindByNumberAndRepositoryID point-lookups a PR using its natural unique key.
func (r *PostgresPullRequestsRepository) FindByNumberAndRepositoryID(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT id, workspace_id, repository_id, number, title, status, merged, heavy, is_draft,
		       provider, url, base_branch_ref, head_branch_ref, user_data, repository_data,
		       files, commits, suggestions_by_pr, pr_level_suggestions, total_added, total_deleted,
		       total_changes, synced_embedded_suggestions, synced_with_issues, opened_at, closed_at,
		       created_at, updated_at
		FROM platform_pull_requests
		WHERE repository_id = $1 AND number = $2
		  AND (workspace_id = NULLIF($3, '')::uuid OR NULLIF($3, '') IS NULL);
	`
	row := r.pool.QueryRow(ctx, query, repoID, number, orgID)
	return r.scanPullRequest(row)
}

// Find retrieves a paginated slice of PRs for a repository.
func (r *PostgresPullRequestsRepository) Find(ctx context.Context, orgID string, repoID string, limit int, offset int) ([]*models.PullRequest, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, workspace_id, repository_id, number, title, status, merged, heavy, is_draft,
		       provider, url, base_branch_ref, head_branch_ref, user_data, repository_data,
		       files, commits, suggestions_by_pr, pr_level_suggestions, total_added, total_deleted,
		       total_changes, synced_embedded_suggestions, synced_with_issues, opened_at, closed_at,
		       created_at, updated_at
		FROM platform_pull_requests
		WHERE repository_id = $1
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL)
		ORDER BY number DESC
		LIMIT $3 OFFSET $4;
	`
	rows, err := r.pool.Query(ctx, query, repoID, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.PullRequest
	for rows.Next() {
		pr, err := r.scanPullRequest(rows)
		if err != nil {
			continue
		}
		result = append(result, pr)
	}
	return result, nil
}

// FindPRNumbersByTitleAndOrganization searches PR numbers matching title regex.
func (r *PostgresPullRequestsRepository) FindPRNumbersByTitleAndOrganization(
	ctx context.Context,
	title string,
	orgID string,
	repoIDs []string,
) ([]struct {
	Number       int
	RepositoryID string
}, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT number, repository_id
		FROM platform_pull_requests
		WHERE title ILIKE $1
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL)
	`
	args := []any{"%" + title + "%", orgID}
	if len(repoIDs) > 0 {
		query += ` AND repository_id = ANY($3)`
		args = append(args, repoIDs)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		Number       int
		RepositoryID string
	}
	for rows.Next() {
		var item struct {
			Number       int
			RepositoryID string
		}
		if err := rows.Scan(&item.Number, &item.RepositoryID); err == nil {
			results = append(results, item)
		}
	}
	return results, nil
}

// FindManyByNumbersAndRepositoryIDs retrieves full PR documents for a batch of (number, repoID) pairs.
func (r *PostgresPullRequestsRepository) FindManyByNumbersAndRepositoryIDs(
	ctx context.Context,
	criteria []struct {
		Number       int
		RepositoryID string
	},
	orgID string,
) ([]*models.PullRequest, error) {
	if r.pool == nil || len(criteria) == 0 {
		return []*models.PullRequest{}, nil
	}

	var clauses []string
	var args []any
	args = append(args, orgID)
	argIdx := 2

	for _, c := range criteria {
		clauses = append(clauses, fmt.Sprintf("(repository_id = $%d AND number = $%d)", argIdx, argIdx+1))
		args = append(args, c.RepositoryID, c.Number)
		argIdx += 2
	}

	query := fmt.Sprintf(`
		SELECT id, workspace_id, repository_id, number, title, status, merged, heavy, is_draft,
		       provider, url, base_branch_ref, head_branch_ref, user_data, repository_data,
		       files, commits, suggestions_by_pr, pr_level_suggestions, total_added, total_deleted,
		       total_changes, synced_embedded_suggestions, synced_with_issues, opened_at, closed_at,
		       created_at, updated_at
		FROM platform_pull_requests
		WHERE (workspace_id = NULLIF($1, '')::uuid OR NULLIF($1, '') IS NULL)
		  AND (%s);
	`, strings.Join(clauses, " OR "))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.PullRequest
	for rows.Next() {
		pr, err := r.scanPullRequest(rows)
		if err != nil {
			continue
		}
		result = append(result, pr)
	}
	return result, nil
}

// FindManyByNumbers performs a minimal projection for developer token attribution queries.
func (r *PostgresPullRequestsRepository) FindManyByNumbers(
	ctx context.Context,
	prNumbers []int,
	orgID string,
) ([]models.PullRequestUserMapping, error) {
	if r.pool == nil || len(prNumbers) == 0 {
		return []models.PullRequestUserMapping{}, nil
	}

	query := `
		SELECT number, user_data, workspace_id
		FROM platform_pull_requests
		WHERE number = ANY($1)
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL);
	`
	rows, err := r.pool.Query(ctx, query, prNumbers, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.PullRequestUserMapping
	for rows.Next() {
		var number int
		var userRaw []byte
		var wsID uuid.UUID
		if err := rows.Scan(&number, &userRaw, &wsID); err != nil {
			continue
		}
		var user models.PullRequestUser
		_ = json.Unmarshal(userRaw, &user)
		results = append(results, models.PullRequestUserMapping{
			Number:         number,
			User:           user,
			OrganizationID: wsID.String(),
		})
	}
	return results, nil
}

// FindNumbersByRepositoryID returns all recorded PR numbers for a repository.
func (r *PostgresPullRequestsRepository) FindNumbersByRepositoryID(
	ctx context.Context,
	orgID string,
	repoID string,
	until *time.Time,
) ([]int, error) {
	if r.pool == nil {
		return []int{}, nil
	}

	query := `
		SELECT number
		FROM platform_pull_requests
		WHERE repository_id = $1
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL)
	`
	args := []any{repoID, orgID}
	if until != nil {
		query += ` AND created_at <= $3`
		args = append(args, *until)
	}
	query += ` ORDER BY number ASC;`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var numbers []int
	for rows.Next() {
		var num int
		if err := rows.Scan(&num); err == nil {
			numbers = append(numbers, num)
		}
	}
	return numbers, nil
}

// FindSuggestionCountsByNumbersAndRepositoryIds computes counts by severity across PR documents.
func (r *PostgresPullRequestsRepository) FindSuggestionCountsByNumbersAndRepositoryIds(
	ctx context.Context,
	criteria []struct {
		Number       int
		RepositoryID string
	},
	orgID string,
) (map[string]models.SuggestionCountsBySeverity, error) {
	prs, err := r.FindManyByNumbersAndRepositoryIDs(ctx, criteria, orgID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]models.SuggestionCountsBySeverity)
	for _, pr := range prs {
		key := fmt.Sprintf("%s_%d", pr.Repository.ID, pr.Number)
		counts := models.SuggestionCountsBySeverity{
			Categories: []string{},
		}
		catSeen := make(map[string]struct{})

		for _, file := range pr.Files {
			for _, s := range file.Suggestions {
				sev := strings.ToLower(s.Severity)
				switch s.DeliveryStatus {
				case enums.DeliveryStatusSent:
					counts.Sent++
					switch sev {
					case "critical":
						counts.BySeverity.Critical++
					case "high":
						counts.BySeverity.High++
					case "medium":
						counts.BySeverity.Medium++
					case "low":
						counts.BySeverity.Low++
					}

					if s.ImplementationStatus != enums.ImplementationStatusImplemented {
						counts.Unresolved++
						switch sev {
						case "critical":
							counts.UnresolvedBySeverity.Critical++
						case "high":
							counts.UnresolvedBySeverity.High++
						case "medium":
							counts.UnresolvedBySeverity.Medium++
						case "low":
							counts.UnresolvedBySeverity.Low++
						}
					}

					if s.Label != "" {
						cat := strings.ToLower(s.Label)
						if _, ok := catSeen[cat]; !ok {
							catSeen[cat] = struct{}{}
							counts.Categories = append(counts.Categories, cat)
						}
					}
				case enums.DeliveryStatusNotSent:
					counts.Filtered++
				case enums.DeliveryStatusFailed, enums.DeliveryStatusFailedLinesMismatch:
					counts.Failed++
				case enums.DeliveryStatusReplaced:
					counts.Replaced++
				}
			}
		}
		result[key] = counts
	}
	return result, nil
}

// FindOpenPullRequestKeysOpenedSince returns keys of open PRs created since timestamp.
func (r *PostgresPullRequestsRepository) FindOpenPullRequestKeysOpenedSince(
	ctx context.Context,
	since time.Time,
	orgID string,
	repoIDs []string,
) ([]struct {
	Number       int
	RepositoryID string
}, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT number, repository_id
		FROM platform_pull_requests
		WHERE status = 'OPEN'
		  AND created_at >= $1
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL)
	`
	args := []any{since, orgID}
	if len(repoIDs) > 0 {
		query += ` AND repository_id = ANY($3)`
		args = append(args, repoIDs)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		Number       int
		RepositoryID string
	}
	for rows.Next() {
		var item struct {
			Number       int
			RepositoryID string
		}
		if err := rows.Scan(&item.Number, &item.RepositoryID); err == nil {
			results = append(results, item)
		}
	}
	return results, nil
}

// FindDistinctAuthorsByRepositoryIds aggregates author suggestions for autocomplete.
func (r *PostgresPullRequestsRepository) FindDistinctAuthorsByRepositoryIds(
	ctx context.Context,
	orgID string,
	repoIDs []string,
	search string,
	limit int,
) ([]models.PullRequestAuthorSuggestion, error) {
	if r.pool == nil {
		return []models.PullRequestAuthorSuggestion{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT user_data->>'id' AS id,
		       COALESCE(user_data->>'name', user_data->>'username', 'unknown') AS name,
		       COALESCE(user_data->>'username', '') AS username,
		       COUNT(*)::int AS count
		FROM platform_pull_requests
		WHERE (workspace_id = NULLIF($1, '')::uuid OR NULLIF($1, '') IS NULL)
	`
	args := []any{orgID}
	argIdx := 2
	if len(repoIDs) > 0 {
		query += fmt.Sprintf(` AND repository_id = ANY($%d)`, argIdx)
		args = append(args, repoIDs)
		argIdx++
	}
	if search != "" {
		query += fmt.Sprintf(` AND (user_data->>'name' ILIKE $%d OR user_data->>'username' ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	query += `
		GROUP BY user_data->>'id', name, username
		ORDER BY count DESC
		LIMIT ` + fmt.Sprintf("$%d", argIdx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []models.PullRequestAuthorSuggestion
	for rows.Next() {
		var s models.PullRequestAuthorSuggestion
		if err := rows.Scan(&s.ID, &s.Name, &s.Username, &s.Count); err == nil {
			suggestions = append(suggestions, s)
		}
	}
	return suggestions, nil
}

// CountDeliveredPullRequests tallies PRs satisfying delivered filter criteria.
func (r *PostgresPullRequestsRepository) CountDeliveredPullRequests(
	ctx context.Context,
	orgID string,
	repoIDs []string,
	opts contracts.DeliveredFilterOpts,
) (int, error) {
	if r.pool == nil {
		return 0, nil
	}

	query := `
		SELECT COUNT(*)::int
		FROM platform_pull_requests
		WHERE (workspace_id = NULLIF($1, '')::uuid OR NULLIF($1, '') IS NULL)
	`
	args := []any{orgID}
	argIdx := 2

	if len(repoIDs) > 0 {
		query += fmt.Sprintf(` AND repository_id = ANY($%d)`, argIdx)
		args = append(args, repoIDs)
		argIdx++
	}
	if opts.OpenOnly {
		query += ` AND status = 'OPEN'`
	}
	if opts.AuthorEmail != "" {
		query += fmt.Sprintf(` AND user_data->>'email' ILIKE $%d`, argIdx)
		args = append(args, opts.AuthorEmail)
		argIdx++
	}

	var count int
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}

// FindFileWithSuggestions retrieves a specific file and its suggestions from a PR document.
func (r *PostgresPullRequestsRepository) FindFileWithSuggestions(
	ctx context.Context,
	orgID string,
	repoID string,
	prNumber int,
	filePath string,
) (*models.File, error) {
	pr, err := r.FindByNumberAndRepositoryID(ctx, orgID, repoID, prNumber)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, nil
	}
	for _, f := range pr.Files {
		if f.Path == filePath || f.Filename == filePath {
			return &f, nil
		}
	}
	return nil, nil
}

// FindSuggestionsByPR retrieves all suggestions for a PR matching a delivery status.
func (r *PostgresPullRequestsRepository) FindSuggestionsByPR(
	ctx context.Context,
	orgID string,
	repoID string,
	prNumber int,
	deliveryStatus enums.DeliveryStatus,
) ([]models.Suggestion, error) {
	pr, err := r.FindByNumberAndRepositoryID(ctx, orgID, repoID, prNumber)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return []models.Suggestion{}, nil
	}
	var results []models.Suggestion
	for _, f := range pr.Files {
		for _, s := range f.Suggestions {
			if deliveryStatus == "" || s.DeliveryStatus == deliveryStatus {
				results = append(results, s)
			}
		}
	}
	return results, nil
}

// BulkApplyFileChanges collapses N+1 file/suggestion operations into an atomic document update.
func (r *PostgresPullRequestsRepository) BulkApplyFileChanges(
	ctx context.Context,
	prUUID string,
	orgID string,
	ops []models.FileBulkOp,
) (*models.BulkApplyResult, error) {
	pr, err := r.FindByID(ctx, prUUID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, errors.New("pull request not found")
	}

	fileMap := make(map[string]int)
	for i, f := range pr.Files {
		fileMap[f.ID] = i
	}

	result := &models.BulkApplyResult{
		Attempted: len(ops),
	}

	for i, op := range ops {
		switch op.Kind {
		case "addFile":
			if op.File != nil {
				pr.Files = append(pr.Files, *op.File)
				fileMap[op.File.ID] = len(pr.Files) - 1
				result.Modified++
			}
		case "updateFile":
			if idx, ok := fileMap[op.FileID]; ok {
				if path, ok := op.FileUpdates["path"].(string); ok && path != "" {
					pr.Files[idx].Path = path
				}
				if fn, ok := op.FileUpdates["filename"].(string); ok && fn != "" {
					pr.Files[idx].Filename = fn
				}
				if st, ok := op.FileUpdates["status"].(string); ok && st != "" {
					pr.Files[idx].Status = st
				}
				result.Modified++
			} else {
				result.Errors = append(result.Errors, models.BulkApplyError{
					OpIndex: i,
					Message: fmt.Sprintf("file id %s not found", op.FileID),
				})
			}
		case "addSuggestions":
			if idx, ok := fileMap[op.FileID]; ok {
				pr.Files[idx].Suggestions = append(pr.Files[idx].Suggestions, op.Suggestions...)
				result.Modified++
			} else {
				result.Errors = append(result.Errors, models.BulkApplyError{
					OpIndex: i,
					Message: fmt.Sprintf("file id %s not found for suggestions", op.FileID),
				})
			}
		}
	}

	_, err = r.Update(ctx, pr)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ComputeFileTotals recalculates total added, deleted, and changes across all files in the PR.
func (r *PostgresPullRequestsRepository) ComputeFileTotals(
	ctx context.Context,
	prUUID string,
	orgID string,
) (added int, deleted int, changes int, err error) {
	pr, err := r.FindByID(ctx, prUUID)
	if err != nil || pr == nil {
		return 0, 0, 0, err
	}
	for _, f := range pr.Files {
		added += f.Added
		deleted += f.Deleted
		changes += f.Changes
	}
	return added, deleted, changes, nil
}

// Update saves modifications to an existing pull request document.
func (r *PostgresPullRequestsRepository) Update(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error) {
	if r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}
	if pr == nil || pr.UUID == "" {
		return nil, errors.New("invalid pull request or missing uuid")
	}

	filesBytes, _ := json.Marshal(pr.Files)
	commitsBytes, _ := json.Marshal(pr.Commits)
	userDataBytes, _ := json.Marshal(pr.User)
	repoDataBytes, _ := json.Marshal(pr.Repository)
	suggestionsByPRBytes, _ := json.Marshal(pr.SuggestionsByPR)
	prLevelSuggestionsBytes, _ := json.Marshal(pr.PRLevelSuggestions)

	query := `
		UPDATE platform_pull_requests
		SET title = $1,
		    status = $2,
		    merged = $3,
		    heavy = $4,
		    is_draft = $5,
		    url = $6,
		    base_branch_ref = $7,
		    head_branch_ref = $8,
		    user_data = $9::jsonb,
		    repository_data = $10::jsonb,
		    files = $11::jsonb,
		    commits = $12::jsonb,
		    suggestions_by_pr = $13::jsonb,
		    pr_level_suggestions = $14::jsonb,
		    total_added = $15,
		    total_deleted = $16,
		    total_changes = $17,
		    synced_embedded_suggestions = $18,
		    synced_with_issues = $19,
		    updated_at = now()
		WHERE id = $20
		RETURNING updated_at;
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, query,
		pr.Title, pr.Status, pr.Merged, pr.Heavy, pr.IsDraft,
		pr.URL, pr.BaseBranchRef, pr.HeadBranchRef, string(userDataBytes), string(repoDataBytes),
		string(filesBytes), string(commitsBytes), string(suggestionsByPRBytes), string(prLevelSuggestionsBytes),
		pr.TotalAdded, pr.TotalDeleted, pr.TotalChanges, pr.SyncedEmbeddedSuggestions, pr.SyncedWithIssues,
		pr.UUID,
	).Scan(&updatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed updating platform pull request: %w", err)
	}
	pr.UpdatedAt = updatedAt
	return pr, nil
}

// UpdateSuggestion updates primitive status fields on a single suggestion inside any file.
func (r *PostgresPullRequestsRepository) UpdateSuggestion(
	ctx context.Context,
	orgID string,
	suggestionID string,
	updateData map[string]interface{},
) error {
	if r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	// Find the PR containing the suggestion using GIN query
	queryFind := `
		SELECT id
		FROM platform_pull_requests
		WHERE files @> $1::jsonb
		  AND (workspace_id = NULLIF($2, '')::uuid OR NULLIF($2, '') IS NULL)
		LIMIT 1;
	`
	probeBytes, _ := json.Marshal([]map[string]any{
		{"suggestions": []map[string]any{{"id": suggestionID}}},
	})

	var prUUID string
	err := r.pool.QueryRow(ctx, queryFind, string(probeBytes), orgID).Scan(&prUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("suggestion not found in any pull request")
		}
		return err
	}

	pr, err := r.FindByID(ctx, prUUID)
	if err != nil || pr == nil {
		return err
	}

	found := false
	for fi, f := range pr.Files {
		for si, s := range f.Suggestions {
			if s.ID == suggestionID {
				if ds, ok := updateData["deliveryStatus"].(string); ok {
					pr.Files[fi].Suggestions[si].DeliveryStatus = enums.DeliveryStatus(ds)
				}
				if is, ok := updateData["implementationStatus"].(string); ok {
					pr.Files[fi].Suggestions[si].ImplementationStatus = enums.ImplementationStatus(is)
				}
				if commentID, ok := updateData["commentId"].(int64); ok {
					if pr.Files[fi].Suggestions[si].Comment == nil {
						pr.Files[fi].Suggestions[si].Comment = &models.CommentReference{}
					}
					pr.Files[fi].Suggestions[si].Comment.ID = commentID
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return errors.New("suggestion id not located in matched document")
	}

	_, err = r.Update(ctx, pr)
	return err
}

// UpdateSyncedSuggestionsFlag updates embedded suggestions sync flag.
func (r *PostgresPullRequestsRepository) UpdateSyncedSuggestionsFlag(
	ctx context.Context,
	prNumbers []int,
	repoID string,
	orgID string,
	synced bool,
) error {
	if r.pool == nil || len(prNumbers) == 0 {
		return nil
	}
	query := `
		UPDATE platform_pull_requests
		SET synced_embedded_suggestions = $1, updated_at = now()
		WHERE repository_id = $2 AND number = ANY($3)
		  AND (workspace_id = NULLIF($4, '')::uuid OR NULLIF($4, '') IS NULL);
	`
	_, err := r.pool.Exec(ctx, query, synced, repoID, prNumbers, orgID)
	return err
}

// UpdateSyncedWithIssuesFlag updates issues sync flag.
func (r *PostgresPullRequestsRepository) UpdateSyncedWithIssuesFlag(
	ctx context.Context,
	prNumber int,
	repoID string,
	orgID string,
	synced bool,
) error {
	if r.pool == nil {
		return nil
	}
	query := `
		UPDATE platform_pull_requests
		SET synced_with_issues = $1, updated_at = now()
		WHERE repository_id = $2 AND number = $3
		  AND (workspace_id = NULLIF($4, '')::uuid OR NULLIF($4, '') IS NULL);
	`
	_, err := r.pool.Exec(ctx, query, synced, repoID, prNumber, orgID)
	return err
}

func (r *PostgresPullRequestsRepository) scanPullRequest(row interface{ Scan(dest ...any) error }) (*models.PullRequest, error) {
	var pr models.PullRequest
	var id, wsID uuid.UUID
	var userDataBytes, repoDataBytes, filesBytes, commitsBytes, suggestionsByPRBytes, prLevelBytes []byte
	var openedAt, closedAt *time.Time

	err := row.Scan(
		&id, &wsID, &pr.Repository.ID, &pr.Number, &pr.Title, &pr.Status, &pr.Merged, &pr.Heavy, &pr.IsDraft,
		&pr.Provider, &pr.URL, &pr.BaseBranchRef, &pr.HeadBranchRef, &userDataBytes, &repoDataBytes,
		&filesBytes, &commitsBytes, &suggestionsByPRBytes, &prLevelBytes, &pr.TotalAdded, &pr.TotalDeleted,
		&pr.TotalChanges, &pr.SyncedEmbeddedSuggestions, &pr.SyncedWithIssues, &openedAt, &closedAt,
		&pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	pr.UUID = id.String()
	pr.OrganizationID = wsID.String()

	_ = json.Unmarshal(userDataBytes, &pr.User)
	_ = json.Unmarshal(repoDataBytes, &pr.Repository)
	_ = json.Unmarshal(filesBytes, &pr.Files)
	_ = json.Unmarshal(commitsBytes, &pr.Commits)
	_ = json.Unmarshal(suggestionsByPRBytes, &pr.SuggestionsByPR)
	_ = json.Unmarshal(prLevelBytes, &pr.PRLevelSuggestions)

	if openedAt != nil {
		pr.OpenedAt = openedAt.UTC().Format(time.RFC3339)
	}
	if closedAt != nil {
		pr.ClosedAt = closedAt.UTC().Format(time.RFC3339)
	}
	return &pr, nil
}
