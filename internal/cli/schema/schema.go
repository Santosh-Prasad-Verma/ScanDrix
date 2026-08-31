// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package schema

// CommandOptionSchema describes a CLI command flag or option.
type CommandOptionSchema struct {
	Flags       string `json:"flags"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// CommandSchema describes a CLI command and its subcommands.
type CommandSchema struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Usage       string                `json:"usage"`
	Options     []CommandOptionSchema `json:"options,omitempty"`
	Subcommands []CommandSchema       `json:"subcommands,omitempty"`
}

// GetMasterSchema returns the full JSON schema of the ScanDrix CLI command tree.
func GetMasterSchema() CommandSchema {
	return CommandSchema{
		Name:        "scandrix",
		Description: "ScanDrix CLI — Autonomous AI Code Review & Security Assurance Platform",
		Usage:       "scandrix <command> [options]",
		Options: []CommandOptionSchema{
			{Flags: "-f, --format <fmt>", Description: "Output format (terminal, json, markdown, sarif, agent, prompt)", Default: "terminal"},
			{Flags: "-o, --output <file>", Description: "Write report directly to specified file path"},
			{Flags: "-v, --verbose", Description: "Enable verbose debug logs"},
			{Flags: "-q, --quiet", Description: "Quiet mode (output errors only)"},
			{Flags: "--agent", Description: "Machine-readable JSON envelope mode for AI agents"},
		},
		Subcommands: []CommandSchema{
			{
				Name:        "review",
				Description: "Analyze local git diffs, patches, or files for security vulnerabilities and code quality",
				Usage:       "scandrix review [files...] [options]",
				Options: []CommandOptionSchema{
					{Flags: "-s, --staged", Description: "Review staged git changes only"},
					{Flags: "-b, --branch <name>", Description: "Review changes against target branch (e.g. main)"},
					{Flags: "-c, --commit <sha>", Description: "Review changes in a specific commit"},
					{Flags: "--file <path>", Description: "Review a unified .diff or .patch file"},
					{Flags: "--rules-only", Description: "Run custom & catalog rules only (skip general suggestions)"},
					{Flags: "--fast", Description: "Fast review mode with lighter checks"},
					{Flags: "--heavy", Description: "Heavy review mode with deep multi-critic verification"},
					{Flags: "--focus <area>", Description: "Steer review to specific area (e.g. 'auth and session logic')"},
					{Flags: "-i, --tui", Description: "Launch interactive Bubbletea TUI cockpit"},
					{Flags: "--fix", Description: "Automatically apply fix suggestions"},
					{Flags: "--fail-on-severity <sev>", Description: "Exit code 1 if findings meet threshold (CRITICAL, HIGH, MEDIUM)"},
					{Flags: "--server <url>", Description: "Override ScanDrix API server endpoint"},
					{Flags: "--key <key>", Description: "Pass API key (scandrix_*) or Bearer token"},
				},
			},
			{
				Name:        "diff",
				Description: "Inspect colored diff hunks, file changes, and ignore-filtered changesets",
				Usage:       "scandrix diff [options]",
				Options: []CommandOptionSchema{
					{Flags: "--staged", Description: "View staged git diff"},
					{Flags: "--branch <name>", Description: "View diff against target branch"},
					{Flags: "--commit <sha>", Description: "View commit diff"},
				},
			},
			{
				Name:        "auth",
				Description: "Authenticate terminal, configure team API keys, and manage session credentials",
				Usage:       "scandrix auth <login|logout|status|token|team-key|team-status>",
				Subcommands: []CommandSchema{
					{Name: "login", Description: "Authenticate via RFC 8628 browser device flow or legacy password"},
					{Name: "logout", Description: "Remove local authentication and stored credentials"},
					{Name: "status", Description: "Show authentication state, active identity, and token status"},
					{Name: "token", Description: "Display Bearer token for CI/CD pipelines"},
					{Name: "team-key", Description: "Configure workspace/team API key (--key <key>)"},
					{Name: "team-status", Description: "Check team API key validation status"},
				},
			},
			{
				Name:        "config",
				Description: "Manage local and remote repository settings and centralized organization rules",
				Usage:       "scandrix config <show|remote|centralized> [options]",
				Subcommands: []CommandSchema{
					{Name: "show", Description: "Show merged global and repo configuration"},
					{Name: "remote", Description: "Manage remote tracked repositories (add, list, show, setup, set)"},
					{Name: "centralized", Description: "Manage centralized repository configurations (status, init, sync, disable, download)"},
				},
			},
			{
				Name:        "hook",
				Description: "Manage Git pre-commit and pre-push automated review guards",
				Usage:       "scandrix hook <install|uninstall|status> [options]",
				Options: []CommandOptionSchema{
					{Flags: "--pre-commit", Description: "Install pre-commit review guard"},
					{Flags: "--pre-push", Description: "Install pre-push review guard"},
					{Flags: "--fail-on-severity <sev>", Description: "Block on severity (CRITICAL, HIGH, MEDIUM)"},
					{Flags: "--dry-run", Description: "Preview planned hook actions without writing files"},
				},
			},
			{
				Name:        "pr",
				Description: "Review remote pull requests, query suggestions, and post automated review comments",
				Usage:       "scandrix pr <number|url|suggestions|comment|business-validation>",
			},
			{
				Name:        "rules",
				Description: "Manage custom repository and organization security & quality rules",
				Usage:       "scandrix rules <init|create|update|view|sync|validate>",
			},
			{
				Name:        "skills",
				Description: "Inspect, install, and synchronize bundled AI assistant skills for Cursor, Claude Code, and Codex",
				Usage:       "scandrix skills <list|install|sync|resync|uninstall>",
			},
			{
				Name:        "trace",
				Description: "Developer coding telemetry, session memory recall, assistant hooks, and commit trailers",
				Usage:       "scandrix trace <paths...|enable|disable|status|forget|pin|ui|trailer|distill>",
			},
			{
				Name:        "status",
				Description: "Display consolidated developer status (Version, Auth, Repo, Hooks, Trace, Skills)",
				Usage:       "scandrix status",
			},
			{
				Name:        "schema",
				Description: "Export CLI command introspection JSON schema for AI agents and developer tooling",
				Usage:       "scandrix schema [--command <path>]",
			},
			{
				Name:        "subscribe",
				Description: "Open the ScanDrix subscription and upgrade billing page in default browser",
				Usage:       "scandrix subscribe",
			},
			{
				Name:        "update",
				Description: "Check for and apply self-updates to the ScanDrix CLI binary",
				Usage:       "scandrix update",
			},
			{
				Name:        "tui",
				Description: "Launch interactive Bubbletea terminal cockpit dashboard",
				Usage:       "scandrix tui [options]",
			},
		},
	}
}
