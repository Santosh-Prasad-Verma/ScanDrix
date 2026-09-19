package infrastructure_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/infrastructure"
	"gopkg.in/yaml.v3"
)

type specMockTreeProvider struct {
	mu       sync.RWMutex
	trees    map[string][]domain.TreeItem
	contents map[string][]byte
	treeErr  error
}

func newSpecMockTreeProvider() *specMockTreeProvider {
	return &specMockTreeProvider{
		trees:    make(map[string][]domain.TreeItem),
		contents: make(map[string][]byte),
	}
}

func (m *specMockTreeProvider) GetRepositoryTree(ctx context.Context, orgID, teamID, repoID string) ([]domain.TreeItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.treeErr != nil {
		return nil, m.treeErr
	}
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	return m.trees[key], nil
}

func (m *specMockTreeProvider) GetFileContent(ctx context.Context, orgID, teamID, repoID, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s:%s", orgID, teamID, repoID, path)
	content, ok := m.contents[key]
	if !ok {
		return nil, errors.New("file not found")
	}
	return content, nil
}

type specMockStoragePort struct {
	mu             sync.RWMutex
	codeReviewCfg  map[string]map[string]any
	customMessages map[string]domain.CustomMessageConfig
	rules          map[string]domain.RuleFileMeta
}

func newSpecMockStoragePort() *specMockStoragePort {
	return &specMockStoragePort{
		codeReviewCfg:  make(map[string]map[string]any),
		customMessages: make(map[string]domain.CustomMessageConfig),
		rules:          make(map[string]domain.RuleFileMeta),
	}
}

func (m *specMockStoragePort) GetCodeReviewParameter(ctx context.Context, orgID, teamID string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	cfg, ok := m.codeReviewCfg[key]
	if !ok {
		return nil, errors.New("parameter not found")
	}
	return cfg, nil
}

func (m *specMockStoragePort) SaveCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string, config map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	m.codeReviewCfg[key] = config
	return nil
}

func (m *specMockStoragePort) DeleteCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string, directoryPaths []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", orgID, teamID)
	delete(m.codeReviewCfg, key)
	return nil
}

func (m *specMockStoragePort) GetCustomMessages(ctx context.Context, orgID, teamID string) ([]domain.CustomMessageConfig, error) {
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

func (m *specMockStoragePort) SaveCustomMessage(ctx context.Context, msg domain.CustomMessageConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customMessages[msg.UUID] = msg
	return nil
}

func (m *specMockStoragePort) DeleteCustomMessage(ctx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.customMessages, uuid)
	return nil
}

func (m *specMockStoragePort) GetRules(ctx context.Context, orgID, teamID string) ([]domain.RuleFileMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []domain.RuleFileMeta
	for _, r := range m.rules {
		list = append(list, r)
	}
	return list, nil
}

func (m *specMockStoragePort) SaveRule(ctx context.Context, rule domain.RuleFileMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[rule.ID] = rule
	return nil
}

func (m *specMockStoragePort) DeleteRule(ctx context.Context, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, ruleID)
	return nil
}

