// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/scandrix/backend/internal/llm"
)

// SlashCommand represents an interactive CLI chat command.
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(ctx context.Context, session *InteractiveREPL, args []string) error
}

// InteractiveREPL manages enhanced REPL state, slash commands, and history persistence.
type InteractiveREPL struct {
	workspaceRoot string
	engine        *AgentEngine
	registry      *ToolRegistry
	history       []llm.ChatMessage
	commands      map[string]SlashCommand
	historyFile   string
	activePersona string
}

// NewInteractiveREPL creates the REPL controller with registered slash commands.
func NewInteractiveREPL(workspaceRoot string, engine *AgentEngine, registry *ToolRegistry) *InteractiveREPL {
	home, _ := os.UserHomeDir()
	historyDir := filepath.Join(home, ".scandrix")
	_ = os.MkdirAll(historyDir, 0700)
	historyPath := filepath.Join(historyDir, "chat_history.jsonl")

	repl := &InteractiveREPL{
		workspaceRoot: workspaceRoot,
		engine:        engine,
		registry:      registry,
		commands:      make(map[string]SlashCommand),
		historyFile:   historyPath,
		activePersona: "Staff Reviewer",
	}

	repl.registerBuiltInCommands()
	return repl
}

func (r *InteractiveREPL) registerBuiltInCommands() {
	r.commands["/help"] = SlashCommand{
		Name:        "/help",
		Description: "Show available slash commands and keybindings",
		Handler: func(ctx context.Context, session *InteractiveREPL, args []string) error {
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("Available Slash Commands:"))
			for _, cmd := range session.commands {
				fmt.Printf("  %-12s %s\n", cmd.Name, cmd.Description)
			}
			fmt.Println()
			return nil
		},
	}

	r.commands["/clear"] = SlashCommand{
		Name:        "/clear",
		Description: "Clear conversation history for the current session",
		Handler: func(ctx context.Context, session *InteractiveREPL, args []string) error {
			session.history = nil
			fmt.Println(lipgloss.NewStyle().Foreground(ColorSuccess).Render("✔ Conversation history cleared."))
			return nil
		},
	}

	r.commands["/tools"] = SlashCommand{
		Name:        "/tools",
		Description: "List all active autonomous agent tools",
		Handler: func(ctx context.Context, session *InteractiveREPL, args []string) error {
			fmt.Println()
			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("Registered Autonomous Tools:"))
			for _, t := range session.registry.All() {
				fmt.Printf("  • %-16s %s\n", t.Name(), t.Description())
			}
			fmt.Println()
			return nil
		},
	}

	r.commands["/persona"] = SlashCommand{
		Name:        "/persona",
		Description: "Switch persona: reviewer, security, performance, architect",
		Handler: func(ctx context.Context, session *InteractiveREPL, args []string) error {
			if len(args) == 0 {
				fmt.Printf("Active Persona: %s\nAvailable: reviewer, security, performance, architect\n", session.activePersona)
				return nil
			}
			choice := strings.ToLower(args[0])
			switch choice {
			case "security":
				session.activePersona = "Application Security Auditor"
			case "performance":
				session.activePersona = "Performance & Concurrency Specialist"
			case "architect":
				session.activePersona = "Staff Systems Architect"
			default:
				session.activePersona = "Staff Code Reviewer"
			}
			fmt.Println(lipgloss.NewStyle().Foreground(ColorSuccess).Render(fmt.Sprintf("✔ Active persona switched to: %s", session.activePersona)))
			return nil
		},
	}

	r.commands["/export"] = SlashCommand{
		Name:        "/export",
		Description: "Export conversation transcript to a markdown file (default: session.md)",
		Handler: func(ctx context.Context, session *InteractiveREPL, args []string) error {
			targetFile := "session.md"
			if len(args) > 0 {
				targetFile = args[0]
			}
			return session.ExportMarkdown(targetFile)
		},
	}
}

// HandleInput processes a line of user input, either routing to slash commands or the autonomous agent.
func (r *InteractiveREPL) HandleInput(ctx context.Context, input string, w io.Writer) error {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	// Check slash commands
	if strings.HasPrefix(trimmed, "/") {
		parts := strings.Fields(trimmed)
		cmdName := parts[0]
		if cmd, ok := r.commands[cmdName]; ok {
			return cmd.Handler(ctx, r, parts[1:])
		}
		fmt.Fprintf(w, "Unknown command %q. Type /help to view available commands.\n", cmdName)
		return nil
	}

	// Append user message to history
	userMsg := llm.ChatMessage{
		Role:    "user",
		Content: trimmed,
	}
	r.history = append(r.history, userMsg)
	r.persistMessage(userMsg)

	// Stream agent execution
	handler := &terminalEventHandler{writer: w}
	ans, newHistory, err := r.engine.RunAutonomousTurn(ctx, r.history, handler)
	if err != nil {
		return err
	}

	r.history = newHistory
	if ans != "" {
		r.persistMessage(llm.ChatMessage{Role: "assistant", Content: ans})
	}

	return nil
}

// ExportMarkdown writes the entire chat transcript to a formatted markdown document.
func (r *InteractiveREPL) ExportMarkdown(filePath string) error {
	var sb strings.Builder
	sb.WriteString("# ScanDrix Chat Session Transcript\n\n")
	sb.WriteString(fmt.Sprintf("Date: %s\nPersona: %s\n\n", time.Now().Format(time.RFC3339), r.activePersona))

	for _, msg := range r.history {
		if msg.Role == "system" {
			continue
		}
		roleTitle := "### 👤 User"
		if msg.Role == "assistant" {
			roleTitle = "### 🤖 ScanDrix"
		}
		sb.WriteString(fmt.Sprintf("%s\n\n%s\n\n---\n\n", roleTitle, msg.Content))
	}

	return os.WriteFile(filePath, []byte(sb.String()), 0600)
}

func (r *InteractiveREPL) persistMessage(msg llm.ChatMessage) {
	if r.historyFile == "" {
		return
	}
	data, err := json.Marshal(map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"role":      msg.Role,
		"content":   msg.Content,
	})
	if err != nil {
		return
	}

	f, err := os.OpenFile(r.historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.WriteString(string(data) + "\n")
}

type terminalEventHandler struct {
	writer io.Writer
}

func (h *terminalEventHandler) OnThinking(thought string) {
	style := lipgloss.NewStyle().Foreground(ColorShortcut).Italic(true)
	fmt.Fprintln(h.writer, style.Render("  💭 "+thought))
}

func (h *terminalEventHandler) OnToolCallStart(toolName string, args map[string]any) {
	toolStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight)
	argStr, _ := json.Marshal(args)
	fmt.Fprintf(h.writer, "  ⚡ %s(%s)...\n", toolStyle.Render(toolName), string(argStr))
}

func (h *terminalEventHandler) OnToolCallComplete(result *ToolResult) {
	status := lipgloss.NewStyle().Foreground(ColorSuccess).Render("✔ Done")
	if result.IsError {
		status = lipgloss.NewStyle().Foreground(ColorDanger).Render("✖ Failed")
	}
	fmt.Fprintf(h.writer, "     ↳ %s (%s)\n\n", status, result.Duration.Round(time.Millisecond))
}

func (h *terminalEventHandler) OnFinalAnswer(answer string) {
	fmt.Fprintln(h.writer)
	fmt.Fprintln(h.writer, lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0")).Render(answer))
	fmt.Fprintln(h.writer)
}
