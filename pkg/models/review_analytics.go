package models

import (
	"github.com/google/uuid"
	"time"
)

type ReviewAnalyticsFilter struct {
	RepositoryID *uuid.UUID
	Start        time.Time
	End          time.Time
}

type ReviewAnalyticsBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type SuggestionSearchFilter struct {
	ReviewAnalyticsFilter
	Search   string
	Severity string
	Category string
	Page     int
	Limit    int
}

type SuggestionSearchItem struct {
	CodeFinding
	Repository string `json:"repository"`
	PullNumber int    `json:"pullNumber"`
	PullTitle  string `json:"pullTitle"`
}

type SuggestionSearchResult struct {
	Items []SuggestionSearchItem `json:"items"`
	Total int                    `json:"total"`
	Page  int                    `json:"page"`
	Limit int                    `json:"limit"`
}

// ReviewAnalytics counts executions, not unique PRs or deployments. The window
// is half-open [start, end), using review creation time in UTC.
type ReviewAnalytics struct {
	Start                time.Time               `json:"start"`
	End                  time.Time               `json:"end"`
	RepositoryID         *uuid.UUID              `json:"repositoryId"`
	Total                int                     `json:"total"`
	Completed            int                     `json:"completed"`
	Failed               int                     `json:"failed"`
	Active               int                     `json:"active"`
	MeanMinutes          *float64                `json:"meanMinutes"`
	Findings             int                     `json:"findings"`
	Weekly               []ReviewAnalyticsBucket `json:"weekly"`
	Severity             []ReviewAnalyticsBucket `json:"severity"`
	Categories           []ReviewAnalyticsBucket `json:"categories"`
	Repositories         []ReviewAnalyticsBucket `json:"repositories"`
	ImplementationRate   *float64                `json:"implementationRate"`
	NegativeFeedbackRate *float64                `json:"negativeFeedbackRate"`
	Unavailable          []AnalyticsUnavailable  `json:"unavailable"`
}
