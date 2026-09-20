package spendlimit

import (
	"sort"
)

// DecideSpendAlerts determines which spend alerts to emit given an evaluation and prior alert state.
// Pure and idempotent: each threshold fires at most once, and the final "won't notify again" notice
// fires exactly one tick AFTER 100% was alerted on a prior tick.
func DecideSpendAlerts(
	evaluation struct {
		CrossedThresholds []int
		IsOverLimit       bool
	},
	state SpendAlertState,
) SpendAlertDecision {
	sent := state.ThresholdsSent
	finalNoticeSent := state.FinalNoticeSent

	sentMap := make(map[int]bool, len(sent))
	for _, t := range sent {
		sentMap[t] = true
	}

	thresholdsToAlert := make([]int, 0)
	for _, t := range evaluation.CrossedThresholds {
		if !sentMap[t] {
			thresholdsToAlert = append(thresholdsToAlert, t)
		}
	}

	mergedSet := make(map[int]bool)
	for _, t := range sent {
		mergedSet[t] = true
	}
	for _, t := range evaluation.CrossedThresholds {
		mergedSet[t] = true
	}

	nextThresholdsSent := make([]int, 0, len(mergedSet))
	for t := range mergedSet {
		nextThresholdsSent = append(nextThresholdsSent, t)
	}
	sort.Ints(nextThresholdsSent)

	// Final notice waits until 100% was alerted on a PRIOR tick — never the same tick
	sendFinalNotice := evaluation.IsOverLimit && sentMap[100] && !finalNoticeSent
	nextFinalNoticeSent := finalNoticeSent || sendFinalNotice

	changed := len(thresholdsToAlert) > 0 || sendFinalNotice

	return SpendAlertDecision{
		ThresholdsToAlert:   thresholdsToAlert,
		SendFinalNotice:     sendFinalNotice,
		NextThresholdsSent:  nextThresholdsSent,
		NextFinalNoticeSent: nextFinalNoticeSent,
		Changed:             changed,
	}
}
