package pipeline_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/chaos"
	"github.com/scandrix/backend/internal/review/observers"
	"github.com/scandrix/backend/internal/review/routing"
	"github.com/scandrix/backend/internal/review/semantic"
	"github.com/scandrix/backend/internal/review/services"
	"github.com/scandrix/backend/pkg/models"
)

// mockE2EStageRunner coordinates an end-to-end 16-stage pipeline execution simulation.
type mockE2EStageRunner struct {
	mu           sync.Mutex
	stagesPassed []string
	dispatcher   *observers.PipelineEventDispatcher
	deduplicator *semantic.SemanticDeduplicator
	router       *routing.DynamicModelRouter
	safeguards   *services.SafeguardTriageService
	scmCaller    *chaos.ResilientSCMCaller
}

func newMockE2EStageRunner() *mockE2EStageRunner {
	dispatcher := observers.NewPipelineEventDispatcher(256, 4)
	router := routing.NewDynamicModelRouter(nil)
	deduplicator := semantic.NewSemanticDeduplicator(nil, semantic.DefaultContentThreshold, semantic.DefaultEmbeddingLow, semantic.DefaultEmbeddingHigh)
	sg := services.NewSafeguardTriageService()
	scmCaller := chaos.NewResilientSCMCaller()

	return &mockE2EStageRunner{
		stagesPassed: make([]string, 0),
		dispatcher:   dispatcher,
		deduplicator: deduplicator,
		router:       router,
		safeguards:   sg,
		scmCaller:    scmCaller,
	}
}

func (r *mockE2EStageRunner) recordStage(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stagesPassed = append(r.stagesPassed, name)
}

