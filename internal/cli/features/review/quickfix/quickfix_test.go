// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package quickfix

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestParseUnifiedDiff_SingleHunk(t *testing.T) {
	diff := `--- a/src/auth.go
+++ b/src/auth.go
@@ -10,3 +10,3 @@
 func Auth() {
-	var secret = "hardcoded"
+	var secret = os.Getenv("SECRET")
 	println(secret)
 }`

	patches, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff failed: %v", err)
	}

	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	p := patches[0]
	if p.OldPath != "src/auth.go" || p.NewPath != "src/auth.go" {
		t.Errorf("unexpected paths: %s -> %s", p.OldPath, p.NewPath)
	}
	if len(p.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(p.Hunks))
	}
}

func TestApplyPatchToFile_Success(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "auth.go")
	initialContent := `package main

import "fmt"

func Auth() {
	var secret = "hardcoded"
	fmt.Println(secret)
}
`
	_ = os.WriteFile(filePath, []byte(initialContent), 0600)

	diff := `--- a/auth.go
+++ b/auth.go
@@ -5,3 +5,3 @@
 func Auth() {
-	var secret = "hardcoded"
+	var secret = os.Getenv("SECRET")
 	fmt.Println(secret)
`

	res, err := ApplySuggestedDiff(filePath, diff, true)
	if err != nil {
		t.Fatalf("ApplySuggestedDiff failed: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected res.Success=true")
	}

	updatedBytes, _ := os.ReadFile(filePath)
	updatedContent := string(updatedBytes)
	if !strings.Contains(updatedContent, `var secret = os.Getenv("SECRET")`) {
		t.Fatalf("expected updated content to contain os.Getenv, got: %s", updatedContent)
	}

	// Verify backup
	backupBytes, err := os.ReadFile(filePath + ".bak")
	if err != nil || string(backupBytes) != initialContent {
		t.Fatalf("expected backup to match initial content")
	}
}

func TestInteractiveFixSession_YesAndSkip(t *testing.T) {
	tempDir := t.TempDir()
	filePath1 := filepath.Join(tempDir, "file1.go")
	filePath2 := filepath.Join(tempDir, "file2.go")

	_ = os.WriteFile(filePath1, []byte("func F1() { println(1) }\n"), 0600)
	_ = os.WriteFile(filePath2, []byte("func F2() { println(2) }\n"), 0600)

	findings := []models.CodeFinding{
		{
			Title:         "Fix F1",
			FilePath:      "file1.go",
			StartLine:     1,
			SuggestedDiff: "-println(1)\n+println(10)\n",
		},
		{
			Title:         "Fix F2",
			FilePath:      "file2.go",
			StartLine:     1,
			SuggestedDiff: "-println(2)\n+println(20)\n",
		},
	}

	// User inputs: "y" for first finding, "n" for second finding
	userInput := "y\nn\n"
	var out bytes.Buffer
	session := NewInteractiveFixSession(tempDir, strings.NewReader(userInput), &out)

	applied, skipped, err := session.ReviewAndApply(findings)
	if err != nil {
		t.Fatalf("ReviewAndApply failed: %v", err)
	}

	if applied != 1 || skipped != 1 {
		t.Fatalf("expected 1 applied and 1 skipped, got applied=%d, skipped=%d", applied, skipped)
	}

	// Verify file1 updated
	f1Bytes, _ := os.ReadFile(filePath1)
	if !strings.Contains(string(f1Bytes), "println(10)") {
		t.Fatalf("file1 not updated: %s", string(f1Bytes))
	}

	// Verify file2 untouched
	f2Bytes, _ := os.ReadFile(filePath2)
	if !strings.Contains(string(f2Bytes), "println(2)") {
		t.Fatalf("file2 was modified when skipped: %s", string(f2Bytes))
	}
}
