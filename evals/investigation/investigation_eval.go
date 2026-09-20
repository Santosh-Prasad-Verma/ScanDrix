// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package investigation

import (
	"os"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	DefaultMinToolCalls = 1
)

// ToolCallExpectation defines required tool usage criteria for an agent run.
type ToolCallExpectation struct {
	Tool            string `json:"tool,omitempty"`
	Path            string `json:"path,omitempty"`
	PathEndsWith    string `json:"path_ends_with,omitempty"`
	Pattern         string `json:"pattern,omitempty"`
	PatternIncludes string `json:"pattern_includes,omitempty"`
}

// InvestigationEvalResult summarizes the tool investigation pass.
type InvestigationEvalResult struct {
	TotalToolCalls    int      `json:"total_tool_calls"`
	ExpectedCalls     int      `json:"expected_calls"`
	MatchedCalls      int      `json:"matched_calls"`
	Recall            float64  `json:"recall"`
	MissingExps       []string `json:"missing_expectations,omitempty"`
	Passed            bool     `json:"passed"`
}

// EvaluateInvestigation asserts that an agent run properly investigated the target codebase
// by matching actual tool invocations against expected investigation patterns.
func EvaluateInvestigation(
	state *contracts.RunState,
	expectations []ToolCallExpectation,
	minToolCalls ...int,
) InvestigationEvalResult {
	minCalls := DefaultMinToolCalls
	if len(minToolCalls) > 0 && minToolCalls[0] >= 0 {
		minCalls = minToolCalls[0]
	} else if val := os.Getenv("SCANDRIX_EVAL_MIN_TOOL_CALLS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			minCalls = parsed
		}
	}

	actualCalls := make([]contracts.ToolCallRecord, 0)
	if state != nil {
		for _, step := range state.Steps {
			actualCalls = append(actualCalls, step.Message.ToolCalls...)
		}
	}

	if len(expectations) == 0 {
		passed := len(actualCalls) >= minCalls
		return InvestigationEvalResult{
			TotalToolCalls: len(actualCalls),
			Passed:         passed,
		}
	}

	matched := 0
	missing := make([]string, 0)

	for _, exp := range expectations {
		found := false
		for _, call := range actualCalls {
			if matchToolCall(call, exp) {
				found = true
				break
			}
		}
		if found {
			matched++
		} else {
			missing = append(missing, formatExp(exp))
		}
	}

	recall := float64(matched) / float64(len(expectations))
	passed := matched == len(expectations) && len(actualCalls) >= minCalls

	return InvestigationEvalResult{
		TotalToolCalls: len(actualCalls),
		ExpectedCalls:  len(expectations),
		MatchedCalls:   matched,
		Recall:         recall,
		MissingExps:    missing,
		Passed:         passed,
	}
}

func matchToolCall(call contracts.ToolCallRecord, exp ToolCallExpectation) bool {
	if exp.Tool != "" && !strings.EqualFold(call.Name, exp.Tool) {
		return false
	}

	inputMap, ok := call.Input.(map[string]any)
	if !ok {
		return exp.Path == "" && exp.Pattern == ""
	}

	pathVal, _ := inputMap["path"].(string)
	if pathVal == "" {
		pathVal, _ = inputMap["file"].(string)
	}
	normPath := normalizePath(pathVal)

	if exp.Path != "" && normPath != normalizePath(exp.Path) {
		return false
	}

	if exp.PathEndsWith != "" && !strings.HasSuffix(normPath, normalizePath(exp.PathEndsWith)) {
		return false
	}

	queryVal, _ := inputMap["query"].(string)
	if queryVal == "" {
		queryVal, _ = inputMap["pattern"].(string)
	}

	if exp.Pattern != "" && queryVal != exp.Pattern {
		return false
	}

	if exp.PatternIncludes != "" && !strings.Contains(queryVal, exp.PatternIncludes) {
		return false
	}

	return true
}

func normalizePath(p string) string {
	clean := strings.TrimPrefix(p, "/")
	clean = strings.ReplaceAll(clean, "\\", "/")
	return strings.TrimRight(clean, "/")
}

func formatExp(e ToolCallExpectation) string {
	parts := []string{}
	if e.Tool != "" {
		parts = append(parts, "tool="+e.Tool)
	}
	if e.Path != "" {
		parts = append(parts, "path="+e.Path)
	}
	if e.PathEndsWith != "" {
		parts = append(parts, "endsWith="+e.PathEndsWith)
	}
	if e.Pattern != "" {
		parts = append(parts, "pattern="+e.Pattern)
	}
	return strings.Join(parts, ", ")
}
