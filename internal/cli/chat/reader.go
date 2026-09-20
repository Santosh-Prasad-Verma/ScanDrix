// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// KnownSlashCommands for live auto-completion and dropdown palette
var KnownSlashCommands = []struct {
	Command     string
	Description string
}{
	{"/scan", "Deep scan workspace files for vulnerabilities & secrets"},
	{"/review", "AI code review on git diff or PR (--staged, --fast, --fix)"},
	{"/diff", "Inspect colorized git diff (--staged, --branch)"},
	{"/fix", "Automatically apply AST security remediations"},
	{"/dry-run", "Dry run review against active rules"},
	{"/pentest", "Trigger Strix dynamic penetration testing"},
	{"/models", "List all available AI models and switch"},
	{"/model", "Switch active AI model (e.g. /model 2 or /model deepseek)"},
	{"/agents", "List all reviewer personas"},
	{"/agent", "Switch reviewer persona (e.g. /agent threat or /agent perf)"},
	{"/rules", "Display active workspace security rules"},
	{"/skills", "Install/sync bundled AI assistant skills"},
	{"/trace", "Developer decision telemetry & local trace cockpit"},
	{"/hooks", "Install or remove automated Git pre-commit guards"},
	{"/pr", "Fetch and review remote Pull Request by number"},
	{"/config", "View and modify workspace configuration"},
	{"/auth", "Display authentication session status & API keys"},
	{"/status", "Full environment telemetry dashboard"},
	{"/schema", "Export CLI JSON schema for AI agents"},
	{"/clear", "Clear screen and reset session history"},
	{"/help", "Show all shortcuts and command reference"},
	{"/exit", "Exit live chat session"},
}

// LineReader provides an interactive terminal input with live slash dropdown, arrow keys, history, and autocomplete.
type LineReader struct {
	history           []string
	historyIndex      int
	prompt            string
	fallback          *bufio.Reader
	renderedMenuLines int
	menuIndex         int
}

// NewLineReader creates a new terminal line reader.
func NewLineReader(prompt string) *LineReader {
	return &LineReader{
		history:           make([]string, 0),
		historyIndex:      0,
		prompt:            prompt,
		fallback:          bufio.NewReader(os.Stdin),
		renderedMenuLines: 0,
		menuIndex:         0,
	}
}

