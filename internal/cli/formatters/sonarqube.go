// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// SonarQubeReport matches the SonarQube Generic Issue Import format specification.
type SonarQubeReport struct {
	Issues []SonarQubeIssue `json:"issues"`
}

type SonarQubeIssue struct {
	EngineID        string            `json:"engineId"`
	RuleID          string            `json:"ruleId"`
	Severity        string            `json:"severity"` // BLOCKER, CRITICAL, MAJOR, MINOR, INFO
	Type            string            `json:"type"`     // BUG, VULNERABILITY, CODE_SMELL
	PrimaryLocation SonarQubeLocation `json:"primaryLocation"`
}

type SonarQubeLocation struct {
	Message   string             `json:"message"`
	FilePath  string             `json:"filePath"`
	TextRange SonarQubeTextRange `json:"textRange"`
}

type SonarQubeTextRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

// FormatSonarQube serializes review issues into SonarQube generic issue import JSON.
func FormatSonarQube(w io.Writer, result *types.ReviewResult) error {
	if result == nil {
		result = &types.ReviewResult{}
	}

	var issues []SonarQubeIssue
	for _, issue := range result.Issues {
		ruleID := issue.ID
		if ruleID == "" {
			ruleID = "scandrix-" + issue.Category
		}

		endLine := issue.EndLine
		if endLine < issue.Line {
			endLine = issue.Line
		}

		issues = append(issues, SonarQubeIssue{
			EngineID: "scandrix",
			RuleID:   ruleID,
			Severity: mapSonarSeverity(string(issue.Severity)),
			Type:     mapSonarType(issue.Category),
			PrimaryLocation: SonarQubeLocation{
				Message:  issue.Message,
				FilePath: issue.File,
				TextRange: SonarQubeTextRange{
					StartLine: issue.Line,
					EndLine:   endLine,
				},
			},
		})
	}

	report := SonarQubeReport{Issues: issues}
	if report.Issues == nil {
		report.Issues = []SonarQubeIssue{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func mapSonarSeverity(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return "BLOCKER"
	case "error", "high":
		return "CRITICAL"
	case "warning", "medium":
		return "MAJOR"
	case "low":
		return "MINOR"
	default:
		return "INFO"
	}
}

func mapSonarType(category string) string {
	cat := strings.ToLower(category)
	if strings.Contains(cat, "security") || strings.Contains(cat, "vuln") {
		return "VULNERABILITY"
	}
	if strings.Contains(cat, "bug") || strings.Contains(cat, "correctness") || strings.Contains(cat, "logic") {
		return "BUG"
	}
	return "CODE_SMELL"
}
