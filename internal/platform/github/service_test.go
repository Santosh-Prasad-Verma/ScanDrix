// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platform/github"
	"github.com/scandrix/backend/pkg/models"
)

func setupMockGitHubServer(t *testing.T) (*httptest.Server, *github.GitHubService) {
	mux := http.NewServeMux()

	// Pull requests list
	mux.HandleFunc("/repos/test-org/test-repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"number": 42,
					"title":  "Test Pull Request",
					"body":   "Review this change",
					"state":  "open",
					"draft":  false,
					"user":   map[string]any{"login": "test-dev", "id": 100},
					"head":   map[string]any{"ref": "feat-branch", "sha": "headsha1234"},
					"base":   map[string]any{"ref": "main", "sha": "basesha1234"},
					"created_at": time.Now().Add(-1 * time.Hour),
					"updated_at": time.Now(),
					"html_url": "https://github.com/test-org/test-repo/pull/42",
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 99,
				"title":  "New PR",
				"body":   "Automated PR",
				"state":  "open",
				"html_url": "https://github.com/test-org/test-repo/pull/99",
				"head":   map[string]any{"ref": "feat-branch", "sha": "headsha1234"},
				"base":   map[string]any{"ref": "main", "sha": "basesha1234"},
				"created_at": time.Now(),
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Single pull request
	mux.HandleFunc("/repos/test-org/test-repo/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 42,
			"title":  "Test Pull Request",
			"body":   "Review this change",
			"state":  "open",
			"draft":  false,
			"user":   map[string]any{"login": "test-dev", "id": 100},
			"head":   map[string]any{"ref": "feat-branch", "sha": "headsha1234"},
			"base":   map[string]any{"ref": "main", "sha": "basesha1234"},
			"created_at": time.Now().Add(-1 * time.Hour),
			"updated_at": time.Now(),
			"html_url": "https://github.com/test-org/test-repo/pull/42",
		})
	})

	// PR files
	mux.HandleFunc("/repos/test-org/test-repo/pulls/42/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"sha": "filesha1234",
				"filename": "main.go",
				"status": "modified",
				"additions": 10,
				"deletions": 2,
				"changes": 12,
				"patch": "@@ -1,3 +1,11 @@",
			},
		})
	})

	// Repositories
	mux.HandleFunc("/repos/test-org/test-repo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 12345,
			"name": "test-repo",
			"full_name": "test-org/test-repo",
			"private": true,
			"owner": map[string]any{"login": "test-org"},
			"default_branch": "main",
			"html_url": "https://github.com/test-org/test-repo",
			"clone_url": "https://github.com/test-org/test-repo.git",
			"language": "Go",
		})
	})

	// Comments in PR
	mux.HandleFunc("/repos/test-org/test-repo/pulls/42/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 555,
				"body": "ScanDrix inline review comment",
				"path": "main.go",
				"line": 10,
				"commit_id": "headsha1234",
				"created_at": time.Now(),
				"user": map[string]any{"login": "drixy-bot"},
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Issue comments
	mux.HandleFunc("/repos/test-org/test-repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 888,
				"body": "<!-- drixy-codereview -->\nReview Completed",
				"created_at": time.Now(),
				"user": map[string]any{"login": "drixy-bot"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})

	// Reviews
	mux.HandleFunc("/repos/test-org/test-repo/pulls/42/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"state": "APPROVED"},
		})
	})

	// User endpoint
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1,
			"login": "scandrix-admin",
			"name": "ScanDrix Admin",
		})
	})

	// GraphQL
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"repository": map[string]any{
					"f0": map[string]any{
						"text": "package main\nfunc main() {}\n",
						"isBinary": false,
						"byteSize": 28,
						"oid": "bloboid1234",
					},
				},
			},
		})
	})

	server := httptest.NewServer(mux)

	svc := github.NewGitHubService(github.GitHubServiceConfig{
		BaseURL:    server.URL,
		GraphQLURL: server.URL + "/graphql",
		HTTPClient: server.Client(),
	})

	return server, svc
}

