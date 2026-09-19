package scm_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/scm"
	"github.com/scandrix/backend/pkg/models"
)

type mockSCMClient struct {
	provider models.SCMProvider
}

func (m *mockSCMClient) Provider() models.SCMProvider { return m.provider }
func (m *mockSCMClient) FetchDiff(ctx context.Context, repoNamespace string, pullNumber int) (string, error) {
	return "diff --git a/test.go b/test.go\n", nil
}
func (m *mockSCMClient) PostInlineComments(ctx context.Context, repoNamespace string, pullNumber int, comments []platform.InlineCommentSpec) error {
	return nil
}
func (m *mockSCMClient) PostReviewSummary(ctx context.Context, repoNamespace string, pullNumber int, summary string, conclusion platform.ReviewConclusion) error {
	return nil
}
func (m *mockSCMClient) SetCommitStatus(ctx context.Context, repoNamespace, commitSHA, contextName string, state platform.CommitStatusState, targetURL, description string) error {
	return nil
}
func (m *mockSCMClient) AsAdapter() platform.SCMAdapter { return nil }

func TestSCMRouter(t *testing.T) {
	router := scm.NewRouter()

	ghClient := &mockSCMClient{provider: models.ProviderGitHub}
	glClient := &mockSCMClient{provider: models.ProviderGitLab}

	router.Register(models.ProviderGitHub, ghClient)
	router.Register(models.ProviderGitLab, glClient)

	// 1. Resolve explicit GitLab provider
	c1, err := router.Resolve(models.ProviderGitLab, "gitlab.com/org/repo")
	if err != nil || c1.Provider() != models.ProviderGitLab {
		t.Fatalf("expected GitLab client, got error: %v", err)
	}

	// 2. Resolve explicit GitHub provider
	c2, err := router.Resolve(models.ProviderGitHub, "acme/repo")
	if err != nil || c2.Provider() != models.ProviderGitHub {
		t.Fatalf("expected GitHub client, got error: %v", err)
	}

	// 3. Resolve via namespace heuristic
	c3, err := router.Resolve("", "gitlab.acme.org/internal/service")
	if err != nil || c3.Provider() != models.ProviderGitLab {
		t.Fatalf("expected GitLab client from namespace heuristic, got error: %v", err)
	}
}
