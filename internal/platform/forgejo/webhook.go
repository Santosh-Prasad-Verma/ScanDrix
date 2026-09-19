package forgejo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform"
)

func verifyForgejoWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	sig := strings.TrimPrefix(signatureHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expectedMAC))
}

func parseForgejoWebhook(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	var event platform.WebhookEventData
	event.RawPayload = payload

	switch eventType {
	case "pull_request", "pull_request_sync":
		event.Type = platform.WebhookEventPullRequest
		var prEvent struct {
			Action     string `json:"action"`
			Repository struct {
				FullName      string `json:"full_name"`
				DefaultBranch string `json:"default_branch"`
			} `json:"repository"`
			Sender struct {
				Username string `json:"username"`
			} `json:"sender"`
			PullRequest struct {
				Index     int                       `json:"number"`
				Title     string                    `json:"title"`
				Head      struct{ SHA, Ref string } `json:"head"`
				Base      struct{ SHA, Ref string } `json:"base"`
				Draft     bool                      `json:"draft"`
				CreatedAt time.Time                 `json:"created_at"`
				User      struct{ Username string } `json:"user"`
			} `json:"pull_request"`
		}

		if err := json.Unmarshal(payload, &prEvent); err != nil {
			return nil, err
		}

		event.Action = prEvent.Action
		event.Repository = prEvent.Repository.FullName
		event.DefaultBranch = prEvent.Repository.DefaultBranch
		event.Sender = prEvent.Sender.Username
		event.PullRequest = &platform.PullRequestDetails{
			Number:       prEvent.PullRequest.Index,
			Title:        prEvent.PullRequest.Title,
			Author:       prEvent.PullRequest.User.Username,
			HeadSHA:      prEvent.PullRequest.Head.SHA,
			BaseSHA:      prEvent.PullRequest.Base.SHA,
			SourceBranch: prEvent.PullRequest.Head.Ref,
			TargetBranch: prEvent.PullRequest.Base.Ref,
			CreatedAt:    prEvent.PullRequest.CreatedAt,
			IsDraft:      prEvent.PullRequest.Draft,
		}

	case "push":
		event.Type = platform.WebhookEventPush
		var pushEvent struct {
			After      string `json:"after"`
			Repository struct {
				FullName      string `json:"full_name"`
				DefaultBranch string `json:"default_branch"`
			} `json:"repository"`
			Pusher struct{ Username string } `json:"pusher"`
		}
		if err := json.Unmarshal(payload, &pushEvent); err != nil {
			return nil, err
		}
		event.CommitSHA = pushEvent.After
		event.Repository = pushEvent.Repository.FullName
		event.DefaultBranch = pushEvent.Repository.DefaultBranch
		event.Sender = pushEvent.Pusher.Username

	case "issue_comment", "pull_request_comment":
		event.Type = platform.WebhookEventReviewComment
		var commentEvent struct {
			Action     string `json:"action"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Comment struct {
				ID   int64                     `json:"id"`
				Body string                    `json:"body"`
				User struct{ Username string } `json:"user"`
			} `json:"comment"`
			PullRequest struct {
				Index int `json:"number"`
			} `json:"pull_request"`
		}
		if err := json.Unmarshal(payload, &commentEvent); err != nil {
			return nil, err
		}
		event.Action = commentEvent.Action
		event.Repository = commentEvent.Repository.FullName
		event.CommentID = commentEvent.Comment.ID
		event.CommentBody = commentEvent.Comment.Body
		event.Sender = commentEvent.Comment.User.Username
	default:
		event.Type = platform.WebhookEventType(eventType)
	}

	return &event, nil
}