func TestCentralizedConfigService_MethodCoverageSpec(t *testing.T) {
	ctx := context.Background()
	orgID := "org-test-1"
	teamID := "team-test-1"

	t.Run("validateCentralizedConfig", func(t *testing.T) {
		t.Run("fails when centralized config is not enabled", func(t *testing.T) {
			storage := newSpecMockStoragePort()
			storage.codeReviewCfg[fmt.Sprintf("%s:%s", orgID, teamID)] = map[string]any{
				"enabled": false,
			}
			svc := infrastructure.NewService(nil, nil, storage)

			res, err := svc.ValidateCentralizedConfig(ctx, orgID, teamID)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.IsValid {
				t.Fatal("expected isValid to be false")
			}
			if res.ErrorMessage != "Centralized config is not enabled for this team" {
				t.Fatalf("unexpected error message: %s", res.ErrorMessage)
			}
		})

		t.Run("fails when enabled but no repository is configured", func(t *testing.T) {
			storage := newSpecMockStoragePort()
			storage.codeReviewCfg[fmt.Sprintf("%s:%s", orgID, teamID)] = map[string]any{
				"enabled":    true,
				"repository": map[string]any{},
			}
			svc := infrastructure.NewService(nil, nil, storage)

			res, err := svc.ValidateCentralizedConfig(ctx, orgID, teamID)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.IsValid {
				t.Fatal("expected isValid to be false")
			}
			if res.ErrorMessage != "Centralized config is enabled, but no repository is configured" {
				t.Fatalf("unexpected error message: %s", res.ErrorMessage)
			}
		})

		t.Run("succeeds when enabled and a repository is configured", func(t *testing.T) {
			storage := newSpecMockStoragePort()
			storage.codeReviewCfg[fmt.Sprintf("%s:%s", orgID, teamID)] = map[string]any{
				"enabled": true,
				"repository": map[string]any{
					"id":   "r1",
					"name": "scandrix-config-repo",
				},
			}
			svc := infrastructure.NewService(nil, nil, storage)

			res, err := svc.ValidateCentralizedConfig(ctx, orgID, teamID)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !res.IsValid {
				t.Fatalf("expected isValid to be true, got error: %s", res.ErrorMessage)
			}
		})
	})

	t.Run("getCentralizedConfigRepository", func(t *testing.T) {
		t.Run("returns the configured repository", func(t *testing.T) {
			storage := newSpecMockStoragePort()
			storage.codeReviewCfg[fmt.Sprintf("%s:%s", orgID, teamID)] = map[string]any{
				"repository": map[string]any{
					"id":   "r1",
					"name": "scandrix-config-repo",
				},
			}
			svc := infrastructure.NewService(nil, nil, storage)

			repo, err := svc.GetCentralizedConfigRepository(ctx, orgID, teamID)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if repo.ID != "r1" || repo.Name != "scandrix-config-repo" {
				t.Fatalf("unexpected repo returned: %+v", repo)
			}
		})

		t.Run("throws when no repository is configured", func(t *testing.T) {
			storage := newSpecMockStoragePort()
			storage.codeReviewCfg[fmt.Sprintf("%s:%s", orgID, teamID)] = map[string]any{}
			svc := infrastructure.NewService(nil, nil, storage)

			_, err := svc.GetCentralizedConfigRepository(ctx, orgID, teamID)
			if err == nil {
				t.Fatal("expected error when no repository configured")
			}
			if err.Error() != "Centralized config repository not configured" {
				t.Fatalf("expected 'Centralized config repository not configured', got: %v", err)
			}
		})
	})

	t.Run("fetchConfigFile", func(t *testing.T) {
		t.Run("returns the config file on success", func(t *testing.T) {
			tree := newSpecMockTreeProvider()
			content := "version: '2'\nreview:\n  strictness: high\n"
			key := fmt.Sprintf("%s:%s:r1:scandrix-config.yaml", orgID, teamID)
			tree.contents[key] = []byte(content)

			svc := infrastructure.NewService(nil, tree, nil)
			data, err := svc.FetchConfigFile(ctx, orgID, teamID, "r1", "scandrix-config.yaml")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if string(data) != content {
				t.Fatalf("unexpected content returned: %s", string(data))
			}
		})

		t.Run("returns default config when the read from tree fails", func(t *testing.T) {
			tree := newSpecMockTreeProvider()
			svc := infrastructure.NewService(nil, tree, nil)

			// File not in tree provider -> falls back gracefully to default
			data, err := svc.FetchConfigFile(ctx, orgID, teamID, "r1", "scandrix-config.yaml")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("expected fallback content")
			}
		})
	})

	t.Run("fetchDrixyRuleFile", func(t *testing.T) {
		t.Run("returns null when the file has no content", func(t *testing.T) {
			tree := newSpecMockTreeProvider()
			svc := infrastructure.NewService(nil, tree, nil)

			rule, err := svc.FetchDrixyRuleFile(ctx, orgID, teamID, "r1", ".drixy-rules/review/empty.yml")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if rule != nil {
				t.Fatalf("expected nil rule for nonexistent or empty file, got: %+v", rule)
			}
		})

		t.Run("decodes and parses a base64 YAML rule file", func(t *testing.T) {
			tree := newSpecMockTreeProvider()
			yamlContent := "title: My rule\nrule: do the thing\nseverity: critical\n"
			encoded := base64.StdEncoding.EncodeToString([]byte(yamlContent))
			key := fmt.Sprintf("%s:%s:r1:.drixy-rules/review/a.yml", orgID, teamID)
			tree.contents[key] = []byte(encoded)

			svc := infrastructure.NewService(nil, tree, nil)
			rule, err := svc.FetchDrixyRuleFile(ctx, orgID, teamID, "r1", ".drixy-rules/review/a.yml")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if rule == nil {
				t.Fatal("expected parsed rule, got nil")
			}
			if rule["title"] != "My rule" {
				t.Fatalf("expected title 'My rule', got %v", rule["title"])
			}
			if rule["severity"] != "critical" {
				t.Fatalf("expected severity 'critical', got %v", rule["severity"])
			}
		})
	})
}