// getMatchingCommands returns filtered slash commands matching current input
func (r *LineReader) getMatchingCommands(line string) []struct {
	Command     string
	Description string
} {
	if !strings.HasPrefix(line, "/") {
		return nil
	}
	prefix := strings.ToLower(line)
	var matches []struct {
		Command     string
		Description string
	}
	for _, c := range KnownSlashCommands {
		if strings.HasPrefix(strings.ToLower(c.Command), prefix) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 && len(prefix) > 1 {
		// Fuzzy/contains fallback
		sub := prefix[1:]
		for _, c := range KnownSlashCommands {
			if strings.Contains(strings.ToLower(c.Command), sub) {
				matches = append(matches, c)
			}
		}
	}
	return matches
}

// clearMenu removes any rendered dropdown lines below the prompt
func (r *LineReader) clearMenu() {
	if r.renderedMenuLines > 0 {
		for i := 0; i < r.renderedMenuLines; i++ {
			fmt.Print("\r\n\033[K")
		}
		fmt.Printf("\033[%dA", r.renderedMenuLines)
		r.renderedMenuLines = 0
	}
}

// render draws prompt, content, and live slash dropdown
func (r *LineReader) render(line []rune, cursor int) {
	r.clearMenu()

	promptStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorPrompt).Render(r.prompt) + " "
	content := string(line)

	// 1. Draw prompt and input line
	fmt.Print("\r\033[K" + promptStyled + content)

	// 2. Position cursor on the input line
	promptLen := utf8.RuneCountInString(r.prompt) + 1
	targetCol := promptLen + cursor + 1
	fmt.Printf("\033[%dG", targetCol)

	// 3. Render live slash command dropdown if input starts with "/"
	matches := r.getMatchingCommands(content)
	if len(matches) > 0 {
		if r.menuIndex >= len(matches) {
			r.menuIndex = 0
		}
		if r.menuIndex < 0 {
			r.menuIndex = len(matches) - 1
		}

		viewportSize := 8
		if viewportSize > len(matches) {
			viewportSize = len(matches)
		}

		startIndex := 0
		if r.menuIndex >= viewportSize {
			startIndex = r.menuIndex - viewportSize + 1
		}
		endIndex := startIndex + viewportSize
		if endIndex > len(matches) {
			endIndex = len(matches)
			startIndex = endIndex - viewportSize
			if startIndex < 0 {
				startIndex = 0
			}
		}

		linesDrawn := 0
		for i := startIndex; i < endIndex; i++ {
			cmd := matches[i]
			fmt.Print("\r\n\033[K")

			if i == r.menuIndex {
				prefixBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#4F46E5")).Render(fmt.Sprintf(" > %-12s", cmd.Command))
				descText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Render(" " + cmd.Description)
				fmt.Print("  " + prefixBadge + descText)
			} else {
				prefixBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#818CF8")).Render(fmt.Sprintf("   %-12s", cmd.Command))
				descText := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(" " + cmd.Description)
				fmt.Print("  " + prefixBadge + descText)
			}
			linesDrawn++
		}

		if len(matches) > viewportSize {
			fmt.Print("\r\n\033[K")
			footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render(fmt.Sprintf("     -- %d of %d commands (arrow up/down to scroll, Tab to select) --", r.menuIndex+1, len(matches)))
			fmt.Print(footer)
			linesDrawn++
		}

		r.renderedMenuLines = linesDrawn
		// Move cursor back up to the prompt line and exact column
		fmt.Printf("\033[%dA", linesDrawn)
		fmt.Printf("\033[%dG", targetCol)
	}
}

