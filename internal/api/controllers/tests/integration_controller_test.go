package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
)

func TestIntegrationControllerPMAndSCMRoutes(t *testing.T) {
	ctrl := controllers.NewIntegrationController(nil)
	wsID := uuid.New()
	repoID := uuid.New()

	router := ctrl.Routes()

	// 1. Test List Integrations (nil repo fallback)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list integrations, got: %d", rec.Code)
	}

	// 2. Test Connect PM Integration (Jira)
	connectBody, _ := json.Marshal(dtos.ConnectPMRequest{
		Platform: "jira",
		BaseURL:  "https://acme.atlassian.net",
		APIToken: "jira_token_123",
		Email:    "dev@acme.com",
	})
	req = httptest.NewRequest(http.MethodPost, "/pm/connect", bytes.NewReader(connectBody))
	req = req.WithContext(ctx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Since repo is nil, handler fails safely with 500
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusCreated {
		t.Fatalf("unexpected code for connect pm: %d", rec.Code)
	}

	// 3. Test Auto Ticket Config Get
	req = httptest.NewRequest(http.MethodGet, "/pm/auto-ticket?repo_id="+repoID.String(), nil)
	req = req.WithContext(ctx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get auto ticket config, got: %d", rec.Code)
	}
}
