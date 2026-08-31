package github_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/platform/github"
	"github.com/scandrix/backend/pkg/models"
)

func TestGitHubAdapterOperations(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/org/repo/pulls/42" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/main.go b/main.go\n+package main"))
		case r.URL.Path == "/repos/org/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 42,
				"title":  "Fix critical flaw",
				"user":   map[string]string{"login": "alice"},
				"head":   map[string]string{"sha": "headsha42", "ref": "feat/patch"},
				"base":   map[string]string{"sha": "basesha42", "ref": "main"},
				"draft":  false,
			})
		case r.URL.Path == "/repos/org/repo/pulls/42/reviews":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/repos/org/repo/pulls/42/comments":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/repos/org/repo/statuses/headsha42":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/repos/org/repo/branches":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "main"},
				{"name": "develop"},
			})
		case r.URL.Path == "/repos/org/repo/contents/config.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": "eyJzdGF0dXMiOiJvazJ9", // base64 for {"status":"ok2"}
			})
		case r.URL.Path == "/repos/org/repo/pulls/42/merge":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	adapter := github.NewAdapter(srv.URL, "ghp_test_token")
	if adapter.Provider() != models.ProviderGitHub {
		t.Fatalf("expected ProviderGitHub, got: %s", adapter.Provider())
	}

	// 1. Fetch PR
	pr, err := adapter.FetchPullRequest(ctx, "org/repo", 42)
	if err != nil || pr.Number != 42 || pr.HeadSHA != "headsha42" {
		t.Fatalf("unexpected PR details: %+v, err: %v", pr, err)
	}

	// 2. Fetch Diff
	diff, err := adapter.FetchDiff(ctx, "org/repo", 42)
	if err != nil || len(diff) == 0 {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	// 3. Post Inline Comments & Review Summary
	err = adapter.PostInlineComments(ctx, "org/repo", 42, []platform.InlineCommentSpec{
		{FilePath: "main.go", Line: 10, Body: "LGTM"},
	})
	if err != nil {
		t.Fatalf("failed posting inline comments: %v", err)
	}

	err = adapter.PostReviewSummary(ctx, "org/repo", 42, "Review completed", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed posting review summary: %v", err)
	}

	// 4. Set Status & Branches
	err = adapter.SetCommitStatus(ctx, "org/repo", "headsha42", "scandrix/review", platform.StatusSuccess, "https://scandrix.dev", "Passed")
	if err != nil {
		t.Fatalf("failed setting commit status: %v", err)
	}

	branches, err := adapter.ListBranches(ctx, "org/repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v, err: %v", branches, err)
	}

	// 5. File Content & Merge
	content, err := adapter.GetFileContent(ctx, "org/repo", "main", "config.json")
	if err != nil || len(content) == 0 {
		t.Fatalf("unexpected file content: %s, err: %v", string(content), err)
	}

	err = adapter.ApprovePullRequest(ctx, "org/repo", 42, "Auto-approved")
	if err != nil {
		t.Fatalf("failed approving PR: %v", err)
	}

	err = adapter.MergePullRequest(ctx, "org/repo", 42, "squash")
	if err != nil {
		t.Fatalf("failed merging PR: %v", err)
	}

	// 6. Webhook verification and parsing
	secret := "secret-webhook-key"
	payload := []byte(`{"action":"opened","pull_request":{"number":42,"title":"Test","user":{"login":"alice"},"head":{"sha":"h1","ref":"f"},"base":{"sha":"b1","ref":"m"}},"repository":{"full_name":"org/repo","default_branch":"main"}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !adapter.VerifyWebhookSignature(secret, payload, sig) {
		t.Fatal("expected valid webhook signature")
	}

	evt, err := adapter.ParseWebhookEvent("pull_request", payload)
	if err != nil || evt.Type != platform.WebhookEventPullRequest || evt.Repository != "org/repo" {
		t.Fatalf("unexpected parsed webhook event: %+v, err: %v", evt, err)
	}
}
