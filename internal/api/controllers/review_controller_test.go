package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/provenance/intoto"
)

func TestReviewControllerAttestationEndpoints(t *testing.T) {
	ctrl := controllers.NewReviewController(nil, nil, nil)
	wsID := uuid.New()
	reviewID := uuid.New()

	router := ctrl.Routes()

	// 1. Test Get Attestations for Review (nil repo returns empty list)
	req := httptest.NewRequest(http.MethodGet, "/"+reviewID.String()+"/attestation", nil)
	ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get attestation, got: %d", rec.Code)
	}

	// 2. Test Cryptographic Verification Endpoint with Real Signed DSSE Envelope
	masterSecret := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
	if masterSecret == "" {
		masterSecret = os.Getenv("KMS_MASTER_KEY")
	}
	privKey, pubKey, keyID := intoto.DeriveTenantKeypair(wsID, masterSecret)
	attestor := intoto.NewProvenanceAttestor(keyID, privKey, pubKey)

	pred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           wsID,
		RepoNamespace:         "acme/service",
		CommitSHA:             "1234567890abcdef1234567890abcdef12345678",
		PullRequestNumber:     10,
		Decision:              intoto.DecisionApproved,
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-agent-consensus-v1",
	}
	env, err := attestor.AttestAndSign(pred)
	if err != nil {
		t.Fatalf("failed signing envelope: %v", err)
	}

	verifyBody, _ := json.Marshal(map[string]any{
		"envelope": env,
	})
	req = httptest.NewRequest(http.MethodPost, "/"+reviewID.String()+"/attestation/verify", bytes.NewReader(verifyBody))
	req = req.WithContext(ctx)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for verify attestation, got %d: %s", rec.Code, rec.Body.String())
	}

	var verifyResp struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&verifyResp); err != nil || !verifyResp.Valid {
		t.Fatalf("expected valid verification outcome, got: %+v", verifyResp)
	}
}
