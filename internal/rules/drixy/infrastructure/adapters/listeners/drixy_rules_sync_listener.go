// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_sync_listener.go
// ═══════════════════════════════════════════════════════════════

package listeners

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/domain/utils"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// PullRequestClosedEvent models SCM webhook events for merged pull requests.
type PullRequestClosedEvent struct {
	OrganizationID string
	TeamID         string
	RepositoryID   string
	RepositoryName string
	Merged         bool
	BaseBranch     string
	ModifiedFiles  []string
}

// DrixyRulesSyncListener responds to merged pull requests by triggering automatic rule synchronization.
type DrixyRulesSyncListener struct {
	syncService  *services.DrixyRulesSyncService
	rulesService *services.DrixyRulesService
	mu           sync.Mutex
}

// NewDrixyRulesSyncListener constructs a new listener instance.
func NewDrixyRulesSyncListener(
	syncService *services.DrixyRulesSyncService,
	rulesService *services.DrixyRulesService,
) *DrixyRulesSyncListener {
	return &DrixyRulesSyncListener{
		syncService:  syncService,
		rulesService: rulesService,
	}
}

// HandlePullRequestClosed inspects merged PR file changes and synchronizes repository rules if rule files changed.
func (l *DrixyRulesSyncListener) HandlePullRequestClosed(ctx context.Context, event PullRequestClosedEvent) error {
	if !event.Merged {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	hasRuleFileChanges := false
	for _, f := range event.ModifiedFiles {
		if utils.IsDrixyRuleTemplateFile(f) ||
			strings.HasSuffix(f, ".cursorrules") ||
			strings.HasSuffix(f, "CLAUDE.md") ||
			strings.HasSuffix(f, "AGENTS.md") {
			hasRuleFileChanges = true
			break
		}
	}

	if !hasRuleFileChanges {
		return nil
	}

	log.Printf("[DrixyRulesSyncListener] Detected rule file modifications in repo %s. Synchronizing rules...", event.RepositoryName)

	syncParams := services.SyncRepositoryParams{
		OrganizationID: event.OrganizationID,
		TeamID:         event.TeamID,
		RepositoryID:   event.RepositoryID,
		RepositoryName: event.RepositoryName,
		DefaultBranch:  event.BaseBranch,
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err := l.syncService.SyncRepositoryMainFast(ctxTimeout, syncParams)
	if err != nil {
		log.Printf("[DrixyRulesSyncListener] Failed to sync rules for repo %s: %v", event.RepositoryName, err)
		return err
	}

	return nil
}

// IsGlobalSourceRepository checks if a repository is configured as the organization-wide global rules source.
func (l *DrixyRulesSyncListener) IsGlobalSourceRepository(config *interfaces.GlobalRulesSourceConfig, repoID string) bool {
	if config == nil {
		return false
	}
	for _, r := range config.Repositories {
		if r.ID == repoID {
			return true
		}
	}
	return false
}
