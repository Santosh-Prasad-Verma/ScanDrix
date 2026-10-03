package domain

// CockpitRangeQuery defines time-bounded filters for cockpit queries.
type CockpitRangeQuery struct {
	OrganizationID string `json:"organization_id"`
	StartDate      string `json:"start_date"` // YYYY-MM-DD
	EndDate        string `json:"end_date"`   // YYYY-MM-DD
	Repository     string `json:"repository,omitempty"`
}

// CockpitValidation verifies data availability for an organization.
type CockpitValidation struct {
	HasData           bool `json:"has_data"`
	PullRequestsCount int  `json:"pull_requests_count"`
}

// SuggestionCategoryCount holds suggestion tallies per category.
type SuggestionCategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// RepositorySuggestions holds suggestion tallies grouped by repository.
type RepositorySuggestions struct {
	Repository string                    `json:"repository"`
	TotalCount int                       `json:"total_count"`
	Categories []SuggestionCategoryCount `json:"categories"`
}

// BugRatioRow tracks weekly bug fix pull requests against total volume.
type BugRatioRow struct {
	WeekStart string  `json:"week_start"` // YYYY-MM-DD
	TotalPRs  int     `json:"total_prs"`
	BugFixPRs int     `json:"bug_fix_prs"`
	Ratio     float64 `json:"ratio"`
}

// ComparisonDetail captures percentage variance and improvement trend.
type ComparisonDetail struct {
	PercentageChange float64 `json:"percentage_change"`
	Trend            string  `json:"trend"` // "improved" | "worsened" | "unchanged"
}

// PeriodComparison wraps current and previous period metrics with calculated trend.
type PeriodComparison[T any] struct {
	CurrentPeriod  T                `json:"current_period"`
	PreviousPeriod T                `json:"previous_period"`
	Comparison     ComparisonDetail `json:"comparison"`
}

// BugRatioData holds raw numbers for a period's bug ratio.
type BugRatioData struct {
	TotalPRs  int     `json:"total_prs"`
	BugFixPRs int     `json:"bug_fix_prs"`
	Ratio     float64 `json:"ratio"`
}

// BugRatioHighlight provides comparative bug ratios across consecutive windows.
type BugRatioHighlight = PeriodComparison[BugRatioData]

// SuggestionsImplementationRate measures adoption of automated suggestions.
type SuggestionsImplementationRate struct {
	SuggestionsSent        int     `json:"suggestions_sent"`
	SuggestionsImplemented int     `json:"suggestions_implemented"`
	ImplementationRate     float64 `json:"implementation_rate"`
}

// DeployFrequencyRow tracks weekly deployments or merged PRs.
type DeployFrequencyRow struct {
	WeekStart string `json:"week_start"`
	PRCount   int    `json:"pr_count"`
}

// DeployFrequencyData summarizes deploy volume and cadence.
type DeployFrequencyData struct {
	TotalDeployments int     `json:"total_deployments"`
	AveragePerWeek   float64 `json:"average_per_week"`
}

// DeployFrequencyHighlight compares deployment cadence across periods.
type DeployFrequencyHighlight = PeriodComparison[DeployFrequencyData]

// LeadTimeRow tracks cycle lead times.
type LeadTimeRow struct {
	WeekStart          string  `json:"week_start"`
	LeadTimeP75Minutes float64 `json:"lead_time_p75_minutes"`
	LeadTimeP75Hours   float64 `json:"lead_time_p75_hours"`
}

// LeadTimeData summarizes P75 cycle latency.
type LeadTimeData struct {
	LeadTimeP75Minutes float64 `json:"lead_time_p75_minutes"`
	LeadTimeP75Hours   float64 `json:"lead_time_p75_hours"`
}

// LeadTimeHighlight compares lead time performance between periods.
type LeadTimeHighlight = PeriodComparison[LeadTimeData]

// PullRequestsByDevRow tracks output by author.
type PullRequestsByDevRow struct {
	WeekStart string `json:"week_start"`
	Author    string `json:"author"`
	PRCount   int    `json:"pr_count"`
}

// PullRequestsOpenedVsClosedRow compares throughput and incoming backlog.
type PullRequestsOpenedVsClosedRow struct {
	WeekStart   string  `json:"week_start"`
	OpenedCount int     `json:"opened_count"`
	ClosedCount int     `json:"closed_count"`
	Ratio       float64 `json:"ratio"`
}

// DeveloperActivityRow tracks daily activity counts for heatmap views.
type DeveloperActivityRow struct {
	Developer string `json:"developer"`
	Date      string `json:"date"`
	PRCount   int    `json:"pr_count"`
}

