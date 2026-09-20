package usage

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
)

var DefaultSpendLimitThresholds = []int{50, 75, 90, 100}

// MonthlySpendResult represents month-to-date spend for an organization.
type MonthlySpendResult struct {
	OrganizationID string                    `json:"organizationId"`
	PeriodKey      string                    `json:"periodKey"`
	SpentUSD       float64                   `json:"spentUsd"`
	TokenUsage     TokenUsageBreakdown       `json:"tokenUsage"`
	ByModel        []pricing.ModelSpend      `json:"byModel"`
	ByCredential   []pricing.CredentialSpend `json:"byCredential"`
	RunRate        pricing.RunRateProjection `json:"runRate"`
}

// BYOKModelConfig represents configured model reference.
type BYOKModelConfig struct {
	Model        string `json:"model"`
	CredentialID string `json:"credentialId"`
}

// BYOKConfigRef represents the minimal BYOK config structure needed for credential attribution.
type BYOKConfigRef struct {
	Version int               `json:"version,omitempty"`
	Models  []BYOKModelConfig `json:"models"`
}

// MonthlySpendUseCase computes live month-to-date spend, credential rollups, and run rates.
type MonthlySpendUseCase struct {
	tokenUsageService ITokenUsageService
	calculator        *pricing.ModelCostCalculator
}

// NewMonthlySpendUseCase creates a new monthly spend use case.
func NewMonthlySpendUseCase(
	tokenUsageService ITokenUsageService,
	calculator *pricing.ModelCostCalculator,
) *MonthlySpendUseCase {
	return &MonthlySpendUseCase{
		tokenUsageService: tokenUsageService,
		calculator:        calculator,
	}
}

// GetMonthToDateSpend computes month-to-date spend and breakdowns.
func (uc *MonthlySpendUseCase) GetMonthToDateSpend(
	ctx context.Context,
	organizationID string,
	now time.Time,
	overrides pricing.ManualPricingOverrides,
	byokConfig *BYOKConfigRef,
) (*MonthlySpendResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	start, end, periodKey, monthMs := uc.getMonthRange(now)

	rows, err := uc.tokenUsageService.GetDailyUsage(ctx, TokenUsageQueryContract{
		OrganizationID: organizationID,
		Start:          start,
		End:            end,
		BYOK:           true,
	})
	if err != nil {
		return nil, err
	}

	costRows := make([]pricing.CostUsageRow, len(rows))
	for i, r := range rows {
		costRows[i] = pricing.CostUsageRow{
			Input:           r.Input,
			Output:          r.Output,
			OutputReasoning: r.OutputReasoning,
			CacheRead:       r.CacheRead,
			CacheWrite:      r.CacheWrite,
			Model:           r.Model,
			ByTier:          r.ByTier,
		}
	}

	byModel, err := uc.calculator.SpendByModel(ctx, costRows, overrides)
	if err != nil {
		return nil, err
	}

	var spentUSD float64
	for _, m := range byModel {
		spentUSD += m.SpentUSD
	}
	spentUSD = uc.roundToCents(spentUSD)

	// Fetch usage-derived (model, credentialId) pairs
	pairs, _ := uc.tokenUsageService.GetModelCredentialPairs(ctx, TokenUsageQueryContract{
		OrganizationID: organizationID,
		Start:          start,
		End:            end,
		BYOK:           true,
	})

	usageCredByModel := make(map[string]string)
	for _, p := range pairs {
		cleanModel := strings.TrimSpace(p.Model)
		cleanCred := strings.TrimSpace(p.CredentialID)
		if cleanModel != "" && cleanCred != "" {
			if _, exists := usageCredByModel[cleanModel]; !exists {
				usageCredByModel[cleanModel] = cleanCred
			}
		}
	}

	byCredential := uc.rollupByCredential(byModel, byokConfig, usageCredByModel)
	runRate := uc.computeRunRate(spentUSD, now, start, monthMs)

	return &MonthlySpendResult{
		OrganizationID: organizationID,
		PeriodKey:      periodKey,
		SpentUSD:       spentUSD,
		TokenUsage:     uc.aggregateTokenUsage(rows),
		ByModel:        byModel,
		ByCredential:   byCredential,
		RunRate:        runRate,
	}, nil
}

