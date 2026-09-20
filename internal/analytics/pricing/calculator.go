package pricing

import (
	"context"
	"math"
	"strings"
)

const UnknownModel = "(unknown)"

// ModelCostCalculator is the single source of truth for converting token usage counts into USD cost.
// Buckets usage by model and prices each independently with tier brackets and cache deductions.
type ModelCostCalculator struct {
	resolver *PricingResolver
}

// NewModelCostCalculator creates a new calculator.
func NewModelCostCalculator(resolver *PricingResolver) *ModelCostCalculator {
	if resolver == nil {
		resolver = NewPricingResolver(nil)
	}
	return &ModelCostCalculator{resolver: resolver}
}

// SpendByModel calculates per-model billed cost for usage rows.
func (c *ModelCostCalculator) SpendByModel(ctx context.Context, rows []CostUsageRow, overrides ManualPricingOverrides) ([]ModelSpend, error) {
	perModel := c.bucketByModel(rows)

	out := make([]ModelSpend, 0, len(perModel))
	for model, agg := range perModel {
		spent := c.costForModel(ctx, model, agg, overrides)
		out = append(out, ModelSpend{
			Model:    model,
			SpentUSD: spent,
		})
	}
	return out, nil
}

// TotalCost calculates the total billed cost across every model in the usage rows.
func (c *ModelCostCalculator) TotalCost(ctx context.Context, rows []CostUsageRow, overrides ManualPricingOverrides) (float64, error) {
	byModel, err := c.SpendByModel(ctx, rows, overrides)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, m := range byModel {
		total += m.SpentUSD
	}
	return total, nil
}

func (c *ModelCostCalculator) bucketByModel(rows []CostUsageRow) map[string][]TierUsage {
	perModel := make(map[string][]TierUsage)

	for _, row := range rows {
		key := strings.TrimSpace(row.Model)
		if key == "" {
			key = UnknownModel
		}

		agg := perModel[key]

		var buckets []TierUsage
		if len(row.ByTier) > 0 {
			buckets = row.ByTier
		} else {
			// Flat row without tiers
			buckets = []TierUsage{
				{
					Input:           row.Input,
					Output:          row.Output,
					Total:           row.Input + row.Output,
					OutputReasoning: row.OutputReasoning,
					CacheRead:       row.CacheRead,
					CacheWrite:      row.CacheWrite,
				},
			}
		}

		for i, bucket := range buckets {
			for len(agg) <= i {
				agg = append(agg, TierUsage{})
			}
			agg[i].Input += bucket.Input
			agg[i].Output += bucket.Output
			agg[i].Total += bucket.Total
			agg[i].OutputReasoning += bucket.OutputReasoning
			agg[i].CacheRead += bucket.CacheRead
			agg[i].CacheWrite += bucket.CacheWrite
		}

		perModel[key] = agg
	}

	return perModel
}

func (c *ModelCostCalculator) costForModel(ctx context.Context, model string, agg []TierUsage, overrides ManualPricingOverrides) float64 {
	if model == UnknownModel {
		return 0
	}

	resolved := c.resolver.Resolve(ctx, model, overrides)

	var total float64
	for i, bucket := range agg {
		cb := BucketCost(bucket, resolved.Rates, i)
		total += cb.Total
	}
	return total
}

// RateFor returns the per-token rate for a given tier bracket.
// Bracket 0 -> Default rate.
// Bracket k (k > 0) -> k-th tier rate (falling back to Default if fewer tiers exist).
func RateFor(rate TokenRate, bracket int) float64 {
	if bracket <= 0 {
		return rate.Default
	}
	if bracket-1 < len(rate.Tiers) {
		return rate.Tiers[bracket-1].Rate
	}
	return rate.Default
}

// BucketCost computes the USD cost of a single bracket bucket, broken down per token type.
// Static formula shared across calculations.
// Cache reads and cache writes are both subsets of reported input tokens, so subtract both
// from the full-price billable pool to avoid double-charging.
func BucketCost(bucket TierUsage, rates ModelTokenRates, bracket int) CostBreakdown {
	inputRate := RateFor(rates.Input, bracket)
	outputRate := RateFor(rates.Output, bracket)
	cacheReadRate := RateFor(rates.CacheRead, bracket)
	cacheWriteRate := RateFor(rates.CacheWrite, bracket)

	uncachedInput := bucket.Input - bucket.CacheRead - bucket.CacheWrite
	if uncachedInput < 0 {
		uncachedInput = 0
	}

	inputCost := float64(uncachedInput) * inputRate
	outputCost := float64(bucket.Output) * outputRate
	cacheReadCost := float64(bucket.CacheRead) * cacheReadRate
	cacheWriteCost := float64(bucket.CacheWrite) * cacheWriteRate

	total := inputCost + outputCost + cacheReadCost + cacheWriteCost
	if math.IsNaN(total) || math.IsInf(total, 0) {
		total = 0
	}

	return CostBreakdown{
		Input:      inputCost,
		Output:     outputCost,
		CacheRead:  cacheReadCost,
		CacheWrite: cacheWriteCost,
		Total:      total,
	}
}
