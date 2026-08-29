package dora

import (
	"time"

	"github.com/google/uuid"
)

// PerformanceTier rates DORA excellence (Elite, High, Medium, Low).
type PerformanceTier string

const (
	TierElite  PerformanceTier = "ELITE"
	TierHigh   PerformanceTier = "HIGH"
	TierMedium PerformanceTier = "MEDIUM"
	TierLow    PerformanceTier = "LOW"
)

// DeploymentRecord tracks a production release event for DORA metrics.
type DeploymentRecord struct {
	ID                 uuid.UUID     `json:"id"`
	WorkspaceID        uuid.UUID     `json:"workspace_id"`
	RepoNamespace      string        `json:"repo_namespace"`
	CommitSHA          string        `json:"commit_sha"`
	DeployedAt         time.Time     `json:"deployed_at"`
	LeadDuration       time.Duration `json:"lead_duration"` // Time from commit to prod
	IsFailed           bool          `json:"is_failed"`     // Did it cause an incident?
	IncidentResolvedAt *time.Time    `json:"incident_resolved_at,omitempty"`
}

// MetricDeploymentFrequency details deployment cadence.
type MetricDeploymentFrequency struct {
	TotalDeployments int             `json:"total_deployments"`
	DeploysPerDay    float64         `json:"deploys_per_day"`
	Tier             PerformanceTier `json:"tier"`
}

// MetricLeadTime details time from code commit to production.
type MetricLeadTime struct {
	AverageLeadTime time.Duration   `json:"average_lead_time"`
	MedianLeadTime  time.Duration   `json:"median_lead_time"`
	Tier            PerformanceTier `json:"tier"`
}

// MetricChangeFailureRate details percentage of deployments requiring remediation.
type MetricChangeFailureRate struct {
	TotalDeployments  int             `json:"total_deployments"`
	FailedDeployments int             `json:"failed_deployments"`
	FailureRate       float64         `json:"failure_rate"` // e.g. 0.05 = 5%
	Tier              PerformanceTier `json:"tier"`
}

// MetricTimeToRestore details mean time to recover from production failures.
type MetricTimeToRestore struct {
	IncidentCount int             `json:"incident_count"`
	AverageMTTR   time.Duration   `json:"average_mttr"`
	Tier          PerformanceTier `json:"tier"`
}

// ReviewVelocityMetrics aggregates automated code review impact.
type ReviewVelocityMetrics struct {
	TotalPRsReviewed         int           `json:"total_prs_reviewed"`
	AverageCycleTime         time.Duration `json:"average_cycle_time"`
	AverageReviewLatency     time.Duration `json:"average_review_latency"`
	DefectsCaughtBeforeMerge int           `json:"defects_caught_before_merge"`
	EngineeringHoursSaved    float64       `json:"engineering_hours_saved"` // e.g. 0.5hr per finding
}

// DORAReport provides an executive summary of team delivery performance.
type DORAReport struct {
	WorkspaceID         uuid.UUID                 `json:"workspace_id"`
	PeriodStart         time.Time                 `json:"period_start"`
	PeriodEnd           time.Time                 `json:"period_end"`
	DeploymentFrequency MetricDeploymentFrequency `json:"deployment_frequency"`
	LeadTimeForChanges  MetricLeadTime            `json:"lead_time_for_changes"`
	ChangeFailureRate   MetricChangeFailureRate   `json:"change_failure_rate"`
	TimeToRestore       MetricTimeToRestore       `json:"time_to_restore"`
	ReviewVelocity      ReviewVelocityMetrics     `json:"review_velocity"`
	OverallTier         PerformanceTier           `json:"overall_tier"`
}
