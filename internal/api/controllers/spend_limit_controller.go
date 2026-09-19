package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	repo SpendLimitRepository
}

// NewSpendLimitController creates the spend limit controller.
func NewSpendLimitController(repo SpendLimitRepository) *SpendLimitController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &SpendLimitController{repo: repo}
}

// Routes mounts the /spend-limit endpoints.
func (c *SpendLimitController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/status", c.handleGetStatus)
	r.Get("/", c.handleGetConfig)
	r.Post("/", c.handleConfigureSpendLimit)

	return r
}

func (c *SpendLimitController) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.SpendLimitEvaluation{
			SpentUSD:            0.0,
			LimitUSD:            250.0,
			PercentageUsed:      0.0,
			IsOverLimit:         false,
			MonthlyBudgetUSD:    250.0,
			AlertThresholdPct:   85.0,
			NotificationEnabled: true,
		})
		return
	}

	status, err := c.repo.GetSpendLimitStatus(r.Context(), wsID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed evaluating spend limit status: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (c *SpendLimitController) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	limitUSD := 250.0
	if c.repo != nil {
		if status, err := c.repo.GetSpendLimitStatus(r.Context(), wsID); err == nil && status != nil {
			limitUSD = status.LimitUSD
		}
	}

	// Model prices table
	prices := []models.ModelPriceInfo{
		{Provider: "anthropic", Model: "claude-3-5-sonnet", InputPrice1M: 3.00, OutputPrice1M: 15.00, CachedInput1M: 0.30},
		{Provider: "anthropic", Model: "claude-3-5-haiku", InputPrice1M: 0.80, OutputPrice1M: 4.00, CachedInput1M: 0.08},
		{Provider: "openai", Model: "gpt-4o", InputPrice1M: 2.50, OutputPrice1M: 10.00, CachedInput1M: 1.25},
		{Provider: "openai", Model: "gpt-4o-mini", InputPrice1M: 0.15, OutputPrice1M: 0.60, CachedInput1M: 0.075},
		{Provider: "gemini", Model: "gemini-2.5-flash", InputPrice1M: 0.075, OutputPrice1M: 0.30, CachedInput1M: 0.018},
		{Provider: "deepseek", Model: "deepseek-coder", InputPrice1M: 0.14, OutputPrice1M: 0.28, CachedInput1M: 0.014},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspaceId":         wsID.String(),
		"monthlyLimitUSD":     limitUSD,
		"alertThresholdPct":   85.0,
		"notificationEnabled": true,
		"modelPrices":         prices,
	})
}

func (c *SpendLimitController) handleConfigureSpendLimit(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
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
		if err := c.repo.UpdateSpendLimit(r.Context(), wsID, req.LimitUSD); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed updating spend limit: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":             true,
		"workspaceId":         wsID.String(),
		"limitUsd":            req.LimitUSD,
		"alertThresholdPct":   req.AlertThresholdPct,
		"notificationEnabled": req.NotificationEnabled,
		"updatedAt":           time.Now().UTC().Format(time.RFC3339),
	})
}
