// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package lifecycle

import (
	"os"
	"testing"
	"time"
)

func TestTurnMonitorLifecycle(t *testing.T) {
	monitor := NewTurnMonitor(10)

	sessionID := "sess-test-123"
	turnID := "turn-001"

	// Start turn
	metrics := monitor.StartTurn(sessionID, turnID)
	if metrics == nil || metrics.TurnID != turnID {
		t.Fatalf("expected active turn %s", turnID)
	}

	// Record tool calls
	monitor.RecordToolExecution(turnID, "view_file", 15*time.Millisecond, true, 1024)
	monitor.RecordToolExecution(turnID, "replace_file_content", 45*time.Millisecond, true, 512)
	monitor.RecordToolExecution(turnID, "run_command", 120*time.Millisecond, false, 0)

	// Record files touched
	monitor.RecordFileAccess(turnID, []string{"internal/auth/jwt.go"}, []string{"internal/auth/jwt.go"})

	// Complete turn
	completed, err := monitor.EndTurn(turnID, 2500)
	if err != nil {
		t.Fatalf("unexpected error ending turn: %v", err)
	}

	if completed.TotalToolsCount != 3 {
		t.Errorf("expected 3 total tool calls, got %d", completed.TotalToolsCount)
	}
	if completed.FailedToolsCount != 1 {
		t.Errorf("expected 1 failed tool call, got %d", completed.FailedToolsCount)
	}
	if completed.EstimatedTokens != 2500 {
		t.Errorf("expected 2500 tokens, got %d", completed.EstimatedTokens)
	}

	summary := monitor.FormatTurnSummary(completed)
	if len(summary) == 0 {
		t.Errorf("expected non-empty summary string")
	}

	history := monitor.GetHistory()
	if len(history) != 1 {
		t.Errorf("expected history length 1, got %d", len(history))
	}
}

func TestTurnRecorderWAL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scandrix-turn-recorder-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	recorder, err := NewTurnRecorder(tmpDir)
	if err != nil {
		t.Fatalf("failed creating turn recorder: %v", err)
	}
	defer recorder.Close()

	// Log start of two turns
	_ = recorder.LogTurnStart("sess-1", "turn-1", "First prompt")
	_ = recorder.LogTurnStart("sess-1", "turn-2", "Second prompt")

	// Complete only turn-1
	_ = recorder.LogTurnComplete("sess-1", "turn-1", "First turn finished successfully")

	// Recovery should identify turn-2 as unfinished
	unfinished, err := recorder.RecoverUnfinishedTurns()
	if err != nil {
		t.Fatalf("unexpected error during recovery: %v", err)
	}

	if len(unfinished) != 1 {
		t.Fatalf("expected 1 unfinished turn, got %d", len(unfinished))
	}
	if unfinished[0].TurnID != "turn-2" {
		t.Errorf("expected turn-2 to be unfinished, got %s", unfinished[0].TurnID)
	}
}

func TestComputeGitTurnDelta(t *testing.T) {
	before := &GitTurnSnapshot{
		HeadSHA:       "abcdef0123456789",
		ModifiedFiles: []string{"file1.go"},
	}

	after := &GitTurnSnapshot{
		HeadSHA:        "abcdef0123456789",
		ModifiedFiles:  []string{"file1.go", "file2.go"},
		UntrackedFiles: []string{"new_file.go"},
	}

	delta := ComputeDelta(before, after)
	if delta == nil {
		t.Fatalf("expected non-nil delta")
	}

	if len(delta.FilesAdded) != 1 || delta.FilesAdded[0] != "new_file.go" {
		t.Errorf("expected new_file.go added, got %v", delta.FilesAdded)
	}
	if len(delta.FilesModified) != 1 || delta.FilesModified[0] != "file2.go" {
		t.Errorf("expected file2.go modified, got %v", delta.FilesModified)
	}

	summary := FormatDeltaSummary(delta)
	if len(summary) == 0 {
		t.Errorf("expected non-empty delta summary")
	}
}
