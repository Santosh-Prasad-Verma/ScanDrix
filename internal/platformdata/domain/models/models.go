// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: models
// File: models.go
// ═══════════════════════════════════════════════════════════════

package models

import (
	"time"

	"github.com/scandrix/backend/internal/platformdata/domain/enums"
)

// RepositoryInfo encapsulates SCM repository metadata.
type RepositoryInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	FullName  string    `json:"fullName"`
	Language  string    `json:"language"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// PullRequestUser represents an author, reviewer, or assignee on a pull request.
type PullRequestUser struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username"`
}

// CommitAuthor contains author credentials and commit timestamp.
type CommitAuthor struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Date     string `json:"date"`
}

// Commit details an individual git revision included in the pull request.
type Commit struct {
	Author    CommitAuthor `json:"author"`
	SHA       string       `json:"sha"`
	Message   string       `json:"message"`
	CreatedAt string       `json:"createdAt,omitempty"`
}

// ClusteringInformation describes semantic group relationships among suggestions.
type ClusteringInformation struct {
	Type                  string   `json:"type,omitempty"`
	RelatedSuggestionsIDs []string `json:"relatedSuggestionsIds,omitempty"`
	ParentSuggestionID    string   `json:"parentSuggestionId,omitempty"`
	ProblemDescription    string   `json:"problemDescription,omitempty"`
	ActionStatement       string   `json:"actionStatement,omitempty"`
}

// CommentReference links a suggestion to an posted review comment on the SCM platform.
type CommentReference struct {
	ID                  int64 `json:"id"`
	PullRequestReviewID int64 `json:"pullRequestReviewId,omitempty"`
}

// Suggestion represents an actionable code improvement, bug fix, or security finding.
type Suggestion struct {
	ID                    string                 `json:"id"`
	RelevantFile          string                 `json:"relevantFile"`
	Language              string                 `json:"language"`
	SuggestionContent     string                 `json:"suggestionContent"`
	ExistingCode          string                 `json:"existingCode"`
	ImprovedCode          string                 `json:"improvedCode"`
	OneSentenceSummary    string                 `json:"oneSentenceSummary"`
	RelevantLinesStart    int                    `json:"relevantLinesStart"`
	RelevantLinesEnd      int                    `json:"relevantLinesEnd"`
	Label                 string                 `json:"label"`
	Severity              string                 `json:"severity"`
	RankScore             float64                `json:"rankScore,omitempty"`
	BrokenDrixyRulesIDs   []string               `json:"brokenDrixyRulesIds,omitempty"`
	ClusteringInformation *ClusteringInformation `json:"clusteringInformation,omitempty"`
	PriorityStatus        enums.PriorityStatus   `json:"priorityStatus"`
	DeliveryStatus        enums.DeliveryStatus   `json:"deliveryStatus"`
	ImplementationStatus  enums.ImplementationStatus `json:"implementationStatus,omitempty"`
	Comment               *CommentReference      `json:"comment,omitempty"`
	Type                  string                 `json:"type,omitempty"`
	CreatedAt             string                 `json:"createdAt,omitempty"`
	UpdatedAt             string                 `json:"updatedAt,omitempty"`
	PRNumber              int                    `json:"prNumber,omitempty"`
	PRTitle               string                 `json:"prTitle,omitempty"`
	PRURL                 string                 `json:"prUrl,omitempty"`
	RepositoryID          string                 `json:"repositoryId,omitempty"`
	RepositoryFullName    string                 `json:"repositoryFullName,omitempty"`
}

// CodeReviewModelUsed captures the specific LLM models utilized during evaluation.
type CodeReviewModelUsed struct {
	GenerateSuggestions string `json:"generateSuggestions,omitempty"`
	Safeguard           string `json:"safeguard,omitempty"`
}

// File tracks changes and suggestions for a single file in a pull request.
type File struct {
	ID                  string               `json:"id"`
	SHA                 string               `json:"sha,omitempty"`
	Path                string               `json:"path"`
	Filename            string               `json:"filename"`
	PreviousName        string               `json:"previousName,omitempty"`
	Status              string               `json:"status"` // "added", "modified", "removed"
	CreatedAt           string               `json:"createdAt,omitempty"`
	UpdatedAt           string               `json:"updatedAt,omitempty"`
	Suggestions         []Suggestion         `json:"suggestions"`
	Added               int                  `json:"added,omitempty"`
	Deleted             int                  `json:"deleted,omitempty"`
	Changes             int                  `json:"changes,omitempty"`
	ReviewMode          string               `json:"reviewMode,omitempty"`
	CodeReviewModelUsed *CodeReviewModelUsed `json:"codeReviewModelUsed,omitempty"`
}

// SuggestionByPR provides a high-level summary of a suggestion tied to the pull request.
type SuggestionByPR struct {
	ID                  string                `json:"id"`
	SuggestionContent   string                `json:"suggestionContent"`
	OneSentenceSummary  string                `json:"oneSentenceSummary"`
	Label               string                `json:"label"`
	Severity            string                `json:"severity,omitempty"`
	BrokenDrixyRulesIDs []string              `json:"brokenDrixyRulesIds,omitempty"`
	PriorityStatus      enums.PriorityStatus  `json:"priorityStatus,omitempty"`
	DeliveryStatus      enums.DeliveryStatus  `json:"deliveryStatus"`
	Comment             *CommentReference     `json:"comment,omitempty"`
	Files               map[string][]string   `json:"files,omitempty"`
	CreatedAt           string                `json:"createdAt,omitempty"`
	UpdatedAt           string                `json:"updatedAt,omitempty"`
}

