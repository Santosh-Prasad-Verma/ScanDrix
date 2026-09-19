package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
)

type mockTreeProvider struct {
	mu       sync.RWMutex
	trees    map[string][]domain.TreeItem
	contents map[string][]byte
	treeErr  error
}

func newMockTreeProvider() *mockTreeProvider {
	return &mockTreeProvider{
		trees:    make(map[string][]domain.TreeItem),
		contents: make(map[string][]byte),
	}
}

func (m *mockTreeProvider) GetRepositoryTree(ctx context.Context, orgID, teamID, repoID string) ([]domain.TreeItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.treeErr != nil {
		return nil, m.treeErr
	}
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	return m.trees[key], nil
}

func (m *mockTreeProvider) GetFileContent(ctx context.Context, orgID, teamID, repoID, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s:%s", orgID, teamID, repoID, path)
	content, ok := m.contents[key]
	if !ok {
		return nil, errors.New("file not found")
	}
	return content, nil
}

type mockStoragePort struct {
	mu             sync.RWMutex
	codeReviewCfg  map[string]map[string]any
	customMessages map[string]domain.CustomMessageConfig
	rules          map[string]domain.RuleFileMeta
}

func newMockStoragePort() *mockStoragePort {
	return &mockStoragePort{
		codeReviewCfg:  make(map[string]map[string]any),
		customMessages: make(map[string]domain.CustomMessageConfig),
		rules:          make(map[string]domain.RuleFileMeta),
	}
}

func (m *mockStoragePort) GetCodeReviewParameter(ctx context.Context, orgID, teamID string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	cfg, ok := m.codeReviewCfg[key]
	if !ok {
		return map[string]any{"enabled": true}, nil
	}
	return cfg, nil
}

func (m *mockStoragePort) SaveCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string, config map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	m.codeReviewCfg[key] = config
	return nil
}

func (m *mockStoragePort) DeleteCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	delete(m.codeReviewCfg, key)
	return nil
}

func (m *mockStoragePort) GetCustomMessages(ctx context.Context, orgID, teamID string) ([]domain.CustomMessageConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []domain.CustomMessageConfig
	for _, msg := range m.customMessages {
		if msg.OrganizationUUID == orgID {
			list = append(list, msg)
		}
	}
	return list, nil
}

func (m *mockStoragePort) SaveCustomMessage(ctx context.Context, msg domain.CustomMessageConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customMessages[msg.UUID] = msg
	return nil
}

func (m *mockStoragePort) DeleteCustomMessage(ctx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.customMessages, uuid)
	return nil
}

func (m *mockStoragePort) GetRules(ctx context.Context, orgID, teamID string) ([]domain.RuleFileMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []domain.RuleFileMeta
	for _, r := range m.rules {
		list = append(list, r)
	}
	return list, nil
}

func (m *mockStoragePort) SaveRule(ctx context.Context, rule domain.RuleFileMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := rule.ID
	if id == "" {
		id = rule.UUID
	}
	m.rules[id] = rule
	return nil
}

func (m *mockStoragePort) DeleteRule(ctx context.Context, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, ruleID)
	return nil
}

func TestCentralizedConfigService_DiscoverConfigFiles(t *testing.T) {
	tree := newMockTreeProvider()
	storage := newMockStoragePort()
	svc := NewService(nil, tree, storage)

	ctx := context.Background()
	orgID := "org-123"
	teamID := "team-456"
	repoID := "central-repo"

	groupFolder, _ := utils.BuildGroupFolderName([]string{"src/api", "app/models"})

	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	tree.trees[key] = []domain.TreeItem{
		{Path: "scandrix-config.yml", Type: "blob"},
		{Path: "repo-1/scandrix-config.yml", Type: "blob"},
		{Path: fmt.Sprintf("repo-1/%s/scandrix-config.yml", groupFolder), Type: "blob"},
		{Path: "repo-1/.drixy-rules/review/security.yml", Type: "blob"}, // Should not be discovered as config
	}

	knownRepos := map[string]string{"repo-1": "repo-1"}

	configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err != nil {
		t.Fatalf("unexpected error discovering config files: %v", err)
	}

	if len(configs) != 3 {
		t.Fatalf("expected 3 config files discovered, got %d", len(configs))
	}

	// First should be organization (global) config
	if configs[0].Level != domain.ConfigLevelOrganization {
		t.Fatalf("expected first config to be Organization level, got %s", configs[0].Level)
	}

	// Second should be repository config
	if configs[1].Level != domain.ConfigLevelRepository || configs[1].RepositoryID != "repo-1" {
		t.Fatalf("expected second config to be Repository level for repo-1, got %+v", configs[1])
	}

	// Third should be directory group config
	if configs[2].Level != domain.ConfigLevelDirectory {
		t.Fatalf("expected third config to be Directory level, got %+v", configs[2])
	}
	if len(configs[2].DirectoryPaths) != 2 {
		t.Fatalf("expected 2 directory paths in group config, got %d", len(configs[2].DirectoryPaths))
	}
}

