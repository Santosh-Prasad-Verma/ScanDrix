package pricing

import (
	"context"
	"strings"
)

// PricingResolver resolves the current price for a model with a fixed precedence:
//  1. manual override (org-entered on BYOK config) — flat, no tiers
//  2. catalog (current rates, may be tiered)
//  3. unpriceable — source: 'none', priced: false
type PricingResolver struct {
	catalog *TokenPricingCatalog
}

// NewPricingResolver initializes a new pricing resolver.
func NewPricingResolver(catalog *TokenPricingCatalog) *PricingResolver {
	if catalog == nil {
		catalog = NewTokenPricingCatalog()
	}
	return &PricingResolver{catalog: catalog}
}

// Resolve resolves the price for a model against overrides and the catalog.
func (r *PricingResolver) Resolve(ctx context.Context, model string, overrides ManualPricingOverrides) ResolvedModelPricing {
	key := strings.TrimSpace(model)
	if manual, ok := overrides[key]; ok {
		return ResolvedModelPricing{
			Model:  key,
			Source: SourceManual,
			Priced: true,
			Rates: ModelTokenRates{
				Input:      TokenRate{Default: manual.Input},
				Output:     TokenRate{Default: manual.Output},
				CacheRead:  TokenRate{Default: manual.CacheRead},
				CacheWrite: TokenRate{Default: manual.CacheWrite},
			},
		}
	}

	info := r.catalog.Execute(ctx, key, "")
	rates := ModelTokenRates{
		Input:      info.Pricing.Input,
		Output:     info.Pricing.Output,
		CacheRead:  info.Pricing.CacheRead,
		CacheWrite: info.Pricing.CacheWrite,
	}

	priced := rates.Input.Default > 0 || rates.Output.Default > 0

	source := SourceNone
	if priced {
		source = SourceCatalog
	}

	return ResolvedModelPricing{
		Model:  key,
		Source: source,
		Priced: priced,
		Rates:  rates,
	}
}

// ResolveMany resolves a list of models, deduplicating and skipping blanks.
func (r *PricingResolver) ResolveMany(ctx context.Context, models []string, overrides ManualPricingOverrides) []ResolvedModelPricing {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(models))

	for _, m := range models {
		clean := strings.TrimSpace(m)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		unique = append(unique, clean)
	}

	res := make([]ResolvedModelPricing, len(unique))
	for i, m := range unique {
		res[i] = r.Resolve(ctx, m, overrides)
	}
	return res
}

// TieredInputThresholds returns canonical model names -> sorted input tier thresholds.
func (r *PricingResolver) TieredInputThresholds(ctx context.Context) map[string][]int64 {
	return r.catalog.TieredInputThresholds(ctx)
}
