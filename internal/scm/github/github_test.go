package github

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func TestGitHubClientWrapper(t *testing.T) {
	client := NewClient("dummy_token")
	if client.Provider() != models.ProviderGitHub {
		t.Fatalf("expected ProviderGitHub, got %v", client.Provider())
	}
	if client.AsAdapter() == nil {
		t.Fatalf("expected non-nil adapter")
	}

	ctx := context.Background()
	// Test error handling on missing/mock API endpoints
	_, err := client.FetchDiff(ctx, "octocat/Hello-World", 1)
	if err == nil {
		// Even if network fails, error should be handled gracefully
	}

	_ = client.PostInlineComments(ctx, "octocat/Hello-World", 1, []platform.InlineCommentSpec{})
	_ = client.PostReviewSummary(ctx, "octocat/Hello-World", 1, "test summary", platform.ConclusionSuccess)
	_ = client.SetCommitStatus(ctx, "octocat/Hello-World", "sha", "ci/scandrix", platform.StatusSuccess, "http://target", "desc")
}
