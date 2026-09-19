package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubAdvancedService_CalculateDiffPosition(t *testing.T) {
	svc := NewGitHubAdvancedService(nil)

	diff := `diff --git a/main.go b/main.go
index 83db48f..bf269f4 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main
 
+import "fmt"
+
 func main() {
+	fmt.Println("hello")
 }`

	// Line 3 is 'import "fmt"' -> position 3
	pos, err := svc.CalculateDiffPosition(diff, "main.go", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 3 {
		t.Errorf("expected position 3, got %d", pos)
	}

	// Line 6 is 'fmt.Println("hello")' -> position 6
	pos, err = svc.CalculateDiffPosition(diff, "main.go", 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 6 {
		t.Errorf("expected position 6, got %d", pos)
	}

	// Non-existent line should error
	_, err = svc.CalculateDiffPosition(diff, "main.go", 99)
	if err == nil {
		t.Errorf("expected error for non-existent line")
	}
}

func TestGitHubAdvancedService_SubmitBatchReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/org/repo/pulls/42/reviews" {
			http.NotFound(w, r)
			return
		}

		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)

		if payload["event"] != "COMMENT" {
			t.Errorf("expected event COMMENT, got %v", payload["event"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 12345,
			"state": "COMMENTED",
			"submitted_at": "2026-09-19T12:00:00Z",
			"html_url": "https://github.com/org/repo/pull/42#pullrequestreview-12345"
		}`))
	}))
	defer server.Close()

	svc := NewGitHubAdvancedService(server.Client(), server.URL)

	res, err := svc.SubmitBatchReview(context.Background(), "test-token", BatchReviewParams{
		Owner:      "org",
		Repo:       "repo",
		PullNumber: 42,
		Event:      "COMMENT",
		Body:       "Overall review feedback",
		Comments: []BatchReviewComment{
			{Path: "main.go", Line: 10, Body: "Avoid global state"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != 12345 || res.State != "COMMENTED" {
		t.Errorf("unexpected batch review result: %+v", res)
	}
}

func TestGitHubAdvancedService_CountReactions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/org/repo/pulls/comments/101/reactions" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"content": "+1"},
			{"content": "+1"},
			{"content": "heart"},
			{"content": "rocket"}
		]`))
	}))
	defer server.Close()

	svc := NewGitHubAdvancedService(server.Client(), server.URL)
	counts, err := svc.CountReactions(context.Background(), "token", "org", "repo", 101)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.TotalCount != 4 || counts.PlusOne != 2 || counts.Heart != 1 || counts.Rocket != 1 {
		t.Errorf("unexpected counts: %+v", counts)
	}
}

func TestGitHubAdvancedService_GetBranchProtection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/org/repo/branches/main/protection" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"required_pull_request_reviews": {
					"required_approving_review_count": 2,
					"dismiss_stale_reviews": true,
					"require_code_owner_reviews": true
				},
				"required_status_checks": {
					"contexts": ["ci/build", "ci/test"]
				},
				"enforce_admins": {
					"enabled": true
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewGitHubAdvancedService(server.Client(), server.URL)

	// Protected branch
	rules, err := svc.GetBranchProtection(context.Background(), "token", "org", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rules.Enabled || rules.RequiredReviews != 2 || !rules.DismissStaleReviews || len(rules.RequiredStatusChecks) != 2 {
		t.Errorf("unexpected rules: %+v", rules)
	}

	// Unprotected branch (404)
	unprotected, err := svc.GetBranchProtection(context.Background(), "token", "org", "repo", "feature")
	if err != nil {
		t.Fatalf("unexpected error on 404: %v", err)
	}
	if unprotected.Enabled {
		t.Errorf("expected unprotected branch to have Enabled=false")
	}
}
