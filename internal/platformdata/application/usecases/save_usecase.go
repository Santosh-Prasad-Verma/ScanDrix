// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: usecases
// File: save_usecase.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

// SavePullRequestInput contains the raw webhook payload and origin platform metadata.
type SavePullRequestInput struct {
	Payload        map[string]interface{}
	PlatformType   string // "github", "gitlab", "bitbucket", "azure_repos", "forgejo"
	Event          string // "pull_request", "merge_request", etc.
	OrganizationID string
}

// SavePullRequestUseCase processes, normalizes, and saves inbound pull request webhooks.
type SavePullRequestUseCase struct {
	service contracts.IPullRequestsService
	logger  *slog.Logger
}

// NewSavePullRequestUseCase constructs a new webhook PR ingestion usecase.
func NewSavePullRequestUseCase(service contracts.IPullRequestsService, logger *slog.Logger) *SavePullRequestUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &SavePullRequestUseCase{
		service: service,
		logger:  logger.With("component", "save_pull_request_usecase"),
	}
}

// Execute parses the platform webhook, normalizes the PR entity, and stores it in PostgreSQL.
func (uc *SavePullRequestUseCase) Execute(ctx context.Context, input SavePullRequestInput) (*models.PullRequest, error) {
	if input.Payload == nil {
		return nil, errors.New("webhook payload is empty")
	}

	platform := strings.ToLower(input.PlatformType)
	normalizedPR, err := uc.normalizePullRequest(input.Payload, platform, input.OrganizationID)
	if err != nil {
		uc.logger.Warn("Skipping unsupported or malformed PR webhook", "error", err, "platform", platform)
		return nil, err
	}

	// Persist normalized PR via the domain service
	saved, err := uc.service.Create(ctx, normalizedPR)
	if err != nil {
		uc.logger.Error("Failed persisting normalized pull request",
			"error", err,
			"repoId", normalizedPR.Repository.ID,
			"prNumber", normalizedPR.Number,
		)
		return nil, err
	}

	uc.logger.Info("Saved platform pull request from webhook",
		"prNumber", saved.Number,
		"repo", saved.Repository.FullName,
		"platform", saved.Provider,
	)
	return saved, nil
}

func (uc *SavePullRequestUseCase) normalizePullRequest(
	payload map[string]interface{},
	platform string,
	orgID string,
) (*models.PullRequest, error) {
	switch platform {
	case "github", "forgejo":
		return uc.normalizeGitHub(payload, platform, orgID)
	case "gitlab":
		return uc.normalizeGitLab(payload, orgID)
	case "bitbucket":
		return uc.normalizeBitbucket(payload, orgID)
	case "azure_repos", "azuredevops":
		return uc.normalizeAzure(payload, orgID)
	default:
		return uc.normalizeGeneric(payload, platform, orgID)
	}
}

