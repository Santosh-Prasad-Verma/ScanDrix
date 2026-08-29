package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
)

// UsageController manages AI token metering and spend caps.
type UsageController struct{}

// NewUsageController initializes the usage controller.
func NewUsageController() *UsageController {
	return &UsageController{}
}

// Routes mounts usage endpoints.
func (c *UsageController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetUsage)
	r.Put("/spend-limit", c.handleUpdateSpendLimit)

	return r
}

func (c *UsageController) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TokenUsageResponse{
		TotalPromptTokens:     1420000,
		TotalCompletionTokens: 380000,
		TotalTokens:           1800000,
		EstimatedCostUSD:      5.40,
		BillingPeriodStart:    startOfMonth,
		BillingPeriodEnd:      startOfMonth.AddDate(0, 1, 0),
	})
}

func (c *UsageController) handleUpdateSpendLimit(w http.ResponseWriter, r *http.Request) {
	var req dtos.UpdateSpendLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid spend limit json"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "UPDATED",
		"spend_limit_usd": req.MonthlySpendLimitUSD,
	})
}
