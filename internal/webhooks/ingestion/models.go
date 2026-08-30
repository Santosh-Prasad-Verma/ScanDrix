package ingestion

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// WebhookAction categorizes normalized pull request actions.
type WebhookAction string

const (
	ActionOpened         WebhookAction = "OPENED"
	ActionSynchronize    WebhookAction = "SYNCHRONIZE"
	ActionReopened       WebhookAction = "REOPENED"
	ActionClosed         WebhookAction = "CLOSED"
	ActionCommentCreated WebhookAction = "COMMENT_CREATED"
	ActionIgnored        WebhookAction = "IGNORED"
)

// NormalizedWebhookEvent translates vendor-specific webhook payloads into a unified format.
type NormalizedWebhookEvent struct {
	ID                uuid.UUID          `json:"id"`
	WorkspaceID       uuid.UUID          `json:"workspace_id"`
	Provider          models.SCMProvider `json:"provider"`
	Action            WebhookAction      `json:"action"`
	RepoNamespace     string             `json:"repo_namespace"`
	PullRequestNumber int                `json:"pull_request_number"`
	Title             string             `json:"title"`
	HeadSHA           string             `json:"head_sha"`
	BaseSHA           string             `json:"base_sha"`
	Sender            string             `json:"sender"`
	CommentID         int64              `json:"comment_id,omitempty"`
	CommentBody       string             `json:"comment_body,omitempty"`
	CommentFilePath   string             `json:"comment_file_path,omitempty"`
	DiffHunk          string             `json:"diff_hunk,omitempty"`
	RawPayload        []byte             `json:"raw_payload"`
	ReceivedAt        time.Time          `json:"received_at"`
}

// IngestionResult reports the HTTP acceptance response.
type IngestionResult struct {
	Status     string    `json:"status"`
	EventID    uuid.UUID `json:"event_id"`
	Message    string    `json:"message"`
	ReceivedAt time.Time `json:"received_at"`
}

// SecretResolver retrieves the webhook verification secret for a given repository.
type SecretResolver interface {
	ResolveSecret(provider models.SCMProvider, repoNamespace string) (string, error)
}

// StaticSecretResolver provides a fixed secret map for testing and single-tenant setups.
type StaticSecretResolver struct {
	secrets map[string]string
}

// NewStaticSecretResolver initializes a fixed secret registry.
func NewStaticSecretResolver(secrets map[string]string) *StaticSecretResolver {
	return &StaticSecretResolver{secrets: secrets}
}

// ResolveSecret looks up the secret by provider:repo or provider default.
func (r *StaticSecretResolver) ResolveSecret(provider models.SCMProvider, repoNamespace string) (string, error) {
	key := string(provider) + ":" + repoNamespace
	if s, ok := r.secrets[key]; ok && s != "" {
		return s, nil
	}
	if s, ok := r.secrets[string(provider)]; ok && s != "" {
		return s, nil
	}
	return "", nil
}
