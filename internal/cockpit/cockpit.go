package cockpit

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CodeHealthMetrics aggregates security risk and quality debt metrics.
type CodeHealthMetrics struct {
	WorkspaceID          uuid.UUID `json:"workspace_id"`
	OpenVulnerabilities  int       `json:"open_vulnerabilities"`
	CriticalExposureDays float64   `json:"critical_exposure_days"`
	SecurityDebtIndex    float64   `json:"security_debt_index"`
	MeanTimeToRemediate  float64   `json:"mean_time_to_remediate_hours"`
}

// DeveloperProductivityMetrics tracks cycle velocity and review workload.
type DeveloperProductivityMetrics struct {
	WorkspaceID            uuid.UUID `json:"workspace_id"`
	PullRequestsReviewed   int       `json:"pull_requests_reviewed"`
	AverageTurnaroundHours float64   `json:"average_turnaround_hours"`
	AutomatedReviewRatio   float64   `json:"automated_review_ratio"` // % PRs reviewed without human delay
	ReviewerSaturationRate float64   `json:"reviewer_saturation_rate"`
}

// ExecutiveCockpitReport provides a high-level weekly digest for engineering leadership.
type ExecutiveCockpitReport struct {
	WorkspaceID  uuid.UUID                    `json:"workspace_id"`
	ReportPeriod string                       `json:"report_period"`
	Health       CodeHealthMetrics            `json:"health"`
	Productivity DeveloperProductivityMetrics `json:"productivity"`
	GeneratedAt  time.Time                    `json:"generated_at"`
}

// FormatMarkdown outputs an executive briefing document.
func (r *ExecutiveCockpitReport) FormatMarkdown() string {
	return fmt.Sprintf(`# 🚀 Executive Engineering Cockpit Report
**Period:** %s | **Generated:** %s

## 🛡️ Code & Security Health
- **Open Vulnerabilities:** %d
- **Security Debt Index:** %.2f
- **Critical Exposure Days:** %.1f days
- **Mean Time to Remediate (MTTR):** %.1f hours

## ⚡ Velocity & Productivity
- **PRs Reviewed:** %d
- **Average Turnaround:** %.1f hours
- **Automated Review Ratio:** %.1f%%
- **Reviewer Workload Saturation:** %.1f%%
`,
		r.ReportPeriod,
		r.GeneratedAt.Format(time.RFC3339),
		r.Health.OpenVulnerabilities,
		r.Health.SecurityDebtIndex,
		r.Health.CriticalExposureDays,
		r.Health.MeanTimeToRemediate,
		r.Productivity.PullRequestsReviewed,
		r.Productivity.AverageTurnaroundHours,
		r.Productivity.AutomatedReviewRatio*100.0,
		r.Productivity.ReviewerSaturationRate*100.0,
	)
}

// CockpitAggregator calculates executive KPIs across repositories.
type CockpitAggregator struct{}

func NewCockpitAggregator() *CockpitAggregator {
	return &CockpitAggregator{}
}

// CalculateSecurityDebt computes the composite debt index: sum(severity_weight * exposure_days).
func (a *CockpitAggregator) CalculateSecurityDebt(criticalCount, highCount int, avgExposureDays float64) float64 {
	// Critical weight: 5.0, High weight: 2.0
	score := (float64(criticalCount)*5.0 + float64(highCount)*2.0) * (1.0 + avgExposureDays/30.0)
	return score
}
