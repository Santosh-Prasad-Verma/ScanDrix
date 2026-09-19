package pipeline_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm/embedding"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/feedback"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/review/services"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestReviewPipelineExecution(t *testing.T) {
	catalog := rules.DefaultCatalog()
	evaluator, err := rules.NewEvaluator(catalog)
	if err != nil {
		t.Fatalf("failed to init evaluator: %v", err)
	}

	engine := pipeline.NewPipelineEngine(
		stages.NewPrerequisitesStage(),
		stages.NewExternalContextStage(),
		stages.NewFileFilterStage(),
		stages.NewASTAnalysisStage(evaluator),
		stages.NewSuggestionValidatorStage(),
		stages.NewHunkFormatterStage(),
		stages.NewPRSummaryStage(),
		stages.NewSCMPublisherStage(1*time.Millisecond),
	)

	rawDiff := `diff --git a/pkg/auth/session.go b/pkg/auth/session.go
new file mode 100644
--- /dev/null
+++ b/pkg/auth/session.go
@@ -0,0 +1,12 @@
+package auth
+
+import "fmt"
+
+func GetUser(id string) string {
+	apiKey := "AKIAIOSFODNN7EXAMPLE"
+	query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id)
+	return query + apiKey
+}
diff --git a/package-lock.json b/package-lock.json
new file mode 100644
--- /dev/null
+++ b/package-lock.json
@@ -0,0 +1,1 @@
+{"name": "test"}
`

	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/auth-service",
		Provider:      models.ProviderGitHub,
		PullNumber:    42,
		Title:         "SEC-102: Migrate auth session storage",
		HeadSHA:       "abcdef123456",
		BaseSHA:       "123456abcdef",
		Author:        "developer@acme.com",
		RawDiff:       rawDiff,
		ReviewParams: dtos.ReviewParametersDTO{
			MaxDiffLines: 1000,
			DryRunMode:   false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = engine.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("pipeline execution failed: %v", err)
	}

	// 1. Verify External Context extraction (SEC-102)
	if pCtx.ExternalContext == nil || pCtx.ExternalContext.IssueKey != "SEC-102" {
		t.Fatalf("expected issue key SEC-102, got %+v", pCtx.ExternalContext)
	}

	// 2. Verify File Filtering (package-lock.json removed)
	if len(pCtx.FilteredPatches) != 1 || pCtx.FilteredPatches[0].NewPath != "pkg/auth/session.go" {
		t.Fatalf("expected 1 filtered file (pkg/auth/session.go), got %d", len(pCtx.FilteredPatches))
	}

	// 3. Verify Static Security Findings (AWS key and SQL injection found)
	if len(pCtx.StaticFindings) == 0 {
		t.Fatalf("expected static security findings, got 0")
	}

	// 4. Verify Suggestions & Inline Comments
	if len(pCtx.InlineComments) == 0 {
		t.Fatalf("expected inline comments, got 0")
	}

	// 5. Verify PR Summary Report
	if pCtx.PassedReview {
		t.Fatalf("expected PassedReview to be false due to critical findings, got true")
	}
	if !strings.Contains(pCtx.PRSummaryBody, "Critical") {
		t.Fatalf("expected PR summary to report critical findings: %s", pCtx.PRSummaryBody)
	}
	if !strings.Contains(pCtx.PRSummaryBody, "Drixy Code Review Summary") || !strings.Contains(pCtx.PRSummaryBody, "Reviewed by **Drixy**") {
		t.Fatalf("expected PR summary to contain Drixy branding and footer: %s", pCtx.PRSummaryBody)
	}

	// 6. Verify Stage Metrics Telemetry
	if len(pCtx.StageMetrics) < 7 {
		t.Fatalf("expected at least 7 stage metrics, got %d", len(pCtx.StageMetrics))
	}
}

func TestPrerequisitesBotAndLifecycleGuards(t *testing.T) {
	prereq := stages.NewPrerequisitesStage()
	ctx := context.Background()

	// 1. Bot Author Skip
	botCtx := &pipeline.PipelineContext{
		Title:   "Bump lodash",
		Author:  "dependabot[bot]",
		RawDiff: "diff --git a/a b/b\n",
	}
	err := prereq.Execute(ctx, botCtx)
	if err == nil || !strings.Contains(err.Error(), "automated bot") {
		t.Fatalf("expected bot skip error, got: %v", err)
	}

	// 2. Closed PR Skip
	closedCtx := &pipeline.PipelineContext{
		Title:   "Add feature",
		Author:  "alice",
		PRState: "closed",
		RawDiff: "diff --git a/a b/b\n",
	}
	err = prereq.Execute(ctx, closedCtx)
	if err == nil || !strings.Contains(err.Error(), "closed or merged") {
		t.Fatalf("expected closed PR error, got: %v", err)
	}

	// 3. Locked Conversation Skip
	lockedCtx := &pipeline.PipelineContext{
		Title:    "Add feature",
		Author:   "alice",
		IsLocked: true,
		RawDiff:  "diff --git a/a b/b\n",
	}
	err = prereq.Execute(ctx, lockedCtx)
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected locked PR error, got: %v", err)
	}

	// 4. Circular Rule Repository Skip
	ruleRepoCtx := &pipeline.PipelineContext{
		Title:         "Update security rules",
		Author:        "alice",
		RepoNamespace: "acme/scandrix-rules",
		RawDiff:       "diff --git a/a b/b\n",
	}
	err = prereq.Execute(ctx, ruleRepoCtx)
	if err == nil || !strings.Contains(err.Error(), "rules definition repository") {
		t.Fatalf("expected rule repo skip error, got: %v", err)
	}
}

