// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_sync_listener_test.go
// ═══════════════════════════════════════════════════════════════

package listeners_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/listeners"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestDrixyRulesSyncListener(t *testing.T) {
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, ruleLikeSvc)
	syncSvc := services.NewDrixyRulesSyncService(rulesSvc)

	listener := listeners.NewDrixyRulesSyncListener(syncSvc, rulesSvc)

	t.Run("Non-merged PR is ignored", func(t *testing.T) {
		err := listener.HandlePullRequestClosed(context.Background(), listeners.PullRequestClosedEvent{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			Merged:         false,
			ModifiedFiles:  []string{".scandrix/rules/security.md"},
		})
		if err != nil {
			t.Fatalf("expected nil error for unmerged PR, got: %v", err)
		}
	})

	t.Run("Merged PR with non-rule files is ignored", func(t *testing.T) {
		err := listener.HandlePullRequestClosed(context.Background(), listeners.PullRequestClosedEvent{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			Merged:         true,
			ModifiedFiles:  []string{"src/main.ts", "package.json"},
		})
		if err != nil {
			t.Fatalf("expected nil error for unrelated files, got: %v", err)
		}
	})

	t.Run("Merged PR with rule files triggers sync check", func(t *testing.T) {
		err := listener.HandlePullRequestClosed(context.Background(), listeners.PullRequestClosedEvent{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			Merged:         true,
			ModifiedFiles:  []string{".scandrix/rules/perf.md"},
		})
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("IsGlobalSourceRepository correctly identifies configured sources", func(t *testing.T) {
		config := &interfaces.GlobalRulesSourceConfig{
			Repositories: []interfaces.GlobalRulesSourceRepository{
				{ID: "global-repo-1", Name: "company/global-rules"},
			},
		}

		if !listener.IsGlobalSourceRepository(config, "global-repo-1") {
			t.Errorf("expected global-repo-1 to be recognized as source")
		}
		if listener.IsGlobalSourceRepository(config, "other-repo") {
			t.Errorf("expected other-repo to NOT be recognized as source")
		}
		if listener.IsGlobalSourceRepository(nil, "global-repo-1") {
			t.Errorf("expected nil config to return false")
		}
	})
}
