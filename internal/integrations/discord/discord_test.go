package discord_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/integrations/discord"
	"github.com/scandrix/backend/pkg/models"
)

func TestPostReviewSummaryNoOp(t *testing.T) {
	n := discord.NewNotifier("") // empty URL = no-op
	err := n.PostReviewSummary(context.Background(), "test/repo", 42, "fix stuff", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty webhook, got %v", err)
	}
}

func TestPostReviewSummarySuccess(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type, got %s", r.Header.Get("Content-Type"))
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&receivedPayload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusNoContent) // Discord returns 204 on success
	}))
	defer server.Close()

	n := discord.NewNotifier(server.URL)
	n.SetHTTPClient(server.Client())

	findings := []models.CodeFinding{
		{Severity: models.SeverityCritical, FilePath: "main.go", StartLine: 42, Title: "SQL injection vulnerability"},
		{Severity: models.SeverityHigh, FilePath: "auth.go", StartLine: 10, Title: "Missing input validation"},
		{Severity: models.SeverityMedium, FilePath: "utils.go", StartLine: 5, Title: "Unused import"},
	}

	err := n.PostReviewSummary(context.Background(), "scandrix/backend", 123, "Add login endpoint", findings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Validate embed structure
	embeds, ok := receivedPayload["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("expected embeds array in payload")
	}

	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatal("expected first embed to be a map")
	}

	title, _ := embed["title"].(string)
	if title == "" {
		t.Error("expected non-empty embed title")
	}

	fields, ok := embed["fields"].([]any)
	if !ok {
		t.Fatal("expected fields array")
	}

	// 3 header fields (repo, PR, total) + 3 finding fields = 6
	if len(fields) != 6 {
		t.Errorf("expected 6 fields (3 header + 3 findings), got %d", len(fields))
	}
}

func TestPostReviewSummaryErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests) // 429
	}))
	defer server.Close()

	n := discord.NewNotifier(server.URL)
	n.SetHTTPClient(server.Client())

	err := n.PostReviewSummary(context.Background(), "test/repo", 1, "test", nil)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}
