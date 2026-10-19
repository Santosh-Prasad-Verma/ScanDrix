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

func TestAutomationControllerEndpoints(t *testing.T) {
	ctrl := controllers.NewAutomationController(nil)
	router := ctrl.Routes()
	wsID := uuid.New()

	// 1. Missing auth context -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}

	// 2. Authorized request -> 200 OK
	ctx := context.WithValue(context.Background(), auth.WorkspaceContextKey, wsID)
	reqAuth := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	wAuth := httptest.NewRecorder()
	router.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", wAuth.Code)
	}
	if !strings.Contains(wAuth.Body.String(), `"data"`) {
		t.Errorf("expected json data response, got %s", wAuth.Body.String())
	}

	// 3. Authorized toggle request -> 200 OK
	ruleID := uuid.New()
	toggleBody := strings.NewReader(`{"enabled":true}`)
	reqToggle := httptest.NewRequest(http.MethodPatch, "/"+ruleID.String()+"/toggle", toggleBody).WithContext(ctx)
	wToggle := httptest.NewRecorder()
	router.ServeHTTP(wToggle, reqToggle)
	if wToggle.Code != http.StatusOK {
		t.Errorf("expected 200 OK for toggle, got %d", wToggle.Code)
	}
	if !strings.Contains(wToggle.Body.String(), `"updated"`) {
		t.Errorf("expected updated status in toggle response, got %s", wToggle.Body.String())
	}

	// 4. Authorized delete request -> 200 OK
	reqDelete := httptest.NewRequest(http.MethodDelete, "/"+ruleID.String(), nil).WithContext(ctx)
	wDelete := httptest.NewRecorder()
	router.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusOK {
		t.Errorf("expected 200 OK for delete, got %d", wDelete.Code)
	}
	if !strings.Contains(wDelete.Body.String(), `"deleted"`) {
		t.Errorf("expected deleted status in delete response, got %s", wDelete.Body.String())
	}
}
