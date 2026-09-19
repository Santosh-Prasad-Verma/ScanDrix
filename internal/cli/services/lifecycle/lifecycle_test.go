// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/trace"
)

// MockSessionsAPI records all events dispatched via SendEvent
type MockSessionsAPI struct {
	mu     sync.Mutex
	events []any
}

func (m *MockSessionsAPI) SendEvent(ctx context.Context, event any, repoRoot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *MockSessionsAPI) FlushPending(ctx context.Context, repoRoot string) error {
	return nil
}

func (m *MockSessionsAPI) GetEvents() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]any, len(m.events))
	copy(copied, m.events)
	return copied
}

func TestLifecycleService_FullFlow(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", filepath.Join(tempDir, "scandrix_home"))

	mockSessions := &MockSessionsAPI{}
	gitSvc := git.NewService(tempDir)
	svc := NewService(nil, mockSessions, gitSvc)

	ctx := context.Background()
	sessionID := "test_sess_flow_001"

	// 1. SessionStart
	startEvt := &trace.LifecycleEvent{
		Type:      "SessionStart",
		SessionID: sessionID,
	}
	if err := svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, startEvt); err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}

	events := mockSessions.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	startRemote, ok := events[0].(api.SessionStartEvent)
	if !ok || startRemote.SessionID != sessionID {
		t.Fatalf("unexpected start remote event: %+v", events[0])
	}

	// Verify local state was saved
	state, err := trace.LoadSessionState(tempDir, sessionID)
	if err != nil || state == nil {
		t.Fatalf("failed to load session state: %v", err)
	}

	// 2. TurnStart
	turnStartEvt := &trace.LifecycleEvent{
		Type:      "TurnStart",
		SessionID: sessionID,
		Prompt:    "Refactor the authentication flow with Bearer sk-ant-api03-test",
	}
	if err := svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, turnStartEvt); err != nil {
		t.Fatalf("TurnStart failed: %v", err)
	}

	events = mockSessions.GetEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	turnStartRemote, ok := events[1].(api.TurnStartEvent)
	if !ok || turnStartRemote.SessionID != sessionID {
		t.Fatalf("unexpected turn start remote event: %+v", events[1])
	}
	// Verify prompt redaction
	if trace.ContainsSecret(turnStartRemote.Prompt) {
		t.Errorf("expected prompt secret to be scrubbed, got %s", turnStartRemote.Prompt)
	}

	// Create a mock transcript file for TurnEnd
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")
	transcriptContent := `{"role":"user","content":"Refactor the auth"}
{"message":{"role":"assistant","content":[{"type":"text","text":"Refactored auth token validation"},{"type":"tool_use","name":"Write","input":{"file_path":"internal/auth.go"}}]}}
`
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0644); err != nil {
		t.Fatalf("failed to create mock transcript: %v", err)
	}

	// 3. TurnEnd
	turnEndEvt := &trace.LifecycleEvent{
		Type:       "TurnEnd",
		SessionID:  sessionID,
		SessionRef: transcriptPath,
	}
	if err := svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, turnEndEvt); err != nil {
		t.Fatalf("TurnEnd failed: %v", err)
	}

	events = mockSessions.GetEvents()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	turnEndRemote, ok := events[2].(api.TurnEndEvent)
	if !ok || turnEndRemote.SessionID != sessionID {
		t.Fatalf("unexpected turn end remote event: %+v", events[2])
	}
	if len(turnEndRemote.FilesModified) != 1 || turnEndRemote.FilesModified[0].Path != "internal/auth.go" {
		t.Errorf("expected modified file internal/auth.go, got %+v", turnEndRemote.FilesModified)
	}

	// Verify Dedup: firing TurnEnd again should be skipped
	if err := svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, turnEndEvt); err != nil {
		t.Fatalf("second TurnEnd failed: %v", err)
	}
	if len(mockSessions.GetEvents()) != 3 {
		t.Errorf("expected deduplication of second TurnEnd, got %d events", len(mockSessions.GetEvents()))
	}

	// 4. SubagentStart & End
	subStart := &trace.LifecycleEvent{
		Type:            "SubagentStart",
		SessionID:       sessionID,
		ToolUseID:       "tool_sub_123",
		SubagentType:    "architect",
		TaskDescription: "Analyze database indexes",
	}
	_ = svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, subStart)

	subEnd := &trace.LifecycleEvent{
		Type:      "SubagentEnd",
		SessionID: sessionID,
		ToolUseID: "tool_sub_123",
	}
	_ = svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, subEnd)

	events = mockSessions.GetEvents()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// 5. SessionEnd
	sessionEnd := &trace.LifecycleEvent{
		Type:      "SessionEnd",
		SessionID: sessionID,
	}
	if err := svc.Dispatch(ctx, tempDir, trace.AgentClaudeCode, sessionEnd); err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}

	events = mockSessions.GetEvents()
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}

	// Ephemeral turn state should have been removed
	statePath := trace.TurnStatePath(tempDir, sessionID)
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("expected turn state %s to be deleted after SessionEnd", statePath)
	}

	// Durable session log should still exist
	recordPath := trace.SessionRecordPath(tempDir, sessionID)
	if _, err := os.Stat(recordPath); os.IsNotExist(err) {
		t.Errorf("expected durable session log %s to exist", recordPath)
	}
}

