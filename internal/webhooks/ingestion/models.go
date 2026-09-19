package ingestion

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// WebhookAction categorizes normalized pull request and installation lifecycle actions.
type WebhookAction string

const (
	ActionOpened              WebhookAction = "OPENED"
	ActionSynchronize         WebhookAction = "SYNCHRONIZE"
	ActionReopened            WebhookAction = "REOPENED"
	ActionClosed              WebhookAction = "CLOSED"
	ActionCommentCreated      WebhookAction = "COMMENT_CREATED"
	ActionInstallationCreated WebhookAction = "INSTALLATION_CREATED"
	ActionInstallationDeleted WebhookAction = "INSTALLATION_DELETED"
	ActionReposAdded          WebhookAction = "REPOS_ADDED"
	ActionReposRemoved        WebhookAction = "REPOS_REMOVED"
	ActionFeedbackDismissed   WebhookAction = "FEEDBACK_DISMISSED"
	ActionIgnored             WebhookAction = "IGNORED"
)

// NormalizedWebhookEvent translates vendor-specific webhook payloads into a unified format.
type NormalizedWebhookEvent struct {
	ID                uuid.UUID          `json:"id"`
	TaskID            uuid.UUID          `json:"task_id,omitempty"`
	EventID           uuid.UUID          `json:"event_id,omitempty"`
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
	DismissalReason   string             `json:"dismissal_reason,omitempty"`
	InstallationID    int64              `json:"installation_id,omitempty"`
	Repositories      []string           `json:"repositories,omitempty"`
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
