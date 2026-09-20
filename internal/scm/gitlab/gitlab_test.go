package gitlab

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/pkg/models"
)

func TestGitLabClientWrapper(t *testing.T) {
	client := NewClient("https://gitlab.com", "dummy_token")
	if client.Provider() != models.ProviderGitLab {
		t.Fatalf("expected ProviderGitLab, got %v", client.Provider())
	}
	if client.AsAdapter() == nil {
		t.Fatalf("expected non-nil adapter")
	}

	ctx := context.Background()
	_, _ = client.FetchDiff(ctx, "gitlab-org/gitlab", 1)
	_ = client.PostInlineComments(ctx, "gitlab-org/gitlab", 1, []platform.InlineCommentSpec{})
	_ = client.PostReviewSummary(ctx, "gitlab-org/gitlab", 1, "test summary", platform.ConclusionSuccess)
	_ = client.SetCommitStatus(ctx, "gitlab-org/gitlab", "sha", "ci/scandrix", platform.StatusSuccess, "http://target", "desc")
}
