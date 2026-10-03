// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

func setupMockForgejoServer(t *testing.T) (*httptest.Server, *ForgejoService) {
	mux := http.NewServeMux()

	// GET /api/v1/user
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        101,
			"login":     "scandrix-bot",
			"full_name": "ScanDrix Reviewer",
			"email":     "drixy@scandrix.dev",
		})
	})

	// GET /api/v1/repos/org/repo/pulls/10
	mux.HandleFunc("/api/v1/repos/org/repo/pulls/10", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 10, "body": "Updated description"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         5001,
			"number":     10,
			"title":      "Feature: Secure Token Handling",
			"body":       "Adds secure token rotation",
			"state":      "open",
			"draft":      false,
			"html_url":   "https://codeberg.org/org/repo/pulls/10",
			"created_at": time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
			"user": map[string]any{
				"id":        42,
				"login":     "octocat",
				"full_name": "Octo Cat",
			},
			"head": map[string]any{
				"ref": "feature/tokens",
				"sha": "fedcba9876543210",
				"repo": map[string]any{
					"id":             200,
					"name":           "repo",
					"full_name":      "org/repo",
					"default_branch": "main",
				},
			},
			"base": map[string]any{
				"ref": "main",
				"sha": "0123456789abcdef",
				"repo": map[string]any{
					"id":             200,
					"name":           "repo",
					"full_name":      "org/repo",
					"default_branch": "main",
				},
			},
		})
	})

	// GET /api/v1/repos/org/repo/pulls/10/files
	mux.HandleFunc("/api/v1/repos/org/repo/pulls/10/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"filename":  "auth/token.go",
				"status":    "modified",
				"additions": 15,
				"deletions": 3,
				"changes":   18,
			},
		})
	})

	// GET /api/v1/repos/org/repo/pulls/10.diff
	mux.HandleFunc("/api/v1/repos/org/repo/pulls/10.diff", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		diffContent := `diff --git a/auth/token.go b/auth/token.go
index 1234567..89abcdef 100644
--- a/auth/token.go
+++ b/auth/token.go
@@ -10,3 +10,4 @@ func Validate() bool {
+    // ScanDrix review: check expiration
     return true
 }
`
		_, _ = w.Write([]byte(diffContent))
	})

	// POST /api/v1/repos/org/repo/pulls/10/reviews
	mux.HandleFunc("/api/v1/repos/org/repo/pulls/10/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		event, _ := body["event"].(string)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           8881,
			"state":        event,
			"submitted_at": time.Now().Format(time.RFC3339),
			"comments": []map[string]any{
				{
					"id":         9991,
					"body":       "Review comment body",
					"path":       "auth/token.go",
					"created_at": time.Now().Format(time.RFC3339),
				},
			},
		})
	})

	// POST /api/v1/repos/org/repo/issues/10/comments
	mux.HandleFunc("/api/v1/repos/org/repo/issues/10/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         7771,
			"body":       "General issue comment",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		})
	})

	// POST /api/v1/repos/org/repo/pulls/10/merge
	mux.HandleFunc("/api/v1/repos/org/repo/pulls/10/merge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
	})

	// POST /api/v1/repos/org/repo/statuses/0123456789abcdef
	mux.HandleFunc("/api/v1/repos/org/repo/statuses/0123456789abcdef", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          3331,
			"state":       "success",
			"context":     CommitStatusContext,
			"description": "Code Review Complete",
		})
	})

	server := httptest.NewServer(mux)
	svc := NewForgejoService(ForgejoServiceConfig{
		BaseURL:        server.URL,
		HTTPClient:     server.Client(),
		DefaultTimeout: 5 * time.Second,
	})

	return server, svc
}

func TestForgejoService_Provider(t *testing.T) {
	svc := NewForgejoService(ForgejoServiceConfig{})
	if svc.Provider() != models.ProviderForgejo {
		t.Fatalf("expected ProviderForgejo, got %v", svc.Provider())
	}
}

func TestForgejoService_VerifyConnection(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "mock-token-123",
	}

	status, err := svc.VerifyConnection(ctx, orgData)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if !status.HasConnection || !status.IsConnected {
		t.Fatalf("expected connected status, got %+v", status)
	}
	if !strings.Contains(status.Message, "scandrix-bot") {
		t.Fatalf("expected user login in message, got: %s", status.Message)
	}
}

func TestForgejoService_GetPullRequest(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-1",
		AuthToken:      "mock-token-123",
	}
	repo := &types.RepositoryDescriptor{
		Owner: "org",
		Name:  "repo",
	}

	pr, err := svc.GetPullRequest(ctx, orgData, repo, 10)
	if err != nil {
		t.Fatalf("GetPullRequest failed: %v", err)
	}
	if pr == nil {
		t.Fatal("expected pull request, got nil")
	}
	if pr.Number != 10 {
		t.Fatalf("expected PR 10, got %d", pr.Number)
	}
	if pr.State != "open" {
		t.Fatalf("expected state 'open', got %s", pr.State)
	}
	if pr.Head.SHA != "fedcba9876543210" {
		t.Fatalf("expected head SHA, got %s", pr.Head.SHA)
	}
}

