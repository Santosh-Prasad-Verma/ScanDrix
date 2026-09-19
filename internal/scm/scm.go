package scm

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

// SCMClient defines the unified contract for SCM provider interactions during review execution.
type SCMClient interface {
	Provider() models.SCMProvider
	FetchDiff(ctx context.Context, repoNamespace string, pullNumber int) (string, error)
	PostInlineComments(ctx context.Context, repoNamespace string, pullNumber int, comments []platform.InlineCommentSpec) error
	PostReviewSummary(ctx context.Context, repoNamespace string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error
	SetCommitStatus(ctx context.Context, repoNamespace, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error
	AsAdapter() platform.SCMAdapter
}

// Router dynamically routes VCS operations to provider-specific clients.
type Router struct {
	clients map[models.SCMProvider]SCMClient
}

// NewRouter constructs an SCM router.
func NewRouter() *Router {
	return &Router{
		clients: make(map[models.SCMProvider]SCMClient),
	}
}

// Register attaches an SCM client for a given provider.
func (r *Router) Register(provider models.SCMProvider, client SCMClient) {
	r.clients[provider] = client
}

// Resolve looks up the registered client based on explicit provider or namespace heuristics.
func (r *Router) Resolve(provider models.SCMProvider, repoNamespace string) (SCMClient, error) {
	if client, ok := r.clients[provider]; ok && client != nil {
		return client, nil
	}

	lower := strings.ToLower(repoNamespace)
	if strings.Contains(lower, "gitlab") {
		if client, ok := r.clients[models.ProviderGitLab]; ok && client != nil {
			return client, nil
		}
	}

	if strings.Contains(lower, "bitbucket") {
		if client, ok := r.clients[models.ProviderBitbucket]; ok && client != nil {
			return client, nil
		}
	}

	if client, ok := r.clients[models.ProviderGitHub]; ok && client != nil {
		return client, nil
	}

	return nil, fmt.Errorf("no SCM client available for provider %q and repo %q", provider, repoNamespace)
}
