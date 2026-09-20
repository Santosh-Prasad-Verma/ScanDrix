// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package types

import "time"
// PullRequestReviewState represents the review decision on a pull request.
type PullRequestReviewState string

const (
	PullRequestReviewStateCommented        PullRequestReviewState = "COMMENTED"
	PullRequestReviewStatePending          PullRequestReviewState = "PENDING"
	PullRequestReviewStateApproved         PullRequestReviewState = "APPROVED"
	PullRequestReviewStateChangesRequested PullRequestReviewState = "CHANGES_REQUESTED"
	PullRequestReviewStateDismissed        PullRequestReviewState = "DISMISSED"
)

// GitActor represents an author or committer in Git.
type GitActor struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// PullRequestFileChange defines a file mutation intended for a PR commit.
type PullRequestFileChange struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Operation string `json:"operation,omitempty"` // "upsert" or "delete"
}

// PullRequestRepoReference captures minimal repository context inside PR head/base.
type PullRequestRepoReference struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	FullName      string `json:"fullName"`
}

// PullRequestBranchReference captures branch and commit SHA.
type PullRequestBranchReference struct {
	Ref  string                   `json:"ref"`
	SHA  string                   `json:"sha,omitempty"`
	Repo PullRequestRepoReference `json:"repo"`
}

