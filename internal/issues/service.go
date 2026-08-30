package issues

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// IssueStore defines persistence methods for tracked issues.
type IssueStore interface {
	CreateIssue(ctx context.Context, issue TrackedIssue) error
	GetIssue(ctx context.Context, workspaceID, issueID uuid.UUID) (*TrackedIssue, error)
	ListIssues(ctx context.Context, workspaceID uuid.UUID, status IssueStatus) ([]TrackedIssue, error)
	UpdateStatus(ctx context.Context, workspaceID, issueID uuid.UUID, status IssueStatus) error
	ResolveByFingerprints(ctx context.Context, workspaceID uuid.UUID, fingerprints []string) (int, error)
}

// MemoryIssueStore provides thread-safe in-memory storage for testing.
type MemoryIssueStore struct {
	mu     sync.RWMutex
	issues map[uuid.UUID]TrackedIssue
}

func NewMemoryIssueStore() *MemoryIssueStore {
	return &MemoryIssueStore{
		issues: make(map[uuid.UUID]TrackedIssue),
	}
}

func (s *MemoryIssueStore) CreateIssue(ctx context.Context, issue TrackedIssue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.issues[issue.ID] = issue
	return nil
}

func (s *MemoryIssueStore) GetIssue(ctx context.Context, workspaceID, issueID uuid.UUID) (*TrackedIssue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	issue, ok := s.issues[issueID]
	if !ok || issue.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("issue not found")
	}
	return &issue, nil
}

func (s *MemoryIssueStore) ListIssues(ctx context.Context, workspaceID uuid.UUID, status IssueStatus) ([]TrackedIssue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []TrackedIssue
	for _, issue := range s.issues {
		if issue.WorkspaceID == workspaceID {
			if status == "" || issue.Status == status {
				list = append(list, issue)
			}
		}
	}
	return list, nil
}

func (s *MemoryIssueStore) UpdateStatus(ctx context.Context, workspaceID, issueID uuid.UUID, status IssueStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, ok := s.issues[issueID]
	if !ok || issue.WorkspaceID != workspaceID {
		return fmt.Errorf("issue not found")
	}
	issue.Status = status
	issue.UpdatedAt = time.Now().UTC()
	if status == StatusResolved {
		now := time.Now().UTC()
		issue.ResolvedAt = &now
	}
	s.issues[issueID] = issue
	return nil
}

func (s *MemoryIssueStore) ResolveByFingerprints(ctx context.Context, workspaceID uuid.UUID, fingerprints []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fpMap := make(map[string]bool)
	for _, fp := range fingerprints {
		fpMap[fp] = true
	}

	resolvedCount := 0
	now := time.Now().UTC()
	for id, issue := range s.issues {
		if issue.WorkspaceID == workspaceID && fpMap[issue.Fingerprint] && issue.Status != StatusResolved {
			issue.Status = StatusResolved
			issue.ResolvedAt = &now
			issue.UpdatedAt = now
			s.issues[id] = issue
			resolvedCount++
		}
	}
	return resolvedCount, nil
}

// IssueService manages lifecycle and auto-creation of tracked issues.
type IssueService struct {
	store  IssueStore
	policy IssueCreationPolicy
}

func NewIssueService(store IssueStore, policy IssueCreationPolicy) *IssueService {
	return &IssueService{
		store:  store,
		policy: policy,
	}
}

// AutoCreateFromFindings inspects review findings and escalates issues exceeding severity threshold.
func (s *IssueService) AutoCreateFromFindings(
	ctx context.Context,
	workspaceID, repoID, reviewID uuid.UUID,
	findings []models.CodeFinding,
) ([]TrackedIssue, error) {
	var created []TrackedIssue

	for _, f := range findings {
		shouldCreate := false
		switch s.policy.AutoCreateFromSeverity {
		case models.SeverityCritical:
			shouldCreate = (f.Severity == models.SeverityCritical)
		case models.SeverityHigh:
			shouldCreate = (f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh)
		case models.SeverityMedium:
			shouldCreate = (f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh || f.Severity == models.SeverityMedium)
		default:
			shouldCreate = true
		}

		if !shouldCreate {
			continue
		}

		issue := TrackedIssue{
			ID:             uuid.New(),
			WorkspaceID:    workspaceID,
			RepositoryID:   repoID,
			Title:          f.Title,
			Description:    f.Description,
			FilePath:       f.FilePath,
			StartLine:      f.StartLine,
			EndLine:        f.EndLine,
			Severity:       f.Severity,
			Category:       f.Category,
			Status:         StatusOpen,
			OriginReviewID: reviewID,
			Remediation:    f.Remediation,
			Fingerprint:    f.Fingerprint,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}

		if err := s.store.CreateIssue(ctx, issue); err != nil {
			return created, fmt.Errorf("failed persisting issue: %w", err)
		}
		created = append(created, issue)
	}

	return created, nil
}

// ResolveIssuesOnMerge marks previously flagged issues as resolved when fixes are merged.
func (s *IssueService) ResolveIssuesOnMerge(ctx context.Context, workspaceID uuid.UUID, fixedFingerprints []string) (int, error) {
	return s.store.ResolveByFingerprints(ctx, workspaceID, fixedFingerprints)
}

// ListIssues retrieves workspace issues filtered by status.
func (s *IssueService) ListIssues(ctx context.Context, workspaceID uuid.UUID, status IssueStatus) ([]TrackedIssue, error) {
	return s.store.ListIssues(ctx, workspaceID, status)
}

// UpdateStatus changes the lifecycle state of a specific issue.
func (s *IssueService) UpdateStatus(ctx context.Context, workspaceID, issueID uuid.UUID, status IssueStatus) error {
	return s.store.UpdateStatus(ctx, workspaceID, issueID, status)
}