func TestCentralizedConfigService_EmptyDiscoveryWipeGuard1518(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"
	repoID := "central-repo-1"

	tree := newSpecMockTreeProvider()
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	tree.trees[key] = []domain.TreeItem{
		{Path: "my-repo/.drixy-rules/review/a.yml", Type: "blob"},
		{Path: "my-repo/scandrix-config.yml", Type: "blob"},
		{Path: "my-repo/src/api/scandrix-config.yml", Type: "blob"},
	}

	svc := infrastructure.NewService(nil, tree, nil)

	t.Run("discoverDrixyRulesFiles THROWS when repository mapping cannot be loaded (nil)", func(t *testing.T) {
		// Transient integration-config read failure -> nil (a FAILURE, not "zero files").
		// Must surface so the sync aborts before deletion.
		_, err := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, repoID, nil)
		if err == nil {
			t.Fatal("expected error when repository mapping is nil")
		}
	})

	t.Run("discoverConfigFiles THROWS when repository mapping cannot be loaded (nil)", func(t *testing.T) {
		_, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, repoID, nil)
		if err == nil {
			t.Fatal("expected error when repository mapping is nil")
		}
	})

	t.Run("discoverConfigFiles silently ignores a nested path deeper than one level", func(t *testing.T) {
		knownRepos := map[string]string{"my-repo": "r-1"}
		discovered, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, repoID, knownRepos)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only the repository-level file survives; the nested one (my-repo/src/api/scandrix-config.yml)
		// is dropped because remainder.length > 1
		if len(discovered) != 1 {
			t.Fatalf("expected 1 file discovered, got %d", len(discovered))
		}
		if discovered[0].RepositoryID != "r-1" {
			t.Fatalf("expected repositoryId r-1, got %s", discovered[0].RepositoryID)
		}
	})
}

