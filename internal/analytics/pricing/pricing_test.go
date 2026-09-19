package pricing

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingCatalog_EmbeddedDefaults(t *testing.T) {
	ctx := context.Background()
	cat := NewTokenPricingCatalog(WithOnlineFetching(false))

	// 1. Claude 3.5 Sonnet
	info := cat.Execute(ctx, "claude-3-5-sonnet", "")
	assert.Equal(t, "claude-3-5-sonnet", info.ID)
	assert.InDelta(t, 3.00/1e6, info.Pricing.Input.Default, 1e-10)
	assert.InDelta(t, 15.00/1e6, info.Pricing.Output.Default, 1e-10)
	assert.InDelta(t, 0.30/1e6, info.Pricing.CacheRead.Default, 1e-10)
	assert.InDelta(t, 3.75/1e6, info.Pricing.CacheWrite.Default, 1e-10)

	// 2. Gemini 2.5 Pro with 200k tier
	gemini := cat.Execute(ctx, "gemini-2.5-pro", "")
	assert.Equal(t, "gemini-2.5-pro", gemini.ID)
	assert.InDelta(t, 1.25/1e6, gemini.Pricing.Input.Default, 1e-10)
	require.Len(t, gemini.Pricing.Input.Tiers, 1)
	assert.Equal(t, int64(200_000), gemini.Pricing.Input.Tiers[0].Threshold)
	assert.InDelta(t, 2.50/1e6, gemini.Pricing.Input.Tiers[0].Rate, 1e-10)

	// 3. Provider prefix lookup
	prefixed := cat.Execute(ctx, "anthropic/claude-3-5-sonnet", "")
	assert.Equal(t, "anthropic/claude-3-5-sonnet", prefixed.ID)

	// 4. Unknown model returns empty zero pricing
	unknown := cat.Execute(ctx, "unknown-custom-model-xyz", "")
	assert.Equal(t, 0.0, unknown.Pricing.Input.Default)
	assert.Equal(t, 0.0, unknown.Pricing.Output.Default)
}

func TestPricingResolver_Precedence(t *testing.T) {
	ctx := context.Background()
	cat := NewTokenPricingCatalog(WithOnlineFetching(false))
	resolver := NewPricingResolver(cat)

	// Case 1: Catalog lookup
	resCat := resolver.Resolve(ctx, "gpt-4o", nil)
	assert.Equal(t, "gpt-4o", resCat.Model)
	assert.True(t, resCat.Priced)
	assert.Equal(t, SourceCatalog, resCat.Source)
	assert.InDelta(t, 2.50/1e6, resCat.Rates.Input.Default, 1e-10)

	// Case 2: Manual override takes precedence
	overrides := ManualPricingOverrides{
		"gpt-4o": ManualModelPricing{
			Input:      1.00 / 1e6,
			Output:     2.00 / 1e6,
			CacheRead:  0.25 / 1e6,
			CacheWrite: 0.50 / 1e6,
		},
	}
	resOverride := resolver.Resolve(ctx, "gpt-4o", overrides)
	assert.Equal(t, "gpt-4o", resOverride.Model)
	assert.True(t, resOverride.Priced)
	assert.Equal(t, SourceManual, resOverride.Source)
	assert.InDelta(t, 1.00/1e6, resOverride.Rates.Input.Default, 1e-10)
	assert.InDelta(t, 2.00/1e6, resOverride.Rates.Output.Default, 1e-10)

	// Case 3: Unpriceable model
	resNone := resolver.Resolve(ctx, "custom-internal-unpriced-model", nil)
	assert.False(t, resNone.Priced)
	assert.Equal(t, SourceNone, resNone.Source)
}

func TestModelCostCalculator_BucketCostAndCacheDeduction(t *testing.T) {
	// Rates: $3.00/1M input, $15.00/1M output, $0.30/1M cache read, $3.75/1M cache write
	rates := ModelTokenRates{
		Input:      TokenRate{Default: 3.00 / 1e6},
		Output:     TokenRate{Default: 15.00 / 1e6},
		CacheRead:  TokenRate{Default: 0.30 / 1e6},
		CacheWrite: TokenRate{Default: 3.75 / 1e6},
	}

	bucket := TierUsage{
		Input:      100_000,
		Output:     20_000,
		CacheRead:  40_000,
		CacheWrite: 10_000,
	}

	// Uncached input = 100k - 40k - 10k = 50k
	// Input cost = 50,000 * 3.00 / 1e6 = $0.15
	// Output cost = 20,000 * 15.00 / 1e6 = $0.30
	// Cache read cost = 40,000 * 0.30 / 1e6 = $0.012
	// Cache write cost = 10,000 * 3.75 / 1e6 = $0.0375
	// Total = 0.15 + 0.30 + 0.012 + 0.0375 = $0.4995

	cost := BucketCost(bucket, rates, 0)
	assert.InDelta(t, 0.15, cost.Input, 1e-6)
	assert.InDelta(t, 0.30, cost.Output, 1e-6)
	assert.InDelta(t, 0.012, cost.CacheRead, 1e-6)
	assert.InDelta(t, 0.0375, cost.CacheWrite, 1e-6)
	assert.InDelta(t, 0.4995, cost.Total, 1e-6)
}

func TestModelCostCalculator_TieredCostCalculation(t *testing.T) {
	ctx := context.Background()
	cat := NewTokenPricingCatalog(WithOnlineFetching(false))
	resolver := NewPricingResolver(cat)
	calc := NewModelCostCalculator(resolver)

	// Gemini 2.5 Pro: default input $1.25/1M, tier >200k input $2.50/1M, output $5.00/1M, tier >200k output $10.00/1M
	// Row 1: Bracket 0 (under 200k) -> 100k input, 10k output
	// Row 2: Bracket 1 (over 200k) -> 300k input, 20k output
	rows := []CostUsageRow{
		{
			Model: "gemini-2.5-pro",
			ByTier: []TierUsage{
				{
					Input:  100_000,
					Output: 10_000,
				},
				{
					Input:  300_000,
					Output: 20_000,
				},
			},
		},
	}

	spends, err := calc.SpendByModel(ctx, rows, nil)
	require.NoError(t, err)
	require.Len(t, spends, 1)

	// Bracket 0: 100k * 1.25/1e6 + 10k * 5.00/1e6 = 0.125 + 0.05 = $0.175
	// Bracket 1: 300k * 2.50/1e6 + 20k * 10.00/1e6 = 0.75 + 0.20 = $0.95
	// Total expected: $1.125
	assert.InDelta(t, 1.125, spends[0].SpentUSD, 1e-6)

	total, err := calc.TotalCost(ctx, rows, nil)
	require.NoError(t, err)
	assert.InDelta(t, 1.125, total, 1e-6)
}

func TestModelCostCalculator_MultiModelSpend(t *testing.T) {
	ctx := context.Background()
	cat := NewTokenPricingCatalog(WithOnlineFetching(false))
	resolver := NewPricingResolver(cat)
	calc := NewModelCostCalculator(resolver)

	rows := []CostUsageRow{
		{
			Model:  "claude-3-5-sonnet",
			Input:  100_000,
			Output: 10_000,
		},
		{
			Model:  "gpt-4o",
			Input:  100_000,
			Output: 10_000,
		},
	}

	spends, err := calc.SpendByModel(ctx, rows, nil)
	require.NoError(t, err)
	assert.Len(t, spends, 2)

	total, err := calc.TotalCost(ctx, rows, nil)
	require.NoError(t, err)
	assert.True(t, total > 0)
	assert.False(t, math.IsNaN(total))
}