// SuggestionSeverityBreakdown reports count distribution across four canonical severity buckets.
type SuggestionSeverityBreakdown struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

// SuggestionCountsBySeverity aggregates delivered, unresolved, and filtered suggestion metrics.
type SuggestionCountsBySeverity struct {
	Sent                 int                         `json:"sent"`
	Filtered             int                         `json:"filtered"`
	Failed               int                         `json:"failed"`
	Replaced             int                         `json:"replaced"`
	Unresolved           int                         `json:"unresolved"`
	UnresolvedBySeverity SuggestionSeverityBreakdown `json:"unresolvedBySeverity"`
	BySeverity           SuggestionSeverityBreakdown `json:"bySeverity"`
	Categories           []string                    `json:"categories"`
}

// PullRequest is the central aggregate root representing an SCM pull / merge request.
type PullRequest struct {
	UUID                      string             `json:"uuid"`
	Title                     string             `json:"title"`
	Status                    string             `json:"status"` // "OPEN", "MERGED", "CLOSED"
	Merged                    bool               `json:"merged"`
	Heavy                     bool               `json:"heavy"`
	Number                    int                `json:"number"`
	URL                       string             `json:"url"`
	BaseBranchRef             string             `json:"baseBranchRef"`
	HeadBranchRef             string             `json:"headBranchRef"`
	Repository                RepositoryInfo     `json:"repository"`
	OpenedAt                  string             `json:"openedAt,omitempty"`
	ClosedAt                  string             `json:"closedAt,omitempty"`
	Files                     []File             `json:"files"`
	TotalAdded                int                `json:"totalAdded"`
	TotalDeleted              int                `json:"totalDeleted"`
	TotalChanges              int                `json:"totalChanges"`
	CreatedAt                 time.Time          `json:"createdAt"`
	UpdatedAt                 time.Time          `json:"updatedAt"`
	Provider                  string             `json:"provider"` // "github", "gitlab", etc.
	User                      PullRequestUser    `json:"user"`
	Reviewers                 []PullRequestUser  `json:"reviewers,omitempty"`
	Assignees                 []PullRequestUser  `json:"assignees,omitempty"`
	OrganizationID            string             `json:"organizationId"`
	Commits                   []Commit           `json:"commits"`
	SyncedEmbeddedSuggestions bool               `json:"syncedEmbeddedSuggestions"`
	SyncedWithIssues          bool               `json:"syncedWithIssues"`
	SuggestionsByPR           []SuggestionByPR   `json:"suggestionsByPR,omitempty"`
	PRLevelSuggestions        []SuggestionByPR   `json:"prLevelSuggestions,omitempty"`
	IsDraft                   bool               `json:"isDraft"`
}

// PullRequestUserMapping maps a PR number to its author for token accounting.
type PullRequestUserMapping struct {
	Number         int             `json:"number"`
	User           PullRequestUser `json:"user"`
	OrganizationID string          `json:"organizationId"`
}

// PullRequestAuthorSuggestion represents author autocomplete aggregate data.
type PullRequestAuthorSuggestion struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Count    int    `json:"count"`
}

// DeliveredSuggestionSummary provides minimal details for tracking posted suggestions.
type DeliveredSuggestionSummary struct {
	ID             string               `json:"id"`
	DeliveryStatus enums.DeliveryStatus `json:"deliveryStatus"`
	Comment        CommentReference     `json:"comment"`
}

// PullRequestWithDeliveredSuggestions pairs a pull request with its posted review comments.
type PullRequestWithDeliveredSuggestions struct {
	ID             string                       `json:"id"`
	Number         int                          `json:"number"`
	OrganizationID string                       `json:"organizationId"`
	Status         string                       `json:"status"`
	Provider       string                       `json:"provider"`
	Repository     RepositoryInfo               `json:"repository"`
	Suggestions    []DeliveredSuggestionSummary `json:"suggestions"`
}

// FileBulkOp defines atomic batch operations for pull request file lists.
type FileBulkOp struct {
	Kind        string                 `json:"kind"` // "addFile" | "updateFile" | "addSuggestions"
	File        *File                  `json:"file,omitempty"`
	FileID      string                 `json:"fileId,omitempty"`
	FileUpdates map[string]interface{} `json:"fileUpdates,omitempty"`
	Suggestions []Suggestion           `json:"suggestions,omitempty"`
}

// BulkApplyError details a specific failure during batch execution.
type BulkApplyError struct {
	OpIndex int    `json:"opIndex"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
}

// BulkApplyResult summarizes the outcome of a batch write operation.
type BulkApplyResult struct {
	Attempted int              `json:"attempted"`
	Modified  int              `json:"modified"`
	Errors    []BulkApplyError `json:"errors"`
}
