package spendlimit

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideSpendAlerts(t *testing.T) {
	// Tick 1: Reached 50% and 75%
	eval1 := struct {
		CrossedThresholds []int
		IsOverLimit       bool
	}{
		CrossedThresholds: []int{50, 75},
		IsOverLimit:       false,
	}

	dec1 := DecideSpendAlerts(eval1, SpendAlertState{})
	assert.Equal(t, []int{50, 75}, dec1.ThresholdsToAlert)
	assert.False(t, dec1.SendFinalNotice)
	assert.Equal(t, []int{50, 75}, dec1.NextThresholdsSent)
	assert.True(t, dec1.Changed)

	// Tick 2: Same evaluation, nothing new to alert
	dec2 := DecideSpendAlerts(eval1, SpendAlertState{
		ThresholdsSent: dec1.NextThresholdsSent,
	})
	assert.Empty(t, dec2.ThresholdsToAlert)
	assert.False(t, dec2.SendFinalNotice)
	assert.False(t, dec2.Changed)

	// Tick 3: Reached 100% and isOverLimit
	eval3 := struct {
		CrossedThresholds []int
		IsOverLimit       bool
	}{
		CrossedThresholds: []int{50, 75, 90, 100},
		IsOverLimit:       true,
	}
	dec3 := DecideSpendAlerts(eval3, SpendAlertState{
		ThresholdsSent: []int{50, 75},
	})
	assert.Equal(t, []int{90, 100}, dec3.ThresholdsToAlert)
	// Final notice waits until next tick after 100% was alerted
	assert.False(t, dec3.SendFinalNotice)
	assert.Equal(t, []int{50, 75, 90, 100}, dec3.NextThresholdsSent)
	assert.True(t, dec3.Changed)

	// Tick 4: 100% was already alerted, now send final notice
	dec4 := DecideSpendAlerts(eval3, SpendAlertState{
		ThresholdsSent:  []int{50, 75, 90, 100},
		FinalNoticeSent: false,
	})
	assert.Empty(t, dec4.ThresholdsToAlert)
	assert.True(t, dec4.SendFinalNotice)
	assert.True(t, dec4.NextFinalNoticeSent)
	assert.True(t, dec4.Changed)
}

func TestBuildSpendLimitStatus(t *testing.T) {
	// Case 1: Standard spend
	status1 := BuildSpendLimitStatus(75.0, 100.0, SpendLimitThresholds)
	assert.Equal(t, 75.0, status1.SpentUSD)
	assert.Equal(t, 100.0, status1.LimitUSD)
	assert.InDelta(t, 75.0, status1.Pct, 1e-6)
	assert.False(t, status1.IsOverLimit)
	assert.Equal(t, []int{50, 75}, status1.CrossedThresholds)

	// Case 2: Over limit
	status2 := BuildSpendLimitStatus(120.0, 100.0, SpendLimitThresholds)
	assert.True(t, status2.IsOverLimit)
	assert.Equal(t, []int{50, 75, 90, 100}, status2.CrossedThresholds)

	// Case 3: Zero or negative limit
	status3 := BuildSpendLimitStatus(50.0, 0, SpendLimitThresholds)
	assert.Equal(t, 0.0, status3.LimitUSD)
	assert.False(t, status3.IsOverLimit)
	assert.Empty(t, status3.CrossedThresholds)
}

func TestCollectByokModels_And_ExtractFromConfig(t *testing.T) {
	byokConfig := &BYOKConfigRef{
		Models: []BYOKModelConfig{
			{Model: "gpt-4o", CredentialID: "cred-1"},
			{Model: "claude-3-5-sonnet", CredentialID: "cred-2"},
		},
	}

	nestedConfig := map[string]any{
		"rules": []any{
			map[string]any{
				"byokModel": "deepseek-coder",
			},
		},
		"overrides": map[string]any{
			"directory": map[string]any{
				"byokModel": "gemini-2.5-pro",
			},
		},
	}

	extra := ExtractByokModelsFromConfig(nestedConfig)
	require.Len(t, extra, 2)
	assert.Contains(t, extra, "deepseek-coder")
	assert.Contains(t, extra, "gemini-2.5-pro")

	allModels := CollectByokModels(byokConfig, extra)
	assert.Len(t, allModels, 4)
	assert.Contains(t, allModels, "gpt-4o")
	assert.Contains(t, allModels, "claude-3-5-sonnet")
	assert.Contains(t, allModels, "deepseek-coder")
	assert.Contains(t, allModels, "gemini-2.5-pro")
}

