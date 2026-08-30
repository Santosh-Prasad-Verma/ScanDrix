package azuredevops_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/platform/azuredevops"
	"github.com/scandrix/backend/pkg/models"
)

func TestAzureDevOpsEndToEnd(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/pullrequests/99":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"pullRequestId": 99,
					"title": "Add KMS Encryption",
					"createdBy": {"uniqueName": "john.doe@corp.internal"},
					"lastMergeSourceCommit": {"commitId": "azure_head_sha"},
					"lastMergeTargetCommit": {"commitId": "azure_base_sha"},
					"sourceRefName": "refs/heads/feature/kms",
					"targetRefName": "refs/heads/main",
					"creationDate": "2026-08-30T00:00:00Z",
					"isDraft": false
				}`))
			} else if r.Method == http.MethodPatch {
				w.WriteHeader(http.StatusOK)
			}
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/pullrequests/99/iterations":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("diff --git a/kms.go b/kms.go\n+package kms"))
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/pullrequests/99/threads":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/commits/azure_head_sha/statuses":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/refs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value": [{"name": "refs/heads/main"}, {"name": "refs/heads/feature/kms"}]}`))
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/items":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("package kms\nfunc Encrypt() {}"))
		case r.URL.Path == "/my-project/_apis/git/repositories/my-repo/pullrequests/99/reviewers/@me":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := azuredevops.NewAdapter(server.URL, "azure_pat_token")

	if client.Provider() != models.ProviderAzure {
		t.Fatalf("unexpected provider: %s", client.Provider())
	}

	pr, err := client.FetchPullRequest(ctx, "my-project/my-repo", 99)
	if err != nil || pr.Number != 99 || pr.HeadSHA != "azure_head_sha" || pr.Author != "john.doe@corp.internal" {
		t.Fatalf("unexpected pr details: %+v, err: %v", pr, err)
	}

	diff, err := client.FetchDiff(ctx, "my-project/my-repo", 99)
	if err != nil || diff == "" {
		t.Fatalf("unexpected diff: %s, err: %v", diff, err)
	}

	err = client.PostInlineComments(ctx, "my-project/my-repo", 99, []platform.InlineCommentSpec{
		{FilePath: "kms.go", Line: 2, Body: "Ensure rotation interval check"},
	})
	if err != nil {
		t.Fatalf("failed to post inline comment: %v", err)
	}

	err = client.PostReviewSummary(ctx, "my-project/my-repo", 99, "KMS checks passed", platform.ConclusionSuccess)
	if err != nil {
		t.Fatalf("failed to post summary: %v", err)
	}

	err = client.SetCommitStatus(ctx, "my-project/my-repo", "azure_head_sha", "scandrix/gate", platform.StatusSuccess, "https://ci.example.com", "Passed")
	if err != nil {
		t.Fatalf("failed to set status: %v", err)
	}

	branches, err := client.ListBranches(ctx, "my-project/my-repo")
	if err != nil || len(branches) != 2 {
		t.Fatalf("unexpected branches: %v", branches)
	}

	content, err := client.GetFileContent(ctx, "my-project/my-repo", "azure_head_sha", "kms.go")
	if err != nil || len(content) == 0 {
		t.Fatalf("unexpected file content: %s", string(content))
	}

	err = client.ApprovePullRequest(ctx, "my-project/my-repo", 99, "Approved")
	if err != nil {
		t.Fatalf("failed to approve PR: %v", err)
	}

	err = client.MergePullRequest(ctx, "my-project/my-repo", 99, "squash")
	if err != nil {
		t.Fatalf("failed to merge PR: %v", err)
	}
}

func TestAzureDevOpsWebhookParsing(t *testing.T) {
	client := azuredevops.NewAdapter("https://dev.azure.com", "token")

	payload := []byte(`{
		"eventType": "git.pullrequest.created",
		"resource": {
			"pullRequestId": 105,
			"title": "Service Hook Test",
			"createdBy": {"uniqueName": "developer@corp.com"},
			"repository": {
				"name": "core-api",
				"project": {"name": "infrastructure"},
				"defaultBranch": "refs/heads/main"
			},
			"lastMergeSourceCommit": {"commitId": "sha_src_123"},
			"lastMergeTargetCommit": {"commitId": "sha_tgt_123"},
			"sourceRefName": "refs/heads/feat/azure",
			"targetRefName": "refs/heads/main",
			"creationDate": "2026-08-30T00:00:00Z",
			"isDraft": false
		}
	}`)

	event, err := client.ParseWebhookEvent("git.pullrequest.created", payload)
	if err != nil || event.Type != platform.WebhookEventPullRequest || event.PullRequest.Number != 105 {
		t.Fatalf("unexpected parsed event: %+v, err: %v", event, err)
	}
	if event.Repository != "infrastructure/core-api" {
		t.Fatalf("unexpected repo: %s", event.Repository)
	}
}
