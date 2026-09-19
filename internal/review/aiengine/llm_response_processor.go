// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package aiengine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RawModelFinding represents the JSON schema expected from LLM reviewer prompts.
type RawModelFinding struct {
	FilePath      string  `json:"file_path"`
	StartLine     int     `json:"start_line"`
	EndLine       int     `json:"end_line"`
	Severity      string  `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO"
	Category      string  `json:"category"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Remediation   string  `json:"remediation,omitempty"`
	SuggestedDiff string  `json:"suggested_diff,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
}

// ExtractionResult carries parsed findings and diagnostic telemetry.
type ExtractionResult struct {
	Findings       []models.CodeFinding `json:"findings"`
	RawPayload     string               `json:"raw_payload"`
	WasRepaired    bool                 `json:"was_repaired"`
	RepairStrategy string               `json:"repair_strategy,omitempty"`
	ParsedCount    int                  `json:"parsed_count"`
	Error          error                `json:"error,omitempty"`
}

// LLMResponseProcessor cleans, repairs, and extracts structured findings from raw LLM completions.
type LLMResponseProcessor struct {
	markdownFenceRegex *regexp.Regexp
	trailingCommaRegex *regexp.Regexp
	singleQuoteKeyReg  *regexp.Regexp
}

// NewLLMResponseProcessor constructs a response processor.
func NewLLMResponseProcessor() *LLMResponseProcessor {
	return &LLMResponseProcessor{
		markdownFenceRegex: regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)\\s*```"),
		trailingCommaRegex: regexp.MustCompile(`,(\s*[}\]])`),
		singleQuoteKeyReg:  regexp.MustCompile(`'([a-zA-Z0-9_]+)':`),
	}
}

// ProcessCompletion extracts structured review findings from raw LLM completions.
func (p *LLMResponseProcessor) ProcessCompletion(
	rawCompletion string,
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
) ExtractionResult {
	trimmed := strings.TrimSpace(rawCompletion)
	if trimmed == "" {
		return ExtractionResult{
			RawPayload: rawCompletion,
			Findings:   nil,
		}
	}

	result := ExtractionResult{
		RawPayload: rawCompletion,
	}

	// 1. Extract JSON content from markdown code fences if present
	jsonStr := p.extractJSONContent(trimmed)

	// 2. First attempt: Direct JSON unmarshaling
	var rawFindings []RawModelFinding
	err := json.Unmarshal([]byte(jsonStr), &rawFindings)
	if err == nil {
		result.Findings = p.convertRawFindings(rawFindings, reviewID, workspaceID)
		result.ParsedCount = len(result.Findings)
		return result
	}

	// 3. Second attempt: Check if payload is an object containing "findings" or "suggestions" key
	var wrapper map[string]json.RawMessage
	if wErr := json.Unmarshal([]byte(jsonStr), &wrapper); wErr == nil {
		for _, key := range []string{"findings", "suggestions", "issues", "recommendations"} {
			if data, exists := wrapper[key]; exists {
				if fErr := json.Unmarshal(data, &rawFindings); fErr == nil {
					result.Findings = p.convertRawFindings(rawFindings, reviewID, workspaceID)
					result.ParsedCount = len(result.Findings)
					return result
				}
			}
		}
	}

	// 4. Third attempt: Heuristic JSON repair (trailing commas, unbalanced brackets, quotes)
	repairedJSON, strategy := p.RepairMalformedJSON(jsonStr)
	result.WasRepaired = true
	result.RepairStrategy = strategy

	if rErr := json.Unmarshal([]byte(repairedJSON), &rawFindings); rErr == nil {
		result.Findings = p.convertRawFindings(rawFindings, reviewID, workspaceID)
		result.ParsedCount = len(result.Findings)
		return result
	}

	// Check if repaired wrapper parses
	if wErr := json.Unmarshal([]byte(repairedJSON), &wrapper); wErr == nil {
		for _, key := range []string{"findings", "suggestions", "issues"} {
			if data, exists := wrapper[key]; exists {
				if fErr := json.Unmarshal(data, &rawFindings); fErr == nil {
					result.Findings = p.convertRawFindings(rawFindings, reviewID, workspaceID)
					result.ParsedCount = len(result.Findings)
					return result
				}
			}
		}
	}

	result.Error = fmt.Errorf("failed to parse structured findings from LLM output: %w", err)
	return result
}

