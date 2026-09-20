// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/comments"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/internal/platform/gitlab"
	"github.com/scandrix/backend/pkg/models"
)

func setupMockGitLabServer(t *testing.T) (*httptest.Server, *gitlab.GitLabService) {
	mux := http.NewServeMux()

	// Merge Requests list
	mux.HandleFunc("/api/v4/projects/123/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":            1001,
					"iid":           42,
					"project_id":    123,
					"title":         "Add database migration engine",
					"description":   "Implements TypeORM equivalent",
					"state":         "opened",
					"source_branch": "feature/db",
					"target_branch": "main",
					"sha":           "abcdef1234567890",
					"web_url":       "https://gitlab.com/scandrix/backend/-/merge_requests/42",
					"author": map[string]any{
						"id":       50,
						"username": "gopher",
						"name":     "Go Developer",
					},
					"diff_refs": map[string]any{
						"base_sha": "base1111",
						"head_sha": "abcdef1234567890",
					},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  1002,
				"iid": 43,
			})
			return
		}
		http.NotFound(w, r)
	})

	// Single Merge Request
	mux.HandleFunc("/api/v4/projects/123/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            1001,
			"iid":           42,
			"project_id":    123,
			"title":         "Add database migration engine",
			"description":   "Implements TypeORM equivalent",
			"state":         "opened",
			"source_branch": "feature/db",
			"target_branch": "main",
			"sha":           "abcdef1234567890",
			"web_url":       "https://gitlab.com/scandrix/backend/-/merge_requests/42",
			"author": map[string]any{
				"id":       50,
				"username": "gopher",
				"name":     "Go Developer",
			},
			"diff_refs": map[string]any{
				"base_sha": "base1111",
				"head_sha": "abcdef1234567890",
			},
		})
	})

	// MR Versions (for diff refs)
	mux.HandleFunc("/api/v4/projects/123/merge_requests/42/versions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":               1,
				"head_commit_sha":  "abcdef1234567890",
				"base_commit_sha":  "base1111",
				"start_commit_sha": "start2222",
			},
		})
	})

	// Discussions API (Inline Comments)
	mux.HandleFunc("/api/v4/projects/123/merge_requests/42/discussions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "disc_abc123",
				"notes": []map[string]any{
					{
						"id":         777,
						"body":       "Please handle error explicitly",
						"created_at": time.Now().Format(time.RFC3339),
						"author": map[string]any{
							"id":       10,
							"username": "drixy",
						},
					},
				},
			})
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": "disc_abc123",
					"notes": []map[string]any{
						{
							"id":         777,
							"body":       "Please handle error explicitly",
							"created_at": time.Now().Format(time.RFC3339),
							"position": map[string]any{
								"new_path": "main.go",
								"new_line": 25,
							},
							"author": map[string]any{
								"username": "drixy",
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})

	// Notes API (Top-level PR comments)
	mux.HandleFunc("/api/v4/projects/123/merge_requests/42/notes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         888,
				"body":       "Automated review completed",
				"created_at": time.Now().Format(time.RFC3339),
				"author": map[string]any{
					"username": "drixy",
				},
			})
			return
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":         888,
					"body":       "Automated review completed",
					"created_at": time.Now().Format(time.RFC3339),
					"author": map[string]any{
						"username": "drixy",
					},
				},
			})
			return
		}
	})

	// Raw file content
	mux.HandleFunc("/api/v4/projects/123/repository/files/main.go/raw", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("package main\n\nfunc main() {}\n"))
	})

	// Project Languages
	mux.HandleFunc("/api/v4/projects/123/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]float64{
			"Go":         85.5,
			"TypeScript": 14.5,
		})
	})

	// Current User
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       10,
			"username": "drixy-bot",
			"name":     "Drixy Bot",
		})
	})

	// Award Emojis (Reactions)
	mux.HandleFunc("/api/v4/projects/123/merge_requests/42/award_emoji", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "thumbsup"})
			return
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "name": "thumbsup"},
			})
			return
		}
	})

	server := httptest.NewServer(mux)

	svc := gitlab.NewGitLabService(gitlab.GitLabServiceConfig{
		BaseURL:        server.URL,
		HTTPClient:     server.Client(),
		DefaultTimeout: 5 * time.Second,
	})

	return server, svc
}

