// Package infrastructure implements Git PR mutation orchestration for centralized configs in ScanDrix.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
	commonCodeReview "github.com/scandrix/backend/internal/common/codereview"
)

// GitPlatformPRClient defines operations for creating remote branches, commits, and pull requests.
type GitPlatformPRClient interface {
	CreateBranch(ctx context.Context, orgID, teamID, repoID, branchName, baseBranch string) error
	CreateCommit(ctx context.Context, orgID, teamID, repoID, branchName, commitMessage string, mutations []domain.FileMutationOp) (string, error)
	OpenPullRequest(ctx context.Context, orgID, teamID, repoID, title, description, sourceBranch, targetBranch string) (string, error)
	GetDefaultBranch(ctx context.Context, orgID, teamID, repoID string) (string, error)
	GetFileContent(ctx context.Context, orgID, teamID, repoID, filePath, ref string) ([]byte, error)
	ListOpenPullRequests(ctx context.Context, orgID, teamID, repoID string) ([]RemotePullRequest, error)
	GetPullRequest(ctx context.Context, orgID, teamID, repoID string, prNumber int) (*RemotePullRequest, error)
}

// RemotePullRequest represents an open pull request in the git platform.
type RemotePullRequest struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	SourceBranch string `json:"sourceBranch"`
	TargetBranch string `json:"targetBranch"`
	URL          string `json:"url"`
	State        string `json:"state"` // "open", "closed", "merged", "OPENED", "CLOSED", "MERGED"
}

// ActivePRMetadata tracks current mutation pull request details.
type ActivePRMetadata struct {
	PRNumber      int       `json:"prNumber"`
	PRURL         string    `json:"prUrl"`
	BranchName    string    `json:"branchName"`
	RepositoryID  string    `json:"repositoryId"`
	Title         string    `json:"title"`
	AuthorName    string    `json:"authorName"`
	AuthorEmail   string    `json:"authorEmail"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt"`
}

// PRService implements domain.CentralizedConfigPRService with active PR tracking and branch reuse.
type PRService struct {
	mu             sync.RWMutex
	gitClient      GitPlatformPRClient
	storage        ConfigStoragePort
	createdPRs     map[string]string                      // Key: orgID+teamID+branch -> PR URL
	activePRMeta   map[string]*ActivePRMetadata           // Key: orgID+teamID -> active PR metadata
	centralConfigs map[string]*domain.CentralizedConfigParameter // Key: orgID+teamID -> config parameter
}

// NewPRService constructs a new PRService instance.
func NewPRService(client GitPlatformPRClient) *PRService {
	return &PRService{
		gitClient:      client,
		createdPRs:     make(map[string]string),
		activePRMeta:   make(map[string]*ActivePRMetadata),
		centralConfigs: make(map[string]*domain.CentralizedConfigParameter),
	}
}

// SetStorage connects the config storage port.
func (s *PRService) SetStorage(storage ConfigStoragePort) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storage = storage
}

