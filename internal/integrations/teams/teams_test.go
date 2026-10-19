package teams_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/integrations/teams"
	"github.com/scandrix/backend/pkg/models"
)

func TestTeamsNotifier_PostReviewSummary(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := teams.NewNotifier(server.URL)
	findings := []models.CodeFinding{
		{
			Title:     "CORS wildcard vulnerability",
			Severity:  models.SeverityHigh,
			FilePath:  "server/cors.go",
			StartLine: 19,
		},
	}

	err := notifier.PostReviewSummary(context.Background(), "acme/gateway", 88, "Allow cross-origin preflight", findings)
	if err != nil {
		t.Fatalf("PostReviewSummary failed: %v", err)
	}

	if !strings.Contains(capturedBody, "acme/gateway") || !strings.Contains(capturedBody, "Total Findings") {
		t.Errorf("unexpected Teams payload: %s", capturedBody)
	}
}

func TestTeamsNotifier_PostInteractiveReviewSummary(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := teams.NewNotifier(server.URL)
	findings := []models.CodeFinding{
		{
			Title:     "Remote Code Execution flaw",
			Severity:  models.SeverityCritical,
			FilePath:  "eval/executor.py",
			StartLine: 55,
		},
	}

	err := notifier.PostInteractiveReviewSummary(
		context.Background(),
		"acme/ml-worker",
		300,
		"Support custom python user script execution",
		"https://scandrix.internal/pull-requests/repo-uuid/300",
		findings,
		"ws-teams-1",
	)
	if err != nil {
		t.Fatalf("PostInteractiveReviewSummary failed: %v", err)
	}

	if !strings.Contains(capturedBody, "approve_pr") {
		t.Errorf("expected approve_pr in Teams Adaptive Card: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "request_changes") {
		t.Errorf("expected request_changes in Teams Adaptive Card: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "https://scandrix.internal/pull-requests/repo-uuid/300") {
		t.Errorf("expected dashboard URL in Teams Adaptive Card: %s", capturedBody)
	}
}

func TestTeamsResolutionCard(t *testing.T) {
	card := teams.BuildResolutionCard("acme/repo", 101, "approve_pr", "Diana Prince")
	if card["attachments"] == nil {
		t.Fatalf("expected attachments in resolution card: %v", card)
	}
}
