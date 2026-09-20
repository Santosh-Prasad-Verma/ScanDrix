// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitHubPullReviews_ReviewsAndReplies(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/scandrix/platform/pulls/10/reviews" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":               1001,
					"body":             "Looks great! Approved by Drixy",
					"state":            "APPROVED",
					"html_url":         "https://github.com/scandrix/platform/pull/10#pullrequestreview-1001",
					"pull_request_url": "https://api.github.com/repos/scandrix/platform/pulls/10",
					"commit_id":        "sha123",
					"submitted_at":     now.Format(time.RFC3339),
					"user":             map[string]any{"login": "drixy-bot"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/reviews/1001" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":               1001,
				"body":             "Looks great! Approved by Drixy",
				"state":            "APPROVED",
				"html_url":         "https://github.com/scandrix/platform/pull/10#pullrequestreview-1001",
				"pull_request_url": "https://api.github.com/repos/scandrix/platform/pulls/10",
				"commit_id":        "sha123",
				"submitted_at":     now.Format(time.RFC3339),
				"user":             map[string]any{"login": "drixy-bot"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/reviews/1001/dismissals" && r.Method == http.MethodPut:
			resp := map[string]any{
				"id":           1001,
				"body":         "Dismissed",
				"state":        "DISMISSED",
				"submitted_at": now.Format(time.RFC3339),
				"user":         map[string]any{"login": "drixy-bot"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/reviews/1001/comments":
			resp := []map[string]any{
				{
					"id":                     5001,
					"pull_request_review_id": 1001,
					"path":                   "cmd/main.go",
					"line":                   42,
					"start_line":             40,
					"side":                   "RIGHT",
					"body":                   "Consider connection pooling here.",
					"commit_id":              "sha123",
					"created_at":             now.Format(time.RFC3339),
					"updated_at":             now.Format(time.RFC3339),
					"html_url":               "https://github.com/scandrix/platform/pull/10#discussion_r5001",
					"user":                   map[string]any{"login": "drixy-bot"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/comments/5001/replies" && r.Method == http.MethodPost:
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":                     5002,
				"pull_request_review_id": 1001,
				"path":                   "cmd/main.go",
				"line":                   42,
				"side":                   "RIGHT",
				"body":                   req["body"],
				"commit_id":              "sha123",
				"created_at":             now.Format(time.RFC3339),
				"updated_at":             now.Format(time.RFC3339),
				"in_reply_to_id":         5001,
				"user":                   map[string]any{"login": "alice"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewGitHubAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. List reviews
	reviews, err := svc.ListPullRequestReviews(ctx, "mock-token", "scandrix", "platform", 10)
	if err != nil {
		t.Fatalf("ListPullRequestReviews error: %v", err)
	}
	if len(reviews) != 1 || reviews[0].User != "drixy-bot" {
		t.Fatalf("unexpected reviews: %+v", reviews)
	}

	// 2. Get review
	rev, err := svc.GetPullRequestReview(ctx, "mock-token", "scandrix", "platform", 10, 1001)
	if err != nil {
		t.Fatalf("GetPullRequestReview error: %v", err)
	}
	if rev.ID != 1001 || rev.State != "APPROVED" {
		t.Fatalf("unexpected review: %+v", rev)
	}

	// 3. Dismiss review
	dismissed, err := svc.DismissPullRequestReview(ctx, "mock-token", "scandrix", "platform", 10, 1001, "Outdated by rebase")
	if err != nil {
		t.Fatalf("DismissPullRequestReview error: %v", err)
	}
	if dismissed.State != "DISMISSED" {
		t.Fatalf("expected DISMISSED, got %s", dismissed.State)
	}

	// 4. List review comments
	comments, err := svc.ListReviewComments(ctx, "mock-token", "scandrix", "platform", 10, 1001)
	if err != nil {
		t.Fatalf("ListReviewComments error: %v", err)
	}
	if len(comments) != 1 || comments[0].Path != "cmd/main.go" {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	// 5. Reply to review comment
	reply, err := svc.CreateReviewCommentReply(ctx, "mock-token", "scandrix", "platform", 10, 5001, "Fixed in latest commit!")
	if err != nil {
		t.Fatalf("CreateReviewCommentReply error: %v", err)
	}
	if reply.InReplyToID != 5001 || reply.Body != "Fixed in latest commit!" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

func TestGitHubPullReviews_ReviewersAndMerge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/scandrix/platform/pulls/10/requested_reviewers" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/requested_reviewers" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/requested_reviewers" && r.Method == http.MethodGet:
			resp := map[string]any{
				"users": []map[string]any{{"login": "alice"}},
				"teams": []map[string]any{{"slug": "security-leads"}},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10" && r.Method == http.MethodGet:
			mergeable := true
			rebaseable := true
			resp := map[string]any{
				"mergeable":       &mergeable,
				"mergeable_state": "clean",
				"rebaseable":      &rebaseable,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/scandrix/platform/pulls/10/merge" && r.Method == http.MethodPut:
			resp := map[string]any{
				"sha":     "merged-sha-123",
				"merged":  true,
				"message": "Pull Request successfully merged",
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewGitHubAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Request reviewers
	if err := svc.RequestReviewers(ctx, "mock-token", "scandrix", "platform", 10, []string{"alice"}, []string{"security-leads"}); err != nil {
		t.Fatalf("RequestReviewers error: %v", err)
	}

	// 2. Get requested reviewers
	reviewers, err := svc.GetRequestedReviewers(ctx, "mock-token", "scandrix", "platform", 10)
	if err != nil {
		t.Fatalf("GetRequestedReviewers error: %v", err)
	}
	if len(reviewers.Users) != 1 || reviewers.Users[0] != "alice" || len(reviewers.Teams) != 1 {
		t.Fatalf("unexpected requested reviewers: %+v", reviewers)
	}

	// 3. Remove reviewers
	if err := svc.RemoveReviewers(ctx, "mock-token", "scandrix", "platform", 10, []string{"alice"}, nil); err != nil {
		t.Fatalf("RemoveReviewers error: %v", err)
	}

	// 4. Check mergeability
	mergeability, err := svc.CheckMergeability(ctx, "mock-token", "scandrix", "platform", 10)
	if err != nil {
		t.Fatalf("CheckMergeability error: %v", err)
	}
	if mergeability.Mergeable == nil || !*mergeability.Mergeable || mergeability.MergeableState != "clean" {
		t.Fatalf("unexpected mergeability: %+v", mergeability)
	}

	// 5. Merge PR
	merged, err := svc.MergePullRequest(ctx, "mock-token", "scandrix", "platform", 10, MergePullRequestInput{
		CommitTitle: "Merge pull request #10",
		MergeMethod: "squash",
	})
	if err != nil {
		t.Fatalf("MergePullRequest error: %v", err)
	}
	if !merged.Merged || merged.SHA != "merged-sha-123" {
		t.Fatalf("unexpected merge result: %+v", merged)
	}
}
