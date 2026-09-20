package spendlimit

import (
	"github.com/scandrix/backend/internal/analytics/pricing"
)

// BuildSpendLimitStatus evaluates month-to-date spend against a monthly budget limit.
// Clamps non-positive limits and returns whether limit is reached and which thresholds were crossed.
func BuildSpendLimitStatus(
	spentUSD float64,
	limitUSD float64,
	thresholds []int,
) pricing.SpendLimitStatus {
	if len(thresholds) == 0 {
		thresholds = SpendLimitThresholds
	}
	return pricing.BuildSpendLimitStatus(spentUSD, limitUSD, thresholds)
}