func TestForgejoService_GetFilesByPullRequestId(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "mock-token-123",
	}
	repo := &types.RepositoryDescriptor{
		Owner: "org",
		Name:  "repo",
	}

	files, err := svc.GetFilesByPullRequestId(ctx, orgData, repo, 10)
	if err != nil {
		t.Fatalf("GetFilesByPullRequestId failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Filename != "auth/token.go" {
		t.Fatalf("expected auth/token.go, got %s", files[0].Filename)
	}
	if !strings.Contains(files[0].Patch, "ScanDrix review") {
		t.Fatalf("expected parsed unified diff patch, got: %s", files[0].Patch)
	}
}

func TestForgejoService_CreateReviewComment(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "mock-token-123",
	}
	repo := &types.RepositoryDescriptor{
		Owner: "org",
		Name:  "repo",
	}

	comment := types.PullRequestReviewComment{
		Path: "auth/token.go",
		Line: 12,
		Body: "Consider using constant time compare here",
	}

	created, err := svc.CreateReviewComment(ctx, orgData, repo, 10, comment)
	if err != nil {
		t.Fatalf("CreateReviewComment failed: %v", err)
	}
	if created == nil || created.ID != "9991" {
		t.Fatalf("expected comment ID 9991, got %+v", created)
	}
	if !strings.Contains(created.Body, "drixy-codereview") {
		t.Fatalf("expected drixy review marker in body, got: %s", created.Body)
	}
}

func TestForgejoService_ApproveAndRequestChanges(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "mock-token-123",
	}
	repo := &types.RepositoryDescriptor{
		Owner: "org",
		Name:  "repo",
	}

	if err := svc.ApprovePullRequest(ctx, orgData, repo, 10, "LGTM!"); err != nil {
		t.Fatalf("ApprovePullRequest failed: %v", err)
	}

	if err := svc.RequestChangesPullRequest(ctx, orgData, repo, 10, "Please fix security issue"); err != nil {
		t.Fatalf("RequestChangesPullRequest failed: %v", err)
	}

	if err := svc.MergePullRequest(ctx, orgData, repo, 10, "squash"); err != nil {
		t.Fatalf("MergePullRequest failed: %v", err)
	}
}

func TestForgejoService_FormatReviewCommentBody(t *testing.T) {
	svc := NewForgejoService(ForgejoServiceConfig{})

	suggestion := "Check token expiry bounds"
	formatted := svc.FormatReviewCommentBody(suggestion, "go", true, true)

	if !strings.Contains(formatted, "<!-- drixy-codereview -->") {
		t.Fatalf("expected drixy marker, got: %s", formatted)
	}
	if !strings.Contains(formatted, "https://scandrix.dev") {
		t.Fatalf("expected scandrix.dev link in footer, got: %s", formatted)
	}
}

func TestForgejoChecksService(t *testing.T) {
	server, svc := setupMockForgejoServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		AuthToken: "mock-token-123",
	}

	checks := svc.checksService

	// 1. FindCheckRun returns nil as Forgejo statuses are keyed by SHA
	id, err := checks.FindCheckRun(ctx, orgData, "org", "repo", "0123456789abcdef", CommitStatusContext)
	if err != nil || id != nil {
		t.Fatalf("expected nil for FindCheckRun on Forgejo, got %v, err: %v", id, err)
	}

	// 2. CreateCheckRun
	createResp, err := checks.CreateCheckRun(ctx, orgData, CreateCheckRunParams{
		Owner:   "org",
		Repo:    "repo",
		HeadSHA: "0123456789abcdef",
		Status:  CheckStatusInProgress,
		Output: &CheckRunOutput{
			Title:   "Analyzing code...",
			Summary: "ScanDrix is reviewing code changes",
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckRun failed: %v", err)
	}
	if createResp.Name != CommitStatusContext {
		t.Fatalf("expected context %s, got %s", CommitStatusContext, createResp.Name)
	}

	// 3. UpdateCheckRun
	conclusion := CheckConclusionSuccess
	updateResp, err := checks.UpdateCheckRun(ctx, orgData, UpdateCheckRunParams{
		Owner:      "org",
		Repo:       "repo",
		HeadSHA:    "0123456789abcdef",
		CheckRunID: 3331,
		Status:     CheckStatusCompleted,
		Conclusion: &conclusion,
	})
	if err != nil {
		t.Fatalf("UpdateCheckRun failed: %v", err)
	}
	if updateResp.Status != CheckStatusCompleted {
		t.Fatalf("expected status completed, got %v", updateResp.Status)
	}
}
