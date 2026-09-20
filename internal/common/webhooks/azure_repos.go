package webhooks

import (
	"fmt"
	"strconv"
)

// AzureReposIdentity describes an Azure DevOps user.
type AzureReposIdentity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	URL         string `json:"url"`
	ImageURL    string `json:"imageUrl"`
}

// AzureReposRepositoryRef describes repository details in Azure webhook payloads.
type AzureReposRepositoryRef struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	RemoteURL     string `json:"remoteUrl"`
	DefaultBranch string `json:"defaultBranch"`
	Project       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
}

// AzureReposPullRequestRef contains pull request attributes in Azure webhook payloads.
type AzureReposPullRequestRef struct {
	PullRequestID int                     `json:"pullRequestId"`
	CodeReviewID  int                     `json:"codeReviewId"`
	Status        string                  `json:"status"`
	CreatedBy     *AzureReposIdentity     `json:"createdBy"`
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	SourceRefName string                  `json:"sourceRefName"`
	TargetRefName string                  `json:"targetRefName"`
	URL           string                  `json:"url"`
	IsDraft       bool                    `json:"isDraft"`
	Repository    AzureReposRepositoryRef `json:"repository"`
	Reviewers     []AzureReposIdentity    `json:"reviewers"`
	Labels        []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	} `json:"labels"`
}

// AzureReposCommentRef contains comment payload structure.
type AzureReposCommentRef struct {
	ID          int                 `json:"id"`
	ParentID    int                 `json:"parentId"`
	Author      *AzureReposIdentity `json:"author"`
	Content     string              `json:"content"`
	CommentType string              `json:"commentType"`
}

// AzureReposWebhookPayload is the root JSON body received from Azure DevOps Service Hooks.
type AzureReposWebhookPayload struct {
	SubscriptionID string `json:"subscriptionId"`
	NotificationID int    `json:"notificationId"`
	ID             string `json:"id"`
	EventType      string `json:"eventType"`
	Resource       struct {
		PullRequest *AzureReposPullRequestRef `json:"pullRequest"`
		Comment     *AzureReposCommentRef     `json:"comment"`
		Repository  *AzureReposRepositoryRef  `json:"repository"`

		// Fallback when fields are inlined directly under resource
		PullRequestID int                 `json:"pullRequestId"`
		Title         string              `json:"title"`
		Description   string              `json:"description"`
		SourceRefName string              `json:"sourceRefName"`
		TargetRefName string              `json:"targetRefName"`
		URL           string              `json:"url"`
		CreatedBy     *AzureReposIdentity `json:"createdBy"`
		IsDraft       bool                `json:"isDraft"`
	} `json:"resource"`
}

// AzureReposMappedPlatform maps Azure DevOps webhook payloads into standardized ScanDrix models.
type AzureReposMappedPlatform struct{}

// MapAction converts Azure DevOps eventType into MappedAction.
func (a *AzureReposMappedPlatform) MapAction(payload AzureReposWebhookPayload) MappedAction {
	switch payload.EventType {
	case "git.pullrequest.created":
		return ActionOpened
	case "git.pullrequest.updated":
		return ActionUpdated
	case "git.pullrequest.merged":
		return ActionMerged
	case "git.pullrequest.comment":
		return ActionCommentCreated
	default:
		return ActionUnknown
	}
}

// MapUser maps the creator or author into MappedUser.
func (a *AzureReposMappedPlatform) MapUser(payload AzureReposWebhookPayload) *MappedUser {
	identity := payload.Resource.CreatedBy
	if payload.Resource.PullRequest != nil && payload.Resource.PullRequest.CreatedBy != nil {
		identity = payload.Resource.PullRequest.CreatedBy
	} else if payload.Resource.Comment != nil && payload.Resource.Comment.Author != nil {
		identity = payload.Resource.Comment.Author
	}

	if identity == nil {
		return nil
	}

	return &MappedUser{
		ID:        identity.ID,
		Login:     identity.UniqueName,
		Name:      identity.DisplayName,
		AvatarURL: identity.ImageURL,
	}
}

// MapPullRequest standardizes Azure pull request details.
func (a *AzureReposMappedPlatform) MapPullRequest(payload AzureReposWebhookPayload) *MappedPullRequest {
	pr := payload.Resource.PullRequest

	prID := 0
	title := ""
	body := ""
	url := ""
	srcRef := ""
	tgtRef := ""
	isDraft := false
	var createdBy *AzureReposIdentity
	repoFullName := ""
	defaultBranch := ""

	if pr != nil {
		prID = pr.PullRequestID
		title = pr.Title
		body = pr.Description
		url = pr.URL
		srcRef = pr.SourceRefName
		tgtRef = pr.TargetRefName
		isDraft = pr.IsDraft
		createdBy = pr.CreatedBy
		if pr.Repository.Project.Name != "" {
			repoFullName = fmt.Sprintf("%s/%s", pr.Repository.Project.Name, pr.Repository.Name)
		} else {
			repoFullName = pr.Repository.Name
		}
		defaultBranch = pr.Repository.DefaultBranch
	} else if payload.Resource.PullRequestID != 0 {
		prID = payload.Resource.PullRequestID
		title = payload.Resource.Title
		body = payload.Resource.Description
		url = payload.Resource.URL
		srcRef = payload.Resource.SourceRefName
		tgtRef = payload.Resource.TargetRefName
		isDraft = payload.Resource.IsDraft
		createdBy = payload.Resource.CreatedBy
	}

	if prID == 0 {
		return nil
	}

	var user *MappedUser
	if createdBy != nil {
		user = &MappedUser{
			ID:        createdBy.ID,
			Login:     createdBy.UniqueName,
			Name:      createdBy.DisplayName,
			AvatarURL: createdBy.ImageURL,
		}
	}

	return &MappedPullRequest{
		ID:      strconv.Itoa(prID),
		Number:  prID,
		Title:   title,
		Body:    body,
		URL:     url,
		IsDraft: isDraft,
		User:    user,
		Head: MappedCommitRef{
			RepoFullName: repoFullName,
			Ref:          srcRef,
		},
		Base: MappedCommitRef{
			RepoFullName: repoFullName,
			Ref:          tgtRef,
		},
		Repository: &MappedRepository{
			FullName:      repoFullName,
			DefaultBranch: defaultBranch,
		},
	}
}

// MapComment parses comment payloads from Azure webhook.
func (a *AzureReposMappedPlatform) MapComment(payload AzureReposWebhookPayload) *MappedComment {
	if payload.Resource.Comment == nil {
		return nil
	}
	cmt := payload.Resource.Comment
	return &MappedComment{
		ID:   strconv.Itoa(cmt.ID),
		Body: cmt.Content,
	}
}
