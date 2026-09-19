// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package agentcore

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

var grepLineRegex = regexp.MustCompile(`^([^:\s]+):\d+:`)

// ToolEvidenceSummary summarizes evidence gathered across tool invocations during an agent loop.
type ToolEvidenceSummary struct {
	StrongFiles    []string `json:"strong_files"` // Files directly read, type-checked, or AST-inspected
	WeakFiles      []string `json:"weak_files"`   // Files matched in grep searches
	TotalToolCalls int      `json:"total_tool_calls"`
}

// AgentAnomalySummary flags investigative and behavioral irregularities during an agent loop execution.
type AgentAnomalySummary struct {
	StepsLe2                 bool     `json:"steps_le_2"`
	ZeroToolCalls            bool     `json:"zero_tool_calls"`
	ZeroStrongEvidenceFiles  bool     `json:"zero_strong_evidence_files"`
	ZeroCoverage             bool     `json:"zero_coverage"`
	LowCoverage              bool     `json:"low_coverage"`
	LowStrongEvidenceFiles   bool     `json:"low_strong_evidence_files"`
	HallucinatedFindingFiles []string `json:"hallucinated_finding_files,omitempty"`
	IntegrityScore           float64  `json:"integrity_score"` // 0.0 (severely compromised) to 1.0 (fully verified)
}

func normalizeFilePath(path string) string {
	if path == "" {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	return strings.ToLower(strings.TrimSpace(cleaned))
}

// BuildToolEvidenceSummary analyzes tool call records to extract direct and indirect evidence files.
func BuildToolEvidenceSummary(toolCalls []contracts.ToolCallRecord) ToolEvidenceSummary {
	strongSet := make(map[string]struct{})
	weakSet := make(map[string]struct{})

	for _, call := range toolCalls {
		toolName := strings.ToLower(strings.TrimSpace(call.Name))

		// Strong evidence tools
		switch toolName {
		case "readfile", "read_file", "checktypes", "check_types", "astinspect", "ast_inspect":
			if inputMap, ok := call.Input.(map[string]any); ok {
				for _, k := range []string{"path", "filePath", "file", "target"} {
					if val, found := inputMap[k]; found {
						if str, ok := val.(string); ok && str != "" {
							norm := normalizeFilePath(str)
							if norm != "" {
								strongSet[norm] = struct{}{}
							}
						}
					}
				}
			}
		case "grep", "grep_search":
			// Extract filenames from grep output
			if call.Output != "" {
				lines := strings.Split(call.Output, "\n")
				for _, line := range lines {
					matches := grepLineRegex.FindStringSubmatch(line)
					if len(matches) > 1 {
						norm := normalizeFilePath(matches[1])
						if norm != "" {
							weakSet[norm] = struct{}{}
						}
					}
				}
			}
		}
	}

	strongList := make([]string, 0, len(strongSet))
	for f := range strongSet {
		strongList = append(strongList, f)
	}

	weakList := make([]string, 0, len(weakSet))
	for f := range weakSet {
		// Only include in weak if not already in strong
		if _, inStrong := strongSet[f]; !inStrong {
			weakList = append(weakList, f)
		}
	}

	return ToolEvidenceSummary{
		StrongFiles:    strongList,
		WeakFiles:      weakList,
		TotalToolCalls: len(toolCalls),
	}
}

// BuildAgentAnomalies evaluates execution steps, tool evidence, and diff coverage to identify audit risks.
func BuildAgentAnomalies(
	steps int,
	toolCalls []contracts.ToolCallRecord,
	coverage CoverageSummary,
	findings []FinderSuggestion,
) AgentAnomalySummary {
	evidence := BuildToolEvidenceSummary(toolCalls)

	touchedTargets := coverage.TouchedTargets
	totalTargets := coverage.TotalTargets

	coveragePct := 0.0
	if totalTargets > 0 {
		coveragePct = float64(touchedTargets) / float64(totalTargets)
	}

	// Check for findings in files that were never read or inspected (potential hallucination)
	strongMap := make(map[string]struct{}, len(evidence.StrongFiles))
	for _, f := range evidence.StrongFiles {
		strongMap[f] = struct{}{}
	}

	var hallucinated []string
	seenHallucinated := make(map[string]struct{})
	for _, f := range findings {
		norm := normalizeFilePath(f.RelevantFile)
		if norm == "" {
			continue
		}
		if _, read := strongMap[norm]; !read {
			if _, already := seenHallucinated[norm]; !already {
				seenHallucinated[norm] = struct{}{}
				hallucinated = append(hallucinated, f.RelevantFile)
			}
		}
	}

	anomalies := AgentAnomalySummary{
		StepsLe2:                 steps <= 2,
		ZeroToolCalls:            len(toolCalls) == 0,
		ZeroStrongEvidenceFiles:  len(evidence.StrongFiles) == 0,
		ZeroCoverage:             touchedTargets == 0 && totalTargets > 0,
		LowCoverage:              totalTargets > 0 && coveragePct < 0.70,
		LowStrongEvidenceFiles:   totalTargets >= 2 && len(evidence.StrongFiles) < 2,
		HallucinatedFindingFiles: hallucinated,
	}

	// Calculate integrity score
	score := 1.0
	if anomalies.ZeroToolCalls {
		score -= 0.50
	}
	if anomalies.StepsLe2 {
		score -= 0.20
	}
	if anomalies.ZeroCoverage {
		score -= 0.30
	} else if anomalies.LowCoverage {
		score -= 0.15
	}
	if len(hallucinated) > 0 {
		score -= float64(len(hallucinated)) * 0.10
	}

	if score < 0.0 {
		score = 0.0
	}
	anomalies.IntegrityScore = score

	return anomalies
}
