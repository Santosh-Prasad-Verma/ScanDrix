package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// WorkspaceController handles multi-tenant workspace management and executive metrics.
type WorkspaceController struct {
	repo *database.Repository
}

// NewWorkspaceController initializes the workspace controller.
func NewWorkspaceController(repo *database.Repository) *WorkspaceController {
	return &WorkspaceController{repo: repo}
}

// Routes mounts workspace routes.
func (c *WorkspaceController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleCreateWorkspace)
	r.Get("/current", c.handleGetCurrentWorkspace)
	r.Get("/cockpit", c.handleGetCockpitMetrics)

	return r
}

func (c *WorkspaceController) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req dtos.CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"workspace name is required"}`, http.StatusBadRequest)
		return
	}

	ws := models.Workspace{
		Name:   req.Name,
		Slug:   req.Slug,
		Status: models.TenantStatusActive,
	}

	if err := c.repo.CreateWorkspace(r.Context(), &ws); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.WorkspaceResponse{
		ID:        ws.ID,
		Name:      ws.Name,
		Slug:      ws.Slug,
		Status:    ws.Status,
		CreatedAt: ws.CreatedAt,
	})
}

func (c *WorkspaceController) handleGetCurrentWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.WorkspaceResponse{
		ID:        wsID,
		Name:      "Production Workspace",
		Slug:      "production",
		Status:    models.TenantStatusActive,
		CreatedAt: time.Now().UTC(),
	})
}

func (c *WorkspaceController) handleGetCockpitMetrics(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	metrics := &models.CockpitMetrics{
		PassRatePercentage: 100.0,
	}
	if c.repo != nil {
		m, err := c.repo.GetCockpitMetrics(r.Context(), wsID)
		if err == nil && m != nil {
			metrics = m
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.CockpitMetricsResponse{
		TotalReviews:       metrics.TotalReviews,
		TotalFindings:      metrics.TotalFindings,
		CriticalFindings:   metrics.CriticalFindings,
		HighFindings:       metrics.HighFindings,
		PassRatePercentage: metrics.PassRatePercentage,
		ActiveRepositories: metrics.ActiveRepositories,
		TotalDevelopers:    metrics.TotalDevelopers,
	})
}

