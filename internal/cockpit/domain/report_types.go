package domain

// ReportTrend describes whether a metric is improving, deteriorating, or flat.
type ReportTrend string

const (
	TrendImproved  ReportTrend = "improved"
	TrendWorsened  ReportTrend = "worsened"
	TrendUnchanged ReportTrend = "unchanged"
)

// WeeklyImplementedPoint represents one data point for 4-week trend charts.
type WeeklyImplementedPoint struct {
	WeekStart   string `json:"week_start"` // YYYY-MM-DD
	Sent        int    `json:"sent"`
	Implemented int    `json:"implemented"`
}

// FeedbackGroup captures adoption and developer sentiment for an origin group.
type FeedbackGroup struct {
	SuggestionsSent    int      `json:"suggestions_sent"`
	ImplementationRate float64  `json:"implementation_rate"` // 0..1
	ThumbsUp           int      `json:"thumbs_up"`
	ThumbsDown         int      `json:"thumbs_down"`
	NegativeRate       *float64 `json:"negative_rate"` // thumbsDown / (up + down); nil if no votes
}

// RuleGroupFeedback contrasts feedback between custom rules and general AI analysis.
type RuleGroupFeedback struct {
	TotalVotes     int           `json:"total_votes"`
	HasEnoughVotes bool          `json:"has_enough_votes"`
	DrixyRules     FeedbackGroup `json:"drixy_rules"`
	General        FeedbackGroup `json:"general"`
}

// CategoryQualityRow reports implementation and voting for a specific suggestion category.
type CategoryQualityRow struct {
	Category           string  `json:"category"`
	Sent               int     `json:"sent"`
	ImplementationRate float64 `json:"implementation_rate"` // 0..1
	ThumbsUp           int     `json:"thumbs_up"`
	ThumbsDown         int     `json:"thumbs_down"`
}

// RuleHealthState categorizes rule effectiveness.
type RuleHealthState = DrixyRuleHealthState

// RuleHealthRow captures a single rule's impact in the reporting window.
type RuleHealthRow struct {
	RuleID             string          `json:"rule_id"`
	Title              string          `json:"title"`
	Triggers           int             `json:"triggers"`
	ImplementationRate float64         `json:"implementation_rate"` // 0..1
	ThumbsUp           int             `json:"thumbs_up"`
	ThumbsDown         int             `json:"thumbs_down"`
	State              RuleHealthState `json:"state"`
}

// RepoReportSection contains review statistics and insights for one repository.
type RepoReportSection struct {
	Repository                 string                   `json:"repository"`
	Reviews                    int                      `json:"reviews"`
	ReviewsTrend               ReportTrend              `json:"reviews_trend"`
	ReviewsChangePct           float64                  `json:"reviews_change_pct"`
	SuggestionsSent            int                      `json:"suggestions_sent"`
	SuggestionsSentTrend       ReportTrend              `json:"suggestions_sent_trend"`
	SuggestionsSentChangePct   float64                  `json:"suggestions_sent_change_pct"`
	ImplementationRate         float64                  `json:"implementation_rate"` // 0..1
	ImplementationRateTrend    ReportTrend              `json:"implementation_rate_trend"`
	ImplementationRatePpChange float64                  `json:"implementation_rate_pp_change"` // percentage points
	CriticalImplemented        int                      `json:"critical_implemented"`
	CriticalSent               int                      `json:"critical_sent"`
	WeeklyImplemented          []WeeklyImplementedPoint `json:"weekly_implemented"`
	Feedback                   RuleGroupFeedback        `json:"feedback"`
	Categories                 []CategoryQualityRow     `json:"categories"`
	Rules                      []RuleHealthRow          `json:"rules"`
	RulesMore                  int                      `json:"rules_more"`
	CockpitLink                string                   `json:"cockpit_link,omitempty"`
}

// RepoReportData represents a complete repository digest email payload.
type RepoReportData struct {
	Company     string              `json:"company"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	Sections    []RepoReportSection `json:"sections"`
	CockpitLink string              `json:"cockpit_link,omitempty"`
}

// MonthlyRatePoint tracks multi-month adoption trajectory.
type MonthlyRatePoint struct {
	MonthStart string  `json:"month_start"` // YYYY-MM-01
	Label      string  `json:"label"`       // e.g. "Apr"
	Rate       float64 `json:"rate"`        // 0..1
}

// RepoRankingRow ranks repositories by review activity and suggestion uptake.
type RepoRankingRow struct {
	Rank               int     `json:"rank"`
	Repository         string  `json:"repository"`
	Reviews            int     `json:"reviews"`
	ImplementationRate float64 `json:"implementation_rate"` // 0..1
}

// ReportHighlight highlights notable milestones such as greatest adoption growth.
type ReportHighlight struct {
	Kind       string `json:"kind"` // "impl_rate_growth"
	Repository string `json:"repository"`
	Detail     string `json:"detail"`
}

// OrgReportData represents the executive-level monthly report payload.
type OrgReportData struct {
	Company                     string             `json:"company"`
	StartDate                   string             `json:"start_date"`
	EndDate                     string             `json:"end_date"`
	Reviews                     int                `json:"reviews"`
	ReviewsTrend                ReportTrend        `json:"reviews_trend"`
	ReviewsChangePct            float64            `json:"reviews_change_pct"`
	ImplementationRate          float64            `json:"implementation_rate"` // 0..1
	ImplementationRateTrend     ReportTrend        `json:"implementation_rate_trend"`
	ImplementationRatePpChange  float64            `json:"implementation_rate_pp_change"`
	SuggestionsImplemented      int                `json:"suggestions_implemented"`
	CriticalImplemented         int                `json:"critical_implemented"`
	PRCycleTimeHours            float64            `json:"pr_cycle_time_hours"`
	PRCycleTimeTrend            ReportTrend        `json:"pr_cycle_time_trend"`
	PRCycleTimeChangePct        float64            `json:"pr_cycle_time_change_pct"`
	ImplementationRateEvolution []MonthlyRatePoint `json:"implementation_rate_evolution"`
	RepoRanking                 []RepoRankingRow   `json:"repo_ranking"`
	Highlights                  []ReportHighlight  `json:"highlights"`
	RulesNeedingAttention       []RuleHealthRow    `json:"rules_needing_attention"`
	RulesNeedingAttentionMore   int                `json:"rules_needing_attention_more"`
	CockpitLink                 string             `json:"cockpit_link,omitempty"`
}
