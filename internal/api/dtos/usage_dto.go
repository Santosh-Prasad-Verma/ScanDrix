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
