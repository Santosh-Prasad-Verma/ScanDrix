package domain

// ReviewLabel represents a categorization label applied during code review.
type ReviewLabel struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CodeReviewVersion denotes the engine review label schema version.
type CodeReviewVersion string

const (
	CodeReviewVersionLegacy  CodeReviewVersion = "legacy"
	CodeReviewVersionV2      CodeReviewVersion = "v2"
	CodeReviewVersionV3Agent CodeReviewVersion = "v3_agent"
)

var (
	ReviewLabelsV3 = []ReviewLabel{
		{
			Type:        "bug",
			Name:        "Bug",
			Description: "Logic errors, race conditions, null dereferences, edge cases, and incorrect behavior.",
		},
		{
			Type:        "security",
			Name:        "Security",
			Description: "Vulnerabilities, injection, auth issues, data exposure, and secrets.",
		},
		{
			Type:        "performance",
			Name:        "Performance",
			Description: "N+1 queries, hot path allocations, missing caching, and blocking I/O.",
		},
		{
			Type:        "business_logic",
			Name:        "Business Logic",
			Description: "Validates the implementation against business rules, requirements, and acceptance criteria linked in the PR.",
		},
	}

	ReviewLabelsV2 = []ReviewLabel{
		{
			Type:        "bug",
			Name:        "Bug",
			Description: "Fixing actual bugs or defects in the code.",
		},
		{
			Type:        "performance",
			Name:        "Performance",
			Description: "Improvements in code performance and efficiency.",
		},
		{
			Type:        "security",
			Name:        "Security",
			Description: "Fixing vulnerabilities or improving code security.",
		},
		{
			Type:        "business_logic",
			Name:        "Business Logic",
			Description: "Validates the implementation against business rules, requirements, and acceptance criteria linked in the PR.",
		},
	}

	ReviewLabelsLegacy = []ReviewLabel{
		{
			Type:        "performance_and_optimization",
			Name:        "Performance and Optimization",
			Description: "Improvements in code efficiency, performance, and resource usage.",
		},
		{
			Type:        "security",
			Name:        "Security",
			Description: "Fixing vulnerabilities or improving code security.",
		},
		{
			Type:        "error_handling",
			Name:        "Error Handling",
			Description: "Enhancements in how errors and exceptions are handled.",
		},
		{
			Type:        "refactoring",
			Name:        "Refactoring",
			Description: "Restructuring code for better readability, maintainability, or modularity.",
		},
		{
			Type:        "maintainability",
			Name:        "Maintainability",
			Description: "Facilitating future maintenance and extension of the code.",
		},
		{
			Type:        "potential_issues",
			Name:        "Potential Issues",
			Description: "Fixing potential bugs or logical errors.",
		},
		{
			Type:        "code_style",
			Name:        "Code Style",
			Description: "Improving consistency and adherence to coding standards.",
		},
		{
			Type:        "documentation_and_comments",
			Name:        "Documentation and Comments",
			Description: "Enhancing documentation and clarity of comments.",
		},
		{
			Type:        "drixy_rules",
			Name:        "Drixy Rules",
			Description: "Suggestions that enforce the rules defined in the Drixy configuration.",
		},
		{
			Type:        "breaking_changes",
			Name:        "Breaking Changes",
			Description: "Changes that break the existing code.",
		},
		{
			Type:        "cross_file",
			Name:        "Cross File",
			Description: "Identifies fails between changed files in the PR.",
		},
	}
)

// GetReviewLabels returns the review label set corresponding to the requested review version.
func GetReviewLabels(version CodeReviewVersion) []ReviewLabel {
	switch version {
	case CodeReviewVersionV3Agent:
		res := make([]ReviewLabel, len(ReviewLabelsV3))
		copy(res, ReviewLabelsV3)
		return res
	case CodeReviewVersionV2:
		res := make([]ReviewLabel, len(ReviewLabelsV2))
		copy(res, ReviewLabelsV2)
		return res
	default:
		res := make([]ReviewLabel, len(ReviewLabelsLegacy))
		copy(res, ReviewLabelsLegacy)
		return res
	}
}
