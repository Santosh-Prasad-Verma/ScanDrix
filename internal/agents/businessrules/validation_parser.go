// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	jsonFenceRegex = regexp.MustCompile(`(?s)^` + "```" + `(?:json)?\s*(.*?)\s*` + "```" + `$`)
)

// ParseValidationResult parses model output into a structured ValidationResult.
func ParseValidationResult(rawText string) ValidationResult {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return buildFallbackResult("empty output from validation analyzer")
	}

	stripped := stripCodeFence(trimmed)

	// Attempt JSON parse
	var target struct {
		NeedsMoreInfo         *bool    `json:"needs_more_info"`
		NeedsMoreInfoCamel    *bool    `json:"needsMoreInfo"`
		IsCompliant           *bool    `json:"is_compliant"`
		IsCompliantCamel      *bool    `json:"isCompliant"`
		Summary               string   `json:"summary"`
		Mode                  string   `json:"mode"`
		Reason                string   `json:"reason"`
		Confidence            string   `json:"confidence"`
		MissingInfo           string   `json:"missing_info"`
		MissingInfoCamel      string   `json:"missingInfo"`
		ViolatedRules         []string `json:"violated_rules"`
		ViolatedRulesCamel    []string `json:"violatedRules"`
		MissingRequirements   []string `json:"missing_requirements"`
		MissingReqsCamel      []string `json:"missingRequirements"`
		Suggestions           []string `json:"suggestions"`
		QualityClassification string   `json:"quality_classification"`
	}

	if err := json.Unmarshal([]byte(stripped), &target); err == nil {
		res := ValidationResult{
			Summary: target.Summary,
			Mode:    target.Mode,
			Reason:  target.Reason,
		}

		if target.NeedsMoreInfo != nil {
			res.NeedsMoreInfo = *target.NeedsMoreInfo
		} else if target.NeedsMoreInfoCamel != nil {
			res.NeedsMoreInfo = *target.NeedsMoreInfoCamel
		}

		if target.IsCompliant != nil {
			res.IsCompliant = *target.IsCompliant
		} else if target.IsCompliantCamel != nil {
			res.IsCompliant = *target.IsCompliantCamel
		} else {
			// If not specified, compliant when there are no missing requirements or violated rules
			res.IsCompliant = len(target.MissingRequirements) == 0 && len(target.ViolatedRules) == 0
		}

		if target.Confidence != "" {
			res.Confidence = target.Confidence
		} else {
			res.Confidence = "high"
		}

		if target.MissingInfo != "" {
			res.MissingInfo = target.MissingInfo
		} else {
			res.MissingInfo = target.MissingInfoCamel
		}

		if len(target.ViolatedRules) > 0 {
			res.ViolatedRules = target.ViolatedRules
		} else {
			res.ViolatedRules = target.ViolatedRulesCamel
		}

		if len(target.MissingRequirements) > 0 {
			res.MissingRequirements = target.MissingRequirements
		} else {
			res.MissingRequirements = target.MissingReqsCamel
		}

		res.Suggestions = target.Suggestions
		res.QualityClassification = target.QualityClassification

		if res.Summary == "" {
			if res.IsCompliant {
				res.Summary = "All business requirements and acceptance criteria have been successfully implemented."
			} else {
				res.Summary = "Business rules validation identified missing requirements or compliance gaps."
			}
		}

		return res
	}

	// If JSON parsing failed, check if output is a natural language limitation response
	lower := strings.ToLower(trimmed)
	if looksLikeLimitation(lower) {
		return ValidationResult{
			NeedsMoreInfo: true,
			IsCompliant:   false,
			Mode:          "limitation_response",
			Reason:        "task_context_missing",
			Confidence:    "medium",
			MissingInfo:   trimmed,
			Summary:       trimmed,
		}
	}

	return ValidationResult{
		NeedsMoreInfo: false,
		IsCompliant:   !strings.Contains(lower, "violation") && !strings.Contains(lower, "missing requirement"),
		Summary:       trimmed,
		Mode:          "full_analysis",
		Reason:        "analysis_ready",
		Confidence:    "medium",
	}
}

func stripCodeFence(val string) string {
	match := jsonFenceRegex.FindStringSubmatch(val)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return val
}

func looksLikeLimitation(lower string) bool {
	return strings.Contains(lower, "need task information") ||
		strings.Contains(lower, "insufficient task context") ||
		strings.Contains(lower, "missing validation context") ||
		strings.Contains(lower, "need pull request diff") ||
		strings.Contains(lower, "no task requirements provided")
}

func buildFallbackResult(reason string) ValidationResult {
	return ValidationResult{
		NeedsMoreInfo: true,
		IsCompliant:   false,
		Mode:          "limitation_response",
		Reason:        "parser_fallback",
		Confidence:    "low",
		MissingInfo:   reason,
		Summary:       "Unable to complete business rules validation due to incomplete task context or parser error.",
	}
}
