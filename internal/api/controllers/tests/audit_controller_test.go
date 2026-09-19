package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
)

func TestAuditControllerListAndExport(t *testing.T) {
	ctrl := controllers.NewAuditController(nil)
	wsID := uuid.New()

	r := chi.NewRouter()
	r.Route("/workspaces/{workspaceId}/audit-logs", func(sub chi.Router) {
		sub.Mount("/", ctrl.Routes())
	})

	// 1. Test List Audit Logs (with authenticated workspace context)
	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list audit logs, got: %d", w.Code)
	}

	// 2. Test BOLA prevention: attempting to access another workspace returns 403
	otherWsID := uuid.New()
	reqBOLA := httptest.NewRequest(http.MethodGet, "/workspaces/"+otherWsID.String()+"/audit-logs", nil)
	reqBOLA = reqBOLA.WithContext(context.WithValue(reqBOLA.Context(), auth.WorkspaceContextKey, wsID))
	wBOLA := httptest.NewRecorder()
	r.ServeHTTP(wBOLA, reqBOLA)

	if wBOLA.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-tenant audit access, got: %d", wBOLA.Code)
	}

	// 3. Test Export CEF
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=cef", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for CEF export, got: %d", w.Code)
	}

	// 4. Test Export JSON
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=json", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for JSON export, got: %d", w.Code)
	}

	// 5. Test invalid workspace UUID
	req = httptest.NewRequest(http.MethodGet, "/workspaces/invalid-uuid/audit-logs", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid uuid, got: %d", w.Code)
	}
}
