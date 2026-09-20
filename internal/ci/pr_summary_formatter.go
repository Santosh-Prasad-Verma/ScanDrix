// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"bytes"
	"fmt"
)

// FormatPRSummaryMarkdown formats a review result into a GitHub Step Summary / GitLab Job Summary report.
func FormatPRSummaryMarkdown(res *HeadlessReviewResult) string {
	var sb bytes.Buffer

	// Title & Gate Badge
	if res.GatePassed {
		sb.WriteString("## 🛡️ ScanDrix Automated Code Review — ✅ PASSED\n\n")
	} else {
		sb.WriteString("## 🛡️ ScanDrix Automated Code Review — ❌ BLOCKED\n\n")
	}

	sb.WriteString(fmt.Sprintf("> **Gate Status:** %s  \n", res.GateReason))
	sb.WriteString(fmt.Sprintf("> **Execution Duration:** %s | **Reviewed In:** `%s`\n\n", res.Duration.Round(1e6), res.Environment.Platform))

	// Summary Metrics Table
	sb.WriteString("### Summary Metrics\n\n")
	sb.WriteString("| Severity | Count | Quality Gate Limit |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| 🔴 **Critical** | %d | 0 |\n", res.CriticalCount))
	sb.WriteString(fmt.Sprintf("| 🟠 **Error** | %d | 0 |\n", res.ErrorCount))
	sb.WriteString(fmt.Sprintf("| 🟡 **Warning** | %d | Allowed |\n", res.WarningCount))
	sb.WriteString(fmt.Sprintf("| ℹ️ **Info** | %d | Allowed |\n\n", res.InfoCount))

	if len(res.Findings) == 0 {
		sb.WriteString("✨ **No code issues or compliance violations found.** Great job!\n\n")
	} else {
		sb.WriteString("### Detailed Findings\n\n")

		for i, f := range res.Findings {
			badge := "ℹ️"
			switch f.Severity {
			case FailCritical:
				badge = "🔴 CRITICAL"
			case FailError:
				badge = "🟠 ERROR"
			case FailWarning:
				badge = "🟡 WARNING"
			case FailInfo:
				badge = "ℹ️ INFO"
			}

			sb.WriteString(fmt.Sprintf("<details>\n<summary><b>#%d [%s] %s</b> — <code>%s:%d</code></summary>\n\n",
				i+1, badge, f.RuleTitle, f.FilePath, f.StartLine))

			sb.WriteString(fmt.Sprintf("- **Category:** `%s`\n", f.Category))
			if f.RuleID != "" {
				sb.WriteString(fmt.Sprintf("- **Rule ID:** `%s`\n", f.RuleID))
			}
			if f.CWETaxonomy != "" {
				sb.WriteString(fmt.Sprintf("- **CWE Taxonomy:** `%s`\n", f.CWETaxonomy))
			}
			sb.WriteString(fmt.Sprintf("- **Location:** `%s` (lines %d-%d)\n\n", f.FilePath, f.StartLine, f.EndLine))

			sb.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", f.Message))

			if f.Suggestion != "" {
				sb.WriteString("**Suggested Fix:**\n```diff\n")
				sb.WriteString(f.Suggestion)
				sb.WriteString("\n```\n\n")
			}

			sb.WriteString("</details>\n\n")
		}
	}

	sb.WriteString("---\n")
	sb.WriteString("*Reviewed by ScanDrix AI Multi-Agent Deliberation (scandrix.dev)*\n")

	return sb.String()
}
