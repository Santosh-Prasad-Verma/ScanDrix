package gitlab

import (
	"context"

	"github.com/scandrix/backend/internal/platform"
	gitlabAdapter "github.com/scandrix/backend/internal/platform/gitlab"
	"github.com/scandrix/backend/pkg/models"
)

// Client wraps the GitLab SCM adapter for review worker and orchestrator workflows.
type Client struct {
	adapter *gitlabAdapter.Adapter
}

// NewClient constructs a GitLab SCM client.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	return &Client{
		adapter: gitlabAdapter.NewAdapter(baseURL, token),
	}
}

// Provider returns models.ProviderGitLab.
func (c *Client) Provider() models.SCMProvider {
	return models.ProviderGitLab
}

// FetchDiff retrieves unified diff for a GitLab merge request.
func (c *Client) FetchDiff(ctx context.Context, repoNamespace string, pullNumber int) (string, error) {
	return c.adapter.FetchDiff(ctx, repoNamespace, pullNumber)
}

// PostInlineComments publishes inline discussion comments on the GitLab merge request.
func (c *Client) PostInlineComments(ctx context.Context, repoNamespace string, pullNumber int, comments []platform.InlineCommentSpec) error {
	return c.adapter.PostInlineComments(ctx, repoNamespace, pullNumber, comments)
}

// PostReviewSummary posts the overall review summary comment to the GitLab merge request.
func (c *Client) PostReviewSummary(ctx context.Context, repoNamespace string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	return c.adapter.PostReviewSummary(ctx, repoNamespace, pullNumber, summary, conclusion)
}

// SetCommitStatus posts pipeline / commit status to GitLab.
func (c *Client) SetCommitStatus(ctx context.Context, repoNamespace, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	return c.adapter.SetCommitStatus(ctx, repoNamespace, commitSHA, contextName, state, targetURL, description)
}

// AsAdapter exposes the underlying platform.SCMAdapter.
func (c *Client) AsAdapter() platform.SCMAdapter {
	return c.adapter
}