// PRSizeData summarizes pull request line sizes.
type PRSizeData struct {
	AveragePRSize float64 `json:"average_pr_size"`
	TotalPRs      int     `json:"total_prs"`
}

// PRSizeHighlight compares pull request sizing across periods.
type PRSizeHighlight = PeriodComparison[PRSizeData]

// PullRequestSizeRow records weekly average PR changes.
type PullRequestSizeRow struct {
	WeekStart     string  `json:"week_start"`
	AveragePRSize float64 `json:"average_pr_size"`
	TotalPRs      int     `json:"total_prs"`
}

// LeadTimeBreakdownRow decomposes cycle phases into coding, pickup, and review latencies.
type LeadTimeBreakdownRow struct {
	WeekStart         string  `json:"week_start"`
	PRCount           int     `json:"pr_count"`
	CodingTimeMinutes float64 `json:"coding_time_minutes"`
	CodingTimeHours   float64 `json:"coding_time_hours"`
	PickupTimeMinutes float64 `json:"pickup_time_minutes"`
	PickupTimeHours   float64 `json:"pickup_time_hours"`
	ReviewTimeMinutes float64 `json:"review_time_minutes"`
	ReviewTimeHours   float64 `json:"review_time_hours"`
	TotalTimeMinutes  float64 `json:"total_time_minutes"`
	TotalTimeHours    float64 `json:"total_time_hours"`
}

// ImplementationRateBreakdown encapsulates sent, implemented, and rate metrics.
type ImplementationRateBreakdown struct {
	Sent        int     `json:"sent"`
	Implemented int     `json:"implemented"`
	Rate        float64 `json:"rate"`
}

// ImplementationRateWeeklyRow breaks down weekly implementation by severity.
type ImplementationRateWeeklyRow struct {
	WeekStart   string                                 `json:"week_start"`
	Sent        int                                    `json:"sent"`
	Implemented int                                    `json:"implemented"`
	Rate        float64                                `json:"rate"`
	BySeverity  map[string]ImplementationRateBreakdown `json:"by_severity"`
}

// ImplementationRateByCategoryRow breaks down implementation by suggestion category.
type ImplementationRateByCategoryRow struct {
	Category    string  `json:"category"`
	Sent        int     `json:"sent"`
	Implemented int     `json:"implemented"`
	Rate        float64 `json:"rate"`
}

// ImplementationRateBySeverityRow details implementation rate with native vs rule-driven split.
type ImplementationRateBySeverityRow struct {
	Severity          string  `json:"severity"`
	Sent              int     `json:"sent"`
	Implemented       int     `json:"implemented"`
	Rate              float64 `json:"rate"`
	NativeSent        int     `json:"native_sent"`
	NativeImplemented int     `json:"native_implemented"`
	NativeRate        float64 `json:"native_rate"`
}

// IgnoredCriticalItem represents a high/critical suggestion merged without implementation.
type IgnoredCriticalItem struct {
	SuggestionID  string  `json:"suggestion_id"`
	Repository    *string `json:"repository"`
	FilePath      *string `json:"file_path"`
	Category      *string `json:"category"`
	Summary       *string `json:"summary"`
	PullRequestID string  `json:"pull_request_id"`
	PRNumber      *int    `json:"pr_number"`
	PRClosedAt    *string `json:"pr_closed_at"`
}

// IgnoredCriticalsHighlight aggregates unimplemeted critical suggestions on merged PRs.
type IgnoredCriticalsHighlight struct {
	Count int                   `json:"count"`
	Items []IgnoredCriticalItem `json:"items"`
}

// WeakestCategory summarizes the category with the lowest adoption in a repo.
type WeakestCategory struct {
	Category string  `json:"category"`
	Rate     float64 `json:"rate"`
	Sent     int     `json:"sent"`
}

// RepositoryHealthRow benchmarks a repository's review engagement and satisfaction.
type RepositoryHealthRow struct {
	Repository             string           `json:"repository"`
	PRsReviewed            int              `json:"prs_reviewed"`
	SuggestionsSent        int              `json:"suggestions_sent"`
	SuggestionsImplemented int              `json:"suggestions_implemented"`
	ImplementationRate     float64          `json:"implementation_rate"`
	ThumbsUp               int              `json:"thumbs_up"`
	ThumbsDown             int              `json:"thumbs_down"`
	WeakestCategory        *WeakestCategory `json:"weakest_category"`
}

// DrixyRuleUsageRow aggregates rule triggers and adoption straight from review analytics.
type DrixyRuleUsageRow struct {
	RuleID          string  `json:"rule_id"`
	Triggers        int     `json:"triggers"`
	Implemented     int     `json:"implemented"`
	Rate            float64 `json:"rate"`
	ThumbsUp        int     `json:"thumbs_up"`
	ThumbsDown      int     `json:"thumbs_down"`
	LastTriggeredAt *string `json:"last_triggered_at"`
}

