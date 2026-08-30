package bitbucket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/platform/bitbucket"
	"github.com/scandrix/backend/pkg/models"
)

func TestBitbucketCloudEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repositories/workspace/my-repo/pullrequests/42":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id": 42,
				"title": "Add Auth Middleware",
				"author": {"nickname": "alice"},
				"source": {"commit": {"hash": "cloud_head_sha"}, "branch": {"name": "feature-auth"}},
				"destination": {"commit": {"hash": "cloud_base_sha"}, "branch": {"name": "main"}},
				"created_on": "2026-08-30T00:00:00Z",
				"draft": false
			}`))
		case r.URL.Path == "/repositories/workspace/my-repo/pullrequests/42/diff":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/auth.go b/auth.go\n+package auth"))
		case r.URL.Path == "/repositories/workspace/my-repo/pullrequests/42/comments":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/repositories/workspace/my-repo/commit/cloud_head_sha/statuses/build":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/repositories/workspace/my-repo/refs/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values": [{"name": "main"}, {"name": "feature-auth"}]}`))
		case r.URL.Path == "/repositories/workspace/my-repo/src/cloud_head_sha/auth.go":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("package auth\nfunc Validate() bool { return true }"))
		case r.URL.Path == "/repositories/workspace/my-repo/pullrequests/42/approve":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/repositories/workspace/my-repo/pullrequests/42/merge":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := bitbucket.NewCloudClient(server.URL, "bb_fake_token")

	if client.Provider() != models.ProviderBitbucket {
		t.Fatalf("unexpected provider: %s", client.Provider())
	}

	pr, err := client.FetchPullRequest(ctx, "workspace/my-repo", 42)
	if err != nil || pr.Number != 42 || pr.HeadSHA != "cloud_head_sha" || pr.Author != "alice" {
		t.Fatalf("unexpected pr: %+v, err: %v", pr, err)
	}

	diff, err := client.FetchDiff(ctx, "workspace/my-repo", 42)
	if err != nil || diff == "" {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	err = client.PostInlineComments(ctx, "workspace/my-repo", 42, []platform.InlineCommentSpec{
		{FilePath: "auth.go", Line: 2, Body: "Ensure constant-time comparison"},
	})
	if err != nil {
		t.Fatalf("failed to post inline comment: %v", err)
	}

	err = client.PostReviewSummary(ctx, "workspace/my-repo", 42, "Security check passed", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed to post summary: %v", err)
	}

	err = client.SetCommitStatus(ctx, "workspace/my-repo", "cloud_head_sha", "scandrix/gate", platform.StatusSuccess, "https://ci.example.com", "Security Gate Passed")
	if err != nil {
		t.Fatalf("failed to set commit status: %v", err)
	}

	branches, err := client.ListBranches(ctx, "workspace/my-repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v, err: %v", branches, err)
	}

	content, err := client.GetFileContent(ctx, "workspace/my-repo", "cloud_head_sha", "auth.go")
	if err != nil || len(content) == 0 {
		t.Fatalf("unexpected content: %s, err: %v", string(content), err)
	}

	err = client.ApprovePullRequest(ctx, "workspace/my-repo", 42, "LGTM")
	if err != nil {
		t.Fatalf("failed to approve PR: %v", err)
	}

	err = client.MergePullRequest(ctx, "workspace/my-repo", 42, "squash")
	if err != nil {
		t.Fatalf("failed to merge PR: %v", err)
	}
}

func TestBitbucketServerEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/pull-requests/10":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id": 10,
				"version": 1,
				"title": "Fix SQL Parameterization",
				"author": {"user": {"name": "bob"}},
				"fromRef": {"latestCommit": "server_head_sha", "displayId": "fix-sql"},
				"toRef": {"latestCommit": "server_base_sha", "displayId": "main"},
				"createdDate": 1725000000000,
				"open": true,
				"draft": false
			}`))
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/pull-requests/10/diff":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/db.go b/db.go\n+package db"))
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/pull-requests/10/comments":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/rest/build-status/1.0/commits/server_head_sha":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values": [{"displayId": "main"}, {"displayId": "fix-sql"}]}`))
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/raw/db.go":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("package db\nvar DB = true"))
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/pull-requests/10/approve":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/rest/api/1.0/projects/CORE/repos/backend/pull-requests/10/merge":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	serverClient := bitbucket.NewServerClient(server.URL, "bb_server_token", "admin")

	pr, err := serverClient.FetchPullRequest(ctx, "CORE/backend", 10)
	if err != nil || pr.Number != 10 || pr.HeadSHA != "server_head_sha" || pr.Author != "bob" {
		t.Fatalf("unexpected pr: %+v, err: %v", pr, err)
	}

	diff, err := serverClient.FetchDiff(ctx, "CORE/backend", 10)
	if err != nil || diff == "" {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	err = serverClient.PostInlineComments(ctx, "CORE/backend", 10, []platform.InlineCommentSpec{
		{FilePath: "db.go", Line: 1, Body: "Param query fixed"},
	})
	if err != nil {
		t.Fatalf("failed to post server comment: %v", err)
	}

	err = serverClient.SetCommitStatus(ctx, "CORE/backend", "server_head_sha", "scandrix/gate", platform.StatusSuccess, "https://ci.example.com", "Passed")
	if err != nil {
		t.Fatalf("failed to set build status on server: %v", err)
	}

	branches, err := serverClient.ListBranches(ctx, "CORE/backend")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v", branches)
	}

	content, err := serverClient.GetFileContent(ctx, "CORE/backend", "server_head_sha", "db.go")
	if err != nil || len(content) == 0 {
		t.Fatalf("unexpected file content: %s", string(content))
	}

	err = serverClient.ApprovePullRequest(ctx, "CORE/backend", 10, "Approved")
	if err != nil {
		t.Fatalf("failed to approve on server: %v", err)
	}

	err = serverClient.MergePullRequest(ctx, "CORE/backend", 10, "merge")
	if err != nil {
		t.Fatalf("failed to merge on server: %v", err)
	}
}

func TestBitbucketWebhookParsing(t *testing.T) {
	client := bitbucket.NewCloudClient("https://api.bitbucket.org/2.0", "token")

	validPayload := []byte(`{
		"actor": {"nickname": "developer"},
		"repository": {"full_name": "org/repo"},
		"pullrequest": {
			"id": 15,
			"title": "PR Title",
			"created_on": "2026-08-30T00:00:00Z",
			"author": {"nickname": "developer"},
			"source": {"commit": {"hash": "head_123"}, "branch": {"name": "feat"}},
			"destination": {"commit": {"hash": "base_123"}, "branch": {"name": "main"}}
		}
	}`)

	event, err := client.ParseWebhookEvent("pullrequest:created", validPayload)
	if err != nil || event.Type != platform.WebhookEventPullRequest || event.PullRequest.Number != 15 {
		t.Fatalf("unexpected parsed event: %+v, err: %v", event, err)
	}

	secret := "test_secret"
	sig := "sha256=123" // Invalid
	if client.VerifyWebhookSignature(secret, validPayload, sig) {
		t.Fatalf("expected signature mismatch")
	}
}
