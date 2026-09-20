// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

// AzureReposWebhookMessage represents message content (text, html, markdown).
type AzureReposWebhookMessage struct {
	Text     string `json:"text"`
	HTML     string `json:"html"`
	Markdown string `json:"markdown"`
}

// AzureReposWebhookResourceContainers holds collection, account, and project IDs.
type AzureReposWebhookResourceContainers struct {
	Collection struct {
		ID string `json:"id"`
	} `json:"collection"`
	Account struct {
		ID string `json:"id"`
	} `json:"account"`
	Project struct {
		ID string `json:"id"`
	} `json:"project"`
}

// AzureReposWebhookUser represents a user, author, or reviewer in Azure DevOps.
type AzureReposWebhookUser struct {
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
	ID          string `json:"id"`
	UniqueName  string `json:"uniqueName"`
	ImageURL    string `json:"imageUrl"`
	IsContainer bool   `json:"isContainer,omitempty"`
}

// AzureReposWebhookReviewer extends AzureReposWebhookUser with review specific fields.
type AzureReposWebhookReviewer struct {
	AzureReposWebhookUser
	ReviewerURL *string `json:"reviewerUrl,omitempty"`
	Vote        int     `json:"vote"`
}

// AzureReposWebhookCommit represents commit metadata in Azure DevOps.
type AzureReposWebhookCommit struct {
	CommitID string `json:"commitId"`
	URL      string `json:"url"`
}

// AzureReposWebhookProject represents project info in Azure DevOps.
type AzureReposWebhookProject struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	URL            string  `json:"url"`
	State          string  `json:"state"`
	Visibility     *string `json:"visibility,omitempty"`
	LastUpdateTime *string `json:"lastUpdateTime,omitempty"`
}

// AzureReposWebhookRepository represents repository info in Azure DevOps.
type AzureReposWebhookRepository struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	URL           string                   `json:"url"`
	Project       AzureReposWebhookProject `json:"project"`
	DefaultBranch string                   `json:"defaultBranch"`
	RemoteURL     string                   `json:"remoteUrl"`
}

// AzureReposWebhookLabel represents label metadata on Azure Repos PR.
type AzureReposWebhookLabel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// AzureReposWebhookCommentLinks contains links for a comment.
type AzureReposWebhookCommentLinks struct {
	Self       struct{ HREF string `json:"href"` } `json:"self"`
	Repository struct{ HREF string `json:"href"` } `json:"repository"`
	Threads    struct{ HREF string `json:"href"` } `json:"threads"`
}

// AzureReposWebhookComment represents a comment inside Azure Repos PR.
type AzureReposWebhookComment struct {
	ID                    int                           `json:"id"`
	ParentCommentID       int                           `json:"parentCommentId"`
	Author                AzureReposWebhookUser         `json:"author"`
	Content               string                        `json:"content"`
	PublishedDate         string                        `json:"publishedDate"`
	LastUpdatedDate       string                        `json:"lastUpdatedDate"`
	LastContentUpdatedDate string                       `json:"lastContentUpdatedDate"`
	CommentType           string                        `json:"commentType"`
	Links                 AzureReposWebhookCommentLinks `json:"_links"`
}

// AzureReposWebhookPullRequest holds pull request metadata in Azure Repos.
type AzureReposWebhookPullRequest struct {
	Repository            AzureReposWebhookRepository            `json:"repository"`
	PullRequestID         int                                    `json:"pullRequestId"`
	Status                string                                 `json:"status"`
	CreatedBy             AzureReposWebhookUser                  `json:"createdBy"`
	CreationDate          string                                 `json:"creationDate"`
	ClosedDate            string                                 `json:"closedDate,omitempty"`
	Title                 string                                 `json:"title"`
	Description           string                                 `json:"description"`
	SourceRefName         string                                 `json:"sourceRefName"`
	TargetRefName         string                                 `json:"targetRefName"`
	MergeStatus           string                                 `json:"mergeStatus"`
	MergeID               string                                 `json:"mergeId"`
	LastMergeSourceCommit *AzureReposWebhookCommit               `json:"lastMergeSourceCommit,omitempty"`
	LastMergeTargetCommit *AzureReposWebhookCommit               `json:"lastMergeTargetCommit,omitempty"`
	LastMergeCommit       *AzureReposWebhookCommit               `json:"lastMergeCommit,omitempty"`
	Reviewers             []AzureReposWebhookReviewer            `json:"reviewers,omitempty"`
	Commits               []AzureReposWebhookCommit              `json:"commits,omitempty"`
	URL                   string                                 `json:"url"`
	Links                 map[string]map[string]string           `json:"_links,omitempty"`
	Labels                []AzureReposWebhookLabel               `json:"labels,omitempty"`
	IsDraft               bool                                   `json:"isDraft"`
}

// AzureReposWebhookResource represents the polymorphic resource payload from Azure Service Hooks.
type AzureReposWebhookResource struct {
	Comment               *AzureReposWebhookComment              `json:"comment,omitempty"`
	PullRequest           *AzureReposWebhookPullRequest          `json:"pullRequest,omitempty"`
	Repository            *AzureReposWebhookRepository           `json:"repository,omitempty"`
	PullRequestID         int                                    `json:"pullRequestId,omitempty"`
	Status                string                                 `json:"status,omitempty"`
	CreatedBy             *AzureReposWebhookUser                 `json:"createdBy,omitempty"`
	CreationDate          string                                 `json:"creationDate,omitempty"`
	ClosedDate            string                                 `json:"closedDate,omitempty"`
	Title                 string                                 `json:"title,omitempty"`
	Description           string                                 `json:"description,omitempty"`
	SourceRefName         string                                 `json:"sourceRefName,omitempty"`
	TargetRefName         string                                 `json:"targetRefName,omitempty"`
	MergeStatus           string                                 `json:"mergeStatus,omitempty"`
	MergeID               string                                 `json:"mergeId,omitempty"`
	LastMergeSourceCommit *AzureReposWebhookCommit               `json:"lastMergeSourceCommit,omitempty"`
	LastMergeTargetCommit *AzureReposWebhookCommit               `json:"lastMergeTargetCommit,omitempty"`
	LastMergeCommit       *AzureReposWebhookCommit               `json:"lastMergeCommit,omitempty"`
	Reviewers             []AzureReposWebhookReviewer            `json:"reviewers,omitempty"`
	Commits               []AzureReposWebhookCommit              `json:"commits,omitempty"`
	URL                   string                                 `json:"url,omitempty"`
	Links                 map[string]map[string]string           `json:"_links,omitempty"`
	IsDraft               bool                                   `json:"isDraft,omitempty"`
	Labels                []AzureReposWebhookLabel               `json:"labels,omitempty"`
}

// AzureReposWebhookPayload is the root schema for Azure DevOps Service Hooks.
type AzureReposWebhookPayload struct {
	ID                 string                              `json:"id"`
	EventType          string                              `json:"eventType"`
	PublisherID        string                              `json:"publisherId"`
	Scope              string                              `json:"scope,omitempty"`
	Message            AzureReposWebhookMessage            `json:"message"`
	DetailedMessage    AzureReposWebhookMessage            `json:"detailedMessage"`
	Resource           AzureReposWebhookResource           `json:"resource"`
	ResourceVersion    string                              `json:"resourceVersion"`
	ResourceContainers AzureReposWebhookResourceContainers `json:"resourceContainers"`
	CreatedDate        string                              `json:"createdDate"`
}
