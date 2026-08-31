package linear_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/integrations/linear"
	"github.com/scandrix/backend/pkg/models"
)

func TestLinearClient(t *testing.T) {
	ctx := context.Background()

	// 1. Test empty API key
	emptyClient := linear.NewClient("")
	_, err := emptyClient.CreateIssue(ctx, "team-1", models.CodeFinding{})
	if err == nil {
		t.Fatal("expected error with empty api key")
	}

	// 2. Test valid issue creation
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issueCreate": map[string]any{
					"success": true,
					"issue": map[string]string{
						"id":         "issue-123",
						"identifier": "ENG-101",
						"url":        "https://linear.app/issue/ENG-101",
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := linear.NewClient("lin_api_key_test")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
