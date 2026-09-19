// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_controller.go
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/rules/drixy/application/usecases"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// DrixyRulesUseCases aggregates all business use cases for the Drixy rules domain.
type DrixyRulesUseCases struct {
	CreateOrUpdateUseCase               *usecases.CreateOrUpdateDrixyRuleUseCase
	FindByOrganizationIDUseCase         *usecases.FindByOrganizationIDDrixyRulesUseCase
	FindRulesByFilterUseCase            *usecases.FindRulesInOrganizationByFilterDrixyRulesUseCase
	DeleteRuleInOrgByIDUseCase          *usecases.DeleteRuleInOrganizationByIDDrixyRulesUseCase
	FindLibraryDrixyRulesUseCase        *usecases.FindLibraryDrixyRulesUseCase
	FindLibraryDrixyRulesWithFeedbackUC *usecases.FindLibraryDrixyRulesWithFeedbackUseCase
	FindLibraryDrixyRulesBucketsUC      *usecases.FindLibraryDrixyRulesBucketsUseCase
	FindRecommendedDrixyRulesUC         *usecases.FindRecommendedDrixyRulesUseCase
	AddLibraryDrixyRulesUseCase         *usecases.AddLibraryDrixyRulesUseCase
	GenerateDrixyRulesUseCase           *usecases.GenerateDrixyRulesUseCase
	ApplyPendingDrixyRulesUseCase       *usecases.ApplyPendingDrixyRulesUseCase
	GetPendingDrixyRulesUseCase         *usecases.GetPendingDrixyRulesUseCase
	ChangeStatusDrixyRulesUseCase       *usecases.ChangeStatusDrixyRulesUseCase
	CheckSyncStatusUseCase              *usecases.CheckSyncStatusUseCase
	ListPastReviewersUseCase            *usecases.ListPastReviewersUseCase
	SyncSelectedReposUseCase            *usecases.SyncSelectedRepositoriesDrixyRulesUseCase
	GetGlobalRulesSourceRepositoriesUC  *usecases.GetGlobalRulesSourceRepositoriesUseCase
	UpdateGlobalRulesSourceReposUC      *usecases.UpdateGlobalRulesSourceRepositoriesUseCase
	ResyncGlobalRulesUseCase            *usecases.ResyncGlobalRulesUseCase
	GetGlobalRulesImportStatusUC        *usecases.GetGlobalRulesImportStatusUseCase
	GetInheritedRulesUseCase            *usecases.GetInheritedDrixyRulesUseCase
	GetRulesLimitStatusUseCase          *usecases.GetRulesLimitStatusUseCase
	FindSuggestionsByRuleUseCase        *usecases.FindSuggestionsByRuleUseCase
	ResyncRulesFromIdeUseCase           *usecases.ResyncRulesFromIdeUseCase
	FastSyncIdeRulesUseCase             *usecases.FastSyncIdeRulesUseCase
	ImportFastDrixyRulesUseCase         *usecases.ImportFastDrixyRulesUseCase
	ConvertPendingUpdatesToNewUseCase   *usecases.ConvertPendingUpdatesToNewUseCase
	ManageImportedDrixyRulesUseCase     *usecases.ManageImportedDrixyRulesUseCase
	CountRulesByRepositoryUseCase       *usecases.CountRulesByRepositoryUseCase
	BackfillRuleDetectorsUseCase        *usecases.BackfillRuleDetectorsUseCase
}

// DrixyRulesController exposes REST API endpoints for Drixy Rules.
type DrixyRulesController struct {
	useCases DrixyRulesUseCases
}

// NewDrixyRulesController initializes the REST controller with injected use cases.
func NewDrixyRulesController(useCases DrixyRulesUseCases) *DrixyRulesController {
	return &DrixyRulesController{useCases: useCases}
}

