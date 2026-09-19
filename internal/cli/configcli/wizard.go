// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package configcli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/utils"
)

// RepositoryReviewSettings encapsulates repo-specific review configuration for the setup wizard.
type RepositoryReviewSettings struct {
	RepositoryID              string         `json:"repository_id"`
	Namespace                 string         `json:"namespace"`
	DefaultBranch             string         `json:"default_branch"`
	ReviewEnabled             bool           `json:"review_enabled"`
	AutoApproveEnabled        bool           `json:"auto_approve_enabled"`
	RequestChangesMinSeverity string         `json:"request_changes_min_severity,omitempty"`
	IgnoredPaths              []string       `json:"ignored_paths"`
	IgnoredFilePatterns       []string       `json:"ignored_file_patterns,omitempty"`
	BaseBranchPatterns        []string       `json:"base_branch_patterns,omitempty"`
	IgnoredTitlePatterns      []string       `json:"ignored_title_patterns,omitempty"`
	FocusAreas                []string       `json:"focus_areas,omitempty"`
	Reviewers                 []string       `json:"reviewers,omitempty"`
	CustomSettings            map[string]any `json:"custom_settings,omitempty"`
}

// Recommended and common patterns for code review setup.
var (
	RecommendedIgnoredFilePatterns = []string{
		"yarn.lock",
		"package-lock.json",
		"package.json",
		".env",
		"**/*.json",
	}

	CommonIgnoredFilePatterns = []string{
		"yarn.lock",
		"package-lock.json",
		"package.json",
		".env",
		"**/*.json",
		"dist/**",
		"coverage/**",
		"**/generated/**",
	}

	CommonBaseBranchPatterns = []string{
		"main",
		"develop",
		"release/*",
		"hotfix/*",
	}

	CommonIgnoredTitlePatterns = []string{
		"wip*",
		"draft*",
		"chore(release)*",
		"release:*",
	}
)

// WizardOptions configures interactive settings collection.
type WizardOptions struct {
	Yes            bool
	NonInteractive bool
	In             io.Reader
	Out            io.Writer
}

// RunWizard runs the interactive setup questionnaire for repository review settings.
func RunWizard(current RepositoryReviewSettings, opts WizardOptions) (RepositoryReviewSettings, error) {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}

	// In non-interactive or yes mode, populate defaults if empty and return
	if opts.Yes || opts.NonInteractive {
		if len(current.IgnoredFilePatterns) == 0 {
			current.IgnoredFilePatterns = append([]string{}, RecommendedIgnoredFilePatterns...)
		}
		if current.RequestChangesMinSeverity == "" {
			current.RequestChangesMinSeverity = "high"
		}
		return current, nil
	}

	reader := bufio.NewReader(opts.In)
	next := current

	for {
		// 1. General Settings
		fmt.Fprintln(opts.Out, "\n── General Settings ───────────────────────────────────")
		fmt.Fprintln(opts.Out, "Configure automated review behaviors for this repository.")
		fmt.Fprintln(opts.Out, "ScanDrix automatically reviews pull requests when opened or updated.")
		fmt.Fprintln(opts.Out, "When disabled, reviews can still be triggered with @scandrix start-review.")

		next.ReviewEnabled = promptBool(reader, opts.Out, "Enable automated code review?", next.ReviewEnabled)
		next.AutoApproveEnabled = promptBool(reader, opts.Out, "Enable automatic pull request approval on clean reviews?", next.AutoApproveEnabled)
		next.RequestChangesMinSeverity = promptChoice(
			reader,
			opts.Out,
			"Minimum severity level for requested changes",
			[]string{"low", "medium", "high", "critical"},
			defaultSeverity(next.RequestChangesMinSeverity),
		)

		// 2. Pattern Settings
		fmt.Fprintln(opts.Out, "\n── Pattern Settings ───────────────────────────────────")
		fmt.Fprintln(opts.Out, "Files, base branches, and pull request titles to include or skip.")

		next.IgnoredFilePatterns = promptPatternList(
			reader,
			opts.Out,
			"Ignored file patterns (skip during review)",
			next.IgnoredFilePatterns,
			RecommendedIgnoredFilePatterns,
		)

		next.BaseBranchPatterns = promptPatternList(
			reader,
			opts.Out,
			"Base branches to review against (e.g. main, develop, release/*)",
			next.BaseBranchPatterns,
			CommonBaseBranchPatterns,
		)

		next.IgnoredTitlePatterns = promptPatternList(
			reader,
			opts.Out,
			"Ignored PR title patterns (skip review if title matches, e.g. wip*, draft*)",
			next.IgnoredTitlePatterns,
			CommonIgnoredTitlePatterns,
		)

		// 3. Review & Confirmation Loop
		for {
			fmt.Fprintln(opts.Out, "\n── Configuration Preview ──────────────────────────────")
			printPreview(opts.Out, current, next)

			choice := promptChoice(
				reader,
				opts.Out,
				"What do you want to do next?",
				[]string{"apply", "edit-general", "edit-patterns", "cancel"},
				"apply",
			)

			switch choice {
			case "apply":
				return next, nil
			case "cancel":
				return current, utils.NewCommandError("WIZARD_CANCELLED", "Setup wizard cancelled by user", 0, nil)
			case "edit-general":
				next.ReviewEnabled = promptBool(reader, opts.Out, "Enable automated code review?", next.ReviewEnabled)
				next.AutoApproveEnabled = promptBool(reader, opts.Out, "Enable automatic pull request approval on clean reviews?", next.AutoApproveEnabled)
				next.RequestChangesMinSeverity = promptChoice(
					reader,
					opts.Out,
					"Minimum severity level for requested changes",
					[]string{"low", "medium", "high", "critical"},
					defaultSeverity(next.RequestChangesMinSeverity),
				)
			case "edit-patterns":
				next.IgnoredFilePatterns = promptPatternList(
					reader,
					opts.Out,
					"Ignored file patterns",
					next.IgnoredFilePatterns,
					RecommendedIgnoredFilePatterns,
				)
				next.BaseBranchPatterns = promptPatternList(
					reader,
					opts.Out,
					"Base branches to review against",
					next.BaseBranchPatterns,
					CommonBaseBranchPatterns,
				)
				next.IgnoredTitlePatterns = promptPatternList(
					reader,
					opts.Out,
					"Ignored PR title patterns",
					next.IgnoredTitlePatterns,
					CommonIgnoredTitlePatterns,
				)
			}
		}
	}
}

