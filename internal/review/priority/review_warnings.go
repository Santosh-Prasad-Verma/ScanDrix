// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority

import (
	"fmt"
	"strings"
)

// ReviewWarningKind categorizes structured pipeline fidelity downgrades.
type ReviewWarningKind string

const (
	WarningPromptCompacted       ReviewWarningKind = "PROMPT_COMPACTED"
	WarningCallGraphDropped      ReviewWarningKind = "CALLGRAPH_DROPPED"
	WarningHunkHeadersOnly       ReviewWarningKind = "HUNK_HEADERS_ONLY"
	WarningDiffTruncated         ReviewWarningKind = "DIFF_TRUNCATED"
	WarningLowSignalFilesDropped ReviewWarningKind = "LOW_SIGNAL_FILES_DROPPED"
	WarningHeavyPassesSkipped    ReviewWarningKind = "HEAVY_PASSES_SKIPPED"
	WarningProviderFallback      ReviewWarningKind = "PROVIDER_FALLBACK"
)

// ReviewWarningReason specifies the root trigger for fidelity adjustments.
type ReviewWarningReason string

const (
	ReasonSmallContextWindow ReviewWarningReason = "small_context_window"
	ReasonProviderFailover   ReviewWarningReason = "provider_failover"
)

// ReviewWarning details a specific fidelity reduction applied during a review pass.
type ReviewWarning struct {
	Kind                ReviewWarningKind   `json:"kind"`
	Reason              ReviewWarningReason `json:"reason"`
	ContextWindowTokens int                 `json:"context_window_tokens"`
	ModelName           string              `json:"model_name"`
	Detail              string              `json:"detail,omitempty"`
	AgentName           string              `json:"agent_name,omitempty"`
}

// BuildProviderFallbackWarning generates an event notice when an upstream LLM fails over to secondary model.
func BuildProviderFallbackWarning(failedModel, usedModel, agentName string) ReviewWarning {
	return ReviewWarning{
		Kind:                WarningProviderFallback,
		Reason:              ReasonProviderFailover,
		ContextWindowTokens: 0,
		ModelName:           usedModel,
		Detail:              fmt.Sprintf("primary provider %s failed; review ran on fallback %s", failedModel, usedModel),
		AgentName:           agentName,
	}
}

// DedupReviewWarnings aggregates duplicate notices across multi-agent executions.
func DedupReviewWarnings(warnings []ReviewWarning) []ReviewWarning {
	if len(warnings) == 0 {
		return nil
	}

	type dedupKey struct {
		Kind                ReviewWarningKind
		ModelName           string
		ContextWindowTokens int
	}

	orderedKeys := make([]dedupKey, 0)
	warningMap := make(map[dedupKey]ReviewWarning)
	detailsMap := make(map[dedupKey][]string)

	for _, w := range warnings {
		key := dedupKey{
			Kind:                w.Kind,
			ModelName:           w.ModelName,
			ContextWindowTokens: w.ContextWindowTokens,
		}

		existing, found := warningMap[key]
		if !found {
			orderedKeys = append(orderedKeys, key)
			warningMap[key] = w
			if w.Detail != "" {
				detailsMap[key] = []string{w.Detail}
			}
			continue
		}

		// When merging multiple agents, clear agent-specific attribution
		existing.AgentName = ""
		warningMap[key] = existing

		if w.Detail != "" {
			seen := false
			for _, d := range detailsMap[key] {
				if d == w.Detail {
					seen = true
					break
				}
			}
			if !seen {
				detailsMap[key] = append(detailsMap[key], w.Detail)
			}
		}
	}

	result := make([]ReviewWarning, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		w := warningMap[key]
		details := detailsMap[key]
		if len(details) > 0 {
			w.Detail = strings.Join(details, ", ")
		}
		result = append(result, w)
	}

	return result
}

// FormatWarningsMarkdown renders a collapsible markdown notice block for the PR review comment.
func FormatWarningsMarkdown(warnings []ReviewWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<details>\n<summary>⚠️ <b>Pipeline Fidelity Notices (")
	sb.WriteString(fmt.Sprintf("%d)", len(warnings)))
	sb.WriteString("</b></summary>\n\n")

	for _, w := range warnings {
		var title string
		switch w.Kind {
		case WarningPromptCompacted:
			title = "Prompt Compacted (Workflow Rules Trimmed)"
		case WarningCallGraphDropped:
			title = "Call Graph Omitted"
		case WarningHunkHeadersOnly:
			title = "Hunk Headers Only (Large File Mode)"
		case WarningDiffTruncated:
			title = "Diff Truncated to Per-File Budget"
		case WarningLowSignalFilesDropped:
			title = "Low-Signal Files Filtered"
		case WarningHeavyPassesSkipped:
			title = "Secondary Verifier Passes Skipped"
		case WarningProviderFallback:
			title = "Model Provider Failover"
		default:
			title = string(w.Kind)
		}

		sb.WriteString(fmt.Sprintf("- **%s** (`%s`", title, w.ModelName))
		if w.ContextWindowTokens > 0 {
			sb.WriteString(fmt.Sprintf(", %d tokens", w.ContextWindowTokens))
		}
		sb.WriteString(")")
		if w.Detail != "" {
			sb.WriteString(fmt.Sprintf(": %s", w.Detail))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n</details>\n")
	return sb.String()
}
