// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package format

import (
	"encoding/json"
	"strings"

	"github.com/scandrix/backend/internal/llm/structured"
)

// FormatEvaluationResult captures structured output conformance.
type FormatEvaluationResult struct {
	ValidJSON       bool
	RequiredRepair  bool
	CleanedOutput   string
	HasMarkdownWrap bool
	ParseError      string
}

// EvaluateStructuredFormat validates model response against structured JSON requirements.
func EvaluateStructuredFormat(rawResponse string) FormatEvaluationResult {
	hasFences := strings.Contains(rawResponse, "```")

	// Try direct parse
	var parsed any
	if err := json.Unmarshal([]byte(rawResponse), &parsed); err == nil {
		return FormatEvaluationResult{
			ValidJSON:       true,
			RequiredRepair:  false,
			CleanedOutput:   rawResponse,
			HasMarkdownWrap: hasFences,
		}
	}

	// Attempt extraction
	extracted := structured.ExtractJSONFromText(rawResponse)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &parsed); err == nil {
			return FormatEvaluationResult{
				ValidJSON:       true,
				RequiredRepair:  true,
				CleanedOutput:   extracted,
				HasMarkdownWrap: hasFences,
			}
		}

		// Attempt repair
		repaired := structured.RepairJSONText(extracted)
		if err := json.Unmarshal([]byte(repaired), &parsed); err == nil {
			return FormatEvaluationResult{
				ValidJSON:       true,
				RequiredRepair:  true,
				CleanedOutput:   repaired,
				HasMarkdownWrap: hasFences,
			}
		}
	}

	return FormatEvaluationResult{
		ValidJSON:       false,
		RequiredRepair:  true,
		CleanedOutput:   "",
		HasMarkdownWrap: hasFences,
		ParseError:      "failed to extract or repair valid JSON",
	}
}
