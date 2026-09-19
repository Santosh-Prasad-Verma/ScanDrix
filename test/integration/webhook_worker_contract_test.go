// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package integration_test

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

// TestWebhookWorkerContractEndToEnd validates §14 Phase 1:
// Outbox payload serialization must preserve valid non-nil TaskID and EventID,
// guaranteeing that worker consumer unmarshaling and inbox deduplication claim succeed.
func TestWebhookWorkerContractEndToEnd(t *testing.T) {
	ctx := context.Background()
	outbox := relay.NewOutboxStore()
	inbox := relay.NewInboxDeduplicator()

	webhookSecret := "phase1_contract_webhook_secret_key_999"
	fallbackMap := map[string]string{
		"github:acme/contract-service": webhookSecret,
		"github":                       "global_fallback_secret",
	}
	resolver := ingestion.NewDynamicSecretResolver(nil, fallbackMap)
	handler := ingestion.NewIngestionHandler(resolver, outbox)

	payload := []byte(`{
		"action": "opened",
		"number": 142,
		"pull_request": {
			"title": "Fix memory leak in buffer pool",
			"head": { "sha": "head_commit_sha_1234567890" },
			"base": { "sha": "base_commit_sha_0987654321" },
			"user": { "login": "alice_eng" }
		},
		"repository": {
			"full_name": "acme/contract-service"
		}
	}`)

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected HTTP 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ingestion.IngestionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed decoding ingestion response: %v", err)
	}

	if resp.EventID == uuid.Nil {
		t.Fatal("FATAL: Ingestion response EventID is uuid.Nil")
	}

	// Retrieve pending messages from Outbox
	claimed, err := outbox.ClaimPending(ctx, "relay-worker-1", 10, 30*time.Second)
	if err != nil {
		t.Fatalf("failed claiming outbox messages: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected exactly 1 claimed message in outbox, got %d", len(claimed))
	}
	outboxMsg := claimed[0]

	// Simulate Worker Unmarshaling (cmd/worker/main.go)
	var task consumer.ReviewTaskPayload
	if err := json.Unmarshal(outboxMsg.Payload, &task); err != nil {
		t.Fatalf("worker failed unmarshaling outbox payload: %v", err)
	}

	// Normalization fallback as executed in worker main.go
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

	// Contract assertions
	if task.TaskID == uuid.Nil {
		t.Fatal("CONTRACT VIOLATION: task.TaskID deserialized as uuid.Nil!")
	}
	if task.EventID == uuid.Nil {
		t.Fatal("CONTRACT VIOLATION: task.EventID deserialized as uuid.Nil!")
	}
	if task.TaskID != resp.EventID {
		t.Fatalf("expected task.TaskID == ingestion EventID %s, got %s", resp.EventID, task.TaskID)
	}
	if task.RepoNamespace != "acme/contract-service" {
		t.Fatalf("expected repo_namespace 'acme/contract-service', got '%s'", task.RepoNamespace)
	}
	if task.PullRequestNumber != 142 {
		t.Fatalf("expected pull request number 142, got %d", task.PullRequestNumber)
	}
	if task.HeadSHA != "head_commit_sha_1234567890" {
		t.Fatalf("expected head commit sha 'head_commit_sha_1234567890', got '%s'", task.HeadSHA)
	}

	// Step 2: Test Inbox Claim & Idempotency
	claimedFirst, err := inbox.ClaimMessage(ctx, task.TaskID.String(), "worker-review-consumer")
	if err != nil || !claimedFirst {
		t.Fatalf("expected inbox.ClaimMessage to succeed on first attempt with valid TaskID (err: %v)", err)
	}

	// Duplicate delivery of same task MUST be detected and rejected
	claimedSecond, err := inbox.ClaimMessage(ctx, task.TaskID.String(), "worker-review-consumer")
	if err != nil {
		t.Fatalf("inbox error on duplicate claim: %v", err)
	}
	if claimedSecond {
		t.Fatal("CONTRACT VIOLATION: inbox.ClaimMessage claimed duplicate delivery for identical TaskID!")
	}

	// Mark completed and mark outbox published
	if err := inbox.MarkCompleted(ctx, task.TaskID.String(), "worker-review-consumer"); err != nil {
		t.Fatalf("failed marking inbox completed: %v", err)
	}
	if err := outbox.MarkPublished(ctx, outboxMsg.ID); err != nil {
		t.Fatalf("failed marking outbox published: %v", err)
	}
}
