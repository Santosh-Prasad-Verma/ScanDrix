package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

const (
	OverviewTTLPast    = 4 * time.Hour
	OverviewTTLCurrent = 4 * time.Hour
)

// BuildUsageSummaryUseCase builds enriched payloads for token usage dashboards.
type BuildUsageSummaryUseCase struct {
	tokenUsageService ITokenUsageService
	pricingResolver   *pricing.PricingResolver
	cache             ICacheService
}

// NewBuildUsageSummaryUseCase creates a new summary use case.
func NewBuildUsageSummaryUseCase(
	tokenUsageService ITokenUsageService,
	pricingResolver *pricing.PricingResolver,
	cache ICacheService,
) *BuildUsageSummaryUseCase {
	return &BuildUsageSummaryUseCase{
		tokenUsageService: tokenUsageService,
		pricingResolver:   pricingResolver,
		cache:             cache,
	}
}

// Execute builds flat totals + per-token-type cost + per-model rows.
func (uc *BuildUsageSummaryUseCase) Execute(
	ctx context.Context,
	query TokenUsageQueryContract,
	overrides pricing.ManualPricingOverrides,
) (*UsageSummaryReportContract, error) {
	totals, err := uc.tokenUsageService.GetSummary(ctx, query)
	if err != nil {
		return nil, err
	}

	byModel, err := uc.tokenUsageService.GetSummaryByModel(ctx, query)
	if err != nil {
		return nil, err
	}

	enrichedRows := make([]EnrichedModelUsage, len(byModel))
	for i, row := range byModel {
		enrichedRows[i] = uc.enrich(ctx, row, overrides)
	}

	sort.Slice(enrichedRows, func(i, j int) bool {
		return enrichedRows[i].Model < enrichedRows[j].Model
	})

	totalCost := zeroCost()
	for _, row := range enrichedRows {
		totalCost = addCost(totalCost, row.Cost)
	}

	return &UsageSummaryReportContract{
		Totals:    totals,
		TotalCost: totalCost,
		ByModel:   enrichedRows,
	}, nil
}

// ExecuteOverview retrieves the complete single-request overview for the token usage screen with caching.
func (uc *BuildUsageSummaryUseCase) ExecuteOverview(
	ctx context.Context,
	query TokenUsageQueryContract,
	overrides pricing.ManualPricingOverrides,
) (*UsageOverviewReportContract, error) {
	cacheKey := uc.overviewCacheKey(query, overrides)

	if uc.cache != nil {
		var cached UsageOverviewReportContract
		if ok := uc.cache.Get(ctx, cacheKey, &cached); ok {
			if len(cached.ByTaskModelSpan) > 0 || len(cached.Summary.ByModel) > 0 || cached.Summary.Totals.Total > 0 {
				return &cached, nil
			}
		}
	}

	report, err := uc.buildOverview(ctx, query, overrides)
	if err != nil {
		return nil, err
	}

	if uc.cache != nil {
		ttl := OverviewTTLCurrent
		if query.End.Before(startOfTodayUTC()) {
			ttl = OverviewTTLPast
		}
		_ = uc.cache.Set(ctx, cacheKey, report, ttl)
	}

	return report, nil
}

func (uc *BuildUsageSummaryUseCase) buildOverview(
	ctx context.Context,
	query TokenUsageQueryContract,
	overrides pricing.ManualPricingOverrides,
) (*UsageOverviewReportContract, error) {
	overview, err := uc.tokenUsageService.GetUsageOverview(ctx, query)
	if err != nil {
		return nil, err
	}

	enrichedRows := make([]EnrichedModelUsage, len(overview.Summary.ByModel))
	for i, row := range overview.Summary.ByModel {
		enrichedRows[i] = uc.enrich(ctx, row.BaseUsageContract, overrides)
	}

	sort.Slice(enrichedRows, func(i, j int) bool {
		return enrichedRows[i].Model < enrichedRows[j].Model
	})

	totalCost := zeroCost()
	for _, row := range enrichedRows {
		totalCost = addCost(totalCost, row.Cost)
	}

	return &UsageOverviewReportContract{
		Summary: UsageSummaryReportContract{
			Totals:    overview.Summary.Totals,
			TotalCost: totalCost,
			ByModel:   enrichedRows,
		},
		Daily:           overview.Daily,
		ByPr:            overview.ByPr,
		ByArea:          overview.ByArea,
		ByTaskArea:      overview.ByTaskArea,
		ByTaskModelSpan: overview.ByTaskModelSpan,
	}, nil
}

func (uc *BuildUsageSummaryUseCase) enrich(
	ctx context.Context,
	row BaseUsageContract,
	overrides pricing.ManualPricingOverrides,
) EnrichedModelUsage {
	resolved := uc.pricingResolver.Resolve(ctx, row.Model, overrides)

	apiSource := ApiSourceMissing
	switch resolved.Source {
	case pricing.SourceManual:
		apiSource = ApiSourceManual
	case pricing.SourceCatalog:
		apiSource = ApiSourceCatalog
	}

	if len(row.ByTier) > 0 {
		costByTier := make([]CostBreakdown, len(row.ByTier))
		total := zeroCost()
		for i, bucket := range row.ByTier {
			cb := pricing.BucketCost(bucket, resolved.Rates, i)
			costByTier[i] = cb
			total = addCost(total, cb)
		}
		return EnrichedModelUsage{
			BaseUsageContract: row,
			Cost:              total,
			CostByTier:        costByTier,
			PricingSource:     apiSource,
		}
	}

	flatBucket := TierUsage{
		Input:           row.Input,
		Output:          row.Output,
		Total:           row.Total,
		OutputReasoning: row.OutputReasoning,
		CacheRead:       row.CacheRead,
		CacheWrite:      row.CacheWrite,
	}
	cost := pricing.BucketCost(flatBucket, resolved.Rates, 0)

	return EnrichedModelUsage{
		BaseUsageContract: row,
		Cost:              cost,
		PricingSource:     apiSource,
	}
}

func (uc *BuildUsageSummaryUseCase) overviewCacheKey(
	query TokenUsageQueryContract,
	overrides pricing.ManualPricingOverrides,
) string {
	ovBytes, _ := json.Marshal(overrides)
	mode := "sys"
	if query.BYOK {
		mode = "byok"
	}
	pr := 0
	if query.PRNumber != nil {
		pr = *query.PRNumber
	}
	tz := query.Timezone
	if tz == "" {
		tz = "UTC"
	}

	return fmt.Sprintf(
		"usage:overview|%s|%s|%d|%d|%s|%s|%d|%s|%s",
		query.OrganizationID,
		mode,
		query.Start.UnixMilli(),
		query.End.UnixMilli(),
		tz,
		query.Models,
		pr,
		query.RepositoryID,
		string(ovBytes),
	)
}

func startOfTodayUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func zeroCost() CostBreakdown {
	return CostBreakdown{
		Input:      0,
		Output:     0,
		CacheRead:  0,
		CacheWrite: 0,
		Total:      0,
	}
}

func addCost(a, b CostBreakdown) CostBreakdown {
	return CostBreakdown{
		Input:      a.Input + b.Input,
		Output:     a.Output + b.Output,
		CacheRead:  a.CacheRead + b.CacheRead,
		CacheWrite: a.CacheWrite + b.CacheWrite,
		Total:      a.Total + b.Total,
	}
}
