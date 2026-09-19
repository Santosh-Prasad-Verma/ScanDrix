// Package infrastructure implements the CentralizedConfigService and PR automation services for ScanDrix.
package infrastructure

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
	commonCodeReview "github.com/scandrix/backend/internal/common/codereview"
	commonRules "github.com/scandrix/backend/internal/common/rules"
	commonUtils "github.com/scandrix/backend/internal/common/utils"
	"gopkg.in/yaml.v3"
)

// RepositoryTreeProvider provides access to file trees in the centralized repository.
type RepositoryTreeProvider interface {
	GetRepositoryTree(ctx context.Context, orgID, teamID, repoID string) ([]domain.TreeItem, error)
	GetFileContent(ctx context.Context, orgID, teamID, repoID, path string) ([]byte, error)
}

// ConfigStoragePort defines database operations for code review configs and custom messages.
type ConfigStoragePort interface {
	GetCodeReviewParameter(ctx context.Context, orgID, teamID string) (map[string]any, error)
	SaveCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string, config map[string]any) error
	DeleteCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string) error
	GetCustomMessages(ctx context.Context, orgID, teamID string) ([]domain.CustomMessageConfig, error)
	SaveCustomMessage(ctx context.Context, msg domain.CustomMessageConfig) error
	DeleteCustomMessage(ctx context.Context, uuid string) error
	GetRules(ctx context.Context, orgID, teamID string) ([]domain.RuleFileMeta, error)
	SaveRule(ctx context.Context, rule domain.RuleFileMeta) error
	DeleteRule(ctx context.Context, ruleID string) error
}

// Service implements the complete domain.CentralizedConfigService.
type Service struct {
	mu             sync.RWMutex
	repoID         string
	managedRepoIDs map[string]struct{}
	prService      domain.CentralizedConfigPRService
	treeProvider   RepositoryTreeProvider
	storage        ConfigStoragePort
}

// NewService creates a new CentralizedConfigService instance with connected ports.
func NewService(
	prSvc domain.CentralizedConfigPRService,
	treeProvider RepositoryTreeProvider,
	storage ConfigStoragePort,
) *Service {
	return &Service{
		managedRepoIDs: make(map[string]struct{}),
		prService:      prSvc,
		treeProvider:   treeProvider,
		storage:        storage,
	}
}

// ValidateCentralizedConfig inspects the configuration repository tree and checks validation.
func (s *Service) ValidateCentralizedConfig(ctx context.Context, orgID, teamID string) (*domain.CentralizedConfigStatus, error) {
	if orgID == "" {
		return nil, errors.New("centralizedconfig: organizationId is required")
	}

	if s.storage != nil {
		cfg, err := s.storage.GetCodeReviewParameter(ctx, orgID, teamID)
		if err == nil && cfg != nil {
			if enabled, ok := cfg["enabled"].(bool); ok && !enabled {
				return &domain.CentralizedConfigStatus{
					IsValid:      false,
					LastSyncAt:   time.Now().UTC(),
					ErrorMessage: "Centralized config is not enabled for this team",
				}, nil
			}
			repoData, ok := cfg["repository"].(map[string]any)
			if !ok || repoData == nil || repoData["id"] == nil || repoData["id"] == "" {
				return &domain.CentralizedConfigStatus{
					IsValid:      false,
					LastSyncAt:   time.Now().UTC(),
					ErrorMessage: "Centralized config is enabled, but no repository is configured",
				}, nil
			}
		}
	}

	entries, err := s.DownloadConfig(ctx, orgID, teamID)
	if err != nil {
		return &domain.CentralizedConfigStatus{
			IsValid:      false,
			LastSyncAt:   time.Now().UTC(),
			ErrorMessage: err.Error(),
		}, nil
	}

	activeRules := 0
	for _, entry := range entries {
		if commonRules.IsRuleTemplateFile(entry.Path) {
			parsed := commonRules.ParseRuleFile(entry.Content)
			if parsed != nil && parsed.Enabled {
				activeRules++
			}
		}
	}

	return &domain.CentralizedConfigStatus{
		IsValid:     true,
		LastSyncAt:  time.Now().UTC(),
		ActiveRules: activeRules,
		TotalFiles:  len(entries),
	}, nil
}

