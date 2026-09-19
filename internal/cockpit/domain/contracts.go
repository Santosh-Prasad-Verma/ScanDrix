package domain

import (
	"context"
)

// RuleMeta holds rule title and severity metadata for reports.
type RuleMeta struct {
	Title    string  `json:"title"`
	Severity *string `json:"severity"`
}

// ReportRecipient represents an individual report recipient.
type ReportRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// RepoAdminRecipient represents a repository administrator with their authorized repositories.
type RepoAdminRecipient struct {
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	Repositories []string `json:"repositories"`
}

// CockpitReviewAnalyticsService defines review efficacy and adoption metrics operations.
type CockpitReviewAnalyticsService interface {
	GetImplementationRateWeekly(ctx context.Context, q CockpitRangeQuery) ([]ImplementationRateWeeklyRow, error)
	GetImplementationRateByCategory(ctx context.Context, q CockpitRangeQuery) ([]ImplementationRateByCategoryRow, error)
	GetImplementationRateBySeverity(ctx context.Context, q CockpitRangeQuery) ([]ImplementationRateBySeverityRow, error)
	GetIgnoredCriticals(ctx context.Context, q CockpitRangeQuery) (*IgnoredCriticalsHighlight, error)
	GetRepositoriesHealth(ctx context.Context, q CockpitRangeQuery) ([]RepositoryHealthRow, error)
	GetNegativeFeedbackByCategory(ctx context.Context, q CockpitRangeQuery) ([]NegativeFeedbackByCategoryRow, error)
	GetNegativeFeedbackWeekly(ctx context.Context, q CockpitRangeQuery) ([]NegativeFeedbackWeeklyRow, error)
	GetNegativeVoteRateHighlight(ctx context.Context, q CockpitRangeQuery) (*NegativeVoteRateHighlight, error)
	GetReviewOperationalMetrics(ctx context.Context, q CockpitRangeQuery) (*ReviewOperationalMetrics, error)
	GetReviewOperationalMetricsWeekly(ctx context.Context, q CockpitRangeQuery) ([]ReviewOperationalMetricsWeeklyRow, error)
	GetDrixyRulesUsage(ctx context.Context, q CockpitRangeQuery) ([]DrixyRuleUsageRow, error)
	GetReviewQualityByRuleGroup(ctx context.Context, q CockpitRangeQuery) ([]ReviewQualityByRuleGroupRow, error)
	GetRepositoryNames(ctx context.Context, organizationID string) (map[string]string, error)
	SearchSuggestions(ctx context.Context, q SuggestionsExplorerQuery) (*SuggestionsExplorerResult, error)
}

// CockpitDeveloperProductivityService computes cycle times, sizing, and throughput metrics.
type CockpitDeveloperProductivityService interface {
	GetDeployFrequencyChart(ctx context.Context, q CockpitRangeQuery) ([]DeployFrequencyRow, error)
	GetDeployFrequencyHighlight(ctx context.Context, q CockpitRangeQuery) (*DeployFrequencyHighlight, error)
	GetLeadTimeChart(ctx context.Context, q CockpitRangeQuery) ([]LeadTimeRow, error)
	GetLeadTimeHighlight(ctx context.Context, q CockpitRangeQuery) (*LeadTimeHighlight, error)
	GetPullRequestsByDev(ctx context.Context, q CockpitRangeQuery) ([]PullRequestsByDevRow, error)
	GetPullRequestSizeHighlight(ctx context.Context, q CockpitRangeQuery) (*PRSizeHighlight, error)
	GetPullRequestSizeChart(ctx context.Context, q CockpitRangeQuery) ([]PullRequestSizeRow, error)
	GetLeadTimeBreakdown(ctx context.Context, q CockpitRangeQuery) ([]LeadTimeBreakdownRow, error)
	GetPullRequestsOpenedVsClosed(ctx context.Context, q CockpitRangeQuery) ([]PullRequestsOpenedVsClosedRow, error)
	GetDeveloperActivity(ctx context.Context, q CockpitRangeQuery) ([]DeveloperActivityRow, error)
	GetCompanyDashboard(ctx context.Context, q CockpitRangeQuery) (*CompanyDashboard, error)
}

// CockpitCodeHealthService computes quality debt, suggestion adoption, and bug ratios.
type CockpitCodeHealthService interface {
	GetSuggestionsByCategory(ctx context.Context, q CockpitRangeQuery) ([]SuggestionCategoryCount, error)
	GetSuggestionsByRepository(ctx context.Context, q CockpitRangeQuery) ([]RepositorySuggestions, error)
	GetBugRatioChart(ctx context.Context, q CockpitRangeQuery) ([]BugRatioRow, error)
	GetBugRatioHighlight(ctx context.Context, q CockpitRangeQuery) (*BugRatioHighlight, error)
	GetImplementationRate(ctx context.Context, q CockpitRangeQuery) (*SuggestionsImplementationRate, error)
}

// CockpitReportsService constructs periodic executive and repository digests.
type CockpitReportsService interface {
	BuildRepoSection(ctx context.Context, orgID, repository, startDate, endDate string, ruleTitles map[string]RuleMeta) (*RepoReportSection, error)
	BuildRepoSections(ctx context.Context, orgID string, repositories []string, startDate, endDate string) ([]RepoReportSection, error)
	BuildOrgReport(ctx context.Context, orgID, company, startDate, endDate string) (*OrgReportData, error)
}

// ReportRecipientsService queries the identity layer for authorized report recipients.
type ReportRecipientsService interface {
	GetOwners(ctx context.Context, organizationID string) ([]ReportRecipient, error)
	GetRepoAdmins(ctx context.Context, organizationID string) ([]RepoAdminRecipient, error)
}
