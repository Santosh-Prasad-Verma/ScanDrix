package repositories

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/issues/domain"
)

// InMemoryIssuesRepository provides a thread-safe in-memory store for issues.
type InMemoryIssuesRepository struct {
	mu     sync.RWMutex
	issues map[string]*domain.Issue
}

// NewInMemoryIssuesRepository creates an initialized issues repository.
func NewInMemoryIssuesRepository() *InMemoryIssuesRepository {
	return &InMemoryIssuesRepository{
		issues: make(map[string]*domain.Issue),
	}
}

// Create inserts a new issue record.
func (r *InMemoryIssuesRepository) Create(ctx context.Context, issue *domain.Issue) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if issue.UUID == "" {
		issue.UUID = uuid.New().String()
	}
	now := time.Now().UTC()
	if issue.CreatedAt.IsZero() {
		issue.CreatedAt = now
	}
	if issue.UpdatedAt.IsZero() {
		issue.UpdatedAt = now
	}

	saved := deepCopyIssue(issue)
	r.issues[saved.UUID] = saved
	return deepCopyIssue(saved), nil
}

// FindByID retrieves an issue by its UUID.
func (r *InMemoryIssuesRepository) FindByID(ctx context.Context, id string) (*domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, nil
	}
	return deepCopyIssue(issue), nil
}

// FindOne returns the first issue matching the filter map.
func (r *InMemoryIssuesRepository) FindOne(ctx context.Context, filter map[string]any) (*domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, issue := range r.issues {
		if matchesFilter(issue, filter) {
			return deepCopyIssue(issue), nil
		}
	}
	return nil, nil
}

// FindByFileAndStatus locates issues for a specific file and optional status.
func (r *InMemoryIssuesRepository) FindByFileAndStatus(
	ctx context.Context,
	orgID, repoID, filePath string,
	status domain.IssueStatus,
) ([]*domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Issue
	for _, issue := range r.issues {
		if issue.OrganizationID != orgID {
			continue
		}
		if issue.Repository.ID != repoID {
			continue
		}
		if issue.FilePath != filePath {
			continue
		}

		if status != "" {
			if issue.Status == status {
				result = append(result, deepCopyIssue(issue))
			}
		} else {
			if issue.Status != domain.StatusOpen {
				result = append(result, deepCopyIssue(issue))
			}
		}
	}
	return result, nil
}

// Find retrieves all issues for an organization with projected fields.
func (r *InMemoryIssuesRepository) Find(ctx context.Context, orgID string) ([]*domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Issue
	for _, issue := range r.issues {
		if issue.OrganizationID == orgID {
			result = append(result, deepCopyIssue(issue))
		}
	}
	return result, nil
}

// FindByFilters searches issues matching complex query criteria.
func (r *InMemoryIssuesRepository) FindByFilters(ctx context.Context, filter map[string]any) ([]*domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Issue
	for _, issue := range r.issues {
		if matchesFilter(issue, filter) {
			result = append(result, deepCopyIssue(issue))
		}
	}
	return result, nil
}

// Count returns the total number of issues matching the filter.
func (r *InMemoryIssuesRepository) Count(ctx context.Context, filter map[string]any) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int64
	for _, issue := range r.issues {
		if matchesFilter(issue, filter) {
			count++
		}
	}
	return count, nil
}

// Update modifies arbitrary fields on an existing issue.
func (r *InMemoryIssuesRepository) Update(ctx context.Context, id string, updateData map[string]any) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, fmt.Errorf("issue not found: %s", id)
	}

	for k, v := range updateData {
		switch k {
		case "title":
			if s, ok := v.(string); ok {
				issue.Title = s
			}
		case "description":
			if s, ok := v.(string); ok {
				issue.Description = s
			}
		case "filePath":
			if s, ok := v.(string); ok {
				issue.FilePath = s
			}
		case "label":
			if s, ok := v.(string); ok {
				issue.Label = s
			}
		case "severity":
			if s, ok := v.(domain.SeverityLevel); ok {
				issue.Severity = s
			} else if s, ok := v.(string); ok {
				issue.Severity = domain.SeverityLevel(s)
			}
		case "status":
			if s, ok := v.(domain.IssueStatus); ok {
				issue.Status = s
			} else if s, ok := v.(string); ok {
				issue.Status = domain.IssueStatus(s)
			}
		}
	}
	issue.UpdatedAt = time.Now().UTC()
	return deepCopyIssue(issue), nil
}

// UpdateLabel updates an issue's categorization label.
func (r *InMemoryIssuesRepository) UpdateLabel(ctx context.Context, id, label string) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	issue.Label = label
	issue.UpdatedAt = time.Now().UTC()
	return deepCopyIssue(issue), nil
}

