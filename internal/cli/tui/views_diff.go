package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderDiffView formats the unified git diff with syntax color highlights.
func (m Model) RenderDiffView(width, height int) string {
	if strings.TrimSpace(m.rawDiff) == "" {
		return lipgloss.Place(width, height-6, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(ColorMuted).Render("No git diff loaded for this workspace."))
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("📝 Unified Diff Inspection\n"))
	b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("─", width-6)) + "\n")

	diffLines := strings.Split(m.rawDiff, "\n")
	paneHeight := height - 8
	if paneHeight < 10 {
		paneHeight = 10
	}

	maxLines := paneHeight * 2
	if len(diffLines) > maxLines {
		diffLines = diffLines[:maxLines]
	}

	for _, line := range diffLines {
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			b.WriteString(DiffHeaderStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "@@") {
			b.WriteString(lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(line) + "\n")
		} else if strings.HasPrefix(line, "+") {
			b.WriteString(DiffAddedStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "-") {
			b.WriteString(DiffRemovedStyle.Render(line) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Render(line) + "\n")
		}
	}

	return CardStyle.Width(width - 4).Height(paneHeight).Render(b.String())
}
