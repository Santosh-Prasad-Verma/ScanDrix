package tui_test

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/tui"
	"github.com/scandrix/backend/pkg/models"
)

const sampleDiff = `diff --git a/server.go b/server.go
--- a/server.go
+++ b/server.go
@@ -1,5 +1,6 @@
 package main
 
+import "crypto/md5"
 func hash(s string) {
-	h := sha256.New()
+	h := md5.New()
 }`

func TestTUIModelInitializationAndKeyNavigation(t *testing.T) {
	runner := engine.NewCLIRunner()
	m := tui.NewModel(sampleDiff, runner)

	// Test Initial State
	if m.Init() != nil {
		t.Fatalf("expected nil Init() command")
	}

	// Test Window resize
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tui.Model)

	// Test Tab Switching (Tab -> TabDiff)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(tui.Model)

	viewDiff := m.View()
	if !strings.Contains(viewDiff, "Unified Diff Inspection") {
		t.Fatalf("expected diff inspection view, got: %s", viewDiff)
	}

	// Test Tab Switching (Tab -> TabStats)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(tui.Model)

	viewStats := m.View()
	if !strings.Contains(viewStats, "Security & Quality Scorecard") {
		t.Fatalf("expected scorecard view, got: %s", viewStats)
	}

	// Test Jump Tab ("1" -> TabFindings)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(tui.Model)

	viewFindings := m.View()
	if !strings.Contains(viewFindings, "Findings Navigator") {
		t.Fatalf("expected findings navigator view, got: %s", viewFindings)
	}

	// Test Navigation (Down key)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(tui.Model)

	// Test Filter Toggle ("f")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(tui.Model)

	// Test Quit ("q")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected tea.Quit command on 'q'")
	}
}

func TestApplyFindingFix(t *testing.T) {
	// ApplyFindingFix is bounded to the working directory (it passes "." to the
	// engine), so the fixture file has to live under it. Using os.TempDir here
	// would be asserting the unbounded behaviour that was removed: a finding
	// naming an absolute path anywhere on the host used to be applied.
	dir, err := os.MkdirTemp(".", "tui_test_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	tmpFile, err := os.CreateTemp(dir, "tui_test_*.go")
	if err != nil {
		t.Fatalf("failed creating temp file: %v", err)
	}

	content := "package main\n\nvar x = \"bad_secret\"\n"
	_, _ = tmpFile.WriteString(content)
	tmpFile.Close()

	finding := models.CodeFinding{
		FilePath:      tmpFile.Name(),
		StartLine:     3,
		EndLine:       3,
		SuggestedDiff: "var x = os.Getenv(\"SECRET\")",
	}

	msg, err := tui.ApplyFindingFix(finding)
	if err != nil {
		t.Fatalf("unexpected error applying fix: %v", err)
	}

	if !strings.Contains(msg, "Applied fix") {
		t.Fatalf("unexpected msg: %s", msg)
	}

	modified, _ := os.ReadFile(tmpFile.Name())
	if !strings.Contains(string(modified), "os.Getenv") {
		t.Fatalf("file content not replaced properly: %s", string(modified))
	}
}

// A finding naming a path outside the working directory must be refused. The TUI
// used to pass "." to the engine yet accept an absolute FilePath, so any
// finding could rewrite any file the process could write.
func TestApplyFindingFixRefusesPathsOutsideTheWorkingDirectory(t *testing.T) {
	outside, err := os.CreateTemp("", "tui_outside_*.go")
	if err != nil {
		t.Fatalf("failed creating temp file: %v", err)
	}
	defer os.Remove(outside.Name())
	content := "package main\n\nvar x = \"bad_secret\"\n"
	_, _ = outside.WriteString(content)
	outside.Close()

	finding := models.CodeFinding{
		FilePath:      outside.Name(),
		StartLine:     3,
		EndLine:       3,
		SuggestedDiff: "var x = os.Getenv(\"SECRET\")",
	}

	if _, err := tui.ApplyFindingFix(finding); err == nil {
		t.Fatal("expected a finding outside the working directory to be refused")
	}

	modified, readErr := os.ReadFile(outside.Name())
	if readErr != nil {
		t.Fatalf("failed reading the file: %v", readErr)
	}
	if string(modified) != content {
		t.Fatalf("the out-of-tree file was modified: %q", string(modified))
	}
}