// ReadLine reads one line with interactive live dropdown, editing, and history navigation.
func (r *LineReader) ReadLine() (string, error) {
	fd := uintptr(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := r.fallback.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		line, err := r.fallback.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), err
	}
	defer func() {
		_ = term.Restore(fd, oldState)
	}()

	line := make([]rune, 0)
	cursor := 0
	savedCurrent := ""
	r.historyIndex = len(r.history)
	r.menuIndex = 0
	r.renderedMenuLines = 0

	r.render(line, cursor)

	buf := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			r.clearMenu()
			if err == io.EOF {
				fmt.Print("\r\n")
				return "", io.EOF
			}
			return "", err
		}
		if n == 0 {
			continue
		}

		seq := buf[:n]

		// 1. Enter / Return
		if seq[0] == '\r' || seq[0] == '\n' {
			content := string(line)
			matches := r.getMatchingCommands(content)

			// If user typed "/" exactly and pressed Enter without navigating, default to /help
			if content == "/" {
				if r.menuIndex > 0 && r.menuIndex < len(matches) {
					line = []rune(matches[r.menuIndex].Command)
				} else {
					line = []rune("/help")
				}
			}

			r.clearMenu()
			fmt.Print("\r\n")
			result := string(line)
			trimmed := strings.TrimSpace(result)
			if trimmed != "" {
				if len(r.history) == 0 || r.history[len(r.history)-1] != trimmed {
					r.history = append(r.history, trimmed)
				}
			}
			return result, nil
		}

		// 2. Ctrl+C (Interrupt / Exit)
		if seq[0] == 3 {
			r.clearMenu()
			fmt.Print("^C\r\n")
			return "/exit", nil
		}

		// 3. Ctrl+D (EOF)
		if seq[0] == 4 {
			if len(line) == 0 {
				r.clearMenu()
				fmt.Print("\r\n")
				return "/exit", nil
			}
			if cursor < len(line) {
				line = append(line[:cursor], line[cursor+1:]...)
				r.render(line, cursor)
			}
			continue
		}

		// 4. Ctrl+L (Clear Screen)
		if seq[0] == 12 {
			r.clearMenu()
			fmt.Print("\033[H\033[2J")
			r.render(line, cursor)
			continue
		}

		// 5. Ctrl+A (Home)
		if seq[0] == 1 {
			cursor = 0
			r.render(line, cursor)
			continue
		}

		// 6. Ctrl+E (End)
		if seq[0] == 5 {
			cursor = len(line)
			r.render(line, cursor)
			continue
		}

		// 7. Backspace (0x7f or 0x08)
		if seq[0] == 127 || seq[0] == 8 {
			if cursor > 0 {
				line = append(line[:cursor-1], line[cursor:]...)
				cursor--
				r.menuIndex = 0
				r.render(line, cursor)
			}
			continue
		}

		// 8. Tab Key (Auto-Completion from dropdown)
		if seq[0] == '\t' {
			content := string(line)
			matches := r.getMatchingCommands(content)
			if len(matches) > 0 {
				completed := matches[r.menuIndex].Command + " "
				line = []rune(completed)
				cursor = len(line)
				r.menuIndex = 0
				r.render(line, cursor)
			}
			continue
		}

		// 9. Escape Sequences (Arrow keys, Home, End, Delete, Esc)
		if seq[0] == 27 {
			if len(seq) == 1 {
				// Standalone Esc key -> dismiss menu
				r.clearMenu()
				continue
			}

			if seq[1] == '[' {
				if len(seq) >= 3 {
					switch seq[2] {
					case 'A': // UP Arrow
						matches := r.getMatchingCommands(string(line))
						if len(matches) > 0 {
							// Navigate dropdown menu up
							if r.menuIndex > 0 {
								r.menuIndex--
							} else {
								r.menuIndex = len(matches) - 1
							}
							r.render(line, cursor)
							continue
						}

						// History recall up
						if len(r.history) > 0 {
							if r.historyIndex == len(r.history) {
								savedCurrent = string(line)
							}
							if r.historyIndex > 0 {
								r.historyIndex--
								line = []rune(r.history[r.historyIndex])
								cursor = len(line)
								r.render(line, cursor)
							}
						}
						continue

					case 'B': // DOWN Arrow
						matches := r.getMatchingCommands(string(line))
						if len(matches) > 0 {
							// Navigate dropdown menu down
							if r.menuIndex < len(matches)-1 {
								r.menuIndex++
							} else {
								r.menuIndex = 0
							}
							r.render(line, cursor)
							continue
						}

						// History recall down
						if r.historyIndex < len(r.history) {
							r.historyIndex++
							if r.historyIndex == len(r.history) {
								line = []rune(savedCurrent)
							} else {
								line = []rune(r.history[r.historyIndex])
							}
							cursor = len(line)
							r.render(line, cursor)
						}
						continue

					case 'C': // RIGHT Arrow
						if cursor < len(line) {
							cursor++
							r.render(line, cursor)
						}
						continue

					case 'D': // LEFT Arrow
						if cursor > 0 {
							cursor--
							r.render(line, cursor)
						}
						continue

					case 'H': // HOME Key
						cursor = 0
						r.render(line, cursor)
						continue

					case 'F': // END Key
						cursor = len(line)
						r.render(line, cursor)
						continue

					case '3': // DELETE Key
						if len(seq) >= 4 && seq[3] == '~' {
							if cursor < len(line) {
								line = append(line[:cursor], line[cursor+1:]...)
								r.render(line, cursor)
							}
						}
						continue
					}
				}
			}
			continue
		}

		// 10. Printable UTF-8 Characters
		for _, rChar := range string(seq) {
			if rChar >= 32 {
				line = append(line[:cursor], append([]rune{rChar}, line[cursor:]...)...)
				cursor++
				r.menuIndex = 0
			}
		}
		r.render(line, cursor)
	}
}
