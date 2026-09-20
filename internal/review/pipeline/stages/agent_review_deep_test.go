// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

func TestDeepAgentReview_FrozenSnapshotDeterminism(t *testing.T) {
	stage := NewDeepAgentReviewStage()

	pCtx := &pipeline.PipelineContext{
		PullNumber:  123,
		Title:       "Feature: Add Auth Gateway",
		Description: "Implements JWT verification and token exchange",
		ExternalContext: &pipeline.ExternalIssueContext{
			IssueKey:    "SD-404",
			Title:       "Authentication Enhancement",
			Description: "Require SHA256 and Argon2",
		},
		TraceDecisions: []pipeline.TraceDecisionInfo{
			{DecisionKey: "ADR-001", Summary: "Use Clean Architecture"},
		},
		ChangedFiles: []pipeline.FileChangeInfo{
			{Filename: "pkg/auth/jwt.go", Additions: 50, Deletions: 5},
			{Filename: "pkg/auth/hash.go", Additions: 20, Deletions: 2},
		},
	}

	snap1 := stage.BuildFrozenSnapshot(pCtx)
	snap2 := stage.BuildFrozenSnapshot(pCtx)

	if snap1.DigestToken == "" || snap2.DigestToken == "" {
		t.Fatalf("expected non-empty digest tokens")
	}
	if snap1.DigestToken != snap2.DigestToken {
		t.Errorf("digest tokens must be deterministic: %s vs %s", snap1.DigestToken, snap2.DigestToken)
	}
}

func TestDeepAgentReview_MultiPersonaExecution(t *testing.T) {
	stage := NewDeepAgentReviewStage(
		WithAgentConcurrency(4),
	)

	pCtx := &pipeline.PipelineContext{
		ReviewID:   uuid.New(),
		PullNumber: 777,
		ChangedFiles: []pipeline.FileChangeInfo{
			{
				Filename: "internal/crypto/legacy.go",
				Patch: `@@ -1,3 +1,5 @@
+h := md5.New()
+h.Write([]byte("password"))
`,
				Additions: 2,
			},
			{
				Filename: "internal/service/worker.go",
				Patch: `@@ -1,4 +1,7 @@
+go func() {
+    defer cleanup()
+    doWork()
+}()
`,
				Additions: 4,
			},
			{
				Filename: "internal/domain/user.go",
				Patch: `@@ -1,3 +1,5 @@
+resp, err := http.Get("https://api.example.com")
+_ = resp
`,
				Additions: 2,
			},
			{
				Filename: "package.json",
				Patch: `@@ -1,3 +1,5 @@
+"license": "AGPL-3.0",
`,
				Additions: 1,
			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("unexpected error executing stage: %v", err)
	}

	if len(pCtx.AgentFindings) < 4 {
		t.Fatalf("expected at least 4 findings across personas, got %d", len(pCtx.AgentFindings))
	}

	// Verify traces recorded in pipeline metadata
	tracesRaw, exists := pCtx.PipelineMetadata["deep_agent_traces"]
	if !exists {
		t.Fatalf("expected deep_agent_traces in pipeline metadata")
	}
	traces, ok := tracesRaw.([]AgentExecutionTrace)
	if !ok || len(traces) != 6 {
		t.Errorf("expected 6 persona execution traces, got %v", tracesRaw)
	}

	// Verify categories present: Security, Concurrency, Architecture, Compliance
	categoryMap := make(map[string]bool)
	for _, f := range pCtx.AgentFindings {
		categoryMap[f.Category] = true
	}

	if !categoryMap["SECURITY"] {
		t.Errorf("expected SECURITY finding from SecurityAuditor")
	}
	if !categoryMap["CONCURRENCY"] {
		t.Errorf("expected CONCURRENCY finding from ConcurrencySpecialist")
	}
	if !categoryMap["ARCHITECTURE"] {
		t.Errorf("expected ARCHITECTURE finding from ArchitectureValidator")
	}
	if !categoryMap["COMPLIANCE"] {
		t.Errorf("expected COMPLIANCE finding from ComplianceChecker")
	}
}

func TestDeepAgentReview_SemanticDeduplicationAndTiebreaking(t *testing.T) {
	stage := NewDeepAgentReviewStage(
		WithDedupThreshold(0.50),
	)

	f1 := models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/service.go",
		StartLine:   10,
		EndLine:     10,
		Severity:    models.SeverityLow,
		Category:    "STYLE",
		Title:       "Unbounded loop iteration detected",
		Description: "The loop iterates without explicit boundary checks",
	}

	// f2 is similar to f1 (targeting same file line 11) with higher severity
	f2 := models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/service.go",
		StartLine:   11,
		EndLine:     11,
		Severity:    models.SeverityHigh,
		Category:    "PERFORMANCE",
		Title:       "Unbounded loop iteration detected in processing",
		Description: "The loop iterates without boundary checks leading to memory exhaustion",
	}

	// f3 is distinct (different file)
	f3 := models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    "pkg/other.go",
		StartLine:   50,
		EndLine:     50,
		Severity:    models.SeverityCritical,
		Category:    "SECURITY",
		Title:       "Critical secret leaked",
		Description: "Hardcoded API key exposed in client initialization",
	}

	deduped := stage.DeduplicateFindings([]models.CodeFinding{f1, f2, f3})

	// f1 and f2 should be merged into 1 finding with the higher severity (HIGH)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated findings, got %d", len(deduped))
	}

	foundPkgService := false
	for _, f := range deduped {
		if f.FilePath == "pkg/service.go" {
			foundPkgService = true
			if f.Severity != models.SeverityHigh {
				t.Errorf("expected merged finding to retain HIGH severity, got %v", f.Severity)
			}
		}
	}
	if !foundPkgService {
		t.Errorf("expected pkg/service.go finding to be retained")
	}
}

func TestDeepAgentReview_ConcurrentStress(t *testing.T) {
	stage := NewDeepAgentReviewStage(
		WithAgentConcurrency(6),
	)

	pCtx := &pipeline.PipelineContext{
		ReviewID:   uuid.New(),
		PullNumber: 999,
		ChangedFiles: []pipeline.FileChangeInfo{
			{
				Filename: "internal/service/server.go",
				Patch: `@@ -1,5 +1,15 @@
 func StartServer() {
+    exec.Command("ls")
+    go func() {
+        defer cleanup()
+        run()
+    }()
+    // Broad exception
+    // catch (Exception e)
 }`,
				Additions: 10,
			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("concurrent execution failed: %v", err)
	}

	if len(pCtx.ValidSuggestions) == 0 {
		t.Errorf("expected valid suggestions to be generated")
	}
}
