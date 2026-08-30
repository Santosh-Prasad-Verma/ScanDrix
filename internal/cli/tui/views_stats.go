package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderStatsView displays security scores, issue breakdowns, and policy gate status.
func (m Model) RenderStatsView(width, height int) string {
	cardWidth := (width - 8) / 2
	paneHeight := height - 8
	if paneHeight < 12 {
		paneHeight = 12
	}

	// 1. Security Score & Policy Gate Card
	var left strings.Builder
	left.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("🛡️  ScanDrix Security & Quality Scorecard\n\n"))

	score := 100 - (m.stats.CriticalCount*25 + m.stats.HighCount*10 + m.stats.MediumCount*4 + m.stats.LowCount*1)
	if score < 0 {
		score = 0
	}

	grade := "A+"
	gradeColor := ColorSuccess
	if score < 60 {
		grade = "F (Failing)"
		gradeColor = ColorDanger
	} else if score < 75 {
		grade = "C (Needs Review)"
		gradeColor = ColorWarning
	} else if score < 90 {
		grade = "B (Good)"
		gradeColor = ColorAccent
	}

	left.WriteString(fmt.Sprintf("  Security Health Index : %s\n",
		lipgloss.NewStyle().Bold(true).Foreground(gradeColor).Render(fmt.Sprintf("%d / 100 (Grade %s)", score, grade))))

	status := "✅ PASSED (No blocking violations)"
	if m.stats.IsBlocking {
		status = "🚫 BLOCKED (Violates security quality threshold)"
	}
	left.WriteString(fmt.Sprintf("  Quality Gate Status   : %s\n",
		lipgloss.NewStyle().Bold(true).Render(status)))

	left.WriteString(fmt.Sprintf("  Files Evaluated       : %d\n", m.stats.FilesReviewed))
	left.WriteString(fmt.Sprintf("  Scan Duration         : %s\n\n", m.stats.Duration.Round(100)))

	left.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("📊 Severity Breakdown:\n"))
	left.WriteString(fmt.Sprintf("  • %s : %d\n", CriticalBadge.Render("Critical"), m.stats.CriticalCount))
	left.WriteString(fmt.Sprintf("  • %s : %d\n", HighBadge.Render("High"), m.stats.HighCount))
	left.WriteString(fmt.Sprintf("  • %s : %d\n", MediumBadge.Render("Medium"), m.stats.MediumCount))
	left.WriteString(fmt.Sprintf("  • %s : %d\n", LowBadge.Render("Low"), m.stats.LowCount))

	leftCard := CardStyle.Width(cardWidth).Height(paneHeight).Render(left.String())

	// 2. Rule & Fix Availability Breakdown
	var right strings.Builder
	right.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("⚙️  Automated Remediation & Rule Coverage\n\n"))

	fixesAvailable := 0
	categoryCounts := make(map[string]int)
	for _, f := range m.findings {
		if f.SuggestedDiff != "" {
			fixesAvailable++
		}
		cat := f.Category
		if cat == "" {
			cat = "OWASP / Security"
		}
		categoryCounts[cat]++
	}

	right.WriteString(fmt.Sprintf("  One-Click Fixes Ready : %s\n\n",
		lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(fmt.Sprintf("%d of %d findings", fixesAvailable, len(m.findings)))))

	right.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("🏷️  Findings By Domain:\n"))
	if len(categoryCounts) == 0 {
		right.WriteString("  No domain violations.\n")
	} else {
		for cat, count := range categoryCounts {
			bar := strings.Repeat("█", count*2)
			right.WriteString(fmt.Sprintf("  • %-24s : %-3d %s\n", cat, count, lipgloss.NewStyle().Foreground(ColorPrimary).Render(bar)))
		}
	}

	right.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorMuted).Render("Tip: Press '1' or 'tab' to jump to Findings list and press 'a' to apply suggested fixes."))

	rightCard := ActiveCardStyle.Width(cardWidth).Height(paneHeight).Render(right.String())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCard)
}
