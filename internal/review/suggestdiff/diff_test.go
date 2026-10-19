package suggestdiff

import (
	"strings"
	"testing"
)

func TestComputeLineDiff_Simple(t *testing.T) {
	oldLines := []string{"func login() {", "  validate()", "}"}
	newLines := []string{"func login() {", "  validate()", "  sanitize()", "}"}

	diff := ComputeLineDiff(oldLines, newLines)
	if len(diff) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(diff))
	}

	foundAdded := false
	for _, l := range diff {
		if l.Type == DiffAdded && strings.Contains(l.Content, "sanitize()") {
			foundAdded = true
		}
	}
	if !foundAdded {
		t.Errorf("expected added line with sanitize()")
	}
}

func TestRenderUnifiedDiff(t *testing.T) {
	oldCode := "const API_URL = 'http://old.internal'"
	newCode := "const API_URL = process.env.API_URL || 'https://api.scandrix.dev'"

	patch := RenderUnifiedDiff(oldCode, newCode, "config.ts")
	if !strings.HasPrefix(patch, "--- a/config.ts\n+++ b/config.ts") {
		t.Fatalf("patch missing git headers:\n%s", patch)
	}
	if !strings.Contains(patch, "+const API_URL = process.env.API_URL") {
		t.Errorf("patch missing addition:\n%s", patch)
	}
	if !strings.Contains(patch, "-const API_URL = 'http://old.internal'") {
		t.Errorf("patch missing deletion:\n%s", patch)
	}
}

func TestRenderANSI(t *testing.T) {
	lines := []DiffLine{
		{Type: DiffContext, Content: "unchanged"},
		{Type: DiffAdded, Content: "new code"},
		{Type: DiffDeleted, Content: "old code"},
	}

	colored := RenderANSI(lines)
	if !strings.Contains(colored, "\033[32m+new code") {
		t.Errorf("expected green ANSI code for additions:\n%s", colored)
	}
	if !strings.Contains(colored, "\033[31m-old code") {
		t.Errorf("expected red ANSI code for deletions:\n%s", colored)
	}
}
