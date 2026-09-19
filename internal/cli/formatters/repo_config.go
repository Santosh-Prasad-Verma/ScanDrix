// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/api"
)

// REPOSITORY CONFIGURATION & TRACKING DISPLAY FORMATTERS

// RepositorySettingSource models the source hierarchy of a setting.
type RepositorySettingSource struct {
	Level           string `json:"level"`
	OverriddenLevel string `json:"overriddenLevel,omitempty"`
}

// RepositorySettingsResult represents full settings payload with sources.
type RepositorySettingsResult struct {
	RepositoryFullName string               `json:"repositoryFullName"`
	Settings           DetailedRepoSettings `json:"settings"`
}

// DetailedRepoSettings holds settings and source metadata.
type DetailedRepoSettings struct {
	ReviewEnabled             bool                               `json:"reviewEnabled"`
	AutoApproveEnabled        bool                               `json:"autoApproveEnabled"`
	RequestChangesMinSeverity string                             `json:"requestChangesMinSeverity"`
	IgnoredFilePatterns       []string                           `json:"ignoredFilePatterns"`
	BaseBranchPatterns         []string                           `json:"baseBranchPatterns"`
	IgnoredTitlePatterns      []string                           `json:"ignoredTitlePatterns"`
	Sources                   map[string]RepositorySettingSource `json:"sources,omitempty"`
}

// FormatPatternList formats a slice of patterns with '(none)' fallback.
func FormatPatternList(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ", ")
}

// FormatEnabledLabel returns colored/formatted enabled or disabled status.
func FormatEnabledLabel(enabled bool) string {
	if enabled {
		return AnsiGreen + AnsiBold + "enabled" + AnsiReset
	}
	return AnsiRed + AnsiBold + "disabled" + AnsiReset
}

// FormatSeverityLabel returns colored severity indicator.
func FormatSeverityLabel(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return AnsiBrightRed + AnsiBold + severity + AnsiReset
	case "high":
		return AnsiRed + AnsiBold + severity + AnsiReset
	case "medium":
		return AnsiYellow + AnsiBold + severity + AnsiReset
	case "low":
		return AnsiCyan + AnsiBold + severity + AnsiReset
	default:
		return severity
	}
}

// FormatSourceLabel formats the source override note.
func FormatSourceLabel(source *RepositorySettingSource) string {
	if source == nil || source.Level == "" {
		return ""
	}
	level := strings.ReplaceAll(source.Level, "_", " ")
	if source.OverriddenLevel != "" {
		overridden := strings.ReplaceAll(source.OverriddenLevel, "_", " ")
		return fmt.Sprintf(" [%s overrides %s]", level, overridden)
	}
	return fmt.Sprintf(" [%s]", level)
}

// FormatRepositorySettings lines for terminal display.
func FormatRepositorySettings(res RepositorySettingsResult) []string {
	lines := []string{
		fmt.Sprintf("Repository settings: %s", res.RepositoryFullName),
		"",
		AnsiBlue + AnsiBold + "Status" + AnsiReset,
	}

	revSrc := res.Settings.Sources["reviewEnabled"]
	lines = append(lines, fmt.Sprintf("Automated review: %s%s",
		FormatEnabledLabel(res.Settings.ReviewEnabled), FormatSourceLabel(&revSrc)))

	apprSrc := res.Settings.Sources["autoApproveEnabled"]
	lines = append(lines, fmt.Sprintf("Auto approve: %s%s",
		FormatEnabledLabel(res.Settings.AutoApproveEnabled), FormatSourceLabel(&apprSrc)))

	sevSrc := res.Settings.Sources["requestChangesMinSeverity"]
	lines = append(lines, fmt.Sprintf("Minimum severity level: %s%s",
		FormatSeverityLabel(res.Settings.RequestChangesMinSeverity), FormatSourceLabel(&sevSrc)))

	lines = append(lines, "", AnsiBlue + AnsiBold + "Patterns" + AnsiReset)

	ignSrc := res.Settings.Sources["ignoredFilePatterns"]
	lines = append(lines, fmt.Sprintf("Ignored file patterns: %s%s",
		FormatPatternList(res.Settings.IgnoredFilePatterns), FormatSourceLabel(&ignSrc)))

	baseSrc := res.Settings.Sources["baseBranchPatterns"]
	lines = append(lines, fmt.Sprintf("Base branch patterns: %s%s",
		FormatPatternList(res.Settings.BaseBranchPatterns), FormatSourceLabel(&baseSrc)))

	titleSrc := res.Settings.Sources["ignoredTitlePatterns"]
	lines = append(lines, fmt.Sprintf("Ignored title patterns: %s%s",
		FormatPatternList(res.Settings.IgnoredTitlePatterns), FormatSourceLabel(&titleSrc)))

	return lines
}

