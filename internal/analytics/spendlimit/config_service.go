package spendlimit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/usage"
)

const (
	ParamKeySpendLimitConfig = "SPEND_LIMIT_CONFIG"
	ParamKeyBYOKConfig       = "BYOK_CONFIG"
)

// SpendLimitConfigService manages per-organization spend limit persistence, evaluation, and priceability verification.
type SpendLimitConfigService struct {
	orgParamsService IOrganizationParametersService
	monthlySpend     *usage.MonthlySpendUseCase
	pricingResolver  *pricing.PricingResolver
}

// NewSpendLimitConfigService creates a new config service.
func NewSpendLimitConfigService(
	orgParamsService IOrganizationParametersService,
	monthlySpend *usage.MonthlySpendUseCase,
	pricingResolver *pricing.PricingResolver,
) *SpendLimitConfigService {
	return &SpendLimitConfigService{
		orgParamsService: orgParamsService,
		monthlySpend:     monthlySpend,
		pricingResolver:  pricingResolver,
	}
}

// GetConfig retrieves the stored spend limit config for an organization.
func (s *SpendLimitConfigService) GetConfig(ctx context.Context, orgID, teamID string) (*SpendLimitConfig, error) {
	if s.orgParamsService == nil {
		return nil, nil
	}

	raw, err := s.orgParamsService.Get(ctx, orgID, teamID, ParamKeySpendLimitConfig)
	if err != nil || raw == nil {
		return nil, nil
	}

	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var cfg SpendLimitConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig persists the spend limit configuration for an organization.
func (s *SpendLimitConfigService) SaveConfig(ctx context.Context, orgID, teamID string, cfg *SpendLimitConfig) error {
	if s.orgParamsService == nil {
		return nil
	}
	return s.orgParamsService.Set(ctx, orgID, teamID, ParamKeySpendLimitConfig, cfg)
}

// LoadAndEvaluate loads configuration and evaluates month-to-date spend against it.
func (s *SpendLimitConfigService) LoadAndEvaluate(
	ctx context.Context,
	orgID, teamID string,
	now time.Time,
) (*SpendLimitConfig, *pricing.SpendLimitEvaluation, error) {
	cfg, err := s.GetConfig(ctx, orgID, teamID)
	if err != nil || cfg == nil || !cfg.Enabled || cfg.MonthlyLimitUSD <= 0 {
		return nil, nil, nil
	}

	byokConfig := s.getByokConfig(ctx, orgID, teamID)

	eval, err := s.monthlySpend.GetStatus(
		ctx,
		orgID,
		cfg.MonthlyLimitUSD,
		now,
		cfg.ModelPricing,
		byokConfig,
	)
	if err != nil {
		return nil, nil, err
	}

	return cfg, eval, nil
}

// Evaluate scores month-to-date spend against the configured limit.
func (s *SpendLimitConfigService) Evaluate(
	ctx context.Context,
	orgID, teamID string,
	now time.Time,
) (*pricing.SpendLimitEvaluation, error) {
	_, eval, err := s.LoadAndEvaluate(ctx, orgID, teamID, now)
	return eval, err
}

func (s *SpendLimitConfigService) getByokConfig(ctx context.Context, orgID, teamID string) *BYOKConfigRef {
	if s.orgParamsService == nil {
		return nil
	}
	raw, err := s.orgParamsService.Get(ctx, orgID, teamID, ParamKeyBYOKConfig)
	if err != nil || raw == nil {
		return nil
	}

	b, _ := json.Marshal(raw)
	var ref BYOKConfigRef
	if err := json.Unmarshal(b, &ref); err != nil {
		return nil
	}
	return &ref
}

// ListEnabledOrganizations lists every organization with an active, positive monthly spend limit.
func (s *SpendLimitConfigService) ListEnabledOrganizations(ctx context.Context) ([]struct {
	OrganizationID string
	TeamID         string
	Config         SpendLimitConfig
}, error) {
	if s.orgParamsService == nil {
		return nil, nil
	}

	records, err := s.orgParamsService.ListConfigured(ctx, ParamKeySpendLimitConfig)
	if err != nil {
		return nil, err
	}

	var enabled []struct {
		OrganizationID string
		TeamID         string
		Config         SpendLimitConfig
	}

	for _, rec := range records {
		b, _ := json.Marshal(rec.ConfigValue)
		var cfg SpendLimitConfig
		if err := json.Unmarshal(b, &cfg); err == nil && cfg.Enabled && cfg.MonthlyLimitUSD > 0 {
			enabled = append(enabled, struct {
				OrganizationID string
				TeamID         string
				Config         SpendLimitConfig
			}{
				OrganizationID: rec.OrganizationID,
				TeamID:         rec.TeamID,
				Config:         cfg,
			})
		}
	}

	return enabled, nil
}

// CheckPriceability verifies whether every provided model has a resolvable catalog or override price.
func (s *SpendLimitConfigService) CheckPriceability(
	ctx context.Context,
	models []string,
	overrides pricing.ManualPricingOverrides,
) PriceabilityResult {
	resolved := s.pricingResolver.ResolveMany(ctx, models, overrides)

	var unpriceable []string
	for _, r := range resolved {
		if !r.Priced {
			unpriceable = append(unpriceable, r.Model)
		}
	}

	return PriceabilityResult{
		Priceable:   len(unpriceable) == 0,
		Unpriceable: unpriceable,
	}
}
