package usage

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

// TokenUsageQueryContract defines parameters for querying token usage.
type TokenUsageQueryContract struct {
	OrganizationID string    `json:"organizationId"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	Models         string    `json:"models,omitempty"`
	PRNumber       *int      `json:"prNumber,omitempty"`
	Timezone       string    `json:"timezone,omitempty"` // For day bucketing
	Developer      string    `json:"developer,omitempty"`
	RepositoryID   string    `json:"repositoryId,omitempty"`
	PRNumbers      []int     `json:"prNumbers,omitempty"`
	BYOK           bool      `json:"byok"`
}

// TierUsage tracks token counts for a single tier bucket of calls.
type TierUsage = pricing.TierUsage

// BaseUsageContract is the foundational contract for token consumption counts.
type BaseUsageContract struct {
	Model           string      `json:"model"`
	Input           int64       `json:"input"`
	Output          int64       `json:"output"`
	Total           int64       `json:"total"`
	OutputReasoning int64       `json:"outputReasoning"`
	CacheRead       int64       `json:"cacheRead,omitempty"`
	CacheWrite      int64       `json:"cacheWrite,omitempty"`
	ByTier          []TierUsage `json:"byTier,omitempty"`
}

// UsageSummaryContract represents summarized base usage.
type UsageSummaryContract = BaseUsageContract

// DailyUsageResultContract contains usage grouped by date.
type DailyUsageResultContract struct {
	BaseUsageContract
	Date string `json:"date"` // YYYY-MM-DD
}

// UsageByPrResultContract contains usage grouped by pull request number.
type UsageByPrResultContract struct {
	BaseUsageContract
	PRNumber int `json:"prNumber"`
}

// DailyUsageByPrResultContract contains usage grouped by date and pull request.
type DailyUsageByPrResultContract struct {
	UsageByPrResultContract
	Date string `json:"date"` // YYYY-MM-DD
}

// UsageByDeveloperResultContract contains usage attributed to a developer.
type UsageByDeveloperResultContract struct {
	BaseUsageContract
	Developer string `json:"developer"`
}

// DailyUsageByDeveloperResultContract contains developer usage grouped by date.
type DailyUsageByDeveloperResultContract struct {
	UsageByDeveloperResultContract
	Date string `json:"date"` // YYYY-MM-DD
}

// UsageByReviewResultContract tracks token usage per review run (correlationId).
type UsageByReviewResultContract struct {
	BaseUsageContract
	Review    string     `json:"review"`
	PRNumber  *int       `json:"prNumber,omitempty"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
}

// UsageByAreaResultContract groups token spend by process stage / area.
type UsageByAreaResultContract struct {
	BaseUsageContract
	Area string `json:"area"`
}

// UsageByTaskAreaResultContract groups token spend by the routing TASK x process AREA cross.
type UsageByTaskAreaResultContract struct {
	BaseUsageContract
	Task string `json:"task"`
	Area string `json:"area"`
}

// UsageByTaskModelSpanContract marks the active first/last usage timestamps of a MODEL within a routing TASK.
type UsageByTaskModelSpanContract struct {
	Task    string `json:"task"`
	Model   string `json:"model"`
	FirstAt string `json:"firstAt"`
	LastAt  string `json:"lastAt"`
}

// CostBreakdown is re-exported from pricing for convenience.
type CostBreakdown = pricing.CostBreakdown

// ApiPricingSource is the wire-facing label for pricing resolution origin.
type ApiPricingSource string

const (
	ApiSourceManual  ApiPricingSource = "manual"
	ApiSourceCatalog ApiPricingSource = "catalog"
	ApiSourceMissing ApiPricingSource = "missing"
)

// EnrichedModelUsage is a per-model usage row enriched with computed USD cost and source.
type EnrichedModelUsage struct {
	BaseUsageContract
	Cost          CostBreakdown    `json:"cost"`
	CostByTier    []CostBreakdown  `json:"costByTier,omitempty"`
	PricingSource ApiPricingSource `json:"pricingSource"`
}

// UsageSummaryReportContract is the rich payload for /usage/tokens/summary.
type UsageSummaryReportContract struct {
	Totals    BaseUsageContract    `json:"totals"`
	TotalCost CostBreakdown        `json:"totalCost"`
	ByModel   []EnrichedModelUsage `json:"byModel"`
}

// UsageOverviewReportContract is the single-request payload for the Token Usage screen.
type UsageOverviewReportContract struct {
	Summary         UsageSummaryReportContract     `json:"summary"`
	Daily           []DailyUsageResultContract     `json:"daily"`
	ByPr            []UsageByPrResultContract      `json:"byPr"`
	ByArea          []UsageByAreaResultContract    `json:"byArea"`
	ByTaskArea      []UsageByTaskAreaResultContract `json:"byTaskArea"`
	ByTaskModelSpan []UsageByTaskModelSpanContract `json:"byTaskModelSpan"`
}

// TokenUsageBreakdown aggregates token categories.
type TokenUsageBreakdown struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
	TotalTokens      int64 `json:"totalTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
}

// CostEstimateContract provides projected costs and developer breakdowns.
type CostEstimateContract struct {
	EstimatedMonthlyCost float64             `json:"estimatedMonthlyCost"`
	CostPerDeveloper     float64             `json:"costPerDeveloper"`
	DeveloperCount       int                 `json:"developerCount"`
	TokenUsage           TokenUsageBreakdown `json:"tokenUsage"`
	PeriodDays           int                 `json:"periodDays"`
	ProjectionDays       int                 `json:"projectionDays"`
}

// ModelCredentialPair represents model to credentialId mapping recorded on usage spans.
type ModelCredentialPair struct {
	Model        string `json:"model"`
	CredentialID string `json:"credentialId"`
}

// ITokenUsageService defines access to token usage aggregations.
type ITokenUsageService interface {
	GetSummary(ctx context.Context, query TokenUsageQueryContract) (UsageSummaryContract, error)
	GetSummaryByModel(ctx context.Context, query TokenUsageQueryContract) ([]BaseUsageContract, error)
	GetDailyUsage(ctx context.Context, query TokenUsageQueryContract) ([]DailyUsageResultContract, error)
	GetModelCredentialPairs(ctx context.Context, query TokenUsageQueryContract) ([]ModelCredentialPair, error)
	GetUsageByPr(ctx context.Context, query TokenUsageQueryContract) ([]UsageByPrResultContract, error)
	GetDailyUsageByPr(ctx context.Context, query TokenUsageQueryContract) ([]DailyUsageByPrResultContract, error)
	GetUsageByReview(ctx context.Context, query TokenUsageQueryContract) ([]UsageByReviewResultContract, error)
	GetUsageByArea(ctx context.Context, query TokenUsageQueryContract) ([]UsageByAreaResultContract, error)
	GetUsageOverview(ctx context.Context, query TokenUsageQueryContract) (*UsageOverviewReportContract, error)
}

// IPullRequestUserMapping provides username for a PR.
type IPullRequestUserMapping struct {
	Number   int    `json:"number"`
	Username string `json:"username"`
}

// IPullRequestsService provides pull request lookups for developer attribution.
type IPullRequestsService interface {
	FindOne(ctx context.Context, orgID string, prNumber int) (*IPullRequestUserMapping, error)
	FindManyByNumbers(ctx context.Context, prNumbers []int, orgID string) ([]IPullRequestUserMapping, error)
}

// ICacheService defines cache get/set operations with TTL.
type ICacheService interface {
	Get(ctx context.Context, key string, dest any) bool
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
}
