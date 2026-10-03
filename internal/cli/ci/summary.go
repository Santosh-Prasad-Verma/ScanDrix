// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ci

import (
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// GenerateMarkdownSummary constructs an executive GitHub/GitLab step summary markdown.
func GenerateMarkdownSummary(ctx *CIContext, findings []models.CodeFinding, passGate bool) string {
	var sb strings.Builder

	sb.WriteString("## 🛡️ ScanDrix AI Code Review & Security Summary\n\n")

	// Status badge banner
	if passGate {
		sb.WriteString("> **Status**: :white_check_mark: **PASSED** — All security and quality gates satisfied.\n\n")
	} else {
		sb.WriteString("> **Status**: :x: **FAILED** — Blocking vulnerabilities or rule violations detected.\n\n")
	}

	// Environment Details
	if ctx != nil && ctx.IsCI {
		sb.WriteString(fmt.Sprintf("**Repository**: `%s` | **Branch**: `%s`", ctx.RepoSlug, ctx.Branch))
		if ctx.IsPR && ctx.PRNumber > 0 {
			sb.WriteString(fmt.Sprintf(" | **PR**: #%d", ctx.PRNumber))
		}
		if ctx.CommitSHA != "" {
			shortSHA := ctx.CommitSHA
			if len(shortSHA) > 8 {
				shortSHA = shortSHA[:8]
			}
			sb.WriteString(fmt.Sprintf(" | **Commit**: `%s`", shortSHA))
		}
		sb.WriteString("\n\n")
	}

	// Count metrics
	critical := 0
	high := 0
	medium := 0
	low := 0
	for _, f := range findings {
		switch strings.ToLower(string(f.Severity)) {
		case "critical":
			critical++
		case "high", "error":
			high++
		case "medium", "warning":
			medium++
		case "low", "info":
			low++
		}
	}

	// Metrics Table
	sb.WriteString("| Total Issues | :red_circle: Critical | :orange_circle: High | :yellow_circle: Medium | :white_circle: Low |\n")
	sb.WriteString("| :---: | :---: | :---: | :---: | :---: |\n")
	sb.WriteString(fmt.Sprintf("| **%d** | **%d** | **%d** | **%d** | **%d** |\n\n", len(findings), critical, high, medium, low))

	if len(findings) == 0 {
		sb.WriteString("### :tada: Clean Review\n")
		sb.WriteString("No security vulnerabilities, bugs, or code quality issues were identified in this changeset.\n\n")
		return sb.String()
	}

	// Findings Table
	sb.WriteString("### 🔍 Issues Found\n\n")
	sb.WriteString("| Severity | File | Line | Rule / Category | Issue |\n")
	sb.WriteString("| :--- | :--- | :---: | :--- | :--- |\n")

	for _, f := range findings {
		sevBadge := formatSeverityBadge(string(f.Severity))
		ruleName := f.Category
		if ruleName == "" {
			ruleName = "general"
		}
		sb.WriteString(fmt.Sprintf("| %s | `%s` | %d | %s | **%s** |\n",
			sevBadge, f.FilePath, f.StartLine, ruleName, f.Title))
	}
	sb.WriteString("\n")

	// Collapsible details for each finding
	sb.WriteString("### 📝 Detailed Findings & Recommendations\n\n")
	for idx, f := range findings {
		sb.WriteString(fmt.Sprintf("<details>\n<summary><b>%d. [%s] %s (%s:%d)</b></summary>\n\n",
			idx+1, strings.ToUpper(string(f.Severity)), f.Title, f.FilePath, f.StartLine))

		if f.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description**:\n%s\n\n", f.Description))
		}

		if f.SuggestedDiff != "" {
			sb.WriteString("**Suggested Fix**:\n```diff\n" + f.SuggestedDiff + "\n```\n\n")
		}

		sb.WriteString("</details>\n\n")
	}

	sb.WriteString("---\n*Generated automatically by [ScanDrix AI](https://scandrix.dev)*\n")
	return sb.String()
}

// WriteStepSummaryFile writes the summary markdown to $GITHUB_STEP_SUMMARY if available.
func WriteStepSummaryFile(summary string) error {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return nil
	}

	// GITHUB_STEP_SUMMARY is set by the Actions runner, but it is still an
	// environment variable, and this opens the path for append. Refusing anything
	// that is not a regular file keeps a symlink or a device node from being
	// written to through it.
	if info, statErr := os.Lstat(summaryPath); statErr == nil && !info.Mode().IsRegular() { // #nosec G703 -- summaryPath is checked with Lstat for a regular file immediately above
		return fmt.Errorf("refusing to append to %q: not a regular file", summaryPath)
	}

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G703 -- summaryPath is checked with Lstat for a regular file immediately above
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(summary + "\n")
	return err
}

func formatSeverityBadge(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return ":red_circle: Critical"
	case "high", "error":
		return ":orange_circle: High"
	case "medium", "warning":
		return ":yellow_circle: Medium"
	default:
		return ":white_circle: Low"
	}
}