// FormatRepositorySetupPreview displays diff preview between current and next settings.
func FormatRepositorySetupPreview(repoFullName string, current, next DetailedRepoSettings) []string {
	lines := []string{
		fmt.Sprintf("Configuration preview for %s:", repoFullName),
		"",
	}

	formatRow := func(label, curVal, nextVal string) string {
		if curVal == nextVal {
			return fmt.Sprintf("· %s: %s", label, nextVal)
		}
		return fmt.Sprintf("+ %s: %s -> %s", label, curVal, nextVal)
	}

	lines = append(lines, formatRow("Automated review", fmt.Sprintf("%v", current.ReviewEnabled), fmt.Sprintf("%v", next.ReviewEnabled)))
	lines = append(lines, formatRow("Auto approve", fmt.Sprintf("%v", current.AutoApproveEnabled), fmt.Sprintf("%v", next.AutoApproveEnabled)))
	lines = append(lines, formatRow("Min severity", current.RequestChangesMinSeverity, next.RequestChangesMinSeverity))
	lines = append(lines, formatRow("Ignored files", FormatPatternList(current.IgnoredFilePatterns), FormatPatternList(next.IgnoredFilePatterns)))
	lines = append(lines, formatRow("Base branches", FormatPatternList(current.BaseBranchPatterns), FormatPatternList(next.BaseBranchPatterns)))
	lines = append(lines, formatRow("Ignored titles", FormatPatternList(current.IgnoredTitlePatterns), FormatPatternList(next.IgnoredTitlePatterns)))

	return lines
}

// PrintRepoSettings displays repository settings in terminal.
func PrintRepoSettings(w io.Writer, s *api.RepositorySettings) {
	fmt.Fprintf(w, "Repository: %s\n", s.Namespace)
	fmt.Fprintf(w, "Default Branch: %s\n", s.DefaultBranch)

	if len(s.IgnoredPaths) > 0 {
		fmt.Fprintln(w, "Ignored Paths:")
		for _, p := range s.IgnoredPaths {
			fmt.Fprintf(w, "  - %s\n", p)
		}
	} else {
		fmt.Fprintln(w, "Ignored Paths: None configured")
	}

	if len(s.FocusAreas) > 0 {
		fmt.Fprintln(w, "Focus Areas:")
		for _, a := range s.FocusAreas {
			fmt.Fprintf(w, "  - %s\n", a)
		}
	}

	if len(s.Reviewers) > 0 {
		fmt.Fprintf(w, "Assigned Reviewers: %s\n", strings.Join(s.Reviewers, ", "))
	}
}

// PrintRepoList displays list of tracked repositories.
func PrintRepoList(w io.Writer, repos []api.TrackedRepository) {
	if len(repos) == 0 {
		fmt.Fprintln(w, "No tracked repositories found in this workspace.")
		return
	}

	fmt.Fprintf(w, "Tracked Repositories (%d):\n", len(repos))
	for i, r := range repos {
		fmt.Fprintf(w, "  %d. %s (%s) [default: %s]\n", i+1, r.Namespace, r.Provider, r.DefaultBranch)
	}
}