// SanitizeFileName normalizes a name into a safe file path segment.
func (s *PRService) SanitizeFileName(name, fallback string, maxLength int) string {
	if maxLength <= 0 {
		maxLength = 30
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	normalized = re.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if len(normalized) > maxLength {
		normalized = normalized[:maxLength]
	}
	if normalized == "" {
		if fallback != "" {
			return fallback
		}
		return "item"
	}
	return normalized
}

// BuildRuleFileName returns a canonical, deterministic filename for a Drixy rule.
func (s *PRService) BuildRuleFileName(title, uuid string) string {
	base := s.SanitizeFileName(title, "rule", 30)
	if uuid != "" {
		suffix := uuid
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		return fmt.Sprintf("%s-%s.yml", base, suffix)
	}
	return fmt.Sprintf("%s.yml", base)
}

// BuildCentralizedPath resolves the destination path in the centralized configuration repository.
func (s *PRService) BuildCentralizedPath(repoFolder, relativePath string) string {
	cleanRel := strings.TrimPrefix(relativePath, "/")
	if repoFolder == "" || repoFolder == "." || repoFolder == "global" {
		return cleanRel
	}
	return path.Join(repoFolder, cleanRel)
}

// BuildDirectoryGroupConfigPath constructs the path for a directory group config file.
func (s *PRService) BuildDirectoryGroupConfigPath(repoFolder, groupFolderName string) string {
	if repoFolder == "" || repoFolder == "global" {
		return path.Join(groupFolderName, "scandrix-config.yaml")
	}
	return path.Join(repoFolder, groupFolderName, "scandrix-config.yaml")
}

// BuildDirectoryGroupRulesPath constructs the path for a directory group rule file.
func (s *PRService) BuildDirectoryGroupRulesPath(repoFolder, groupFolderName, rulesDirectory, fileName string) string {
	if repoFolder == "" || repoFolder == "global" {
		return path.Join(groupFolderName, ".drixy-rules", rulesDirectory, fileName)
	}
	return path.Join(repoFolder, groupFolderName, ".drixy-rules", rulesDirectory, fileName)
}

// BuildScanDrixConfigRelativePath builds the relative path for scandrix-config.yaml within a directory scope.
func (s *PRService) BuildScanDrixConfigRelativePath(directoryPath string) string {
	clean := strings.Trim(directoryPath, "/")
	if clean == "" {
		return "scandrix-config.yaml"
	}
	return path.Join(clean, "scandrix-config.yaml")
}

// ResolveDirectoryGroupFolderName encodes one or more directory paths into a safe group folder name.
func (s *PRService) ResolveDirectoryGroupFolderName(directoryPaths []string) (string, error) {
	return utils.BuildGroupFolderName(directoryPaths)
}

// ExtractPullRequestNumber extracts the integer PR number from a standard pull request URL.
func (s *PRService) ExtractPullRequestNumber(prURL string) int {
	re := regexp.MustCompile(`/(?:pull|pull-requests|merge_requests)/(\d+)`)
	matches := re.FindStringSubmatch(prURL)
	if len(matches) > 1 {
		num, err := strconv.Atoi(matches[1])
		if err == nil {
			return num
		}
	}
	return 0
}

// SetCentralizedConfigParameter sets the team configuration parameter in memory.
func (s *PRService) SetCentralizedConfigParameter(orgID, teamID string, param *domain.CentralizedConfigParameter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	s.centralConfigs[key] = param
}

// GetCentralizedConfigParameter retrieves the team configuration parameter.
func (s *PRService) GetCentralizedConfigParameter(orgID, teamID string) *domain.CentralizedConfigParameter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	return s.centralConfigs[key]
}

// GetCentralizedConfigWithValidatedPullRequest returns centralized configuration with active PR state validated against git provider.
func (s *PRService) GetCentralizedConfigWithValidatedPullRequest(ctx context.Context, orgID, teamID string) (*domain.CentralizedConfigParameter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", orgID, teamID)
	cfg := s.centralConfigs[key]
	if cfg == nil || !cfg.Enabled || cfg.ActivePullRequest == nil {
		return cfg, nil
	}

	trackedPR := cfg.ActivePullRequest
	centralRepo := cfg.Repository
	if centralRepo == nil {
		return cfg, nil
	}

	// 1. Verify repository ID match
	if trackedPR.Repository.ID != "" && centralRepo.ID != "" &&
		strings.TrimSpace(trackedPR.Repository.ID) != strings.TrimSpace(centralRepo.ID) {
		cfg.ActivePullRequest = nil
		delete(s.activePRMeta, key)
		return cfg, nil
	}

	// 2. Extract and verify PR number
	prNumber := trackedPR.PRNumber
	if prNumber == 0 {
		prNumber = s.ExtractPullRequestNumber(trackedPR.PRURL)
	}
	if prNumber == 0 {
		cfg.ActivePullRequest = nil
		delete(s.activePRMeta, key)
		return cfg, nil
	}

	// 3. Query git client for PR status
	if s.gitClient != nil {
		remotePR, err := s.gitClient.GetPullRequest(ctx, orgID, teamID, centralRepo.ID, prNumber)
		if err != nil {
			// API failure resilience: return stale active pull request metadata without clearing
			return cfg, nil
		}

		if remotePR != nil {
			state := strings.ToUpper(remotePR.State)
			if state == "OPEN" || state == "OPENED" {
				return cfg, nil
			}

			// Closed or Merged: clear active PR metadata
			cfg.ActivePullRequest = nil
			delete(s.activePRMeta, key)
			return cfg, nil
		}
	}

	return cfg, nil
}

// PersistActivePullRequestMetadata records the active PR metadata in memory.
func (s *PRService) PersistActivePullRequestMetadata(orgID, teamID string, meta ActivePRMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	meta.LastUpdatedAt = time.Now().UTC()
	s.activePRMeta[key] = &meta

	if cfg, ok := s.centralConfigs[key]; ok && cfg != nil {
		cfg.ActivePullRequest = &domain.CentralizedConfigActivePullRequest{
			PRURL:        meta.PRURL,
			PRNumber:     meta.PRNumber,
			SourceBranch: meta.BranchName,
			TargetBranch: "main",
			Repository: domain.RepoRef{
				ID:   meta.RepositoryID,
				Name: meta.RepositoryID,
			},
			CreatedAt: meta.LastUpdatedAt,
			UpdatedAt: meta.LastUpdatedAt,
		}
	}
}

