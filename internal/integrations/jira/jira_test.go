package jira_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/integrations/jira"
	"github.com/scandrix/backend/pkg/models"
)

func TestExtractIssueKeys(t *testing.T) {
	keys := jira.ExtractIssueKeys("fix: [SEC-4091] resolve sql injection bug in (CORE-102)")
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %+v", len(keys), keys)
	}
	if keys[0] != "SEC-4091" || keys[1] != "CORE-102" {
		t.Errorf("unexpected keys: %+v", keys)
	}
}

func TestCreateFindingIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"SEC-8821"}`))
	}))
	defer server.Close()

	client := jira.NewClient(server.URL, "user@test.com", "token123")
	key, err := client.CreateFindingIssue(context.Background(), "SEC", models.CodeFinding{
		Title:       "SQL Injection",
		Severity:    models.SeverityCritical,
		FilePath:    "db/query.go",
		StartLine:   10,
		Description: "Raw string concat",
		Remediation: "Use params",
	})

	if err != nil {
		t.Fatalf("CreateFindingIssue failed: %v", err)
	}
	if key != "SEC-8821" {
		t.Errorf("expected key SEC-8821, got %s", key)
	}
}
