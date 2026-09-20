package stages

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
)

func TestExtractValidDiffLines_RightSideRanges(t *testing.T) {
	patch := `@@ -10,6 +10,8 @@
 func main() {
-    oldCall()
+    newCall1()
+    newCall2()
     commonCall()
 }
@@ -30,4 +32,5 @@
 func helper() {
+    extraLine()
 }
\ No newline at end of file`

	ranges := ExtractValidDiffLines(patch)
	if len(ranges) != 2 {
		t.Fatalf("expected 2 hunk ranges, got %d", len(ranges))
	}

	// Hunk 1: starts at right-side line 10, context (10), additions (11, 12), context (13, 14) -> [10, 14]
	if ranges[0][0] != 10 || ranges[0][1] < 13 {
		t.Errorf("expected hunk 1 starting at 10, got %v", ranges[0])
	}

	// Hunk 2: starts at right-side line 32
	if ranges[1][0] != 32 {
		t.Errorf("expected hunk 2 starting at 32, got %v", ranges[1])
	}
}

func TestConvertToUnifiedDiffWithLineNumbers(t *testing.T) {
	patch := `@@ -1,3 +1,4 @@
 commonLine()
+addedLine()
 tailLine()`

	formatted := ConvertToUnifiedDiffWithLineNumbers(patch, "app.go")
	if !strings.Contains(formatted, "+    2 | addedLine()") {
		t.Errorf("expected formatted added line with line number 2, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "--- a/app.go") {
		t.Errorf("expected header in formatted output")
	}
}

func TestDeepFetchChangedFilesStage_AllBranches(t *testing.T) {
	ctx := context.Background()
	stage := NewDeepFetchChangedFilesStage(10)

	// 1. Zero changed files
	pCtx := &pipeline.PipelineContext{}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "NO_FILES_IN_PR" {
		t.Fatalf("expected 0 files to skip review")
	}

	// 2. All files ignored
	pCtx = &pipeline.PipelineContext{
		ResolvedConfig: domain.CodeReviewConfig{
			IgnorePaths: []string{"*.min.js", "vendor/**"},
		},
		ParsedPatches: []*diff.FilePatch{
			{NewPath: "bundle.min.js"},
			{NewPath: "vendor/lib.go"},
		},
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "ALL_FILES_IGNORED" {
		t.Fatalf("expected all ignored files to skip review")
	}
	if len(pCtx.IgnoredFiles) != 2 {
		t.Errorf("expected 2 ignored files tracked, got %d", len(pCtx.IgnoredFiles))
	}

	// 3. Too many files
	smallStage := NewDeepFetchChangedFilesStage(1)
	pCtx = &pipeline.PipelineContext{
		ParsedPatches: []*diff.FilePatch{
			{NewPath: "src/file1.go"},
			{NewPath: "src/file2.go"},
		},
	}
	if err := smallStage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "TOO_MANY_FILES" {
		t.Fatalf("expected files count exceeding limit to skip review")
	}

	// 4. Valid files processed
	pCtx = &pipeline.PipelineContext{
		ResolvedConfig: domain.CodeReviewConfig{
			IgnorePaths: []string{"*.lock"},
		},
		ParsedPatches: []*diff.FilePatch{
			{
				NewPath:   "src/auth.go",
				Additions: 15,
				Deletions: 3,
				Hunks: []diff.Hunk{
					{
						Header: "@@ -1,5 +1,6 @@",
						Lines: []diff.DiffLine{
							{Type: diff.LineAddition, Content: "import auth"},
						},
					},
				},
			},
			{NewPath: "yarn.lock"}, // Ignored
		},
	}

	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected valid file changes to be processed without skipping")
	}
	if len(pCtx.ChangedFiles) != 1 || pCtx.ChangedFiles[0].Filename != "src/auth.go" {
		t.Errorf("expected src/auth.go in ChangedFiles, got %v", pCtx.ChangedFiles)
	}
	if len(pCtx.IgnoredFiles) != 1 || pCtx.IgnoredFiles[0] != "yarn.lock" {
		t.Errorf("expected yarn.lock in IgnoredFiles, got %v", pCtx.IgnoredFiles)
	}
	if pCtx.PRStats.TotalAdditions != 15 || pCtx.PRStats.TotalDeletions != 3 {
		t.Errorf("expected PR stats additions=15 deletions=3, got %v", pCtx.PRStats)
	}
}
