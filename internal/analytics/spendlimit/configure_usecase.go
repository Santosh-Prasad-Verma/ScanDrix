package spendlimit

import (
	"context"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

// ConfigureSpendLimitInput contains parameters to update spend limit settings.
type ConfigureSpendLimitInput struct {
	OrganizationID  string                         `json:"organizationId"`
	TeamID          string                         `json:"teamId"`
	Enabled         bool                           `json:"enabled"`
	MonthlyLimitUSD float64                        `json:"monthlyLimitUsd"`
	Scope           SpendLimitScope                `json:"scope,omitempty"`
	ModelPricing    pricing.ManualPricingOverrides `json:"modelPricing,omitempty"`
	Models          []string                       `json:"models,omitempty"`
}

// ConfigureSpendLimitUseCase enables, configures, or disables organization spend limits.
type ConfigureSpendLimitUseCase struct {
	configService *SpendLimitConfigService
}

// NewConfigureSpendLimitUseCase creates a new configure spend limit use case.
func NewConfigureSpendLimitUseCase(configService *SpendLimitConfigService) *ConfigureSpendLimitUseCase {
	return &ConfigureSpendLimitUseCase{configService: configService}
}

// Execute validates requirements, ensures model priceability, and saves configuration.
func (uc *ConfigureSpendLimitUseCase) Execute(
	ctx context.Context,
	input ConfigureSpendLimitInput,
) (*SpendLimitConfig, error) {
	if input.Enabled {
		if input.MonthlyLimitUSD <= 0 {
			return nil, &SpendLimitConfigError{
				Message: "A positive monthly limit is required to enable spend alerts.",
			}
		}

		priceResult := uc.configService.CheckPriceability(ctx, input.Models, input.ModelPricing)
		if !priceResult.Priceable {
			return nil, &SpendLimitPriceabilityError{
				UnpriceableModels: priceResult.Unpriceable,
			}
		}
	}

	existing, _ := uc.configService.GetConfig(ctx, input.OrganizationID, input.TeamID)

	scope := input.Scope
	if scope == "" && existing != nil && existing.Scope != "" {
		scope = existing.Scope
	}
	if scope == "" {
		scope = ScopeTotal
	}

	modelPricing := input.ModelPricing
	if modelPricing == nil && existing != nil {
		modelPricing = existing.ModelPricing
	}

	cfg := &SpendLimitConfig{
		Enabled:         input.Enabled,
		MonthlyLimitUSD: input.MonthlyLimitUSD,
		Scope:           scope,
		ModelPricing:    modelPricing,
	}

	if existing != nil {
		cfg.ThresholdsSent = existing.ThresholdsSent
		cfg.FinalNoticeSent = existing.FinalNoticeSent
	}

	if err := uc.configService.SaveConfig(ctx, input.OrganizationID, input.TeamID, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
