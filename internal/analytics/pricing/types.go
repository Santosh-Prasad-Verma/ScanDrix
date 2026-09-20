package pricing

// TierRate specifies a context tier threshold and its corresponding per-token rate.
// For example, calls with prompt tokens > 200k are billed at rate instead of default.
type TierRate struct {
	Threshold int64   `json:"threshold"`
	Rate      float64 `json:"rate"`
}

// TokenRate specifies default rate with optional context tiers (sorted ascending by threshold).
// This is per-request-total tiering (how Gemini, Doubao, etc. bill), NOT graduated tiering.
// Prices are stored per-token (NOT per million) to match the cost arithmetic.
type TokenRate struct {
	Default float64    `json:"default"`
	Tiers   []TierRate `json:"tiers,omitempty"`
}

// ModelTokenRates defines per-token rates for input, output, cache read, and cache write.
type ModelTokenRates struct {
	Input      TokenRate `json:"input"`
	Output     TokenRate `json:"output"`
	CacheRead  TokenRate `json:"cacheRead"`
	CacheWrite TokenRate `json:"cacheWrite"`
}

// ManualModelPricing specifies organization-entered pricing for a model.
// Flat per-token USD, one rate per token type — no tiers.
type ManualModelPricing struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ManualPricingOverrides is a map of model name -> organization-entered pricing.
type ManualPricingOverrides map[string]ManualModelPricing

// PricingSource defines where a model price was resolved from.
type PricingSource string

const (
	SourceManual  PricingSource = "manual"
	SourceCatalog PricingSource = "catalog"
	SourceNone    PricingSource = "none"
)

// ResolvedModelPricing contains resolved rates and metadata for a single model.
type ResolvedModelPricing struct {
	Model        string          `json:"model"`
	Source       PricingSource   `json:"source"`
	Priced       bool            `json:"priced"`
	Rates        ModelTokenRates `json:"rates"`
	CatalogRates *ModelTokenRates `json:"catalogRates,omitempty"`
}

// CostBreakdown provides USD-denominated cost broken down by token type.
// Total = Input + Output + CacheRead + CacheWrite.
type CostBreakdown struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// ModelSpend records billed spend for a single model in USD.
type ModelSpend struct {
	Model    string  `json:"model"`
	SpentUSD float64 `json:"spentUsd"`
}

// TierUsage tracks token counts for a single tier bracket of calls.
type TierUsage struct {
	Input           int64 `json:"input"`
	Output          int64 `json:"output"`
	Total           int64 `json:"total"`
	OutputReasoning int64 `json:"outputReasoning"`
	CacheRead       int64 `json:"cacheRead"`
	CacheWrite      int64 `json:"cacheWrite"`
}

// CostUsageRow represents a usage row carrying token counts, model id, and optional tier breakdowns.
type CostUsageRow struct {
	Input           int64       `json:"input"`
	Output          int64       `json:"output"`
	OutputReasoning int64       `json:"outputReasoning"`
	CacheRead       int64       `json:"cacheRead"`
	CacheWrite      int64       `json:"cacheWrite"`
	Model           string      `json:"model"`
	ByTier          []TierUsage `json:"byTier,omitempty"`
}

const UnattributedCredential = "unattributed"

// CredentialSpend records spend attributed to a single BYOK credential.
type CredentialSpend struct {
	CredentialID string  `json:"credentialId"`
	SpentUSD     float64 `json:"spentUsd"`
}

// RunRateProjection extrapolates month-to-date spend to a full calendar month.
type RunRateProjection struct {
	ProjectedMonthlyUSD float64 `json:"projectedMonthlyUsd"`
	ElapsedFraction     float64 `json:"elapsedFraction"`
}

// SpendLimitStatus represents the evaluation of spend against a limit.
type SpendLimitStatus struct {
	SpentUSD          float64 `json:"spentUsd"`
	LimitUSD          float64 `json:"limitUsd"`
	Pct               float64 `json:"pct"`
	IsOverLimit       bool    `json:"isOverLimit"`
	CrossedThresholds []int   `json:"crossedThresholds"`
}

// SpendLimitEvaluation contains full evaluation details including breakdowns.
type SpendLimitEvaluation struct {
	SpendLimitStatus
	OrganizationID string            `json:"organizationId"`
	PeriodKey      string            `json:"periodKey"`
	ByModel        []ModelSpend      `json:"byModel"`
	ByCredential   []CredentialSpend `json:"byCredential"`
	RunRate        RunRateProjection `json:"runRate"`
}

// BuildSpendLimitStatus evaluates spent USD against limit USD.
func BuildSpendLimitStatus(spentUSD float64, limitUSD float64, thresholds []int) SpendLimitStatus {
	safeSpent := spentUSD
	if safeSpent < 0 {
		safeSpent = 0
	}

	if limitUSD <= 0 {
		return SpendLimitStatus{
			SpentUSD:          safeSpent,
			LimitUSD:          0,
			Pct:               0,
			IsOverLimit:       false,
			CrossedThresholds: []int{},
		}
	}

	pct := (safeSpent / limitUSD) * 100.0
	var crossed []int
	for _, t := range thresholds {
		if pct >= float64(t) {
			crossed = append(crossed, t)
		}
	}

	return SpendLimitStatus{
		SpentUSD:          safeSpent,
		LimitUSD:          limitUSD,
		Pct:               pct,
		IsOverLimit:       pct >= 100.0,
		CrossedThresholds: crossed,
	}
}