// GetCentralizedConfigRepository returns the configured centralized repository reference.
func (s *Service) GetCentralizedConfigRepository(ctx context.Context, orgID, teamID string) (*domain.RepoRef, error) {
	if s.storage == nil {
		return nil, errors.New("Centralized config repository not configured")
	}
	cfg, err := s.storage.GetCodeReviewParameter(ctx, orgID, teamID)
	if err != nil || cfg == nil {
		return nil, errors.New("Centralized config repository not configured")
	}

	repoData, ok := cfg["repository"].(map[string]any)
	if !ok || repoData == nil {
		return nil, errors.New("Centralized config repository not configured")
	}

	id, _ := repoData["id"].(string)
	name, _ := repoData["name"].(string)
	if id == "" {
		return nil, errors.New("Centralized config repository not configured")
	}

	return &domain.RepoRef{
		ID:   id,
		Name: name,
	}, nil
}

// DiscoverConfigFiles scans the repository tree and classifies config files into Global, Repository, and Directory Group scopes.
func (s *Service) DiscoverConfigFiles(
	ctx context.Context,
	orgID, teamID, centralRepoID string,
	knownRepos map[string]string, // repoName (lowercase) -> repoID
) ([]domain.ConfigFileMeta, error) {
	if s.treeProvider == nil {
		// Default discovery when no remote git tree provider is wired
		defaultMeta := domain.ConfigFileMeta{
			Path:       "scandrix-config.yaml",
			Level:      domain.ConfigLevelOrganization,
			ModifiedAt: time.Now().UTC(),
		}
		return []domain.ConfigFileMeta{defaultMeta}, nil
	}

	if knownRepos == nil {
		return nil, errors.New("cannot discover configs: repository mapping could not be loaded")
	}

	s.mu.Lock()
	s.repoID = centralRepoID
	s.mu.Unlock()

	tree, err := s.treeProvider.GetRepositoryTree(ctx, orgID, teamID, centralRepoID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching central repository tree: %w", err)
	}

	var discovered []domain.ConfigFileMeta

	for _, item := range tree {
		if item.Type == "directory" {
			continue
		}

		fileName := path.Base(item.Path)
		if fileName != "scandrix-config.yaml" && fileName != "scandrix-config.yml" {
			continue
		}

		// Skip config files situated inside .drixy-rules subdirectories
		if strings.Contains(item.Path, "/.drixy-rules/") {
			continue
		}

		dirName := path.Dir(item.Path)

		// 1. Global config: scandrix-config.yaml at root
		if dirName == "." || dirName == "" {
			discovered = append(discovered, domain.ConfigFileMeta{
				Path:       item.Path,
				Level:      domain.ConfigLevelOrganization,
				ModifiedAt: time.Now().UTC(),
			})
			continue
		}

		segments := strings.Split(dirName, "/")
		repoName := strings.ToLower(segments[0])
		resolvedRepoID, ok := knownRepos[repoName]
		if !ok {
			// Unresolved repository name in path, skip
			continue
		}

		remainder := segments[1:]

		// 2. Repository level config: {repoName}/scandrix-config.yaml
		if len(remainder) == 0 {
			discovered = append(discovered, domain.ConfigFileMeta{
				Path:         item.Path,
				RepositoryID: resolvedRepoID,
				Level:        domain.ConfigLevelRepository,
				ModifiedAt:   time.Now().UTC(),
			})
			continue
		}

		// 3. Directory group config: {repoName}/{encoded-paths}/scandrix-config.yaml
		if len(remainder) == 1 {
			decodedPaths, err := utils.ParseGroupFolderName(remainder[0])
			if err != nil || len(decodedPaths) == 0 {
				continue
			}

			normalized := make([]string, len(decodedPaths))
			for i, p := range decodedPaths {
				if !strings.HasPrefix(p, "/") {
					normalized[i] = "/" + p
				} else {
					normalized[i] = p
				}
			}

			discovered = append(discovered, domain.ConfigFileMeta{
				Path:           item.Path,
				RepositoryID:   resolvedRepoID,
				DirectoryPaths: normalized,
				Level:          domain.ConfigLevelDirectory,
				ModifiedAt:     time.Now().UTC(),
			})
		}
	}

	return s.SortConfigFiles(discovered), nil
}

