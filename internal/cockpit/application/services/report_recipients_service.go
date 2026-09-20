package services

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/cockpit/domain"
)

// UserIdentity models user details retrieved from identity providers.
type UserIdentity struct {
	UUID                   string   `json:"uuid"`
	Email                  string   `json:"email"`
	Name                   string   `json:"name"`
	Role                   string   `json:"role"`
	Status                 string   `json:"status"`
	AssignedRepositoryIDs []string `json:"assigned_repository_ids"`
}

// UserDirectoryService abstracts user lookups.
type UserDirectoryService interface {
	FindUsers(ctx context.Context, organizationID, role, status string) ([]UserIdentity, error)
}

// DefaultReportRecipientsService resolves recipient emails and authorized repositories.
type DefaultReportRecipientsService struct {
	userDirectory   UserDirectoryService
	reviewAnalytics domain.CockpitReviewAnalyticsService
}

// NewDefaultReportRecipientsService creates an initialized recipient service.
func NewDefaultReportRecipientsService(
	userDirectory UserDirectoryService,
	reviewAnalytics domain.CockpitReviewAnalyticsService,
) *DefaultReportRecipientsService {
	return &DefaultReportRecipientsService{
		userDirectory:   userDirectory,
		reviewAnalytics: reviewAnalytics,
	}
}

func (s *DefaultReportRecipientsService) GetOwners(ctx context.Context, organizationID string) ([]domain.ReportRecipient, error) {
	users, err := s.userDirectory.FindUsers(ctx, organizationID, "OWNER", "ACTIVE")
	if err != nil {
		return nil, err
	}

	var recipients []domain.ReportRecipient
	for _, u := range users {
		if u.Email != "" {
			recipients = append(recipients, domain.ReportRecipient{
				Email: u.Email,
				Name:  resolveDisplayName(u),
			})
		}
	}
	return recipients, nil
}

func (s *DefaultReportRecipientsService) GetRepoAdmins(ctx context.Context, organizationID string) ([]domain.RepoAdminRecipient, error) {
	users, err := s.userDirectory.FindUsers(ctx, organizationID, "REPO_ADMIN", "ACTIVE")
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}

	repoNames, err := s.reviewAnalytics.GetRepositoryNames(ctx, organizationID)
	if err != nil {
		repoNames = make(map[string]string)
	}

	var recipients []domain.RepoAdminRecipient
	for _, u := range users {
		if u.Email == "" {
			continue
		}

		var repositories []string
		for _, repoID := range u.AssignedRepositoryIDs {
			if name, exists := repoNames[repoID]; exists && name != "" {
				repositories = append(repositories, name)
			}
		}

		if len(repositories) > 0 {
			recipients = append(recipients, domain.RepoAdminRecipient{
				Email:        u.Email,
				Name:         resolveDisplayName(u),
				Repositories: repositories,
			})
		}
	}

	return recipients, nil
}

func resolveDisplayName(u UserIdentity) string {
	if strings.TrimSpace(u.Name) != "" {
		return strings.Fields(u.Name)[0]
	}
	if strings.TrimSpace(u.Email) != "" {
		parts := strings.Split(u.Email, "@")
		return parts[0]
	}
	return "there"
}