func TestLifecycleService_EnableDisableHooks(t *testing.T) {
	tempDir := t.TempDir()
	codexCfg := filepath.Join(tempDir, "codex_config.toml")

	svc := NewService(nil, nil, nil)
	installed, err := svc.EnableHooks(tempDir, []string{"cursor", "claude-code", "codex"}, codexCfg)
	if err != nil {
		t.Fatalf("EnableHooks failed: %v", err)
	}
	if len(installed) != 3 {
		t.Errorf("expected 3 hooks installed, got %d", len(installed))
	}

	cursorMdc := filepath.Join(tempDir, ".cursor", "rules", "scandrix.mdc")
	if _, err := os.Stat(cursorMdc); err != nil {
		t.Errorf("expected cursor rule file %s", cursorMdc)
	}

	claudeJson := filepath.Join(tempDir, ".claude", "settings.json")
	if _, err := os.Stat(claudeJson); err != nil {
		t.Errorf("expected claude settings %s", claudeJson)
	}

	if _, err := os.Stat(codexCfg); err != nil {
		t.Errorf("expected codex config %s", codexCfg)
	}

	// Disable hooks
	if err := svc.DisableHooks(tempDir); err != nil {
		t.Fatalf("DisableHooks failed: %v", err)
	}
	if _, err := os.Stat(cursorMdc); !os.IsNotExist(err) {
		t.Errorf("expected cursor rule file to be removed")
	}
	if _, err := os.Stat(claudeJson); !os.IsNotExist(err) {
		t.Errorf("expected claude settings to be removed")
	}
}

func TestLifecycleService_CleanupStaleSessions(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("SCANDRIX_TRACE_HOME", filepath.Join(tempDir, "scandrix_home"))

	mockSessions := &MockSessionsAPI{}
	gitSvc := git.NewService(tempDir)
	svc := NewService(nil, mockSessions, gitSvc)

	ctx := context.Background()
	staleID := "stale_sess_999"

	// Create a state with timestamp 40 minutes ago
	oldTime := time.Now().Add(-40 * time.Minute).UTC().Format(time.RFC3339)
	oldState := &trace.SessionState{
		SessionID: staleID,
		TurnCount: 1,
		UpdatedAt: oldTime,
	}
	_ = os.MkdirAll(trace.TurnStateDir(tempDir), 0755)
	raw, _ := json.Marshal(oldState)
	_ = os.WriteFile(trace.TurnStatePath(tempDir, staleID), raw, 0600)

	if err := svc.CleanupStaleSessions(ctx, tempDir, trace.AgentClaudeCode); err != nil {
		t.Fatalf("CleanupStaleSessions failed: %v", err)
	}

	events := mockSessions.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 session end event for stale session, got %d", len(events))
	}
	endEvt, ok := events[0].(api.SessionEndEvent)
	if !ok || endEvt.SessionID != staleID {
		t.Fatalf("unexpected event: %+v", events[0])
	}

	// Verify state file was removed
	if _, err := os.Stat(trace.TurnStatePath(tempDir, staleID)); !os.IsNotExist(err) {
		t.Error("expected stale session state file to be removed")
	}
}
