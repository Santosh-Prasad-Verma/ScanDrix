package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
)

func TestAuditControllerListAndExport(t *testing.T) {
	ctrl := controllers.NewAuditController(nil)
	wsID := uuid.New()

	r := chi.NewRouter()
	r.Route("/workspaces/{workspaceId}/audit-logs", func(sub chi.Router) {
		sub.Mount("/", ctrl.Routes())
	})

	// 1. Test List Audit Logs (empty repository fallback)
	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list audit logs, got: %d", w.Code)
	}

	// 2. Test Export CEF
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=cef", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for CEF export, got: %d", w.Code)
	}

	// 3. Test Export JSON
	req = httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/export?format=json", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for JSON export, got: %d", w.Code)
	}

	// 4. Test invalid workspace UUID
	req = httptest.NewRequest(http.MethodGet, "/workspaces/invalid-uuid/audit-logs", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid uuid, got: %d", w.Code)
	}
}
