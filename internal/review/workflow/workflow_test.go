package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/review/services"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestByokConcurrencyGate(t *testing.T) {
	gate := NewByokConcurrencyGate(2) // max 2 concurrent
	ctx := context.Background()

	// Acquire slot 1
	rel1, err := gate.AcquireSlot(ctx, "org_1", "openai", "gpt-4o")
	if err != nil {
		t.Fatalf("failed to acquire slot 1: %v", err)
	}

	// Acquire slot 2
	rel2, err := gate.AcquireSlot(ctx, "org_1", "openai", "gpt-4o")
	if err != nil {
		t.Fatalf("failed to acquire slot 2: %v", err)
	}

	// Slot 3 should block with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err = gate.AcquireSlot(ctxTimeout, "org_1", "openai", "gpt-4o")
	if err == nil {
		t.Fatalf("expected slot 3 to timeout, but it succeeded")
	}

	// Release slot 1
	rel1()

	// Now slot 3 should succeed
	ctxTimeout2, cancel2 := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel2()

	rel3, err := gate.AcquireSlot(ctxTimeout2, "org_1", "openai", "gpt-4o")
	if err != nil {
		t.Fatalf("failed to acquire slot after release: %v", err)
	}

	rel2()
	rel3()
}

func TestPrReviewDeferralService(t *testing.T) {
	deferral := NewPrReviewDeferralService()
	ctx := context.Background()

	repoID := "repo-123"
	prNumber := 42
	headSHA := "abc1234"

	// 1. Initially should not defer
	shouldDefer, err := deferral.ShouldDefer(ctx, repoID, prNumber, headSHA)
	if err != nil || shouldDefer {
		t.Fatalf("expected shouldDefer to be false, got %v, err: %v", shouldDefer, err)
	}

	// 2. Mark started
	err = deferral.MarkStarted(ctx, repoID, prNumber, headSHA)
	if err != nil {
		t.Fatalf("failed to mark started: %v", err)
	}

	// 3. Now should defer because review is active
	shouldDefer, _ = deferral.ShouldDefer(ctx, repoID, prNumber, headSHA)
	if !shouldDefer {
		t.Fatalf("expected shouldDefer to be true while review is running")
	}

	// 4. Exponential backoff calculation
	delay, count, ok := deferral.CalculateDelay(repoID, prNumber)
	if !ok || count != 1 || delay != 15*time.Second {
		t.Errorf("expected count 1 with 15s delay, got count %d, delay %v", count, delay)
	}

	// 5. Mark completed
	err = deferral.MarkCompleted(ctx, repoID, prNumber, headSHA)
	if err != nil {
		t.Fatalf("failed to mark completed: %v", err)
	}

	// 6. Should no longer defer
	shouldDefer, _ = deferral.ShouldDefer(ctx, repoID, prNumber, headSHA)
	if shouldDefer {
		t.Fatalf("expected shouldDefer to be false after completion")
	}
}

func TestReviewJobProcessor(t *testing.T) {
	catalog := rules.DefaultCatalog()
	evaluator, _ := rules.NewEvaluator(catalog)

	strategy := pipeline.NewCanonicalPipelineStrategy(
		stages.NewPrerequisitesStage(),
		stages.NewValidateNewCommitsStage(),
		stages.NewResolveConfigStage(nil),
		stages.NewValidateConfigStage(),
		stages.NewFileFilterStage(),
		stages.NewExternalContextStage(),
		stages.NewInitialCommentStage(nil),
		stages.NewBusinessLogicValidationStage(),
		stages.NewCreateSandboxStage(nil),
		stages.NewASTAnalysisStage(evaluator),
		stages.NewPRSummaryStage(),
		stages.NewSuggestionValidatorStage(),
		stages.NewSCMPublisherStage(1*time.Millisecond),
		stages.NewAggregateResultsStage(),
		stages.NewFinishCommentsStage(nil, services.NewMessageTemplateProcessor()),
		stages.NewFinishProcessReviewStage(nil),
	)

	deferral := NewPrReviewDeferralService()
	gate := NewByokConcurrencyGate(5)
	processor := NewReviewJobProcessor(strategy, deferral, gate)

	payload := ReviewJobPayload{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "scandrix/backend",
		Provider:      models.ProviderGitHub,
		PullNumber:    77,
		Title:         "fix: handle concurrent slot acquisitions",
		HeadSHA:       "headsha123",
		BaseSHA:       "basesha123",
		Author:        "developer@scandrix.dev",
		RawDiff: `diff --git a/pkg/service.go b/pkg/service.go
new file mode 100644
--- /dev/null
+++ b/pkg/service.go
@@ -0,0 +1,5 @@
+package pkg
+
+func Do() {}
+`,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	ctx := context.Background()
	err = processor.ProcessReviewJob(ctx, data)
	if err != nil {
		t.Fatalf("ProcessReviewJob failed: %v", err)
	}

	// Verify concurrent execution gets deferred
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = deferral.MarkStarted(ctx, payload.RepositoryID.String(), payload.PullNumber, payload.HeadSHA)
	}()
	wg.Wait()

	collidingErr := processor.ProcessReviewJob(ctx, data)
	if collidingErr == nil {
		t.Fatalf("expected colliding job to be deferred, got nil error")
	}
}
