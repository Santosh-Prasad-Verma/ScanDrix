package bitbucket

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func init() {
	platform.RegisterAdapter(models.ProviderBitbucket, func(cfg platform.AdapterConfig) (platform.SCMAdapter, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.bitbucket.org/2.0"
		}
		return NewAdapter(baseURL, cfg.Token, cfg.Username), nil
	})
}

// NewAdapter creates an SCMAdapter for Bitbucket, automatically delegating to
// CloudClient or ServerClient based on whether the baseURL points to bitbucket.org or an on-prem server.
func NewAdapter(baseURL, token string, username ...string) platform.SCMAdapter {
	cleanURL := strings.TrimRight(baseURL, "/")
	if cleanURL == "" || strings.Contains(cleanURL, "api.bitbucket.org") {
		return NewCloudClient(cleanURL, token)
	}

	user := ""
	if len(username) > 0 {
		user = username[0]
	}
	return NewServerClient(cleanURL, token, user)
}

// Adapter satisfies platform.SCMAdapter wrapping either Cloud or Server implementation.
type Adapter struct {
	delegate platform.SCMAdapter
}

func (a *Adapter) Provider() models.SCMProvider {
	return a.delegate.Provider()
}

func (a *Adapter) FetchPullRequest(ctx context.Context, repo string, pullNumber int) (*platform.PullRequestDetails, error) {
	return a.delegate.FetchPullRequest(ctx, repo, pullNumber)
}

func (a *Adapter) FetchDiff(ctx context.Context, repo string, pullNumber int) (string, error) {
	return a.delegate.FetchDiff(ctx, repo, pullNumber)
}

func (a *Adapter) PostInlineComments(ctx context.Context, repo string, pullNumber int, comments []platform.InlineCommentSpec) error {
	return a.delegate.PostInlineComments(ctx, repo, pullNumber, comments)
}

func (a *Adapter) PostReviewSummary(ctx context.Context, repo string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	return a.delegate.PostReviewSummary(ctx, repo, pullNumber, summary, conclusion)
}

func (a *Adapter) SetCommitStatus(ctx context.Context, repo, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	return a.delegate.SetCommitStatus(ctx, repo, commitSHA, contextName, state, targetURL, description)
}

func (a *Adapter) ListBranches(ctx context.Context, repo string) ([]string, error) {
	return a.delegate.ListBranches(ctx, repo)
}

func (a *Adapter) GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error) {
	return a.delegate.GetFileContent(ctx, repo, ref, path)
}

func (a *Adapter) ApprovePullRequest(ctx context.Context, repo string, pullNumber int, message string) error {
	return a.delegate.ApprovePullRequest(ctx, repo, pullNumber, message)
}

func (a *Adapter) MergePullRequest(ctx context.Context, repo string, pullNumber int, mergeMethod string) error {
	return a.delegate.MergePullRequest(ctx, repo, pullNumber, mergeMethod)
}

func (a *Adapter) VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool {
	return a.delegate.VerifyWebhookSignature(secret, payload, signatureHeader)
}

func (a *Adapter) ParseWebhookEvent(eventType string, payload []byte) (*platform.WebhookEventData, error) {
	return a.delegate.ParseWebhookEvent(eventType, payload)
}
