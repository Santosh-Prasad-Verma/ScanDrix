package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// InMemoryAutomationExecutionRepository manages automation executions with full query capabilities.
type InMemoryAutomationExecutionRepository struct {
	mu         sync.RWMutex
	executions map[string]*domain.AutomationExecutionEntity
}

// NewInMemoryAutomationExecutionRepository creates an initialized execution repository.
func NewInMemoryAutomationExecutionRepository() *InMemoryAutomationExecutionRepository {
	return &InMemoryAutomationExecutionRepository{
		executions: make(map[string]*domain.AutomationExecutionEntity),
	}
}

func (r *InMemoryAutomationExecutionRepository) Create(ctx context.Context, exec *domain.AutomationExecutionEntity) (*domain.AutomationExecutionEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if exec.UUID == "" {
		exec.UUID = uuid.New().String()
	}
	now := time.Now().UTC()
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = now
	}
	exec.UpdatedAt = now

	copy := *exec
	r.executions[exec.UUID] = &copy
	return &copy, nil
}

func (r *InMemoryAutomationExecutionRepository) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.AutomationExecutionEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, exec := range r.executions {
		if matchesExecutionFilter(exec, filter) {
			if s, ok := data["status"].(domain.AutomationStatus); ok {
				exec.Status = s
			}
			if msg, ok := data["errorMessage"].(string); ok {
				exec.ErrorMessage = msg
			}
			if de, ok := data["dataExecution"].(map[string]any); ok {
				exec.DataExecution = de
			}
			exec.UpdatedAt = time.Now().UTC()
			copy := *exec
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("automation execution not found for update")
}

func (r *InMemoryAutomationExecutionRepository) FindStaleInProgress(ctx context.Context, cutoffDate time.Time, limit int) ([]*domain.AutomationExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var stale []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if exec.Status == domain.StatusInProgress && exec.UpdatedAt.Before(cutoffDate) {
			copy := *exec
			stale = append(stale, &copy)
		}
	}

	sort.Slice(stale, func(i, j int) bool {
		return stale[i].UpdatedAt.Before(stale[j].UpdatedAt)
	})

	if limit > 0 && len(stale) > limit {
		stale = stale[:limit]
	}
	return stale, nil
}

func (r *InMemoryAutomationExecutionRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.executions, id)
	return nil
}

func (r *InMemoryAutomationExecutionRepository) FindByID(ctx context.Context, id string) (*domain.AutomationExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exec, exists := r.executions[id]
	if !exists {
		return nil, nil
	}
	copy := *exec
	return &copy, nil
}

func (r *InMemoryAutomationExecutionRepository) Find(ctx context.Context, filter map[string]any) ([]*domain.AutomationExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if filter == nil || matchesExecutionFilter(exec, filter) {
			copy := *exec
			results = append(results, &copy)
		}
	}
	return results, nil
}

func (r *InMemoryAutomationExecutionRepository) FindPullRequestExecutionsByOrganizationAndTeam(ctx context.Context, params domain.PRQueryParams) ([]*domain.AutomationExecutionEntity, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if params.Status != "" && exec.Status != params.Status {
			continue
		}
		if params.RepositoryID != "" && exec.RepositoryID != params.RepositoryID {
			continue
		}
		if params.StartDate != nil && exec.CreatedAt.Before(*params.StartDate) {
			continue
		}
		if params.EndDate != nil && exec.CreatedAt.After(*params.EndDate) {
			continue
		}
		if params.Search != "" {
			searchLower := strings.ToLower(params.Search)
			msgLower := strings.ToLower(exec.ErrorMessage)
			if !strings.Contains(msgLower, searchLower) {
				continue
			}
		}
		copy := *exec
		matched = append(matched, &copy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := int64(len(matched))
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}

	startIdx := (page - 1) * pageSize
	if startIdx >= len(matched) {
		return []*domain.AutomationExecutionEntity{}, total, nil
	}
	endIdx := startIdx + pageSize
	if endIdx > len(matched) {
		endIdx = len(matched)
	}

	return matched[startIdx:endIdx], total, nil
}

