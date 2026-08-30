package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/scandrix/backend/pkg/models"
)

// RenderFindingsView displays the split-pane findings navigator and detail inspector.
func (m Model) RenderFindingsView(width, height int) string {
	if len(m.filteredFindings) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true).
			Render("✨ No security or code quality issues detected! Clean scan.")
		return lipgloss.Place(width, height-6, lipgloss.Center, lipgloss.Center, emptyMsg)
	}

	leftWidth := width*45/100 - 2
	rightWidth := width - leftWidth - 6
	paneHeight := height - 8
	if paneHeight < 10 {
		paneHeight = 10
	}

	// 1. Left Pane: Findings List
	var listRows []string
	listRows = append(listRows, lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("  SEVERITY  FINDING & LOCATION"))
	listRows = append(listRows, lipgloss.NewStyle().Foreground(ColorMuted).Render("  " + strings.Repeat("─", leftWidth-4)))

	for i, f := range m.filteredFindings {
		isSelected := (i == m.cursorIndex)
		var badge string
		switch f.Severity {
		case models.SeverityCritical:
			badge = CriticalBadge.Render("CRIT")
		case models.SeverityHigh:
			badge = HighBadge.Render("HIGH")
		case models.SeverityMedium:
			badge = MediumBadge.Render("MED ")
		case models.SeverityLow:
			badge = LowBadge.Render("LOW ")
		default:
			badge = InfoBadge.Render("INFO")
		}

		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}

		title := f.Title
		if len(title) > leftWidth-20 {
			title = title[:leftWidth-23] + "..."
		}

		fileLoc := fmt.Sprintf("%s:%d", truncatePath(f.FilePath, leftWidth-12), f.StartLine)

		rowText := fmt.Sprintf("%s%s %s\n     %s", cursor, badge, title, lipgloss.NewStyle().Foreground(ColorMuted).Render(fileLoc))

		if isSelected {
			listRows = append(listRows, SelectedRowStyle.Width(leftWidth-2).Render(rowText))
		} else {
			listRows = append(listRows, UnselectedRowStyle.Width(leftWidth-2).Render(rowText))
		}
	}

	leftPane := CardStyle.Width(leftWidth).Height(paneHeight).Render(strings.Join(listRows, "\n"))

	// 2. Right Pane: Finding Detail Inspector
	var rightContent string
	if m.cursorIndex >= 0 && m.cursorIndex < len(m.filteredFindings) {
		selected := m.filteredFindings[m.cursorIndex]
		rightContent = renderFindingDetail(selected, rightWidth)
	} else {
		rightContent = lipgloss.NewStyle().Foreground(ColorMuted).Render("Select a finding from the left list to inspect.")
	}

	rightPane := ActiveCardStyle.Width(rightWidth).Height(paneHeight).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func renderFindingDetail(f models.CodeFinding, width int) string {
	var b strings.Builder

	var badge string
	switch f.Severity {
	case models.SeverityCritical:
		badge = CriticalBadge.Render(" CRITICAL SEVERITY ")
	case models.SeverityHigh:
		badge = HighBadge.Render(" HIGH SEVERITY ")
	case models.SeverityMedium:
		badge = MediumBadge.Render(" MEDIUM SEVERITY ")
	case models.SeverityLow:
		badge = LowBadge.Render(" LOW SEVERITY ")
	default:
		badge = InfoBadge.Render(" INFO ")
	}

	b.WriteString(fmt.Sprintf("%s  %s\n\n", badge, lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Render(f.Title)))
	b.WriteString(fmt.Sprintf("%s %s (Lines %d-%d)\n",
		lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("📍 File:"),
		f.FilePath, f.StartLine, f.EndLine))
	b.WriteString(fmt.Sprintf("%s %s\n\n",
		lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("🏷️  Category:"),
		f.Category))

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("📖 Description:\n"))
	b.WriteString(lipgloss.NewStyle().Foreground(ColorWhite).Render(wrapText(f.Description, width-6)) + "\n\n")

	if f.Remediation != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render("💡 Remediation Guidance:\n"))
		b.WriteString(lipgloss.NewStyle().Foreground(ColorWhite).Render(wrapText(f.Remediation, width-6)) + "\n\n")
	}

	if f.SuggestedDiff != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("🛠️  Suggested Fix [Press 'a' to Apply]:\n"))
		diffLines := strings.Split(f.SuggestedDiff, "\n")
		for _, line := range diffLines {
			if strings.HasPrefix(line, "+") {
				b.WriteString(DiffAddedStyle.Render(line) + "\n")
			} else if strings.HasPrefix(line, "-") {
				b.WriteString(DiffRemovedStyle.Render(line) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render(line) + "\n")
			}
		}
	}

	return b.String()
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen || maxLen <= 5 {
		return path
	}
	return "..." + path[len(path)-(maxLen-3):]
}

func wrapText(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	currLine := words[0]

	for _, w := range words[1:] {
		if len(currLine)+1+len(w) <= maxLen {
			currLine += " " + w
		} else {
			lines = append(lines, currLine)
			currLine = w
		}
	}
	lines = append(lines, currLine)
	return strings.Join(lines, "\n")
}
