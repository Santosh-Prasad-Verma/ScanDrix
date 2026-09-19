package review

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// ScanDrixHunkFindings models the sidecar findings schema consumed by hunk extensions.
type ScanDrixHunkFindings struct {
	Version  int                   `json:"version"`
	Summary  string                `json:"summary,omitempty"`
	Findings []ScanDrixHunkFinding `json:"findings"`
}

// ScanDrixHunkFinding represents a single structured finding entry.
type ScanDrixHunkFinding struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	EndLine  int    `json:"endLine"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`
	RuleID   string `json:"ruleId,omitempty"`
}

const titleMax = 200

// ConvertReviewToHunkFindings transforms a ReviewResult into structured hunk sidecar findings.
func ConvertReviewToHunkFindings(result *types.ReviewResult) ScanDrixHunkFindings {
	var findings []ScanDrixHunkFinding
	if result == nil {
		return ScanDrixHunkFindings{Version: 1}
	}

	for i, issue := range result.Issues {
		if issue.File == "" {
			continue
		}

		line := issue.Line
		if line <= 0 {
			continue
		}
		endLine := issue.EndLine
		if endLine < line {
			endLine = line
		}

		title := issue.Message
		if title == "" {
			title = issue.Suggestion
		}
		if title == "" {
			title = "ScanDrix finding"
		}
		if len(title) > titleMax {
			title = title[:titleMax-3] + "..."
		}

		sev := strings.ToLower(string(issue.Severity))
		if sev == "" {
			sev = "info"
		}

		finding := ScanDrixHunkFinding{
			ID:       fmt.Sprintf("scandrix-%d", i),
			File:     issue.File,
			Line:     line,
			EndLine:  endLine,
			Severity: sev,
			Title:    title,
			Category: issue.Category,
			RuleID:   issue.RuleID,
		}
		findings = append(findings, finding)
	}

	return ScanDrixHunkFindings{
		Version:  1,
		Summary:  strings.TrimSpace(result.Summary),
		Findings: findings,
	}
}