func TestCentralizedConfigService_DiscoverDrixyRulesFiles(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"
	repoID := "central-repo-1"

	tree := newSpecMockTreeProvider()
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)

	tree.trees[key] = []domain.TreeItem{
		{Path: "scandrix-config.yml", Type: "blob"},
		{Path: ".drixy-rules/memories/logging.yml", Type: "blob"},
		{Path: ".drixy-rules/review/security.yml", Type: "blob"},
		{Path: "org-a/.drixy-rules/memories/auth.yml", Type: "blob"},
		{Path: "org-a/services%2Fapi/.drixy-rules/review/api.yml", Type: "blob"},
		{Path: "other-files/README.md", Type: "blob"},
		{Path: "rules.txt", Type: "blob"},
	}

	knownRepos := map[string]string{
		"org-a": "org-a-id",
	}

	svc := infrastructure.NewService(nil, tree, nil)
	result, err := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, repoID, knownRepos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should discover 4 valid rule files:
	// 1. .drixy-rules/memories/logging.yml (global memory)
	// 2. .drixy-rules/review/security.yml (global standard)
	// 3. org-a/.drixy-rules/memories/auth.yml (scoped repository)
	// 4. org-a/services%2Fapi/.drixy-rules/review/api.yml (scoped directory group)
	if len(result) != 4 {
		t.Fatalf("expected 4 rule files discovered, got %d", len(result))
	}

	paths := make(map[string]domain.RuleFileMeta)
	for _, r := range result {
		paths[r.Path] = r
	}

	if r, ok := paths[".drixy-rules/memories/logging.yml"]; !ok || !r.IsMemory || r.Scope != domain.RuleScopeGlobal {
		t.Fatalf("unexpected global memory rule: %+v", r)
	}
	if r, ok := paths[".drixy-rules/review/security.yml"]; !ok || r.IsMemory || r.Scope != domain.RuleScopeGlobal {
		t.Fatalf("unexpected global review rule: %+v", r)
	}
	if r, ok := paths["org-a/.drixy-rules/memories/auth.yml"]; !ok || r.RepositoryID != "org-a-id" || r.Scope != domain.RuleScopeRepository {
		t.Fatalf("unexpected repo-scoped rule: %+v", r)
	}
	if r, ok := paths["org-a/services%2Fapi/.drixy-rules/review/api.yml"]; !ok || r.RepositoryID != "org-a-id" || r.Scope != domain.RuleScopeDirectory {
		t.Fatalf("unexpected directory group rule: %+v", r)
	} else if len(r.DirectoryPaths) != 1 || r.DirectoryPaths[0] != "/services/api" {
		t.Fatalf("expected directory path '/services/api', got: %v", r.DirectoryPaths)
	}
}

func TestCentralizedConfigService_SynchronizeDrixyRules(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"
	repoID := "central-repo-1"

	tree := newSpecMockTreeProvider()
	storage := newSpecMockStoragePort()
	svc := infrastructure.NewService(nil, tree, storage)

	ruleContent := map[string]any{
		"title": "Logging Rule",
		"rule":  "Use structured logging",
		"examples": []any{
			map[string]any{"snippet": "console.log('test')", "isCorrect": false},
		},
		"inheritance": map[string]any{
			"inheritable": true,
			"exclude":     []any{},
			"include":     []any{},
		},
	}
	yamlBytes, _ := yaml.Marshal(ruleContent)

	key := fmt.Sprintf("%s:%s:%s:.drixy-rules/memories/logging.yml", orgID, teamID, repoID)
	tree.contents[key] = yamlBytes

	ruleFiles := []domain.RuleFileMeta{
		{
			ID:           "logging",
			Title:        "Logging Rule",
			Path:         ".drixy-rules/memories/logging.yml",
			RepositoryID: repoID,
			Scope:        domain.RuleScopeGlobal,
			IsMemory:     true,
			Enabled:      true,
		},
	}

	result, err := svc.SynchronizeDrixyRules(ctx, orgID, teamID, ruleFiles)
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}
	if result.SyncedFiles != 1 {
		t.Fatalf("expected 1 file synced, got %d", result.SyncedFiles)
	}

	saved, _ := storage.GetRules(ctx, orgID, teamID)
	if len(saved) != 1 {
		t.Fatalf("expected 1 rule in storage, got %d", len(saved))
	}
	if saved[0].Title != "Logging Rule" {
		t.Fatalf("unexpected rule title in storage: %s", saved[0].Title)
	}

	// Remove stale test
	err = svc.RemoveStaleDrixyRules(ctx, orgID, teamID, []domain.RuleFileMeta{})
	if err != nil {
		t.Fatalf("unexpected remove stale error: %v", err)
	}
	savedAfter, _ := storage.GetRules(ctx, orgID, teamID)
	if len(savedAfter) != 0 {
		t.Fatalf("expected rules to be cleaned up, got %d", len(savedAfter))
	}
}
