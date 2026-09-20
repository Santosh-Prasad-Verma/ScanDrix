// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TaskProgressTracker provides Antigravity/Cursor style collapsible step-by-step progress cards.
type TaskProgressTracker struct {
	startTime     time.Time
	lastUpdate    time.Time
	filesScanned  int
	linesScanned  int
	analyzedFiles map[string]bool
	analyzedCount int
	maxAnalyzed   int
	target        string
}

func NewTaskProgressTracker(target string) *TaskProgressTracker {
	return &TaskProgressTracker{
		startTime:     time.Now(),
		lastUpdate:    time.Now(),
		analyzedFiles: make(map[string]bool),
		maxAnalyzed:   5,
		target:        target,
	}
}

func (t *TaskProgressTracker) Start(summary string) {
	fmt.Println()
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Bold(true).Render(summary + " ⌄")
	fmt.Println(header)
	fmt.Println()
}

func (t *TaskProgressTracker) OnProgress(relPath string, current int, lineCount int) {
	t.filesScanned = current
	t.linesScanned += lineCount

	now := time.Now()
	if now.Sub(t.lastUpdate) < 90*time.Millisecond && current%30 != 0 {
		return
	}
	t.lastUpdate = now

	lower := strings.ToLower(relPath)
	isKeyFile := strings.Contains(lower, "auth") || strings.Contains(lower, "login") ||
		strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "controller") || strings.Contains(lower, "gateway") ||
		strings.Contains(lower, "chat.go") || strings.Contains(lower, "main.go") ||
		strings.Contains(lower, "runner.go")

	if isKeyFile && !t.analyzedFiles[relPath] && t.analyzedCount < t.maxAnalyzed {
		t.analyzedFiles[relPath] = true
		t.analyzedCount++
		fmt.Print("\r\033[K")
		fmt.Printf("  %s %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render("Analyzed"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true).Render(filepath.Base(relPath)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render(fmt.Sprintf("#L1-%d", lineCount)),
		)
	}

	// Single transient line - truncated to 65 chars to never wrap
	status := fmt.Sprintf("  Scanning... %d files (%d lines)", current, t.linesScanned)
	if len(status) > 65 {
		status = status[:65]
	}
	fmt.Printf("\r\033[K%s", lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render(status))
}

func (t *TaskProgressTracker) OnStatus(statusMsg string) {
	fmt.Print("\r\033[K")
	if strings.Contains(statusMsg, "Analyzed") {
		parts := strings.SplitN(statusMsg, "Analyzed ", 2)
		if len(parts) == 2 {
			f := parts[1]
			if !t.analyzedFiles[f] && t.analyzedCount < t.maxAnalyzed {
				t.analyzedFiles[f] = true
				t.analyzedCount++
				fmt.Printf("  %s %s\n",
					lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render("Analyzed"),
					lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true).Render(filepath.Base(f)),
				)
				return
			}
		}
	} else if strings.Contains(statusMsg, "AST") || strings.Contains(statusMsg, "rule") {
		fmt.Printf("  %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render("Searched"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Render("security AST patterns & invariant rules"),
		)
	} else if strings.Contains(statusMsg, "synthesized") {
		fmt.Printf("  %s %s\n",
			lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("✦"),
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")).Render(statusMsg),
		)
	} else if strings.Contains(statusMsg, "skipped") || strings.Contains(statusMsg, "failed") {
		fmt.Printf("  %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("⚠️"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(statusMsg),
		)
	} else if strings.Contains(statusMsg, "AI") || strings.Contains(statusMsg, "Synthesizing") {
		elapsed := int(time.Since(t.startTime).Seconds())
		if elapsed < 1 {
			elapsed = 1
		}
		fmt.Printf("  %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Render("Thought"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(fmt.Sprintf("for %ds ›", elapsed)),
		)
		fmt.Printf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("Working.."))
	}
}

func (t *TaskProgressTracker) Finish() {
	fmt.Print("\r\033[K")
	elapsed := int(time.Since(t.startTime).Seconds())
	if elapsed < 1 {
		elapsed = 1
	}
	fmt.Printf("  %s\n\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(fmt.Sprintf("Thought for %ds › Completed audit", elapsed)),
	)
}
