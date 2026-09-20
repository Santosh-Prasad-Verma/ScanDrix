package github

import (
	"context"

	"github.com/scandrix/backend/internal/platform"
	githubAdapter "github.com/scandrix/backend/internal/platform/github"
	"github.com/scandrix/backend/pkg/models"
)

// Client wraps the GitHub SCM adapter for review worker and orchestrator workflows.
type Client struct {
	adapter *githubAdapter.Adapter
}

// NewClient constructs a GitHub SCM client.
func NewClient(token string) *Client {
	return &Client{
		adapter: githubAdapter.NewAdapter("https://api.github.com", token),
	}
}

// Provider returns models.ProviderGitHub.
func (c *Client) Provider() models.SCMProvider {
	return models.ProviderGitHub
}

// FetchDiff retrieves unified diff for a GitHub pull request.
func (c *Client) FetchDiff(ctx context.Context, repoNamespace string, pullNumber int) (string, error) {
	return c.adapter.FetchDiff(ctx, repoNamespace, pullNumber)
}

// PostInlineComments publishes inline review comments on the GitHub pull request.
func (c *Client) PostInlineComments(ctx context.Context, repoNamespace string, pullNumber int, comments []platform.InlineCommentSpec) error {
	return c.adapter.PostInlineComments(ctx, repoNamespace, pullNumber, comments)
}

// PostReviewSummary posts the overall review summary comment to the GitHub pull request.
func (c *Client) PostReviewSummary(ctx context.Context, repoNamespace string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	return c.adapter.PostReviewSummary(ctx, repoNamespace, pullNumber, summary, conclusion)
}

// SetCommitStatus posts commit status / check to GitHub.
func (c *Client) SetCommitStatus(ctx context.Context, repoNamespace, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	return c.adapter.SetCommitStatus(ctx, repoNamespace, commitSHA, contextName, state, targetURL, description)
}

// AsAdapter exposes the underlying platform.SCMAdapter.
func (c *Client) AsAdapter() platform.SCMAdapter {
	return c.adapter
}
