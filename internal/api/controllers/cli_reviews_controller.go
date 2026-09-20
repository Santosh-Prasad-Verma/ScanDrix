package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/clireview"
)

// CliReviewsController exposes endpoints for querying historical CLI review executions.
type CliReviewsController struct {
	dashboardStore *clireview.DashboardStore
}

// NewCliReviewsController creates an initialized CLI reviews history controller.
func NewCliReviewsController(store *clireview.DashboardStore) *CliReviewsController {
	if store == nil {
		store = clireview.NewDashboardStore()
	}
	return &CliReviewsController{
		dashboardStore: store,
	}
}

// Routes mounts the /cli-reviews endpoints.
func (c *CliReviewsController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/executions", c.handleListExecutions)
	r.Get("/{executionUuid}", c.handleGetExecutionByID)

	return r
}

func (c *CliReviewsController) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing workspace or organization context"})
		return
	}

	q := r.URL.Query()
	teamID := q.Get("teamId")
	search := q.Get("search")
	sinceStr := q.Get("since")

	var since *time.Time
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = &t
		}
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	list := c.dashboardStore.GetCliReviews(clireview.CliReviewsQuery{
		OrganizationID: wsID.String(),
		TeamID:         teamID,
		StartDate:      since,
		Limit:          pageSize,
		Offset:         offset,
		Search:         search,
	})

	_ = json.NewEncoder(w).Encode(map[string]any{
		"items":    list.Items,
		"total":    list.Total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (c *CliReviewsController) handleGetExecutionByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	executionUUID := chi.URLParam(r, "executionUuid")
	if executionUUID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "executionUuid is required"})
		return
	}

	review, err := c.dashboardStore.GetCliReviewByID(executionUUID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "CLI review execution not found"})
		return
	}

	_ = json.NewEncoder(w).Encode(review)
}
