package slack_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/integrations/slack"
	"github.com/scandrix/backend/pkg/models"
)

func TestPostReviewSummary(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := slack.NewNotifier(server.URL)
	findings := []models.CodeFinding{
		{
			Title:     "Hardcoded Secret",
			Severity:  models.SeverityCritical,
			FilePath:  "auth/jwt.go",
			StartLine: 42,
		},
	}

	err := notifier.PostReviewSummary(context.Background(), "acme/auth-service", 42, "Refactor credentials", findings)
	if err != nil {
		t.Fatalf("PostReviewSummary failed: %v", err)
	}

	if !strings.Contains(capturedBody, "acme/auth-service") || !strings.Contains(capturedBody, "Critical Finding") {
		t.Errorf("unexpected Slack payload: %s", capturedBody)
	}
}

func TestPostInteractiveReviewSummary(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := slack.NewNotifier(server.URL)
	findings := []models.CodeFinding{
		{
			Title:     "SQL Injection in Search query",
			Severity:  models.SeverityCritical,
			FilePath:  "internal/search/repo.go",
			StartLine: 104,
		},
	}

	err := notifier.PostInteractiveReviewSummary(
		context.Background(),
		"acme/search-service",
		108,
		"Add elastic search query",
		"https://scandrix.internal/pull-requests/repo-uuid/108",
		findings,
		"ws-123",
	)
	if err != nil {
		t.Fatalf("PostInteractiveReviewSummary failed: %v", err)
	}

	if !strings.Contains(capturedBody, "approve_pr") {
		t.Errorf("expected approve_pr action button in payload: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "request_changes") {
		t.Errorf("expected request_changes action button in payload: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "https://scandrix.internal/pull-requests/repo-uuid/108") {
		t.Errorf("expected dashboard URL in payload: %s", capturedBody)
	}
}

func TestBuildResolutionResponse(t *testing.T) {
	approved := slack.BuildResolutionResponse("acme/repo", 7, "approve_pr", "alice")
	if approved["text"] == "" || !strings.Contains(approved["text"].(string), "APPROVED") {
		t.Errorf("expected approved text: %v", approved)
	}

	changes := slack.BuildResolutionResponse("acme/repo", 7, "request_changes", "bob")
	if changes["text"] == "" || !strings.Contains(changes["text"].(string), "CHANGES REQUESTED") {
		t.Errorf("expected changes requested text: %v", changes)
	}
}