// GetStatus evaluates month-to-date spend against a monthly budget limit.
func (uc *MonthlySpendUseCase) GetStatus(
	ctx context.Context,
	organizationID string,
	limitUSD float64,
	now time.Time,
	overrides pricing.ManualPricingOverrides,
	byokConfig *BYOKConfigRef,
) (*pricing.SpendLimitEvaluation, error) {
	spend, err := uc.GetMonthToDateSpend(ctx, organizationID, now, overrides, byokConfig)
	if err != nil {
		return nil, err
	}

	status := pricing.BuildSpendLimitStatus(spend.SpentUSD, limitUSD, DefaultSpendLimitThresholds)

	return &pricing.SpendLimitEvaluation{
		SpendLimitStatus: status,
		OrganizationID:   organizationID,
		PeriodKey:        spend.PeriodKey,
		ByModel:          spend.ByModel,
		ByCredential:     spend.ByCredential,
		RunRate:          spend.RunRate,
	}, nil
}

func (uc *MonthlySpendUseCase) getMonthRange(now time.Time) (time.Time, time.Time, string, int64) {
	year := now.Year()
	month := now.Month()

	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)

	periodKey := fmt.Sprintf("%04d-%02d", year, month)
	monthMs := nextMonthStart.Sub(start).Milliseconds()

	return start, now, periodKey, monthMs
}

func (uc *MonthlySpendUseCase) rollupByCredential(
	byModel []pricing.ModelSpend,
	byokConfig *BYOKConfigRef,
	usageCredByModel map[string]string,
) []pricing.CredentialSpend {
	modelToCredential := make(map[string]string)
	if byokConfig != nil {
		for _, m := range byokConfig.Models {
			name := strings.TrimSpace(m.Model)
			if name != "" {
				if _, exists := modelToCredential[name]; !exists {
					modelToCredential[name] = m.CredentialID
				}
			}
		}
	}

	totals := make(map[string]float64)
	for _, m := range byModel {
		credID := pricing.UnattributedCredential
		if id, ok := usageCredByModel[m.Model]; ok && id != "" {
			credID = id
		} else if id, ok := modelToCredential[m.Model]; ok && id != "" {
			credID = id
		}
		totals[credID] += m.SpentUSD
	}

	out := make([]pricing.CredentialSpend, 0, len(totals))
	for credID, sum := range totals {
		out = append(out, pricing.CredentialSpend{
			CredentialID: credID,
			SpentUSD:     uc.roundToCents(sum),
		})
	}
	return out
}

func (uc *MonthlySpendUseCase) computeRunRate(
	spentUSD float64,
	now time.Time,
	start time.Time,
	monthMs int64,
) pricing.RunRateProjection {
	elapsedMs := now.Sub(start).Milliseconds()
	var elapsedFraction float64
	if monthMs > 0 {
		elapsedFraction = float64(elapsedMs) / float64(monthMs)
	}

	var projectedMonthlyUSD float64
	if elapsedFraction > 0 {
		projectedMonthlyUSD = uc.roundToCents(spentUSD / elapsedFraction)
	}

	return pricing.RunRateProjection{
		ProjectedMonthlyUSD: projectedMonthlyUSD,
		ElapsedFraction:     elapsedFraction,
	}
}

func (uc *MonthlySpendUseCase) aggregateTokenUsage(rows []DailyUsageResultContract) TokenUsageBreakdown {
	var totals TokenUsageBreakdown
	for _, r := range rows {
		totals.InputTokens += r.Input
		totals.OutputTokens += r.Output
		totals.ReasoningTokens += r.OutputReasoning
		totals.CacheReadTokens += r.CacheRead
		totals.CacheWriteTokens += r.CacheWrite
	}
	totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	return totals
}

func (uc *MonthlySpendUseCase) roundToCents(value float64) float64 {
	return math.Round(value*100) / 100
}
