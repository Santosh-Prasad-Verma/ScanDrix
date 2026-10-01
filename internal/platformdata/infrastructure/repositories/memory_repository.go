// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: repositories
// File: memory_repository.go
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

// MemoryPullRequestsRepository provides an in-memory thread-safe implementation
// of contracts.IPullRequestsRepository for unit tests and local execution.
type MemoryPullRequestsRepository struct {
	mu  sync.RWMutex
	prs map[string]*models.PullRequest // keyed by UUID
}

// NewMemoryPullRequestsRepository creates a new in-memory PR repository.
func NewMemoryPullRequestsRepository() *MemoryPullRequestsRepository {
	return &MemoryPullRequestsRepository{
		prs: make(map[string]*models.PullRequest),
	}
}

func clonePR(pr *models.PullRequest) *models.PullRequest {
	if pr == nil {
		return nil
	}
	cp := *pr
	// Deep copy files
	cp.Files = make([]models.File, len(pr.Files))
	for i, f := range pr.Files {
		cf := f
		cf.Suggestions = make([]models.Suggestion, len(f.Suggestions))
		copy(cf.Suggestions, f.Suggestions)
		cp.Files[i] = cf
	}
	// Deep copy commits
	cp.Commits = make([]models.Commit, len(pr.Commits))
	copy(cp.Commits, pr.Commits)
	// Deep copy PR level suggestions
	cp.PRLevelSuggestions = make([]models.SuggestionByPR, len(pr.PRLevelSuggestions))
	copy(cp.PRLevelSuggestions, pr.PRLevelSuggestions)
	return &cp
}

// Create stores or upserts a pull request.
func (r *MemoryPullRequestsRepository) Create(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error) {
	if pr == nil {
		return nil, errors.New("pull request cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if pr.UUID == "" {
		pr.UUID = uuid.NewString()
	}
	now := time.Now().UTC()
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = now
	}
	pr.UpdatedAt = now

	// Check if matching orgID, repoID, number already exists
	for _, existing := range r.prs {
		if existing.OrganizationID == pr.OrganizationID &&
			existing.Repository.ID == pr.Repository.ID &&
			existing.Number == pr.Number {
			pr.UUID = existing.UUID
			pr.CreatedAt = existing.CreatedAt
			break
		}
	}

	cloned := clonePR(pr)
	r.prs[pr.UUID] = cloned
	return clonePR(cloned), nil
}

// FindByID retrieves a pull request by its UUID.
func (r *MemoryPullRequestsRepository) FindByID(ctx context.Context, id string) (*models.PullRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pr, ok := r.prs[id]
	if !ok {
		return nil, nil
	}
	return clonePR(pr), nil
}

// FindOne finds a PR by organization ID, repository ID, and PR number.
func (r *MemoryPullRequestsRepository) FindOne(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error) {
	return r.FindByNumberAndRepositoryID(ctx, orgID, repoID, number)
}

// FindByNumberAndRepositoryID finds a PR by number and repository ID within an organization.
func (r *MemoryPullRequestsRepository) FindByNumberAndRepositoryID(ctx context.Context, orgID string, repoID string, number int) (*models.PullRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && pr.Repository.ID == repoID && pr.Number == number {
			return clonePR(pr), nil
		}
	}
	return nil, nil
}

// Find lists pull requests for an organization and repository with pagination.
func (r *MemoryPullRequestsRepository) Find(ctx context.Context, orgID string, repoID string, limit int, offset int) ([]*models.PullRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.PullRequest
	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && (repoID == "" || pr.Repository.ID == repoID) {
			result = append(result, clonePR(pr))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if offset >= len(result) {
		return []*models.PullRequest{}, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

// FindPRNumbersByTitleAndOrganization finds PRs matching a title substring across repositories.
func (r *MemoryPullRequestsRepository) FindPRNumbersByTitleAndOrganization(ctx context.Context, title string, orgID string, repoIDs []string) ([]struct {
	Number       int
	RepositoryID string
}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool)
	for _, id := range repoIDs {
		repoSet[id] = true
	}

	var results []struct {
		Number       int
		RepositoryID string
	}

	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		if len(repoIDs) > 0 && !repoSet[pr.Repository.ID] {
			continue
		}
		if strings.Contains(strings.ToLower(pr.Title), strings.ToLower(title)) {
			results = append(results, struct {
				Number       int
				RepositoryID string
			}{
				Number:       pr.Number,
				RepositoryID: pr.Repository.ID,
			})
		}
	}
	return results, nil
}

// FindManyByNumbersAndRepositoryIDs retrieves multiple PR documents by criteria tuples.
func (r *MemoryPullRequestsRepository) FindManyByNumbersAndRepositoryIDs(ctx context.Context, criteria []struct {
	Number       int
	RepositoryID string
}, orgID string) ([]*models.PullRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	set := make(map[string]bool)
	for _, c := range criteria {
		set[fmt.Sprintf("%s:%d", c.RepositoryID, c.Number)] = true
	}

	var results []*models.PullRequest
	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		key := fmt.Sprintf("%s:%d", pr.Repository.ID, pr.Number)
		if set[key] {
			results = append(results, clonePR(pr))
		}
	}
	return results, nil
}

