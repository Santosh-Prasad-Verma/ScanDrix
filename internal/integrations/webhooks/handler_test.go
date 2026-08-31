package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "super-secure-webhook-secret"
	body := []byte(`{"action":"opened","number":42}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validHeader := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(secret, body, validHeader) {
		t.Error("expected signature to be valid")
	}

	invalidHeader := "sha256=1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if verifyGitHubSignature(secret, body, invalidHeader) {
		t.Error("expected invalid signature to be rejected")
	}
}

func TestVerifyGitLabToken(t *testing.T) {
	secret := "gitlab-secret-token-123"

	if !verifyGitLabToken(secret, "gitlab-secret-token-123") {
		t.Error("expected valid token to match")
	}

	if verifyGitLabToken(secret, "wrong-token") {
		t.Error("expected wrong token to be rejected")
	}

	if verifyGitLabToken(secret, "") {
		t.Error("expected empty token to be rejected")
	}

	if verifyGitLabToken("", "some-token") {
		t.Error("expected empty secret to be rejected")
	}
}

func TestHandleGitHubWebhook(t *testing.T) {
	secret := "test-secret"
	handler := NewIngestionHandler(nil, secret, "")

	// 1. Invalid signature
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w := httptest.NewRecorder()
	handler.HandleGitHub(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid signature, got: %d", w.Code)
	}

	// 2. Valid signature with ping event
	pingPayload := []byte(`{"zen":"Keep it logically awesome."}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(pingPayload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req = httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(pingPayload))
	req.Header.Set("X-Hub-Signature-256", validSig)
	req.Header.Set("X-GitHub-Event", "ping")
	w = httptest.NewRecorder()
	handler.HandleGitHub(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ping, got: %d", w.Code)
	}
}

func TestHandleGitLabWebhook(t *testing.T) {
	secret := "gitlab-secret"
	handler := NewIngestionHandler(nil, "", secret)

	// 1. Invalid token
	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Gitlab-Token", "wrong")
	w := httptest.NewRecorder()
	handler.HandleGitLab(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong token, got: %d", w.Code)
	}

	// 2. Valid token with unsupported event
	req = httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader([]byte(`{"object_kind":"pipeline"}`)))
	req.Header.Set("X-Gitlab-Token", secret)
	w = httptest.NewRecorder()
	handler.HandleGitLab(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ignored event, got: %d", w.Code)
	}
}
