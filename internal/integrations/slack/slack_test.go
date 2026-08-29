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
			Title:    "Hardcoded Secret",
			Severity: models.SeverityCritical,
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