// FindManyByNumbers retrieves author user info for a list of PR numbers.
func (r *MemoryPullRequestsRepository) FindManyByNumbers(ctx context.Context, prNumbers []int, orgID string) ([]models.PullRequestUserMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	numSet := make(map[int]bool)
	for _, num := range prNumbers {
		numSet[num] = true
	}

	var results []models.PullRequestUserMapping
	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && numSet[pr.Number] {
			results = append(results, models.PullRequestUserMapping{
				Number: pr.Number,
				User:   pr.User,
			})
		}
	}
	return results, nil
}

// FindNumbersByRepositoryID returns PR numbers up to a specified timestamp.
func (r *MemoryPullRequestsRepository) FindNumbersByRepositoryID(ctx context.Context, orgID string, repoID string, until *time.Time) ([]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var numbers []int
	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && pr.Repository.ID == repoID {
			if until != nil && pr.CreatedAt.After(*until) {
				continue
			}
			numbers = append(numbers, pr.Number)
		}
	}
	sort.Ints(numbers)
	return numbers, nil
}

// FindSuggestionCountsByNumbersAndRepositoryIds aggregates suggestions by severity.
func (r *MemoryPullRequestsRepository) FindSuggestionCountsByNumbersAndRepositoryIds(ctx context.Context, criteria []struct {
	Number       int
	RepositoryID string
}, orgID string) (map[string]models.SuggestionCountsBySeverity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	critSet := make(map[string]bool)
	for _, c := range criteria {
		critSet[fmt.Sprintf("%s_%d", c.RepositoryID, c.Number)] = true
	}

	counts := make(map[string]models.SuggestionCountsBySeverity)

	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		key := fmt.Sprintf("%s_%d", pr.Repository.ID, pr.Number)
		if len(criteria) > 0 && !critSet[key] {
			continue
		}

		sc := counts[key]
		for _, f := range pr.Files {
			for _, s := range f.Suggestions {
				if s.DeliveryStatus == enums.DeliveryStatusSent {
					sc.Sent++
				}
				switch strings.ToLower(s.Severity) {
				case "critical":
					sc.BySeverity.Critical++
				case "high":
					sc.BySeverity.High++
				case "medium":
					sc.BySeverity.Medium++
				case "low":
					sc.BySeverity.Low++
				}
			}
		}
		counts[key] = sc
	}
	return counts, nil
}

// FindOpenPullRequestKeysOpenedSince returns keys for PRs opened since a timestamp.
func (r *MemoryPullRequestsRepository) FindOpenPullRequestKeysOpenedSince(ctx context.Context, since time.Time, orgID string, repoIDs []string) ([]struct {
	Number       int
	RepositoryID string
}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool)
	for _, id := range repoIDs {
		repoSet[id] = true
	}

	var results []struct {
		Number       int
		RepositoryID string
	}

	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		if len(repoIDs) > 0 && !repoSet[pr.Repository.ID] {
			continue
		}
		if strings.ToUpper(pr.Status) == "OPEN" && pr.CreatedAt.After(since) {
			results = append(results, struct {
				Number       int
				RepositoryID string
			}{
				Number:       pr.Number,
				RepositoryID: pr.Repository.ID,
			})
		}
	}
	return results, nil
}

// FindDistinctAuthorsByRepositoryIds aggregates distinct authors with counts.
func (r *MemoryPullRequestsRepository) FindDistinctAuthorsByRepositoryIds(ctx context.Context, orgID string, repoIDs []string, search string, limit int) ([]models.PullRequestAuthorSuggestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool)
	for _, id := range repoIDs {
		repoSet[id] = true
	}

	authorMap := make(map[string]*models.PullRequestAuthorSuggestion)

	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		if len(repoIDs) > 0 && !repoSet[pr.Repository.ID] {
			continue
		}

		u := pr.User
		if u.Username == "" && u.Email == "" {
			continue
		}

		if search != "" {
			q := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(u.Username), q) &&
				!strings.Contains(strings.ToLower(u.Name), q) &&
				!strings.Contains(strings.ToLower(u.Email), q) {
				continue
			}
		}

		key := u.Username
		if key == "" {
			key = u.ID
		}

		if existing, ok := authorMap[key]; ok {
			existing.Count++
		} else {
			authorMap[key] = &models.PullRequestAuthorSuggestion{
				ID:       u.ID,
				Username: u.Username,
				Name:     u.Name,
				Count:    1,
			}
		}
	}

	var list []models.PullRequestAuthorSuggestion
	for _, v := range authorMap {
		list = append(list, *v)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Count > list[j].Count
	})

	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// CountDeliveredPullRequests counts pull requests matching delivery criteria.
