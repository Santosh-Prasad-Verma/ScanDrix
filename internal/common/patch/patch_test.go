package patch

import (
	"strings"
	"testing"
)

func TestHandlePatchDeletions(t *testing.T) {
	// Empty patch with non-add/modify editType returns nil
	if res := HandlePatchDeletions("", "test.go", "deleted"); res != nil {
		t.Fatalf("expected nil for deleted empty patch, got %v", *res)
	}

	// Patch with only deletions should have deletion hunks removed
	diffOnlyDeletions := strings.Join([]string{
		"@@ -10,3 +10,0 @@",
		"-line 1",
		"-line 2",
		"-line 3",
	}, "\n")

	res := HandlePatchDeletions(diffOnlyDeletions, "test.go", "modified")
	if res == nil || *res != "" {
		t.Fatalf("expected empty patch for deletions-only hunk, got %v", res)
	}

	// Patch with additions and deletions
	mixedDiff := strings.Join([]string{
		"@@ -10,3 +10,2 @@",
		"-line 1",
		"-line 2",
		"+line 1 new",
		"+line 2 new",
	}, "\n")

	resMixed := HandlePatchDeletions(mixedDiff, "test.go", "modified")
	if resMixed == nil || !strings.Contains(*resMixed, "+line 1 new") {
		t.Fatalf("expected mixed diff to retain additions, got %v", resMixed)
	}
}

func TestConvertToHunksWithLinesNumbers(t *testing.T) {
	patchInput := strings.Join([]string{
		"@@ -1,3 +1,4 @@",
		" context 1",
		"+added 1",
		"+added 2",
		"-deleted 1",
		" context 2",
	}, "\n")

	hunkStr := ConvertToHunksWithLinesNumbers(patchInput, FileInput{Filename: "service.go"})
	if !strings.Contains(hunkStr, "## file: 'service.go'") {
		t.Fatalf("expected file header in hunkStr, got %s", hunkStr)
	}
	if !strings.Contains(hunkStr, "__new hunk__") {
		t.Fatalf("expected __new hunk__ marker, got %s", hunkStr)
	}
	if !strings.Contains(hunkStr, "__old hunk__") {
		t.Fatalf("expected __old hunk__ marker, got %s", hunkStr)
	}
	if !strings.Contains(hunkStr, "2 +added 1") {
		t.Fatalf("expected line numbered added line, got %s", hunkStr)
	}
}

func TestConvertToUnifiedDiffWithLineNumbers(t *testing.T) {
	patchInput := strings.Join([]string{
		"@@ -5,4 +10,4 @@",
		" context before",
		"-old line",
		"+new line",
		" context after",
	}, "\n")

	unified := ConvertToUnifiedDiffWithLineNumbers(patchInput, FileInput{Filename: "pkg/api.go"})
	if !strings.Contains(unified, "## file: 'pkg/api.go'") {
		t.Fatalf("expected file header, got %s", unified)
	}
	if !strings.Contains(unified, "    10  context before") {
		t.Fatalf("expected line 10 padded, got %s", unified)
	}
	if !strings.Contains(unified, "       -old line") {
		t.Fatalf("expected unnumbered padded deletion, got %s", unified)
	}
	if !strings.Contains(unified, "    11 +new line") {
		t.Fatalf("expected numbered addition, got %s", unified)
	}
}

func TestExtractLinesFromDiffHunk(t *testing.T) {
	hunk := strings.Join([]string{
		"@@ -1,5 +10,6 @@",
		"__new hunk__",
		"10  ctx",
		"11 +first",
		"12 +second",
		"13  ctx",
		"14 +third",
	}, "\n")

	ranges := ExtractLinesFromDiffHunk(hunk)
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d: %+v", len(ranges), ranges)
	}
	if ranges[0].Start != 11 || ranges[0].End != 12 {
		t.Fatalf("expected range 11-12, got %+v", ranges[0])
	}
	if ranges[1].Start != 14 || ranges[1].End != 14 {
		t.Fatalf("expected range 14-14, got %+v", ranges[1])
	}
}

func TestExtractLinesFromUnifiedDiff(t *testing.T) {
	unified := strings.Join([]string{
		"## file: 'test.go'",
		"@@ -1,5 +20,5 @@",
		"    20  ctx",
		"    21 +added 1",
		"    22 +added 2",
		"    23 +added 3",
		"       -removed",
		"    24  ctx",
		"    25 +added separate",
	}, "\n")

	ranges := ExtractLinesFromUnifiedDiff(unified)
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d: %+v", len(ranges), ranges)
	}
	if ranges[0].Start != 21 || ranges[0].End != 23 {
		t.Fatalf("expected range 21-23, got %+v", ranges[0])
	}
	if ranges[1].Start != 25 || ranges[1].End != 25 {
		t.Fatalf("expected range 25-25, got %+v", ranges[1])
	}
}
