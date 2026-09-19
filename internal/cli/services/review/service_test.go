// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestExtractChangedFiles(t *testing.T) {
	diff := `diff --git a/pkg/auth/token.go b/pkg/auth/token.go
index 1234567..89abcdef 100644
--- a/pkg/auth/token.go
+++ b/pkg/auth/token.go
@@ -10,3 +10,3 @@
-func Old() {}
+func New() {}
diff --git a/pkg/auth/token_test.go b/pkg/auth/token_test.go
--- a/pkg/auth/token_test.go
+++ b/pkg/auth/token_test.go
@@ -5,1 +5,1 @@
+func TestToken() {}
diff --git a/deleted.txt b/dev/null
--- a/deleted.txt
+++ /dev/null
`
	files := ExtractChangedFiles(diff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "pkg/auth/token.go" || files[1] != "pkg/auth/token_test.go" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestBuildNoChangesMessages(t *testing.T) {
	// Specific files
	msgs1 := BuildNoChangesMessages([]string{"foo.go"}, ReviewOptions{}, nil)
	if len(msgs1) == 0 || msgs1[0] != "None of the requested files have diff content in the selected scope." {
		t.Errorf("unexpected files msgs: %v", msgs1)
	}

	// Branch
	msgs2 := BuildNoChangesMessages(nil, ReviewOptions{Branch: "main"}, nil)
	if len(msgs2) == 0 || msgs2[0] != "No diff was found against 'main'." {
		t.Errorf("unexpected branch msgs: %v", msgs2)
	}

	// Commit
	msgs3 := BuildNoChangesMessages(nil, ReviewOptions{Commit: "abc1234"}, nil)
	if len(msgs3) == 0 || msgs3[0] != "No diff was found for commit 'abc1234'." {
		t.Errorf("unexpected commit msgs: %v", msgs3)
	}

	// Staged
	msgs4 := BuildNoChangesMessages(nil, ReviewOptions{Staged: true}, nil)
	if len(msgs4) == 0 || msgs4[0] != "There are no staged changes to review." {
		t.Errorf("unexpected staged msgs: %v", msgs4)
	}

	// Untracked hints
	msgs5 := BuildNoChangesMessages(nil, ReviewOptions{}, []string{"new1.go", "new2.go"})
	foundUntracked := false
	for _, m := range msgs5 {
		if m == "Found 2 untracked file(s) (run 'git add' to track and stage them)." {
			foundUntracked = true
			break
		}
	}
	if !foundUntracked {
		t.Errorf("expected untracked message in %v", msgs5)
	}
}

func TestEvaluateBlocking(t *testing.T) {
	cases := []struct {
		crit, high, med, low int
		threshold            string
		expectedBlocking     bool
		expectedExit         int
	}{
		{0, 1, 0, 0, "CRITICAL", false, 0},
		{1, 0, 0, 0, "CRITICAL", true, 1},
		{0, 1, 0, 0, "HIGH", true, 1},
		{0, 0, 1, 0, "HIGH", false, 0},
		{0, 0, 1, 0, "MEDIUM", true, 1},
		{0, 0, 0, 1, "LOW", true, 1},
	}

	for _, c := range cases {
		isBlocking, exitCode := evaluateBlocking(c.crit, c.high, c.med, c.low, c.threshold)
		if isBlocking != c.expectedBlocking || exitCode != c.expectedExit {
			t.Errorf("eval(%d,%d,%d,%d, %s) = (%v, %d); want (%v, %d)",
				c.crit, c.high, c.med, c.low, c.threshold, isBlocking, exitCode, c.expectedBlocking, c.expectedExit)
		}
	}
}

func TestFilterFindingsByFocus(t *testing.T) {
	findings := []models.CodeFinding{
		{Title: "SQL Injection", Description: "Unsanitized user input", Category: "security"},
		{Title: "Dead Code", Description: "Unused variable", Category: "cleanliness"},
	}

	res := filterFindingsByFocus(findings, "sql")
	if len(res) != 1 || res[0].Title != "SQL Injection" {
		t.Errorf("expected 1 finding for focus 'sql', got %v", res)
	}
}

func TestServiceAnalyzeWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	patchFile := filepath.Join(tmpDir, "test.patch")
	patchContent := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
-package main
+package main // changed
`
	if err := os.WriteFile(patchFile, []byte(patchContent), 0600); err != nil {
		t.Fatal(err)
	}

	svc := DefaultService()
	res, err := svc.Analyze(context.Background(), ReviewOptions{
		File:    patchFile,
		Offline: true,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if res.Status != "passed" {
		t.Errorf("expected passed, got %s", res.Status)
	}
	if res.FilesAnalyzed != 1 {
		t.Errorf("expected 1 file analyzed, got %d", res.FilesAnalyzed)
	}
}
