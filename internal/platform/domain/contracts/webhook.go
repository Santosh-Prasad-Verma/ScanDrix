package contracts

import (
	"context"
	"encoding/json"

	"github.com/scandrix/backend/pkg/models"
)

// WebhookEventParams holds parameters supplied to webhook handlers.
type WebhookEventParams struct {
	PlatformType models.SCMProvider `json:"platformType"`
	Event        string             `json:"event"`
	Action       string             `json:"action,omitempty"`
	RawPayload   json.RawMessage    `json:"rawPayload"`
	Headers      map[string]string  `json:"headers,omitempty"`
}

// IWebhookEventHandler processes normalized incoming provider webhooks.
type IWebhookEventHandler interface {
	CanHandle(params WebhookEventParams) bool
	Handle(ctx context.Context, params WebhookEventParams) error
}

// IMappedUsers provides normalized actors from a webhook payload.
type IMappedUsers struct {
	User      any `json:"user"`
	Assignees any `json:"assignees,omitempty"`
	Reviewers any `json:"reviewers,omitempty"`
}

// IMappedBranchRef holds head or base branch references.
type IMappedBranchRef struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha,omitempty"`
	Repo struct {
		FullName      string `json:"fullName"`
		DefaultBranch string `json:"defaultBranch,omitempty"`
	} `json:"repo"`
}

// IMappedPullRequest provides standardized PR details from webhook events.
type IMappedPullRequest struct {
	Repository any              `json:"repository"`
	Title      string           `json:"title"`
	Body       string           `json:"body"`
	Number     int              `json:"number"`
	User       any              `json:"user"`
	Head       IMappedBranchRef `json:"head"`
	Base       IMappedBranchRef `json:"base"`
	Status     string           `json:"status,omitempty"`
	IsDraft    bool             `json:"isDraft"`
	URL        string           `json:"url"`
	Tags       []string         `json:"tags,omitempty"`
}

// IMappedRepository normalizes repository fields across providers.
type IMappedRepository struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
	FullName string `json:"fullName"`
	URL      string `json:"url"`
}

// IMappedComment normalizes PR or issue comment fields.
type IMappedComment struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// IMappedPlatform abstracts provider-specific webhook mapping functions.
type IMappedPlatform interface {
	MapPullRequest(payload any) *IMappedPullRequest
	MapUsers(payload any) *IMappedUsers
	MapRepository(payload any) *IMappedRepository
	MapComment(payload any) *IMappedComment
	MapAction(payload any, event string) string
}
