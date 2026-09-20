// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"testing"
)

func TestCountDiffChanges(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,4 +1,5 @@
 package main
-import "fmt"
+import "os"
+import "strings"
 func main() {}
`

	summary := CountDiffChanges(diff)
	if summary.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", summary.Additions)
	}
	if summary.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", summary.Deletions)
	}
}

func TestParseGitStatus(t *testing.T) {
	tests := []struct {
		char string
		want string
	}{
		{"A", "added"},
		{"M", "modified"},
		{"D", "deleted"},
		{"R100", "renamed"},
		{"?", "modified"},
		{"", "modified"},
	}

	for _, tt := range tests {
		got := ParseGitStatus(tt.char)
		if got != tt.want {
			t.Errorf("ParseGitStatus(%q) = %q, want %q", tt.char, got, tt.want)
		}
	}
}

func TestParseGitNameStatusOutput(t *testing.T) {
	raw := "M\tinternal/auth.go\nA\tinternal/new_file.go\nD\told_file.go\nR100\told_name.go\tnew_name.go\n"
	entries := ParseGitNameStatusOutput(raw)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].File != "internal/auth.go" || entries[0].Status != "modified" {
		t.Errorf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].File != "internal/new_file.go" || entries[1].Status != "added" {
		t.Errorf("unexpected entry 1: %+v", entries[1])
	}
	if entries[2].File != "old_file.go" || entries[2].Status != "deleted" {
		t.Errorf("unexpected entry 2: %+v", entries[2])
	}
	if entries[3].File != "new_name.go" || entries[3].Status != "renamed" {
		t.Errorf("unexpected entry 3: %+v", entries[3])
	}

	files, statusMap := CreateFileSelectionFromNameStatus(raw)
	if len(files) != 4 || statusMap["new_name.go"] != "renamed" {
		t.Errorf("unexpected selection map: %+v, %+v", files, statusMap)
	}
}
