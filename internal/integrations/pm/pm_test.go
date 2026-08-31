package pm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/pkg/models"
)

func TestPMIntegrationsAndDispatcher(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	// 1. Mock Jira Server
	jiraServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/issue" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":   "10001",
				"key":  "SEC-102",
				"self": "http://jira/rest/api/3/issue/10001",
			})
		case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/SEC-102/remotelink"):
			w.WriteHeader(http.StatusCreated)
		case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/SEC-102"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  "10001",
				"key": "SEC-102",
				"fields": map[string]any{
					"summary": "Fix AWS Key Leak",
					"status":  map[string]string{"name": "To Do"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer jiraServer.Close()

	// 2. Mock Linear GraphQL Server
	linearServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issueCreate": map[string]any{
					"success": true,
					"issue": map[string]any{
						"id":         "lin_123",
						"identifier": "ENG-404",
						"title":      "Fix SQL Injection",
						"url":        "https://linear.app/issue/ENG-404",
						"state":      map[string]string{"name": "Todo"},
					},
				},
			},
		})
	}))
	defer linearServer.Close()

	// 3. Mock Azure Boards Server
	boardsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  555,
			"url": "https://dev.azure.com/acme/project/_apis/wit/workitems/555",
			"fields": map[string]any{
				"System.Title": "Fix XSS Vulnerability",
				"System.State": "New",
			},
		})
	}))
	defer boardsServer.Close()

	// 4. Test Jira Adapter
	jiraAdapter := pm.NewJiraAdapter(jiraServer.URL, "dev@acme.com", "token_123")
	if jiraAdapter.Platform() != pm.PlatformJira {
		t.Fatalf("expected platform jira")
	}

	issue, err := jiraAdapter.CreateIssue(ctx, pm.IssueCreationRequest{
		ProjectKey:  "SEC",
		IssueType:   "Bug",
		Title:       "Test finding issue",
		Description: "Found test secret",
		Severity:    models.SeverityCritical,
	})
	if err != nil || issue.Key != "SEC-102" {
		t.Fatalf("jira issue creation failed: %+v, err: %v", issue, err)
	}

	fetchedIssue, err := jiraAdapter.GetIssue(ctx, "SEC-102")
	if err != nil || fetchedIssue.Status != "To Do" {
		t.Fatalf("jira get issue failed: %+v, err: %v", fetchedIssue, err)
	}

	// 5. Test PM Dispatcher Export Finding
	dispatcher := pm.NewPMDispatcher()
	dispatcher.RegisterAdapter(wsID, jiraAdapter)

	finding := models.CodeFinding{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Title:       "Hardcoded AWS Token",
		FilePath:    "pkg/auth/keys.go",
		StartLine:   42,
		Severity:    models.SeverityCritical,
		Category:    "SECURITY_SECRET",
		Description: "AWS credentials leaked in source code",
		Remediation: "Use environment variables or Vault",
	}

	exported, err := dispatcher.ExportFinding(ctx, wsID, pm.PlatformJira, "SEC", finding, "https://github.com/acme/repo/pull/12")
	if err != nil {
		t.Fatalf("failed exporting finding through dispatcher: %v", err)
	}
	if exported.Key != "SEC-102" || exported.Platform != pm.PlatformJira {
		t.Fatalf("unexpected exported issue: %+v", exported)
	}

	// 6. Test Adapter Factory NewAdapterFromConfig
	jAdapter, err := pm.NewAdapterFromConfig(pm.PMConfig{
		Platform: pm.PlatformJira,
		BaseURL:  jiraServer.URL,
		APIToken: "tok",
		Email:    "e@acme.com",
	})
	if err != nil || jAdapter.Platform() != pm.PlatformJira {
		t.Fatalf("failed creating jira adapter from config: %v", err)
	}

	lAdapter, err := pm.NewAdapterFromConfig(pm.PMConfig{
		Platform: pm.PlatformLinear,
		APIToken: "lin_tok",
	})
	if err != nil || lAdapter.Platform() != pm.PlatformLinear {
		t.Fatalf("failed creating linear adapter from config: %v", err)
	}

	bAdapter, err := pm.NewAdapterFromConfig(pm.PMConfig{
		Platform:     pm.PlatformAzureBoards,
		BaseURL:      boardsServer.URL,
		Organization: "acme",
		APIToken:     "pat",
	})
	if err != nil || bAdapter.Platform() != pm.PlatformAzureBoards {
		t.Fatalf("failed creating azure boards adapter from config: %v", err)
	}

	// 7. Test AutoTicketManager with Nil Repo
	autoTicketMgr := pm.NewAutoTicketManager(nil, dispatcher)
	created, err := autoTicketMgr.ProcessFindingsForAutoTicket(ctx, wsID, uuid.New(), []models.CodeFinding{finding}, "")
	if err != nil || len(created) != 0 {
		t.Fatalf("expected nil or empty created list for nil repo, got %v, err: %v", created, err)
	}
}