func (uc *SavePullRequestUseCase) normalizeGitHub(payload map[string]interface{}, platform string, orgID string) (*models.PullRequest, error) {
	prMap, ok := payload["pull_request"].(map[string]interface{})
	if !ok {
		return nil, errors.New("missing pull_request key in GitHub payload")
	}

	repoMap, _ := payload["repository"].(map[string]interface{})
	userMap, _ := prMap["user"].(map[string]interface{})

	num := int(asInt64(prMap["number"]))
	title := asString(prMap["title"])
	url := asString(prMap["html_url"])
	state := strings.ToUpper(asString(prMap["state"]))
	merged := asBool(prMap["merged"])
	isDraft := asBool(prMap["draft"])

	baseBranch := ""
	if base, ok := prMap["base"].(map[string]interface{}); ok {
		baseBranch = asString(base["ref"])
	}
	headBranch := ""
	if head, ok := prMap["head"].(map[string]interface{}); ok {
		headBranch = asString(head["ref"])
	}

	repoID := fmt.Sprintf("%v", repoMap["id"])
	repoName := asString(repoMap["name"])
	repoFullName := asString(repoMap["full_name"])

	return &models.PullRequest{
		UUID:          uuid.NewString(),
		Title:         title,
		Status:        state,
		Merged:        merged,
		Number:        num,
		URL:           url,
		BaseBranchRef: baseBranch,
		HeadBranchRef: headBranch,
		IsDraft:       isDraft,
		Provider:      platform,
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       repoID,
			Name:     repoName,
			FullName: repoFullName,
			URL:      asString(repoMap["html_url"]),
		},
		User: models.PullRequestUser{
			ID:       fmt.Sprintf("%v", userMap["id"]),
			Username: asString(userMap["login"]),
		},
		TotalAdded:   int(asInt64(prMap["additions"])),
		TotalDeleted: int(asInt64(prMap["deletions"])),
		TotalChanges: int(asInt64(prMap["changed_files"])),
		Files:        []models.File{},
		Commits:      []models.Commit{},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

func (uc *SavePullRequestUseCase) normalizeGitLab(payload map[string]interface{}, orgID string) (*models.PullRequest, error) {
	attrs, ok := payload["object_attributes"].(map[string]interface{})
	if !ok {
		return nil, errors.New("missing object_attributes in GitLab payload")
	}

	projMap, _ := payload["project"].(map[string]interface{})
	userMap, _ := payload["user"].(map[string]interface{})

	num := int(asInt64(attrs["iid"]))
	title := asString(attrs["title"])
	url := asString(attrs["url"])
	state := strings.ToUpper(asString(attrs["state"]))
	merged := state == "MERGED"

	return &models.PullRequest{
		UUID:          uuid.NewString(),
		Title:         title,
		Status:        state,
		Merged:        merged,
		Number:        num,
		URL:           url,
		BaseBranchRef: asString(attrs["target_branch"]),
		HeadBranchRef: asString(attrs["source_branch"]),
		Provider:      "gitlab",
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       fmt.Sprintf("%v", projMap["id"]),
			Name:     asString(projMap["name"]),
			FullName: asString(projMap["path_with_namespace"]),
			URL:      asString(projMap["web_url"]),
		},
		User: models.PullRequestUser{
			ID:       fmt.Sprintf("%v", userMap["id"]),
			Username: asString(userMap["username"]),
			Name:     asString(userMap["name"]),
			Email:    asString(userMap["email"]),
		},
		Files:     []models.File{},
		Commits:   []models.Commit{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (uc *SavePullRequestUseCase) normalizeBitbucket(payload map[string]interface{}, orgID string) (*models.PullRequest, error) {
	prMap, ok := payload["pullrequest"].(map[string]interface{})
	if !ok {
		return nil, errors.New("missing pullrequest in Bitbucket payload")
	}

	repoMap, _ := payload["repository"].(map[string]interface{})
	authorMap, _ := prMap["author"].(map[string]interface{})

	num := int(asInt64(prMap["id"]))
	title := asString(prMap["title"])
	state := strings.ToUpper(asString(prMap["state"]))

	return &models.PullRequest{
		UUID:          uuid.NewString(),
		Title:         title,
		Status:        state,
		Merged:        state == "MERGED",
		Number:        num,
		Provider:      "bitbucket",
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       asString(repoMap["uuid"]),
			Name:     asString(repoMap["name"]),
			FullName: asString(repoMap["full_name"]),
		},
		User: models.PullRequestUser{
			ID:       asString(authorMap["uuid"]),
			Username: asString(authorMap["username"]),
			Name:     asString(authorMap["display_name"]),
		},
		Files:     []models.File{},
		Commits:   []models.Commit{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (uc *SavePullRequestUseCase) normalizeAzure(payload map[string]interface{}, orgID string) (*models.PullRequest, error) {
	resource, ok := payload["resource"].(map[string]interface{})
	if !ok {
		return nil, errors.New("missing resource in Azure DevOps payload")
	}

	repoMap, _ := resource["repository"].(map[string]interface{})
	createdBy, _ := resource["createdBy"].(map[string]interface{})

	num := int(asInt64(resource["pullRequestId"]))
	title := asString(resource["title"])
	status := strings.ToUpper(asString(resource["status"]))

	return &models.PullRequest{
		UUID:          uuid.NewString(),
		Title:         title,
		Status:        status,
		Merged:        status == "COMPLETED",
		Number:        num,
		Provider:      "azure_repos",
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       asString(repoMap["id"]),
			Name:     asString(repoMap["name"]),
			FullName: asString(repoMap["name"]),
		},
		User: models.PullRequestUser{
			ID:       asString(createdBy["id"]),
			Username: asString(createdBy["uniqueName"]),
			Name:     asString(createdBy["displayName"]),
		},
		Files:     []models.File{},
		Commits:   []models.Commit{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (uc *SavePullRequestUseCase) normalizeGeneric(payload map[string]interface{}, platform string, orgID string) (*models.PullRequest, error) {
	num := int(asInt64(payload["number"]))
	title := asString(payload["title"])

	return &models.PullRequest{
		UUID:          uuid.NewString(),
		Title:         title,
		Number:        num,
		Provider:      platform,
		OrganizationID: orgID,
		Files:         []models.File{},
		Commits:       []models.Commit{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}, nil
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func asBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func asInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	case string:
		i, _ := strconv.ParseInt(val, 10, 64)
		return i
	default:
		return 0
	}
}
