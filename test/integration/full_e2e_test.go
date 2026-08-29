package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agents/deliberation"
	"github.com/scandrix/backend/internal/analytics/dora"
	"github.com/scandrix/backend/internal/api/streaming"
	"github.com/scandrix/backend/internal/codeanalysis/ast"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

func TestCompleteEndToEndEnterpriseReviewCycle(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	repoNamespace := "acme/payment-gateway"
	webhookSecret := "production_webhook_secret_key_123"

	// 1. Setup Infrastructure
	outbox := relay.NewOutboxStore()
	inbox := relay.NewInboxDeduplicator()
	streamBroker := streaming.NewStreamBroker()
	doraStore := dora.NewDORAStore()
	astAnalyzer := ast.NewASTComplexityAnalyzer()
	deliberator := deliberation.NewAgentDeliberator(0.70)

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating Ed25519 keys: %v", err)
	}
	attestor := intoto.NewProvenanceAttestor("scandrix-key-1", privKey, pubKey)

	// 2. Step 1: Webhook Ingestion Receiver (Fire-and-forget to Outbox)
	secretResolver := ingestion.NewStaticSecretResolver(map[string]string{
		"github": webhookSecret,
	})
	webhookHandler := ingestion.NewIngestionHandler(secretResolver, outbox)

	webhookPayload := []byte(`{
		"action": "opened",
		"number": 240,
		"pull_request": {
			"title": "Add payment processing endpoint",
			"head": {"sha": "head_sha_e2e_999"},
			"base": {"sha": "base_main_sha"},
			"user": {"login": "dev_alice"}
		},
		"repository": {
			"full_name": "acme/payment-gateway"
		}
	}`)

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(webhookPayload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(webhookPayload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()

	webhookHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec.Code)
	}

	// Verify message in Outbox
	claimedMsgs, err := outbox.ClaimPending(ctx, "worker-relay", 10, 1*time.Minute)
	if err != nil || len(claimedMsgs) != 1 {
		t.Fatalf("expected 1 claimed outbox message, got %d (err: %v)", len(claimedMsgs), err)
	}
	taskMsg := claimedMsgs[0]

	// 3. Step 2: Queue Consumer with Inbox Deduplication & Pipeline Execution
	var pipelineExecuted bool
	reviewExecutor := func(ctx context.Context, task consumer.ReviewTaskPayload) ([]models.CodeFinding, error) {
		pipelineExecuted = true

		// A. Real-time Stream: Stage Transition
		streamBroker.Broadcast(streaming.StreamEvent{
			ReviewID: task.TaskID,
			Type:     streaming.EventStageTransition,
			Payload: streaming.StageTransitionPayload{
				StageName: "AST_ANALYSIS",
				Progress:  0.25,
				Message:   "Inspecting code complexity",
			},
		})

		// B. AST & Complexity Analysis
		sampleCode := `package payment
func Process(amount int) bool {
	if amount <= 0 {
		return false
	}
	return true
}`
		rep, err := astAnalyzer.Analyze("payment.go", sampleCode)
		if err != nil || len(rep.Functions) == 0 {
			t.Fatalf("AST analysis failed: %v", err)
		}

		// C. Multi-Agent Deliberation
		cand := deliberation.ProposeFinding(
			deliberation.PersonaSecurityAuditor,
			"payment.go", 2, 4,
			"Potential negative amount validation bypass",
			models.SeverityHigh, 0.95,
			"Amount parameter must be checked for overflow",
			"Use uint64 or checked arithmetic",
		)
		critique := deliberation.ProposeCritique(
			cand.ID,
			deliberation.PersonaCleanCodeReviewer,
			deliberation.VerdictAgree,
			"Agree, amount checks should use strict unsigned or bounded bounds",
			0.0,
		)

		findings, decisions := deliberator.DeliberateExec(ctx, []deliberation.CandidateFinding{cand}, []deliberation.PeerCritique{critique})
		if len(findings) != 1 || len(decisions) != 1 {
			t.Fatalf("expected 1 surviving consensus finding, got %d", len(findings))
		}

		// D. Real-time Stream: Finding Discovered
		streamBroker.Broadcast(streaming.StreamEvent{
			ReviewID: task.TaskID,
			Type:     streaming.EventFindingDiscovered,
			Payload:  findings[0],
		})

		return findings, nil
	}

	queueConsumer := consumer.NewReviewConsumer(consumer.ConsumerConfig{
		MaxRetries: 3,
	}, inbox, reviewExecutor)

	taskPayload := consumer.ReviewTaskPayload{
		TaskID:            taskMsg.ID,
		EventID:           taskMsg.ID,
		WorkspaceID:       wsID,
		Provider:          models.ProviderGitHub,
		RepoNamespace:     repoNamespace,
		PullRequestNumber: 240,
		HeadSHA:           "head_sha_e2e_999",
	}

	// Connect SSE subscriber to receive real-time updates
	clientSub := streamBroker.Subscribe(taskPayload.TaskID, "client-dashboard")
	defer streamBroker.Unsubscribe(clientSub)

	// Execute task through consumer
	execResult := queueConsumer.ProcessTask(ctx, taskPayload)
	if execResult.Status != consumer.TaskStatusSuccess {
		t.Fatalf("expected task execution SUCCESS, got %s: %s", execResult.Status, execResult.ErrorMsg)
	}
	if !pipelineExecuted {
		t.Fatal("review pipeline callback was not executed")
	}

	// 4. Step 3: Cryptographic Attestation Generation (in-toto SLSA)
	attestationPred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           wsID,
		RepoNamespace:         repoNamespace,
		CommitSHA:             taskPayload.HeadSHA,
		PullRequestNumber:     240,
		Decision:              intoto.DecisionApproved,
		TotalFindings:         1,
		CriticalCount:         0,
		HighCount:             1,
		ConsensusScore:        0.85,
		EvaluatedRules:        []string{"SEC001_AMOUNT_VALIDATION"},
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-multi-agent-v1",
	}

	dsseEnvelope, err := attestor.AttestAndSign(attestationPred)
	if err != nil {
		t.Fatalf("attestation generation failed: %v", err)
	}

	verifiedStmt, err := attestor.VerifyEnvelope(dsseEnvelope, pubKey)
	if err != nil {
		t.Fatalf("attestation verification failed: %v", err)
	}
	if verifiedStmt.Predicate.CommitSHA != taskPayload.HeadSHA {
		t.Fatalf("commit SHA mismatch: got %s, want %s", verifiedStmt.Predicate.CommitSHA, taskPayload.HeadSHA)
	}

	// 5. Step 4: DORA Metrics Record & Velocity Report
	now := time.Now().UTC()
	err = doraStore.RecordDeployment(ctx, dora.DeploymentRecord{
		WorkspaceID:   wsID,
		RepoNamespace: repoNamespace,
		CommitSHA:     taskPayload.HeadSHA,
		DeployedAt:    now,
		LeadDuration:  45 * time.Minute,
		IsFailed:      false,
	})
	if err != nil {
		t.Fatalf("failed recording DORA deployment: %v", err)
	}

	doraReport, err := doraStore.GenerateReport(ctx, wsID, now.Add(-24*time.Hour), now.Add(1*time.Hour), 1, 1)
	if err != nil {
		t.Fatalf("failed generating DORA report: %v", err)
	}
	if doraReport.OverallTier != dora.TierElite {
		t.Fatalf("expected ELITE DORA tier, got %s", doraReport.OverallTier)
	}

	// 6. Step 5: Verify Real-Time SSE Events Received
	eventsCount := 0
drainLoop:
	for {
		select {
		case <-clientSub.EventChan:
			eventsCount++
		default:
			break drainLoop
		}
	}
	if eventsCount != 2 {
		t.Fatalf("expected 2 real-time broadcast events, got %d", eventsCount)
	}
}