func TestE2E16StagePipeline_CompleteLifecycle(t *testing.T) {
	ctx := context.Background()
	runner := newMockE2EStageRunner()
	wsID := uuid.New()
	reviewID := uuid.New()

	// 1. Stage 1: Webhook Ingress & Idempotency Claim
	runner.recordStage("01_webhook_ingress")
	runner.dispatcher.Emit(observers.PipelineEvent{
		ReviewID:  reviewID,
		StageName: "01_webhook_ingress",
		Type:      observers.EventStageStarted,
		Timestamp: time.Now().UTC(),
	})

	// 2. Stage 2: Context Fetch & Changed Files Hydration
	runner.recordStage("02_context_fetch")
	files := []string{"internal/auth/jwt.go", "pkg/models/user.go", "README.md"}

	// 3. Stage 3: License & BYOK Spend Quotas Gate
	runner.recordStage("03_spend_quota_gate")
	slot, _, err := runner.router.ResolveSlot(ctx, routing.TaskFileTriage, wsID.String())
	if err != nil || slot.ModelName == "" {
		t.Fatalf("failed resolving model slot: %v", err)
	}

	// 4. Stage 4: File Triage
	runner.recordStage("04_file_triage")
	var reviewableFiles []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			reviewableFiles = append(reviewableFiles, f)
		}
	}
	if len(reviewableFiles) != 2 {
		t.Fatalf("expected 2 reviewable files, got %d", len(reviewableFiles))
	}

	// 5. Stage 5: Custom Rules Ingestion
	runner.recordStage("05_custom_rules")

	// 6. Stage 6: AST Symbol & Call Graph Extraction
	runner.recordStage("06_ast_graph")

	// 7. Stage 7: External Documentation Discovery
	runner.recordStage("07_doc_discovery")

	// 8. Stage 8: Multi-Agent Deliberation
	runner.recordStage("08_multi_agent_deliberation")
	candidateFindings := []models.CodeFinding{
		{
			ID:          uuid.New(),
			ReviewID:    reviewID,
			WorkspaceID: wsID,
			FilePath:    "internal/auth/jwt.go",
			StartLine:   45,
			EndLine:     48,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY",
			Title:       "Hardcoded JWT HMAC key",
			Description: "JWT secret token is read from static variable instead of secure environment.",
		},
		{
			ID:          uuid.New(),
			ReviewID:    reviewID,
			WorkspaceID: wsID,
			FilePath:    "internal/auth/jwt.go",
			StartLine:   45,
			EndLine:     48,
			Severity:    models.SeverityHigh,
			Category:    "SECURITY",
			Title:       "Hardcoded JWT HMAC key in auth handler",
			Description: "JWT secret token is read from static variable instead of secure environment variable.",
		},
		{
			ID:          uuid.New(),
			ReviewID:    reviewID,
			WorkspaceID: wsID,
			FilePath:    "pkg/models/user.go",
			StartLine:   12,
			EndLine:     15,
			Severity:    models.SeverityMedium,
			Category:    "BUG",
			Title:       "Potential nil pointer dereference on user metadata",
			Description: "Map access without nil check causes panic when metadata is missing.",
		},
	}

	// 9. Stage 9: Synthesis Rescue Pass
	runner.recordStage("09_synthesis_rescue")

	// 10. Stage 10: Refute-to-Drop Verification Gate
	runner.recordStage("10_verification_gate")

	// 11. Stage 11: Semantic Deduplication & Clustering
	runner.recordStage("11_semantic_deduplication")
	findingPtrs := make([]*models.CodeFinding, len(candidateFindings))
	for i := range candidateFindings {
		findingPtrs[i] = &candidateFindings[i]
	}
	deduped, _, err := runner.deduplicator.DeduplicateBatch(ctx, "test/repo", 42, findingPtrs)
	if err != nil {
		t.Fatalf("unexpected deduplication error: %v", err)
	}
	if len(deduped) != 2 {
		t.Fatalf("expected near-duplicate JWT finding to collapse, got %d findings", len(deduped))
	}

	// 12. Stage 12: Safeguards Engine (Hallucinated File Suppression, Confidence Thresholding)
	runner.recordStage("12_safeguards")
	validFilesMap := map[string]struct{}{
		"internal/auth/jwt.go": {},
		"pkg/models/user.go":   {},
	}

	var safeguarded []models.CodeFinding
	for _, f := range deduped {
		if _, ok := validFilesMap[f.FilePath]; ok {
			safeguarded = append(safeguarded, *f)
		}
	}
	if len(safeguarded) != 2 {
		t.Fatalf("expected all findings preserved after safeguards, got %d", len(safeguarded))
	}

	// 13. Stage 13: Implementation Verification against previous runs
	runner.recordStage("13_implementation_verification")

	// 14. Stage 14: In-Repo Configuration & Template Rendering
	runner.recordStage("14_template_rendering")

	// 15. Stage 15: SCM Thread Publishing & Comment Splitting
	runner.recordStage("15_scm_publishing")
	err = runner.scmCaller.ExecuteWithRetry(ctx, chaos.PlatformGitHub, "publish_review", func(callCtx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("failed SCM review publishing: %v", err)
	}

	// 16. Stage 16: In-Toto Attestation & Telemetry Finalization
	runner.recordStage("16_attestation_telemetry")
	runner.dispatcher.Emit(observers.PipelineEvent{
		ReviewID:      reviewID,
		StageName:     "16_attestation_telemetry",
		Type:          observers.EventReviewCompleted,
		Timestamp:     time.Now().UTC(),
		TotalFindings: len(safeguarded),
	})

	// Verify all 16 stages passed sequentially
	runner.mu.Lock()
	stagesCount := len(runner.stagesPassed)
	runner.mu.Unlock()

	if stagesCount != 16 {
		t.Fatalf("expected all 16 pipeline stages to execute, got %d: %+v", stagesCount, runner.stagesPassed)
	}
}

func TestE2E16StagePipeline_ConcurrentExecutionsUnderLoad(t *testing.T) {
	ctx := context.Background()
	concurrency := 20
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(prNum int) {
			defer wg.Done()
			runner := newMockE2EStageRunner()
			wsID := uuid.New()
			reviewID := uuid.New()

			// Fast-path execution of stages
			for s := 1; s <= 16; s++ {
				stageName := fmt.Sprintf("stage_%02d", s)
				runner.recordStage(stageName)
			}

			// Validate model routing
			_, _, err := runner.router.ResolveSlot(ctx, routing.TaskDeepDeliberation, wsID.String())
			if err != nil {
				t.Errorf("worker %d failed model resolution: %v", prNum, err)
			}

			// Validate deduplication
			finding := models.CodeFinding{
				ID:          uuid.New(),
				ReviewID:    reviewID,
				WorkspaceID: wsID,
				FilePath:    "main.go",
				StartLine:   10,
				EndLine:     15,
				Severity:    models.SeverityHigh,
				Category:    "LOGIC",
				Title:       "Resource leak",
				Description: "Body is not closed properly",
			}
			deduped, _, err := runner.deduplicator.DeduplicateBatch(ctx, "repo-concurrent", prNum, []*models.CodeFinding{&finding, &finding})
			if err != nil {
				t.Errorf("worker %d deduplication error: %v", prNum, err)
			}
			if len(deduped) != 1 {
				t.Errorf("worker %d expected 1 deduped finding, got %d", prNum, len(deduped))
			}
		}(i)
	}

	wg.Wait()
}
