package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
)

// UsageRepository defines the data contract for AI token usage and quota metrics (Clean Architecture).
type UsageRepository interface {
	GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (promptTokens, completionTokens int64, costUSD float64, err error)
	GetLiveTokenQuota(ctx context.Context, wsID uuid.UUID) (*database.LiveQuotaStatus, error)
	GetWorkspaceDailyUsageHistory(ctx context.Context, wsID uuid.UUID, days int) ([]database.DailyUsageSummary, error)
	UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error
}

// UsageController manages AI token metering, live quota headroom, and spend caps.
type UsageController struct {
	repo UsageRepository
}

// NewUsageController initializes the usage controller with database repository.
func NewUsageController(repo UsageRepository) *UsageController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &UsageController{repo: repo}
}

// Routes mounts usage endpoints.
func (c *UsageController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetUsage)
	r.Get("/quota", c.handleGetQuota)
	r.Get("/history", c.handleGetUsageHistory)
	r.Put("/spend-limit", c.handleUpdateSpendLimit)

	return r
}

func (c *UsageController) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	var promptTokens, completionTokens int64
	var costUSD float64

	if c.repo != nil {
		var err error
		promptTokens, completionTokens, costUSD, err = c.repo.GetWorkspaceUsage(r.Context(), wsID, startOfMonth)
		if err != nil {
			http.Error(w, `{"error":"failed querying token usage"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.TokenUsageResponse{
		TotalPromptTokens:     promptTokens,
		TotalCompletionTokens: completionTokens,
		TotalTokens:           promptTokens + completionTokens,
		EstimatedCostUSD:      costUSD,
		BillingPeriodStart:    startOfMonth,
		BillingPeriodEnd:      startOfMonth.AddDate(0, 1, 0),
	})
}

func (c *UsageController) handleGetQuota(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var quota *database.LiveQuotaStatus
	if c.repo != nil {
		var err error
		quota, err = c.repo.GetLiveTokenQuota(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed evaluating live token quota"}`, http.StatusInternalServerError)
			return
		}
	}

	if quota == nil {
		now := time.Now().UTC()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		quota = &database.LiveQuotaStatus{
			Tier:               "COMMUNITY",
			MonthlyTokenLimit:  500000,
			BurstLimitPerMin:   60,
			BillingPeriodStart: startOfMonth,
			BillingPeriodEnd:   startOfMonth.AddDate(0, 1, 0),
		}
	}

	isExhausted := !quota.BYOKEnabled && quota.MonthlyTokenLimit > 0 && quota.TokensUsedThisMonth >= quota.MonthlyTokenLimit

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.LiveQuotaResponse{
		Tier:                quota.Tier,
		BYOKEnabled:         quota.BYOKEnabled,
		MonthlyTokenLimit:   quota.MonthlyTokenLimit,
		TokensUsedThisMonth: quota.TokensUsedThisMonth,
		TokensRemaining:     quota.TokensRemaining,
		PercentUsed:         quota.PercentUsed,
		EstimatedCostUSD:    quota.EstimatedCostUSD,
		BurstLimitPerMin:    quota.BurstLimitPerMin,
		IsExhausted:         isExhausted,
		BillingPeriodStart:  quota.BillingPeriodStart,
		BillingPeriodEnd:    quota.BillingPeriodEnd,
	})
}

func (c *UsageController) handleGetUsageHistory(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	days := 30
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	var history []database.DailyUsageSummary
	if c.repo != nil {
		var err error
		history, err = c.repo.GetWorkspaceDailyUsageHistory(r.Context(), wsID, days)
		if err != nil {
			http.Error(w, `{"error":"failed querying usage history"}`, http.StatusInternalServerError)
			return
		}
	}

	var totalTokens int64
	var totalCost float64
	points := make([]dtos.DailyUsagePoint, 0, len(history))
	for _, h := range history {
		totalTokens += h.TotalTokens
		totalCost += h.CostUSD
		points = append(points, dtos.DailyUsagePoint{
			Date:             h.Date,
			PromptTokens:     h.PromptTokens,
			CompletionTokens: h.CompletionTokens,
			TotalTokens:      h.TotalTokens,
			CostUSD:          h.CostUSD,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.UsageHistoryResponse{
		Days:         days,
		DailyUsage:   points,
		TotalTokens:  totalTokens,
		TotalCostUSD: totalCost,
	})
}

func (c *UsageController) handleUpdateSpendLimit(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.UpdateSpendLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MonthlySpendLimitUSD < 0 {
		http.Error(w, `{"error":"invalid spend limit json"}`, http.StatusBadRequest)
		return
	}

	if c.repo != nil {
		if err := c.repo.UpdateSpendLimit(r.Context(), wsID, req.MonthlySpendLimitUSD); err != nil {
			http.Error(w, `{"error":"failed updating spend limit"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "UPDATED",
		"spend_limit_usd": req.MonthlySpendLimitUSD,
	})
}
