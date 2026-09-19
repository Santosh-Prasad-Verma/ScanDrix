// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package agents

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

// CursorAdapter adapts Cursor IDE lifecycle hooks (.cursor/hooks.json) to ScanDrix LifecycleEvents.
type CursorAdapter struct{}

func NewCursorAdapter() *CursorAdapter {
	return &CursorAdapter{}
}

func (a *CursorAdapter) AgentType() trace.AgentType {
	return trace.AgentCursor
}

var cursorHookMap = map[string]string{
	"sessionstart":       "SessionStart",
	"sessionend":         "SessionEnd",
	"stop":               "TurnEnd",
	"beforesubmitprompt": "TurnStart",
	"subagentstart":      "SubagentStart",
	"subagentstop":       "SubagentEnd",
}

func (a *CursorAdapter) ParseHookEvent(hookName string, payload []byte) (*trace.LifecycleEvent, error) {
	normHook := strings.ToLower(strings.TrimSpace(hookName))
	eventType, ok := cursorHookMap[normHook]
	if !ok {
		return nil, nil
	}

	var data map[string]any
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &data)
	}
	if data == nil {
		data = make(map[string]any)
	}

	evt := &trace.LifecycleEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}

	for _, k := range []string{"session_id", "sessionId"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SessionID = v
			break
		}
	}

	for _, k := range []string{"transcript_path", "transcriptPath", "session_ref"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SessionRef = v
			break
		}
	}

	for _, k := range []string{"prompt", "user_message"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.Prompt = trace.Redact(v)
			break
		}
	}

	for _, k := range []string{"subagent_id", "subagentId"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SubagentID = v
			evt.ToolUseID = v
			break
		}
	}

	for _, k := range []string{"subagent_type", "subagentType"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SubagentType = v
			break
		}
	}

	for _, k := range []string{"task_description", "taskDescription", "task"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.TaskDescription = v
			break
		}
	}

	return evt, nil
}

func (a *CursorAdapter) ReadTranscript(transcriptPath string, fromOffset int64) (*trace.TranscriptParseResult, error) {
	return trace.ParseTranscript(transcriptPath, fromOffset)
}

func (a *CursorAdapter) ExtractPrompts(transcriptPath string) ([]string, error) {
	return trace.ExtractPrompts(transcriptPath)
}

func (a *CursorAdapter) ExtractSummary(transcriptPath string) (string, error) {
	return trace.ExtractSummary(transcriptPath)
}

func (a *CursorAdapter) ExtractModifiedFiles(transcriptPath string) ([]string, error) {
	return trace.ExtractModifiedFiles(transcriptPath)
}

func (a *CursorAdapter) CalculateTokenUsage(transcriptPath string) (*trace.TokenUsage, error) {
	return trace.CalculateTokenUsage(transcriptPath)
}

func (a *CursorAdapter) WaitForTranscriptFlush(transcriptPath string, timeout time.Duration) bool {
	return trace.WaitForTranscriptFlush(transcriptPath, timeout)
}