func TestCentralizedConfigService_EmptyDiscoveryWipeGuard(t *testing.T) {
	tree := newMockTreeProvider()
	storage := newMockStoragePort()
	svc := NewService(nil, tree, storage)

	ctx := context.Background()
	orgID := "org-123"
	teamID := "team-456"
	repoID := "central-repo"

	// Seed existing rules and configs in database
	storage.rules["rule-1"] = domain.RuleFileMeta{
		ID:           "rule-1",
		UUID:         "rule-1-uuid",
		Title:        "Existing Synced Rule",
		Path:         "repo-1/.drixy-rules/sec.yml",
		RepositoryID: "repo-1",
		Enabled:      true,
	}

	// 1. When tree read fails (error returned), discovery must return error, NOT empty slice
	tree.treeErr = errors.New("git provider rate limit")
	knownRepos := map[string]string{"repo-1": "repo-1"}
	_, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err == nil {
		t.Fatalf("expected error when git tree read fails")
	}

	_, err = svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err == nil {
		t.Fatalf("expected error when git tree read fails for rules")
	}

	// Rule must NOT be deleted because discovery errored
	if _, exists := storage.rules["rule-1"]; !exists {
		t.Fatalf("wipe guard failed: rule was deleted when git read errored")
	}
}

func TestCentralizedConfigService_SynchronizeAndStaleCleanup(t *testing.T) {
	tree := newMockTreeProvider()
	storage := newMockStoragePort()
	svc := NewService(nil, tree, storage)

	ctx := context.Background()
	orgID := "org-123"
	teamID := "team-456"
	repoID := "central-repo"

	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	tree.trees[key] = []domain.TreeItem{
		{Path: "scandrix-config.yml", Type: "blob"},
		{Path: "repo-1/scandrix-config.yml", Type: "blob"},
		{Path: "repo-1/.drixy-rules/review/auth.yml", Type: "blob"},
	}

	configYAML := `
version: "1.0"
customMessages:
  generalMessage: "Please follow code quality standards."
`
	tree.contents[fmt.Sprintf("%s:scandrix-config.yml", key)] = []byte(configYAML)
	tree.contents[fmt.Sprintf("%s:repo-1/scandrix-config.yml", key)] = []byte(configYAML)

	ruleYAML := `
title: Enforce HTTPS
severity: critical
enabled: true
prompt: Never use plain HTTP in production.
`
	tree.contents[fmt.Sprintf("%s:repo-1/.drixy-rules/review/auth.yml", key)] = []byte(ruleYAML)

	knownRepos := map[string]string{"repo-1": "repo-1"}

	// Sync configs
	configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err != nil {
		t.Fatalf("discovery error: %v", err)
	}

	actor := domain.ActorContext{OrganizationID: orgID, UserID: "admin"}
	syncRes, err := svc.SynchronizeConfigs(ctx, orgID, teamID, configs, actor)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if syncRes.SyncedFiles == 0 {
		t.Fatalf("expected at least 1 file synced")
	}

	// Sync rules
	rules, err := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err != nil {
		t.Fatalf("rule discovery error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule file discovered, got %d", len(rules))
	}

	ruleSyncRes, err := svc.SynchronizeDrixyRules(ctx, orgID, teamID, rules)
	if err != nil {
		t.Fatalf("rule sync error: %v", err)
	}
	if ruleSyncRes.SyncedFiles != 1 {
		t.Fatalf("expected 1 rule synced, got %d", ruleSyncRes.SyncedFiles)
	}

	// Verify rule was saved
	savedRules, _ := storage.GetRules(ctx, orgID, teamID)
	if len(savedRules) == 0 {
		t.Fatalf("expected rule to be persisted in storage")
	}

	// Now simulate removing repo-1/.drixy-rules/review/auth.yml while keeping root config
	tree.trees[key] = []domain.TreeItem{
		{Path: "scandrix-config.yml", Type: "blob"},
	}

	rulesRemaining, _ := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, repoID, knownRepos)

	// Stale cleanup should remove the rule since tree read succeeded and root config proved valid repo
	err = svc.RemoveStaleDrixyRules(ctx, orgID, teamID, rulesRemaining)
	if err != nil {
		t.Fatalf("stale rule cleanup error: %v", err)
	}

	savedRulesAfter, _ := storage.GetRules(ctx, orgID, teamID)
	if len(savedRulesAfter) != 0 {
		t.Fatalf("expected rule to be removed by stale cleanup, got %d", len(savedRulesAfter))
	}
}
