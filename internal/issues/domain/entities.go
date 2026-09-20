package domain

import (
	"time"
)

// UserRef identifies a git author or platform user.
type UserRef struct {
	GitID    string `json:"gitId"`
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
}

// ContributingSuggestion ties a code review suggestion to an issue.
type ContributingSuggestion struct {
	ID                 string   `json:"id"`
	PRNumber           int      `json:"prNumber"`
	PRAuthor           UserRef  `json:"prAuthor"`
	SuggestionContent  string   `json:"suggestionContent,omitempty"`
	OneSentenceSummary string   `json:"oneSentenceSummary,omitempty"`
	RelevantFile       string   `json:"relevantFile,omitempty"`
	Language           string   `json:"language,omitempty"`
	ExistingCode       string   `json:"existingCode,omitempty"`
	ImprovedCode       string   `json:"improvedCode,omitempty"`
	StartLine          int      `json:"startLine,omitempty"`
	EndLine            int      `json:"endLine,omitempty"`
	BrokenDrixyRulesIDs []string `json:"brokenDrixyRulesIds,omitempty"`
}

// RepositoryToIssues describes the repository owning the issue.
type RepositoryToIssues struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	FullName string       `json:"full_name"`
	Platform PlatformType `json:"platform"`
	URL      string       `json:"url,omitempty"`
	HTTPURL  string       `json:"http_url,omitempty"`
}

// Issue represents an enterprise tracked code finding persistent across commits.
type Issue struct {
	UUID                    string                   `json:"uuid,omitempty"`
	Title                   string                   `json:"title"`
	Description             string                   `json:"description"`
	FilePath                string                   `json:"filePath"`
	Language                string                   `json:"language"`
	Label                   string                   `json:"label"`
	Severity                SeverityLevel            `json:"severity"`
	Status                  IssueStatus              `json:"status"`
	ContributingSuggestions []ContributingSuggestion `json:"contributingSuggestions,omitempty"`
	Repository              RepositoryToIssues       `json:"repository"`
	OrganizationID          string                   `json:"organizationId"`
	Age                     string                   `json:"age,omitempty"`
	CreatedAt               time.Time                `json:"createdAt"`
	UpdatedAt               time.Time                `json:"updatedAt"`
	PRNumbers               []string                 `json:"prNumbers,omitempty"`
	Owner                   *UserRef                 `json:"owner,omitempty"`
	Reporter                *UserRef                 `json:"reporter,omitempty"`
}

// SourceFilters defines which review sources can feed auto-generated issues.
type SourceFilters struct {
	IncludeDrixyRules       bool `json:"includeDrixyRules"`
	IncludeCodeReviewEngine bool `json:"includeCodeReviewEngine"`
}

// SeverityFilters controls which severities trigger issue creation.
type SeverityFilters struct {
	MinimumSeverity   SeverityLevel   `json:"minimumSeverity,omitempty"`
	AllowedSeverities []SeverityLevel `json:"allowedSeverities,omitempty"`
}

// IssueCreationConfig configures automated issue generation for an organization or team.
type IssueCreationConfig struct {
	AutomaticCreationEnabled bool            `json:"automaticCreationEnabled"`
	SourceFilters            SourceFilters   `json:"sourceFilters"`
	SeverityFilters          SeverityFilters `json:"severityFilters"`
	OrganizationID           string          `json:"organizationId"`
	TeamID                   string          `json:"teamId,omitempty"`
}

// LinkInfo provides a labeled navigation URL.
type LinkInfo struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ReactionStats counts thumbs up/down reactions across linked suggestions.
type ReactionStats struct {
	ThumbsUp   int `json:"thumbsUp"`
	ThumbsDown int `json:"thumbsDown"`
}

// RepoShortInfo provides a brief repository reference.
type RepoShortInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IssueDetails provides full detail views with resolved URLs and reaction counts.
type IssueDetails struct {
	ID                      string                   `json:"id"`
	Title                   string                   `json:"title"`
	Description             string                   `json:"description"`
	Age                     string                   `json:"age"`
	Label                   string                   `json:"label"`
	Severity                SeverityLevel            `json:"severity"`
	Status                  IssueStatus              `json:"status"`
	ContributingSuggestions []ContributingSuggestion `json:"contributingSuggestions"`
	FileLink                LinkInfo                 `json:"fileLink"`
	PRLinks                 []LinkInfo               `json:"prLinks"`
	RepositoryLink          LinkInfo                 `json:"repositoryLink"`
	Language                string                   `json:"language"`
	Reactions               ReactionStats            `json:"reactions"`
	GitOrganizationName     string                   `json:"gitOrganizationName"`
	Repository              RepoShortInfo            `json:"repository"`
}

