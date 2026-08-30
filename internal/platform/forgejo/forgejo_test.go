package forgejo_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/platform/forgejo"
	"github.com/scandrix/backend/pkg/models"
)

func TestForgejoEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/org/repo/pulls/7":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"index": 7,
				"title": "Add AST Parser",
				"user": {"username": "forgejo_user"},
				"head": {"sha": "fg_head_sha", "ref": "feat-ast"},
				"base": {"sha": "fg_base_sha", "ref": "main"},
				"created_at": "2026-08-30T00:00:00Z",
				"draft": false
			}`))
		case r.URL.Path == "/api/v1/repos/org/repo/pulls/7.diff":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/ast.go b/ast.go\n+package ast"))
		case r.URL.Path == "/api/v1/repos/org/repo/pulls/7/reviews":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/repos/org/repo/statuses/fg_head_sha":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/api/v1/repos/org/repo/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name": "main"}, {"name": "feat-ast"}]`))
		case r.URL.Path == "/api/v1/repos/org/repo/raw/ast.go":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("package ast\nfunc Parse() {}"))
		case r.URL.Path == "/api/v1/repos/org/repo/pulls/7/merge":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := forgejo.NewAdapter(server.URL, "forgejo_secret_token")

	if client.Provider() != models.ProviderForgejo {
		t.Fatalf("unexpected provider: %s", client.Provider())
	}

	pr, err := client.FetchPullRequest(ctx, "org/repo", 7)
	if err != nil || pr.Number != 7 || pr.HeadSHA != "fg_head_sha" || pr.Author != "forgejo_user" {
		t.Fatalf("unexpected pr: %+v, err: %v", pr, err)
	}

	diff, err := client.FetchDiff(ctx, "org/repo", 7)
	if err != nil || diff == "" {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	err = client.PostInlineComments(ctx, "org/repo", 7, []platform.InlineCommentSpec{
		{FilePath: "ast.go", Line: 2, Body: "Scope tracking optimal"},
	})
	if err != nil {
		t.Fatalf("failed to post inline comments: %v", err)
	}

	err = client.PostReviewSummary(ctx, "org/repo", 7, "AST review passed", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed to post summary: %v", err)
	}

	err = client.SetCommitStatus(ctx, "org/repo", "fg_head_sha", "scandrix/gate", platform.StatusSuccess, "https://ci.example.com", "Passed")
	if err != nil {
		t.Fatalf("failed to set status: %v", err)
	}

	branches, err := client.ListBranches(ctx, "org/repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v", branches)
	}

	content, err := client.GetFileContent(ctx, "org/repo", "fg_head_sha", "ast.go")
	if err != nil || len(content) == 0 {
		t.Fatalf("unexpected file content: %s", string(content))
	}

	err = client.ApprovePullRequest(ctx, "org/repo", 7, "LGTM")
	if err != nil {
		t.Fatalf("failed to approve PR: %v", err)
	}

	err = client.MergePullRequest(ctx, "org/repo", 7, "rebase")
	if err != nil {
		t.Fatalf("failed to merge PR: %v", err)
	}
}

func TestForgejoWebhookParsing(t *testing.T) {
	client := forgejo.NewAdapter("https://codeberg.org", "token")

	payload := []byte(`{
		"action": "opened",
		"repository": {
			"full_name": "foss/project",
			"default_branch": "main"
		},
		"sender": {"username": "contributor"},
		"number": 88,
		"pull_request": {
			"number": 88,
			"title": "Enhance Parser",
			"user": {"username": "contributor"},
			"head": {"sha": "sha_fg_head", "ref": "feat-parse"},
			"base": {"sha": "sha_fg_base", "ref": "main"},
			"draft": false,
			"created_at": "2026-08-30T00:00:00Z"
		}
	}`)

	event, err := client.ParseWebhookEvent("pull_request", payload)
	if err != nil || event.Type != platform.WebhookEventPullRequest || event.PullRequest.Number != 88 {
		t.Fatalf("unexpected event: %+v, err: %v", event, err)
	}

	secret := "gitea_webhook_secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	if !client.VerifyWebhookSignature(secret, payload, validSig) {
		t.Fatalf("expected signature validation to pass")
	}

	if client.VerifyWebhookSignature(secret, payload, "invalid_sig") {
		t.Fatalf("expected signature validation to fail for bad signature")
	}
}
