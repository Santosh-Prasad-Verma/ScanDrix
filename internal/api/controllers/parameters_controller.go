package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
)

// ParametersController handles review settings, model selections, and organizational thresholds.
type ParametersController struct {
	repo *database.Repository
}

// NewParametersController initializes the parameters controller with database persistence.
func NewParametersController(repo *database.Repository) *ParametersController {
	return &ParametersController{repo: repo}
}

// Routes mounts parameters routes.
func (c *ParametersController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/review", c.handleGetReviewParameters)
	r.Put("/review", c.handleUpdateReviewParameters)
	r.Get("/org", c.handleGetOrgParameters)
	r.Put("/org", c.handleUpdateOrgParameters)

	return r
}

func (c *ParametersController) handleGetReviewParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	reviewBytes, _, err := c.repo.GetWorkspaceParameters(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed loading parameters"}`, http.StatusInternalServerError)
		return
	}

	var dto dtos.ReviewParametersDTO
	if len(reviewBytes) > 2 {
		_ = json.Unmarshal(reviewBytes, &dto)
	} else {
		// Defaults
		dto = dtos.ReviewParametersDTO{
			DefaultAIModel:           "claude-3-7-sonnet",
			MaxDiffLines:             1500,
			DryRunMode:               false,
			AutoApproveCleanPRs:      true,
			EnforceConventionalTitle: true,
			IgnorePatterns:           []string{"*.generated.*", "vendor/*", "*.pb.go"},
			CustomSystemPrompt:       "Prioritize finding concurrency races, missing input validation, and unauthorized data leakage.",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

func (c *ParametersController) handleUpdateReviewParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.ReviewParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error":"failed serializing parameters"}`, http.StatusInternalServerError)
		return
	}

	if err := c.repo.UpdateWorkspaceReviewParameters(r.Context(), wsID, bytes); err != nil {
		http.Error(w, `{"error":"failed updating parameters"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

func (c *ParametersController) handleGetOrgParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	_, orgBytes, err := c.repo.GetWorkspaceParameters(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed loading org parameters"}`, http.StatusInternalServerError)
		return
	}

	var dto dtos.OrgParametersDTO
	if len(orgBytes) > 2 {
		_ = json.Unmarshal(orgBytes, &dto)
	} else {
		// Defaults
		dto = dtos.OrgParametersDTO{
			BlockPRMergeOnCritical:    true,
			RequireReviewDismissalRole: "ADMIN",
			DefaultBranchOnly:         false,
			NotificationSlackChannel:  "#security-reviews",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

func (c *ParametersController) handleUpdateOrgParameters(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.OrgParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid org parameters payload"}`, http.StatusBadRequest)
		return
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		http.Error(w, `{"error":"failed serializing org parameters"}`, http.StatusInternalServerError)
		return
	}

	if err := c.repo.UpdateWorkspaceOrgParameters(r.Context(), wsID, bytes); err != nil {
		http.Error(w, `{"error":"failed updating org parameters"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}