// GetActivePullRequestMetadata retrieves any recorded active PR metadata.
func (s *PRService) GetActivePullRequestMetadata(orgID, teamID string) *ActivePRMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	return s.activePRMeta[key]
}

// ClearActivePullRequestMetadata removes any tracked PR for the team.
func (s *PRService) ClearActivePullRequestMetadata(orgID, teamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	delete(s.activePRMeta, key)
	if cfg, ok := s.centralConfigs[key]; ok && cfg != nil {
		cfg.ActivePullRequest = nil
	}
}

// ClearActivePullRequestMetadataIfMatching clears the tracked PR only if the PR number matches.
func (s *PRService) ClearActivePullRequestMetadataIfMatching(orgID, teamID string, prNumber int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	current, exists := s.activePRMeta[key]
	if exists && current.PRNumber == prNumber {
		delete(s.activePRMeta, key)
		if cfg, ok := s.centralConfigs[key]; ok && cfg != nil {
			cfg.ActivePullRequest = nil
		}
		return true
	}
	return false
}

// ClearActivePullRequestMetadataIfMatchingRepoAndPR clears the tracked PR if both repo ID and PR number match.
func (s *PRService) ClearActivePullRequestMetadataIfMatchingRepoAndPR(orgID, teamID, repoID string, prNumber int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	current, exists := s.activePRMeta[key]
	if !exists {
		return false
	}

	normCurrRepo := strings.TrimSpace(current.RepositoryID)
	normTargetRepo := strings.TrimSpace(repoID)
	if normCurrRepo != "" && normTargetRepo != "" && normCurrRepo != normTargetRepo {
		return false
	}

	if prNumber != 0 && current.PRNumber != prNumber {
		return false
	}

	delete(s.activePRMeta, key)
	if cfg, ok := s.centralConfigs[key]; ok && cfg != nil {
		cfg.ActivePullRequest = nil
	}
	return true
}

// TryDiscoverAndReuseTrackedPullRequest searches for an existing open centralized PR on the repo and reuses it.
func (s *PRService) TryDiscoverAndReuseTrackedPullRequest(
	ctx context.Context,
	orgID, teamID, repoID string,
	mutations []domain.FileMutationOp,
	commitMessage, targetBranch string,
) (*domain.InitRepoResult, bool, error) {
	if s.gitClient == nil {
		return nil, false, nil
	}

	openPRs, err := s.gitClient.ListOpenPullRequests(ctx, orgID, teamID, repoID)
	if err != nil {
		return nil, false, err
	}

	var candidates []RemotePullRequest
	for _, pr := range openPRs {
		if strings.HasPrefix(pr.SourceBranch, "scandrix-centralized-") && pr.TargetBranch == targetBranch {
			candidates = append(candidates, pr)
		}
	}

	if len(candidates) != 1 {
		return nil, false, nil
	}

	matched := candidates[0]
	// Commit to the existing branch
	_, err = s.gitClient.CreateCommit(ctx, orgID, teamID, repoID, matched.SourceBranch, commitMessage, mutations)
	if err != nil {
		return nil, false, fmt.Errorf("failed committing to existing PR branch %q: %w", matched.SourceBranch, err)
	}

	s.PersistActivePullRequestMetadata(orgID, teamID, ActivePRMetadata{
		PRNumber:     matched.Number,
		PRURL:        matched.URL,
		BranchName:   matched.SourceBranch,
		RepositoryID: repoID,
		Title:        matched.Title,
	})

	return &domain.InitRepoResult{
		Success: true,
		PRURL:   matched.URL,
		Message: fmt.Sprintf("Reused existing pull request %s on branch %s", matched.URL, matched.SourceBranch),
	}, true, nil
}

// CreateMutationPR creates or updates a pull request with configuration mutations.
func (s *PRService) CreateMutationPR(
	ctx context.Context,
	orgID, teamID string,
	ops []domain.FileMutationOp,
	title, description string,
) (string, error) {
	if orgID == "" {
		return "", errors.New("prservice: organizationId is required")
	}
	if len(ops) == 0 {
		return "", errors.New("prservice: at least one mutation operation is required")
	}

	active := s.GetActivePullRequestMetadata(orgID, teamID)
	branchName := fmt.Sprintf("scandrix-centralized-update-%d", time.Now().Unix())
	if active != nil && active.BranchName != "" {
		branchName = active.BranchName
	}

	commitMsg := title
	if commitMsg == "" {
		commitMsg = "Update ScanDrix centralized configuration"
	}

	req := domain.CentralizedPRRequest{
		OrganizationID: orgID,
		TeamID:         teamID,
		BranchName:     branchName,
		CommitMessage:  commitMsg,
		Title:          title,
		Description:    description,
		Mutations:      ops,
	}

	res, err := s.CreateOrUpdatePR(ctx, req)
	if err != nil {
		return "", err
	}

	prNum := s.ExtractPullRequestNumber(res.PRURL)
	s.PersistActivePullRequestMetadata(orgID, teamID, ActivePRMetadata{
		PRNumber:   prNum,
		PRURL:      res.PRURL,
		BranchName: branchName,
		Title:      title,
	})

	return res.PRURL, nil
}

