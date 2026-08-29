package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubClientFetchDiff(t *testing.T) {
	expectedDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.v3.diff" {
			t.Errorf("expected Accept header for diff, got %s", r.Header.Get("Accept"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedDiff))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	diff, err := client.FetchPullRequestDiff(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error fetching diff: %v", err)
	}

	if diff != expectedDiff {
		t.Errorf("expected diff %q, got %q", expectedDiff, diff)
	}
}

func TestGitHubClientSubmitReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 101, "state": "COMMENTED"}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	submission := PullReviewSubmission{
		Body:  "Automated Code Assurance Completed",
		Event: "COMMENT",
		Comments: []ReviewCommentPayload{
			{
				Path: "file.go",
				Line: 1,
				Body: "Consider using structured logging instead of print statements.",
			},
		},
	}

	err := client.SubmitPullRequestReview(context.Background(), "owner", "repo", 42, submission)
	if err != nil {
		t.Fatalf("unexpected error submitting review: %v", err)
	}
}