// MockParamsService provides parameter storage.
type MockParamsService struct {
	Store map[string]any
}

func NewMockParams() *MockParamsService {
	return &MockParamsService{Store: make(map[string]any)}
}

func (m *MockParamsService) Get(ctx context.Context, orgID, teamID, key string) (any, error) {
	return m.Store[orgID+"|"+key], nil
}

func (m *MockParamsService) Set(ctx context.Context, orgID, teamID, key string, val any) error {
	m.Store[orgID+"|"+key] = val
	return nil
}

func (m *MockParamsService) ListConfigured(ctx context.Context, key string) ([]OrgParameterRecord, error) {
	return nil, nil
}

func TestConfigureSpendLimitUseCase_ValidationAndSave(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	mockParams := NewMockParams()

	calc := pricing.NewModelCostCalculator(resolver)
	monthlySpend := usage.NewMonthlySpendUseCase(nil, calc)

	configService := NewSpendLimitConfigService(mockParams, monthlySpend, resolver)
	uc := NewConfigureSpendLimitUseCase(configService)

	// Case 1: Enabled with non-positive limit fails
	_, err := uc.Execute(ctx, ConfigureSpendLimitInput{
		OrganizationID:  "org-1",
		Enabled:         true,
		MonthlyLimitUSD: 0,
	})
	assert.Error(t, err)

	// Case 2: Enabled with unpriceable model fails
	_, err = uc.Execute(ctx, ConfigureSpendLimitInput{
		OrganizationID:  "org-1",
		Enabled:         true,
		MonthlyLimitUSD: 100,
		Models:          []string{"unpriceable-custom-model"},
	})
	assert.Error(t, err)
	var priceErr *SpendLimitPriceabilityError
	assert.ErrorAs(t, err, &priceErr)

	// Case 3: Enabled with known catalog models succeeds
	cfg, err := uc.Execute(ctx, ConfigureSpendLimitInput{
		OrganizationID:  "org-1",
		Enabled:         true,
		MonthlyLimitUSD: 250,
		Models:          []string{"gpt-4o", "claude-3-5-sonnet"},
	})
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 250.0, cfg.MonthlyLimitUSD)
}

// MockNotificationService tracks emitted alerts.
type MockNotificationService struct {
	Emitted []NotificationEvent
}

func (m *MockNotificationService) Emit(ctx context.Context, event NotificationEvent, orgID string, payload NotificationPayload) error {
	m.Emitted = append(m.Emitted, event)
	return nil
}

type mockUsageService struct {
	dailyFunc func(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageResultContract, error)
}

func (m *mockUsageService) GetSummary(ctx context.Context, q usage.TokenUsageQueryContract) (usage.UsageSummaryContract, error) {
	return usage.UsageSummaryContract{}, nil
}
func (m *mockUsageService) GetSummaryByModel(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.BaseUsageContract, error) {
	return nil, nil
}
func (m *mockUsageService) GetDailyUsage(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageResultContract, error) {
	if m.dailyFunc != nil {
		return m.dailyFunc(ctx, q)
	}
	return nil, nil
}
func (m *mockUsageService) GetModelCredentialPairs(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.ModelCredentialPair, error) {
	return nil, nil
}
func (m *mockUsageService) GetUsageByPr(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByPrResultContract, error) {
	return nil, nil
}
func (m *mockUsageService) GetDailyUsageByPr(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageByPrResultContract, error) {
	return nil, nil
}
func (m *mockUsageService) GetUsageByReview(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByReviewResultContract, error) {
	return nil, nil
}
func (m *mockUsageService) GetUsageByArea(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByAreaResultContract, error) {
	return nil, nil
}
func (m *mockUsageService) GetUsageOverview(ctx context.Context, q usage.TokenUsageQueryContract) (*usage.UsageOverviewReportContract, error) {
	return nil, nil
}

