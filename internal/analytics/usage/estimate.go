package usage

import (
	"context"
	"math"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

const (
	PeriodDays     = 14
	ProjectionDays = 30
)

// CostEstimateUseCase calculates trailing cost trends and estimates future monthly spend per developer.
type CostEstimateUseCase struct {
	tokenUsageService   ITokenUsageService
	pullRequestsService IPullRequestsService
	calculator          *pricing.ModelCostCalculator
}

// NewCostEstimateUseCase creates a new cost estimate use case.
func NewCostEstimateUseCase(
	tokenUsageService ITokenUsageService,
	pullRequestsService IPullRequestsService,
	calculator *pricing.ModelCostCalculator,
) *CostEstimateUseCase {
	return &CostEstimateUseCase{
		tokenUsageService:   tokenUsageService,
		pullRequestsService: pullRequestsService,
		calculator:          calculator,
	}
}

// Execute computes 14-day trailing cost and 30-day projection per developer.
func (uc *CostEstimateUseCase) Execute(ctx context.Context, organizationID string) (*CostEstimateContract, error) {
	start, end := uc.getDateRange()

	usageByPr, err := uc.tokenUsageService.GetUsageByPr(ctx, TokenUsageQueryContract{
		OrganizationID: organizationID,
		Start:          start,
		End:            end,
		BYOK:           false,
	})
	if err != nil {
		return nil, err
	}

	totals := uc.aggregateTokenUsage(usageByPr)

	prNumbers := make([]int, len(usageByPr))
	costRows := make([]pricing.CostUsageRow, len(usageByPr))
	for i, u := range usageByPr {
		prNumbers[i] = u.PRNumber
		costRows[i] = pricing.CostUsageRow{
			Input:           u.Input,
			Output:          u.Output,
			OutputReasoning: u.OutputReasoning,
			CacheRead:       u.CacheRead,
			CacheWrite:      u.CacheWrite,
			Model:           u.Model,
			ByTier:          u.ByTier,
		}
	}

	developerCount, err := uc.countUniqueDevelopers(ctx, prNumbers, organizationID)
	if err != nil || developerCount < 1 {
		developerCount = 1
	}

	totalCost14Days, err := uc.calculator.TotalCost(ctx, costRows, nil)
	if err != nil {
		return nil, err
	}

	estimatedMonthlyCost := totalCost14Days * (float64(ProjectionDays) / float64(PeriodDays))
	costPerDeveloper := estimatedMonthlyCost / float64(developerCount)

	return &CostEstimateContract{
		EstimatedMonthlyCost: uc.roundToTwoDecimals(estimatedMonthlyCost),
		CostPerDeveloper:     uc.roundToTwoDecimals(costPerDeveloper),
		DeveloperCount:       developerCount,
		TokenUsage:           totals,
		PeriodDays:           PeriodDays,
		ProjectionDays:       ProjectionDays,
	}, nil
}

func (uc *CostEstimateUseCase) getDateRange() (time.Time, time.Time) {
	now := time.Now().UTC()
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)
	start := time.Date(now.Year(), now.Month(), now.Day()-PeriodDays, 0, 0, 0, 0, time.UTC)
	return start, end
}

func (uc *CostEstimateUseCase) aggregateTokenUsage(usages []UsageByPrResultContract) TokenUsageBreakdown {
	var totals TokenUsageBreakdown

	for _, u := range usages {
		totals.InputTokens += u.Input
		totals.OutputTokens += u.Output
		totals.ReasoningTokens += u.OutputReasoning
		totals.CacheReadTokens += u.CacheRead
		totals.CacheWriteTokens += u.CacheWrite
	}

	// OutputTokens already includes reasoningTokens for every provider
	totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	return totals
}

func (uc *CostEstimateUseCase) countUniqueDevelopers(ctx context.Context, prNumbers []int, orgID string) (int, error) {
	if len(prNumbers) == 0 || uc.pullRequestsService == nil {
		return 1, nil
	}

	uniqueNums := make([]int, 0, len(prNumbers))
	seen := make(map[int]bool)
	for _, num := range prNumbers {
		if !seen[num] {
			seen[num] = true
			uniqueNums = append(uniqueNums, num)
		}
	}

	mappings, err := uc.pullRequestsService.FindManyByNumbers(ctx, uniqueNums, orgID)
	if err != nil {
		return 1, nil
	}

	devs := make(map[string]bool)
	for _, m := range mappings {
		if m.Username != "" {
			devs[m.Username] = true
		}
	}

	if len(devs) == 0 {
		return 1, nil
	}
	return len(devs), nil
}

func (uc *CostEstimateUseCase) roundToTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
