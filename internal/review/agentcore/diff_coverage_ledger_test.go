package agentcore

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestExtractChangedLineRanges(t *testing.T) {
	patch := `
@@ -10,5 +15,10 @@ func Foo() {
+newline1
+newline2
@@ -40,10 +50,20 @@ func Bar() {
+newline3
@@ -100 +120 @@ func Baz() {
+newline4
`
	ranges := ExtractChangedLineRanges(patch)
	if len(ranges) != 3 {
		t.Fatalf("expected 3 ranges, got %d", len(ranges))
	}

	// Range 1: 15 to 15+10-1 = 24
	if ranges[0][0] != 15 || ranges[0][1] != 24 {
		t.Errorf("expected range 1 to be [15, 24], got %v", ranges[0])
	}

	// Range 2: 50 to 50+20-1 = 69
	if ranges[1][0] != 50 || ranges[1][1] != 69 {
		t.Errorf("expected range 2 to be [50, 69], got %v", ranges[1])
	}

	// Range 3: 120 to 120 (count=1 default)
	if ranges[2][0] != 120 || ranges[2][1] != 120 {
		t.Errorf("expected range 3 to be [120, 120], got %v", ranges[2])
	}
}

func TestMergeRanges(t *testing.T) {
	tests := []struct {
		name     string
		input    [][2]int
		expected [][2]int
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single",
			input:    [][2]int{{10, 20}},
			expected: [][2]int{{10, 20}},
		},
		{
			name:     "overlapping",
			input:    [][2]int{{10, 30}, {20, 50}},
			expected: [][2]int{{10, 50}},
		},
		{
			name:     "contiguous",
			input:    [][2]int{{1, 50}, {51, 100}},
			expected: [][2]int{{1, 100}},
		},
		{
			name:     "disjoint",
			input:    [][2]int{{1, 20}, {40, 60}},
			expected: [][2]int{{1, 20}, {40, 60}},
		},
		{
			name:     "unsorted and inverted",
			input:    [][2]int{{60, 40}, {20, 10}, {30, 15}},
			expected: [][2]int{{10, 30}, {40, 60}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged := MergeRanges(tc.input)
			if len(merged) != len(tc.expected) {
				t.Fatalf("expected %d ranges, got %d: %v", len(tc.expected), len(merged), merged)
			}
			for i := range merged {
				if merged[i] != tc.expected[i] {
					t.Errorf("at index %d: expected %v, got %v", i, tc.expected[i], merged[i])
				}
			}
		})
	}
}

func TestPathsMatch(t *testing.T) {
	// Exact match
	if !PathsMatch("src/auth/service.go", "src/auth/service.go") {
		t.Errorf("expected exact match to succeed")
	}

	// Absolute prefix match
	if !PathsMatch("src/auth/service.go", "/home/repo/src/auth/service.go") {
		t.Errorf("expected repo-prefix match to succeed")
	}
	if !PathsMatch("/home/repo/src/auth/service.go", "src/auth/service.go") {
		t.Errorf("expected inverted repo-prefix match to succeed")
	}

	// Negative match: different directory prefix
	if PathsMatch("src/auth/service.go", "other/auth/service.go") {
		t.Errorf("expected different directory prefix to fail")
	}

	// CRITICAL: Basename-only collisions MUST NOT match
	if PathsMatch("apps/web/page.tsx", "apps/admin/page.tsx") {
		t.Errorf("basename collision must not match across different folders")
	}
}

func TestDiffCoverageLedger_MultiHunkPartialRead(t *testing.T) {
	patch := `
@@ -10,5 +10,10 @@ func Header() {
+hunk1
@@ -100,5 +100,10 @@ func Footer() {
+hunk2
`
	files := []ChangedFile{
		{
			Filename: "src/component.tsx",
			Patch:    patch,
		},
	}

	ledger := NewDiffCoverageLedger(DiffCoverageLedgerParams{
		ChangedFiles: files,
	})

	sum := ledger.Summary()
	if sum.TotalTargets != 1 || sum.PendingTargets != 1 {
		t.Fatalf("expected 1 total, 1 pending target, got %+v", sum)
	}

	// Read only hunk 1 (lines 10 to 20)
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path":      "src/component.tsx",
		"startLine": 10,
		"endLine":   20,
	}, 1)

	// File must STILL be pending because hunk 2 (lines 100-109) has not been read
	sum = ledger.Summary()
	if sum.PendingTargets != 1 {
		t.Errorf("expected file to remain pending after partial hunk read, got %+v", sum)
	}

	// Debt note must indicate pending lines for hunk 2
	debt := ledger.DebtNote()
	if debt == nil || !strings.Contains(*debt, "src/component.tsx") || !strings.Contains(*debt, "100-109") {
		t.Errorf("expected debt note mentioning pending lines 100-109, got %v", debt)
	}

	// Now read hunk 2 (lines 95 to 115)
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path":      "src/component.tsx",
		"startLine": 95,
		"endLine":   115,
	}, 2)

	// Now target is fully covered
	sum = ledger.Summary()
	if sum.PendingTargets != 0 {
		t.Errorf("expected 0 pending targets, got %+v", sum)
	}
	if ledger.DebtNote() != nil {
		t.Errorf("expected nil debt note when fully covered, got %v", *ledger.DebtNote())
	}
}

