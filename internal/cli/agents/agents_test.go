// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package agents

import (
	"os"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

func TestClaudeCodeAdapter(t *testing.T) {
	adapter := NewClaudeCodeAdapter()
	if adapter.AgentType() != trace.AgentClaudeCode {
		t.Errorf("expected %s, got %s", trace.AgentClaudeCode, adapter.AgentType())
	}

	payload := []byte(`{
		"session_id": "claude_sess_123",
		"transcript_path": "/tmp/transcript.jsonl",
		"prompt": "Create login endpoint using sk-ant-secretkey12345"
	}`)

	evt, err := adapter.ParseHookEvent("user-prompt-submit", payload)
	if err != nil {
		t.Fatalf("ParseHookEvent failed: %v", err)
	}
	if evt == nil {
		t.Fatal("expected non-nil event")
	}
	if evt.Type != "TurnStart" {
		t.Errorf("expected TurnStart, got %s", evt.Type)
	}
	if evt.SessionID != "claude_sess_123" {
		t.Errorf("expected session_id claude_sess_123, got %s", evt.SessionID)
	}
	if evt.SessionRef != "/tmp/transcript.jsonl" {
		t.Errorf("expected session_ref /tmp/transcript.jsonl, got %s", evt.SessionRef)
	}
	// Prompt should be scrubbed
	if trace.ContainsSecret(evt.Prompt) {
		t.Errorf("expected prompt secret to be scrubbed, got %s", evt.Prompt)
	}
}

func TestCursorAdapter(t *testing.T) {
	adapter := NewCursorAdapter()
	if adapter.AgentType() != trace.AgentCursor {
		t.Errorf("expected %s, got %s", trace.AgentCursor, adapter.AgentType())
	}

	payload := []byte(`{
		"session_id": "cursor_sess_456",
		"prompt": "Fix database connection leak"
	}`)

	evt, err := adapter.ParseHookEvent("beforeSubmitPrompt", payload)
	if err != nil {
		t.Fatalf("ParseHookEvent failed: %v", err)
	}
	if evt == nil {
		t.Fatal("expected non-nil event")
	}
	if evt.Type != "TurnStart" {
		t.Errorf("expected TurnStart, got %s", evt.Type)
	}
	if evt.SessionID != "cursor_sess_456" {
		t.Errorf("expected session_id cursor_sess_456, got %s", evt.SessionID)
	}
}

func TestCodexAdapter(t *testing.T) {
	adapter := NewCodexAdapter()
	if adapter.AgentType() != trace.AgentCodex {
		t.Errorf("expected %s, got %s", trace.AgentCodex, adapter.AgentType())
	}

	payload := []byte(`{
		"thread_id": "thread_789"
	}`)

	evt, err := adapter.ParseHookEvent("AfterAgent", payload)
	if err != nil {
		t.Fatalf("ParseHookEvent failed: %v", err)
	}
	if evt == nil {
		t.Fatal("expected non-nil event")
	}
	if evt.Type != "TurnEnd" {
		t.Errorf("expected TurnEnd, got %s", evt.Type)
	}
	if evt.SessionID != "thread_789" {
		t.Errorf("expected session_id thread_789, got %s", evt.SessionID)
	}
}

func TestAgentTranscriptMethods(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := tempDir + "/test_transcript.jsonl"

	content := `{"type":"USER_INPUT","content":"Fix the database connection leak"}
{"type":"TOOL_USE","name":"view_file","input":{"path":"db/conn.go"}}
{"type":"TOOL_USE","name":"replace_file_content","input":{"TargetFile":"/repo/internal/db.go"}}
{"type":"PLANNER_RESPONSE","content":"I fixed the connection leak by adding defer close()"}
{"type":"MESSAGE","usage":{"input_tokens":120,"output_tokens":80}}
`
	if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test transcript: %v", err)
	}

	adapters := []AgentAdapter{
		NewClaudeCodeAdapter(),
		NewCursorAdapter(),
		NewCodexAdapter(),
	}

	for _, a := range adapters {
		t.Run(string(a.AgentType()), func(t *testing.T) {
			res, err := a.ReadTranscript(transcriptPath, 0)
			if err != nil {
				t.Fatalf("ReadTranscript failed: %v", err)
			}
			if res.EntryCount != 5 {
				t.Errorf("expected 5 entries, got %d", res.EntryCount)
			}

			prompts, err := a.ExtractPrompts(transcriptPath)
			if err != nil {
				t.Fatalf("ExtractPrompts failed: %v", err)
			}
			if len(prompts) != 1 || prompts[0] != "Fix the database connection leak" {
				t.Errorf("unexpected prompts: %v", prompts)
			}

			summary, err := a.ExtractSummary(transcriptPath)
			if err != nil {
				t.Fatalf("ExtractSummary failed: %v", err)
			}
			if summary == "" {
				t.Error("expected non-empty summary")
			}

			files, err := a.ExtractModifiedFiles(transcriptPath)
			if err != nil {
				t.Fatalf("ExtractModifiedFiles failed: %v", err)
			}
			if len(files) != 1 || files[0] != "/repo/internal/db.go" {
				t.Errorf("unexpected modified files: %v", files)
			}

			usage, err := a.CalculateTokenUsage(transcriptPath)
			if err != nil {
				t.Fatalf("CalculateTokenUsage failed: %v", err)
			}
			if usage.InputTokens != 120 || usage.OutputTokens != 80 {
				t.Errorf("unexpected usage: %+v", usage)
			}

			// WaitForTranscriptFlush on existing file
			if !a.WaitForTranscriptFlush(transcriptPath, 50*time.Millisecond) {
				t.Error("expected WaitForTranscriptFlush to return true for existing file")
			}
		})
	}
}