// PullRequestUser defines the author or actor of a pull request.
type PullRequestUser struct {
	Login     string `json:"login"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	ID        string `json:"id"`
	IsBot     bool   `json:"is_bot,omitempty"`
}

// PullRequestParticipant defines an active commenter or reviewer on a PR.
type PullRequestParticipant struct {
	ID string `json:"id"`
}

// PullRequest aggregates normalized metadata across all Git providers.
// Matches libs/platform/domain/platformIntegrations/types/codeManagement/pullRequests.type.ts
type PullRequest struct {
	ID             string                     `json:"id"`
	Number         int                        `json:"number"`
	PullNumber     int                        `json:"pull_number"`
	Body           string                     `json:"body"`
	Description    string                     `json:"description,omitempty"`
	Title          string                     `json:"title"`
	Message        string                     `json:"message"`
	State          string                     `json:"state"` // "open", "closed", "merged"
	Author         string                     `json:"author,omitempty"`
	OrganizationID string                     `json:"organizationId"`
	Repository     string                     `json:"repository"`
	RepositoryID   string                     `json:"repositoryId"`
	RepositoryData RepositoryDescriptor       `json:"repositoryData"`
	PRURL          string                     `json:"prURL"`
	URL            string                     `json:"url,omitempty"`
	CreatedAt      string                     `json:"created_at"`
	ClosedAt       string                     `json:"closed_at,omitempty"`
	UpdatedAt      string                     `json:"updated_at"`
	MergedAt       string                     `json:"merged_at,omitempty"`
	Participants   []PullRequestParticipant   `json:"participants,omitempty"`
	Reviewers      []PullRequestParticipant   `json:"reviewers,omitempty"`
	SourceRefName  string                     `json:"sourceRefName"`
	SourceBranch   string                     `json:"sourceBranch,omitempty"`
	Head           PullRequestBranchReference `json:"head"`
	HeadSHA        string                     `json:"headSha,omitempty"`
	TargetRefName  string                     `json:"targetRefName"`
	TargetBranch   string                     `json:"targetBranch,omitempty"`
	Base           PullRequestBranchReference `json:"base"`
	BaseSHA        string                     `json:"baseSha,omitempty"`
	User           PullRequestUser            `json:"user"`
	IsDraft        bool                       `json:"isDraft"`
}

// RepositoryDescriptor identifies repository id, name, and owner details.
type RepositoryDescriptor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Owner    string `json:"owner,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

// PullRequestFile holds diff metrics and paths for a single modified file.
type PullRequestFile struct {
	SHA         string `json:"sha,omitempty"`
	Filename    string `json:"filename"`
	Status      string `json:"status,omitempty"` // "added", "modified", "removed"
	Additions   int    `json:"additions,omitempty"`
	Deletions   int    `json:"deletions,omitempty"`
	Changes     int    `json:"changes"`
	BlobURL     string `json:"blob_url,omitempty"`
	RawURL      string `json:"raw_url,omitempty"`
	ContentsURL string `json:"contents_url,omitempty"`
	Patch       string `json:"patch,omitempty"`
}

// PullRequestCodeReviewTime measures time to merge/close review metrics.
type PullRequestCodeReviewTime struct {
	PRNumber       int        `json:"pr_number"`
	ReviewDuration float64    `json:"review_duration_seconds"`
	CreatedAt      time.Time  `json:"created_at"`
	MergedAt       *time.Time `json:"merged_at,omitempty"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
	Author         string     `json:"author"`
}

// PullRequestWithFiles embeds pull request details along with affected file changes.
type PullRequestWithFiles struct {
	PullRequest
	ID               int                `json:"id,omitempty"`
	PullNumber       int                `json:"pull_number"`
	State            string             `json:"state"`
	Title            string             `json:"title"`
	Repository       any                `json:"repository,omitempty"`
	PullRequestFiles []*PullRequestFile `json:"pullRequestFiles,omitempty"`
	Files            []*PullRequestFile `json:"files,omitempty"`
}

// PullRequestReviewComment represents an inline or general code review comment.
type PullRequestReviewComment struct {
	ID             string                    `json:"id"`
	ThreadID       string                    `json:"threadId,omitempty"`
	FullDatabaseID string                    `json:"fullDatabaseId,omitempty"`
	IsResolved     bool                      `json:"isResolved,omitempty"`
	IsOutdated     bool                      `json:"isOutdated,omitempty"`
	Body           string                    `json:"body"`
	Path           string                    `json:"path,omitempty"`
	Line           int                       `json:"line,omitempty"`
	StartLine      int                       `json:"start_line,omitempty"`
	CommitID       string                    `json:"commit_id,omitempty"`
	Author         *PullRequestCommentAuthor `json:"author,omitempty"`
	CreatedAt      string                    `json:"createdAt,omitempty"`
	UpdatedAt      string                    `json:"updatedAt,omitempty"`
}

// PullRequestCommentAuthor describes who authored a comment.
type PullRequestCommentAuthor struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
}

// ReactionsInComments counts emoji reactions on review comments.
type ReactionsInComments struct {
	Reaction   string `json:"reaction,omitempty"`
	Count      int    `json:"count,omitempty"`
	Reactions  struct {
		ThumbsUp   int `json:"thumbsUp"`
		ThumbsDown int `json:"thumbsDown"`
	} `json:"reactions"`
	Comment struct {
		ID                  string `json:"id"`
		Body                string `json:"body"`
		PullRequestReviewID string `json:"pull_request_review_id"`
	} `json:"comment"`
	PullRequest struct {
		ID         string `json:"id"`
		Number     int    `json:"number"`
		Repository struct {
			ID       string `json:"id"`
			FullName string `json:"fullName"`
		} `json:"repository"`
	} `json:"pullRequest"`
}

// PullRequestsWithChangesRequested notes PRs that need author fixes before merge.
type PullRequestsWithChangesRequested struct {
	Title            string                 `json:"title"`
	Number           int                    `json:"number"`
	ReviewDecision   PullRequestReviewState `json:"reviewDecision"`
	PullRequest      PullRequest            `json:"pullRequest,omitempty"`
	ChangesRequested bool                   `json:"changesRequested,omitempty"`
}

// PullRequestAuthor details author contributions.
type PullRequestAuthor struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Username      string `json:"username,omitempty"`
	Contributions int    `json:"contributions,omitempty"`
	Type          string `json:"type,omitempty"`
	IsBot         bool   `json:"is_bot,omitempty"`
}

// OneSentenceSummaryItem provides short AI-generated summary items.
type OneSentenceSummaryItem struct {
	ID                 int    `json:"id,omitempty"`
	OneSentenceSummary string `json:"oneSentenceSummary"`
}

// CodeManagementConnectionStatus reports VCS connection and setup health.
type CodeManagementConnectionStatus struct {
	HasConnection   bool   `json:"hasConnection"`
	IsConnected     bool   `json:"isConnected,omitempty"`
	Message         string `json:"message,omitempty"`
	IsSetupComplete bool   `json:"isSetupComplete"`
	Config          any    `json:"config,omitempty"`
	PlatformName    string `json:"platformName"`
	Category        string `json:"category,omitempty"`
}
