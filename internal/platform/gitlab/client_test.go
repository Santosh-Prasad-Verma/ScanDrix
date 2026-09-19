package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/platform/gitlab"
	"github.com/scandrix/backend/pkg/models"
)

func TestGitLabAdapterOperations(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/merge_requests/10/raw_diff"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/pkg.go b/pkg.go\n+package pkg"))
		case strings.HasSuffix(p, "/merge_requests/10/discussions"):
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(p, "/merge_requests/10/notes"):
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(p, "/merge_requests/10/approve"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(p, "/merge_requests/10/merge"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(p, "/merge_requests/10"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"iid":              10,
				"title":            "Add feature",
				"author":           map[string]string{"username": "bob"},
				"sha":              "gitlabheadsha",
				"diff_refs":        map[string]string{"base_sha": "gitlabbasesha"},
				"source_branch":    "feature-x",
				"target_branch":    "main",
				"work_in_progress": false,
			})
		case strings.HasSuffix(p, "/statuses/gitlabheadsha"):
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(p, "/repository/branches"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "main"},
				{"name": "feature-x"},
			})
		case strings.Contains(p, "/repository/files/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("# ScanDrix GitLab Test"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	adapter := gitlab.NewAdapter(srv.URL, "glpat-secret-token")
	if adapter.Provider() != models.ProviderGitLab {
		t.Fatalf("expected ProviderGitLab, got: %s", adapter.Provider())
	}

	// 1. Fetch MR
	pr, err := adapter.FetchPullRequest(ctx, "group/repo", 10)
	if err != nil || pr.Number != 10 || pr.HeadSHA != "gitlabheadsha" {
		t.Fatalf("unexpected MR details: %+v, err: %v", pr, err)
	}

	// 2. Fetch Diff
	diff, err := adapter.FetchDiff(ctx, "group/repo", 10)
	if err != nil || len(diff) == 0 {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	// 3. Post Inline Comments & Review Summary
	err = adapter.PostInlineComments(ctx, "group/repo", 10, []platform.InlineCommentSpec{
		{FilePath: "pkg.go", Line: 5, Body: "Inline comment"},
	})
	if err != nil {
		t.Fatalf("failed posting inline comments: %v", err)
	}

	err = adapter.PostReviewSummary(ctx, "group/repo", 10, "Summary passed", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed posting review summary: %v", err)
	}

	// 4. Status, Branches & File
	err = adapter.SetCommitStatus(ctx, "group/repo", "gitlabheadsha", "scandrix/review", platform.StatusSuccess, "https://scandrix.dev", "OK")
	if err != nil {
		t.Fatalf("failed setting commit status: %v", err)
	}

	branches, err := adapter.ListBranches(ctx, "group/repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v, err: %v", branches, err)
	}

	content, err := adapter.GetFileContent(ctx, "group/repo", "main", "README.md")
	if err != nil || string(content) != "# ScanDrix GitLab Test" {
		t.Fatalf("unexpected file content: %s, err: %v", string(content), err)
	}

	err = adapter.ApprovePullRequest(ctx, "group/repo", 10, "Approved")
	if err != nil {
		t.Fatalf("failed approving MR: %v", err)
	}

	err = adapter.MergePullRequest(ctx, "group/repo", 10, "squash")
	if err != nil {
		t.Fatalf("failed merging MR: %v", err)
	}

	// 5. Webhook token validation & event parsing
	secret := "gitlab-secret-token"
	payload := []byte(`{"object_kind":"merge_request","object_attributes":{"iid":10,"title":"MR title","last_commit":{"id":"c123"},"source_branch":"feat","target_branch":"main","action":"open"},"project":{"path_with_namespace":"group/repo","default_branch":"main"},"user":{"username":"bob"}}`)

	if !adapter.VerifyWebhookSignature(secret, payload, secret) {
		t.Fatal("expected valid GitLab secret token")
	}

	evt, err := adapter.ParseWebhookEvent("Merge Request Hook", payload)
	if err != nil || evt.Type != platform.WebhookEventPullRequest || evt.Repository != "group/repo" {
		t.Fatalf("unexpected parsed GitLab event: %+v, err: %v", evt, err)
	}
}
