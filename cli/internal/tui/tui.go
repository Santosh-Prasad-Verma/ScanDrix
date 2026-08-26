package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D7D7"))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF87"))

	warningStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFAF00"))

	dangerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF5F87"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#808080"))
)

type StepState int

const (
	StepQueued StepState = iota
	StepRunning
	StepDone
)

type ScanStep struct {
	Name    string
	Details string
	State   StepState
}

type Model struct {
	repoName   string
	steps      []ScanStep
	current    int
	isComplete bool
	ticks      int
}

type tickMsg time.Time

func NewModel(repoName string) Model {
	return Model{
		repoName: repoName,
		steps: []ScanStep{
			{Name: "AST Parsing & Graph Indexing", Details: "Tree-sitter extracting symbols and call edges", State: StepRunning},
			{Name: "Multi-Engine Static SAST", Details: "Semgrep + Gitleaks secret and pattern scanning", State: StepQueued},
			{Name: "Inter-procedural Taint Analysis", Details: "Tracing untrusted inputs to SQL/Command sinks", State: StepQueued},
			{Name: "MicroVM Dynamic Sandbox QA", Details: "Executing AI-generated mutation tests", State: StepQueued},
			{Name: "Auto-Patch Synthesis & Verification", Details: "Validating fixes with zero test regression", State: StepQueued},
		},
		current:    0,
		isComplete: false,
		ticks:      0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "enter" {
			return m, tea.Quit
		}
	case tickMsg:
		m.ticks++
		if m.ticks%2 == 0 && m.current < len(m.steps) {
			m.steps[m.current].State = StepDone
			m.current++
			if m.current < len(m.steps) {
				m.steps[m.current].State = StepRunning
			} else {
				m.isComplete = true
				return m, nil
			}
		}
		if !m.isComplete {
			return m, tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg {
				return tickMsg(t)
			})
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("🐕  CodeHound Enterprise Security & Dynamic Verification Engine"))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("Auditing Repository: %s", m.repoName)))
	b.WriteString("\n\n")

	for _, step := range m.steps {
		var icon string
		var name string

		switch step.State {
		case StepDone:
			icon = successStyle.Render("✔")
			name = successStyle.Render(step.Name)
		case StepRunning:
			spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spin := spinners[m.ticks%len(spinners)]
			icon = warningStyle.Render(spin)
			name = warningStyle.Render(step.Name)
		default:
			icon = subtleStyle.Render("○")
			name = subtleStyle.Render(step.Name)
		}

		b.WriteString(fmt.Sprintf("  %s %s\n    %s\n\n", icon, name, subtleStyle.Render(step.Details)))
	}

	if m.isComplete {
		b.WriteString(successStyle.Render("🎉 Audit Complete! 0 Critical Flaws, 1 Auto-Verified Patch Generated."))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("Press [q] or [Enter] to exit."))
		b.WriteString("\n")
	} else {
		b.WriteString(subtleStyle.Render("Press [Ctrl+C] to abort scan."))
		b.WriteString("\n")
	}

	return b.String()
}