func (r *InMemoryAutomationExecutionRepository) GetAwaitingReviewPullRequestKeys(ctx context.Context, params domain.AwaitingReviewParams) ([]domain.PRKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool, len(params.RepositoryIDs))
	for _, id := range params.RepositoryIDs {
		repoSet[id] = true
	}

	seen := make(map[domain.PRKey]bool)
	var keys []domain.PRKey

	for _, exec := range r.executions {
		if exec.Status == domain.StatusPending || exec.Status == domain.StatusInProgress {
			if len(repoSet) > 0 && !repoSet[exec.RepositoryID] {
				continue
			}
			if !params.Since.IsZero() && exec.CreatedAt.Before(params.Since) {
				continue
			}
			key := domain.PRKey{
				RepositoryID:      exec.RepositoryID,
				PullRequestNumber: exec.PullRequestNumber,
			}
			if !seen[key] && key.PullRequestNumber > 0 {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return keys, nil
}

func (r *InMemoryAutomationExecutionRepository) GetDistinctReviewedPullRequestKeys(ctx context.Context, params domain.DistinctReviewedParams) ([]domain.PRKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSet := make(map[string]bool, len(params.RepositoryIDs))
	for _, id := range params.RepositoryIDs {
		repoSet[id] = true
	}

	seen := make(map[domain.PRKey]bool)
	var keys []domain.PRKey

	for _, exec := range r.executions {
		if exec.Status == domain.StatusSuccess || exec.Status == domain.StatusPartialError {
			if len(repoSet) > 0 && !repoSet[exec.RepositoryID] {
				continue
			}
			if !params.StartDate.IsZero() && exec.CreatedAt.Before(params.StartDate) {
				continue
			}
			if !params.EndDate.IsZero() && exec.CreatedAt.After(params.EndDate) {
				continue
			}
			key := domain.PRKey{
				RepositoryID:      exec.RepositoryID,
				PullRequestNumber: exec.PullRequestNumber,
			}
			if !seen[key] && key.PullRequestNumber > 0 {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return keys, nil
}

func (r *InMemoryAutomationExecutionRepository) FindCliReviewExecutionsByOrganization(ctx context.Context, params domain.CliQueryParams) ([]*domain.AutomationExecutionEntity, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if !strings.HasPrefix(strings.ToLower(exec.Origin), "cli") &&
			!strings.HasPrefix(strings.ToLower(exec.Origin), "command") {
			continue
		}
		if params.Status != "" && exec.Status != params.Status {
			continue
		}
		if params.StartDate != nil && exec.CreatedAt.Before(*params.StartDate) {
			continue
		}
		if params.EndDate != nil && exec.CreatedAt.After(*params.EndDate) {
			continue
		}
		copy := *exec
		matched = append(matched, &copy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := int64(len(matched))
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}

	startIdx := (page - 1) * pageSize
	if startIdx >= len(matched) {
		return []*domain.AutomationExecutionEntity{}, total, nil
	}
	endIdx := startIdx + pageSize
	if endIdx > len(matched) {
		endIdx = len(matched)
	}

	return matched[startIdx:endIdx], total, nil
}

func (r *InMemoryAutomationExecutionRepository) FindLatestExecutionByFilters(ctx context.Context, filter map[string]any) (*domain.AutomationExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if matchesExecutionFilter(exec, filter) {
			copy := *exec
			matched = append(matched, &copy)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	return matched[0], nil
}

func (r *InMemoryAutomationExecutionRepository) FindByPeriodAndTeamAutomationID(ctx context.Context, teamAutomationID string, start, end time.Time) ([]*domain.AutomationExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*domain.AutomationExecutionEntity
	for _, exec := range r.executions {
		if exec.TeamAutomation != nil && exec.TeamAutomation.UUID == teamAutomationID {
			if !start.IsZero() && exec.CreatedAt.Before(start) {
				continue
			}
			if !end.IsZero() && exec.CreatedAt.After(end) {
				continue
			}
			copy := *exec
			results = append(results, &copy)
		}
	}
	return results, nil
}

func (r *InMemoryAutomationExecutionRepository) FindEligiblePullRequestRefsForApprovalByPeriodAndTeamAutomationID(ctx context.Context, teamAutomationID string, start, end time.Time) ([]domain.PRRef, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[domain.PRKey]*domain.AutomationExecutionEntity)

	for _, exec := range r.executions {
		if exec.TeamAutomation != nil && exec.TeamAutomation.UUID == teamAutomationID {
			if exec.Status == domain.StatusSuccess {
				if !start.IsZero() && exec.CreatedAt.Before(start) {
					continue
				}
				if !end.IsZero() && exec.CreatedAt.After(end) {
					continue
				}
				key := domain.PRKey{
					RepositoryID:      exec.RepositoryID,
					PullRequestNumber: exec.PullRequestNumber,
				}
				existing := seen[key]
				if existing == nil || exec.CreatedAt.After(existing.CreatedAt) {
					seen[key] = exec
				}
			}
		}
	}

	var refs []domain.PRRef
	for key, exec := range seen {
		headCommit := ""
		if exec.DataExecution != nil {
			if commitMap, ok := exec.DataExecution["lastAnalyzedCommit"].(map[string]any); ok {
				if sha, ok := commitMap["sha"].(string); ok {
					headCommit = sha
				}
			}
		}
		refs = append(refs, domain.PRRef{
			RepositoryID:      key.RepositoryID,
			PullRequestNumber: key.PullRequestNumber,
			LastExecutionDate: exec.CreatedAt,
			HeadCommitSHA:     headCommit,
		})
	}
	return refs, nil
}

func matchesExecutionFilter(e *domain.AutomationExecutionEntity, filter map[string]any) bool {
	if filter == nil {
		return true
	}
	if u, ok := filter["uuid"].(string); ok && u != "" && e.UUID != u {
		return false
	}
	if s, ok := filter["status"].(domain.AutomationStatus); ok && e.Status != s {
		return false
	}
	if pr, ok := filter["pullRequestNumber"].(int); ok && e.PullRequestNumber != pr {
		return false
	}
	if repoID, ok := filter["repositoryId"].(string); ok && repoID != "" && e.RepositoryID != repoID {
		return false
	}
	if taMap, ok := filter["teamAutomation"].(map[string]any); ok {
		if taUUID, ok := taMap["uuid"].(string); ok && taUUID != "" {
			if e.TeamAutomation == nil || e.TeamAutomation.UUID != taUUID {
				return false
			}
		}
	}
	if taID, ok := filter["teamAutomationId"].(string); ok && taID != "" {
		if e.TeamAutomation == nil || e.TeamAutomation.UUID != taID {
			return false
		}
	}
	if since, ok := filter["createdAtGte"].(time.Time); ok {
		if e.CreatedAt.Before(since) {
			return false
		}
	}
	return true
}