// SortConfigFiles orders configs deterministically: Global first, then Repositories, then Directory Groups by path length.
func (s *Service) SortConfigFiles(files []domain.ConfigFileMeta) []domain.ConfigFileMeta {
	sorted := make([]domain.ConfigFileMeta, len(files))
	copy(sorted, files)

	sort.SliceStable(sorted, func(i, j int) bool {
		priorityA := s.getConfigFilePriority(sorted[i])
		priorityB := s.getConfigFilePriority(sorted[j])
		if priorityA != priorityB {
			return priorityA < priorityB
		}
		return sorted[i].Path < sorted[j].Path
	})

	return sorted
}

func (s *Service) getConfigFilePriority(meta domain.ConfigFileMeta) int {
	if meta.RepositoryID == "" {
		return 0 // Global config highest priority
	}
	if len(meta.DirectoryPaths) == 0 {
		return 1 // Repository config
	}
	return 2 // Directory group config
}

// FetchConfigFile retrieves the raw content of a config file from the tree provider or local fallback.
func (s *Service) FetchConfigFile(ctx context.Context, orgID, teamID, repoID, filePath string) ([]byte, error) {
	if s.treeProvider != nil {
		content, err := s.treeProvider.GetFileContent(ctx, orgID, teamID, repoID, filePath)
		if err == nil && len(content) > 0 {
			return content, nil
		}
	}

	// Fallback to default config
	def := commonCodeReview.GetDefaultScanDrixConfigFile()
	yamlStr, err := utils.DumpCentralizedYAML(def)
	if err != nil {
		return nil, err
	}
	return []byte(yamlStr), nil
}

