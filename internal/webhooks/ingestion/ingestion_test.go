package ingestion_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
)

func TestWebhookIngestionAndSignatureVerification(t *testing.T) {
	outbox := relay.NewOutboxStore()
	secret := "super_secret_webhook_key_123"

	resolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github:acme/backend": secret,
		"gitlab:acme/backend": secret,
		"bitbucket":           secret,
	})

	handler := ingestion.NewIngestionHandler(resolver, outbox)

	// 1. GitHub Pull Request Event with Valid Signature
	ghPayload := []byte(`{
		"action": "opened",
		"number": 105,
		"pull_request": {
			"title": "feat: enterprise scim provider",
			"head": {"sha": "sha_head_105"},
			"base": {"sha": "sha_base_main"},
			"user": {"login": "octocat"}
		},
		"repository": {
			"full_name": "acme/backend"
		}
	}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(ghPayload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(ghPayload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", validSig)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", w.Code, w.Body.String())
	}

	var res ingestion.IngestionResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Status != "accepted" {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}

	// Verify event written to outbox
	lag, err := outbox.GetLag(req.Context())
	if err != nil || lag.PendingCount != 1 {
		t.Fatalf("expected 1 outbox message queued, got %+v", lag)
	}

	// 2. GitHub Event with Invalid Signature (must fail with 401)
	reqBadSig := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(ghPayload))
	reqBadSig.Header.Set("X-GitHub-Event", "pull_request")
	reqBadSig.Header.Set("X-Hub-Signature-256", "sha256=invalid_signature_hex")
	wBad := httptest.NewRecorder()

	handler.ServeHTTP(wBad, reqBadSig)

	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on forged signature, got %d", wBad.Code)
	}

	// 3. GitLab Merge Request Event
	glPayload := []byte(`{
		"object_kind": "merge_request",
		"project": {"path_with_namespace": "acme/backend"},
		"user": {"username": "gitlab_dev"},
		"object_attributes": {
			"iid": 42,
			"title": "Fix auth flow",
			"action": "open",
			"last_commit": {"id": "gl_commit_sha_123"}
		}
	}`)

	reqGL := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(glPayload))
	reqGL.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	reqGL.Header.Set("X-Gitlab-Token", secret)
	wGL := httptest.NewRecorder()

	handler.ServeHTTP(wGL, reqGL)

	if wGL.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for gitlab, got %d: %s", wGL.Code, wGL.Body.String())
	}

	// 4. GitHub Closed PR Event (Ignored without queuing)
	closedPayload := []byte(`{
		"action": "closed",
		"number": 105,
		"pull_request": {"title": "closed pr", "head": {"sha": "1"}, "base": {"sha": "2"}, "user": {"login": "dev"}},
		"repository": {"full_name": "acme/backend"}
	}`)
	macClosed := hmac.New(sha256.New, []byte(secret))
	macClosed.Write(closedPayload)
	sigClosed := "sha256=" + hex.EncodeToString(macClosed.Sum(nil))

	reqClosed := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(closedPayload))
	reqClosed.Header.Set("X-GitHub-Event", "pull_request")
	reqClosed.Header.Set("X-Hub-Signature-256", sigClosed)
	wClosed := httptest.NewRecorder()

	handler.ServeHTTP(wClosed, reqClosed)

	if wClosed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ignored action, got %d", wClosed.Code)
	}

	// 5. GitHub Event with Delivery ID for Deduplication & Replay Protection
	deliveryUUID := "72d3162e-cc78-11e3-81ab-4c9367dc0958"
	reqDelivery := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(ghPayload))
	reqDelivery.Header.Set("X-GitHub-Event", "pull_request")
	reqDelivery.Header.Set("X-Hub-Signature-256", validSig)
	reqDelivery.Header.Set("X-GitHub-Delivery", deliveryUUID)
	wDelivery := httptest.NewRecorder()

	handler.ServeHTTP(wDelivery, reqDelivery)
	if wDelivery.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for delivery with header, got %d", wDelivery.Code)
	}

	var resDelivery ingestion.IngestionResult
	if err := json.Unmarshal(wDelivery.Body.Bytes(), &resDelivery); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}
	if resDelivery.EventID.String() != deliveryUUID {
		t.Fatalf("expected EventID %s from X-GitHub-Delivery, got %s", deliveryUUID, resDelivery.EventID)
	}
}

