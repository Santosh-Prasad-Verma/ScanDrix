// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package agents

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

// ClaudeCodeAdapter adapts Claude Code lifecycle hooks to ScanDrix LifecycleEvents.
type ClaudeCodeAdapter struct{}

func NewClaudeCodeAdapter() *ClaudeCodeAdapter {
	return &ClaudeCodeAdapter{}
}

func (a *ClaudeCodeAdapter) AgentType() trace.AgentType {
	return trace.AgentClaudeCode
}

var claudeHookMap = map[string]string{
	"session-start":      "SessionStart",
	"session-end":        "SessionEnd",
	"stop":               "TurnEnd",
	"user-prompt-submit": "TurnStart",
	"subagent-start":     "SubagentStart",
	"subagent-stop":      "SubagentEnd",
	"post-todo":          "TurnEnd",
	"pre-task":           "SubagentStart",
	"post-task":          "SubagentEnd",
}

func (a *ClaudeCodeAdapter) ParseHookEvent(hookName string, payload []byte) (*trace.LifecycleEvent, error) {
	normHook := strings.ToLower(strings.TrimSpace(hookName))
	eventType, ok := claudeHookMap[normHook]
	if !ok {
		return nil, nil // Ignore unrecognized hook
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

	for _, k := range []string{"tool_use_id", "toolUseId"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.ToolUseID = v
			break
		}
	}

	for _, k := range []string{"subagent_id", "subagentId"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SubagentID = v
			break
		}
	}

	for _, k := range []string{"subagent_type", "subagentType"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SubagentType = v
			break
		}
	}

	for _, k := range []string{"task_description", "taskDescription"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.TaskDescription = v
			break
		}
	}

	if inp, ok := data["tool_input"]; ok {
		evt.ToolInput = trace.RedactDeep(inp)
	} else if inp, ok := data["input"]; ok {
		evt.ToolInput = trace.RedactDeep(inp)
	}

	return evt, nil
}

func (a *ClaudeCodeAdapter) ReadTranscript(transcriptPath string, fromOffset int64) (*trace.TranscriptParseResult, error) {
	return trace.ParseTranscript(transcriptPath, fromOffset)
}

func (a *ClaudeCodeAdapter) ExtractPrompts(transcriptPath string) ([]string, error) {
	return trace.ExtractPrompts(transcriptPath)
}

func (a *ClaudeCodeAdapter) ExtractSummary(transcriptPath string) (string, error) {
	return trace.ExtractSummary(transcriptPath)
}

func (a *ClaudeCodeAdapter) ExtractModifiedFiles(transcriptPath string) ([]string, error) {
	return trace.ExtractModifiedFiles(transcriptPath)
}

func (a *ClaudeCodeAdapter) CalculateTokenUsage(transcriptPath string) (*trace.TokenUsage, error) {
	return trace.CalculateTokenUsage(transcriptPath)
}

func (a *ClaudeCodeAdapter) WaitForTranscriptFlush(transcriptPath string, timeout time.Duration) bool {
	return trace.WaitForTranscriptFlush(transcriptPath, timeout)
}