func TestGitLabService_Comprehensive(t *testing.T) {
	server, svc := setupMockGitLabServer(t)
	defer server.Close()

	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-1",
		TeamID:         "team-1",
		AuthToken:      "glpat-mock-token-xyz",
	}
	repo := &types.RepositoryDescriptor{
		ID:       "123",
		Name:     "backend",
		FullName: "scandrix/backend",
	}

	t.Run("Provider", func(t *testing.T) {
		if svc.Provider() != models.ProviderGitLab {
			t.Fatalf("expected gitlab provider, got %s", svc.Provider())
		}
	})

	t.Run("VerifyConnection", func(t *testing.T) {
		status, err := svc.VerifyConnection(ctx, orgData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !status.IsConnected {
			t.Errorf("expected connection status connected, got: %+v", status)
		}
	})

	t.Run("GetPullRequests", func(t *testing.T) {
		prs, err := svc.GetPullRequests(ctx, orgData, repo, "opened", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prs) != 1 {
			t.Fatalf("expected 1 MR, got %d", len(prs))
		}
		if prs[0].Number != 42 {
			t.Errorf("expected MR #42, got %d", prs[0].Number)
		}
		if prs[0].HeadSHA != "abcdef1234567890" {
			t.Errorf("expected head SHA, got %s", prs[0].HeadSHA)
		}
	})

	t.Run("GetPullRequest", func(t *testing.T) {
		pr, err := svc.GetPullRequest(ctx, orgData, repo, 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pr == nil || pr.Number != 42 {
			t.Fatalf("expected MR #42, got %+v", pr)
		}
	})

	t.Run("CreateReviewComment", func(t *testing.T) {
		comment := types.PullRequestReviewComment{
			Path: "main.go",
			Line: 25,
			Body: "Please handle error explicitly",
		}
		created, err := svc.CreateReviewComment(ctx, orgData, repo, 42, comment)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created == nil || created.ID != "777" {
			t.Errorf("expected note ID 777, got %+v", created)
		}
		if created.ThreadID != "disc_abc123" {
			t.Errorf("expected thread ID disc_abc123, got %s", created.ThreadID)
		}
	})

	t.Run("CreateCommentInPullRequest", func(t *testing.T) {
		created, err := svc.CreateCommentInPullRequest(ctx, orgData, repo, 42, "Automated review completed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created == nil || created.ID != "888" {
			t.Errorf("expected note ID 888, got %+v", created)
		}
	})

	t.Run("GetRepositoryContentFile", func(t *testing.T) {
		file, err := svc.GetRepositoryContentFile(ctx, orgData, repo, "main.go", "main")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if file == nil || file.Content == "" {
			t.Fatalf("expected file content, got %+v", file)
		}
	})

	t.Run("GetLanguageRepository", func(t *testing.T) {
		langs, err := svc.GetLanguageRepository(ctx, orgData, repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if langs["Go"] != 8550 { // 85.5 * 100
			t.Errorf("expected Go language percentage 8550, got %d", langs["Go"])
		}
	})

	t.Run("AddReactionToPR", func(t *testing.T) {
		err := svc.AddReactionToPR(ctx, orgData, *repo, 42, "+1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("FormatReviewCommentBody", func(t *testing.T) {
		formatted := svc.FormatReviewCommentBody("Fix error check", "go", true, true)
		if !comments.HasReviewMarker(formatted) {
			t.Errorf("expected drixy review marker in formatted comment: %s", formatted)
		}
	})
}
