// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

// WebhookAction constants represent lifecycle transitions across Git platforms.
type WebhookAction string

const (
	ActionOpened      WebhookAction = "opened"
	ActionSynchronize WebhookAction = "synchronize"
	ActionUpdated     WebhookAction = "updated"
	ActionClosed      WebhookAction = "closed"
	ActionReopened    WebhookAction = "reopened"
	ActionEdited      WebhookAction = "edited"
	ActionCreated     WebhookAction = "created"
	ActionDeleted     WebhookAction = "deleted"
	ActionSubmitted   WebhookAction = "submitted"
	ActionDismissed   WebhookAction = "dismissed"
)

// CommonSender represents the identity that triggered a webhook event.
type CommonSender struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Type      string `json:"type,omitempty"` // "User", "Bot"
}

// CommonRepository captures repository coordinates in a webhook.
type CommonRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	IsPrivate     bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
}

// CommonInstallation identifies an App or integration tenant installation.
type CommonInstallation struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id,omitempty"`
}

// PingEventPayload handles verification pings sent by Git hosts.
type PingEventPayload struct {
	Zen          string              `json:"zen,omitempty"`
	HookID       int64               `json:"hook_id,omitempty"`
	Repository   *CommonRepository   `json:"repository,omitempty"`
	Sender       *CommonSender       `json:"sender,omitempty"`
	Installation *CommonInstallation `json:"installation,omitempty"`
}
