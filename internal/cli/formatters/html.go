// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// HTMLFormatter produces a single-file, responsive, offline HTML review report.
type HTMLFormatter struct {
	Theme string // "dark" or "light"
}

// NewHTMLFormatter creates an HTML review report formatter.
func NewHTMLFormatter(theme string) *HTMLFormatter {
	if theme == "" {
		theme = "dark"
	}
	return &HTMLFormatter{Theme: theme}
}

// Format writes the complete standalone HTML document to w.
func (f *HTMLFormatter) Format(w io.Writer, result *types.ReviewResult) error {
	if result == nil {
		result = &types.ReviewResult{
			Issues:  []types.ReviewIssue{},
			Summary: "No findings recorded.",
		}
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	sb.WriteString(fmt.Sprintf("  <title>ScanDrix Code Review Report - %s</title>\n", html.EscapeString(result.ReviewID)))
	sb.WriteString(`  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --critical: #f85149;
      --error: #ea4a5a;
      --warning: #d29922;
      --info: #58a6ff;
      --success: #3fb950;
      --font-mono: ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, monospace;
    }
    body {
      background-color: var(--bg);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
      margin: 0;
      padding: 24px;
      line-height: 1.5;
    }
    .container { max-width: 1100px; margin: 0 auto; }
    .header { border-bottom: 1px solid var(--border); padding-bottom: 16px; margin-bottom: 24px; display: flex; justify-content: space-between; align-items: center; }
    .header h1 { margin: 0; font-size: 24px; color: #58a6ff; font-weight: 600; }
    .status-badge { padding: 4px 12px; border-radius: 12px; font-weight: 600; font-size: 13px; text-transform: uppercase; }
    .status-passed { background: rgba(63, 185, 80, 0.15); color: var(--success); border: 1px solid var(--success); }
    .status-failed { background: rgba(248, 81, 73, 0.15); color: var(--critical); border: 1px solid var(--critical); }
    .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 16px; margin-bottom: 24px; }
    .summary-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: 16px; text-align: center; }
    .summary-card .number { font-size: 28px; font-weight: 700; margin-bottom: 4px; }
    .summary-card .label { font-size: 12px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.5px; }
    .issue-card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 16px; overflow: hidden; }
    .issue-header { padding: 12px 16px; background: rgba(255,255,255,0.02); border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
    .issue-title { font-weight: 600; font-size: 15px; display: flex; align-items: center; gap: 8px; }
    .issue-body { padding: 16px; }
    .code-block { background: #010409; border: 1px solid var(--border); border-radius: 6px; padding: 12px; font-family: var(--font-mono); font-size: 13px; overflow-x: auto; white-space: pre; margin-top: 8px; }
    .badge { font-size: 11px; padding: 2px 8px; border-radius: 4px; font-weight: 600; text-transform: uppercase; }
    .badge-critical { background: var(--critical); color: #fff; }
    .badge-error { background: var(--error); color: #fff; }
    .badge-warning { background: var(--warning); color: #000; }
    .badge-info { background: var(--info); color: #fff; }
    .footer { margin-top: 40px; text-align: center; font-size: 12px; color: var(--text-muted); border-top: 1px solid var(--border); padding-top: 16px; }
  </style>
</head>
<body>
<div class="container">
`)

	statusClass := "status-passed"
	if result.ExitCode != 0 || result.Stats.CriticalCount > 0 || result.Stats.ErrorCount > 0 {
		statusClass = "status-failed"
	}

	sb.WriteString("  <div class=\"header\">\n")
	sb.WriteString("    <div>\n")
	sb.WriteString("      <h1>ScanDrix Automated Code Review</h1>\n")
	sb.WriteString(fmt.Sprintf("      <div style=\"color: var(--text-muted); font-size: 13px; margin-top: 4px;\">Review ID: %s | Duration: %v</div>\n",
		html.EscapeString(result.ReviewID), result.Duration))
	sb.WriteString("    </div>\n")
	sb.WriteString(fmt.Sprintf("    <div class=\"status-badge %s\">%s</div>\n", statusClass, html.EscapeString(result.Status)))
	sb.WriteString("  </div>\n\n")

	// Summary Cards
	sb.WriteString("  <div class=\"summary-grid\">\n")
	sb.WriteString(fmt.Sprintf("    <div class=\"summary-card\"><div class=\"number\">%d</div><div class=\"label\">Files Analyzed</div></div>\n", result.FilesAnalyzed))
	sb.WriteString(fmt.Sprintf("    <div class=\"summary-card\"><div class=\"number\" style=\"color: var(--critical);\">%d</div><div class=\"label\">Critical</div></div>\n", result.Stats.CriticalCount))
	sb.WriteString(fmt.Sprintf("    <div class=\"summary-card\"><div class=\"number\" style=\"color: var(--error);\">%d</div><div class=\"label\">Errors</div></div>\n", result.Stats.ErrorCount))
	sb.WriteString(fmt.Sprintf("    <div class=\"summary-card\"><div class=\"number\" style=\"color: var(--warning);\">%d</div><div class=\"label\">Warnings</div></div>\n", result.Stats.WarningCount))
	sb.WriteString(fmt.Sprintf("    <div class=\"summary-card\"><div class=\"number\" style=\"color: var(--success);\">%d</div><div class=\"label\">Fixable Issues</div></div>\n", result.Stats.FixableCount))
	sb.WriteString("  </div>\n\n")

	if result.Summary != "" {
		sb.WriteString(fmt.Sprintf("  <div class=\"summary-card\" style=\"text-align: left; margin-bottom: 24px;\"><strong>Executive Summary:</strong> %s</div>\n\n",
			html.EscapeString(result.Summary)))
	}

	// Issues List
	sb.WriteString("  <h2>Findings & Recommendations</h2>\n")
	if len(result.Issues) == 0 {
		sb.WriteString("  <p style=\"color: var(--success);\">✔ Zero defects, security vulnerabilities, or code quality smells identified.</p>\n")
	} else {
		for i, issue := range result.Issues {
			badgeClass := fmt.Sprintf("badge-%s", strings.ToLower(string(issue.Severity)))
			sb.WriteString("  <div class=\"issue-card\">\n")
			sb.WriteString("    <div class=\"issue-header\">\n")
			sb.WriteString(fmt.Sprintf("      <div class=\"issue-title\"><span class=\"badge %s\">%s</span> #%d: %s</div>\n",
				badgeClass, strings.ToUpper(string(issue.Severity)), i+1, html.EscapeString(issue.Message)))
			sb.WriteString(fmt.Sprintf("      <div style=\"font-family: var(--font-mono); font-size: 12px; color: var(--text-muted);\">%s:%d</div>\n",
				html.EscapeString(issue.File), issue.Line))
			sb.WriteString("    </div>\n")
			sb.WriteString("    <div class=\"issue-body\">\n")
			if issue.Recommendation != "" {
				sb.WriteString(fmt.Sprintf("      <p><strong>Rationale:</strong> %s</p>\n", html.EscapeString(issue.Recommendation)))
			}
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("      <p><strong>Remediation:</strong> %s</p>\n", html.EscapeString(issue.Suggestion)))
			}
			if issue.Fix != nil && issue.Fix.NewCode != "" {
				sb.WriteString("      <p><strong>Suggested Diff:</strong></p>\n")
				sb.WriteString(fmt.Sprintf("      <div class=\"code-block\">%s</div>\n", html.EscapeString(issue.Fix.NewCode)))
			}
			sb.WriteString("    </div>\n")
			sb.WriteString("  </div>\n")
		}
	}

	sb.WriteString("  <div class=\"footer\">\n")
	sb.WriteString("    Generated by ScanDrix Enterprise Code Review Platform.\n")
	sb.WriteString("  </div>\n")
	sb.WriteString("</div>\n</body>\n</html>\n")

	_, err := w.Write([]byte(sb.String()))
	return err
}
