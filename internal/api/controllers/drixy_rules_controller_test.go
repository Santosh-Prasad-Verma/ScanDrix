// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Controller Tests
// File: drixy_rules_controller_test.go
// ═══════════════════════════════════════════════════════════════

package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func setupTestDrixyRulesController() (chi.Router, *controllers.DrixyRulesController) {
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, ruleLikeSvc)

	summarySvc := services.NewDrixyRuleSummaryService(nil)
	detectorCompiler := services.NewDrixyRuleDetectorCompiler(rulesSvc, nil)
	refLoader := services.NewExternalReferenceLoaderService()

	createUC := usecases.NewCreateOrUpdateDrixyRuleUseCase(rulesSvc, detectorCompiler, summarySvc, refLoader)
	findByOrgUC := usecases.NewFindByOrganizationIDDrixyRulesUseCase(rulesSvc, refLoader)
	findRulesByFilterUC := usecases.NewFindRulesInOrganizationByFilterDrixyRulesUseCase(rulesSvc, refLoader)
	deleteRuleInOrgUC := usecases.NewDeleteRuleInOrganizationByIDDrixyRulesUseCase(rulesSvc)
	findLibraryRulesUC := usecases.NewFindLibraryDrixyRulesUseCase(rulesSvc)
	findLibraryFeedbackUC := usecases.NewFindLibraryDrixyRulesWithFeedbackUseCase(rulesSvc)
	findLibraryBucketsUC := usecases.NewFindLibraryDrixyRulesBucketsUseCase(rulesSvc)
	findRecommendedRulesUC := usecases.NewFindRecommendedDrixyRulesUseCase(rulesSvc, nil)
	addLibraryRulesUC := usecases.NewAddLibraryDrixyRulesUseCase(createUC)
	changeStatusUC := usecases.NewChangeStatusDrixyRulesUseCase(rulesSvc)
	applyPendingUC := usecases.NewApplyPendingDrixyRulesUseCase(rulesSvc)
	getPendingUC := usecases.NewGetPendingDrixyRulesUseCase(rulesSvc)
	getInheritedUC := usecases.NewGetInheritedDrixyRulesUseCase(rulesSvc)
	getLimitsUC := usecases.NewGetRulesLimitStatusUseCase(rulesSvc)

	ctrl := controllers.NewDrixyRulesController(controllers.DrixyRulesUseCases{
		CreateOrUpdateUseCase:               createUC,
		FindByOrganizationIDUseCase:         findByOrgUC,
		FindRulesByFilterUseCase:            findRulesByFilterUC,
		DeleteRuleInOrgByIDUseCase:          deleteRuleInOrgUC,
		FindLibraryDrixyRulesUseCase:        findLibraryRulesUC,
		FindLibraryDrixyRulesWithFeedbackUC: findLibraryFeedbackUC,
		FindLibraryDrixyRulesBucketsUC:      findLibraryBucketsUC,
		FindRecommendedDrixyRulesUC:         findRecommendedRulesUC,
		AddLibraryDrixyRulesUseCase:         addLibraryRulesUC,
		ChangeStatusDrixyRulesUseCase:       changeStatusUC,
		ApplyPendingDrixyRulesUseCase:       applyPendingUC,
		GetPendingDrixyRulesUseCase:         getPendingUC,
		GetInheritedRulesUseCase:            getInheritedUC,
		GetRulesLimitStatusUseCase:          getLimitsUC,
	})

	r := chi.NewRouter()
	r.Mount("/api/v1/drixy-rules", ctrl.Routes())
	return r, ctrl
}

func TestDrixyRulesController_CreateAndFetch(t *testing.T) {
	router, _ := setupTestDrixyRulesController()

	// 1. Create rule
	dto := dtos.CreateDrixyRuleDto{
		Title:        "Enforce Secure Headers",
		Rule:         "Always set X-Content-Type-Options nosniff in HTTP responses",
		Path:         "**/*.go",
		Severity:     "HIGH",
		Scope:        interfaces.DrixyRulesScopeFile,
		RepositoryID: "repo-core",
		Status:       interfaces.DrixyRulesStatusActive,
		Type:         interfaces.DrixyRulesTypeStandard,
	}

	payload, _ := json.Marshal(dto)
	req := httptest.NewRequest("POST", "/api/v1/drixy-rules", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-organization-id", "org-test-ctrl")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on create rule, got %d: %s", w.Code, w.Body.String())
	}

	var createdRule interfaces.DrixyRule
	if err := json.Unmarshal(w.Body.Bytes(), &createdRule); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if createdRule.UUID == "" {
		t.Fatal("expected non-empty rule UUID")
	}

	// 2. Fetch rules for org
	fetchReq := httptest.NewRequest("GET", "/api/v1/drixy-rules", nil)
	fetchReq.Header.Set("x-organization-id", "org-test-ctrl")

	wFetch := httptest.NewRecorder()
	router.ServeHTTP(wFetch, fetchReq)

	if wFetch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on fetch rules, got %d: %s", wFetch.Code, wFetch.Body.String())
	}

	// 3. Query library buckets
	bucketReq := httptest.NewRequest("GET", "/api/v1/drixy-rules/find-library-drixy-rules-buckets", nil)
	wBucket := httptest.NewRecorder()
	router.ServeHTTP(wBucket, bucketReq)

	if wBucket.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on library buckets, got %d: %s", wBucket.Code, wBucket.Body.String())
	}
}