func TestSpendLimitAlertService_RunForOrganization(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	calc := pricing.NewModelCostCalculator(resolver)

	mockUsage := &mockUsageService{
		dailyFunc: func(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageResultContract, error) {
			return []usage.DailyUsageResultContract{
				{
					BaseUsageContract: usage.BaseUsageContract{
						Model:  "gpt-4o",
						Input:  10_000_000, // $25
						Output: 10_000_000, // $100 => $125 total
						Total:  20_000_000,
					},
					Date: "2026-09-01",
				},
			}, nil
		},
	}

	monthlySpend := usage.NewMonthlySpendUseCase(mockUsage, calc)
	mockParams := NewMockParams()
	mockParams.Store["org-test|SPEND_LIMIT_CONFIG"] = SpendLimitConfig{
		Enabled:         true,
		MonthlyLimitUSD: 100.0, // $125 is 125% -> all thresholds crossed (50, 75, 90, 100)
	}

	configService := NewSpendLimitConfigService(mockParams, monthlySpend, resolver)
	notifService := &MockNotificationService{}
	alertService := NewSpendLimitAlertService(configService, notifService)

	err := alertService.RunForOrganization(ctx, "org-test", "", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	// Should emit 4 threshold alerts: 50, 75, 90, 100
	assert.Len(t, notifService.Emitted, 4)
	for _, e := range notifService.Emitted {
		assert.Equal(t, EventSpendLimitThresholdReached, e)
	}

	// Stored config should have saved thresholdsSent
	saved, err := configService.GetConfig(ctx, "org-test", "")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, []int{50, 75, 90, 100}, saved.ThresholdsSent["2026-09"])
}

func TestGetOrgByokModelsUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("collects every configured v2 model plus per-repo/directory overrides deduped", func(t *testing.T) {
		mockOrg := NewMockParams()
		mockOrg.Store["org-1|BYOK_CONFIG"] = BYOKConfigRef{
			Version: 2,
			Models: []BYOKModelConfig{
				{Model: "gpt-main", CredentialID: "c1"},
				{Model: "claude-fallback", CredentialID: "c1"},
			},
		}

		mockRepo := NewMockParams()
		mockRepo.Store["org-1|CODE_REVIEW_CONFIG"] = map[string]any{
			"byokModel": "global-model",
			"repositories": []any{
				map[string]any{
					"configs": map[string]any{"byokModel": "repo-model"},
					"directories": []any{
						map[string]any{"configs": map[string]any{"byokModel": "dir-model"}},
						map[string]any{"configs": map[string]any{"byokModel": "repo-model"}}, // duplicate
					},
				},
			},
		}

		uc := NewGetOrgByokModelsUseCase(mockOrg, mockRepo)
		models := uc.Execute(ctx, "org-1", "")

		assert.Equal(t, []string{
			"gpt-main",
			"claude-fallback",
			"global-model",
			"repo-model",
			"dir-model",
		}, models)
	})

	t.Run("ignores inherit-marker empty string byokModel overrides", func(t *testing.T) {
		mockOrg := NewMockParams()
		mockOrg.Store["org-1|BYOK_CONFIG"] = BYOKConfigRef{
			Version: 2,
			Models: []BYOKModelConfig{
				{Model: "gpt-main"},
			},
		}

		mockRepo := NewMockParams()
		mockRepo.Store["org-1|CODE_REVIEW_CONFIG"] = map[string]any{
			"repositories": []any{
				map[string]any{"configs": map[string]any{"byokModel": ""}},
				map[string]any{"configs": map[string]any{"byokModel": "real-model"}},
			},
		}

		uc := NewGetOrgByokModelsUseCase(mockOrg, mockRepo)
		models := uc.Execute(ctx, "org-1", "")

		assert.Equal(t, []string{"gpt-main", "real-model"}, models)
	})

	t.Run("falls back to configured models when code-review config is absent", func(t *testing.T) {
		mockOrg := NewMockParams()
		mockOrg.Store["org-1|BYOK_CONFIG"] = BYOKConfigRef{
			Version: 2,
			Models: []BYOKModelConfig{
				{Model: "only-main"},
			},
		}
		mockRepo := NewMockParams()

		uc := NewGetOrgByokModelsUseCase(mockOrg, mockRepo)
		models := uc.Execute(ctx, "org-1", "")

		assert.Equal(t, []string{"only-main"}, models)
	})

	t.Run("empty list when nothing configured", func(t *testing.T) {
		mockOrg := NewMockParams()
		mockRepo := NewMockParams()

		uc := NewGetOrgByokModelsUseCase(mockOrg, mockRepo)
		models := uc.Execute(ctx, "org-1", "")

		assert.Empty(t, models)
	})
}
