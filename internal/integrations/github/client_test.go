package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

func TestGitHubClientCheckRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 9988, "status": "in_progress", "html_url": "https://github.com/check/9988"}`))
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 9988, "status": "completed", "conclusion": "success"}`))
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	// 1. Create check run
	createReq := CreateCheckRunRequest{
		Name:    "scandrix/review",
		HeadSHA: "abcdef123456",
		Status:  "in_progress",
	}
	res, err := client.CreateCheckRun(context.Background(), "acme", "repo", createReq)
	if err != nil {
		t.Fatalf("failed creating check run: %v", err)
	}
	if res.ID != 9988 || res.Status != "in_progress" {
		t.Errorf("unexpected check run response: %+v", res)
	}

	// 2. Update check run
	updateReq := UpdateCheckRunRequest{
		Status:     "completed",
		Conclusion: "success",
		Output: &CheckRunOutput{
			Title:   "ScanDrix Review Passed",
			Summary: "Zero vulnerabilities discovered",
		},
	}
	err = client.UpdateCheckRun(context.Background(), "acme", "repo", res.ID, updateReq)
	if err != nil {
		t.Fatalf("failed updating check run: %v", err)
	}
}

func TestGitHubClientCreateCommentReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 12345, "body": "acknowledged"}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	err := client.CreateCommentReply(context.Background(), "owner", "repo", 42, 1001, "Fix applied in commit abc")
	if err != nil {
		t.Fatalf("unexpected error creating comment reply: %v", err)
	}
}

func TestGitHubAppRS256JWTAndInstallationToken(t *testing.T) {
	// Generate in-memory RSA key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed generating test RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(privKey)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	})

	// 1. Generate JWT
	jwtToken, err := GenerateAppJWT("app-12345", pemBytes)
	if err != nil {
		t.Fatalf("failed generating app JWT: %v", err)
	}
	if len(jwtToken) == 0 {
		t.Fatal("expected non-empty JWT token")
	}

	// 2. Mock token exchange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+jwtToken {
			t.Errorf("expected Bearer JWT in header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token": "ghs_installation_ephemeral_token_7788"}`))
	}))
	defer server.Close()

	client := NewClient("")
	client.baseURL = server.URL

	instToken, err := client.CreateInstallationToken(context.Background(), 9911, jwtToken)
	if err != nil {
		t.Fatalf("failed exchanging installation token: %v", err)
	}
	if instToken != "ghs_installation_ephemeral_token_7788" {
		t.Errorf("unexpected installation token: %s", instToken)
	}
}


