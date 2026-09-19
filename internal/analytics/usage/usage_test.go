package usage

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTokenUsageService provides test responses for usage tests.
type MockTokenUsageService struct {
	SummaryFunc              func(ctx context.Context, q TokenUsageQueryContract) (UsageSummaryContract, error)
	SummaryByModelFunc       func(ctx context.Context, q TokenUsageQueryContract) ([]BaseUsageContract, error)
	DailyUsageFunc           func(ctx context.Context, q TokenUsageQueryContract) ([]DailyUsageResultContract, error)
	ModelCredentialPairsFunc func(ctx context.Context, q TokenUsageQueryContract) ([]ModelCredentialPair, error)
	UsageByPrFunc            func(ctx context.Context, q TokenUsageQueryContract) ([]UsageByPrResultContract, error)
	DailyUsageByPrFunc       func(ctx context.Context, q TokenUsageQueryContract) ([]DailyUsageByPrResultContract, error)
	UsageOverviewFunc        func(ctx context.Context, q TokenUsageQueryContract) (*UsageOverviewReportContract, error)
}

func (m *MockTokenUsageService) GetSummary(ctx context.Context, q TokenUsageQueryContract) (UsageSummaryContract, error) {
	if m.SummaryFunc != nil {
		return m.SummaryFunc(ctx, q)
	}
	return UsageSummaryContract{Input: 1000, Output: 200, Total: 1200}, nil
}

func (m *MockTokenUsageService) GetSummaryByModel(ctx context.Context, q TokenUsageQueryContract) ([]BaseUsageContract, error) {
	if m.SummaryByModelFunc != nil {
		return m.SummaryByModelFunc(ctx, q)
	}
	return []BaseUsageContract{
		{Model: "claude-3-5-sonnet", Input: 1000, Output: 200, Total: 1200},
	}, nil
}

func (m *MockTokenUsageService) GetDailyUsage(ctx context.Context, q TokenUsageQueryContract) ([]DailyUsageResultContract, error) {
	if m.DailyUsageFunc != nil {
		return m.DailyUsageFunc(ctx, q)
	}
	return []DailyUsageResultContract{
		{BaseUsageContract: BaseUsageContract{Model: "gpt-4o", Input: 500, Output: 100, Total: 600}, Date: "2026-09-01"},
	}, nil
}

func (m *MockTokenUsageService) GetModelCredentialPairs(ctx context.Context, q TokenUsageQueryContract) ([]ModelCredentialPair, error) {
	if m.ModelCredentialPairsFunc != nil {
		return m.ModelCredentialPairsFunc(ctx, q)
	}
	return []ModelCredentialPair{
		{Model: "gpt-4o", CredentialID: "cred-openai-1"},
	}, nil
}

func (m *MockTokenUsageService) GetUsageByPr(ctx context.Context, q TokenUsageQueryContract) ([]UsageByPrResultContract, error) {
	if m.UsageByPrFunc != nil {
		return m.UsageByPrFunc(ctx, q)
	}
	return []UsageByPrResultContract{
		{BaseUsageContract: BaseUsageContract{Model: "gpt-4o", Input: 500, Output: 100, Total: 600}, PRNumber: 101},
	}, nil
}

func (m *MockTokenUsageService) GetDailyUsageByPr(ctx context.Context, q TokenUsageQueryContract) ([]DailyUsageByPrResultContract, error) {
	if m.DailyUsageByPrFunc != nil {
		return m.DailyUsageByPrFunc(ctx, q)
	}
	return []DailyUsageByPrResultContract{
		{
			UsageByPrResultContract: UsageByPrResultContract{
				BaseUsageContract: BaseUsageContract{Model: "gpt-4o", Input: 500, Output: 100, Total: 600},
				PRNumber:          101,
			},
			Date: "2026-09-01",
		},
	}, nil
}

func (m *MockTokenUsageService) GetUsageByReview(ctx context.Context, q TokenUsageQueryContract) ([]UsageByReviewResultContract, error) {
	return nil, nil
}

func (m *MockTokenUsageService) GetUsageByArea(ctx context.Context, q TokenUsageQueryContract) ([]UsageByAreaResultContract, error) {
	return nil, nil
}

func (m *MockTokenUsageService) GetUsageOverview(ctx context.Context, q TokenUsageQueryContract) (*UsageOverviewReportContract, error) {
	if m.UsageOverviewFunc != nil {
		return m.UsageOverviewFunc(ctx, q)
	}
	return &UsageOverviewReportContract{
		Summary: UsageSummaryReportContract{
			Totals: BaseUsageContract{Input: 1000, Output: 200, Total: 1200},
			ByModel: []EnrichedModelUsage{
				{BaseUsageContract: BaseUsageContract{Model: "claude-3-5-sonnet", Input: 1000, Output: 200, Total: 1200}},
			},
		},
	}, nil
}

// MockPullRequestsService provides pull request user mappings.
type MockPullRequestsService struct {
	Mappings map[int]string
}

func (m *MockPullRequestsService) FindOne(ctx context.Context, orgID string, prNumber int) (*IPullRequestUserMapping, error) {
	if u, ok := m.Mappings[prNumber]; ok {
		return &IPullRequestUserMapping{Number: prNumber, Username: u}, nil
	}
	return nil, nil
}

func (m *MockPullRequestsService) FindManyByNumbers(ctx context.Context, prNumbers []int, orgID string) ([]IPullRequestUserMapping, error) {
	res := make([]IPullRequestUserMapping, 0, len(prNumbers))
	for _, num := range prNumbers {
		if u, ok := m.Mappings[num]; ok {
			res = append(res, IPullRequestUserMapping{Number: num, Username: u})
		}
	}
	return res, nil
}

