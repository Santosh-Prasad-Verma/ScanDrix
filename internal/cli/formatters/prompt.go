// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// PROMPT FORMATTER (AI Agent Structured Output)

// PromptFormatOptions represents the input data needed for the prompt-only formatter.
type PromptFormatOptions struct {
	FilesAnalyzed int
	DurationMs    int64
	Summary       string
	Findings      []models.CodeFinding
}

// RenderPrompt outputs a minimal, structured text format optimized for AI agents.
func RenderPrompt(w io.Writer, opts PromptFormatOptions) error {
	fmt.Fprintln(w, "REVIEW_ANALYSIS_COMPLETE")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "FILES_ANALYZED: %d\n", opts.FilesAnalyzed)
	fmt.Fprintf(w, "ISSUES_FOUND: %d\n", len(opts.Findings))
	fmt.Fprintf(w, "DURATION_MS: %d\n", opts.DurationMs)
	fmt.Fprintln(w)

	critical, errors, warnings, info := 0, 0, 0, 0
	for _, f := range opts.Findings {
		switch f.Severity {
		case models.SeverityCritical:
			critical++
		case models.SeverityHigh:
			errors++
		case models.SeverityMedium:
			warnings++
		case models.SeverityLow:
			info++
		}
	}

	fmt.Fprintln(w, "SEVERITY_BREAKDOWN:")
	fmt.Fprintf(w, "  CRITICAL: %d\n", critical)
	fmt.Fprintf(w, "  ERROR: %d\n", errors)
	fmt.Fprintf(w, "  WARNING: %d\n", warnings)
	fmt.Fprintf(w, "  INFO: %d\n", info)
	fmt.Fprintln(w)

	if len(opts.Findings) > 0 {
		fmt.Fprintln(w, "ISSUES:")
		fmt.Fprintln(w)
		for i, f := range opts.Findings {
			fmt.Fprintf(w, "ISSUE_%d:\n", i+1)
			fmt.Fprintf(w, "  file: %s\n", f.FilePath)
			fmt.Fprintf(w, "  line: %d\n", f.StartLine)
			if f.EndLine > 0 && f.EndLine != f.StartLine {
				fmt.Fprintf(w, "  end_line: %d\n", f.EndLine)
			}
			fmt.Fprintf(w, "  severity: %s\n", strings.ToLower(string(f.Severity)))
			if f.Category != "" {
				fmt.Fprintf(w, "  category: %s\n", f.Category)
			}
			fmt.Fprintf(w, "  message: %s\n", f.Title)
			if f.Description != "" {
				fmt.Fprintf(w, "  description: %s\n", f.Description)
			}
			if f.Remediation != "" {
				fmt.Fprintf(w, "  remediation: %s\n", f.Remediation)
			}
			if f.SuggestedDiff != "" {
				fmt.Fprintln(w, "  suggested_diff: |")
				diffLines := strings.Split(f.SuggestedDiff, "\n")
				for _, dl := range diffLines {
					fmt.Fprintf(w, "    %s\n", dl)
				}
			}
			fmt.Fprintln(w)
		}
	} else {
		fmt.Fprintln(w, "NO_ISSUES_FOUND")
		fmt.Fprintln(w)
	}

	if opts.Summary != "" {
		fmt.Fprintln(w, "SUMMARY:")
		fmt.Fprintln(w, opts.Summary)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "END_REVIEW")
	return nil
}
