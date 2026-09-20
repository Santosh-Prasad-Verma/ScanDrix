package diff

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiff(t *testing.T) {
	rawDiff := `diff --git a/main.go b/main.go
index 83db48f..bf269f4 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@ package main
 
 func main() {
-	println("hello")
+	// Modified greeting
+	println("hello world")
 }
`

	patches, err := ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed: %v", err)
	}

	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	p := patches[0]
	if p.OldPath != "main.go" || p.NewPath != "main.go" {
		t.Errorf("unexpected paths: %s -> %s", p.OldPath, p.NewPath)
	}

	if p.Additions != 2 || p.Deletions != 1 {
		t.Errorf("expected 2 additions and 1 deletion, got +%d -%d", p.Additions, p.Deletions)
	}

	if len(p.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(p.Hunks))
	}

	h := p.Hunks[0]
	if h.OldStart != 1 || h.NewStart != 1 {
		t.Errorf("unexpected hunk line numbers: old=%d, new=%d", h.OldStart, h.NewStart)
	}
}

func TestParseUnifiedDiffWithExtremeLongLines(t *testing.T) {
	// Create a minified line of 2MB (exceeding old 1MB scanner buffer limit)
	longLine := strings.Repeat("var a = 1; ", 200000) // ~2.2 MB line
	rawDiff := "diff --git a/bundle.min.js b/bundle.min.js\n" +
		"--- a/bundle.min.js\n" +
		"+++ b/bundle.min.js\n" +
		"@@ -1,1 +1,1 @@\n" +
		"+" + longLine + "\n" +
		"diff --git a/normal.go b/normal.go\n" +
		"--- a/normal.go\n" +
		"+++ b/normal.go\n" +
		"@@ -1,1 +1,1 @@\n" +
		"+func Normal() {}\n"

	patches, err := ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed on extreme line length: %v", err)
	}

	if len(patches) != 2 {
		t.Fatalf("expected 2 patches despite extreme line length, got %d", len(patches))
	}

	if patches[1].NewPath != "normal.go" {
		t.Errorf("expected second patch to be normal.go, got %s", patches[1].NewPath)
	}
}