func TestDiffCoverageLedger_TieringSatisfactionGate(t *testing.T) {
	files := []ChangedFile{
		{Filename: "src/auth/jwt.go", Patch: "@@ -1,5 +1,10 @@\n+auth"},
		{Filename: "src/api/user.go", Patch: "@@ -1,5 +1,10 @@\n+user"},
		{Filename: "src/utils/math.go", Patch: "@@ -1,5 +1,10 @@\n+math"},
	}

	tiers := map[string]CoverageTier{
		"src/auth/jwt.go":   TierCritical,
		"src/api/user.go":   TierWarm,
		"src/utils/math.go": TierOptional,
	}

	ledger := NewDiffCoverageLedger(DiffCoverageLedgerParams{
		ChangedFiles:      files,
		FileTiers:         tiers,
		CoverageThreshold: 0.70,
	})

	// Initial: 0% coverage, critical pending -> not satisfied
	if ledger.IsSatisfied() {
		t.Errorf("should not be satisfied initially")
	}

	// Read warm file only (user.go)
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path": "src/api/user.go",
	}, 1)

	// Critical still pending -> not satisfied
	if ledger.IsSatisfied() {
		t.Errorf("should not be satisfied when critical is pending")
	}

	// Read critical file (jwt.go)
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path": "src/auth/jwt.go",
	}, 2)

	// Critical is covered, so IsSatisfied() is true
	if !ledger.IsSatisfied() {
		t.Errorf("IsSatisfied should be true once critical is covered")
	}

	// But 2 of 3 targets covered (66.6%) -> still below 70% threshold, so IsCoverageSatisfied is false
	if ledger.IsCoverageSatisfied() {
		t.Errorf("IsCoverageSatisfied should be false when ratio (2/3 = 66%%) is below 70%% threshold")
	}

	// Read optional file (math.go) -> 3 of 3 covered (100%) >= 70% and critical covered
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path": "src/utils/math.go",
	}, 3)

	if !ledger.IsCoverageSatisfied() {
		t.Errorf("IsCoverageSatisfied should be true when critical covered and ratio 100%% >= 70%%")
	}
}

func TestDiffCoverageLedger_Formatting(t *testing.T) {
	files := []ChangedFile{
		{
			Filename: "src/security/crypto.go",
			Patch:    "@@ -10,5 +10,10 @@\n+crypto",
		},
		{
			Filename: "src/views/button.tsx",
			Patch:    "@@ -50,5 +50,5 @@\n+button",
		},
	}

	tiers := map[string]CoverageTier{
		"src/security/crypto.go": TierCritical,
		"src/views/button.tsx":   TierOptional,
	}

	promptFormat := FormatCoverageTargetsForPrompt(files, 10, tiers)
	if !strings.Contains(promptFormat, "CRITICAL files (1)") {
		t.Errorf("expected prompt format to contain CRITICAL tier header")
	}
	if !strings.Contains(promptFormat, "OPTIONAL files (1)") {
		t.Errorf("expected prompt format to contain OPTIONAL tier header")
	}
}

func TestDiffCoverageLedger_ConcurrentAccessRace(t *testing.T) {
	files := make([]ChangedFile, 20)
	for i := 0; i < 20; i++ {
		files[i] = ChangedFile{
			Filename: fmt.Sprintf("src/pkg%d/file.go", i),
			Patch:    "@@ -10,5 +10,15 @@\n+line",
		}
	}

	ledger := NewDiffCoverageLedger(DiffCoverageLedgerParams{
		ChangedFiles: files,
	})

	var wg sync.WaitGroup
	// Concurrently read and mark
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			fileIdx := idx % 20
			ledger.MarkFromToolCall("readFile", map[string]any{
				"path":      fmt.Sprintf("src/pkg%d/file.go", fileIdx),
				"startLine": 10,
				"endLine":   24,
			}, idx)
		}(i)

		go func() {
			defer wg.Done()
			_ = ledger.Summary()
			_ = ledger.CoverageSummary()
			_ = ledger.DebtNote()
			_ = ledger.IsSatisfied()
			_ = ledger.IsLowCoverage()
		}()
	}

	wg.Wait()
}
