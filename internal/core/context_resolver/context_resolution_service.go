package contextresolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNoActiveIntegrations = errors.New("no active integrations found for organization")
	ErrRepositoryNotFound   = errors.New("repository not found in integration config")
	ErrCodeReviewConfigNone = errors.New("code review config not found")
	ErrNoRepositoriesConfig = errors.New("no repositories found in code review config")
	ErrNoDirectoriesConfig  = errors.New("no directories found for repository")
	ErrDirectoryNotFound    = errors.New("directory not found for repository")
)

// IntegrationConfigModel represents an integration configuration record from the database.
type IntegrationConfigModel struct {
	UUID        uuid.UUID
	TeamID      uuid.UUID
	ConfigKey   string
	ConfigValue any
}

// ParameterModel represents an organization/team parameter record.
type ParameterModel struct {
	UUID        uuid.UUID
	Key         string
	ConfigValue any
}

// IntegrationConfigReader defines the interface to fetch integration configurations.
type IntegrationConfigReader interface {
	FindByOrganizationAndConfigKey(ctx context.Context, organizationID uuid.UUID, configKey string) ([]IntegrationConfigModel, error)
}

// ParametersReader defines the interface to fetch scoped parameters.
type ParametersReader interface {
	FindByKey(ctx context.Context, key string, organizationID, teamID uuid.UUID) (*ParameterModel, error)
}

// ContextResolutionService mirrors ScanDrix ContextResolutionService, resolving team, directory, and repository context.
type ContextResolutionService struct {
	integrationConfigReader IntegrationConfigReader
	parametersReader        ParametersReader
}

// NewContextResolutionService instantiates a new ContextResolutionService.
func NewContextResolutionService(
	integrationConfigReader IntegrationConfigReader,
	parametersReader ParametersReader,
) *ContextResolutionService {
	return &ContextResolutionService{
		integrationConfigReader: integrationConfigReader,
		parametersReader:        parametersReader,
	}
}

// GetTeamIDByOrganizationAndRepository resolves the team UUID managing a specific repository in an organization.
func (s *ContextResolutionService) GetTeamIDByOrganizationAndRepository(
	ctx context.Context,
	organizationID uuid.UUID,
	repositoryID string,
) (uuid.UUID, error) {
	if organizationID == uuid.Nil || strings.TrimSpace(repositoryID) == "" {
		return uuid.Nil, errors.New("organizationId and repositoryId must be provided")
	}

	integrationConfigs, err := s.integrationConfigReader.FindByOrganizationAndConfigKey(
		ctx,
		organizationID,
		"repositories",
	)
	if err != nil {
		return uuid.Nil, err
	}
	if len(integrationConfigs) == 0 {
		return uuid.Nil, ErrNoActiveIntegrations
	}

	for _, config := range integrationConfigs {
		repos := parseRepositoryList(config.ConfigValue)
		for _, repo := range repos {
			if repo.ID == repositoryID || repo.ExternalID == repositoryID {
				if config.TeamID != uuid.Nil {
					return config.TeamID, nil
				}
			}
		}
	}

	return uuid.Nil, fmt.Errorf("%w: repository with id %s not found in any integration config", ErrRepositoryNotFound, repositoryID)
}

// GetDirectoryPathByOrganizationAndRepository resolves the configured filesystem directory path.
func (s *ContextResolutionService) GetDirectoryPathByOrganizationAndRepository(
	ctx context.Context,
	organizationID uuid.UUID,
	repositoryID string,
	directoryID string,
) (string, error) {
	if organizationID == uuid.Nil || repositoryID == "" || directoryID == "" {
		return "", nil
	}

	// 1. Get the teamId using the previous method
	teamID, err := s.GetTeamIDByOrganizationAndRepository(ctx, organizationID, repositoryID)
	if err != nil {
		return "", err
	}

	// 2. Search the PARAMETERS table for the CODE_REVIEW_CONFIG key
	codeReviewConfig, err := s.parametersReader.FindByKey(ctx, "code_review_config", organizationID, teamID)
	if err != nil {
		return "", err
	}
	if codeReviewConfig == nil {
		return "", ErrCodeReviewConfigNone
	}

	// 3. Search the repository list for the one matching repositoryId
	targetRepo := findRepositoryInConfig(codeReviewConfig.ConfigValue, repositoryID)
	if targetRepo == nil {
		return "", fmt.Errorf("%w: repository with id %s not found in code review config", ErrRepositoryNotFound, repositoryID)
	}

	// 4. Search the directories node for the path matching directoryId
	if len(targetRepo.Directories) == 0 {
		return "", fmt.Errorf("%w: for repository %s", ErrNoDirectoriesConfig, repositoryID)
	}

	for _, dir := range targetRepo.Directories {
		if dir.ID == directoryID {
			return dir.Path, nil
		}
	}

	return "", fmt.Errorf("%w: directory with id %s not found for repository %s", ErrDirectoryNotFound, directoryID, repositoryID)
}

// GetRepositoryNameByOrganizationAndRepository resolves the repository display name from configuration.
func (s *ContextResolutionService) GetRepositoryNameByOrganizationAndRepository(
	ctx context.Context,
	organizationID uuid.UUID,
	repositoryID string,
) (string, error) {
	if organizationID == uuid.Nil || repositoryID == "" {
		return "", nil
	}

	// 1. Get the teamId using the previous method
	teamID, err := s.GetTeamIDByOrganizationAndRepository(ctx, organizationID, repositoryID)
	if err != nil {
		return "", err
	}

	// 2. Search the PARAMETERS table for the CODE_REVIEW_CONFIG key
	codeReviewConfig, err := s.parametersReader.FindByKey(ctx, "code_review_config", organizationID, teamID)
	if err != nil {
		return "", err
	}
	if codeReviewConfig == nil {
		return "", ErrCodeReviewConfigNone
	}

	// 3. Search the repository list for the one matching repositoryId
	targetRepo := findRepositoryInConfig(codeReviewConfig.ConfigValue, repositoryID)
	if targetRepo == nil {
		return "", fmt.Errorf("%w: repository with id %s not found in code review config", ErrRepositoryNotFound, repositoryID)
	}

	return targetRepo.Name, nil
}

type repoItem struct {
	ID          string          `json:"id"`
	ExternalID  string          `json:"external_id"`
	Name        string          `json:"name"`
	Directories []directoryItem `json:"directories"`
}

type directoryItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

func parseRepositoryList(val any) []repoItem {
	if val == nil {
		return nil
	}
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil
	}
	var repos []repoItem
	if err := json.Unmarshal(bytes, &repos); err == nil && len(repos) > 0 {
		return repos
	}
	// Try parsing wrapped structure { repositories: [...] }
	var wrapped struct {
		Repositories []repoItem `json:"repositories"`
	}
	if err := json.Unmarshal(bytes, &wrapped); err == nil {
		return wrapped.Repositories
	}
	return nil
}

func findRepositoryInConfig(configValue any, repositoryID string) *repoItem {
	repos := parseRepositoryList(configValue)
	for _, repo := range repos {
		if repo.ID == repositoryID || repo.ExternalID == repositoryID {
			return &repo
		}
	}
	return nil
}