func (r *MemoryPullRequestsRepository) CountDeliveredPullRequests(ctx context.Context, orgID string, repoIDs []string, opts contracts.DeliveredFilterOpts) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool)
	for _, id := range repoIDs {
		repoSet[id] = true
	}

	sevSet := make(map[string]bool)
	for _, s := range opts.Severities {
		sevSet[strings.ToLower(s)] = true
	}

	count := 0
	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		if len(repoIDs) > 0 && !repoSet[pr.Repository.ID] {
			continue
		}
		if opts.OpenOnly && strings.ToUpper(pr.Status) != "OPEN" {
			continue
		}
		if opts.AuthorEmail != "" && !strings.EqualFold(pr.User.Email, opts.AuthorEmail) {
			continue
		}

		hasDelivered := false
		for _, f := range pr.Files {
			for _, s := range f.Suggestions {
				if s.DeliveryStatus == enums.DeliveryStatusSent {
					if len(sevSet) > 0 && !sevSet[strings.ToLower(s.Severity)] {
						continue
					}
					if opts.UnresolvedOnly && s.ImplementationStatus == enums.ImplementationStatusImplemented {
						continue
					}
					hasDelivered = true
					break
				}
			}
			if hasDelivered {
				break
			}
		}

		if hasDelivered {
			count++
		}
	}
	return count, nil
}

// FindFileWithSuggestions returns a single file document within a PR.
func (r *MemoryPullRequestsRepository) FindFileWithSuggestions(ctx context.Context, orgID string, repoID string, prNumber int, filePath string) (*models.File, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pr, err := r.FindByNumberAndRepositoryID(ctx, orgID, repoID, prNumber)
	if err != nil || pr == nil {
		return nil, err
	}

	norm := strings.TrimPrefix(filePath, "./")
	for _, f := range pr.Files {
		if strings.TrimPrefix(f.Path, "./") == norm {
			cp := f
			return &cp, nil
		}
	}
	return nil, nil
}