func TestGitHubService_FullLifecycle(t *testing.T) {
	server, svc := setupMockGitHubServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		IntegrationCredentials: map[string]any{
			"token": "ghp_mock_token_1234",
			"org":   "test-org",
		},
	}
	repo := &types.RepositoryDescriptor{
		Owner: "test-org",
		Name:  "test-repo",
	}

	t.Run("Provider", func(t *testing.T) {
		if svc.Provider() != models.ProviderGitHub {
			t.Errorf("expected provider github, got %v", svc.Provider())
		}
	})

	t.Run("FindRepositoryByName", func(t *testing.T) {
		r, err := svc.FindRepositoryByName(ctx, orgData, "test-repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Name != "test-repo" || r.DefaultBranch != "main" {
			t.Errorf("unexpected repository: %+v", r)
		}
	})

	t.Run("GetPullRequests", func(t *testing.T) {
		prs, err := svc.GetPullRequests(ctx, orgData, repo, "open", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prs) != 1 || prs[0].Number != 42 {
			t.Errorf("unexpected PRs: %+v", prs)
		}
	})

	t.Run("GetPullRequest", func(t *testing.T) {
		pr, err := svc.GetPullRequest(ctx, orgData, repo, 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pr.Number != 42 || pr.Author != "test-dev" {
			t.Errorf("unexpected PR: %+v", pr)
		}
	})

	t.Run("GetFilesByPullRequestId", func(t *testing.T) {
		files, err := svc.GetFilesByPullRequestId(ctx, orgData, repo, 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 || files[0].Filename != "main.go" {
			t.Errorf("unexpected files: %+v", files)
		}
	})

	t.Run("CreateReviewComment", func(t *testing.T) {
		cmt, err := svc.CreateReviewComment(ctx, orgData, repo, 42, types.PullRequestReviewComment{
			Body:     "Check nil pointer",
			Path:     "main.go",
			Line:     10,
			CommitID: "headsha1234",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmt.ID != "555" || cmt.Author == nil || cmt.Author.Username != "drixy-bot" {
			t.Errorf("unexpected comment: %+v", cmt)
		}
	})

	t.Run("CreateIssueComment", func(t *testing.T) {
		cmt, err := svc.CreateIssueComment(ctx, orgData, repo, 42, "Summary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmt.ID != "888" {
			t.Errorf("unexpected issue comment id: %s", cmt.ID)
		}
	})

	t.Run("ApprovePullRequest", func(t *testing.T) {
		err := svc.ApprovePullRequest(ctx, orgData, repo, 42, "Looks great!")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("CheckIfPullRequestShouldBeApproved", func(t *testing.T) {
		ok, err := svc.CheckIfPullRequestShouldBeApproved(ctx, orgData, 42, *repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("expected PR to be approved")
		}
	})

	t.Run("VerifyConnection", func(t *testing.T) {
		status, err := svc.VerifyConnection(ctx, orgData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !status.IsConnected {
			t.Errorf("expected connection to be valid")
		}
	})

	t.Run("GetCloneParams", func(t *testing.T) {
		params, err := svc.GetCloneParams(ctx, orgData, *repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.Token != "ghp_mock_token_1234" {
			t.Errorf("unexpected token in clone params: %s", params.Token)
		}
	})

	t.Run("GetRepositoryContentBatch", func(t *testing.T) {
		files, err := svc.GetRepositoryContentBatch(ctx, orgData, *repo, []string{"main.go"}, "main")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if files["main.go"] == nil || files["main.go"].Content == "" {
			t.Errorf("expected file content for main.go: %+v", files)
		}
	})

	t.Run("FormatReviewCommentBody", func(t *testing.T) {
		formatted := svc.FormatReviewCommentBody("Fix error check", "go", true, true)
		if !comments.HasReviewMarker(formatted) {
			t.Errorf("expected drixy review marker in formatted comment: %s", formatted)
		}
	})
}
