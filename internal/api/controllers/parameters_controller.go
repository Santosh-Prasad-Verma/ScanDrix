package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
)

// ParametersController handles review settings, model selections, and organizational thresholds.
type ParametersController struct{}

// NewParametersController initializes the parameters controller.
func NewParametersController() *ParametersController {
	return &ParametersController{}
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.ReviewParametersDTO{
		DefaultAIModel:           "claude-3-7-sonnet",
		MaxDiffLines:             1500,
		DryRunMode:               false,
		AutoApproveCleanPRs:      true,
		EnforceConventionalTitle: true,
		IgnorePatterns:           []string{"*.generated.*", "vendor/*", "*.pb.go"},
		CustomSystemPrompt:       "Prioritize finding concurrency races, missing input validation, and unauthorized data leakage.",
	})
}

func (c *ParametersController) handleUpdateReviewParameters(w http.ResponseWriter, r *http.Request) {
	var req dtos.ReviewParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid parameters payload"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

func (c *ParametersController) handleGetOrgParameters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.OrgParametersDTO{
		BlockPRMergeOnCritical:    true,
		RequireReviewDismissalRole: "ADMIN",
		DefaultBranchOnly:         false,
		NotificationSlackChannel:  "#security-reviews",
	})
}

func (c *ParametersController) handleUpdateOrgParameters(w http.ResponseWriter, r *http.Request) {
	var req dtos.OrgParametersDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid org parameters payload"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}
