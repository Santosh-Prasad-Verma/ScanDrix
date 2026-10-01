package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/spendlimit"
	"github.com/scandrix/backend/internal/analytics/usage"
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

// UsageController manages AI token metering, live quota headroom, spend caps, and token analytics.
type UsageController struct {
	repo                    UsageRepository
	tokenUsageService       usage.ITokenUsageService
	summaryUseCase          *usage.BuildUsageSummaryUseCase
	estimateUseCase         *usage.CostEstimateUseCase
	developerUseCase        *usage.TokensByDeveloperUseCase
	pricingCatalog          *pricing.TokenPricingCatalog
	spendLimitConfigService *spendlimit.SpendLimitConfigService
}

// NewUsageController initializes the usage controller with database repository.
func NewUsageController(repo UsageRepository) *UsageController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &UsageController{repo: repo}
}

// WithAnalyticsServices injects rich analytics use cases and services into the controller.
func (c *UsageController) WithAnalyticsServices(
	tokenUsageService usage.ITokenUsageService,
	summaryUC *usage.BuildUsageSummaryUseCase,
	estimateUC *usage.CostEstimateUseCase,
	devUC *usage.TokensByDeveloperUseCase,
	pricingCatalog *pricing.TokenPricingCatalog,
	spendLimitCfgSvc *spendlimit.SpendLimitConfigService,
) *UsageController {
	c.tokenUsageService = tokenUsageService
	c.summaryUseCase = summaryUC
	c.estimateUseCase = estimateUC
	c.developerUseCase = devUC
	c.pricingCatalog = pricingCatalog
	c.spendLimitConfigService = spendLimitCfgSvc
	return c
}

// Routes mounts usage and token analytics endpoints.
func (c *UsageController) Routes() chi.Router {
	r := chi.NewRouter()

	// Legacy Workspace Metering Endpoints
	r.Get("/", c.handleGetUsage)
	r.Get("/quota", c.handleGetQuota)
	r.Get("/history", c.handleGetUsageHistory)
	r.Put("/spend-limit", c.handleUpdateSpendLimit)

	// Rich Token Analytics Endpoints Scandrix
	r.Get("/tokens/summary", c.handleGetTokenSummary)
	r.Get("/tokens/overview", c.handleGetTokenOverview)
	r.Get("/tokens/estimate", c.handleGetTokenEstimate)
	r.Get("/tokens/developer", c.handleGetTokenDeveloper)
	r.Get("/tokens/pricing", c.handleGetTokenPricing)
	r.Get("/tokens/daily", c.handleGetTokenDaily)
	r.Get("/tokens/pr", c.handleGetTokenByPr)
	r.Get("/tokens/review", c.handleGetTokenByReview)
	r.Get("/tokens/area", c.handleGetTokenByArea)

	return r
}

func (c *UsageController) parseQuery(r *http.Request) usage.TokenUsageQueryContract {
	orgID := r.URL.Query().Get("organizationId")
	if orgID == "" {
		if wsID, err := auth.WorkspaceFromContext(r.Context()); err == nil {
			orgID = wsID.String()
		}
	}

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := now

	if s := r.URL.Query().Get("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			start = t
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			end = t
		}
	}

	byok := r.URL.Query().Get("byok") == "true"
	models := r.URL.Query().Get("models")
	tz := r.URL.Query().Get("timezone")
	dev := r.URL.Query().Get("developer")
	repoID := r.URL.Query().Get("repositoryId")

	var prNumber *int
	if prStr := r.URL.Query().Get("prNumber"); prStr != "" {
		if n, err := strconv.Atoi(prStr); err == nil {
			prNumber = &n
		}
	}

	return usage.TokenUsageQueryContract{
		OrganizationID: orgID,
		Start:          start,
		End:            end,
		BYOK:           byok,
		Models:         models,
		Timezone:       tz,
		Developer:      dev,
		RepositoryID:   repoID,
		PRNumber:       prNumber,
	}
}

