package spendlimit

import (
	"context"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/usage"
)

var SpendLimitThresholds = []int{50, 75, 90, 100}

// SpendLimitScope defines which spend breakdown the UI readout highlights.
type SpendLimitScope string

const (
	ScopeTotal         SpendLimitScope = "total"
	ScopePerModel      SpendLimitScope = "per-model"
	ScopePerCredential SpendLimitScope = "per-credential"
)

// SpendLimitConfig defines the persistent configuration for an organization's monthly spend alerts.
type SpendLimitConfig struct {
	Enabled         bool                           `json:"enabled"`
	MonthlyLimitUSD float64                        `json:"monthlyLimitUsd"`
	Scope           SpendLimitScope                `json:"scope,omitempty"`
	ModelPricing    pricing.ManualPricingOverrides `json:"modelPricing,omitempty"`
	ThresholdsSent  map[string][]int               `json:"thresholdsSent,omitempty"`  // Keyed by periodKey YYYY-MM
	FinalNoticeSent map[string]bool                `json:"finalNoticeSent,omitempty"` // Keyed by periodKey YYYY-MM
}

// PriceabilityResult defines whether every configured model can be priced.
type PriceabilityResult struct {
	Priceable   bool     `json:"priceable"`
	Unpriceable []string `json:"unpriceable"`
}

// SpendAlertState carries the per-period alert state.
type SpendAlertState struct {
	ThresholdsSent  []int `json:"thresholdsSent,omitempty"`
	FinalNoticeSent bool  `json:"finalNoticeSent,omitempty"`
}

// SpendAlertDecision records the deterministic decision for alerts in the current tick.
type SpendAlertDecision struct {
	ThresholdsToAlert   []int `json:"thresholdsToAlert"`
	SendFinalNotice     bool  `json:"sendFinalNotice"`
	NextThresholdsSent  []int `json:"nextThresholdsSent"`
	NextFinalNoticeSent bool  `json:"nextFinalNoticeSent"`
	Changed             bool  `json:"changed"`
}

// SpendLimitConfigView is the read model consumed by the frontend configuration screen.
type SpendLimitConfigView struct {
	Enabled         bool                           `json:"enabled"`
	MonthlyLimitUSD float64                        `json:"monthlyLimitUsd"`
	ModelPricing    pricing.ManualPricingOverrides `json:"modelPricing"`
	Models          []pricing.ResolvedModelPricing `json:"models"`
	Priceable       bool                           `json:"priceable"`
	Scope           SpendLimitScope                `json:"scope"`
}

// NotificationEvent represents standard system alert events.
type NotificationEvent string

const (
	EventSpendLimitThresholdReached NotificationEvent = "SPEND_LIMIT_THRESHOLD_REACHED"
	EventSpendLimitExceededFinal    NotificationEvent = "SPEND_LIMIT_EXCEEDED_FINAL"
)

// NotificationPayload provides the fields emitted on spend alerts.
type NotificationPayload struct {
	Percentage      int     `json:"percentage,omitempty"`
	MonthlyLimitUSD float64 `json:"monthlyLimitUsd"`
	SpentUSD        float64 `json:"spentUsd"`
	PeriodKey       string  `json:"periodKey"`
}

// INotificationService emits alert events.
type INotificationService interface {
	Emit(ctx context.Context, event NotificationEvent, orgID string, payload NotificationPayload) error
}

// IOrganizationParametersService stores organization-level configuration parameters.
type IOrganizationParametersService interface {
	Get(ctx context.Context, orgID, teamID, key string) (any, error)
	Set(ctx context.Context, orgID, teamID, key string, val any) error
	ListConfigured(ctx context.Context, key string) ([]OrgParameterRecord, error)
}

// OrgParameterRecord represents an organization parameter entry.
type OrgParameterRecord struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	ConfigKey      string `json:"configKey"`
	ConfigValue    any    `json:"configValue"`
}

// IParametersService accesses team or repo parameters.
type IParametersService interface {
	Get(ctx context.Context, orgID, teamID, key string) (any, error)
}

// Re-export BYOKConfigRef and monthly types for convenience
type BYOKConfigRef = usage.BYOKConfigRef
type BYOKModelConfig = usage.BYOKModelConfig