func TestPRSummaryStageWithDiagnostics(t *testing.T) {
	summaryStage := stages.NewPRSummaryStage()
	ctx := context.Background()

	pCtx := &pipeline.PipelineContext{
		ReviewID:    uuid.New(),
		WorkspaceID: uuid.New(),
		PullNumber:  101,
		Title:       "Refactor core payment gateway",
		Author:      "alice",
		LastReviewError: &pipeline.ReviewErrorInfo{
			FriendlyMessage: "The AI provider failed to process the diff.",
			Provider:        "anthropic",
			Model:           "claude-3-5-sonnet",
			HTTPStatus:      429,
			ProviderMessage: "Rate limit exceeded. Key sk-ant-secret123 was throttled.",
			AgentName:       "semantic_reviewer",
			OccurredAt:      time.Now().UTC(),
		},
	}

	err := summaryStage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("unexpected summary stage execution failure: %v", err)
	}

	if !strings.Contains(pCtx.PRSummaryBody, "### ⚠️ Review Diagnostics") {
		t.Fatalf("expected diagnostics section in summary body: %s", pCtx.PRSummaryBody)
	}
	if !strings.Contains(pCtx.PRSummaryBody, "anthropic · claude-3-5-sonnet · HTTP 429 · semantic_reviewer") {
		t.Fatalf("expected facts line in summary body: %s", pCtx.PRSummaryBody)
	}
	if !strings.Contains(pCtx.PRSummaryBody, "Provider said: Rate limit exceeded.") {
		t.Fatalf("expected provider quote: %s", pCtx.PRSummaryBody)
	}
	if strings.Contains(pCtx.PRSummaryBody, "sk-ant-secret123") {
		t.Fatalf("secret key was not redacted: %s", pCtx.PRSummaryBody)
	}
}

type testMemoryStore struct {
	mems map[string]*database.SecurityMemoryRecord
}

func (s *testMemoryStore) UpsertSecurityMemoryWithEmbedding(_ context.Context, _ uuid.UUID, mem *database.SecurityMemoryRecord, _ []float32) error {
	s.mems[mem.FindingFingerprint] = mem
	return nil
}
func (s *testMemoryStore) SearchSimilarSecurityFindings(_ context.Context, _ uuid.UUID, _ []float32, _ string, _ int, _ float64) ([]database.SecurityMemoryRecord, error) {
	return nil, nil
}
func (s *testMemoryStore) GetSecurityMemoryByFingerprint(_ context.Context, _ uuid.UUID, fp string) (*database.SecurityMemoryRecord, error) {
	return s.mems[fp], nil
}

func TestReviewPipeline_SemanticSuppressorStage(t *testing.T) {
	store := &testMemoryStore{mems: make(map[string]*database.SecurityMemoryRecord)}
	embedder := embedding.NewDeterministicSemanticEmbedder()
	fbService := feedback.NewSemanticFeedbackService(store, embedder)

	wsID := uuid.New()
	finding := &models.CodeFinding{
		ID:          uuid.New(),
		Fingerprint: "fp-dismissed-1",
		Title:       "Test Rule",
		Category:    "security",
	}
	_ = fbService.RecordDismissal(context.Background(), wsID, finding, "FALSE_POSITIVE", "admin@scandrix.dev")

	suppressor := stages.NewSemanticSuppressorStage(fbService)

	pCtx := &pipeline.PipelineContext{
		WorkspaceID: wsID,
		AllFindings: []models.CodeFinding{
			*finding,
			{
				ID:          uuid.New(),
				Fingerprint: "fp-active-2",
				Title:       "Active Vulnerability",
				Category:    "security",
			},
		},
	}

	if err := suppressor.Execute(context.Background(), pCtx); err != nil {
		t.Fatalf("suppressor stage failed: %v", err)
	}

	if len(pCtx.AllFindings) != 1 {
		t.Fatalf("expected 1 active finding, got %d", len(pCtx.AllFindings))
	}
	if pCtx.AllFindings[0].Fingerprint != "fp-active-2" {
		t.Errorf("expected fp-active-2 to remain active, got %s", pCtx.AllFindings[0].Fingerprint)
	}
	if len(pCtx.SuppressedFindings) != 1 {
		t.Fatalf("expected 1 suppressed finding, got %d", len(pCtx.SuppressedFindings))
	}
}

