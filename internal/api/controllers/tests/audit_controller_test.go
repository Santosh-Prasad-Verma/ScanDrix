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
	//
	// No repository is configured, so the honest answer is 503. It used to
	// answer 200 with an empty array, which a SOC reads as "zero security
	// events in this period" (AUDIT_REMEDIATION.md F-10).
	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for list audit logs with no repository, got: %d", w.Code)
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
	//
	// A compliance export must never answer 200 with an empty body: a SOC
	// consuming that concludes there were no security events. With no
	// repository the correct answer is 503 (AUDIT_REMEDIATION.md F-10).
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=cef", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for CEF export with no repository, got: %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatalf("a 503 export must explain itself, got an empty body")
	}

	// 4. Test Export JSON
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=json", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for JSON export with no repository, got: %d", w.Code)
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
