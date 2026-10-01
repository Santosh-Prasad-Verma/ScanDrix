package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// CockpitRepository defines the data contract for executive metrics and warehouse health.
type CockpitRepository interface {
	CountActiveReviews(ctx context.Context, wsID uuid.UUID) (int, error)
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
}

// AnalyticsRepository computes workspace metrics from real data. A metric that
// cannot be derived is reported through the response's Unavailable list instead
// of being filled with a constant.
type AnalyticsRepository interface {
	GetCockpitOverview(ctx context.Context, wsID uuid.UUID) (*models.CockpitOverview, error)
	GetDoraMetrics(ctx context.Context, wsID uuid.UUID) (*models.DoraMetrics, error)
	GetProductivityMetrics(ctx context.Context, wsID uuid.UUID) (*models.ProductivityMetrics, error)
	GetCodeHealthMetrics(ctx context.Context, wsID uuid.UUID) (*models.CodeHealthMetrics, error)
	ListCodeHotspotFiles(ctx context.Context, wsID uuid.UUID, limit int) ([]models.CodeHotspotFile, error)
}

// CockpitController provides enterprise security cockpit, code health, and productivity endpoints.
type CockpitController struct {
	repo      CockpitRepository
	analytics AnalyticsRepository
}

// NewCockpitController creates the cockpit controller.
func NewCockpitController(repo CockpitRepository) *CockpitController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &CockpitController{repo: repo}
}

// WithAnalytics attaches the real analytics repository. Without it the metric
// endpoints answer 503 rather than serving placeholder numbers.
func (c *CockpitController) WithAnalytics(a AnalyticsRepository) *CockpitController {
	if isNilInterface(a) {
		c.analytics = nil
		return c
	}
	c.analytics = a
	return c
}

// requireAnalytics resolves the workspace and the analytics repository, writing
// the error response itself when either is unavailable. It fails closed: a
// missing analytics source produces a 503, never a fabricated payload.
func (c *CockpitController) requireAnalytics(w http.ResponseWriter, r *http.Request) (uuid.UUID, AnalyticsRepository, bool) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		writeCockpitError(w, http.StatusUnauthorized, "missing workspace context")
		return uuid.Nil, nil, false
	}
	if c.analytics == nil {
		writeCockpitError(w, http.StatusServiceUnavailable,
			"analytics is not available on this deployment")
		return uuid.Nil, nil, false
	}
	return wsID, c.analytics, true
}

func writeCockpitError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Routes mounts the /cockpit routes.
func (c *CockpitController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/overview", c.handleGetOverview)

	return r
}

// CodeHealthRoutes mounts /code-health routes.
func (c *CockpitController) CodeHealthRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/overview", c.handleCodeHealthOverview)
	r.Get("/files", c.handleCodeHealthFiles)

	return r
}

// ProductivityRoutes mounts /productivity routes.
func (c *CockpitController) ProductivityRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/overview", c.handleProductivityOverview)
	r.Get("/dora", c.handleProductivityDORA)

	return r
}

func (c *CockpitController) handleGetOverview(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}

	overview, err := analytics.GetCockpitOverview(r.Context(), wsID)
	if err != nil {
		slog.Error("Cockpit overview query failed", "error", err)
		writeCockpitError(w, http.StatusInternalServerError, "failed querying cockpit overview")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(overview)
}

func (c *CockpitController) handleCodeHealthOverview(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}

	metrics, err := analytics.GetCodeHealthMetrics(r.Context(), wsID)
	if err != nil {
		slog.Error("Code health query failed", "error", err)
		writeCockpitError(w, http.StatusInternalServerError, "failed querying code health metrics")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

func (c *CockpitController) handleCodeHealthFiles(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	files, err := analytics.ListCodeHotspotFiles(r.Context(), wsID, limit)
	if err != nil {
		slog.Error("Code hotspot query failed", "error", err)
		writeCockpitError(w, http.StatusInternalServerError, "failed querying code hotspot files")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(files)
}

func (c *CockpitController) handleProductivityOverview(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}

	metrics, err := analytics.GetProductivityMetrics(r.Context(), wsID)
	if err != nil {
		slog.Error("Productivity query failed", "error", err)
		writeCockpitError(w, http.StatusInternalServerError, "failed querying productivity metrics")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

func (c *CockpitController) handleProductivityDORA(w http.ResponseWriter, r *http.Request) {
	wsID, analytics, ok := c.requireAnalytics(w, r)
	if !ok {
		return
	}

	metrics, err := analytics.GetDoraMetrics(r.Context(), wsID)
	if err != nil {
		slog.Error("DORA query failed", "error", err)
		writeCockpitError(w, http.StatusInternalServerError, "failed querying DORA metrics")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}
