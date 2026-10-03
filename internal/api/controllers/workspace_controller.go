package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/pkg/models"
)

// WorkspaceRepository abstracts workspace persistence operations adhering to clean architecture.
type WorkspaceRepository interface {
	CreateWorkspace(ctx context.Context, ws *models.Workspace) error
	GetWorkspaceByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error)
	GetCockpitMetrics(ctx context.Context, id uuid.UUID) (*models.CockpitMetrics, error)
}

// WorkspaceController handles multi-tenant workspace management and executive metrics.
type WorkspaceController struct {
	repo WorkspaceRepository
}

// NewWorkspaceController initializes the workspace controller.
func NewWorkspaceController(repo WorkspaceRepository) *WorkspaceController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &WorkspaceController{repo: repo}
}

// Routes mounts workspace routes.
func (c *WorkspaceController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", c.handleCreateWorkspace)
	r.Get("/current", c.handleGetCurrentWorkspace)
	r.Get("/cockpit", c.handleGetCockpitMetrics)
	r.Get("/attestation-key", c.handleGetAttestationPublicKey)

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
		slog.Error("Failed creating workspace", "name", req.Name, "error", err)
		http.Error(w, `{"error":"failed creating workspace"}`, http.StatusInternalServerError)
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

	if c.repo != nil {
		ws, err := c.repo.GetWorkspaceByID(r.Context(), wsID)
		if err == nil && ws != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dtos.WorkspaceResponse{
				ID:        ws.ID,
				Name:      ws.Name,
				Slug:      ws.Slug,
				Status:    ws.Status,
				CreatedAt: ws.CreatedAt,
			})
			return
		}
	}

	http.Error(w, `{"error":"workspace not found"}`, http.StatusNotFound)
}

func (c *WorkspaceController) handleGetCockpitMetrics(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	// Fail closed. This handler previously pre-seeded a 100.0 pass rate and
	// then ignored both a nil repository and a query error, so a database
	// outage was reported to the customer as a flawless security posture
	// (AUDIT_REMEDIATION.md F-07).
	if c.repo == nil {
		http.Error(w, `{"error":"cockpit metrics unavailable: no data source"}`, http.StatusServiceUnavailable)
		return
	}

	metrics, err := c.repo.GetCockpitMetrics(r.Context(), wsID)
	if err != nil {
		slog.Error("cockpit.metrics.query_failed", "workspace_id", wsID, "error", err)
		http.Error(w, `{"error":"cockpit metrics unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if metrics == nil {
		http.Error(w, `{"error":"cockpit metrics unavailable"}`, http.StatusServiceUnavailable)
		return
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
		Unavailable:        metrics.Unavailable,
	})
}

func (c *WorkspaceController) handleGetAttestationPublicKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil || wsID == uuid.Nil {
		http.Error(w, `{"error":"unauthorized: workspace context required"}`, http.StatusUnauthorized)
		return
	}

	masterSecret := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
	if masterSecret == "" {
		masterSecret = os.Getenv("KMS_MASTER_KEY")
	}
	_, pubKey, keyID := intoto.DeriveTenantKeypair(wsID, masterSecret)

	pemStr, err := intoto.ExportPublicKeyPEM(pubKey)
	if err != nil {
		http.Error(w, `{"error":"failed formatting public key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspace_id": wsID,
		"key_id":       keyID,
		"algorithm":    "Ed25519",
		"public_key":   pemStr,
	})
}