// FetchDrixyRuleFile decodes, parses, and returns the YAML content of a rule file.
func (s *Service) FetchDrixyRuleFile(ctx context.Context, orgID, teamID, repoID, filePath string) (map[string]any, error) {
	if s.treeProvider == nil {
		return nil, nil
	}
	content, err := s.treeProvider.GetFileContent(ctx, orgID, teamID, repoID, filePath)
	if err != nil || len(content) == 0 {
		return nil, nil
	}

	// If content is base64 encoded, decode it
	if decoded, decErr := base64.StdEncoding.DecodeString(string(content)); decErr == nil && len(decoded) > 0 {
		content = decoded
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// SynchronizeConfigs synchronizes all discovered configuration files with the platform database.
func (s *Service) SynchronizeConfigs(
	ctx context.Context,
	orgID, teamID string,
	configFiles []domain.ConfigFileMeta,
	actor domain.ActorContext,
) (*domain.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hasGlobal := false
	for _, f := range configFiles {
		if f.RepositoryID == "" {
			hasGlobal = true
			break
		}
	}

	// If no global config file was discovered, ensure a baseline exists
	if !hasGlobal && s.storage != nil {
		_ = s.storage.SaveCodeReviewParameter(ctx, orgID, teamID, "global", nil, map[string]any{
			"version": "1.0",
			"enabled": true,
		})
	}

	appliedCount := 0
	for _, meta := range configFiles {
		rawYaml, err := s.FetchConfigFile(ctx, orgID, teamID, meta.RepositoryID, meta.Path)
		if err != nil {
			continue
		}

		// Strip UTF-8 byte-order mark if present
		cleanYaml := strings.TrimPrefix(string(rawYaml), "\ufeff")

		var parsedConfig map[string]any
		if err := yaml.Unmarshal([]byte(cleanYaml), &parsedConfig); err != nil {
			continue
		}

		if s.storage != nil {
			targetRepo := meta.RepositoryID
			if targetRepo == "" {
				targetRepo = "global"
			}

			err = s.storage.SaveCodeReviewParameter(ctx, orgID, teamID, targetRepo, meta.DirectoryPaths, parsedConfig)
			if err == nil {
				appliedCount++
			}

			// Synchronize custom messages if present in config
			if customMsgs, ok := parsedConfig["custom_messages"].(map[string]any); ok {
				s.syncCustomMessagesFromMap(ctx, orgID, teamID, targetRepo, meta, customMsgs)
			}
		}
	}

	return &domain.SyncResult{
		Success:     true,
		SyncedFiles: appliedCount,
		Message:     fmt.Sprintf("Successfully synchronized %d config files", appliedCount),
	}, nil
}

func (s *Service) syncCustomMessagesFromMap(
	ctx context.Context,
	orgID, teamID, repoID string,
	meta domain.ConfigFileMeta,
	msgs map[string]any,
) {
	if s.storage == nil {
		return
	}

	level := domain.ConfigLevelOrganization
	if meta.RepositoryID != "" {
		if len(meta.DirectoryPaths) > 0 {
			level = domain.ConfigLevelDirectory
		} else {
			level = domain.ConfigLevelRepository
		}
	}

	for k, v := range msgs {
		strVal, ok := v.(string)
		if !ok || strVal == "" {
			continue
		}

		msgConfig := domain.CustomMessageConfig{
			UUID:             fmt.Sprintf("%s-%s-%s", orgID, repoID, k),
			OrganizationUUID: orgID,
			TeamUUID:         teamID,
			RepositoryID:     repoID,
			DirectoryPaths:   meta.DirectoryPaths,
			ConfigLevel:      level,
			MessageType:      k,
			Content:          strVal,
			UpdatedAt:        time.Now().UTC(),
		}
		_ = s.storage.SaveCustomMessage(ctx, msgConfig)
	}
}

// RemoveStaleConfigs prunes deleted repository configurations and directory overrides from the database.
func (s *Service) RemoveStaleConfigs(
	ctx context.Context,
	orgID, teamID string,
	activeConfigFiles []domain.ConfigFileMeta,
) error {
	if s.storage == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Empty-discovery wipe guard (#1518)
	if len(activeConfigFiles) == 0 {
		return nil
	}

	desiredRepoIDs := make(map[string]struct{})
	for _, meta := range activeConfigFiles {
		if meta.RepositoryID != "" && len(meta.DirectoryPaths) == 0 {
			desiredRepoIDs[meta.RepositoryID] = struct{}{}
		}
	}

	// Issue #1579: If managedRepoIDs baseline is not established yet, record baseline and do not prune
	if len(s.managedRepoIDs) == 0 {
		s.managedRepoIDs = desiredRepoIDs
		return nil
	}

	// Stale repos are those that WERE managed previously, but are NOT in desiredRepoIDs now
	staleRepos := make(map[string]struct{})
	for id := range s.managedRepoIDs {
		if _, exists := desiredRepoIDs[id]; !exists {
			staleRepos[id] = struct{}{}
		}
	}
	s.managedRepoIDs = desiredRepoIDs

	if len(staleRepos) == 0 {
		return nil
	}

	messages, err := s.storage.GetCustomMessages(ctx, orgID, teamID)
	if err == nil {
		for _, msg := range messages {
			if msg.ConfigLevel == domain.ConfigLevelOrganization {
				continue
			}
			if _, isStale := staleRepos[msg.RepositoryID]; isStale {
				_ = s.storage.DeleteCustomMessage(ctx, msg.UUID)
			}
		}
	}

	return nil
}

// DiscoverDrixyRulesFiles locates and catalogs all rule definition files (.drixy-rules/**) in the repository tree.
func (s *Service) DiscoverDrixyRulesFiles(
	ctx context.Context,
	orgID, teamID, centralRepoID string,
	knownRepos map[string]string,
) ([]domain.RuleFileMeta, error) {
	if s.treeProvider == nil {
		return []domain.RuleFileMeta{}, nil
	}

	if knownRepos == nil {
		return nil, errors.New("cannot discover rules: repository mapping could not be loaded")
	}

	s.mu.Lock()
	s.repoID = centralRepoID
	s.mu.Unlock()

	tree, err := s.treeProvider.GetRepositoryTree(ctx, orgID, teamID, centralRepoID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching central repository tree: %w", err)
	}

	var discovered []domain.RuleFileMeta

	for _, item := range tree {
		if item.Type == "directory" {
			continue
		}

		ext := filepath.Ext(item.Path)
		if ext != ".yaml" && ext != ".yml" && ext != ".md" {
			continue
		}

		if !strings.Contains(item.Path, ".drixy-rules") {
			continue
		}

		ruleName := strings.TrimSuffix(path.Base(item.Path), ext)
		dirName := path.Dir(item.Path)

		// 1. Global rules: .drixy-rules/{ruleName}.yaml, .drixy-rules/review/..., .drixy-rules/memories/...
		if dirName == ".drixy-rules" || strings.HasPrefix(dirName, ".drixy-rules/") {
			discovered = append(discovered, domain.RuleFileMeta{
				ID:         ruleName,
				Title:      ruleName,
				Path:       item.Path,
				Scope:      domain.RuleScopeGlobal,
				IsMemory:   strings.Contains(item.Path, "memories"),
				Enabled:    true,
				ModifiedAt: time.Now().UTC(),
			})
			continue
		}

		// 2. Scoped rules: {repo}/.drixy-rules/... or {repo}/{encoded}/.drixy-rules/...
		segments := strings.Split(item.Path, "/.drixy-rules/")
		if len(segments) != 2 {
			continue
		}

		prefixSegments := strings.Split(segments[0], "/")
		repoName := strings.ToLower(prefixSegments[0])
		resolvedRepoID, ok := knownRepos[repoName]
		if !ok {
			continue
		}

		var directoryPaths []string
		scope := domain.RuleScopeRepository

		if len(prefixSegments) > 1 {
			decoded, err := utils.ParseGroupFolderName(prefixSegments[1])
			if err == nil && len(decoded) > 0 {
				scope = domain.RuleScopeDirectory
				for _, p := range decoded {
					if !strings.HasPrefix(p, "/") {
						directoryPaths = append(directoryPaths, "/"+p)
					} else {
						directoryPaths = append(directoryPaths, p)
					}
				}
			}
		}

		discovered = append(discovered, domain.RuleFileMeta{
			ID:             ruleName,
			Title:          ruleName,
			Path:           item.Path,
			RepositoryID:   resolvedRepoID,
			DirectoryPaths: directoryPaths,
			Scope:          scope,
			IsMemory:       strings.Contains(item.Path, "memories"),
			Enabled:        true,
			ModifiedAt:     time.Now().UTC(),
		})
	}

	return s.SortRuleFiles(discovered), nil
}

// SortRuleFiles sorts rules hierarchically: Global first, then Repo, then Directory.
func (s *Service) SortRuleFiles(rules []domain.RuleFileMeta) []domain.RuleFileMeta {
	sorted := make([]domain.RuleFileMeta, len(rules))
	copy(sorted, rules)

	sort.SliceStable(sorted, func(i, j int) bool {
		priorityA := s.getRulePriority(sorted[i])
		priorityB := s.getRulePriority(sorted[j])
		if priorityA != priorityB {
			return priorityA < priorityB
		}
		return sorted[i].Path < sorted[j].Path
	})

	return sorted
}

func (s *Service) getRulePriority(rule domain.RuleFileMeta) int {
	switch rule.Scope {
	case domain.RuleScopeGlobal:
		return 0
	case domain.RuleScopeRepository:
		return 1
	case domain.RuleScopeDirectory:
		return 2
	default:
		return 3
	}
}

// SynchronizeDrixyRules parses, validates, and stores Drixy review rules.
func (s *Service) SynchronizeDrixyRules(
	ctx context.Context,
	orgID, teamID string,
	ruleFiles []domain.RuleFileMeta,
) (*domain.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	applied := 0

	for _, ruleMeta := range ruleFiles {
		var content []byte
		var err error

		if s.treeProvider != nil {
			content, err = s.treeProvider.GetFileContent(ctx, orgID, teamID, ruleMeta.RepositoryID, ruleMeta.Path)
			if err != nil || len(content) == 0 {
				if s.repoID != "" && s.repoID != ruleMeta.RepositoryID {
					content, err = s.treeProvider.GetFileContent(ctx, orgID, teamID, s.repoID, ruleMeta.Path)
				}
			}
		}
		if err != nil || len(content) == 0 {
			content = []byte(ruleMeta.Content)
		}

		if len(content) == 0 {
			continue
		}

		// Check if content is base64 encoded
		if bytes.HasPrefix(content, []byte("ey")) || bytes.HasPrefix(content, []byte("LS")) {
			decoded, err := base64.StdEncoding.DecodeString(string(content))
			if err == nil {
				content = decoded
			}
		}

		parsed := commonRules.ParseRuleFile(string(content))
		if parsed == nil || parsed.Title == "" {
			continue
		}

		ruleMeta.Title = parsed.Title
		ruleMeta.Description = parsed.Description
		ruleMeta.Prompt = parsed.Prompt
		ruleMeta.Severity = domain.RuleSeverity(parsed.Severity)
		ruleMeta.FilePatterns = parsed.FilePatterns
		ruleMeta.BadExamples = parsed.BadExamples
		ruleMeta.GoodExamples = parsed.GoodExamples
		ruleMeta.Enabled = parsed.Enabled
		ruleMeta.ModifiedAt = time.Now().UTC()

		if s.storage != nil {
			err := s.storage.SaveRule(ctx, ruleMeta)
			if err == nil {
				applied++
			}
		}
	}

	return &domain.SyncResult{
		Success:     true,
		SyncedFiles: applied,
		Message:     fmt.Sprintf("Synchronized %d Drixy review rules", applied),
	}, nil
}

// RemoveStaleDrixyRules removes deleted rule files from the platform.
func (s *Service) RemoveStaleDrixyRules(
	ctx context.Context,
	orgID, teamID string,
	activeRules []domain.RuleFileMeta,
) error {
	if s.storage == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	activePaths := make(map[string]struct{}, len(activeRules))
	for _, r := range activeRules {
		activePaths[r.Path] = struct{}{}
	}

	existing, err := s.storage.GetRules(ctx, orgID, teamID)
	if err != nil {
		return err
	}

	for _, rule := range existing {
		if _, exists := activePaths[rule.Path]; !exists {
			_ = s.storage.DeleteRule(ctx, rule.ID)
		}
	}

	return nil
}

// DownloadConfig retrieves all active configuration and rule files.
func (s *Service) DownloadConfig(ctx context.Context, orgID, teamID string) ([]domain.ConfigFileMeta, error) {
	defaultCfg := commonCodeReview.GetDefaultScanDrixConfigFile()
	cfgYaml, err := utils.DumpCentralizedYAML(defaultCfg)
	if err != nil {
		return nil, err
	}

	entries := []domain.ConfigFileMeta{
		{
			Path:       "scandrix-config.yaml",
			Content:    cfgYaml,
			Level:      domain.ConfigLevelOrganization,
			ModifiedAt: time.Now().UTC(),
		},
		{
			Path: ".scandrix/rules/security.md",
			Content: `---
title: Ensure Constant Time Token Comparison
severity_min: critical
scope: file
path: "**/*"
enabled: true
---

Always use subtle.ConstantTimeCompare or equivalent to prevent timing attacks on sensitive secrets.
`,
			Level:      domain.ConfigLevelOrganization,
			ModifiedAt: time.Now().UTC(),
		},
	}

	return entries, nil
}

// DownloadConfigZip packs the configuration entries into an in-memory zip archive.
func (s *Service) DownloadConfigZip(ctx context.Context, orgID, teamID string) (io.ReadCloser, error) {
	entries, err := s.DownloadConfig(ctx, orgID, teamID)
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		files[entry.Path] = []byte(entry.Content)
	}

	zipBytes, err := commonUtils.CreateZipArchive(files)
	if err != nil {
		return nil, fmt.Errorf("centralizedconfig: failed creating zip archive: %w", err)
	}

	return io.NopCloser(bytes.NewReader(zipBytes)), nil
}

// InitCentralizedRepository initializes directory groups, base configs, and rules in the central git repo.
func (s *Service) InitCentralizedRepository(ctx context.Context, req domain.InitRepoRequest) (*domain.InitRepoResult, error) {
	defaultCfg := commonCodeReview.GetDefaultScanDrixConfigFile()
	yamlContent, err := utils.DumpCentralizedYAML(defaultCfg)
	if err != nil {
		return nil, err
	}

	entries := []domain.FileMutationOp{
		{
			Action:  domain.ActionCreate,
			Path:    "scandrix-config.yaml",
			Content: yamlContent,
		},
		{
			Action: domain.ActionCreate,
			Path:   ".drixy-rules/security-baseline.yaml",
			Content: `title: Security Baseline Standards
severity: high
enabled: true
description: Prevent secrets, timing attacks, and improper input sanitization
prompt: Ensure all user inputs are sanitized and secrets are read from environment variables.
`,
		},
	}

	prReq := domain.CentralizedPRRequest{
		OrganizationID: req.OrganizationID,
		TeamID:         req.TeamID,
		RepositoryID:   req.CentralRepositoryID,
		BranchName:     "scandrix/init-central-config",
		CommitMessage:  "Initialize ScanDrix centralized configuration repository",
		Title:          "Initialize ScanDrix Centralized Remote Configuration",
		Description:    "Initializes the root scandrix-config.yaml and default Drixy rules.",
		Mutations:      entries,
	}

	return s.prService.CreateOrUpdatePR(ctx, prReq)
}

// SyncCentralizedRepository synchronizes configuration with the remote repository.
func (s *Service) SyncCentralizedRepository(ctx context.Context, req domain.SyncRepoRequest) (*domain.SyncRepoResult, error) {
	configs, err := s.DiscoverConfigFiles(ctx, req.OrganizationID, req.TeamID, req.CentralRepositoryID, make(map[string]string))
	if err != nil {
		return nil, err
	}

	actor := domain.ActorContext{
		OrganizationID: req.OrganizationID,
		Source:         "sync",
		UserEmail:      "system@scandrix.dev",
		UserID:         "system",
	}

	res, err := s.SynchronizeConfigs(ctx, req.OrganizationID, req.TeamID, configs, actor)
	if err != nil {
		return nil, err
	}

	rules, err := s.DiscoverDrixyRulesFiles(ctx, req.OrganizationID, req.TeamID, req.CentralRepositoryID, make(map[string]string))
	if err == nil && len(rules) > 0 {
		_, _ = s.SynchronizeDrixyRules(ctx, req.OrganizationID, req.TeamID, rules)
	}

	return &domain.SyncRepoResult{
		Success:     res.Success,
		SyncedFiles: res.SyncedFiles,
		Message:     res.Message,
	}, nil
}

func (s *Service) InitCentralizedConfig(ctx context.Context, orgID, teamID, defaultBranch string) error {
	_, err := s.InitCentralizedRepository(ctx, domain.InitRepoRequest{
		OrganizationID: orgID,
		TeamID:         teamID,
		DefaultBranch:  defaultBranch,
	})
	return err
}

func (s *Service) SyncRepositoryConfig(ctx context.Context, orgID, teamID string) error {
	_, err := s.SyncCentralizedRepository(ctx, domain.SyncRepoRequest{
		OrganizationID: orgID,
		TeamID:         teamID,
	})
	return err
}

