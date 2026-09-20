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
	"testing"
	"time"
)

func TestForgejoExtended_ReviewsAndComments(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15/reviews" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":           101,
					"state":        "APPROVED",
					"body":         "LGTM from Drixy",
					"html_url":     "https://codeberg.org/scandrix-org/core-repo/pulls/15/reviews/101",
					"submitted_at": now.Format(time.RFC3339),
					"user":         map[string]any{"username": "drixy-bot"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15/reviews/101" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":           101,
				"state":        "APPROVED",
				"body":         "LGTM from Drixy",
				"html_url":     "https://codeberg.org/scandrix-org/core-repo/pulls/15/reviews/101",
				"submitted_at": now.Format(time.RFC3339),
				"user":         map[string]any{"username": "drixy-bot"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15/reviews/101/dismissals" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15/reviews/101/comments":
			resp := []map[string]any{
				{
					"id":                     501,
					"pull_request_review_id": 101,
					"path":                   "cmd/main.go",
					"old_line_num":           0,
					"new_line_num":           25,
					"body":                   "Consider graceful shutdown here.",
					"commit_id":              "sha999",
					"created_at":             now.Format(time.RFC3339),
					"updated_at":             now.Format(time.RFC3339),
					"html_url":               "https://codeberg.org/scandrix-org/core-repo/pulls/15#comment-501",
					"pull_request_url":       "https://codeberg.org/scandrix-org/core-repo/pulls/15",
					"user":                   map[string]any{"username": "drixy-bot"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewForgejoAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. List reviews
	reviews, err := svc.ListPullReviews(ctx, "test-token", "scandrix-org", "core-repo", 15)
	if err != nil {
		t.Fatalf("ListPullReviews error: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Reviewer != "drixy-bot" {
		t.Fatalf("unexpected reviews: %+v", reviews)
	}

	// 2. Get review
	rev, err := svc.GetPullReview(ctx, "test-token", "scandrix-org", "core-repo", 15, 101)
	if err != nil {
		t.Fatalf("GetPullReview error: %v", err)
	}
	if rev.ID != 101 || rev.State != "APPROVED" {
		t.Fatalf("unexpected review: %+v", rev)
	}

	// 3. Dismiss review
	if err := svc.DismissPullReview(ctx, "test-token", "scandrix-org", "core-repo", 15, 101, "Outdated by rebase"); err != nil {
		t.Fatalf("DismissPullReview error: %v", err)
	}

	// 4. List review comments
	comments, err := svc.ListReviewComments(ctx, "test-token", "scandrix-org", "core-repo", 15, 101)
	if err != nil {
		t.Fatalf("ListReviewComments error: %v", err)
	}
	if len(comments) != 1 || comments[0].Path != "cmd/main.go" {
		t.Fatalf("unexpected comments: %+v", comments)
	}
}

func TestForgejoExtended_DiffsAndFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15.diff":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n+package main\n"))

		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15.patch":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("From sha123 Mon Sep 17 00:00:00 2001\nSubject: [PATCH] add main\n"))

		case r.URL.Path == "/repos/scandrix-org/core-repo/pulls/15/files":
			w.Header().Set("Content-Type", "application/json")
			resp := []map[string]any{
				{
					"filename":  "main.go",
					"status":    "added",
					"additions": 10,
					"deletions": 0,
					"changes":   10,
					"patch":     "@@ -0,0 +1,10 @@\n+package main",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewForgejoAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	diff, err := svc.GetPullRequestDiff(ctx, "test-token", "scandrix-org", "core-repo", 15, false)
	if err != nil {
		t.Fatalf("GetPullRequestDiff error: %v", err)
	}
	if len(diff) == 0 {
		t.Fatalf("empty diff returned")
	}

	patch, err := svc.GetPullRequestPatch(ctx, "test-token", "scandrix-org", "core-repo", 15)
	if err != nil {
		t.Fatalf("GetPullRequestPatch error: %v", err)
	}
	if len(patch) == 0 {
		t.Fatalf("empty patch returned")
	}

	files, err := svc.GetPullRequestFiles(ctx, "test-token", "scandrix-org", "core-repo", 15)
	if err != nil {
		t.Fatalf("GetPullRequestFiles error: %v", err)
	}
	if len(files) != 1 || files[0].Filename != "main.go" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestForgejoExtended_BranchProtections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/scandrix-org/core-repo/branch_protections" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"branch_name":        "main",
					"enable_push":        false,
					"required_approvals": 2,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/branch_protections/main" && r.Method == http.MethodGet:
			resp := map[string]any{
				"branch_name":        "main",
				"enable_push":        false,
				"required_approvals": 2,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/branch_protections" && r.Method == http.MethodPost:
			var bp BranchProtection
			_ = json.NewDecoder(r.Body).Decode(&bp)
			_ = json.NewEncoder(w).Encode(bp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/branch_protections/main" && r.Method == http.MethodPatch:
			var bp BranchProtection
			_ = json.NewDecoder(r.Body).Decode(&bp)
			_ = json.NewEncoder(w).Encode(bp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/branch_protections/main" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewForgejoAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get branch protections
	bps, err := svc.GetBranchProtections(ctx, "test-token", "scandrix-org", "core-repo")
	if err != nil {
		t.Fatalf("GetBranchProtections error: %v", err)
	}
	if len(bps) != 1 || bps[0].BranchName != "main" {
		t.Fatalf("unexpected protections: %+v", bps)
	}

	// 2. Get specific protection
	bp, err := svc.GetBranchProtection(ctx, "test-token", "scandrix-org", "core-repo", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection error: %v", err)
	}
	if bp.RequiredApprovals != 2 {
		t.Fatalf("expected 2 approvals, got %d", bp.RequiredApprovals)
	}

	// 3. Create protection
	created, err := svc.CreateBranchProtection(ctx, "test-token", "scandrix-org", "core-repo", BranchProtection{
		BranchName:        "release",
		EnablePush:        false,
		RequiredApprovals: 1,
	})
	if err != nil {
		t.Fatalf("CreateBranchProtection error: %v", err)
	}
	if created.BranchName != "release" {
		t.Fatalf("unexpected created branch: %s", created.BranchName)
	}

	// 4. Update protection
	updated, err := svc.UpdateBranchProtection(ctx, "test-token", "scandrix-org", "core-repo", "main", BranchProtection{
		BranchName:        "main",
		RequiredApprovals: 3,
	})
	if err != nil {
		t.Fatalf("UpdateBranchProtection error: %v", err)
	}
	if updated.RequiredApprovals != 3 {
		t.Fatalf("expected 3 approvals, got %d", updated.RequiredApprovals)
	}

	// 5. Delete protection
	if err := svc.DeleteBranchProtection(ctx, "test-token", "scandrix-org", "core-repo", "main"); err != nil {
		t.Fatalf("DeleteBranchProtection error: %v", err)
	}
}

func TestForgejoExtended_StatusesTopicsAndCollaborators(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/scandrix-org/core-repo/commits/sha123/status":
			resp := map[string]any{
				"state":       "success",
				"sha":         "sha123",
				"total_count": 1,
				"statuses": []map[string]any{
					{
						"id":          1,
						"state":       "success",
						"context":     "scandrix/ci",
						"description": "Passed all scans",
						"created_at":  now.Format(time.RFC3339),
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/commits/sha123/statuses":
			resp := []map[string]any{
				{
					"id":          1,
					"state":       "success",
					"context":     "scandrix/ci",
					"description": "Passed all scans",
					"created_at":  now.Format(time.RFC3339),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/topics" && r.Method == http.MethodGet:
			resp := map[string]any{
				"topics": []string{"security", "ai-review", "go"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix-org/core-repo/topics" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/repos/scandrix-org/core-repo/collaborators/alice":
			resp := map[string]any{
				"permission": "write",
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewForgejoAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Combined status
	combined, err := svc.GetCombinedCommitStatus(ctx, "test-token", "scandrix-org", "core-repo", "sha123")
	if err != nil {
		t.Fatalf("GetCombinedCommitStatus error: %v", err)
	}
	if combined.State != "success" || combined.TotalCount != 1 {
		t.Fatalf("unexpected combined status: %+v", combined)
	}

	// 2. List commit statuses
	statuses, err := svc.ListCommitStatuses(ctx, "test-token", "scandrix-org", "core-repo", "sha123", 1, 10)
	if err != nil {
		t.Fatalf("ListCommitStatuses error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Context != "scandrix/ci" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	// 3. Topics
	topics, err := svc.ListRepoTopics(ctx, "test-token", "scandrix-org", "core-repo")
	if err != nil {
		t.Fatalf("ListRepoTopics error: %v", err)
	}
	if len(topics) != 3 || topics[0] != "security" {
		t.Fatalf("unexpected topics: %+v", topics)
	}

	if err := svc.SetRepoTopics(ctx, "test-token", "scandrix-org", "core-repo", []string{"security", "go"}); err != nil {
		t.Fatalf("SetRepoTopics error: %v", err)
	}

	// 4. Check collaborator
	isCollab, perm, err := svc.CheckCollaborator(ctx, "test-token", "scandrix-org", "core-repo", "alice")
	if err != nil {
		t.Fatalf("CheckCollaborator error: %v", err)
	}
	if !isCollab || perm != "write" {
		t.Fatalf("expected collaborator with write permission, got %v, %s", isCollab, perm)
	}
}
