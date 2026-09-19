// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ui

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorBgRed   = "\033[41m"
	colorBgGreen = "\033[42m"
)

// SeverityColor returns the ANSI color escape for a given severity.
func SeverityColor(sev models.FindingSeverity) string {
	switch strings.ToUpper(string(sev)) {
	case "CRITICAL":
		return colorMagenta + colorBold
	case "HIGH":
		return colorRed + colorBold
	case "MEDIUM":
		return colorYellow
	case "LOW":
		return colorCyan
	default:
		return colorBlue
	}
}

// FormatFileChoice formats a file selection choice with severity indicators.
func FormatFileChoice(file string, findings []models.CodeFinding) string {
	stats := GetFileStats(findings)
	var parts []string

	if stats.Critical > 0 {
		parts = append(parts, fmt.Sprintf("%s%d crit%s", colorMagenta, stats.Critical, colorReset))
	}
	if stats.High > 0 {
		parts = append(parts, fmt.Sprintf("%s%d high%s", colorRed, stats.High, colorReset))
	}
	if stats.Medium > 0 {
		parts = append(parts, fmt.Sprintf("%s%d med%s", colorYellow, stats.Medium, colorReset))
	}
	if stats.Low > 0 {
		parts = append(parts, fmt.Sprintf("%s%d low%s", colorCyan, stats.Low, colorReset))
	}

	badge := ""
	if len(parts) > 0 {
		badge = fmt.Sprintf(" [%s]", strings.Join(parts, ", "))
	}

	return fmt.Sprintf("%s%s (%d findings)%s", file, badge, len(findings), colorReset)
}

// RenderFindingDetails prints formatted details of a single finding.
func RenderFindingDetails(f models.CodeFinding) {
	sevCol := SeverityColor(f.Severity)
	fmt.Printf("\n  %s%s%s: %s%s%s\n", sevCol, strings.ToUpper(string(f.Severity)), colorReset, colorBold, f.Title, colorReset)
	fmt.Printf("  %sLocation:%s %s:%d-%d\n", colorDim, colorReset, f.FilePath, f.StartLine, f.EndLine)
	if f.Category != "" {
		fmt.Printf("  %sCategory:%s %s\n", colorDim, colorReset, FormatCategoryBadge(f.Category))
	}
	fmt.Println()
	for _, line := range strings.Split(f.Description, "\n") {
		fmt.Printf("    %s\n", line)
	}

	if f.Remediation != "" {
		fmt.Println()
		fmt.Printf("  %s%sRemediation:%s\n", colorGreen, colorBold, colorReset)
		for _, line := range strings.Split(f.Remediation, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
	fmt.Println()
}

// RenderDiffPreview renders a colorized preview of a suggested diff.
func RenderDiffPreview(diff string) {
	fmt.Println()
	fmt.Println(colorCyan + "  ─── Suggested Patch Preview ───" + colorReset)
	lines := strings.Split(diff, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "+") {
			fmt.Printf("  %s%s%s\n", colorGreen, l, colorReset)
		} else if strings.HasPrefix(l, "-") {
			fmt.Printf("  %s%s%s\n", colorRed, l, colorReset)
		} else if strings.HasPrefix(l, "@@") {
			fmt.Printf("  %s%s%s\n", colorCyan, l, colorReset)
		} else {
			fmt.Printf("  %s%s%s\n", colorDim, l, colorReset)
		}
	}
	fmt.Println(colorCyan + "  ───────────────────────────────" + colorReset)
	fmt.Println()
}
