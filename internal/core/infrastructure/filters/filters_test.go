package filters

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExceptionsFilterPg22P02(t *testing.T) {
	filter := NewExceptionsFilter("test-api")
	req := httptest.NewRequest("GET", "/api/v1/workspaces/invalid-id", nil)
	w := httptest.NewRecorder()

	pgErr := errors.New("ERROR: invalid input syntax for type uuid: 22P02")
	filter.HandleHTTP(w, req, pgErr)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Invalid parameter format") {
		t.Fatalf("expected Invalid parameter format message, got %s", body)
	}
}

func TestExceptionsFilterCustomException(t *testing.T) {
	filter := NewExceptionsFilter("test-api")
	req := httptest.NewRequest("POST", "/api/v1/workspaces", nil)
	w := httptest.NewRecorder()

	dupErr := &DuplicateRecordException{
		Entity: "Workspace",
		Key:    "ws-12345",
	}
	filter.HandleHTTP(w, req, dupErr)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "DUPLICATE_RECORD") {
		t.Fatalf("expected DUPLICATE_RECORD error key, got %s", body)
	}
}

func TestExceptionsFilterInternalError(t *testing.T) {
	filter := NewExceptionsFilter("test-api")
	req := httptest.NewRequest("GET", "/api/v1/broken", nil)
	w := httptest.NewRecorder()

	filter.HandleHTTP(w, req, errors.New("unexpected database network timeout"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}