func defaultSeverity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "high"
	}
	return s
}

func promptBool(r *bufio.Reader, w io.Writer, question string, def bool) bool {
	defStr := "Y/n"
	if !def {
		defStr = "y/N"
	}
	fmt.Fprintf(w, "%s [%s]: ", question, defStr)
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return def
	}
	return input == "y" || input == "yes" || input == "true"
}

func promptChoice(r *bufio.Reader, w io.Writer, question string, choices []string, def string) string {
	fmt.Fprintf(w, "%s (%s) [default: %s]: ", question, strings.Join(choices, "/"), def)
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return def
	}
	for _, c := range choices {
		if strings.ToLower(c) == input {
			return c
		}
	}
	return def
}

func promptPatternList(r *bufio.Reader, w io.Writer, label string, current []string, presets []string) []string {
	currStr := "(none)"
	if len(current) > 0 {
		currStr = strings.Join(current, ", ")
	}
	presetStr := strings.Join(presets, ", ")

	fmt.Fprintf(w, "\n%s\n", label)
	fmt.Fprintf(w, "  Current: %s\n", currStr)
	fmt.Fprintf(w, "  Preset recommendation: %s\n", presetStr)
	fmt.Fprintf(w, "  Choose: [1] Use preset, [2] Keep current, [3] Enter custom patterns (comma-separated): ")

	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		return append([]string{}, presets...)
	case "2", "":
		if len(current) > 0 {
			return current
		}
		return append([]string{}, presets...)
	case "3":
		fmt.Fprint(w, "  Enter patterns (comma-separated): ")
		customInput, _ := r.ReadString('\n')
		return parseCommaPatterns(customInput)
	default:
		// If user typed custom comma patterns directly
		if strings.Contains(input, ",") || strings.Contains(input, "*") {
			return parseCommaPatterns(input)
		}
		return append([]string{}, presets...)
	}
}

func parseCommaPatterns(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func printPreview(w io.Writer, old, new RepositoryReviewSettings) {
	fmt.Fprintf(w, "  Repository:                %s\n", new.Namespace)
	fmt.Fprintf(w, "  Automated Code Review:     %v\n", new.ReviewEnabled)
	fmt.Fprintf(w, "  Auto-Approve Clean PRs:    %v\n", new.AutoApproveEnabled)
	fmt.Fprintf(w, "  Minimum Severity Level:    %s\n", new.RequestChangesMinSeverity)
	fmt.Fprintf(w, "  Ignored File Patterns:     %s\n", summarizePatterns(new.IgnoredFilePatterns))
	fmt.Fprintf(w, "  Base Branch Patterns:      %s\n", summarizePatterns(new.BaseBranchPatterns))
	fmt.Fprintf(w, "  Ignored PR Title Patterns: %s\n", summarizePatterns(new.IgnoredTitlePatterns))
}

func summarizePatterns(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ", ")
}