func TestMultiLanguageSuggestionValidation(t *testing.T) {
	stage := stages.NewSuggestionValidatorStage()

	pCtx := &pipeline.PipelineContext{
		ParsedPatches: []*diff.FilePatch{
			{
				NewPath: "services/auth.py",
				Hunks: []diff.Hunk{
					{NewStart: 1, NewLines: 20},
				},
			},
			{
				NewPath: "web/client.ts",
				Hunks: []diff.Hunk{
					{NewStart: 1, NewLines: 20},
				},
			},
			{
				NewPath: "config/schema.json",
				Hunks: []diff.Hunk{
					{NewStart: 1, NewLines: 20},
				},
			},
		},
		AllFindings: []models.CodeFinding{
			{
				ID:            uuid.New(),
				FilePath:      "services/auth.py",
				StartLine:     5,
				EndLine:       6,
				SuggestedDiff: "def verify_token(token):\n    return token == 'valid'\n",
			},
			{
				ID:            uuid.New(),
				FilePath:      "web/client.ts",
				StartLine:     2,
				EndLine:       3,
				SuggestedDiff: "export function getToken(): string {\n  return 'valid';\n}",
			},
			{
				ID:            uuid.New(),
				FilePath:      "config/schema.json",
				StartLine:     1,
				EndLine:       3,
				SuggestedDiff: "{\n  \"valid\": true\n}",
			},
			{
				ID:            uuid.New(),
				FilePath:      "config/schema.json",
				StartLine:     1,
				EndLine:       3,
				SuggestedDiff: "{\n  \"invalid\": true,\n", // broken json
			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if len(pCtx.Suggestions) != 4 {
		t.Fatalf("expected 4 suggestions, got %d", len(pCtx.Suggestions))
	}

	// Python valid
	if !pCtx.Suggestions[0].SyntaxValid || pCtx.Suggestions[0].ConfidenceScore < 0.9 {
		t.Errorf("expected python suggestion to be valid with high confidence, got %+v", pCtx.Suggestions[0])
	}
	// TypeScript valid
	if !pCtx.Suggestions[1].SyntaxValid || pCtx.Suggestions[1].ConfidenceScore < 0.9 {
		t.Errorf("expected TS suggestion to be valid with high confidence, got %+v", pCtx.Suggestions[1])
	}
	// JSON valid
	if !pCtx.Suggestions[2].SyntaxValid || pCtx.Suggestions[2].ConfidenceScore < 0.9 {
		t.Errorf("expected JSON suggestion to be valid with high confidence, got %+v", pCtx.Suggestions[2])
	}
	// JSON invalid
	if pCtx.Suggestions[3].SyntaxValid || pCtx.Suggestions[3].ConfidenceScore > 0.6 {
		t.Errorf("expected broken JSON suggestion to be marked invalid, got %+v", pCtx.Suggestions[3])
	}
}

func TestCanonicalCodeReviewPipelineStrategy_16Stages(t *testing.T) {
	catalog := rules.DefaultCatalog()
	evaluator, err := rules.NewEvaluator(catalog)
	if err != nil {
		t.Fatalf("failed to init evaluator: %v", err)
	}

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

	if strategy.GetPipelineName() != "CanonicalCodeReviewPipeline" {
		t.Errorf("expected CanonicalCodeReviewPipeline, got %s", strategy.GetPipelineName())
	}

	stagesList := strategy.ConfigureStages()
	if len(stagesList) != 16 {
		t.Fatalf("expected exactly 16 stages in canonical pipeline, got %d", len(stagesList))
	}

	// Verify order matches canonical 16-stage strategy
	expectedOrder := []string{
		"prerequisites_validator",
		"ValidateNewCommitsStage",
		"ResolveConfigStage",
		"ValidateConfigStage",
		"file_filter",
		"external_context_loader",
		"InitialCommentStage",
		"BusinessLogicValidationStage",
		"create_sandbox",
		"ast_static_analysis",
		"pr_summary_generator",
		"suggestion_validator",
		"scm_publisher",
		"AggregateResultsStage",
		"FinishCommentsStage",
		"FinishProcessReviewStage",
	}

	for i, expectedName := range expectedOrder {
		if stagesList[i].Name() != expectedName {
			t.Errorf("stage %d name = %s; want %s", i+1, stagesList[i].Name(), expectedName)
		}
	}

	// Execute through strategy
	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "scandrix/platform",
		Provider:      models.ProviderGitHub,
		PullNumber:    101,
		Title:         "feat: add secure credential store",
		HeadSHA:       "abcdef789",
		BaseSHA:       "123456789",
		Author:        "tarun@scandrix.dev",
		RawDiff: `diff --git a/pkg/crypto/store.go b/pkg/crypto/store.go
new file mode 100644
--- /dev/null
+++ b/pkg/crypto/store.go
@@ -0,0 +1,5 @@
+package crypto
+
+func HashKey(key string) string {
+	return key
+}
`,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = strategy.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("strategy execution failed: %v", err)
	}

	if len(pCtx.StageMetrics) < 16 {
		t.Errorf("expected at least 16 stage telemetry metrics recorded, got %d", len(pCtx.StageMetrics))
	}
}


