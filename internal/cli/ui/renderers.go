// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ui

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// RenderIssueDetailsLines renders a bordered box displaying finding attributes.
func RenderIssueDetailsLines(finding models.CodeFinding) []string {
	lines := []string{
		"",
		colorBold + "┌─ Issue Details ─────────────────────────────────────────" + colorReset,
		colorDim + "│" + colorReset,
		colorDim + "│ " + colorReset + colorBold + "File: " + colorReset + colorCyan + finding.FilePath + colorReset,
		colorDim + "│ " + colorReset + colorBold + "Line: " + colorReset + colorYellow + fmt.Sprintf("%d", finding.StartLine) + colorReset,
		colorDim + "│ " + colorReset + colorBold + "Severity: " + colorReset + SeverityColor(finding.Severity) + string(finding.Severity) + colorReset,
	}

	if finding.Category != "" {
		lines = append(lines, colorDim+"│ "+colorReset+colorBold+"Category: "+colorReset+colorMagenta+finding.Category+colorReset)
	}

	if finding.Title != "" {
		lines = append(lines, colorDim+"│ "+colorReset+colorBold+"Title: "+colorReset+colorDim+finding.Title+colorReset)
	}

	lines = append(lines,
		colorDim+"│"+colorReset,
		colorDim+"│ "+colorReset+colorBold+"Description:"+colorReset,
		colorDim+"│   "+colorReset+finding.Description,
	)

	if finding.Remediation != "" {
		lines = append(lines,
			colorDim+"│"+colorReset,
			colorDim+"│ "+colorReset+colorBold+"Remediation:"+colorReset,
			colorDim+"│   "+colorReset+colorGreen+finding.Remediation+colorReset,
		)
	}

	if finding.SuggestedDiff != "" {
		lines = append(lines,
			colorDim+"│"+colorReset,
			colorDim+"│ "+colorReset+colorBold+colorGreen+"✓ Suggested diff available"+colorReset,
		)
	}

	lines = append(lines,
		colorBold+"└─────────────────────────────────────────────────────────"+colorReset,
		"",
	)

	return lines
}

// RenderFixPreviewLines renders diff lines for an issue remediation proposal.
func RenderFixPreviewLines(finding models.CodeFinding) []string {
	if finding.SuggestedDiff == "" {
		return []string{colorYellow + "No suggested diff available for this issue" + colorReset}
	}

	lines := []string{
		"",
		colorBold + "┌─ Fix Preview ───────────────────────────────────────────" + colorReset,
		colorDim + "│" + colorReset,
	}

	for _, line := range strings.Split(finding.SuggestedDiff, "\n") {
		if strings.HasPrefix(line, "+") {
			lines = append(lines, colorDim+"│ "+colorReset+colorGreen+line+colorReset)
		} else if strings.HasPrefix(line, "-") {
			lines = append(lines, colorDim+"│ "+colorReset+colorRed+line+colorReset)
		} else {
			lines = append(lines, colorDim+"│ "+colorReset+colorDim+line+colorReset)
		}
	}

	lines = append(lines,
		colorBold+"└─────────────────────────────────────────────────────────"+colorReset,
		"",
	)

	return lines
}

// RenderFileHeaderLines formats a section divider for a file and its issue count.
func RenderFileHeaderLines(file string, issueCount int) []string {
	suffix := " issue"
	if issueCount != 1 {
		suffix = " issues"
	}
	return []string{
		"",
		colorBold + colorCyan + fmt.Sprintf("┌─ %s ────────────────────────────────────────────────", file) + colorReset,
		colorDim + fmt.Sprintf("│ %d%s in this file", issueCount, suffix) + colorReset,
		colorBold + colorCyan + "└────────────────────────────────────────────────────────────" + colorReset,
	}
}

// RenderReviewSummaryLines produces a summary of total, fixed, and remaining findings.
func RenderReviewSummaryLines(totalIssues, fixedCount int) []string {
	remaining := totalIssues - fixedCount
	if remaining < 0 {
		remaining = 0
	}
	return []string{
		"",
		colorBold + colorCyan + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + colorReset,
		colorBold + "Review Summary" + colorReset,
		colorBold + colorCyan + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + colorReset,
		"",
		colorDim + "Total issues: " + colorReset + colorWhite + fmt.Sprintf("%d", totalIssues) + colorReset,
		colorDim + "Fixed: " + colorReset + colorGreen + fmt.Sprintf("%d", fixedCount) + colorReset,
		colorDim + "Remaining: " + colorReset + colorYellow + fmt.Sprintf("%d", remaining) + colorReset,
		"",
	}
}
