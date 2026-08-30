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
	// Create temporary test file
	tmpFile, err := os.CreateTemp("", "tui_test_*.go")
	if err != nil {
		t.Fatalf("failed creating temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

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