// FindSuggestionsByPR retrieves flattened suggestions filtered by delivery status.
func (r *MemoryPullRequestsRepository) FindSuggestionsByPR(ctx context.Context, orgID string, repoID string, prNumber int, deliveryStatus enums.DeliveryStatus) ([]models.Suggestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pr, err := r.FindByNumberAndRepositoryID(ctx, orgID, repoID, prNumber)
	if err != nil || pr == nil {
		return nil, err
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

// BulkApplyFileChanges applies a batch of file additions, modifications, and suggestion updates.
func (r *MemoryPullRequestsRepository) BulkApplyFileChanges(ctx context.Context, prUUID string, orgID string, ops []models.FileBulkOp) (*models.BulkApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pr, ok := r.prs[prUUID]
	if !ok || pr.OrganizationID != orgID {
		return nil, errors.New("pull request not found")
	}

	res := &models.BulkApplyResult{
		Attempted: len(ops),
	}
	fileMap := make(map[string]*models.File)
	for i := range pr.Files {
		f := &pr.Files[i]
		norm := strings.TrimPrefix(f.Path, "./")
		fileMap[norm] = f
	}

	for i, op := range ops {
		switch op.Kind {
		case "addFile":
			if op.File == nil {
				continue
			}
			norm := strings.TrimPrefix(op.File.Path, "./")
			if _, exists := fileMap[norm]; !exists {
				if op.File.ID == "" {
					op.File.ID = uuid.NewString()
				}
				pr.Files = append(pr.Files, *op.File)
				fileMap[norm] = &pr.Files[len(pr.Files)-1]
				res.Modified++
			}
		case "updateFile":
			var target *models.File
			for fi := range pr.Files {
				if (op.FileID != "" && pr.Files[fi].ID == op.FileID) ||
					(op.File != nil && strings.TrimPrefix(pr.Files[fi].Path, "./") == strings.TrimPrefix(op.File.Path, "./")) {
					target = &pr.Files[fi]
					break
				}
			}
			if target != nil {
				if op.File != nil {
					target.Added = op.File.Added
					target.Deleted = op.File.Deleted
					target.Changes = op.File.Changes
					target.Status = op.File.Status
				}
				if op.FileUpdates != nil {
					if added, ok := op.FileUpdates["added"].(int); ok {
						target.Added = added
					}
					if deleted, ok := op.FileUpdates["deleted"].(int); ok {
						target.Deleted = deleted
					}
					if changes, ok := op.FileUpdates["changes"].(int); ok {
						target.Changes = changes
					}
					if status, ok := op.FileUpdates["status"].(string); ok {
						target.Status = status
					}
				}
				res.Modified++
			} else {
				res.Errors = append(res.Errors, models.BulkApplyError{
					OpIndex: i,
					Message: "file not found for update",
				})
			}
		case "addSuggestions":
			if len(op.Suggestions) == 0 {
				continue
			}
			for _, sug := range op.Suggestions {
				norm := strings.TrimPrefix(sug.RelevantFile, "./")
				target, exists := fileMap[norm]
				if !exists {
					// The suggestion references a file the PR does not
					// contain, so there is no diff to classify. Reporting it
					// as "modified" asserted a change that never happened.
					// "added" is the honest value: this entry is new to the
					// report (SARIF baselineState "new"). AUDIT_REMEDIATION.md
					// F-47.
					newFile := models.File{
						ID:          uuid.NewString(),
						Path:        sug.RelevantFile,
						Filename:    sug.RelevantFile,
						Status:      "added",
						Suggestions: []models.Suggestion{},
					}
					pr.Files = append(pr.Files, newFile)
					target = &pr.Files[len(pr.Files)-1]
					fileMap[norm] = target
				}
				if sug.ID == "" {
					sug.ID = uuid.NewString()
				}
				target.Suggestions = append(target.Suggestions, sug)
			}
			res.Modified++
		}
	}

	pr.UpdatedAt = time.Now().UTC()
	return res, nil
}

// ComputeFileTotals aggregates added, deleted, and changes totals.
func (r *MemoryPullRequestsRepository) ComputeFileTotals(ctx context.Context, prUUID string, orgID string) (added int, deleted int, changes int, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pr, ok := r.prs[prUUID]
	if !ok || pr.OrganizationID != orgID {
		return 0, 0, 0, errors.New("pull request not found")
	}

	for _, f := range pr.Files {
		added += f.Added
		deleted += f.Deleted
		changes += f.Changes
	}
	return added, deleted, changes, nil
}

// Update updates an existing pull request aggregate root.
func (r *MemoryPullRequestsRepository) Update(ctx context.Context, pr *models.PullRequest) (*models.PullRequest, error) {
	if pr == nil {
		return nil, errors.New("pull request cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.prs[pr.UUID]
	if !ok {
		return nil, errors.New("pull request not found")
	}

	pr.CreatedAt = existing.CreatedAt
	pr.UpdatedAt = time.Now().UTC()
	cloned := clonePR(pr)
	r.prs[pr.UUID] = cloned
	return clonePR(cloned), nil
}

// UpdateSuggestion mutates a suggestion across all PRs for an organization.
func (r *MemoryPullRequestsRepository) UpdateSuggestion(ctx context.Context, orgID string, suggestionID string, updateData map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, pr := range r.prs {
		if pr.OrganizationID != orgID {
			continue
		}
		for fi := range pr.Files {
			for si := range pr.Files[fi].Suggestions {
				s := &pr.Files[fi].Suggestions[si]
				if s.ID == suggestionID {
					applySuggestionUpdates(s, updateData)
					pr.UpdatedAt = time.Now().UTC()
					return nil
				}
			}
		}
	}
	return nil
}

func applySuggestionUpdates(s *models.Suggestion, updates map[string]interface{}) {
	if status, ok := updates["delivery_status"].(string); ok {
		s.DeliveryStatus = enums.DeliveryStatus(status)
	}
	if status, ok := updates["implementation_status"].(string); ok {
		s.ImplementationStatus = enums.ImplementationStatus(status)
	}
	if priority, ok := updates["priority_status"].(string); ok {
		s.PriorityStatus = enums.PriorityStatus(priority)
	}
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

// UpdateSyncedSuggestionsFlag updates synced_embedded_suggestions flag for PRs.
func (r *MemoryPullRequestsRepository) UpdateSyncedSuggestionsFlag(ctx context.Context, prNumbers []int, repoID string, orgID string, synced bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	numSet := make(map[int]bool)
	for _, n := range prNumbers {
		numSet[n] = true
	}

	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && pr.Repository.ID == repoID && numSet[pr.Number] {
			pr.SyncedEmbeddedSuggestions = synced
			pr.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}

// UpdateSyncedWithIssuesFlag updates synced_with_issues flag for a PR.
func (r *MemoryPullRequestsRepository) UpdateSyncedWithIssuesFlag(ctx context.Context, prNumber int, repoID string, orgID string, synced bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, pr := range r.prs {
		if pr.OrganizationID == orgID && pr.Repository.ID == repoID && pr.Number == prNumber {
			pr.SyncedWithIssues = synced
			pr.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return nil
}
