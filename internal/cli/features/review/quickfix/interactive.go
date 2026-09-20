// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package quickfix

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/scandrix/backend/pkg/models"
)

var (
	colorTitle   = lipgloss.Color("#818CF8")
	colorAdd     = lipgloss.Color("#34D399")
	colorDel     = lipgloss.Color("#F87171")
	colorPrompt  = lipgloss.Color("#FBBF24")
	colorSuccess = lipgloss.Color("#34D399")
)

// InteractiveFixSession coordinates step-by-step review and application of code suggestions.
type InteractiveFixSession struct {
	workspaceRoot string
	reader        *bufio.Reader
	writer        io.Writer
	autoApplyAll  bool
}

// NewInteractiveFixSession initializes the interactive fix session.
func NewInteractiveFixSession(workspaceRoot string, r io.Reader, w io.Writer) *InteractiveFixSession {
	return &InteractiveFixSession{
		workspaceRoot: workspaceRoot,
		reader:        bufio.NewReader(r),
		writer:        w,
	}
}

// ReviewAndApply prompts the user for each finding with a suggested fix.
func (s *InteractiveFixSession) ReviewAndApply(findings []models.CodeFinding) (applied int, skipped int, err error) {
	var fixable []models.CodeFinding
	for _, f := range findings {
		if strings.TrimSpace(f.SuggestedDiff) != "" {
			fixable = append(fixable, f)
		}
	}

	if len(fixable) == 0 {
		fmt.Fprintln(s.writer, "No fixable suggestions with diffs available.")
		return 0, 0, nil
	}

	fmt.Fprintf(s.writer, "\nFound %d fixable suggestion(s). Reviewing interactively:\n\n", len(fixable))

	for idx, f := range fixable {
		targetFile := filepath.Join(s.workspaceRoot, f.FilePath)

		fmt.Fprintln(s.writer, strings.Repeat("─", 65))
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
		fmt.Fprintf(s.writer, "[%d/%d] %s\n", idx+1, len(fixable), titleStyle.Render(f.Title))
		fmt.Fprintf(s.writer, "Location: %s:%d\n\n", f.FilePath, f.StartLine)

		// Render diff preview
		s.renderDiffPreview(f.SuggestedDiff)

		if !s.autoApplyAll {
			promptStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrompt)
			fmt.Fprintf(s.writer, "%s [y]es / [n]o / [a]ll / [q]uit: ", promptStyle.Render("Apply this fix?"))

			input, readErr := s.reader.ReadString('\n')
			if readErr != nil && len(input) == 0 {
				return applied, skipped, readErr
			}

			choice := strings.ToLower(strings.TrimSpace(input))
			switch choice {
			case "q", "quit":
				fmt.Fprintln(s.writer, "\nExiting interactive fix session.")
				return applied, skipped, nil
			case "n", "no":
				skipped++
				fmt.Fprintln(s.writer, "Skipped.")
				continue
			case "a", "all":
				s.autoApplyAll = true
			case "y", "yes", "":
				// Proceed to apply
			default:
				skipped++
				continue
			}
		}

		// Apply fix
		res, applyErr := ApplySuggestedDiff(targetFile, f.SuggestedDiff, true)
		if applyErr != nil {
			fmt.Fprintf(s.writer, "❌ Error applying fix to %s: %v\n", f.FilePath, applyErr)
			skipped++
		} else {
			succStyle := lipgloss.NewStyle().Foreground(colorSuccess)
			fmt.Fprintf(s.writer, "✔ %s\n", succStyle.Render(fmt.Sprintf("Fix applied successfully to %s", f.FilePath)))
			if res.BackupPath != "" {
				fmt.Fprintf(s.writer, "  Backup saved to %s\n", res.BackupPath)
			}
			applied++
		}
	}

	fmt.Fprintln(s.writer, strings.Repeat("─", 65))
	fmt.Fprintf(s.writer, "Summary: %d applied, %d skipped.\n\n", applied, skipped)
	return applied, skipped, nil
}

func (s *InteractiveFixSession) renderDiffPreview(diffText string) {
	lines := strings.Split(diffText, "\n")
	addStyle := lipgloss.NewStyle().Foreground(colorAdd)
	delStyle := lipgloss.NewStyle().Foreground(colorDel)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))

	for _, l := range lines {
		if strings.HasPrefix(l, "+") {
			fmt.Fprintln(s.writer, addStyle.Render(l))
		} else if strings.HasPrefix(l, "-") {
			fmt.Fprintln(s.writer, delStyle.Render(l))
		} else {
			fmt.Fprintln(s.writer, dimStyle.Render(l))
		}
	}
	fmt.Fprintln(s.writer)
}