// ReviewQualityByRuleGroupRow compares rule-driven feedback against native AI suggestions.
type ReviewQualityByRuleGroupRow struct {
	Group       string  `json:"group"` // "drixy_rules" | "general"
	Sent        int     `json:"sent"`
	Implemented int     `json:"implemented"`
	Rate        float64 `json:"rate"`
	ThumbsUp    int     `json:"thumbs_up"`
	ThumbsDown  int     `json:"thumbs_down"`
}

// DrixyRuleHealthState categorizes rule health based on firing frequency and developer reception.
type DrixyRuleHealthState string

const (
	RuleStateHealthy DrixyRuleHealthState = "healthy"
	RuleStateNoisy   DrixyRuleHealthState = "noisy"
	RuleStateIgnored DrixyRuleHealthState = "ignored"
	RuleStateStale   DrixyRuleHealthState = "stale"
	RuleStateLowData DrixyRuleHealthState = "low_data"
)

// NegativeFeedbackByCategoryRow tracks developer reaction across issue categories.
type NegativeFeedbackByCategoryRow struct {
	Category   string `json:"category"`
	ThumbsUp   int    `json:"thumbs_up"`
	ThumbsDown int    `json:"thumbs_down"`
}

// NegativeFeedbackWeeklyRow tracks satisfaction trends week-over-week.
type NegativeFeedbackWeeklyRow struct {
	WeekStart  string `json:"week_start"`
	ThumbsUp   int    `json:"thumbs_up"`
	ThumbsDown int    `json:"thumbs_down"`
}

// NegativeVoteData captures voting volumes and negative ratio.
type NegativeVoteData struct {
	ThumbsUp     int     `json:"thumbs_up"`
	ThumbsDown   int     `json:"thumbs_down"`
	NegativeRate float64 `json:"negative_rate"`
}

// NegativeVoteRateHighlight compares negative feedback rates between periods.
type NegativeVoteRateHighlight = PeriodComparison[NegativeVoteData]

// ReviewOperationalMetricsPeriod summarizes pipeline execution counts and success ratios.
type ReviewOperationalMetricsPeriod struct {
	ProcessedPRs      int     `json:"processed_prs"`
	ProcessedReviews  int     `json:"processed_reviews"`
	SuccessfulReviews int     `json:"successful_reviews"`
	ErrorReviews      int     `json:"error_reviews"`
	SkippedReviews    int     `json:"skipped_reviews"`
	SuccessRate       float64 `json:"success_rate"`
	ErrorRate         float64 `json:"error_rate"`
	SkippedRate       float64 `json:"skipped_rate"`
}

// ReviewOperationalMetricsWeeklyRow records weekly pipeline reliability.
type ReviewOperationalMetricsWeeklyRow struct {
	ReviewOperationalMetricsPeriod
	WeekStart string `json:"week_start"`
}

// ReviewOperationalMetricComparison measures percentage variance in operational volume.
type ReviewOperationalMetricComparison struct {
	PercentageChange float64 `json:"percentage_change"`
	Trend            string  `json:"trend"`
}

// ReviewOperationalRateComparison measures percentage point shifts in operational rates.
type ReviewOperationalRateComparison struct {
	PercentageChange      float64 `json:"percentage_change"`
	Trend                 string  `json:"trend"`
	PercentagePointChange float64 `json:"percentage_point_change"`
}

// ReviewOperationalMetrics aggregates current, previous, and comparison pipeline stats.
type ReviewOperationalMetrics struct {
	CurrentPeriod  ReviewOperationalMetricsPeriod `json:"current_period"`
	PreviousPeriod ReviewOperationalMetricsPeriod `json:"previous_period"`
	Comparison     struct {
		ProcessedPRs     ReviewOperationalMetricComparison `json:"processed_prs"`
		ProcessedReviews ReviewOperationalMetricComparison `json:"processed_reviews"`
		SuccessRate      ReviewOperationalRateComparison   `json:"success_rate"`
		ErrorRate        ReviewOperationalRateComparison   `json:"error_rate"`
		SkippedRate      ReviewOperationalRateComparison   `json:"skipped_rate"`
	} `json:"comparison"`
}

// DrixyRuleHealthRow merges warehouse usage with rule configuration metadata.
type DrixyRuleHealthRow struct {
	DrixyRuleUsageRow
	Title            string               `json:"title"`
	Severity         *string              `json:"severity"`
	RepositoryID     *string              `json:"repository_id"`
	RepositoryName   *string              `json:"repository_name"`
	DirectoryID      *string              `json:"directory_id"`
	DirectoryFolders []string             `json:"directory_folders"`
	State            DrixyRuleHealthState `json:"state"`
}

