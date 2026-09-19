package domain

// IssueStatus tracks the life-cycle of a detected issue across PRs and branches.
type IssueStatus string

const (
	StatusOpen       IssueStatus = "open"
	StatusInProgress IssueStatus = "in_progress"
	StatusResolved   IssueStatus = "resolved"
	StatusDismissed  IssueStatus = "dismissed"
)

// SeverityLevel ranks issue criticality.
type SeverityLevel string

const (
	SeverityCritical SeverityLevel = "critical"
	SeverityHigh     SeverityLevel = "high"
	SeverityMedium   SeverityLevel = "medium"
	SeverityLow      SeverityLevel = "low"
)

// SeverityOrder defines descending severity hierarchy for filtering.
var SeverityOrder = []SeverityLevel{
	SeverityCritical,
	SeverityHigh,
	SeverityMedium,
	SeverityLow,
}

// IssueLabel categorizes issues by area.
type IssueLabel string

const (
	LabelBug                      IssueLabel = "bug"
	LabelSecurity                 IssueLabel = "security"
	LabelPerformance              IssueLabel = "performance"
	LabelBusinessLogic            IssueLabel = "business_logic"
	LabelDrixyRules               IssueLabel = "drixy_rules"
	LabelCodeStyle                IssueLabel = "code_style"
	LabelRefactoring              IssueLabel = "refactoring"
	LabelMaintainability          IssueLabel = "maintainability"
	LabelPotentialIssues          IssueLabel = "potential_issues"
	LabelDocumentationAndComments IssueLabel = "documentation_and_comments"
	LabelErrorHandling            IssueLabel = "error_handling"
	LabelBreakingChanges          IssueLabel = "breaking_changes"
	LabelCrossFile                IssueLabel = "cross_file"
)

// IssuePropertyField denotes a mutable property on an issue.
type IssuePropertyField string

const (
	FieldSeverity IssuePropertyField = "severity"
	FieldLabel    IssuePropertyField = "label"
	FieldStatus   IssuePropertyField = "status"
)

// IssueTrackerType identifies external issue tracking providers.
type IssueTrackerType string

const (
	TrackerJira   IssueTrackerType = "jira"
	TrackerLinear IssueTrackerType = "linear"
	TrackerGitHub IssueTrackerType = "github"
)

// PlatformType identifies the git forge provider.
type PlatformType string

const (
	PlatformGitHub     PlatformType = "github"
	PlatformGitLab     PlatformType = "gitlab"
	PlatformAzureRepos PlatformType = "azure_repos"
	PlatformBitbucket  PlatformType = "bitbucket"
	PlatformForgejo    PlatformType = "forgejo"
)

// PriorityStatus identifies filtering and tuning verdicts.
type PriorityStatus string

const (
	PriorityDiscardedBySafeguard       PriorityStatus = "DISCARDED_BY_SAFEGUARD"
	PriorityDiscardedByDrixyFineTuning PriorityStatus = "DISCARDED_BY_DRIXY_FINE_TUNING"
	PriorityDiscardedByCodeDiff        PriorityStatus = "DISCARDED_BY_CODE_DIFF"
)

// ImplementationStatus reflects whether a suggestion was already implemented in code.
type ImplementationStatus string

const (
	ImplementationNotImplemented ImplementationStatus = "not_implemented"
	ImplementationImplemented    ImplementationStatus = "implemented"
)
