// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package agents

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/trace"
)

// CodexAdapter adapts Codex CLI notification events (~/.codex/config.toml) to ScanDrix LifecycleEvents.
type CodexAdapter struct{}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{}
}

func (a *CodexAdapter) AgentType() trace.AgentType {
	return trace.AgentCodex
}

func (a *CodexAdapter) ParseHookEvent(hookName string, payload []byte) (*trace.LifecycleEvent, error) {
	normHook := strings.ToLower(strings.TrimSpace(hookName))
	var eventType string
	switch normHook {
	case "afteragent", "stop":
		eventType = "TurnEnd"
	default:
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

	for _, k := range []string{"session_id", "sessionId", "thread_id", "threadId", "conversation_id"} {
		if v, ok := data[k].(string); ok && v != "" {
			evt.SessionID = v
			break
		}
	}

	return evt, nil
}

func (a *CodexAdapter) ReadTranscript(transcriptPath string, fromOffset int64) (*trace.TranscriptParseResult, error) {
	return trace.ParseTranscript(transcriptPath, fromOffset)
}

func (a *CodexAdapter) ExtractPrompts(transcriptPath string) ([]string, error) {
	return trace.ExtractPrompts(transcriptPath)
}

func (a *CodexAdapter) ExtractSummary(transcriptPath string) (string, error) {
	return trace.ExtractSummary(transcriptPath)
}

func (a *CodexAdapter) ExtractModifiedFiles(transcriptPath string) ([]string, error) {
	return trace.ExtractModifiedFiles(transcriptPath)
}

func (a *CodexAdapter) CalculateTokenUsage(transcriptPath string) (*trace.TokenUsage, error) {
	return trace.CalculateTokenUsage(transcriptPath)
}

func (a *CodexAdapter) WaitForTranscriptFlush(transcriptPath string, timeout time.Duration) bool {
	return trace.WaitForTranscriptFlush(transcriptPath, timeout)
}
