package tui

import "github.com/charmbracelet/lipgloss"

// Color palette - Sleek modern cyber/dark theme
var (
	ColorPrimary   = lipgloss.Color("#6366F1") // Indigo
	ColorSecondary = lipgloss.Color("#EC4899") // Pink
	ColorAccent    = lipgloss.Color("#06B6D4") // Cyan
	ColorSuccess   = lipgloss.Color("#10B981") // Emerald
	ColorWarning   = lipgloss.Color("#F59E0B") // Amber
	ColorDanger    = lipgloss.Color("#EF4444") // Rose / Red
	ColorMuted     = lipgloss.Color("#64748B") // Slate Muted
	ColorDarkBg    = lipgloss.Color("#0F172A") // Deep Slate
	ColorCardBg    = lipgloss.Color("#1E293B") // Slate Card
	ColorHighlight = lipgloss.Color("#38BDF8") // Sky Blue
	ColorWhite     = lipgloss.Color("#F8FAFC")
)

// UI Component Styles
var (
	// Header & App Bar
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorPrimary).
			Padding(0, 2).
			MarginBottom(1)

	SubHeaderStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Tab bar
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorHighlight).
			Background(ColorCardBg).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorHighlight)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Background(ColorDarkBg).
				Padding(0, 2).
				Border(lipgloss.HiddenBorder())

	// Severity Badges
	CriticalBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorDanger).
			Padding(0, 1)

	HighBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7F1D1D")).
			Background(lipgloss.Color("#FCA5A5")).
			Padding(0, 1)

	MediumBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#78350F")).
			Background(ColorWarning).
			Padding(0, 1)

	LowBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#064E3B")).
			Background(ColorSuccess).
			Padding(0, 1)

	InfoBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorAccent).
			Padding(0, 1)

	// Card Containers
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(1, 2).
			Background(ColorCardBg)

	ActiveCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			Background(ColorCardBg)

	// Finding List Row
	SelectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorHighlight).
				Background(lipgloss.Color("#334155")).
				Padding(0, 1)

	UnselectedRowStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Padding(0, 1)

	// Diff Viewer Styling
	DiffAddedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399")).
			Background(lipgloss.Color("#064E3B"))

	DiffRemovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F87171")).
				Background(lipgloss.Color("#7F1D1D"))

	DiffHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	// Status & Footer
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorHighlight)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)
)
