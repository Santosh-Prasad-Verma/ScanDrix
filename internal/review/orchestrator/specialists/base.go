// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/pkg/models"
)

// LLMClient represents the interface needed by specialists to invoke the model.
type LLMClient interface {
	GenerateResponse(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// RawFinding represents the wire format emitted by specialist LLMs.
type RawFinding struct {
	FilePath           string `json:"file_path"`
	StartLine          int    `json:"start_line"`
	EndLine            int    `json:"end_line"`
	Severity           string `json:"severity"`
	Confidence         string `json:"confidence"`
	Category           string `json:"category"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Remediation        string `json:"remediation"`
	SuggestedDiff      string `json:"suggested_diff,omitempty"`
	ExistingCode       string `json:"existing_code,omitempty"`
	ImprovedCode       string `json:"improved_code,omitempty"`
	OneSentenceSummary string `json:"one_sentence_summary,omitempty"`
	Blocking           bool   `json:"blocking"`
}

// RawSpecialistResponse models the expected JSON array of findings.
type RawSpecialistResponse struct {
	Findings []RawFinding `json:"findings"`
}

// ParseSpecialistResponse unmarshals and validates model output into typed findings.
func ParseSpecialistResponse(raw, agentName, defaultCategory string) ([]orchestrator.AgentFinding, error) {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(clean, "```json") {
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	} else if strings.HasPrefix(clean, "```") {
		clean = strings.TrimPrefix(clean, "```")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	}

	var resp RawSpecialistResponse
	if err := json.Unmarshal([]byte(clean), &resp); err != nil {
		// Fallback: try parsing as raw slice of findings
		var slice []RawFinding
		if errSlice := json.Unmarshal([]byte(clean), &slice); errSlice == nil {
			resp.Findings = slice
		} else {
			return nil, fmt.Errorf("failed to parse specialist findings: %w (raw: %s)", err, clean)
		}
	}

	var findings []orchestrator.AgentFinding
	for _, rf := range resp.Findings {
		if strings.TrimSpace(rf.FilePath) == "" || strings.TrimSpace(rf.Title) == "" {
			continue
		}

		cat := rf.Category
		if cat == "" {
			cat = defaultCategory
		}

		sev := normalizeSeverity(rf.Severity)
		blocking := rf.Blocking
		if sev == models.SeverityCritical || sev == models.SeverityHigh {
			blocking = true
		}

		conf := strings.ToUpper(strings.TrimSpace(rf.Confidence))
		if conf != "HIGH" && conf != "MEDIUM" && conf != "LOW" {
			conf = "HIGH"
		}

		startLine := rf.StartLine
		if startLine <= 0 {
			startLine = 1
		}
		endLine := rf.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		findings = append(findings, orchestrator.AgentFinding{
			ID:                 uuid.New(),
			AgentName:          agentName,
			FilePath:           rf.FilePath,
			StartLine:          startLine,
			EndLine:            endLine,
			Severity:           sev,
			Confidence:         conf,
			Category:           cat,
			Title:              rf.Title,
			Description:        rf.Description,
			Remediation:        rf.Remediation,
			SuggestedDiff:      rf.SuggestedDiff,
			ExistingCode:       rf.ExistingCode,
			ImprovedCode:       rf.ImprovedCode,
			OneSentenceSummary: rf.OneSentenceSummary,
			Blocking:           blocking,
			Fingerprint:        generateFingerprint(rf.FilePath, startLine, rf.Title),
			ContributingAgents: []string{agentName},
		})
	}

	return findings, nil
}

func normalizeSeverity(raw string) models.FindingSeverity {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CRITICAL":
		return models.SeverityCritical
	case "HIGH":
		return models.SeverityHigh
	case "MEDIUM":
		return models.SeverityMedium
	case "LOW":
		return models.SeverityLow
	default:
		return models.SeverityInfo
	}
}

func generateFingerprint(file string, line int, title string) string {
	raw := fmt.Sprintf("%s:%d:%s", file, line, title)
	return fmt.Sprintf("%x", raw)
}

// BuildSpecialistUserPrompt constructs the diff text and PR metadata for an agent.
func BuildSpecialistUserPrompt(input orchestrator.ReviewAgentInput, focusArea string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pull Request #%d: %s\n", input.PRNumber, input.Title))
	if input.Description != "" {
		sb.WriteString(fmt.Sprintf("Description:\n%s\n\n", input.Description))
	}

	if input.ExternalContext != "" {
		sb.WriteString(fmt.Sprintf("External Architectural Context:\n%s\n\n", input.ExternalContext))
	}

	sb.WriteString(fmt.Sprintf("### Focus Area: %s\n", focusArea))
	sb.WriteString("Evaluate the following changed files. Provide line-level findings matching the specified schema.\n\n")

	for _, f := range input.ChangedFiles {
		if f.IsBinary || f.IsVendored {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- File: %s (+%d, -%d) ---\n", f.Filename, f.Additions, f.Deletions))
		sb.WriteString("```\n")
		if f.Patch != "" {
			sb.WriteString(f.Patch)
		} else {
			sb.WriteString(f.Content)
		}
		sb.WriteString("\n```\n\n")
	}

	return sb.String()
}
