package dora

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CalculateDORA derives standard DORA metrics and overall tier from deployment and review records.
func CalculateDORA(
	wsID uuid.UUID,
	start, end time.Time,
	records []DeploymentRecord,
	totalPRs int,
	defectsCaught int,
) *DORAReport {
	totalDays := end.Sub(start).Hours() / 24.0
	if totalDays <= 0 {
		totalDays = 1.0
	}

	report := &DORAReport{
		WorkspaceID: wsID,
		PeriodStart: start,
		PeriodEnd:   end,
	}

	// 1. Deployment Frequency
	totalDeploys := len(records)
	deploysPerDay := float64(totalDeploys) / totalDays
	dfTier := classifyDeploymentFrequency(deploysPerDay)

	report.DeploymentFrequency = MetricDeploymentFrequency{
		TotalDeployments: totalDeploys,
		DeploysPerDay:    math.Round(deploysPerDay*100) / 100,
		Tier:             dfTier,
	}

	// 2. Lead Time for Changes
	var totalLead time.Duration
	var leads []time.Duration
	for _, r := range records {
		if r.LeadDuration > 0 {
			totalLead += r.LeadDuration
			leads = append(leads, r.LeadDuration)
		}
	}

	avgLead := time.Duration(0)
	medianLead := time.Duration(0)
	ltTier := TierLow

	if len(leads) > 0 {
		avgLead = totalLead / time.Duration(len(leads))
		sort.Slice(leads, func(i, j int) bool { return leads[i] < leads[j] })
		medianLead = leads[len(leads)/2]
		ltTier = classifyLeadTime(medianLead)
	}

	report.LeadTimeForChanges = MetricLeadTime{
		AverageLeadTime: avgLead,
		MedianLeadTime:  medianLead,
		Tier:            ltTier,
	}

	// 3. Change Failure Rate
	failedCount := 0
	var totalRecovery time.Duration
	resolvedIncidents := 0

	for _, r := range records {
		if r.IsFailed {
			failedCount++
			if r.IncidentResolvedAt != nil && r.IncidentResolvedAt.After(r.DeployedAt) {
				recovery := r.IncidentResolvedAt.Sub(r.DeployedAt)
				totalRecovery += recovery
				resolvedIncidents++
			}
		}
	}

	cfr := 0.0
	if totalDeploys > 0 {
		cfr = float64(failedCount) / float64(totalDeploys)
	}
	cfrTier := classifyChangeFailureRate(cfr)

	report.ChangeFailureRate = MetricChangeFailureRate{
		TotalDeployments:  totalDeploys,
		FailedDeployments: failedCount,
		FailureRate:       math.Round(cfr*100) / 100,
		Tier:              cfrTier,
	}

	// 4. Time to Restore (MTTR)
	avgMTTR := time.Duration(0)
	mttrTier := TierLow
	if resolvedIncidents > 0 {
		avgMTTR = totalRecovery / time.Duration(resolvedIncidents)
		mttrTier = classifyTimeToRestore(avgMTTR)
	} else if failedCount == 0 {
		mttrTier = TierElite // No failures = Elite MTTR
	}

	report.TimeToRestore = MetricTimeToRestore{
		IncidentCount: failedCount,
		AverageMTTR:   avgMTTR,
		Tier:          mttrTier,
	}

	// 5. Review Velocity & Developer Impact
	hoursSaved := float64(defectsCaught) * 0.50 // 30 minutes saved per prevented vulnerability
	// Cycle time and review latency have no collected source data. They used
	// to be hardcoded to 24h and 45s, which reported invented values as
	// measurements. They are left absent and named in Unavailable instead.
	// AUDIT_REMEDIATION.md F-40.
	report.ReviewVelocity = ReviewVelocityMetrics{
		TotalPRsReviewed:         totalPRs,
		DefectsCaughtBeforeMerge: defectsCaught,
		EngineeringHoursSaved:    hoursSaved,
	}
	report.Unavailable = append(report.Unavailable,
		"review_velocity.average_cycle_time:no_data_source",
		"review_velocity.average_review_latency:no_data_source",
	)

	// 6. Overall Performance Tier
	report.OverallTier = computeOverallTier([]PerformanceTier{dfTier, ltTier, cfrTier, mttrTier})

	return report
}

func classifyDeploymentFrequency(perDay float64) PerformanceTier {
	switch {
	case perDay >= 1.0:
		return TierElite
	case perDay >= 0.14: // at least once a week
		return TierHigh
	case perDay >= 0.033: // at least once a month
		return TierMedium
	default:
		return TierLow
	}
}

func classifyLeadTime(d time.Duration) PerformanceTier {
	switch {
	case d < 1*time.Hour:
		return TierElite
	case d <= 7*24*time.Hour:
		return TierHigh
	case d <= 30*24*time.Hour:
		return TierMedium
	default:
		return TierLow
	}
}

func classifyChangeFailureRate(rate float64) PerformanceTier {
	switch {
	case rate <= 0.15:
		return TierElite
	case rate <= 0.30:
		return TierHigh
	case rate <= 0.45:
		return TierMedium
	default:
		return TierLow
	}
}

func classifyTimeToRestore(d time.Duration) PerformanceTier {
	switch {
	case d < 1*time.Hour:
		return TierElite
	case d <= 24*time.Hour:
		return TierHigh
	case d <= 7*24*time.Hour:
		return TierMedium
	default:
		return TierLow
	}
}

func computeOverallTier(tiers []PerformanceTier) PerformanceTier {
	scores := map[PerformanceTier]int{
		TierElite:  4,
		TierHigh:   3,
		TierMedium: 2,
		TierLow:    1,
	}

	sum := 0
	for _, t := range tiers {
		sum += scores[t]
	}

	avg := float64(sum) / float64(len(tiers))
	switch {
	case avg >= 3.5:
		return TierElite
	case avg >= 2.5:
		return TierHigh
	case avg >= 1.5:
		return TierMedium
	default:
		return TierLow
	}
}
