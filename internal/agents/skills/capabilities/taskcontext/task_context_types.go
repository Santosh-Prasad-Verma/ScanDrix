package taskcontext

// ResolutionMode configures how task context is retrieved.
type ResolutionMode string

const (
	ResolutionModeCacheFirst ResolutionMode = "cache_first"
	ResolutionModeAgentFirst ResolutionMode = "agent_first"
)

// TaskContextSignalHints provides hints mined from outside sources.
type TaskContextSignalHints struct {
	TicketKeys          []string `json:"ticketKeys,omitempty"`
	TaskLinks           []string `json:"taskLinks,omitempty"`
	RequirementKeywords []string `json:"requirementKeywords,omitempty"`
}

// TaskContextReadParams parameters passed to task-context read capability.
type TaskContextReadParams struct {
	SkillName                 string                  `json:"skillName"`
	OrganizationID            string                  `json:"organizationId"`
	TeamID                    string                  `json:"teamId"`
	RepositoryOwner           string                  `json:"repositoryOwner,omitempty"`
	RepositoryName            string                  `json:"repositoryName,omitempty"`
	PullRequestNumber         int                     `json:"pullRequestNumber,omitempty"`
	PRBody                    string                  `json:"prBody,omitempty"`
	HeadRef                   string                  `json:"headRef,omitempty"`
	UserQuestion              string                  `json:"userQuestion,omitempty"`
	PullRequestDescription    string                  `json:"pullRequestDescription,omitempty"`
	TaskContext               string                  `json:"taskContext,omitempty"`
	TaskID                    string                  `json:"taskId,omitempty"`
	TaskURL                   string                  `json:"taskUrl,omitempty"`
	TaskReference             string                  `json:"taskReference,omitempty"`
	UserLanguage              string                  `json:"userLanguage,omitempty"`
	ExcludedTools             []string                `json:"excludedTools,omitempty"`
	TaskContextResolutionMode ResolutionMode          `json:"taskContextResolutionMode,omitempty"`
	EnableAgenticFallback     bool                    `json:"enableAgenticFallback,omitempty"`
	BusinessSignals           *TaskContextSignalHints `json:"businessSignals,omitempty"`
}

// TaskContextHints contains reference signals mined from PR/task text.
type TaskContextHints struct {
	IssueKeys          []string `json:"issueKeys"`
	IssueNumbers       []int    `json:"issueNumbers"`
	IssueLinks         []string `json:"issueLinks"`
	ExplicitIssueKeys   []string `json:"explicitIssueKeys"`
	ExplicitIssueLinks  []string `json:"explicitIssueLinks"`
	QueryText          string   `json:"queryText"`
	URLHosts           []string `json:"urlHosts"`
	SiteURLs           []string `json:"siteUrls"`
	SiteIDs            []string `json:"siteIds"`
	ResourceIDs        []string `json:"resourceIds"`
}

// TaskContextToolSignature represents an MCP tool input schema normalized for matching.
type TaskContextToolSignature struct {
	RequiredParams       []string                          `json:"requiredParams"`
	Properties           map[string]map[string]any         `json:"properties"`
	NormalizedProperties map[string]map[string]any         `json:"normalizedProperties"`
}

// TaskContextNormalized is the standard output of task context reading.
type TaskContextNormalized struct {
	ID                 string   `json:"id,omitempty"`
	Title              string   `json:"title,omitempty"`
	Description        string   `json:"description,omitempty"`
	AcceptanceCriteria []string `json:"acceptanceCriteria,omitempty"`
	Links              []string `json:"links,omitempty"`
	RawPayload         any      `json:"rawPayload,omitempty"`
}