func (c *UsageController) handleGetTokenSummary(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if q.OrganizationID == "" {
		http.Error(w, `{"error":"missing organizationId context"}`, http.StatusBadRequest)
		return
	}

	var overrides pricing.ManualPricingOverrides
	if c.spendLimitConfigService != nil {
		if cfg, _ := c.spendLimitConfigService.GetConfig(r.Context(), q.OrganizationID, ""); cfg != nil {
			overrides = cfg.ModelPricing
		}
	}

	if c.summaryUseCase != nil {
		report, err := c.summaryUseCase.Execute(r.Context(), q, overrides)
		if err != nil {
			http.Error(w, `{"error":"failed building usage summary"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(usage.UsageSummaryReportContract{
		Totals:    usage.BaseUsageContract{Model: "total", Input: 0, Output: 0, Total: 0},
		TotalCost: pricing.CostBreakdown{},
		ByModel:   []usage.EnrichedModelUsage{},
	})
}

func (c *UsageController) handleGetTokenOverview(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if q.OrganizationID == "" {
		http.Error(w, `{"error":"missing organizationId context"}`, http.StatusBadRequest)
		return
	}

	var overrides pricing.ManualPricingOverrides
	if c.spendLimitConfigService != nil {
		if cfg, _ := c.spendLimitConfigService.GetConfig(r.Context(), q.OrganizationID, ""); cfg != nil {
			overrides = cfg.ModelPricing
		}
	}

	if c.summaryUseCase != nil {
		report, err := c.summaryUseCase.ExecuteOverview(r.Context(), q, overrides)
		if err != nil {
			http.Error(w, `{"error":"failed building usage overview"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(usage.UsageOverviewReportContract{})
}

func (c *UsageController) handleGetTokenEstimate(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if q.OrganizationID == "" {
		http.Error(w, `{"error":"missing organizationId context"}`, http.StatusBadRequest)
		return
	}

	if c.estimateUseCase != nil {
		est, err := c.estimateUseCase.Execute(r.Context(), q.OrganizationID)
		if err != nil {
			http.Error(w, `{"error":"failed building cost estimate"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(est)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(usage.CostEstimateContract{
		PeriodDays:     14,
		ProjectionDays: 30,
		DeveloperCount: 1,
	})
}

func (c *UsageController) handleGetTokenDeveloper(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if q.OrganizationID == "" {
		http.Error(w, `{"error":"missing organizationId context"}`, http.StatusBadRequest)
		return
	}

	daily := r.URL.Query().Get("daily") == "true"

	if c.developerUseCase != nil {
		if daily {
			res, err := c.developerUseCase.ExecuteDaily(r.Context(), q)
			if err != nil {
				http.Error(w, `{"error":"failed querying daily developer usage"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}

		res, err := c.developerUseCase.ExecuteAggregated(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"failed querying developer usage"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]usage.UsageByDeveloperResultContract{})
}

func (c *UsageController) handleGetTokenPricing(w http.ResponseWriter, r *http.Request) {
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))

	if c.pricingCatalog != nil {
		info := c.pricingCatalog.Execute(r.Context(), model, provider)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": model, "provider": provider})
}

func (c *UsageController) handleGetTokenDaily(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if c.tokenUsageService != nil {
		res, err := c.tokenUsageService.GetDailyUsage(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"failed querying daily usage"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]usage.DailyUsageResultContract{})
}

func (c *UsageController) handleGetTokenByPr(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if c.tokenUsageService != nil {
		res, err := c.tokenUsageService.GetUsageByPr(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"failed querying usage by pr"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]usage.UsageByPrResultContract{})
}

func (c *UsageController) handleGetTokenByReview(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if c.tokenUsageService != nil {
		res, err := c.tokenUsageService.GetUsageByReview(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"failed querying usage by review"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]usage.UsageByReviewResultContract{})
}

func (c *UsageController) handleGetTokenByArea(w http.ResponseWriter, r *http.Request) {
	q := c.parseQuery(r)
	if c.tokenUsageService != nil {
		res, err := c.tokenUsageService.GetUsageByArea(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"failed querying usage by area"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]usage.UsageByAreaResultContract{})
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