// UpdateSeverity updates an issue's severity ranking.
func (r *InMemoryIssuesRepository) UpdateSeverity(ctx context.Context, id string, severity domain.SeverityLevel) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	issue.Severity = severity
	issue.UpdatedAt = time.Now().UTC()
	return deepCopyIssue(issue), nil
}

// UpdateStatus updates the resolution status of an issue.
func (r *InMemoryIssuesRepository) UpdateStatus(ctx context.Context, id string, status domain.IssueStatus) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	issue.Status = status
	issue.UpdatedAt = time.Now().UTC()
	return deepCopyIssue(issue), nil
}

// UpdateStatusByIds bulk updates statuses for an array of issue UUIDs.
func (r *InMemoryIssuesRepository) UpdateStatusByIds(ctx context.Context, ids []string, status domain.IssueStatus) ([]*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idMap := make(map[string]bool, len(ids))
	for _, id := range ids {
		idMap[id] = true
	}

	var updated []*domain.Issue
	now := time.Now().UTC()
	for id, issue := range r.issues {
		if idMap[id] {
			issue.Status = status
			issue.UpdatedAt = now
			updated = append(updated, deepCopyIssue(issue))
		}
	}
	return updated, nil
}

// AddSuggestionIDs appends suggestion IDs to an issue.
func (r *InMemoryIssuesRepository) AddSuggestionIDs(ctx context.Context, id string, suggestionIDs []string) (*domain.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	issue, exists := r.issues[id]
	if !exists {
		return nil, fmt.Errorf("issue not found: %s", id)
	}

	existingMap := make(map[string]bool)
	for _, s := range issue.ContributingSuggestions {
		existingMap[s.ID] = true
	}

	for _, sid := range suggestionIDs {
		if !existingMap[sid] {
			issue.ContributingSuggestions = append(issue.ContributingSuggestions, domain.ContributingSuggestion{
				ID: sid,
			})
			existingMap[sid] = true
		}
	}
	issue.UpdatedAt = time.Now().UTC()
	return deepCopyIssue(issue), nil
}

func matchesFilter(issue *domain.Issue, filter map[string]any) bool {
	if filter == nil {
		return true
	}

	for k, v := range filter {
		switch k {
		case "organizationId":
			if s, ok := v.(string); ok && issue.OrganizationID != s {
				return false
			}
		case "severity":
			if s, ok := v.(string); ok && string(issue.Severity) != s {
				return false
			}
		case "category", "label":
			if s, ok := v.(string); ok && issue.Label != s {
				return false
			}
		case "filePath":
			if s, ok := v.(string); ok && issue.FilePath != s {
				return false
			}
		case "status":
			if s, ok := v.(string); ok && string(issue.Status) != s {
				return false
			}
		case "title":
			if s, ok := v.(string); ok {
				if !strings.Contains(strings.ToLower(issue.Title), strings.ToLower(s)) {
					return false
				}
			} else if rgx, ok := v.(*regexp.Regexp); ok {
				if !rgx.MatchString(issue.Title) {
					return false
				}
			}
		case "repository.name":
			if s, ok := v.(string); ok {
				if !strings.Contains(strings.ToLower(issue.Repository.Name), strings.ToLower(s)) {
					return false
				}
			} else if rgx, ok := v.(*regexp.Regexp); ok {
				if !rgx.MatchString(issue.Repository.Name) {
					return false
				}
			}
		case "repository.id":
			if s, ok := v.(string); ok && issue.Repository.ID != s {
				return false
			} else if ids, ok := v.([]string); ok {
				found := false
				for _, id := range ids {
					if issue.Repository.ID == id {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "createdAt":
			if m, ok := v.(map[string]any); ok {
				if lt, ok := m["$lt"].(time.Time); ok && !issue.CreatedAt.Before(lt) {
					return false
				}
				if gt, ok := m["$gt"].(time.Time); ok && !issue.CreatedAt.After(gt) {
					return false
				}
			}
		}
	}
	return true
}

func deepCopyIssue(src *domain.Issue) *domain.Issue {
	if src == nil {
		return nil
	}
	cp := *src

	if src.ContributingSuggestions != nil {
		cp.ContributingSuggestions = make([]domain.ContributingSuggestion, len(src.ContributingSuggestions))
		copy(cp.ContributingSuggestions, src.ContributingSuggestions)
	}
	if src.PRNumbers != nil {
		cp.PRNumbers = make([]string, len(src.PRNumbers))
		copy(cp.PRNumbers, src.PRNumbers)
	}
	if src.Owner != nil {
		owner := *src.Owner
		cp.Owner = &owner
	}
	if src.Reporter != nil {
		reporter := *src.Reporter
		cp.Reporter = &reporter
	}
	return &cp
}
