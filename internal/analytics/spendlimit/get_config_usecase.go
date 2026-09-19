package spendlimit

import (
	"context"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

const ParamKeyCodeReviewConfig = "CODE_REVIEW_CONFIG"

// GetSpendLimitConfigUseCase builds the read view for the spend limit settings UI.
type GetSpendLimitConfigUseCase struct {
	configService        *SpendLimitConfigService
	getOrgByokModels     *GetOrgByokModelsUseCase
	pricingResolver      *pricing.PricingResolver
}

// NewGetSpendLimitConfigUseCase creates a new get config use case.
func NewGetSpendLimitConfigUseCase(
	configService *SpendLimitConfigService,
	orgParamsService IOrganizationParametersService,
	parametersService IParametersService,
	pricingResolver *pricing.PricingResolver,
) *GetSpendLimitConfigUseCase {
	return &GetSpendLimitConfigUseCase{
		configService:    configService,
		getOrgByokModels: NewGetOrgByokModelsUseCase(orgParamsService, parametersService),
		pricingResolver:  pricingResolver,
	}
}

// Execute retrieves configuration, discovers active BYOK models, and resolves catalog/override pricing.
func (uc *GetSpendLimitConfigUseCase) Execute(
	ctx context.Context,
	orgID, teamID string,
) (*SpendLimitConfigView, error) {
	cfg, _ := uc.configService.GetConfig(ctx, orgID, teamID)

	models := uc.getOrgByokModels.Execute(ctx, orgID, teamID)

	var overrides pricing.ManualPricingOverrides
	if cfg != nil {
		overrides = cfg.ModelPricing
	}

	resolved := uc.pricingResolver.ResolveMany(ctx, models, overrides)
	catalogResolved := uc.pricingResolver.ResolveMany(ctx, models, nil)

	catalogByModel := make(map[string]pricing.ResolvedModelPricing)
	for _, r := range catalogResolved {
		catalogByModel[r.Model] = r
	}

	modelsWithCatalog := make([]pricing.ResolvedModelPricing, len(resolved))
	for i, r := range resolved {
		item := r
		if cat, ok := catalogByModel[r.Model]; ok && cat.Priced {
			item.CatalogRates = &cat.Rates
		}
		modelsWithCatalog[i] = item
	}

	priceable := true
	for _, r := range resolved {
		if !r.Priced {
			priceable = false
			break
		}
	}

	enabled := false
	var monthlyLimitUSD float64
	scope := ScopeTotal

	if cfg != nil {
		enabled = cfg.Enabled
		monthlyLimitUSD = cfg.MonthlyLimitUSD
		if cfg.Scope != "" {
			scope = cfg.Scope
		}
	}

	return &SpendLimitConfigView{
		Enabled:         enabled,
		MonthlyLimitUSD: monthlyLimitUSD,
		ModelPricing:    overrides,
		Models:          modelsWithCatalog,
		Priceable:       priceable,
		Scope:           scope,
	}, nil
}