// Routes mounts all Drixy rules endpoints matching the enterprise API contract.
func (c *DrixyRulesController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleCreateOrUpdate)
	r.Get("/", c.handleFindByOrganizationID)
	r.Post("/create-or-update", c.handleCreateOrUpdate)
	r.Get("/find-by-organization-id", c.handleFindByOrganizationID)
	r.Get("/limits", c.handleGetRulesLimitStatus)
	r.Get("/counts-by-repository", c.handleCountRulesByRepository)
	r.Get("/suggestions", c.handleFindSuggestionsByRule)
	r.Get("/find-rules-in-organization-by-filter", c.handleFindRulesByFilter)
	r.Delete("/delete-rule-in-organization-by-id", c.handleDeleteRuleInOrgByID)
	r.Get("/find-library-drixy-rules", c.handleFindLibraryDrixyRules)
	r.Get("/find-library-drixy-rules-with-feedback", c.handleFindLibraryDrixyRulesWithFeedback)
	r.Get("/find-library-drixy-rules-buckets", c.handleFindLibraryDrixyRulesBuckets)
	r.Get("/find-recommended-drixy-rules", c.handleFindRecommendedDrixyRules)
	r.Post("/add-library-drixy-rules", c.handleAddLibraryDrixyRules)
	r.Post("/generate-drixy-rules", c.handleGenerateDrixyRules)
	r.Post("/change-status-drixy-rules", c.handleChangeStatusDrixyRules)
	r.Post("/pending/apply", c.handleApplyPendingDrixyRules)
	r.Post("/pending/discard", c.handleDiscardPendingDrixyRules)
	r.Post("/pending/convert-updates-to-new", c.handleConvertPendingUpdatesToNew)
	r.Get("/check-sync-status", c.handleCheckSyncStatus)
	r.Get("/past-reviewers", c.handleListPastReviewers)
	r.Post("/sync-ide-rules", c.handleSyncIdeRules)
	r.Post("/fast-sync-ide-rules", c.handleFastSyncIdeRules)
	r.Get("/global-source-repositories", c.handleGetGlobalSourceRepositories)
	r.Get("/global-source-repositories/import-status", c.handleGetGlobalImportStatus)
	r.Post("/global-source-repositories", c.handleUpdateGlobalSourceRepositories)
	r.Post("/resync-global-rules", c.handleResyncGlobalRules)
	r.Get("/pending", c.handleGetPendingDrixyRules)
	r.Get("/pending-ide-rules", c.handleGetPendingIdeRules)
	r.Post("/import-fast-ide-rules", c.handleImportFastIdeRules)
	r.Post("/review-fast-ide-rules", c.handleReviewFastIdeRules)
	r.Get("/inherited-rules", c.handleGetInheritedRules)
	r.Post("/resync-ide-rules", c.handleResyncIdeRules)
	r.Post("/imported/manage", c.handleManageImportedDrixyRules)
	r.Get("/imported/count", c.handleCountImportedDrixyRules)

	return r
}

// ─────────────────────────────────────────────────────────────
// Context Helpers
// ─────────────────────────────────────────────────────────────

func (c *DrixyRulesController) getOrgAndUserInfo(r *http.Request) (string, *contracts.UserAuditInfo) {
	orgID := ""
	var userInfo *contracts.UserAuditInfo

	if profile, ok := auth.AccountProfileFromContext(r.Context()); ok && profile != nil {
		orgID = profile.WorkspaceID.String()
		userInfo = &contracts.UserAuditInfo{
			UserID:    profile.ID.String(),
			UserEmail: profile.Email,
		}
	} else if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil {
		orgID = wsID.String()
		userInfo = &contracts.UserAuditInfo{
			UserID: wsID.String(),
		}
	}

	if reqOrg := r.URL.Query().Get("organizationId"); reqOrg != "" && orgID == "" {
		orgID = reqOrg
	}
	if hOrg := r.Header.Get("x-organization-id"); hOrg != "" && orgID == "" {
		orgID = hOrg
	}

	return orgID, userInfo
}


func (c *DrixyRulesController) jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func (c *DrixyRulesController) jsonError(w http.ResponseWriter, status int, msg string) {
	c.jsonResponse(w, status, map[string]string{"error": msg})
}

// ─────────────────────────────────────────────────────────────
// Route Handlers
// ─────────────────────────────────────────────────────────────

