package ingestion_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/queue/consumer"
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

	// 6. Redelivery of same delivery UUID should be idempotently accepted with 202
	reqRedeliver := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(ghPayload))
	reqRedeliver.Header.Set("X-GitHub-Event", "pull_request")
	reqRedeliver.Header.Set("X-Hub-Signature-256", validSig)
	reqRedeliver.Header.Set("X-GitHub-Delivery", deliveryUUID)
	wRedeliver := httptest.NewRecorder()

	handler.ServeHTTP(wRedeliver, reqRedeliver)
	if wRedeliver.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on duplicate redelivery, got %d", wRedeliver.Code)
	}
}

func TestGitHubInstallationLifecycleEvents(t *testing.T) {
	outbox := relay.NewOutboxStore()
	secret := "install_secret_test_456"
	resolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github:acme-org": secret,
	})
	handler := ingestion.NewIngestionHandler(resolver, outbox)

	// 1. Installation created
	createPayload := []byte(`{
		"action": "created",
		"installation": {
			"id": 987654,
			"account": {"login": "acme-org"}
		},
		"repositories": [
			{"full_name": "acme-org/repo-1"},
			{"full_name": "acme-org/repo-2"}
		],
		"sender": {"login": "org-admin"}
	}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(createPayload)
	sigCreate := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	reqCreate := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(createPayload))
	reqCreate.Header.Set("X-GitHub-Event", "installation")
	reqCreate.Header.Set("X-Hub-Signature-256", sigCreate)
	wCreate := httptest.NewRecorder()

	handler.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on installation created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	// 2. Installation deleted (uninstall)
	deletePayload := []byte(`{
		"action": "deleted",
		"installation": {
			"id": 987654,
			"account": {"login": "acme-org"}
		},
		"sender": {"login": "org-admin"}
	}`)

	macDel := hmac.New(sha256.New, []byte(secret))
	macDel.Write(deletePayload)
	sigDel := "sha256=" + hex.EncodeToString(macDel.Sum(nil))

	reqDel := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(deletePayload))
	reqDel.Header.Set("X-GitHub-Event", "installation")
	reqDel.Header.Set("X-Hub-Signature-256", sigDel)
	wDel := httptest.NewRecorder()

	handler.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on installation deleted, got %d: %s", wDel.Code, wDel.Body.String())
	}
}

func TestOutboxPayloadContractParityWithWorker(t *testing.T) {
	outbox := relay.NewOutboxStore()
	secret := "contract_test_secret"
	resolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github:acme/contract-test": secret,
	})
	handler := ingestion.NewIngestionHandler(resolver, outbox)

	payload := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 99,
			"title": "Contract parity test PR",
			"head": {"sha": "headsha12345"},
			"base": {"sha": "basesha12345"},
			"user": {"login": "dev-user"}
		},
		"repository": {
			"full_name": "acme/contract-test"
		}
	}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Delivery", "11111111-2222-3333-4444-555555555555")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", w.Code, w.Body.String())
	}

	messages, err := outbox.ClaimPending(context.Background(), "test-worker", 10, time.Minute)
	if err != nil || len(messages) == 0 {
		t.Fatalf("expected at least 1 outbox message to be claimed, err: %v", err)
	}

	lastMsg := messages[len(messages)-1]

	// Simulate Worker Unmarshal (cmd/worker/main.go)
	var task consumer.ReviewTaskPayload
	if err := json.Unmarshal(lastMsg.Payload, &task); err != nil {
		t.Fatalf("failed unmarshaling outbox payload into consumer.ReviewTaskPayload: %v", err)
	}

	// Normalization logic as in worker main.go
	if task.TaskID == uuid.Nil {
		if task.EventID != uuid.Nil {
			task.TaskID = task.EventID
		} else if task.ID != uuid.Nil {
			task.TaskID = task.ID
		}
	}
	if task.EventID == uuid.Nil {
		task.EventID = task.TaskID
	}

	if task.TaskID == uuid.Nil {
		t.Fatal("FATAL: task.TaskID unmarshaled as uuid.Nil!")
	}
	if task.EventID == uuid.Nil {
		t.Fatal("FATAL: task.EventID unmarshaled as uuid.Nil!")
	}
	if task.TaskID != lastMsg.ID {
		t.Fatalf("expected task.TaskID == outbox message ID %s, got %s", lastMsg.ID, task.TaskID)
	}
	if task.RepoNamespace != "acme/contract-test" {
		t.Fatalf("expected repo_namespace 'acme/contract-test', got %s", task.RepoNamespace)
	}
	if task.PullRequestNumber != 99 {
		t.Fatalf("expected PR number 99, got %d", task.PullRequestNumber)
	}
}

func TestDynamicSecretResolver(t *testing.T) {
	fallbackMap := map[string]string{
		"github:custom/repo": "repo_specific_fallback_secret",
		"github":             "global_github_secret",
		"gitlab":             "global_gitlab_secret",
	}

	resolver := ingestion.NewDynamicSecretResolver(nil, fallbackMap)

	// 1. Repo-specific fallback match
	s1, err := resolver.ResolveSecret("github", "custom/repo")
	if err != nil || s1 != "repo_specific_fallback_secret" {
		t.Fatalf("expected 'repo_specific_fallback_secret', got '%s' (err: %v)", s1, err)
	}

	// 2. Global provider fallback match
	s2, err := resolver.ResolveSecret("github", "other/repo")
	if err != nil || s2 != "global_github_secret" {
		t.Fatalf("expected 'global_github_secret', got '%s' (err: %v)", s2, err)
	}

	// 3. Different provider global fallback match
	s3, err := resolver.ResolveSecret("gitlab", "any/project")
	if err != nil || s3 != "global_gitlab_secret" {
		t.Fatalf("expected 'global_gitlab_secret', got '%s' (err: %v)", s3, err)
	}

	// 4. Unknown provider returns empty string without error
	s4, err := resolver.ResolveSecret("unknown_provider", "foo/bar")
	if err != nil || s4 != "" {
		t.Fatalf("expected empty secret for unknown provider, got '%s' (err: %v)", s4, err)
	}
}

func TestWebhookIngestion_DismissalComment(t *testing.T) {
	parser := ingestion.NewWebhookParser()
	payload := []byte(`{
		"action": "created",
		"issue": {
			"number": 42
		},
		"comment": {
			"id": 1001,
			"body": "@scandrix dismiss false-positive: verified test mock token",
			"path": "pkg/auth/token.go",
			"user": {
				"login": "security-lead"
			}
		},
		"repository": {
			"full_name": "acme/payments-api"
		}
	}`)

	event, err := parser.Parse("github", "issue_comment", payload)
	if err != nil {
		t.Fatalf("failed parsing issue_comment: %v", err)
	}

	if event.Action != ingestion.ActionFeedbackDismissed {
		t.Fatalf("expected ActionFeedbackDismissed, got %s", event.Action)
	}
	if event.DismissalReason != "FALSE_POSITIVE" {
		t.Fatalf("expected DismissalReason FALSE_POSITIVE, got %s", event.DismissalReason)
	}
	if event.PullRequestNumber != 42 {
		t.Errorf("expected PR #42, got %d", event.PullRequestNumber)
	}
}



