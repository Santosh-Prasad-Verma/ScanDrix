package dtos

import (
	"time"
)

// TokenUsageResponse reports AI token consumption metrics.
type TokenUsageResponse struct {
	TotalPromptTokens     int64     `json:"total_prompt_tokens"`
	TotalCompletionTokens int64     `json:"total_completion_tokens"`
	TotalTokens           int64     `json:"total_tokens"`
	EstimatedCostUSD      float64   `json:"estimated_cost_usd"`
	BillingPeriodStart    time.Time `json:"billing_period_start"`
	BillingPeriodEnd      time.Time `json:"billing_period_end"`
}

// UpdateSpendLimitRequest sets monthly spending limits and budget alerts.
type UpdateSpendLimitRequest struct {
	MonthlySpendLimitUSD float64 `json:"monthly_spend_limit_usd"`
	AlertThreshold80     bool    `json:"alert_threshold_80"`
	AlertThreshold100    bool    `json:"alert_threshold_100"`
	HardStopOnCap        bool    `json:"hard_stop_on_cap"`
}

// LiveQuotaResponse provides real-time multi-tenant token quotas, BYOK status, and headroom.
type LiveQuotaResponse struct {
	Tier                string    `json:"tier"`
	BYOKEnabled         bool      `json:"byok_enabled"`
	MonthlyTokenLimit   int64     `json:"monthly_token_limit"`
	TokensUsedThisMonth int64     `json:"tokens_used_this_month"`
	TokensRemaining     int64     `json:"tokens_remaining"`
	PercentUsed         float64   `json:"percent_used"`
	EstimatedCostUSD    float64   `json:"estimated_cost_usd"`
	BurstLimitPerMin    int64     `json:"burst_limit_per_min"`
	IsExhausted         bool      `json:"is_exhausted"`
	BillingPeriodStart  time.Time `json:"billing_period_start"`
	BillingPeriodEnd    time.Time `json:"billing_period_end"`
}

// DailyUsagePoint holds token telemetry for a single day.
type DailyUsagePoint struct {
	Date             string  `json:"date"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// UsageHistoryResponse reports historical usage data over a lookback window.
type UsageHistoryResponse struct {
	Days         int               `json:"days"`
	DailyUsage   []DailyUsagePoint `json:"daily_usage"`
	TotalTokens  int64             `json:"total_tokens"`
	TotalCostUSD float64           `json:"total_cost_usd"`
}