// RepairMalformedJSON repairs common LLM JSON syntax deviations.
func (p *LLMResponseProcessor) RepairMalformedJSON(input string) (string, string) {
	repaired := strings.TrimSpace(input)
	strategies := make([]string, 0)

	// A. Strip trailing commas before } or ]
	if p.trailingCommaRegex.MatchString(repaired) {
		repaired = p.trailingCommaRegex.ReplaceAllString(repaired, "$1")
		strategies = append(strategies, "strip_trailing_commas")
	}

	// B. Normalize single-quoted keys: 'key': -> "key":
	if p.singleQuoteKeyReg.MatchString(repaired) {
		repaired = p.singleQuoteKeyReg.ReplaceAllString(repaired, `"$1":`)
		strategies = append(strategies, "normalize_single_quotes")
	}

	// C. Repair truncated array or object (LLM hit max token limit)
	openBrackets := strings.Count(repaired, "[") - strings.Count(repaired, "]")
	openBraces := strings.Count(repaired, "{") - strings.Count(repaired, "}")

	if openBraces > 0 || openBrackets > 0 {
		// Truncate any trailing partial key or string
		lastComma := strings.LastIndex(repaired, ",")
		lastBrace := strings.LastIndex(repaired, "}")
		if lastComma > lastBrace && lastComma != -1 {
			repaired = repaired[:lastComma]
		}

		// Close unclosed braces then brackets
		for i := 0; i < openBraces; i++ {
			repaired += "}"
		}
		for i := 0; i < openBrackets; i++ {
			repaired += "]"
		}
		strategies = append(strategies, "close_unbalanced_brackets")
	}

	return repaired, strings.Join(strategies, "+")
}

func (p *LLMResponseProcessor) extractJSONContent(text string) string {
	matches := p.markdownFenceRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// If no code fence, locate first '[' or '{' and last ']' or '}'
	startIdx := strings.IndexAny(text, "[{")
	endIdx := strings.LastIndexAny(text, "]}")
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		return text[startIdx : endIdx+1]
	}

	return text
}

func (p *LLMResponseProcessor) convertRawFindings(
	raw []RawModelFinding,
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
) []models.CodeFinding {
	findings := make([]models.CodeFinding, 0, len(raw))

	for _, rf := range raw {
		if rf.FilePath == "" || rf.Title == "" {
			continue
		}

		startLine := rf.StartLine
		if startLine <= 0 {
			startLine = 1
		}
		endLine := rf.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		finding := models.CodeFinding{
			ID:            uuid.New(),
			ReviewID:      reviewID,
			WorkspaceID:   workspaceID,
			FilePath:      rf.FilePath,
			StartLine:     startLine,
			EndLine:       endLine,
			Severity:      normalizeSeverity(rf.Severity),
			Category:      normalizeCategory(rf.Category),
			Title:         rf.Title,
			Description:   rf.Description,
			Remediation:   rf.Remediation,
			SuggestedDiff: rf.SuggestedDiff,
			Fingerprint:   fmt.Sprintf("%s:%d:%s", rf.FilePath, startLine, rf.Title),
			CreatedAt:     time.Now().UTC(),
		}

		findings = append(findings, finding)
	}

	return findings
}

func normalizeSeverity(sev string) models.FindingSeverity {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL":
		return models.SeverityCritical
	case "HIGH":
		return models.SeverityHigh
	case "MEDIUM":
		return models.SeverityMedium
	case "LOW":
		return models.SeverityLow
	case "INFO":
		return models.SeverityInfo
	default:
		return models.SeverityMedium
	}
}

func normalizeCategory(cat string) string {
	clean := strings.ToUpper(strings.TrimSpace(cat))
	if clean == "" {
		return "QUALITY"
	}
	return clean
}
