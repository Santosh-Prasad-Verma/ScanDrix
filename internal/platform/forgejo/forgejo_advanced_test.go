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

func TestForgejoAdvancedService_ReviewsAndStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/myorg/myrepo/pulls/15/reviews" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":           float64(301),
				"state":        req["event"],
				"body":         req["body"],
				"submitted_at": time.Now().Format(time.RFC3339),
				"html_url":     "https://codeberg.org/myorg/myrepo/pulls/15#review-301",
				"user": map[string]any{
					"username": "drixy",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repos/myorg/myrepo/statuses/c0ffee" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)

		case r.URL.Path == "/repos/myorg/myrepo/branch_protections/main":
			resp := map[string]any{
				"branch_name":               "main",
				"enable_push":               false,
				"enable_push_whitelist":     true,
				"push_whitelist_usernames":  []string{"lead-dev"},
				"enable_merge_whitelist":    true,
				"required_approvals":        float64(1),
				"block_on_rejected_reviews": true,
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewForgejoAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Submit pull review
	submission := PullReviewSubmission{
		Event: "APPROVED",
		Body:  "Looks great! All checks pass.",
		Comments: []ReviewCommentDraft{
			{
				Path:    "main.go",
				NewLine: 10,
				Body:    "Clean implementation.",
			},
		},
	}
	res, err := svc.SubmitPullReview(ctx, "mock-token", "myorg", "myrepo", 15, submission)
	if err != nil {
		t.Fatalf("SubmitPullReview failed: %v", err)
	}
	if res.ID != 301 || res.State != "APPROVED" || res.Reviewer != "drixy" {
		t.Errorf("unexpected review result: %+v", res)
	}

	// 2. Create commit status
	status := CommitStatusSubmission{
		State:       "success",
		TargetURL:   "https://scandrix.dev/reviews/123",
		Description: "ScanDrix analysis: 0 defects",
		Context:     "scandrix/code-review",
	}
	err = svc.CreateCommitStatus(ctx, "mock-token", "myorg", "myrepo", "c0ffee", status)
	if err != nil {
		t.Fatalf("CreateCommitStatus failed: %v", err)
	}

	// 3. Get branch protection
	bp, err := svc.GetBranchProtection(ctx, "mock-token", "myorg", "myrepo", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection failed: %v", err)
	}
	if bp.BranchName != "main" || bp.RequiredApprovals != 1 || !bp.BlockOnRejectedReviews {
		t.Errorf("unexpected branch protection: %+v", bp)
	}
}
