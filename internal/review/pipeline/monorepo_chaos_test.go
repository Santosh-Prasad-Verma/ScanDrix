package pipeline_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/review/checker"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/review/rulesengine"
	"github.com/scandrix/backend/internal/review/services"
	"github.com/scandrix/backend/internal/review/trace"
	"github.com/scandrix/backend/internal/review/verifier"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MonorepoFileModel represents a simulated source file in a multi-language enterprise monorepo.
type MonorepoFileModel struct {
	Path     string
	Language checker.LanguageKind
	Content  string
	DiffHunk string
}

// GenerateSyntheticMonorepo constructs 100+ files across Go, TS/React, Python, JSON, YAML, and SQL.
func GenerateSyntheticMonorepo() []MonorepoFileModel {
	var files []MonorepoFileModel

	// 1. 25 Go microservices files (api, domain, auth, database, worker)
	for i := 1; i <= 25; i++ {
		path := fmt.Sprintf("services/core/pkg/service%d/handler.go", i)
		content := fmt.Sprintf(`package service%d

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

type Handler%d struct {
	mu sync.Mutex
	items map[string]string
}

func (h *Handler%d) Process(ctx context.Context, id string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if val, ok := h.items[id]; ok {
		return val, nil
	}
	return fmt.Sprintf("service_%d_%%s", id), nil
}
`, i, i, i, i)
		diffHunk := fmt.Sprintf(`@@ -15,4 +15,6 @@
 func (h *Handler%d) Process(ctx context.Context, id string) (string, error) {
+	h.mu.Lock()
+	defer h.mu.Unlock()
`, i)
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangGo,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	// 2. 25 TypeScript & React components (apps/web/src/components/*)
	for i := 1; i <= 25; i++ {
		path := fmt.Sprintf("apps/web/src/components/Widget%d.tsx", i)
		content := fmt.Sprintf(`import React, { useState } from 'react';

export interface WidgetProps%d {
	title: string;
	initialCount?: number;
}

export const Widget%d: React.FC<WidgetProps%d> = ({ title, initialCount = 0 }) => {
	const [count, setCount] = useState<number>(initialCount);
	return (
		<div className="widget-box">
			<h3>{title}</h3>
			<p>Count: {count}</p>
			<button onClick={() => setCount(count + 1)}>Increment</button>
		</div>
	);
};
`, i, i, i)
		diffHunk := fmt.Sprintf(`@@ -8,3 +8,4 @@
 export const Widget%d: React.FC<WidgetProps%d> = ({ title, initialCount = 0 }) => {
+	const [count, setCount] = useState<number>(initialCount);
`, i, i)
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangTSX,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	// 3. 25 Python analytics & ML pipelines (workers/ml/pipeline_*.py)
	for i := 1; i <= 25; i++ {
		path := fmt.Sprintf("workers/ml/pipeline_%d.py", i)
		content := fmt.Sprintf(`import os
import json

class Pipeline%d:
    def __init__(self, model_version: str):
        self.version = model_version

    def transform(self, records: list) -> list:
        results = []
        for r in records:
            if r.get("active", False):
                results.append({"id": r.get("id"), "processed": True})
        return results
`, i)
		diffHunk := `@@ -8,3 +8,4 @@
     def transform(self, records: list) -> list:
+        results = []
`
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangPython,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	// 4. 10 Database Migrations (migrations/*.sql)
	for i := 1; i <= 10; i++ {
		path := fmt.Sprintf("migrations/00%d_tenant_partition.sql", i)
		content := fmt.Sprintf(`-- Migration 00%d
CREATE TABLE IF NOT EXISTS tenant_table_%d (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	workspace_id UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tenant_%d_ws ON tenant_table_%d(workspace_id);
`, i, i, i, i)
		diffHunk := fmt.Sprintf(`@@ -1,4 +1,7 @@
+CREATE TABLE IF NOT EXISTS tenant_table_%d (
+	id UUID PRIMARY KEY DEFAULT gen_random_uuid()
+);
`, i)
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangSQL,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	// 5. 10 Configuration and CI workflows (JSON and YAML)
	for i := 1; i <= 10; i++ {
		path := fmt.Sprintf(".scandrix/config_%d.yml", i)
		content := fmt.Sprintf(`version: "1.0"
service_%d:
  enabled: true
  strictness: strict
  max_suggestions: 15
`, i)
		diffHunk := `@@ -1,3 +1,4 @@
+  strictness: strict
`
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangYAML,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	// 6. 5 Architecture Decision Records (docs/adr/ADR-*.md)
	for i := 1; i <= 5; i++ {
		path := fmt.Sprintf("docs/adr/ADR-00%d-architecture-guideline.md", i)
		content := fmt.Sprintf(`# ADR-00%d: Architecture Standard %d

## Context
All database calls must use parameterized queries. Dynamic SQL concatenation is strictly forbidden.

## Decision
We enforce parameterized queries across all services.

## Scope: services/**
`, i, i)
		diffHunk := `@@ -1,3 +1,4 @@
+We enforce parameterized queries across all services.
`
		files = append(files, MonorepoFileModel{
			Path:     path,
			Language: checker.LangUnknown,
			Content:  content,
			DiffHunk: diffHunk,
		})
	}

	return files
}

func TestMonorepoSimulation_EndToEndChaos(t *testing.T) {
	monorepo := GenerateSyntheticMonorepo()
	require.GreaterOrEqual(t, len(monorepo), 100, "monorepo must contain at least 100 files")

	syntaxValidator := checker.NewASTSyntaxValidator()
	diffGate := verifier.NewDiffGate(syntaxValidator)
	clusterEngine := services.NewSuggestionClusterEngine()
	truncationFitter := services.NewSCMCommentTruncationFitter(models.ProviderGitHub)
	adrStore := trace.NewTraceStore()
	ctx := context.Background()
	_ = adrStore.SetTraceEnabled(ctx, "org-test", "repo-test", true)
	_ = adrStore.SaveDecision(ctx, &trace.TraceDecision{
		ID:          uuid.New(),
		OrgID:       "org-test",
		RepoID:      "repo-test",
		DecisionKey: "ADR-001",
		Title:       "Parameterized Queries Mandatory",
		Status:      trace.StatusAccepted,
		Scope:       []string{"services/**"},
		Decision:    "All database queries must be parameterized. Raw SQL string concatenation is forbidden.",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	invalidator := trace.NewTraceInvalidator(adrStore)

	t.Run("High Concurrency Validation Across All 100 Monorepo Files", func(t *testing.T) {
		var wg sync.WaitGroup
		errChan := make(chan error, len(monorepo))

		for _, f := range monorepo {
			wg.Add(1)
			go func(file MonorepoFileModel) {
				defer wg.Done()
				rep := syntaxValidator.ValidateFile(ctx, file.Path, file.Content)
				if !rep.IsValid {
					errChan <- fmt.Errorf("file %s failed AST validation: %v", file.Path, rep.Diagnostics)
				}
			}(f)
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Errorf("monorepo AST error: %v", err)
		}
	})

	t.Run("Injected Flaws Triage, Clustering, and Gate Rejections", func(t *testing.T) {
		findings := []models.CodeFinding{
			// Flaw 1: Valid Go Committable Suggestion
			{
				ID:            uuid.New(),
				FilePath:      "services/core/pkg/service1/handler.go",
				StartLine:     16,
				EndLine:       17,
				Severity:      models.SeverityHigh,
				Category:      "concurrency",
				Title:         "Missing Mutex Unlock",
				SuggestedDiff: "h.mu.Lock()\ndefer h.mu.Unlock()\n",
			},
			// Flaw 2: Duplicate across service2 (Repeated finding for clustering)
			{
				ID:            uuid.New(),
				FilePath:      "services/core/pkg/service2/handler.go",
				StartLine:     16,
				EndLine:       17,
				Severity:      models.SeverityHigh,
				Category:      "concurrency",
				Title:         "Missing Mutex Unlock",
				SuggestedDiff: "h.mu.Lock()\ndefer h.mu.Unlock()\n",
			},
			// Flaw 3: Duplicate across service3 (Repeated finding for clustering)
			{
				ID:            uuid.New(),
				FilePath:      "services/core/pkg/service3/handler.go",
				StartLine:     16,
				EndLine:       17,
				Severity:      models.SeverityCritical, // Higher severity in service3
				Category:      "concurrency",
				Title:         "Missing Mutex Unlock",
				SuggestedDiff: "h.mu.Lock()\ndefer h.mu.Unlock()\n",
			},
			// Flaw 4: Broken Syntax (Unclosed Brace) -> Downgraded to Advisory
			{
				ID:            uuid.New(),
				FilePath:      "apps/web/src/components/Widget1.tsx",
				StartLine:     10,
				EndLine:       10,
				Severity:      models.SeverityMedium,
				Category:      "syntax",
				Title:         "Broken Hook Syntax",
				SuggestedDiff: "const [broken, setBroken] = useState(", // unclosed paren
			},
			// Flaw 5: No-op diff -> Rejected by DiffGate
			{
				ID:            uuid.New(),
				FilePath:      "workers/ml/pipeline_1.py",
				StartLine:     9,
				EndLine:       9,
				Severity:      models.SeverityLow,
				Category:      "style",
				Title:         "Redundant Format",
				SuggestedDiff: "        results = []", // Exactly identical to existing file line 9
			},
		}

		fileContexts := map[string]string{
			"services/core/pkg/service1/handler.go": monorepo[0].Content,
			"services/core/pkg/service2/handler.go": monorepo[1].Content,
			"services/core/pkg/service3/handler.go": monorepo[2].Content,
			"apps/web/src/components/Widget1.tsx":   monorepo[25].Content,
			"workers/ml/pipeline_1.py":              monorepo[50].Content,
		}

		// Run through DiffGate
		filtered, gateResults := diffGate.ValidateFindingsBatch(ctx, findings, fileContexts)
		assert.Len(t, filtered, 4, "no-op finding must be rejected and excluded from kept findings")

		// Verify Committable vs Advisory
		assert.Equal(t, verifier.DecisionCommittable, gateResults[findings[0].ID.String()].Decision)
		assert.True(t, gateResults[findings[0].ID.String()].IsCommittable)

		assert.Equal(t, verifier.DecisionAdvisory, gateResults[findings[3].ID.String()].Decision)
		assert.False(t, gateResults[findings[3].ID.String()].IsCommittable)

		assert.Equal(t, verifier.DecisionRejected, gateResults[findings[4].ID.String()].Decision)

		// Run through Repeated Suggestion Clustering
		masters, standalone := clusterEngine.ClusterFindings(ctx, filtered)
		require.Len(t, masters, 1, "repeated concurrency findings across 3 services must be clustered into 1 master")
		assert.Len(t, standalone, 1, "unrelated TSX finding must remain standalone")

		master := masters[0]
		assert.Equal(t, models.SeverityCritical, master.Severity, "master must inherit critical severity from service3")
		assert.Len(t, master.Locations, 3)
		assert.Contains(t, master.FormattedMasterBody, "### 📍 Affected Locations (3 occurrences)")

		// Check SCM Comment Truncation Fitter
		fitted := truncationFitter.FitComment(master.FormattedMasterBody)
		assert.NotEmpty(t, fitted)
		assert.LessOrEqual(t, len(fitted), 65536)
	})

	t.Run("ADR Architectural Violation Audit in Monorepo Diff", func(t *testing.T) {
		diffText := `diff --git a/services/core/pkg/service1/db.go b/services/core/pkg/service1/db.go
new file mode 100644
--- /dev/null
+++ b/services/core/pkg/service1/db.go
@@ -0,0 +1,5 @@
+package service1
+func RunQuery(id string) {
+    db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id))
+}
`
		patches, err := diff.ParseUnifiedDiff(strings.NewReader(diffText))
		require.NoError(t, err)

		report, err := invalidator.AuditPullRequest(ctx, uuid.New(), uuid.New(), "org-test", "repo-test", patches)
		require.NoError(t, err)
		require.NotEmpty(t, report.Violations, "raw SQL concat must trigger ADR-001 violation finding")
		assert.Equal(t, "ADR-001", report.Violations[0].DecisionKey)
		assert.Equal(t, models.SeverityHigh, report.Violations[0].Severity)
	})

	t.Run("Rule Atoms Mechanical Evaluation Against Monorepo Patches", func(t *testing.T) {
		evaluator := rulesengine.NewAtomEvaluator()

		atoms := &rulesengine.DrixyRuleAtoms{
			Items: []*rulesengine.DrixyRuleAtom{
				{
					ID:             "atom-1",
					Title:          "Token must not be hardcoded",
					NormativeLevel: rulesengine.NormativeMustNot,
					Spec:           "Do not hardcode Bearer tokens in source",
					Severity:       models.SeverityCritical,
					Category:       "security",
					Detector: &rulesengine.CompiledRuleDetector{
						Type:    rulesengine.DetectorRegex,
						Pattern: `(?i)bearer\s+ey[a-zA-Z0-9_\-\.]{20,}`,
						Reason:  "Hardcoded JWT token in request authorization header",
					},
					Remediation: "os.Getenv(\"AUTH_TOKEN\")",
				},
			},
		}

		diffText := `diff --git a/services/core/pkg/service1/client.go b/services/core/pkg/service1/client.go
--- a/services/core/pkg/service1/client.go
+++ b/services/core/pkg/service1/client.go
@@ -10,1 +10,1 @@
-token := get()
+req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.fake")
`
		patches, err := diff.ParseUnifiedDiff(strings.NewReader(diffText))
		require.NoError(t, err)
		report := evaluator.EvaluateAtoms(ctx, uuid.New(), uuid.New(), atoms, patches)
		require.Len(t, report.Findings, 1)
		assert.Equal(t, models.SeverityCritical, report.Findings[0].Severity)
		assert.Contains(t, report.Findings[0].Remediation, "AUTH_TOKEN")
		assert.Equal(t, 1, report.MechanicalViolations)
		assert.Equal(t, 0.0, report.ComplianceScore)
	})
}

func TestMonorepoSimulation_Full16StagesExecution(t *testing.T) {
	// Execute the full 16-stage pipeline on a simulated PR modifying multiple monorepo services
	pCtx := &pipeline.PipelineContext{
		ReviewID:      uuid.New(),
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		RepoNamespace: "scandrix/enterprise-monorepo",
		Provider:      models.ProviderGitHub,
		PullNumber:    501,
		Title:         "feat: update microservices handlers & React components",
		HeadSHA:       "f00dbabe1234567890abcdef",
		BaseSHA:       "1234567890abcdeff00dbabe",
		Author:        "lead-architect@scandrix.dev",
		RawDiff: `diff --git a/services/core/pkg/service1/handler.go b/services/core/pkg/service1/handler.go
--- a/services/core/pkg/service1/handler.go
+++ b/services/core/pkg/service1/handler.go
@@ -15,2 +15,3 @@
 func (h *Handler1) Process(ctx context.Context, id string) (string, error) {
+	h.mu.Lock()
+	defer h.mu.Unlock()
diff --git a/apps/web/src/components/Widget1.tsx b/apps/web/src/components/Widget1.tsx
--- a/apps/web/src/components/Widget1.tsx
+++ b/apps/web/src/components/Widget1.tsx
@@ -8,2 +8,3 @@
 export const Widget1: React.FC<WidgetProps1> = ({ title, initialCount = 0 }) => {
+	const [count, setCount] = useState<number>(initialCount);
`,
		ReviewParams: dtos.ReviewParametersDTO{
			MaxDiffLines: 5000,
		},
		ActiveRules: []rules.RuleSpec{
			{
				ID:          uuid.New(),
				Name:        "Secret Leakage Check",
				Severity:    models.SeverityCritical,
				RegexRule:   `(?i)bearer\s+ey[a-zA-Z0-9_\-\.]{20,}`,
				Remediation: "Use secret manager or environment variable",
			},
		},
		ResolvedConfig: domain.DefaultCodeReviewConfig(),
		StartTime:      time.Now().UTC(),
	}

	logger := slog.Default()

	// 16 Stage Pipeline Strategy
	stage1 := stages.NewDeepValidatePrerequisitesStage(
		&mockPermValidator{},
		&mockExclusionChecker{},
		&mockRateLimiter{},
		&mockFeedbackReaction{},
	)
	stage2 := stages.NewDeepValidateNewCommitsStage(&mockCommitFetcher{})
	baseCfg := domain.DefaultCodeReviewConfig()
	stage3 := stages.NewDeepResolveConfigStage(&mockConfigResolver{cfg: baseCfg}, &mockSlotResolver{})
	cadenceEngine := stages.NewReviewCadenceEngine(&mockCadenceStore{})
	stage4 := stages.NewDeepValidateConfigStage(cadenceEngine, &mockFeedbackReaction{})
	stage5 := stages.NewDeepFetchChangedFilesStage(350)
	stage6 := stages.NewDeepLoadExternalContextStage(&mockTraceLoader{}, &mockIssueTrackerResolver{})
	initialCommentMgr := &mockCommentManagerService{}
	stage7 := stages.NewDeepInitialCommentStage(initialCommentMgr, &mockTemplateProcessor{})
	stage8 := stages.NewDeepBusinessLogicValidationStage(logger, &mockBusinessRulesAgent{}, &mockMCPManagerService{})
	stage9 := stages.NewCreateSandboxStage(nil)
	stage10 := stages.NewFileFilterStage()
	stage11 := stages.NewDeepCreatePrLevelCommentsStage(logger, &mockPRLevelCommentsMgr{}, &mockPRLevelStorageService{})
	stage12 := stages.NewDeepValidateSuggestionsStage(logger, nil, nil)
	fileCm := &mockFileCommentsMgr{}
	stage13 := stages.NewDeepCreateFileCommentsStage(logger, fileCm, nil, nil)
	stage14 := stages.NewDeepAggregateResultsStage(logger)
	summaryMgr := &mockReviewSummaryMgr{}
	stage15 := stages.NewDeepFinishCommentsStage(logger, summaryMgr, &mockReviewTraceCommenter{})
	reviewStateMgr := &mockReviewStateMgr{}
	notifier := &mockAutoApproveNotifier{}
	stage16 := stages.NewDeepRequestChangesOrApproveStage(logger, reviewStateMgr, notifier)

	strategy := pipeline.NewCanonicalPipelineStrategy(
		stage1, stage2, stage3, stage4, stage5,
		stage6, stage7, stage8, stage9, stage10,
		stage11, stage12, stage13, stage14, stage15, stage16,
	)

	ctx := context.Background()
	err := strategy.Execute(ctx, pCtx)
	require.NoError(t, err)

	// Verify PR processing outcomes
	assert.NotEmpty(t, pCtx.ParsedPatches, "diff patches must be parsed")
	assert.NotEmpty(t, pCtx.StageMetrics, "all executed stage metrics must be recorded")
	assert.True(t, pCtx.PassedReview, "clean PR with valid mutex lock must pass review")
}
