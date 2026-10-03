package models

// UnavailableMetricReason explains why a metric is absent from a response.
//
// A metric that cannot be computed is reported as null and named here, rather
// than being filled with a plausible literal. A zero would be indistinguishable
// from a real zero, and a constant would be indistinguishable from a
// measurement, so neither is acceptable.
type UnavailableMetricReason string

const (
	// ReasonNoDataSource means the underlying events are not collected yet.
	ReasonNoDataSource UnavailableMetricReason = "no_data_source"
	// ReasonNoFormula means a product definition of the metric is required
	// before it can be computed.
	ReasonNoFormula UnavailableMetricReason = "no_defined_formula"
	// ReasonInsufficientData means the query ran but the sample is too small or
	// the required fields are all null.
	ReasonInsufficientData UnavailableMetricReason = "insufficient_data"
)

// AnalyticsUnavailable names a metric that is not in the response and why.
type AnalyticsUnavailable struct {
	Metric string                   `json:"metric"`
	Reason UnavailableMetricReason  `json:"reason"`
	Detail string                   `json:"detail,omitempty"`
}

// CockpitOverview is the workspace activity summary.
//
// Every field is either computed from a real query or listed in Unavailable.
// Nil pointers mean "not computable", never "zero".
type CockpitOverview struct {
	WorkspaceID string `json:"workspaceId"`

	ActiveReviews      int  `json:"activeReviews"`
	ActiveRepositories int  `json:"activeRepositories"`
	CompletedReviews   int  `json:"completedReviews"`
	FailedReviews      int  `json:"failedReviews"`

	MeanTimeToReviewMin *float64 `json:"meanTimeToReviewMin"`
	PassRatePercentage  *float64 `json:"passRatePercentage"`

	// securityScore, developerHoursSaved, passRate and the DORA series need
	// either a product formula or data that is not collected, so they are
	// reported through Unavailable instead of being invented here.
	SecurityScore       *float64 `json:"securityScore"`
	DeveloperHoursSaved *float64 `json:"developerHoursSaved"`

	Unavailable []AnalyticsUnavailable `json:"unavailable"`
}

// DoraMetrics are the four DORA indicators.
//
// Deployment frequency, lead time, change failure rate and time to restore are
// derived from deployment events. No deployment event source exists in the
// schema, so every field is nil and each is named in Unavailable with
// ReasonNoDataSource. This is the honest representation: reporting a rating such
// as ELITE without a single deployment record would be fabrication.
type DoraMetrics struct {
	DeploymentFrequency *string `json:"deploymentFrequency"`
	LeadTimeForChanges  *string `json:"leadTimeForChanges"`
	ChangeFailureRate   *float64 `json:"changeFailureRate"`
	TimeToRestore       *string `json:"timeToRestore"`
	Rating              *string `json:"rating"`

	Unavailable []AnalyticsUnavailable `json:"unavailable"`
}

// ProductivityMetrics are review throughput and turnaround measures.
type ProductivityMetrics struct {
	CycleTimeHours       *float64 `json:"cycleTimeHours"`
	ReviewTurnaroundMin  *float64 `json:"reviewTurnaroundMin"`
	ThroughputPerWeek    *int     `json:"throughputPerWeek"`
	PrsReviewedByScanDrix *int    `json:"prsReviewedByScanDrix"`

	Unavailable []AnalyticsUnavailable `json:"unavailable"`
}

// CodeHealthMetrics summarises structural debt.
//
// The health score is a composite over cyclomatic complexity, which is not
// recorded anywhere: code_ast_nodes carries kinds and symbols but no complexity
// measure. It is therefore reported as unavailable rather than approximated.
type CodeHealthMetrics struct {
	WorkspaceID         string `json:"workspaceId"`
	HealthScore         *float64 `json:"healthScore"`
	CriticalDebtFiles   *int     `json:"criticalDebtFiles"`
	ComplexityHotspots  *int     `json:"complexityHotspots"`
	TotalFindings       int      `json:"totalFindings"`
	OpenFindings        int      `json:"openFindings"`
	Trend               *string  `json:"trend"`
	EvaluatedAt         string   `json:"evaluatedAt"`

	Unavailable []AnalyticsUnavailable `json:"unavailable"`
}

// CodeHotspotFile is one entry of the structural hotspot list.
//
// Populated from real finding counts per file. Complexity score and risk tier
// are omitted rather than invented, because no complexity measurement exists.
type CodeHotspotFile struct {
	FilePath      string  `json:"filePath"`
	FindingsCount int     `json:"findingsCount"`
	CriticalCount int     `json:"criticalCount"`
	ComplexityScore *float64 `json:"complexityScore"`
	RiskTier      *string `json:"riskTier"`
}