// RepresentativeSuggestion provides normalized context for AI deduplication and merging.
type RepresentativeSuggestion struct {
	ID                 string `json:"id"`
	Language           string `json:"language"`
	RelevantFile       string `json:"relevantFile"`
	SuggestionContent  string `json:"suggestionContent"`
	ExistingCode       string `json:"existingCode"`
	ImprovedCode       string `json:"improvedCode"`
	OneSentenceSummary string `json:"oneSentenceSummary"`
}

// SuggestionItem holds suggestion payload details within a pull request file.
type SuggestionItem struct {
	ID                   string               `json:"id"`
	OneSentenceSummary   string               `json:"oneSentenceSummary"`
	SuggestionContent    string               `json:"suggestionContent"`
	RelevantFile         string               `json:"relevantFile"`
	Language             string               `json:"language"`
	Label                string               `json:"label"`
	Severity             SeverityLevel        `json:"severity"`
	ExistingCode         string               `json:"existingCode"`
	ImprovedCode         string               `json:"improvedCode"`
	StartLine            int                  `json:"startLine"`
	EndLine              int                  `json:"endLine"`
	BrokenDrixyRulesIDs  []string             `json:"brokenDrixyRulesIds,omitempty"`
	ImplementationStatus ImplementationStatus `json:"implementationStatus"`
	PriorityStatus       PriorityStatus       `json:"priorityStatus"`
	DeliveryStatus       string               `json:"deliveryStatus"`
}

// PRFileInfo contains changed file information and associated suggestions.
type PRFileInfo struct {
	Path        string           `json:"path"`
	Status      string           `json:"status"` // "added", "modified", "removed"
	FileContent string           `json:"fileContent,omitempty"`
	Suggestions []SuggestionItem `json:"suggestions,omitempty"`
}

// PullRequestInfo describes the pull request triggering issue generation.
type PullRequestInfo struct {
	Number     int           `json:"number"`
	Title      string        `json:"title,omitempty"`
	Status     string        `json:"status,omitempty"`
	User       UserRef       `json:"user"`
	Repository RepoShortInfo `json:"repository,omitempty"`
}

// OrganizationAndTeamData holds organizational scoping IDs.
type OrganizationAndTeamData struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId,omitempty"`
}

// ContextToGenerateIssues groups all parameters required to generate issues on PR close.
type ContextToGenerateIssues struct {
	OrganizationAndTeamData OrganizationAndTeamData `json:"organizationAndTeamData"`
	Repository              RepositoryToIssues      `json:"repository"`
	PullRequest             PullRequestInfo         `json:"pullRequest"`
	PRFiles                 []PRFileInfo            `json:"prFiles,omitempty"`
}

// ExternalIssueRef links a ScanDrix issue to an external issue tracker item.
type ExternalIssueRef struct {
	TrackerType IssueTrackerType `json:"trackerType"`
	ExternalID  string           `json:"externalId"`
	ExternalKey string           `json:"externalKey"`
	ExternalURL string           `json:"externalUrl"`
	Status      string           `json:"status"`
}

// GetIssuesFilter specifies parameters for listing issues.
type GetIssuesFilter struct {
	OrganizationID string     `json:"organizationId"`
	Title          string     `json:"title,omitempty"`
	Severity       string     `json:"severity,omitempty"`
	Category       string     `json:"category,omitempty"`
	FilePath       string     `json:"filePath,omitempty"`
	Status         string     `json:"status,omitempty"`
	RepositoryName string     `json:"repositoryName,omitempty"`
	RepositoryIDs  []string   `json:"repositoryIds,omitempty"`
	BeforeAt       *time.Time `json:"beforeAt,omitempty"`
	AfterAt        *time.Time `json:"afterAt,omitempty"`
}