func (c *DrixyRulesController) handleCreateOrUpdate(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.CreateDrixyRuleDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	res, err := c.useCases.CreateOrUpdateUseCase.Execute(r.Context(), dto, orgID, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleFindByOrganizationID(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	res, err := c.useCases.FindByOrganizationIDUseCase.Execute(r.Context(), orgID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGetRulesLimitStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")

	res, err := c.useCases.GetRulesLimitStatusUseCase.Execute(r.Context(), orgID, teamID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleCountRulesByRepository(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	res, err := c.useCases.CountRulesByRepositoryUseCase.Execute(r.Context(), orgID, nil)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleFindSuggestionsByRule(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	ruleID := r.URL.Query().Get("ruleId")

	res, err := c.useCases.FindSuggestionsByRuleUseCase.Execute(r.Context(), orgID, ruleID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleFindRulesByFilter(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	filter := make(map[string]any)
	repoID := r.URL.Query().Get("repositoryId")
	dirID := r.URL.Query().Get("directoryId")
	status := r.URL.Query().Get("status")
	if status != "" {
		filter["status"] = status
	}

	res, err := c.useCases.FindRulesByFilterUseCase.Execute(r.Context(), orgID, filter, repoID, dirID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleDeleteRuleInOrgByID(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	ruleID := r.URL.Query().Get("ruleId")
	teamID := r.URL.Query().Get("teamId")

	if ruleID == "" {
		c.jsonError(w, http.StatusBadRequest, "ruleId query parameter is required")
		return
	}

	deleted, err := c.useCases.DeleteRuleInOrgByIDUseCase.Execute(r.Context(), orgID, teamID, ruleID, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, map[string]bool{"success": deleted})
}

func (c *DrixyRulesController) handleFindLibraryDrixyRules(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	dto := dtos.FindLibraryDrixyRulesDto{
		Page:     page,
		Limit:    limit,
		Search:   r.URL.Query().Get("search"),
		Language: r.URL.Query().Get("language"),
		Scope:    r.URL.Query().Get("scope"),
		Severity: r.URL.Query().Get("severity"),
	}

	res, err := c.useCases.FindLibraryDrixyRulesUseCase.Execute(r.Context(), dto)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}


func (c *DrixyRulesController) handleFindLibraryDrixyRulesWithFeedback(w http.ResponseWriter, r *http.Request) {
	_, userInfo := c.getOrgAndUserInfo(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	filterDTO := dtos.FindLibraryDrixyRulesDto{
		Page:     page,
		Limit:    limit,
		Search:   r.URL.Query().Get("search"),
		Language: r.URL.Query().Get("language"),
		Scope:    r.URL.Query().Get("scope"),
		Severity: r.URL.Query().Get("severity"),
	}

	res, err := c.useCases.FindLibraryDrixyRulesWithFeedbackUC.Execute(r.Context(), filterDTO, userInfo.UserID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleFindLibraryDrixyRulesBuckets(w http.ResponseWriter, r *http.Request) {
	res, err := c.useCases.FindLibraryDrixyRulesBucketsUC.Execute(r.Context())
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleFindRecommendedDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	res, err := c.useCases.FindRecommendedDrixyRulesUC.Execute(r.Context(), orgID, teamID, limit)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleAddLibraryDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.AddLibraryDrixyRulesDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.AddLibraryDrixyRulesUseCase.Execute(r.Context(), orgID, dto, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGenerateDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto usecases.GenerateDrixyRulesDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.GenerateDrixyRulesUseCase.Execute(r.Context(), dto, orgID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleChangeStatusDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.ChangeStatusDrixyRulesDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.ChangeStatusDrixyRulesUseCase.Execute(r.Context(), orgID, dto)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleApplyPendingDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.RuleIdsDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.ApplyPendingDrixyRulesUseCase.Execute(r.Context(), orgID, dto, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleDiscardPendingDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.RuleIdsDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	statusDTO := dtos.ChangeStatusDrixyRulesDTO{
		TeamID:  dto.TeamID,
		RuleIDs: dto.RuleIDs,
		Status:  interfaces.DrixyRulesStatusDeleted,
	}

	res, err := c.useCases.ChangeStatusDrixyRulesUseCase.Execute(r.Context(), orgID, statusDTO)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleConvertPendingUpdatesToNew(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.RuleIdsDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.ConvertPendingUpdatesToNewUseCase.Execute(r.Context(), orgID, dto, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleCheckSyncStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")
	repoID := r.URL.Query().Get("repositoryId")

	res, err := c.useCases.CheckSyncStatusUseCase.Execute(r.Context(), orgID, teamID, repoID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleListPastReviewers(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")
	repoID := r.URL.Query().Get("repositoryId")
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))

	res, err := c.useCases.ListPastReviewersUseCase.Execute(r.Context(), orgID, teamID, repoID, months)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleSyncIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var params usecases.ResyncRulesFromIdeParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	err := c.useCases.ResyncRulesFromIdeUseCase.Execute(r.Context(), orgID, params)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (c *DrixyRulesController) handleFastSyncIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var params usecases.FastSyncIdeRulesParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.FastSyncIdeRulesUseCase.Execute(r.Context(), orgID, params)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGetGlobalSourceRepositories(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")

	res, err := c.useCases.GetGlobalRulesSourceRepositoriesUC.Execute(r.Context(), orgID, teamID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGetGlobalImportStatus(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")

	res, err := c.useCases.GetGlobalRulesImportStatusUC.Execute(r.Context(), orgID, teamID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleUpdateGlobalSourceRepositories(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var body struct {
		TeamID       string                                 `json:"teamId"`
		Repositories []interfaces.GlobalRulesSourceRepository `json:"repositories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.UpdateGlobalRulesSourceReposUC.Execute(r.Context(), orgID, body.TeamID, body.Repositories)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleResyncGlobalRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")

	res, err := c.useCases.ResyncGlobalRulesUseCase.Execute(r.Context(), orgID, teamID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGetPendingDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	repoID := r.URL.Query().Get("repositoryId")

	res, err := c.useCases.GetPendingDrixyRulesUseCase.Execute(r.Context(), orgID, repoID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleGetPendingIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	repoID := r.URL.Query().Get("repositoryId")

	res, err := c.useCases.GetPendingDrixyRulesUseCase.Execute(r.Context(), orgID, repoID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleImportFastIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, userInfo := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.ImportFastDrixyRulesDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.ImportFastDrixyRulesUseCase.Execute(r.Context(), orgID, dto, userInfo)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleReviewFastIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var dto dtos.ReviewFastDrixyRulesDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if len(dto.ActivateRuleIDs) > 0 {
		_, _ = c.useCases.ChangeStatusDrixyRulesUseCase.Execute(r.Context(), orgID, dtos.ChangeStatusDrixyRulesDTO{
			RuleIDs: dto.ActivateRuleIDs,
			Status:  interfaces.DrixyRulesStatusActive,
		})
	}
	if len(dto.DeleteRuleIDs) > 0 {
		_, _ = c.useCases.ChangeStatusDrixyRulesUseCase.Execute(r.Context(), orgID, dtos.ChangeStatusDrixyRulesDTO{
			RuleIDs: dto.DeleteRuleIDs,
			Status:  interfaces.DrixyRulesStatusDeleted,
		})
	}

	c.jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (c *DrixyRulesController) handleGetInheritedRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	repoID := r.URL.Query().Get("repositoryId")
	dirID := r.URL.Query().Get("directoryId")

	res, err := c.useCases.GetInheritedRulesUseCase.Execute(r.Context(), orgID, repoID, dirID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleResyncIdeRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}

	var params usecases.ResyncRulesFromIdeParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	err := c.useCases.ResyncRulesFromIdeUseCase.Execute(r.Context(), orgID, params)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (c *DrixyRulesController) handleManageImportedDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")

	var dto dtos.ManageImportedDrixyRulesDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		c.jsonError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	res, err := c.useCases.ManageImportedDrixyRulesUseCase.Execute(r.Context(), orgID, teamID, dto)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, res)
}

func (c *DrixyRulesController) handleCountImportedDrixyRules(w http.ResponseWriter, r *http.Request) {
	orgID, _ := c.getOrgAndUserInfo(r)
	if orgID == "" {
		c.jsonError(w, http.StatusUnauthorized, "Organization context required")
		return
	}
	teamID := r.URL.Query().Get("teamId")
	repoID := r.URL.Query().Get("repositoryId")

	counts, err := c.useCases.ManageImportedDrixyRulesUseCase.Count(r.Context(), orgID, teamID, repoID)
	if err != nil {
		c.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.jsonResponse(w, http.StatusOK, counts)
}

