// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// RunStateErrorText extracts the aggregated error messages from a failed RunState trace.
func RunStateErrorText(state contracts.RunState) string {
	if state.Status != contracts.StatusError {
		return ""
	}

	var sb strings.Builder
	for _, event := range state.Trace {
		if event.Kind == "error" && event.Detail != nil {
			if msg, ok := event.Detail["message"].(string); ok && msg != "" {
				sb.WriteString(msg)
				sb.WriteString(" ")
			}
			if resp, ok := event.Detail["responseBody"].(string); ok && resp != "" {
				sb.WriteString(resp)
				sb.WriteString(" ")
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// IsContextOverflowResult returns true if the run failed due to exceeding the model's context window.
// This is the signal for callers (e.g. OverflowRecoveringRunner) to re-run with tighter compression.
func IsContextOverflowResult(state contracts.RunState) bool {
	text := RunStateErrorText(state)
	if text == "" {
		return false
	}
	return IsContextOverflowError(errors.New(text))
}

// FormatContextOverflowDetail formats an informational message for logs and telemetry.
func FormatContextOverflowDetail(state contracts.RunState) string {
	errText := RunStateErrorText(state)
	if errText == "" {
		return "context window overflow detected"
	}
	return fmt.Sprintf("context window overflow detected: %s", errText)
}
