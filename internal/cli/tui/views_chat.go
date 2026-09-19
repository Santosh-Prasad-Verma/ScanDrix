package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderChatView formats the live interactive AI agent conversation inside the TUI cockpit.
func (m Model) RenderChatView(width, height int) string {
	var b strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("🤖 ScanDrix Live AI Assistant & Code Reviewer")
	sub := lipgloss.NewStyle().Foreground(ColorMuted).Render("  (Ask about findings, code architecture, security fixes, or git diffs)")
	b.WriteString(header + sub + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("─", width-6)) + "\n\n")

	paneHeight := height - 10
	if paneHeight < 10 {
		paneHeight = 10
	}

	if len(m.chatHistory) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(
			"👋 Welcome to ScanDrix Live AI Assistant!\n\n" +
				"Try asking:\n" +
				"  • \"Explain the findings discovered in this review\"\n" +
				"  • \"How do I fix the SQL injection vulnerability in my database query?\"\n" +
				"  • \"Is my JWT authentication and session handling secure?\"\n" +
				"  • \"Write unit tests for the changed files\"\n\n",
		))
	} else {
		for _, msg := range m.chatHistory {
			if msg.Role == "user" {
				userBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8")).Render("👤 You: ")
				b.WriteString(userBadge + msg.Content + "\n\n")
			} else {
				aiBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A855F7")).Render("🤖 ScanDrix AI: ")
				lines := strings.Split(msg.Content, "\n")
				var aiContent strings.Builder
				for _, l := range lines {
					if strings.HasPrefix(l, "```") {
						aiContent.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render(l) + "\n")
					} else if strings.HasPrefix(l, "#") {
						aiContent.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render(l) + "\n")
					} else {
						aiContent.WriteString(l + "\n")
					}
				}
				b.WriteString(aiBadge + "\n" + aiContent.String() + "\n")
			}
		}
	}

	if m.isChatLoading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("⏳ ScanDrix AI is reasoning and synthesizing response...\n"))
	}

	// Input Bar
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorHighlight).
		Background(ColorCardBg).
		Padding(0, 1).
		Width(width - 8)

	promptText := m.chatInput
	if promptText == "" && !m.isChatLoading {
		promptText = lipgloss.NewStyle().Foreground(ColorMuted).Render("Type a question and press Enter...")
	}

	inputLine := fmt.Sprintf("❯ %s", promptText)
	b.WriteString("\n" + inputBox.Render(inputLine))

	return CardStyle.Width(width - 4).Height(paneHeight).Render(b.String())
}
