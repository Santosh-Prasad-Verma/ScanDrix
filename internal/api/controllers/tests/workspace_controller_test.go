package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
)

func TestWorkspaceControllerAttestationKey(t *testing.T) {
	ctrl := controllers.NewWorkspaceController(nil)
	wsID := uuid.New()

	router := ctrl.Routes()

	// 1. Test Get Attestation Public Key
	req := httptest.NewRequest(http.MethodGet, "/attestation-key", nil)
	ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for attestation key, got: %d", rec.Code)
	}

	var resp struct {
		WorkspaceID string `json:"workspace_id"`
		KeyID       string `json:"key_id"`
		Algorithm   string `json:"algorithm"`
		PublicKey   string `json:"public_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding attestation key response: %v", err)
	}

	if resp.Algorithm != "Ed25519" {
		t.Fatalf("expected Ed25519 algorithm, got: %s", resp.Algorithm)
	}
	if !strings.HasPrefix(resp.PublicKey, "-----BEGIN PUBLIC KEY-----") {
		t.Fatalf("expected PEM format public key, got: %s", resp.PublicKey)
	}
}
