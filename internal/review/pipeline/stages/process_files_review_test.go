// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

func TestProcessFilesReview_FilterFiles(t *testing.T) {
	stage := NewProcessFilesReviewStage()

	files := []pipeline.FileChangeInfo{
		{Filename: "src/index.ts", Status: "modified"},
		{Filename: "assets/logo.png", Status: "added"},
		{Filename: "package-lock.json", Status: "modified"},
		{Filename: "vendor/bundle.min.js", Status: "modified"},
		{Filename: "old_module.go", Status: "removed"},
		{Filename: "internal/auth/handler.go", Status: "added"},
	}
	ignored := []string{"package-lock.json"}

	filtered := stage.filterAndPrepareFiles(files, ignored)

	// Should keep only src/index.ts and internal/auth/handler.go
	if len(filtered) != 2 {
		t.Fatalf("expected 2 valid files, got %d", len(filtered))
	}
	if filtered[0].Filename != "src/index.ts" || filtered[1].Filename != "internal/auth/handler.go" {
		t.Errorf("unexpected filtered files: %+v", filtered)
	}
}

func TestProcessFilesReview_BatchingOptimization(t *testing.T) {
	stage := NewProcessFilesReviewStage(
		WithBatchBounds(2, 4),
	)

	var files []pipeline.FileChangeInfo
	for i := 0; i < 9; i++ {
		files = append(files, pipeline.FileChangeInfo{
			Filename:  fmt.Sprintf("pkg/file_%d.go", i),
			Additions: 10 + i*5,
			Deletions: 2,
		})
	}

	batches := stage.createOptimizedBatches(files)

	// 9 files with maxBatchSize=4 should yield 3 batches (4, 4, 1)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0].Files) != 4 || len(batches[1].Files) != 4 || len(batches[2].Files) != 1 {
		t.Errorf("unexpected batch distribution: %d, %d, %d",
			len(batches[0].Files), len(batches[1].Files), len(batches[2].Files))
	}
}

func TestProcessFilesReview_DiffBoundaryAndRuleEvaluation(t *testing.T) {
	stage := NewProcessFilesReviewStage(
		WithFindingsCaps(2, 5),
	)

	pCtx := &pipeline.PipelineContext{
		ReviewID:   uuid.New(),
		PullNumber: 42,
		ActiveRules: []rules.RuleSpec{
			{
				ID:          uuid.New(),
				Name:        "No Raw SQL Query",
				PathPattern: "**/*.go",
				RegexRule:   `(?i)db\.Query\(.*fmt\.Sprintf`,
				Severity:    models.SeverityCritical,
				Category:    "SECURITY",
				Description: "SQL injection danger via raw format string",
			},
			{
				ID:          uuid.New(),
				Name:        "No Println In Production",
				PathPattern: "**/*.go",
				RegexRule:   `fmt\.Println\(`,
				Severity:    models.SeverityLow,
				Category:    "STYLE",
				Description: "Use structured logger instead of println",
			},
		},
		ChangedFiles: []pipeline.FileChangeInfo{
			{
				Filename: "internal/repo/user.go",
				Patch: `@@ -10,6 +10,11 @@
 func GetUser(id string) error {
-    return nil
+    fmt.Println("debug user")
+    rows, err := db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id))
+    _ = rows
+    return err
 }`,
 				Additions: 5,
 				Deletions: 1,
 			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("unexpected error executing stage: %v", err)
	}

	if len(pCtx.AllFindings) == 0 {
		t.Fatalf("expected findings to be emitted, got 0")
	}

	// Verify findings are ranked by severity: Critical first
	if pCtx.AllFindings[0].Severity != models.SeverityCritical {
		t.Errorf("expected highest severity finding first, got %v", pCtx.AllFindings[0].Severity)
	}

	// Verify valid suggestions were created
	if len(pCtx.ValidSuggestions) == 0 {
		t.Fatalf("expected valid suggestions, got 0")
	}
	if pCtx.ValidSuggestions[0].Severity != domain.SeverityCritical {
		t.Errorf("expected suggestion severity to match finding, got %v", pCtx.ValidSuggestions[0].Severity)
	}

	// Verify line number falls within the hunk: 11-16
	for _, f := range pCtx.AllFindings {
		if f.StartLine < 10 || f.StartLine > 20 {
			t.Errorf("finding line %d out of expected hunk bounds", f.StartLine)
		}
	}
}

func TestProcessFilesReview_CapsEnforcement(t *testing.T) {
	stage := NewProcessFilesReviewStage(
		WithFindingsCaps(1, 2), // Max 1 per file, max 2 for PR
	)

	pCtx := &pipeline.PipelineContext{
		ReviewID:   uuid.New(),
		PullNumber: 88,
		ActiveRules: []rules.RuleSpec{
			{
				Name:        "Debug Println",
				PathPattern: "**/*.go",
				RegexRule:   `fmt\.Println`,
				Severity:    models.SeverityLow,
			},
		},
		ChangedFiles: []pipeline.FileChangeInfo{
			{
				Filename: "pkg/a.go",
				Patch: `@@ -1,3 +1,5 @@
+fmt.Println("1")
+fmt.Println("2")
`,
				Additions: 2,
			},
			{
				Filename: "pkg/b.go",
				Patch: `@@ -1,3 +1,5 @@
+fmt.Println("3")
+fmt.Println("4")
`,
				Additions: 2,
			},
			{
				Filename: "pkg/c.go",
				Patch: `@@ -1,3 +1,5 @@
+fmt.Println("5")
`,
				Additions: 1,
			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Max 2 findings across whole PR
	if len(pCtx.AllFindings) > 2 {
		t.Errorf("expected at most 2 findings for PR, got %d", len(pCtx.AllFindings))
	}
	if len(pCtx.ValidSuggestions) > 2 {
		t.Errorf("expected at most 2 suggestions for PR, got %d", len(pCtx.ValidSuggestions))
	}
}

func TestProcessFilesReview_ConcurrentStress(t *testing.T) {
	stage := NewProcessFilesReviewStage(
		WithBatchBounds(5, 10),
	)

	var files []pipeline.FileChangeInfo
	for i := 0; i < 25; i++ {
		files = append(files, pipeline.FileChangeInfo{
			Filename: fmt.Sprintf("service/module_%d/handler.go", i),
			Patch: fmt.Sprintf(`@@ -1,2 +1,6 @@
 func Handle%d() {
+    // mutation %d
+    fmt.Println("running")
 }`, i, i),
 			Additions: 3,
 		})
	}

	pCtx := &pipeline.PipelineContext{
		ReviewID:     uuid.New(),
		PullNumber:   100,
		ChangedFiles: files,
		ActiveRules: []rules.RuleSpec{
			{
				Name:        "Log rule",
				PathPattern: "**/*.go",
				RegexRule:   `fmt\.Println`,
				Severity:    models.SeverityMedium,
			},
		},
	}

	err := stage.Execute(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("concurrent execution failed: %v", err)
	}

	if len(pCtx.AllFindings) == 0 {
		t.Errorf("expected findings from concurrent execution")
	}
}
