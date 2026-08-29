package pipeline_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
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
