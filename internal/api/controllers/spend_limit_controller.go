package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// SpendLimitRepository defines data access for spend-limit configuration and live token usage.
type SpendLimitRepository interface {
	GetSpendLimitStatus(ctx context.Context, wsID uuid.UUID) (*models.SpendLimitEvaluation, error)
	GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (promptTokens, completionTokens int64, costUSD float64, err error)
	UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error
}

// SpendLimitController manages BYOK spend limits and token price transparency.
type SpendLimitController struct {
	repo             SpendLimitRepository
	configService    *spendlimit.SpendLimitConfigService
	configureUseCase *spendlimit.ConfigureSpendLimitUseCase
	getConfigUseCase *spendlimit.GetSpendLimitConfigUseCase
}

// NewSpendLimitController creates the spend limit controller.
func NewSpendLimitController(repo SpendLimitRepository) *SpendLimitController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &SpendLimitController{repo: repo}
}

// WithSpendLimitServices connects the full analytics spend limit domain services and use cases.
func (c *SpendLimitController) WithSpendLimitServices(
	configService *spendlimit.SpendLimitConfigService,
	configureUC *spendlimit.ConfigureSpendLimitUseCase,
	getConfigUC *spendlimit.GetSpendLimitConfigUseCase,
) *SpendLimitController {
	c.configService = configService
	c.configureUseCase = configureUC
	c.getConfigUseCase = getConfigUC
	return c
}

// Routes mounts the /spend-limit endpoints.
func (c *SpendLimitController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/status", c.handleGetStatus)
	r.Get("/", c.handleGetConfig)
	r.Post("/", c.handleConfigureSpendLimit)

	return r
}

func (c *SpendLimitController) resolveOrgAndTeam(r *http.Request) (string, string) {
	orgID := r.URL.Query().Get("organizationId")
	teamID := r.URL.Query().Get("teamId")

	if orgID == "" {
		if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil {
			orgID = wsID.String()
		}
	}
	return orgID, teamID
}

func (c *SpendLimitController) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	orgID, teamID := c.resolveOrgAndTeam(r)
	if orgID == "" {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.configService != nil {
		eval, err := c.configService.Evaluate(r.Context(), orgID, teamID, time.Now().UTC())
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed evaluating spend limit status: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if eval == nil {
			// No limit is configured. A bare null cannot distinguish that from a
			// broken evaluation, so the state is stated explicitly and the
			// counters are reported as absent rather than as a measured zero.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"configured": false,
				"reason":     "no spend limit is configured for this workspace",
			})
			return
		}
		// The evaluation already serialises to camelCase, so it is returned whole
		// with a configured marker rather than re-projected field by field.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configured": true,
			"status":     eval,
		})
		return
	}

	if c.repo == nil {
		// Without a data source the spend status cannot be measured. Reporting a
		// $250 budget here would present an invented budget as a real one.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "spend status is not available on this deployment",
			"configured": false,
		})
		return
	}

	wsUUID, _ := uuid.Parse(orgID)
	status, err := c.repo.GetSpendLimitStatus(r.Context(), wsUUID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed evaluating spend limit status: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (c *SpendLimitController) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	orgID, teamID := c.resolveOrgAndTeam(r)
	if orgID == "" {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.getConfigUseCase != nil {
		view, err := c.getConfigUseCase.Execute(r.Context(), orgID, teamID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed getting spend limit config: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
		return
	}

	limitUSD := 250.0
	if c.repo != nil {
		wsUUID, _ := uuid.Parse(orgID)
		if status, err := c.repo.GetSpendLimitStatus(r.Context(), wsUUID); err == nil && status != nil {
			limitUSD = status.LimitUSD
		}
	}

	prices := []models.ModelPriceInfo{
		{Provider: "anthropic", Model: "claude-3-5-sonnet", InputPrice1M: 3.00, OutputPrice1M: 15.00, CachedInput1M: 0.30},
		{Provider: "anthropic", Model: "claude-3-5-haiku", InputPrice1M: 0.80, OutputPrice1M: 4.00, CachedInput1M: 0.08},
		{Provider: "openai", Model: "gpt-4o", InputPrice1M: 2.50, OutputPrice1M: 10.00, CachedInput1M: 1.25},
		{Provider: "openai", Model: "gpt-4o-mini", InputPrice1M: 0.15, OutputPrice1M: 0.60, CachedInput1M: 0.075},
		{Provider: "google", Model: "gemini-2.5-pro", InputPrice1M: 1.25, OutputPrice1M: 5.00, CachedInput1M: 0.3125},
		{Provider: "deepseek", Model: "deepseek-coder", InputPrice1M: 0.14, OutputPrice1M: 0.28, CachedInput1M: 0.014},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspaceId":         orgID,
		"monthlyLimitUSD":     limitUSD,
		"alertThresholdPct":   85.0,
		"notificationEnabled": true,
		"modelPrices":         prices,
	})
}

func (c *SpendLimitController) handleConfigureSpendLimit(w http.ResponseWriter, r *http.Request) {
	orgID, teamID := c.resolveOrgAndTeam(r)
	if orgID == "" {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.configureUseCase != nil {
		var input spendlimit.ConfigureSpendLimitInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid spend limit request payload"}`, http.StatusBadRequest)
			return
		}
		input.OrganizationID = orgID
		input.TeamID = teamID

		cfg, err := c.configureUseCase.Execute(r.Context(), input)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
		return
	}

	var req struct {
		LimitUSD            float64 `json:"limitUsd"`
		AlertThresholdPct   float64 `json:"alertThresholdPct"`
		NotificationEnabled bool    `json:"notificationEnabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid spend limit request payload"}`, http.StatusBadRequest)
		return
	}

	if req.LimitUSD <= 0 {
		http.Error(w, `{"error":"limitUsd must be greater than zero"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		wsUUID, _ := uuid.Parse(orgID)
		if err := c.repo.UpdateSpendLimit(r.Context(), wsUUID, req.LimitUSD); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed updating spend limit: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":             true,
		"workspaceId":         orgID,
		"limitUsd":            req.LimitUSD,
		"alertThresholdPct":   req.AlertThresholdPct,
		"notificationEnabled": req.NotificationEnabled,
		"updatedAt":           time.Now().UTC().Format(time.RFC3339),
	})
}
