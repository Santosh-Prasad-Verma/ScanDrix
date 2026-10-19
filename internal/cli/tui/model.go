package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/pkg/models"
)

// ActiveTab enum defines the current dashboard page.
type ActiveTab int

const (
	TabFindings ActiveTab = iota
	TabDiff
	TabStats
)

// Model represents the top-level Bubbletea state machine.
type Model struct {
	rawDiff          string
	findings         []models.CodeFinding
	filteredFindings []models.CodeFinding
	stats            *engine.CLIResult
	activeTab        ActiveTab
	cursorIndex      int
	severityFilter   models.FindingSeverity
	notification     string
	showHelp         bool
	width            int
	height           int
	runner           *engine.CLIRunner
	gateway          *llm.Gateway
}

// NewModel creates an initialized TUI dashboard state.
func NewModel(rawDiff string, runner *engine.CLIRunner) Model {
	if runner == nil {
		runner = engine.NewCLIRunner()
	}
	gw := engine.InitLocalGateway()

	var findings []models.CodeFinding
	result := &engine.CLIResult{Status: "passed"}
	initialTab := TabFindings
	notification := ""

	if strings.TrimSpace(rawDiff) == "" {
		initialTab = TabFindings
		notification = "✨ Clean working tree — no uncommitted changes"
	} else {
		// Run fast local review on startup to prevent TUI freeze
		res, err := runner.RunReview(context.Background(), rawDiff, engine.CLIOptions{
			DryRun:    true,
			RulesOnly: true,
			Fast:      true,
		})
		if err == nil && res != nil {
			result = res
			findings = res.Findings
		}
	}

	m := Model{
		rawDiff:          rawDiff,
		findings:         findings,
		filteredFindings: findings,
		stats:            result,
		activeTab:        initialTab,
		cursorIndex:      0,
		notification:     notification,
		runner:           runner,
		gateway:          gw,
		width:            100,
		height:           30,
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Standard navigation keys
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab":
			m.activeTab = (m.activeTab + 1) % 3
			return m, nil

		case "shift+tab":
			m.activeTab = (m.activeTab + 2) % 3
			return m, nil

		case "1":
			m.activeTab = TabFindings
			return m, nil

		case "2":
			m.activeTab = TabDiff
			return m, nil

		case "3":
			m.activeTab = TabStats
			return m, nil

		case "up", "k":
			if m.cursorIndex > 0 {
				m.cursorIndex--
			}
			return m, nil

		case "down", "j":
			if m.cursorIndex < len(m.filteredFindings)-1 {
				m.cursorIndex++
			}
			return m, nil

		case "f":
			m.cycleFilter()
			return m, nil

		case "a":
			if m.activeTab == TabFindings && len(m.filteredFindings) > 0 && m.cursorIndex < len(m.filteredFindings) {
				selected := m.filteredFindings[m.cursorIndex]
				status, err := ApplyFindingFix(selected)
				if err != nil {
					m.notification = fmt.Sprintf("❌ Fix Error: %v", err)
				} else {
					m.notification = fmt.Sprintf("✅ %s", status)
				}
			}
			return m, nil

		case "r":
			result, err := m.runner.RunReview(context.Background(), m.rawDiff, engine.CLIOptions{DryRun: true})
			if err == nil && result != nil {
				m.findings = result.Findings
				m.stats = result
				m.applyFilter()
				m.notification = "🔄 Diff re-evaluated successfully!"
			}
			return m, nil

		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) cycleFilter() {
	switch m.severityFilter {
	case "":
		m.severityFilter = models.SeverityCritical
	case models.SeverityCritical:
		m.severityFilter = models.SeverityHigh
	case models.SeverityHigh:
		m.severityFilter = models.SeverityMedium
	case models.SeverityMedium:
		m.severityFilter = models.SeverityLow
	case models.SeverityLow:
		m.severityFilter = ""
	}
	m.applyFilter()
}

func (m *Model) applyFilter() {
	if m.severityFilter == "" {
		m.filteredFindings = m.findings
	} else {
		var filtered []models.CodeFinding
		for _, f := range m.findings {
			if f.Severity == m.severityFilter {
				filtered = append(filtered, f)
			}
		}
		m.filteredFindings = filtered
	}
	if m.cursorIndex >= len(m.filteredFindings) {
		m.cursorIndex = 0
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing ScanDrix TUI..."
	}

	var b strings.Builder

	// 1. Header Bar
	headerLeft := "⚡ SCANDRIX DEVELOPER TERMINAL COCKPIT"
	filterText := "ALL"
	if m.severityFilter != "" {
		filterText = string(m.severityFilter)
	}
	headerRight := fmt.Sprintf("Filter: %s  |  Findings: %d", filterText, len(m.filteredFindings))
	headerRow := lipgloss.JoinHorizontal(lipgloss.Center,
		HeaderStyle.Render(headerLeft),
		lipgloss.NewStyle().Foreground(ColorHighlight).Padding(0, 2).Render(headerRight),
	)
	b.WriteString(headerRow + "\n")

	// 2. Tab Navigation Bar (4 tabs)
	var tabs []string
	if m.activeTab == TabFindings {
		tabs = append(tabs, ActiveTabStyle.Render("[1] Findings Navigator"))
	} else {
		tabs = append(tabs, InactiveTabStyle.Render("[1] Findings Navigator"))
	}

	if m.activeTab == TabDiff {
		tabs = append(tabs, ActiveTabStyle.Render("[2] Diff & Patch Browser"))
	} else {
		tabs = append(tabs, InactiveTabStyle.Render("[2] Diff & Patch Browser"))
	}

	if m.activeTab == TabStats {
		tabs = append(tabs, ActiveTabStyle.Render("[3] Security Metrics"))
	} else {
		tabs = append(tabs, InactiveTabStyle.Render("[3] Security Metrics"))
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + "\n\n")

	// 3. Body View
	switch m.activeTab {
	case TabFindings:
		b.WriteString(m.RenderFindingsView(m.width, m.height))
	case TabDiff:
		b.WriteString(m.RenderDiffView(m.width, m.height))
	case TabStats:
		b.WriteString(m.RenderStatsView(m.width, m.height))
	}

	// 4. Notification / Status Banner
	if m.notification != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("📢 "+m.notification))
	}

	// 5. Footer Shortcuts
	footer := lipgloss.JoinHorizontal(lipgloss.Left,
		HelpKeyStyle.Render("Tab/1-3:"), HelpDescStyle.Render("Tabs | "),
		HelpKeyStyle.Render("↑/↓:"), HelpDescStyle.Render("Navigate | "),
		HelpKeyStyle.Render("a:"), HelpDescStyle.Render("Apply Fix | "),
		HelpKeyStyle.Render("f:"), HelpDescStyle.Render("Filter | "),
		HelpKeyStyle.Render("r:"), HelpDescStyle.Render("Re-run | "),
		HelpKeyStyle.Render("q:"), HelpDescStyle.Render("Quit"),
	)
	b.WriteString("\n" + FooterStyle.Width(m.width).Render(footer))

	return b.String()
}
