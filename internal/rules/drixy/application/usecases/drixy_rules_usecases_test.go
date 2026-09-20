// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Unit Tests
// File: drixy_rules_usecases_test.go
// ═══════════════════════════════════════════════════════════════

package usecases_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestDrixyRulesUseCases_Lifecycle(t *testing.T) {
	ctx := context.Background()

	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, ruleLikeSvc)

	summarySvc := services.NewDrixyRuleSummaryService(nil)
	detectorCompiler := services.NewDrixyRuleDetectorCompiler(rulesSvc, nil)
	refLoader := services.NewExternalReferenceLoaderService()

	createUC := usecases.NewCreateOrUpdateDrixyRuleUseCase(rulesSvc, detectorCompiler, summarySvc, refLoader)
	findByOrgUC := usecases.NewFindByOrganizationIDDrixyRulesUseCase(rulesSvc, refLoader)
	changeStatusUC := usecases.NewChangeStatusDrixyRulesUseCase(rulesSvc)
	getInheritedUC := usecases.NewGetInheritedDrixyRulesUseCase(rulesSvc)

	orgID := "org-test-enterprise"
	userInfo := &contracts.UserAuditInfo{
		UserID:    "user-123",
		UserEmail: "engineer@scandrix.dev",
	}

	// 1. Create a global rule
	globalRuleDto := dtos.CreateDrixyRuleDto{
		Title:        "Ensure Parameterized Queries",
		Rule:         "Never format raw inputs directly into SQL queries to prevent SQL injections",
		Path:         "**/*.go",
		Severity:     "CRITICAL",
		Scope:        interfaces.DrixyRulesScopeFile,
		RepositoryID: "global",
		Status:       interfaces.DrixyRulesStatusActive,
		Type:         interfaces.DrixyRulesTypeStandard,
	}

	createdGlobal, err := createUC.Execute(ctx, globalRuleDto, orgID, userInfo)
	if err != nil {
		t.Fatalf("failed to create global rule: %v", err)
	}
	if createdGlobal.UUID == "" {
		t.Fatal("expected non-empty UUID for created rule")
	}

	// 2. Create a repository-scoped rule
	repoRuleDto := dtos.CreateDrixyRuleDto{
		Title:        "Enforce Struct Tags",
		Rule:         "All JSON models must declare explicit json struct tags",
		Path:         "pkg/models/*.go",
		Severity:     "HIGH",
		Scope:        interfaces.DrixyRulesScopeFile,
		RepositoryID: "repo-payments",
		Status:       interfaces.DrixyRulesStatusActive,
		Type:         interfaces.DrixyRulesTypeStandard,
	}

	createdRepoRule, err := createUC.Execute(ctx, repoRuleDto, orgID, userInfo)
	if err != nil {
		t.Fatalf("failed to create repo rule: %v", err)
	}

	// 3. Find rules by organization
	orgRules, err := findByOrgUC.Execute(ctx, orgID)
	if err != nil {
		t.Fatalf("failed to find org rules: %v", err)
	}
	if len(orgRules.Rules) != 2 {
		t.Fatalf("expected 2 rules for organization, got %d", len(orgRules.Rules))
	}

	// 4. Verify inheritance resolution
	inherited, err := getInheritedUC.Execute(ctx, orgID, "repo-payments", "")
	if err != nil {
		t.Fatalf("failed to resolve inherited rules: %v", err)
	}

	if len(inherited.GlobalRules) != 1 {
		t.Fatalf("expected 1 global rule, got %d", len(inherited.GlobalRules))
	}
	if len(inherited.RepoRules) != 1 {
		t.Fatalf("expected 1 repository rule, got %d", len(inherited.RepoRules))
	}

	// 5. Change status to paused
	pausedStatus, err := changeStatusUC.Execute(ctx, orgID, dtos.ChangeStatusDrixyRulesDTO{
		RuleIDs: []string{createdRepoRule.UUID},
		Status:  interfaces.DrixyRulesStatusPaused,
	})
	if err != nil {
		t.Fatalf("failed to pause rule: %v", err)
	}
	if len(pausedStatus) != 1 || pausedStatus[0].Status != interfaces.DrixyRulesStatusPaused {
		t.Fatalf("expected rule to be paused, got: %v", pausedStatus)
	}
}
