package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// ReviewConclusion reflects the final decision on a code review.
type ReviewConclusion string

const (
	ConclusionSuccess ReviewConclusion = "SUCCESS"
	ConclusionFailure ReviewConclusion = "FAILURE"
	ConclusionNeutral ReviewConclusion = "NEUTRAL"
)

// CommitStatusState reports CI status check states.
type CommitStatusState string

const (
	StatusPending CommitStatusState = "PENDING"
	StatusSuccess CommitStatusState = "SUCCESS"
	StatusFailure CommitStatusState = "FAILURE"
	StatusError   CommitStatusState = "ERROR"
)

// PullRequestDetails aggregates core metadata across VCS providers.
type PullRequestDetails struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	HeadSHA      string    `json:"head_sha"`
	BaseSHA      string    `json:"base_sha"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	CreatedAt    time.Time `json:"created_at"`
	IsDraft      bool      `json:"is_draft"`
}

// InlineCommentSpec defines a line-targeted review comment.
type InlineCommentSpec struct {
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	StartLine int    `json:"start_line,omitempty"`
	Body      string `json:"body"`
}

// WebhookEventType identifies normalized event kinds across all SCMs.
type WebhookEventType string

const (
	WebhookEventPullRequest        WebhookEventType = "pull_request"
	WebhookEventPush               WebhookEventType = "push"
	WebhookEventIssueComment       WebhookEventType = "issue_comment"
	WebhookEventReviewComment      WebhookEventType = "review_comment"
	WebhookEventCommitStatus       WebhookEventType = "commit_status"
	WebhookEventPing               WebhookEventType = "ping"
)

// WebhookEventData represents a normalized event across GitHub, GitLab, Bitbucket, Azure DevOps, and Forgejo.
type WebhookEventData struct {
	Type         WebhookEventType   `json:"type"`
	Action       string             `json:"action"` // opened, synchronize, closed, edited, created
	Repository   string             `json:"repository"`
	DefaultBranch string            `json:"default_branch"`
	PullRequest  *PullRequestDetails `json:"pull_request,omitempty"`
	CommitSHA    string             `json:"commit_sha,omitempty"`
	Sender       string             `json:"sender,omitempty"`
	CommentBody  string             `json:"comment_body,omitempty"`
	CommentID    int64              `json:"comment_id,omitempty"`
	RawPayload   json.RawMessage    `json:"raw_payload,omitempty"`
}

// SCMAdapter defines the unified interface across all SCM platforms (GitHub, GitLab, Bitbucket, Azure, Forgejo).
type SCMAdapter interface {
	Provider() models.SCMProvider
	FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*PullRequestDetails, error)
	FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error)
	PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []InlineCommentSpec) error
	PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion ReviewConclusion) error
	SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state CommitStatusState, targetURL, description string) error
	ListBranches(ctx context.Context, repo string) ([]string, error)
	GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error)
	ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error
	MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error
	VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool
	ParseWebhookEvent(eventType string, payload []byte) (*WebhookEventData, error)
}

// AdapterConfig holds credentials and endpoints to construct an SCM adapter.
type AdapterConfig struct {
	Provider      models.SCMProvider
	BaseURL       string
	Token         string
	Username      string // Optional, e.g. for Bitbucket or Azure DevOps basic auth
	WebhookSecret string
}

// Registry stores and instantiates SCM adapters by provider.
type FactoryFunc func(cfg AdapterConfig) (SCMAdapter, error)

var adapterRegistry = make(map[models.SCMProvider]FactoryFunc)

// RegisterAdapter associates a provider with an adapter constructor.
func RegisterAdapter(provider models.SCMProvider, factory FactoryFunc) {
	adapterRegistry[provider] = factory
}

// NewAdapter instantiates an SCM adapter for the given provider.
func NewAdapter(cfg AdapterConfig) (SCMAdapter, error) {
	factory, ok := adapterRegistry[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported SCM provider: %s", cfg.Provider)
	}
	if cfg.Token == "" {
		return nil, errors.New("SCM authentication token is required")
	}
	return factory(cfg)
}
