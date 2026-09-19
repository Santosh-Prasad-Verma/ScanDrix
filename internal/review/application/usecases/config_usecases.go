package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// IConfigRepository persists and retrieves code review parameters across the org hierarchy.
type IConfigRepository interface {
	GetByScope(ctx context.Context, orgID, teamID, repoID string) (*domain.CodeReviewParameter, error)
	Upsert(ctx context.Context, param *domain.CodeReviewParameter) error
	Delete(ctx context.Context, orgID, teamID, repoID string) error
	ListByOrg(ctx context.Context, orgID string) ([]domain.CodeReviewParameter, error)
}

// ConfigUseCases coordinates code review configuration operations.
type ConfigUseCases struct {
	repo IConfigRepository
}

// NewConfigUseCases creates a new config use cases coordinator.
func NewConfigUseCases(repo IConfigRepository) *ConfigUseCases {
	return &ConfigUseCases{repo: repo}
}

// GetCodeReviewParameter resolves review configuration following Repo -> Team -> Org -> Default fallback hierarchy.
func (u *ConfigUseCases) GetCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string) (domain.CodeReviewConfig, error) {
	// 1. Try repository-specific configuration
	if repoID != "" {
		param, err := u.repo.GetByScope(ctx, orgID, teamID, repoID)
		if err == nil && param != nil {
			return param.Config, nil
		}
	}

	// 2. Try team-level configuration
	if teamID != "" {
		param, err := u.repo.GetByScope(ctx, orgID, teamID, "")
		if err == nil && param != nil {
			return param.Config, nil
		}
	}

	// 3. Try organization-level configuration
	if orgID != "" {
		param, err := u.repo.GetByScope(ctx, orgID, "", "")
		if err == nil && param != nil {
			return param.Config, nil
		}
	}

	// 4. Fallback to production baseline defaults
	return domain.DefaultCodeReviewConfig(), nil
}

// UpdateOrCreateCodeReviewParameter persists or updates configuration for a given hierarchy scope.
func (u *ConfigUseCases) UpdateOrCreateCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, cfg domain.CodeReviewConfig) (*domain.CodeReviewParameter, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	existing, err := u.repo.GetByScope(ctx, orgID, teamID, repoID)
	now := time.Now().UTC()

	param := &domain.CodeReviewParameter{
		OrganizationID: orgID,
		TeamID:         teamID,
		RepositoryID:   repoID,
		Config:         cfg,
		UpdatedAt:      now,
	}

	if err == nil && existing != nil {
		param.ID = existing.ID
		param.CreatedAt = existing.CreatedAt
		param.Repositories = existing.Repositories
	} else {
		param.ID = uuid.New()
		param.CreatedAt = now
	}

	if err := u.repo.Upsert(ctx, param); err != nil {
		return nil, fmt.Errorf("failed to save code review parameters: %w", err)
	}

	return param, nil
}

// UpdateCodeReviewParameterRepositories updates tracked repository associations.
func (u *ConfigUseCases) UpdateCodeReviewParameterRepositories(ctx context.Context, orgID, teamID string, repoIDs []string) error {
	param, err := u.repo.GetByScope(ctx, orgID, teamID, "")
	now := time.Now().UTC()

	if err != nil || param == nil {
		param = &domain.CodeReviewParameter{
			ID:             uuid.New(),
			OrganizationID: orgID,
			TeamID:         teamID,
			Config:         domain.DefaultCodeReviewConfig(),
			Repositories:   repoIDs,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	} else {
		param.Repositories = repoIDs
		param.UpdatedAt = now
	}

	return u.repo.Upsert(ctx, param)
}

// DeleteRepositoryCodeReviewParameter removes repository override, reverting to team/org defaults.
func (u *ConfigUseCases) DeleteRepositoryCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string) error {
	if repoID == "" {
		return fmt.Errorf("repositoryId is required for deletion")
	}
	return u.repo.Delete(ctx, orgID, teamID, repoID)
}

// GenerateConfigFile produces JSON or YAML content for checked-in `.scandrix/config.json`.
func (u *ConfigUseCases) GenerateConfigFile(ctx context.Context, orgID, teamID, repoID string) ([]byte, error) {
	cfg, err := u.GetCodeReviewParameter(ctx, orgID, teamID, repoID)
	if err != nil {
		return nil, err
	}

	formatted := map[string]interface{}{
		"$schema":              "https://scandrix.dev/schema/config.v1.json",
		"version":              1,
		"enabled":              cfg.Enabled,
		"strictness":           cfg.Strictness,
		"autoApprove":          cfg.AutoApprove,
		"approvalThreshold":    cfg.ApprovalThreshold,
		"ignorePaths":          cfg.IgnorePaths,
		"maxSuggestions":       cfg.MaxSuggestions,
		"maxFilesPerReview":    cfg.MaxFilesPerReview,
		"fileSizeLimits":       cfg.FileSizeLimits,
		"syntaxCheckInSandbox": cfg.SyntaxCheckInSandbox,
	}

	return json.MarshalIndent(formatted, "", "  ")
}

// CliRepositorySettings mirrors settings consumed by the ScanDrix CLI reviewer.
type CliRepositorySettings struct {
	Enabled        bool                   `json:"enabled"`
	Strictness     domain.ModelStrictness `json:"strictness"`
	IgnorePaths    []string               `json:"ignorePaths"`
	MaxSuggestions int                    `json:"maxSuggestions"`
	MaxFiles       int                    `json:"maxFiles"`
}

// GetCliRepositorySettings exports concise review policies for CLI headless runs.
func (u *ConfigUseCases) GetCliRepositorySettings(ctx context.Context, orgID, teamID, repoID string) (CliRepositorySettings, error) {
	cfg, err := u.GetCodeReviewParameter(ctx, orgID, teamID, repoID)
	if err != nil {
		return CliRepositorySettings{}, err
	}

	return CliRepositorySettings{
		Enabled:        cfg.Enabled,
		Strictness:     cfg.Strictness,
		IgnorePaths:    cfg.IgnorePaths,
		MaxSuggestions: cfg.MaxSuggestions,
		MaxFiles:       cfg.MaxFilesPerReview,
	}, nil
}

// UpdateCliRepositorySettings updates settings from CLI configuration calls.
func (u *ConfigUseCases) UpdateCliRepositorySettings(ctx context.Context, orgID, teamID, repoID string, settings CliRepositorySettings) error {
	cfg, _ := u.GetCodeReviewParameter(ctx, orgID, teamID, repoID)
	cfg.Enabled = settings.Enabled
	cfg.Strictness = settings.Strictness
	cfg.IgnorePaths = settings.IgnorePaths
	cfg.MaxSuggestions = settings.MaxSuggestions
	cfg.MaxFilesPerReview = settings.MaxFiles

	_, err := u.UpdateOrCreateCodeReviewParameter(ctx, orgID, teamID, repoID, cfg)
	return err
}

// ListCodeReviewAutomationLabels returns standard labels with active status.
func (u *ConfigUseCases) ListCodeReviewAutomationLabels(ctx context.Context) []domain.AutomationLabel {
	return domain.DefaultAutomationLabels()
}
