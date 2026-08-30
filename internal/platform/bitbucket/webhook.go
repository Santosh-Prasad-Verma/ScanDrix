package bitbucket

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
)

func verifyBitbucketSignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	sig := strings.TrimPrefix(signatureHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expectedMAC))
}

func parseBitbucketWebhook(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	var event platform.WebhookEventData
	event.RawPayload = payload

	switch eventType {
	case "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected":
		event.Type = platform.WebhookEventPullRequest
		event.Action = strings.TrimPrefix(eventType, "pullrequest:")

		var bbPR struct {
			Actor struct {
				DisplayName string `json:"display_name"`
				Nickname    string `json:"nickname"`
			} `json:"actor"`
			Repository struct {
				FullName string `json:"full_name"`
				Name     string `json:"name"`
			} `json:"repository"`
			PullRequest struct {
				ID          int       `json:"id"`
				Title       string    `json:"title"`
				Description string    `json:"description"`
				CreatedOn   time.Time `json:"created_on"`
				Source      struct {
					Branch struct{ Name string } `json:"branch"`
					Commit struct{ Hash string } `json:"commit"`
				} `json:"source"`
				Destination struct {
					Branch struct{ Name string } `json:"branch"`
					Commit struct{ Hash string } `json:"commit"`
				} `json:"destination"`
				Author struct {
					Nickname    string `json:"nickname"`
					DisplayName string `json:"display_name"`
				} `json:"author"`
			} `json:"pullrequest"`
		}

		if err := json.Unmarshal(payload, &bbPR); err != nil {
			return nil, err
		}

		author := bbPR.PullRequest.Author.Nickname
		if author == "" {
			author = bbPR.PullRequest.Author.DisplayName
		}
		sender := bbPR.Actor.Nickname
		if sender == "" {
			sender = bbPR.Actor.DisplayName
		}

		event.Repository = bbPR.Repository.FullName
		event.DefaultBranch = bbPR.PullRequest.Destination.Branch.Name
		event.Sender = sender
		event.PullRequest = &platform.PullRequestDetails{
			Number:       bbPR.PullRequest.ID,
			Title:        bbPR.PullRequest.Title,
			Author:       author,
			HeadSHA:      bbPR.PullRequest.Source.Commit.Hash,
			BaseSHA:      bbPR.PullRequest.Destination.Commit.Hash,
			SourceBranch: bbPR.PullRequest.Source.Branch.Name,
			TargetBranch: bbPR.PullRequest.Destination.Branch.Name,
			CreatedAt:    bbPR.PullRequest.CreatedOn,
			IsDraft:      false,
		}

	case "repo:push":
		event.Type = platform.WebhookEventPush
		var pushEvent struct {
			Actor struct {
				DisplayName string `json:"display_name"`
				Nickname    string `json:"nickname"`
			} `json:"actor"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Push struct {
				Changes []struct {
					New struct {
						Name   string `json:"name"`
						Target struct {
							Hash string `json:"hash"`
						} `json:"target"`
					} `json:"new"`
				} `json:"changes"`
			} `json:"push"`
		}
		if err := json.Unmarshal(payload, &pushEvent); err != nil {
			return nil, err
		}
		event.Repository = pushEvent.Repository.FullName
		sender := pushEvent.Actor.Nickname
		if sender == "" {
			sender = pushEvent.Actor.DisplayName
		}
		event.Sender = sender
		if len(pushEvent.Push.Changes) > 0 {
			event.CommitSHA = pushEvent.Push.Changes[0].New.Target.Hash
			event.DefaultBranch = pushEvent.Push.Changes[0].New.Name
		}

	case "pullrequest:comment_created", "pullrequest:comment_updated":
		event.Type = platform.WebhookEventReviewComment
		var commentEvent struct {
			Actor struct {
				Nickname string `json:"nickname"`
			} `json:"actor"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Comment struct {
				ID      int64 `json:"id"`
				Content struct {
					Raw string `json:"raw"`
				} `json:"content"`
			} `json:"comment"`
			PullRequest struct {
				ID int `json:"id"`
			} `json:"pullrequest"`
		}
		if err := json.Unmarshal(payload, &commentEvent); err != nil {
			return nil, err
		}
		event.Repository = commentEvent.Repository.FullName
		event.CommentID = commentEvent.Comment.ID
		event.CommentBody = commentEvent.Comment.Content.Raw
		event.Sender = commentEvent.Actor.Nickname
	default:
		event.Type = platform.WebhookEventType(eventType)
	}

	return &event, nil
}