// CreateOrUpdatePR creates a branch, commits file mutations, and opens a pull request.
func (s *PRService) CreateOrUpdatePR(ctx context.Context, req domain.CentralizedPRRequest) (*domain.InitRepoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", req.OrganizationID, req.TeamID, req.BranchName)

	if s.gitClient != nil {
		targetBranch := "main"
		defBranch, err := s.gitClient.GetDefaultBranch(ctx, req.OrganizationID, req.TeamID, req.RepositoryID)
		if err == nil && defBranch != "" {
			targetBranch = defBranch
		}

		_ = s.gitClient.CreateBranch(ctx, req.OrganizationID, req.TeamID, req.RepositoryID, req.BranchName, targetBranch)

		_, err = s.gitClient.CreateCommit(ctx, req.OrganizationID, req.TeamID, req.RepositoryID, req.BranchName, req.CommitMessage, req.Mutations)
		if err != nil {
			return nil, fmt.Errorf("failed creating commit on branch %q: %w", req.BranchName, err)
		}

		title := req.Title
		if title == "" {
			title = "Update ScanDrix Centralized Configuration"
		}
		desc := req.Description
		if desc == "" {
			desc = "Automated configuration updates from ScanDrix dashboard."
		}

		prURL, err := s.gitClient.OpenPullRequest(ctx, req.OrganizationID, req.TeamID, req.RepositoryID, title, desc, req.BranchName, targetBranch)
		if err != nil {
			// Pull request might already exist for this branch
			if existingURL, exists := s.createdPRs[key]; exists {
				return &domain.InitRepoResult{
					Success: true,
					PRURL:   existingURL,
					Message: fmt.Sprintf("Updated existing configuration branch %q", req.BranchName),
				}, nil
			}
			return nil, fmt.Errorf("failed opening pull request: %w", err)
		}

		s.createdPRs[key] = prURL
		return &domain.InitRepoResult{
			Success: true,
			PRURL:   prURL,
			Message: fmt.Sprintf("Opened pull request %s", prURL),
		}, nil
	}

	// Fallback execution when git client is nil
	prURL := fmt.Sprintf("https://github.com/scandrix/config/pull/%d", time.Now().Unix()%10000)
	s.createdPRs[key] = prURL
	return &domain.InitRepoResult{
		Success: true,
		PRURL:   prURL,
		Message: "Simulated PR creation successful",
	}, nil
}

// GetScopedScanDrixConfigFileContent retrieves the configuration content for a specific repository and directory.
func (s *PRService) GetScopedScanDrixConfigFileContent(
	ctx context.Context,
	orgID, teamID, centralRepoID string,
	repoFolderName string,
	directoryPaths []string,
) (string, error) {
	relPath := "scandrix-config.yaml"
	if repoFolderName != "" {
		if len(directoryPaths) > 0 {
			groupFolder, err := utils.BuildGroupFolderName(directoryPaths)
			if err != nil {
				return "", err
			}
			relPath = path.Join(repoFolderName, groupFolder, "scandrix-config.yaml")
		} else {
			relPath = path.Join(repoFolderName, "scandrix-config.yaml")
		}
	}

	if s.gitClient != nil {
		bytes, err := s.gitClient.GetFileContent(ctx, orgID, teamID, centralRepoID, relPath, "main")
		if err == nil && len(bytes) > 0 {
			return string(bytes), nil
		}
	}

	// Fallback to default
	def := commonCodeReview.GetDefaultScanDrixConfigFile()
	return utils.DumpCentralizedYAML(def)
}

// HandleTrackedPullRequestClose handles PR merge/close lifecycle events and triggers stale cleanup.
func (s *PRService) HandleTrackedPullRequestClose(
	ctx context.Context,
	orgID, teamID string,
	prNumber int,
	merged bool,
) error {
	s.ClearActivePullRequestMetadataIfMatching(orgID, teamID, prNumber)
	return nil
}