// MockCacheService stores items in memory for cache testing.
type MockCacheService struct {
	store map[string]any
}

func NewMockCache() *MockCacheService {
	return &MockCacheService{store: make(map[string]any)}
}

func (m *MockCacheService) Get(ctx context.Context, key string, dest any) bool {
	return false
}

func (m *MockCacheService) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	m.store[key] = val
	return nil
}

func TestBuildUsageSummaryUseCase_Execute(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	mockUsage := &MockTokenUsageService{}
	mockCache := NewMockCache()

	uc := NewBuildUsageSummaryUseCase(mockUsage, resolver, mockCache)

	now := time.Now().UTC()
	report, err := uc.Execute(ctx, TokenUsageQueryContract{
		OrganizationID: "org-123",
		Start:          now.Add(-24 * time.Hour),
		End:            now,
		BYOK:           false,
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, int64(1200), report.Totals.Total)
	require.Len(t, report.ByModel, 1)
	assert.Equal(t, "claude-3-5-sonnet", report.ByModel[0].Model)
	assert.Equal(t, ApiSourceCatalog, report.ByModel[0].PricingSource)
	assert.True(t, report.TotalCost.Total > 0)
}

func TestCostEstimateUseCase_Execute(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	calc := pricing.NewModelCostCalculator(resolver)

	mockUsage := &MockTokenUsageService{
		UsageByPrFunc: func(ctx context.Context, q TokenUsageQueryContract) ([]UsageByPrResultContract, error) {
			return []UsageByPrResultContract{
				{
					BaseUsageContract: BaseUsageContract{
						Model:  "claude-3-5-sonnet",
						Input:  100_000,
						Output: 20_000,
						Total:  120_000,
					},
					PRNumber: 42,
				},
			}, nil
		},
	}

	mockPR := &MockPullRequestsService{
		Mappings: map[int]string{
			42: "developer-alice",
		},
	}

	uc := NewCostEstimateUseCase(mockUsage, mockPR, calc)

	est, err := uc.Execute(ctx, "org-test")
	require.NoError(t, err)
	require.NotNil(t, est)

	assert.Equal(t, 1, est.DeveloperCount)
	assert.Equal(t, 14, est.PeriodDays)
	assert.Equal(t, 30, est.ProjectionDays)
	assert.True(t, est.EstimatedMonthlyCost > 0)
	assert.Equal(t, est.EstimatedMonthlyCost, est.CostPerDeveloper)
}

func TestMonthlySpendUseCase_GetStatus(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	calc := pricing.NewModelCostCalculator(resolver)

	mockUsage := &MockTokenUsageService{
		DailyUsageFunc: func(ctx context.Context, q TokenUsageQueryContract) ([]DailyUsageResultContract, error) {
			return []DailyUsageResultContract{
				{
					BaseUsageContract: BaseUsageContract{
						Model:  "gpt-4o",
						Input:  1_000_000, // $2.50
						Output: 500_000,   // $5.00 => $7.50 total
						Total:  1_500_000,
					},
					Date: "2026-09-05",
				},
			}, nil
		},
		ModelCredentialPairsFunc: func(ctx context.Context, q TokenUsageQueryContract) ([]ModelCredentialPair, error) {
			return []ModelCredentialPair{
				{Model: "gpt-4o", CredentialID: "cred-openai-prod"},
			}, nil
		},
	}

	uc := NewMonthlySpendUseCase(mockUsage, calc)

	eval, err := uc.GetStatus(ctx, "org-test", 10.00, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, eval)

	assert.InDelta(t, 7.50, eval.SpentUSD, 0.05)
	assert.Equal(t, 10.00, eval.LimitUSD)
	assert.InDelta(t, 75.0, eval.Pct, 0.5)
	assert.False(t, eval.IsOverLimit)
	assert.Contains(t, eval.CrossedThresholds, 50)
	assert.Contains(t, eval.CrossedThresholds, 75)
	require.Len(t, eval.ByCredential, 1)
	assert.Equal(t, "cred-openai-prod", eval.ByCredential[0].CredentialID)
}

func TestTokensByDeveloperUseCase_ExecuteAggregated(t *testing.T) {
	ctx := context.Background()
	mockUsage := &MockTokenUsageService{
		UsageByPrFunc: func(ctx context.Context, q TokenUsageQueryContract) ([]UsageByPrResultContract, error) {
			return []UsageByPrResultContract{
				{BaseUsageContract: BaseUsageContract{Model: "gpt-4o", Input: 100, Output: 20, Total: 120}, PRNumber: 1},
				{BaseUsageContract: BaseUsageContract{Model: "gpt-4o", Input: 200, Output: 40, Total: 240}, PRNumber: 2},
			}, nil
		},
	}

	mockPR := &MockPullRequestsService{
		Mappings: map[int]string{
			1: "alice",
			2: "alice",
		},
	}

	uc := NewTokensByDeveloperUseCase(mockUsage, mockPR, nil)

	res, err := uc.ExecuteAggregated(ctx, TokenUsageQueryContract{
		OrganizationID: "org-1",
		Start:          time.Now().Add(-24 * time.Hour),
		End:            time.Now(),
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "alice", res[0].Developer)
	assert.Equal(t, int64(300), res[0].Input)
	assert.Equal(t, int64(60), res[0].Output)
	assert.Equal(t, int64(360), res[0].Total)
}
