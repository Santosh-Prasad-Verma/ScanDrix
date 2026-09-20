package helpers

import (
	"github.com/scandrix/backend/internal/cockpit/domain"
)

const (
	// MinTriggersToJudge specifies the minimum executions needed to evaluate rule health.
	MinTriggersToJudge = 5

	// IgnoredMaxRate specifies the implementation threshold under which a rule is deemed ignored.
	IgnoredMaxRate = 0.2

	// NoisyMinThumbsDown specifies the minimum downvotes needed to qualify a rule as noisy.
	NoisyMinThumbsDown = 3

	// NoisyMinDownvoteRate specifies the minimum ratio of downvotes to triggers for noisy classification.
	NoisyMinDownvoteRate = 0.1
)

// ComputeRuleState evaluates the health category of a rule based on real execution data and developer feedback.
func ComputeRuleState(usage *domain.DrixyRuleUsageRow) (domain.DrixyRuleHealthState, domain.DrixyRuleUsageRow) {
	if usage == nil || usage.Triggers == 0 {
		var emptyUsage domain.DrixyRuleUsageRow
		if usage != nil {
			emptyUsage.RuleID = usage.RuleID
			emptyUsage.ThumbsUp = usage.ThumbsUp
			emptyUsage.ThumbsDown = usage.ThumbsDown
			emptyUsage.LastTriggeredAt = usage.LastTriggeredAt
		}
		return domain.RuleStateStale, emptyUsage
	}

	state := domain.RuleStateHealthy
	if usage.Triggers < MinTriggersToJudge {
		state = domain.RuleStateLowData
	} else if usage.ThumbsDown >= NoisyMinThumbsDown &&
		usage.ThumbsDown > usage.ThumbsUp &&
		(float64(usage.ThumbsDown)/float64(usage.Triggers)) >= NoisyMinDownvoteRate {
		state = domain.RuleStateNoisy
	} else if usage.Rate <= IgnoredMaxRate {
		state = domain.RuleStateIgnored
	}

	return state, *usage
}
