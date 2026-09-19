package infrastructure

import (
	"context"
	"fmt"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
)

func TestIssue1579_StaleConfigScenarios(t *testing.T) {
	ctx := context.Background()
	orgID := "org-1"
	teamID := "team-1"
	centralRepo := "central-config-repo"
	actor := domain.ActorContext{
		OrganizationID: orgID,
		Source:         "sync",
		UserEmail:      "drixy@scandrix.dev",
		UserID:         "drixy",
	}

	knownRepos := map[string]string{
		"repo-1": "repo-1",
		"repo-2": "repo-2",
		"repo-3": "repo-3",
	}

	// Test A: Global-only discovery does NOT remove connected repositories (Issue #1579)
	t.Run("TestA_GlobalOnly_DoesNotRemoveConnectedRepos", func(t *testing.T) {
		tree := newMockTreeProvider()
		storage := newMockStoragePort()
		svc := NewService(nil, tree, storage)

		key := fmt.Sprintf("%s:%s:%s", orgID, teamID, centralRepo)
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
		}
		tree.contents[fmt.Sprintf("%s:scandrix-config.yml", key)] = []byte("version: '1.2'\n")

		// Seed repository custom messages
		storage.customMessages["msg-1"] = domain.CustomMessageConfig{
			UUID:             "msg-1",
			OrganizationUUID: orgID,
			RepositoryID:     "repo-1",
			ConfigLevel:      domain.ConfigLevelRepository,
			Content:          "Follow repo-1 guidelines",
		}

		configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		if err != nil {
			t.Fatalf("discovery error: %v", err)
		}
		if len(configs) != 1 || configs[0].Level != domain.ConfigLevelOrganization {
			t.Fatalf("expected 1 global config discovered, got %+v", configs)
		}

		// When sync runs, global-only discovery must NOT prune repo-1 because repo-1 was not in managed set
		err = svc.RemoveStaleConfigs(ctx, orgID, teamID, configs)
		if err != nil {
			t.Fatalf("stale config cleanup failed: %v", err)
		}

		msgs, _ := storage.GetCustomMessages(ctx, orgID, teamID)
		if len(msgs) == 0 {
			t.Fatalf("Test A failed: global-only discovery wiped unmanaged repo custom messages")
		}
	})

	// Test B: Sparse config discovery (only repo-1 has a config) does NOT deselect repo-2 and repo-3
	t.Run("TestB_SparseConfig_DoesNotDeselectOtherRepos", func(t *testing.T) {
		tree := newMockTreeProvider()
		storage := newMockStoragePort()
		svc := NewService(nil, tree, storage)

		key := fmt.Sprintf("%s:%s:%s", orgID, teamID, centralRepo)
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
			{Path: "repo-1/scandrix-config.yml", Type: "blob"},
		}
		tree.contents[fmt.Sprintf("%s:scandrix-config.yml", key)] = []byte("version: '1.2'\n")
		tree.contents[fmt.Sprintf("%s:repo-1/scandrix-config.yml", key)] = []byte("version: '1.2'\n")

		storage.customMessages["msg-repo-2"] = domain.CustomMessageConfig{
			UUID:         "msg-repo-2",
			RepositoryID: "repo-2",
			ConfigLevel:  domain.ConfigLevelRepository,
			Content:      "Repo-2 guidelines",
		}

		configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		if err != nil {
			t.Fatalf("discovery error: %v", err)
		}
		if len(configs) != 2 {
			t.Fatalf("expected 2 configs discovered, got %d", len(configs))
		}

		syncRes, err := svc.SynchronizeConfigs(ctx, orgID, teamID, configs, actor)
		if err != nil || !syncRes.Success {
			t.Fatalf("sync error: %v", err)
		}
	})

	// Test C: Complete discovery tracks all repositories
	t.Run("TestC_CompleteDiscovery_KeepsAllRepos", func(t *testing.T) {
		tree := newMockTreeProvider()
		storage := newMockStoragePort()
		svc := NewService(nil, tree, storage)

		key := fmt.Sprintf("%s:%s:%s", orgID, teamID, centralRepo)
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
			{Path: "repo-1/scandrix-config.yml", Type: "blob"},
			{Path: "repo-2/scandrix-config.yml", Type: "blob"},
			{Path: "repo-3/scandrix-config.yml", Type: "blob"},
		}

		configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		if err != nil {
			t.Fatalf("discovery error: %v", err)
		}
		if len(configs) != 4 {
			t.Fatalf("expected 4 configs, got %d", len(configs))
		}

		sorted := svc.SortConfigFiles(configs)
		if sorted[0].Level != domain.ConfigLevelOrganization {
			t.Fatalf("expected global config to be sorted first")
		}
	})

	// Test D: Directory-only centralized file for repo-1 does not wipe repo-1
	t.Run("TestD_DirectoryOnlyConfig_MaintainsRepoScope", func(t *testing.T) {
		tree := newMockTreeProvider()
		storage := newMockStoragePort()
		svc := NewService(nil, tree, storage)

		encodedGroup, err := utils.BuildGroupFolderName([]string{"src/api"})
		if err != nil {
			t.Fatalf("failed group folder encode: %v", err)
		}

		key := fmt.Sprintf("%s:%s:%s", orgID, teamID, centralRepo)
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
			{Path: fmt.Sprintf("repo-1/%s/scandrix-config.yml", encodedGroup), Type: "blob"},
		}
		tree.contents[fmt.Sprintf("%s:scandrix-config.yml", key)] = []byte("version: '1.2'\n")
		tree.contents[fmt.Sprintf("%s:repo-1/%s/scandrix-config.yml", key, encodedGroup)] = []byte("version: '1.2'\n")

		configs, err := svc.DiscoverConfigFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		if err != nil {
			t.Fatalf("discovery error: %v", err)
		}
		if len(configs) != 2 {
			t.Fatalf("expected 2 configs, got %d", len(configs))
		}

		dirConfig := configs[1]
		if dirConfig.Level != domain.ConfigLevelDirectory || len(dirConfig.DirectoryPaths) != 1 {
			t.Fatalf("expected directory level config: %+v", dirConfig)
		}
	})

	// Test E: Genuine removal path works when a managed rule or config is deleted
	t.Run("TestE_GenuineRemoval_PrunesDeletedRules", func(t *testing.T) {
		tree := newMockTreeProvider()
		storage := newMockStoragePort()
		svc := NewService(nil, tree, storage)

		key := fmt.Sprintf("%s:%s:%s", orgID, teamID, centralRepo)
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
			{Path: "repo-1/.drixy-rules/sec.yml", Type: "blob"},
		}

		ruleContent := `title: Security Rule
severity: critical
enabled: true
prompt: Always check authorization headers.`
		tree.contents[fmt.Sprintf("%s:repo-1/.drixy-rules/sec.yml", key)] = []byte(ruleContent)

		// Discover and sync rule
		rules, err := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		if err != nil || len(rules) != 1 {
			t.Fatalf("expected 1 rule discovered, got %d, err: %v", len(rules), err)
		}

		_, err = svc.SynchronizeDrixyRules(ctx, orgID, teamID, rules)
		if err != nil {
			t.Fatalf("rule sync failed: %v", err)
		}

		saved, _ := storage.GetRules(ctx, orgID, teamID)
		if len(saved) != 1 {
			t.Fatalf("expected rule saved in storage, got %d", len(saved))
		}

		// Now simulate deleting the rule from repository tree
		tree.trees[key] = []domain.TreeItem{
			{Path: "scandrix-config.yml", Type: "blob"},
		}

		remainingRules, _ := svc.DiscoverDrixyRulesFiles(ctx, orgID, teamID, centralRepo, knownRepos)
		err = svc.RemoveStaleDrixyRules(ctx, orgID, teamID, remainingRules)
		if err != nil {
			t.Fatalf("stale rule cleanup error: %v", err)
		}

		after, _ := storage.GetRules(ctx, orgID, teamID)
		if len(after) != 0 {
			t.Fatalf("expected rule to be removed after deletion from git tree")
		}
	})
}
