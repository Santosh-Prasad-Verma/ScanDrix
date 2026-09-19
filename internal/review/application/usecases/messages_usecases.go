package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// MessagesUseCases coordinates custom pull request comment template configuration.
type MessagesUseCases struct {
	repo domain.IPullRequestMessagesRepository
}

// NewMessagesUseCases creates a new messages use case coordinator.
func NewMessagesUseCases(repo domain.IPullRequestMessagesRepository) *MessagesUseCases {
	return &MessagesUseCases{repo: repo}
}

// CreateOrUpdatePullRequestMessages saves or updates custom message configurations.
func (u *MessagesUseCases) CreateOrUpdatePullRequestMessages(ctx context.Context, msg *domain.PullRequestMessages) (*domain.PullRequestMessages, error) {
	if msg.OrganizationID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	filter := domain.MessagesFilter{
		OrganizationID: msg.OrganizationID,
		ConfigLevel:    msg.ConfigLevel,
		RepositoryID:   msg.RepositoryID,
		DirectoryID:    msg.DirectoryID,
	}

	existing, err := u.repo.FindOne(ctx, filter)
	now := time.Now().UTC()

	if err == nil && existing != nil {
		msg.ID = existing.ID
		msg.CreatedAt = existing.CreatedAt
		msg.UpdatedAt = now
		return u.repo.Update(ctx, msg)
	}

	msg.ID = uuid.New()
	msg.CreatedAt = now
	msg.UpdatedAt = now
	return u.repo.Create(ctx, msg)
}

// FindByRepoOrDirectory resolves effective messages following Directory -> Repo -> Global hierarchy.
func (u *MessagesUseCases) FindByRepoOrDirectory(ctx context.Context, orgID, repoID, dirID string) (*domain.PullRequestMessages, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	// 1. Directory-level override
	if repoID != "" && dirID != "" {
		msg, err := u.repo.FindOne(ctx, domain.MessagesFilter{
			OrganizationID: orgID,
			ConfigLevel:    domain.ConfigLevelDirectory,
			RepositoryID:   repoID,
			DirectoryID:    dirID,
		})
		if err == nil && msg != nil {
			return msg, nil
		}
	}

	// 2. Repository-level override
	if repoID != "" {
		msg, err := u.repo.FindOne(ctx, domain.MessagesFilter{
			OrganizationID: orgID,
			ConfigLevel:    domain.ConfigLevelRepository,
			RepositoryID:   repoID,
		})
		if err == nil && msg != nil {
			return msg, nil
		}
	}

	// 3. Organization global configuration
	msg, err := u.repo.FindOne(ctx, domain.MessagesFilter{
		OrganizationID: orgID,
		ConfigLevel:    domain.ConfigLevelGlobal,
	})
	if err == nil && msg != nil {
		return msg, nil
	}

	// 4. Fallback defaults
	startMsg := domain.DefaultStartReviewTemplate()
	endMsg := domain.DefaultEndReviewTemplate()
	errMsg := domain.DefaultErrorReviewTemplate()

	return &domain.PullRequestMessages{
		ID:                 uuid.New(),
		OrganizationID:     orgID,
		ConfigLevel:        domain.ConfigLevelGlobal,
		StartReviewMessage: &startMsg,
		EndReviewMessage:   &endMsg,
		ErrorReviewMessage: &errMsg,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}, nil
}

// FindOverrideCountsByRepository aggregates custom directory configurations across repositories.
func (u *MessagesUseCases) FindOverrideCountsByRepository(ctx context.Context, orgID string) ([]domain.DirectoryOverrideCount, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}
	return u.repo.FindOverrideCountsByOrg(ctx, orgID)
}

// DeleteByRepositoryOrDirectory removes custom message template overrides.
func (u *MessagesUseCases) DeleteByRepositoryOrDirectory(ctx context.Context, orgID, repoID, dirID string) error {
	filter := domain.MessagesFilter{
		OrganizationID: orgID,
		RepositoryID:   repoID,
		DirectoryID:    dirID,
	}
	if dirID != "" {
		filter.ConfigLevel = domain.ConfigLevelDirectory
	} else if repoID != "" {
		filter.ConfigLevel = domain.ConfigLevelRepository
	} else {
		filter.ConfigLevel = domain.ConfigLevelGlobal
	}

	_, err := u.repo.DeleteByFilter(ctx, filter)
	return err
}
