// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package types

import "time"

// AzureReposProject models an Azure DevOps team project.
type AzureReposProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	State       string `json:"state,omitempty"`
	Revision    int    `json:"revision,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

// AzureReposRepository models an Azure DevOps Git repository.
type AzureReposRepository struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Project       AzureReposProject `json:"project"`
	DefaultBranch string            `json:"defaultBranch,omitempty"`
	Size          int64             `json:"size,omitempty"`
	RemoteURL     string            `json:"remoteUrl,omitempty"`
	SSHURL        string            `json:"sshUrl,omitempty"`
	WebURL        string            `json:"webUrl,omitempty"`
	IsFork        bool              `json:"isFork,omitempty"`
}

// AzureRepoPullRequest represents native Azure DevOps Git pull request schema.
type AzureRepoPullRequest struct {
	PullRequestID int                  `json:"pullRequestId"`
	CodeReviewID  int                  `json:"codeReviewId,omitempty"`
	Status        string               `json:"status"` // "active", "abandoned", "completed"
	CreatedBy     AzureIdentityRef     `json:"createdBy"`
	CreationDate  time.Time            `json:"creationDate"`
	Title         string               `json:"title"`
	Description   string               `json:"description,omitempty"`
	SourceRefName string               `json:"sourceRefName"`
	TargetRefName string               `json:"targetRefName"`
	MergeStatus   string               `json:"mergeStatus,omitempty"`
	IsDraft       bool                 `json:"isDraft,omitempty"`
	MergeID       string               `json:"mergeId,omitempty"`
	LastMergeSHA  *AzureGitCommitRef   `json:"lastMergeSourceCommit,omitempty"`
	Repository    AzureReposRepository `json:"repository"`
	URL           string               `json:"url"`
	Reviewers     []AzureReviewerRef   `json:"reviewers,omitempty"`
}

// AzureIdentityRef describes an Azure DevOps identity (user, group, service principal).
type AzureIdentityRef struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName,omitempty"`
	URL         string `json:"url,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// AzureReviewerRef extends identity with voting status.
type AzureReviewerRef struct {
	AzureIdentityRef
	Vote            int    `json:"vote"` // 10 = approved, 5 = approved with suggestions, 0 = no vote, -5 = waiting, -10 = rejected
	HasDeclined     bool   `json:"hasDeclined,omitempty"`
	IsRequired      bool   `json:"isRequired,omitempty"`
	ReviewerURL     string `json:"reviewerUrl,omitempty"`
}

// AzureGitCommitRef contains minimal commit identifier.
type AzureGitCommitRef struct {
	CommitID string `json:"commitId"`
	URL      string `json:"url,omitempty"`
}

// AzureRepoThread captures discussion threads on a PR.
type AzureRepoThread struct {
	ID             int                 `json:"id"`
	PublishedDate  time.Time           `json:"publishedDate"`
	LastUpdatedDate time.Time          `json:"lastUpdatedDate"`
	Comments       []AzureRepoComment  `json:"comments"`
	Status         string              `json:"status"` // "active", "fixed", "wontFix", "closed", "byDesign", "pending"
	ThreadContext  *AzureThreadContext `json:"threadContext,omitempty"`
	IsDeleted      bool                `json:"isDeleted,omitempty"`
}

// AzureRepoComment describes a comment within a thread.
type AzureRepoComment struct {
	ID              int              `json:"id"`
	ParentCommentID int              `json:"parentCommentId,omitempty"`
	Author          AzureIdentityRef `json:"author"`
	Content         string           `json:"content"`
	PublishedDate   time.Time        `json:"publishedDate"`
	LastUpdatedDate time.Time        `json:"lastUpdatedDate"`
	CommentType     string           `json:"commentType"` // "text", "codeChange", "system"
}

// AzureThreadContext anchors a discussion thread to specific lines in a file.
type AzureThreadContext struct {
	FilePath       string             `json:"filePath"`
	RightFileStart *AzureFilePosition `json:"rightFileStart,omitempty"`
	RightFileEnd   *AzureFilePosition `json:"rightFileEnd,omitempty"`
	LeftFileStart  *AzureFilePosition `json:"leftFileStart,omitempty"`
	LeftFileEnd    *AzureFilePosition `json:"leftFileEnd,omitempty"`
}

// AzureFilePosition holds line and offset coordinates.
type AzureFilePosition struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

// AzureRepoExtras carries Azure-specific iterations and thread metadata.
type AzureRepoExtras struct {
	IterationID int               `json:"iterationId,omitempty"`
	Threads     []AzureRepoThread `json:"threads,omitempty"`
}