// SuggestionsExplorerQuery configures faceted search across all reviewed suggestions.
type SuggestionsExplorerQuery struct {
	OrganizationID       string  `json:"organization_id"`
	StartDate            string  `json:"start_date"`
	EndDate              string  `json:"end_date"`
	Repository           *string `json:"repository,omitempty"`
	Category             *string `json:"category,omitempty"`
	Severity             *string `json:"severity,omitempty"`
	RuleID               *string `json:"rule_id,omitempty"`
	ImplementationStatus *string `json:"implementation_status,omitempty"`
	Search               *string `json:"search,omitempty"`
	Page                 int     `json:"page,omitempty"`
	PageSize             int     `json:"page_size,omitempty"`
}

// SuggestionsExplorerItem represents a single searchable suggestion with diff and context.
type SuggestionsExplorerItem struct {
	SuggestionID         string  `json:"suggestion_id"`
	Repository           *string `json:"repository"`
	RepositoryID         *string `json:"repository_id"`
	FilePath             *string `json:"file_path"`
	Category             *string `json:"category"`
	Severity             *string `json:"severity"`
	ImplementationStatus *string `json:"implementation_status"`
	Summary              *string `json:"summary"`
	ExistingCode         *string `json:"existing_code"`
	ImprovedCode         *string `json:"improved_code"`
	Language             *string `json:"language"`
	PullRequestID        string  `json:"pull_request_id"`
	PRNumber             *int    `json:"pr_number"`
	CommentID            *int64  `json:"comment_id"`
	CreatedAt            *string `json:"created_at"`
}

// SuggestionsExplorerResult wraps paginated suggestion explorer search results.
type SuggestionsExplorerResult struct {
	Total    int                       `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Items    []SuggestionsExplorerItem `json:"items"`
}

// TopDeveloper models the top PR contributor.
type TopDeveloper struct {
	Name     string `json:"name"`
	TotalPRs int    `json:"total_prs"`
}

// CompanyRanking ranks an organization against a cohort of companies.
//
// Every field is a pointer because no cross-company cohort data is collected.
// These were previously hardcoded to Rank 1 / TotalCompanies 1 /
// PercentageOfTotalPRs 100.0, where the percentage was computed against a
// denominator containing only this company's own PRs. That reads as a #1
// global ranking while being an artefact of the placeholder.
// AUDIT_REMEDIATION.md F-41.
type CompanyRanking struct {
	Rank                 *int     `json:"rank"`
	TotalCompanies       *int     `json:"total_companies"`
	PercentageOfTotalPRs *float64 `json:"percentage_of_total_prs"`
	TotalPRsAllCompanies *int     `json:"total_prs_all_companies"`
}

// CompanyDashboardMetrics aggregates headline executive KPIs.
type CompanyDashboardMetrics struct {
	TotalPRs                 int                       `json:"total_prs"`
	CriticalSuggestions      int                       `json:"critical_suggestions"`
	TotalSuggestions         int                       `json:"total_suggestions"`
	TopSuggestionsCategories []SuggestionCategoryCount `json:"top_suggestions_categories"`
	TopDeveloper             TopDeveloper              `json:"top_developer"`
	CompanyRanking           CompanyRanking            `json:"company_ranking"`

	// Unavailable names absent metrics as "<metric>:<reason>".
	// AUDIT_REMEDIATION.md F-41.
	Unavailable []string `json:"unavailable,omitempty"`
}

// CompanyDashboardAdditionalMetrics provides supplementary engineering health signals.
type CompanyDashboardAdditionalMetrics struct {
	SuggestionsAppliedPercentage *float64                  `json:"suggestions_applied_percentage,omitempty"`
	SuggestionsImplementedCount  *int                      `json:"suggestions_implemented_count,omitempty"`
	CycleTime                    *LeadTimeHighlight        `json:"cycle_time,omitempty"`
	DeployFrequency              *DeployFrequencyHighlight `json:"deploy_frequency,omitempty"`
	BugRatio                     *BugRatioHighlight        `json:"bug_ratio,omitempty"`
	LeadTimeBreakdown            []LeadTimeBreakdownRow    `json:"lead_time_breakdown,omitempty"`
}

// CompanyDashboard encapsulates the executive overview for engineering leadership.
type CompanyDashboard struct {
	OrganizationID    string                              `json:"organization_id"`
	Period            struct{ StartDate, EndDate string } `json:"period"`
	Metrics           CompanyDashboardMetrics             `json:"metrics"`
	AdditionalMetrics CompanyDashboardAdditionalMetrics   `json:"additional_metrics"`
}
