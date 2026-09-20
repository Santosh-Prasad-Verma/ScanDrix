package services

import (
	"context"

	"github.com/scandrix/backend/internal/issues/domain"
)

// IssuesService implements domain.IssuesService wrapping the repository layer.
type IssuesService struct {
	repo domain.IssuesRepository
}

// NewIssuesService creates an initialized IssuesService.
func NewIssuesService(repo domain.IssuesRepository) *IssuesService {
	return &IssuesService{repo: repo}
}

func (s *IssuesService) Create(ctx context.Context, issue *domain.Issue) (*domain.Issue, error) {
	return s.repo.Create(ctx, issue)
}

func (s *IssuesService) FindByID(ctx context.Context, uuid string) (*domain.Issue, error) {
	return s.repo.FindByID(ctx, uuid)
}

func (s *IssuesService) FindOne(ctx context.Context, filter map[string]any) (*domain.Issue, error) {
	return s.repo.FindOne(ctx, filter)
}

func (s *IssuesService) FindByFileAndStatus(ctx context.Context, orgID, repoID, filePath string, status domain.IssueStatus) ([]*domain.Issue, error) {
	return s.repo.FindByFileAndStatus(ctx, orgID, repoID, filePath, status)
}

func (s *IssuesService) Find(ctx context.Context, orgID string) ([]*domain.Issue, error) {
	return s.repo.Find(ctx, orgID)
}

func (s *IssuesService) FindByFilters(ctx context.Context, filter map[string]any) ([]*domain.Issue, error) {
	return s.repo.FindByFilters(ctx, filter)
}

func (s *IssuesService) Count(ctx context.Context, filter map[string]any) (int64, error) {
	return s.repo.Count(ctx, filter)
}

func (s *IssuesService) Update(ctx context.Context, uuid string, updateData map[string]any) (*domain.Issue, error) {
	return s.repo.Update(ctx, uuid, updateData)
}

func (s *IssuesService) UpdateLabel(ctx context.Context, uuid, label string) (*domain.Issue, error) {
	return s.repo.UpdateLabel(ctx, uuid, label)
}

func (s *IssuesService) UpdateSeverity(ctx context.Context, uuid string, severity domain.SeverityLevel) (*domain.Issue, error) {
	return s.repo.UpdateSeverity(ctx, uuid, severity)
}

func (s *IssuesService) UpdateStatus(ctx context.Context, uuid string, status domain.IssueStatus) (*domain.Issue, error) {
	return s.repo.UpdateStatus(ctx, uuid, status)
}

func (s *IssuesService) UpdateStatusByIds(ctx context.Context, uuids []string, status domain.IssueStatus) ([]*domain.Issue, error) {
	return s.repo.UpdateStatusByIds(ctx, uuids, status)
}

func (s *IssuesService) AddSuggestionIDs(ctx context.Context, uuid string, suggestionIDs []string) (*domain.Issue, error) {
	return s.repo.AddSuggestionIDs(ctx, uuid, suggestionIDs)
}
