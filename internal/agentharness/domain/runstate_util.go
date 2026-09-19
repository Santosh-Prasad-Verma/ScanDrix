// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package domain

import (
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// FinalText extracts the free-form text answer of a completed run.
// It scans steps in reverse to find the last assistant turn carrying non-empty text.
// This is the output mode for agents without a resultToolName (chat, business-rules,
// single-shot analysis). Returns empty string if no step produced text.
func FinalText(state *contracts.RunState) string {
	if state == nil {
		return ""
	}

	for i := len(state.Steps) - 1; i >= 0; i-- {
		step := state.Steps[i]
		if step.Message.Role == contracts.RoleAssistant {
			if strContent, ok := step.Message.Content.(string); ok {
				trimmed := strings.TrimSpace(strContent)
				if len(trimmed) > 0 {
					return trimmed
				}
			}
		}
	}

	return ""
}

// ToolCallsFromState extracts all tool calls executed during the run in chronological order.
func ToolCallsFromState(state *contracts.RunState) []contracts.ToolCallRecord {
	if state == nil {
		return nil
	}

	var records []contracts.ToolCallRecord
	for _, step := range state.Steps {
		records = append(records, step.Message.ToolCalls...)
	}
	return records
}

// ArtifactsByType returns all materialized artifacts matching the specified type.
func ArtifactsByType(state *contracts.RunState, artifactType string) []contracts.Artifact {
	if state == nil {
		return nil
	}

	var matched []contracts.Artifact
	for _, a := range state.Artifacts {
		if a.Type == artifactType {
			matched = append(matched, a)
		}
	}
	return matched
}

// LastArtifact returns the final materialized artifact of the specified type,
// adhering to the "result tool" convention where the last call represents the final output.
func LastArtifact(state *contracts.RunState, artifactType string) (contracts.Artifact, bool) {
	if state == nil {
		return contracts.Artifact{}, false
	}

	for i := len(state.Artifacts) - 1; i >= 0; i-- {
		if state.Artifacts[i].Type == artifactType {
			return state.Artifacts[i], true
		}
	}
	return contracts.Artifact{}, false
}
