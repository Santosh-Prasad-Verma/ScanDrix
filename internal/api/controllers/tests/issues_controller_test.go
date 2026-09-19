package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
)

func TestIssuesControllerEndpoints(t *testing.T) {
	ctrl := controllers.NewIssuesController(nil)
	router := ctrl.Routes()
	wsID := uuid.New()

	// 1. Missing workspace auth header -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", w.Code)
	}

	// 2. Authorized request with tenant context -> 200 OK
	ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, wsID)
	reqAuth := httptest.NewRequest(http.MethodGet, "/?status=OPEN", nil).WithContext(ctx)
	wAuth := httptest.NewRecorder()
	router.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", wAuth.Code)
	}
	if !strings.Contains(wAuth.Body.String(), `"data"`) {
		t.Errorf("expected json response containing 'data', got %s", wAuth.Body.String())
	}

	// 3. Count issues with status filter -> 200 OK
	reqCount := httptest.NewRequest(http.MethodGet, "/count?status=OPEN", nil).WithContext(ctx)
	wCount := httptest.NewRecorder()
	router.ServeHTTP(wCount, reqCount)
	if wCount.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", wCount.Code)
	}
	if !strings.Contains(wCount.Body.String(), `"count"`) {
		t.Errorf("expected json response containing 'count', got %s", wCount.Body.String())
	}
}
