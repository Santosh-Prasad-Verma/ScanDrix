package azuredevops

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
)

func verifyAzureServiceHookSignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" {
		return true // If no secret configured
	}
	if signatureHeader == "" {
		return false
	}
	sig := strings.TrimPrefix(signatureHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expectedMAC)) || signatureHeader == secret
}

func parseAzureWebhook(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	var event platform.WebhookEventData
	event.RawPayload = payload

	var root struct {
		EventType    string `json:"eventType"`
		Resource     json.RawMessage `json:"resource"`
		ResourceVersion string `json:"resourceVersion"`
	}
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, err
	}

	actualEvent := eventType
	if actualEvent == "" {
		actualEvent = root.EventType
	}

	switch actualEvent {
	case "git.pullrequest.created", "git.pullrequest.updated", "git.pullrequest.merged":
		event.Type = platform.WebhookEventPullRequest
		event.Action = strings.TrimPrefix(actualEvent, "git.pullrequest.")

		var pr struct {
			PullRequestID int    `json:"pullRequestId"`
			Title         string `json:"title"`
			CreatedBy     struct {
				DisplayName string `json:"displayName"`
				UniqueName  string `json:"uniqueName"`
			} `json:"createdBy"`
			Repository struct {
				Name    string `json:"name"`
				Project struct {
					Name string `json:"name"`
				} `json:"project"`
				DefaultBranch string `json:"defaultBranch"`
			} `json:"repository"`
			LastMergeSourceCommit struct {
				CommitID string `json:"commitId"`
			} `json:"lastMergeSourceCommit"`
			LastMergeTargetCommit struct {
				CommitID string `json:"commitId"`
			} `json:"lastMergeTargetCommit"`
			SourceRefName string    `json:"sourceRefName"`
			TargetRefName string    `json:"targetRefName"`
			CreationDate  time.Time `json:"creationDate"`
			IsDraft       bool      `json:"isDraft"`
		}

		if err := json.Unmarshal(root.Resource, &pr); err != nil {
			return nil, err
		}

		repoPath := fmt.Sprintf("%s/%s", pr.Repository.Project.Name, pr.Repository.Name)
		author := pr.CreatedBy.UniqueName
		if author == "" {
			author = pr.CreatedBy.DisplayName
		}

		event.Repository = repoPath
		event.DefaultBranch = strings.TrimPrefix(pr.Repository.DefaultBranch, "refs/heads/")
		event.Sender = author
		event.PullRequest = &platform.PullRequestDetails{
			Number:       pr.PullRequestID,
			Title:        pr.Title,
			Author:       author,
			HeadSHA:      pr.LastMergeSourceCommit.CommitID,
			BaseSHA:      pr.LastMergeTargetCommit.CommitID,
			SourceBranch: strings.TrimPrefix(pr.SourceRefName, "refs/heads/"),
			TargetBranch: strings.TrimPrefix(pr.TargetRefName, "refs/heads/"),
			CreatedAt:    pr.CreationDate,
			IsDraft:      pr.IsDraft,
		}

	case "git.push":
		event.Type = platform.WebhookEventPush
		var push struct {
			PushedBy struct {
				DisplayName string `json:"displayName"`
				UniqueName  string `json:"uniqueName"`
			} `json:"pushedBy"`
			Repository struct {
				Name    string `json:"name"`
				Project struct {
					Name string `json:"name"`
				} `json:"project"`
				DefaultBranch string `json:"defaultBranch"`
			} `json:"repository"`
			Commits []struct {
				CommitID string `json:"commitId"`
			} `json:"commits"`
			RefUpdates []struct {
				Name    string `json:"name"`
				NewObjectID string `json:"newObjectId"`
			} `json:"refUpdates"`
		}

		if err := json.Unmarshal(root.Resource, &push); err != nil {
			return nil, err
		}

		event.Repository = fmt.Sprintf("%s/%s", push.Repository.Project.Name, push.Repository.Name)
		event.DefaultBranch = strings.TrimPrefix(push.Repository.DefaultBranch, "refs/heads/")
		sender := push.PushedBy.UniqueName
		if sender == "" {
			sender = push.PushedBy.DisplayName
		}
		event.Sender = sender
		if len(push.RefUpdates) > 0 {
			event.CommitSHA = push.RefUpdates[0].NewObjectID
		} else if len(push.Commits) > 0 {
			event.CommitSHA = push.Commits[0].CommitID
		}

	case "ms.vss-code.git-pullrequest-comment-event":
		event.Type = platform.WebhookEventReviewComment
		var commentResource struct {
			Comment struct {
				ID      int64  `json:"id"`
				Content string `json:"content"`
				Author  struct {
					DisplayName string `json:"displayName"`
					UniqueName  string `json:"uniqueName"`
				} `json:"author"`
			} `json:"comment"`
			PullRequest struct {
				PullRequestID int `json:"pullRequestId"`
				Repository    struct {
					Name    string `json:"name"`
					Project struct {
						Name string `json:"name"`
					} `json:"project"`
				} `json:"repository"`
			} `json:"pullRequest"`
		}

		if err := json.Unmarshal(root.Resource, &commentResource); err != nil {
			return nil, err
		}

		event.Repository = fmt.Sprintf("%s/%s", commentResource.PullRequest.Repository.Project.Name, commentResource.PullRequest.Repository.Name)
		event.CommentID = commentResource.Comment.ID
		event.CommentBody = commentResource.Comment.Content
		sender := commentResource.Comment.Author.UniqueName
		if sender == "" {
			sender = commentResource.Comment.Author.DisplayName
		}
		event.Sender = sender
	default:
		event.Type = platform.WebhookEventType(actualEvent)
	}

	return &event, nil
}
