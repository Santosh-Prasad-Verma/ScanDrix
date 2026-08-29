package platform_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	_ "github.com/scandrix/backend/internal/platform/azuredevops"
	_ "github.com/scandrix/backend/internal/platform/bitbucket"
	_ "github.com/scandrix/backend/internal/platform/forgejo"
	_ "github.com/scandrix/backend/internal/platform/github"
	_ "github.com/scandrix/backend/internal/platform/gitlab"
	"github.com/scandrix/backend/pkg/models"
)

func TestSCMAdaptersEndToEnd(t *testing.T) {
	ctx := context.Background()

	// 1. Mock GitHub API Server
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/repo/pulls/1" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/a.go b/a.go\n+package a"))
		case r.URL.Path == "/repos/acme/repo/pulls/1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"number": 1,
				"title": "Test PR",
				"user": {"login": "octocat"},
				"head": {"sha": "head123", "ref": "feat"},
				"base": {"sha": "base123", "ref": "main"},
				"draft": false
			}`))
		case r.URL.Path == "/repos/acme/repo/pulls/1/reviews":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/repos/acme/repo/statuses/head123":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/repos/acme/repo/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"main"},{"name":"dev"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ghServer.Close()

	// 2. Test GitHub Adapter
	ghAdapter, err := platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderGitHub,
		BaseURL:  ghServer.URL,
		Token:    "ghp_fake_test_token_12345",
	})
	if err != nil {
		t.Fatalf("failed to create github adapter: %v", err)
	}

	prDetails, err := ghAdapter.FetchPullRequest(ctx, "acme/repo", 1)
	if err != nil || prDetails.Number != 1 || prDetails.HeadSHA != "head123" {
		t.Fatalf("unexpected pr details: %+v, err: %v", prDetails, err)
	}

	diffContent, err := ghAdapter.FetchDiff(ctx, "acme/repo", 1)
	if err != nil || diffContent == "" {
		t.Fatalf("unexpected diff: %s, err: %v", diffContent, err)
	}

	err = ghAdapter.PostInlineComments(ctx, "acme/repo", 1, []platform.InlineCommentSpec{
		{FilePath: "a.go", Line: 1, Body: "Great addition!"},
	})
	if err != nil {
		t.Fatalf("failed to post inline comments: %v", err)
	}

	err = ghAdapter.PostReviewSummary(ctx, "acme/repo", 1, "Overall good", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed to post review summary: %v", err)
	}

	err = ghAdapter.SetCommitStatus(ctx, "acme/repo", "head123", "scandrix/gate", platform.StatusSuccess, "https://ci.acme.com", "Passing")
	if err != nil {
		t.Fatalf("failed to set commit status: %v", err)
	}

	branches, err := ghAdapter.ListBranches(ctx, "acme/repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v", branches)
	}

	// 3. Test GitLab Adapter Factory
	glAdapter, err := platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderGitLab,
		BaseURL:  "https://gitlab.example.com",
		Token:    "glpat-fake-token",
	})
	if err != nil || glAdapter.Provider() != models.ProviderGitLab {
		t.Fatalf("failed to create gitlab adapter: %v", err)
	}

	// 4. Test Bitbucket Adapter Factory
	bbAdapter, err := platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderBitbucket,
		BaseURL:  "https://api.bitbucket.org/2.0",
		Token:    "bb_token",
	})
	if err != nil || bbAdapter.Provider() != models.ProviderBitbucket {
		t.Fatalf("failed to create bitbucket adapter: %v", err)
	}

	// 5. Test Azure DevOps Adapter Factory
	azAdapter, err := platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderAzure,
		BaseURL:  "https://dev.azure.com/acme",
		Token:    "azure_pat_token",
	})
	if err != nil || azAdapter.Provider() != models.ProviderAzure {
		t.Fatalf("failed to create azure devops adapter: %v", err)
	}

	// 6. Test Forgejo Adapter Factory
	fgAdapter, err := platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderForgejo,
		BaseURL:  "https://codeberg.org",
		Token:    "forgejo_token",
	})
	if err != nil || fgAdapter.Provider() != models.ProviderForgejo {
		t.Fatalf("failed to create forgejo adapter: %v", err)
	}

	// 7. Verify Missing Token Fails
	_, err = platform.NewAdapter(platform.AdapterConfig{
		Provider: models.ProviderGitHub,
		Token:    "",
	})
	if err == nil {
		t.Fatalf("expected error on missing token, got nil")
	}
}
