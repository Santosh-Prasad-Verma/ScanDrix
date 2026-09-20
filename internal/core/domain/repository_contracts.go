package domain

import (
	"context"

	"github.com/google/uuid"
)

// TrackedRepositoryCreateDTO payload for connecting a repository.
type TrackedRepositoryCreateDTO struct {
	WorkspaceID     uuid.UUID `json:"workspace_id"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	SCMProvider     string    `json:"scm_provider"`
	ExternalRepoID  string    `json:"external_repo_id"`
	FullName        string    `json:"full_name"`
	DefaultBranch   string    `json:"default_branch"`
	IsPrivate       bool      `json:"is_private"`
	ConfigFilePath  string    `json:"config_file_path"`
	CustomPrompt    *string   `json:"custom_prompt,omitempty"`
	Settings        JSONBMap  `json:"settings"`
}

// TrackedRepositoryRepository contract.
type TrackedRepositoryRepository interface {
	FindByID(ctx context.Context, wsID, repoID uuid.UUID) (*TrackedRepository, error)
	FindByFullName(ctx context.Context, wsID uuid.UUID, provider, fullName string) (*TrackedRepository, error)
	Create(ctx context.Context, repo *TrackedRepository) error
	Update(ctx context.Context, repo *TrackedRepository) error
	ListByOrganization(ctx context.Context, wsID, orgID uuid.UUID) ([]*TrackedRepository, error)
}
