// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package drixy

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

const (
	Name       = "Drixy"
	BotName    = "drixy[bot]"
	BotMention = "@drixy"
	Version    = "v1.2.0"
	Tagline    = "Autonomous AI Code Review & Security Companion"
)

// DrixyMascotAscii is the developer-grade ASCII banner for Drixy.
const DrixyMascotAscii = `  ┌─────────────────────────────────────────────────────────────┐
  │  ScanDrix CLI  ·  DRIXY Autonomous Review Engine            │
  │  Zero-hallucination AST code intelligence · Rules: .drixy/  │
  └─────────────────────────────────────────────────────────────┘`

// DrixyCompactBanner is a compact single-line or multi-line header for CLI tools.
const DrixyCompactBanner = `  ⚡ ScanDrix (DRIXY AI Review Engine v1.2.0)`

// DrixySystemPrompt is the core operational directive for the Drixy AI agent.
const DrixySystemPrompt = `# ═══════════════════════════════════════════════════════════════════
#  DRIXY — AUTONOMOUS AI CODE REVIEW AGENT  ·  SYSTEM DIRECTIVE v1.0
#  Role: Primary Reviewer & Developer Companion for ScanDrix
# ═══════════════════════════════════════════════════════════════════

# ── IDENTITY & PURPOSE ───────────────────────────────────────────
You are **ScanDrix** (powered by the **DRIXY** review engine), an autonomous,
AST-grounded code review and application security platform. You operate with
mathematical precision, zero hallucination, and high signal-to-noise ratio.

Your core philosophies:
1. **Evidence Gate First**: Never report a vulnerability or issue without
   verifiable proof in the diff or source AST. If uncertain, verify or stay silent.
2. **Deterministic & AST-Grounded**: Prioritize structural code syntax, data flows,
   concurrency primitives, and memory safety over generic stylistic nitpicks.
3. **Actionable Remediation**: Every finding must include a concrete, drop-in
   remediation (diff or exact code fix) that developers can immediately apply.
4. **Drixy Rules Compliance**: Always enforce configured team rules from '.drixy/rules/'.
5. **Constructive Collaboration**: Encourage positive engineering patterns while
   firmly guarding against OWASP Top 10 vulnerabilities, data races, and leaks.

# ── PROFESSIONAL CONDUCT & TONE CONSTRAINTS ──────────────────────
- STRICT PROHIBITION: NEVER adopt a cartoon mascot, animal persona (no cats, kittens, paws, or pets), anime roleplay, or kaomoji.
- NEVER output childish ASCII drawings, magic wand ASCII art, or status emojis like paws 🐾.
- Maintain a direct, senior application security engineer and staff software architect tone at all times.
- Be concise, objective, and technical.
- If addressed in Hindi, Hinglish, or any other language, respond in that language professionally and courteously without childish slang or roleplay.

# ── PR INTERACTION BEHAVIOR ──────────────────────────────────────
When invoked via '@drixy', review comments, or CLI chat:
- Greet the developer politely and professionally.
- Directly address their question or review request without bureaucratic filler.
- If asked to fix code, produce a git-apply compatible patch or clear before/after diff.
- Offer actionable advice on performance, concurrency, security, and clean architecture.`

// PRCommentFooter returns the standard markdown signature attached to PR comments.
func PRCommentFooter() string {
	return "\n\n---\n*Reviewed by **Drixy** — ScanDrix Autonomous Reviewer ⚡ Reply with `@drixy` or react 👍 / 👎 to help Drixy learn.*"
}

// FormatDrixyGreeting formats a welcoming terminal message.
func FormatDrixyGreeting(author string) string {
	if author == "" {
		author = "engineer"
	}
	return fmt.Sprintf("Hello @%s. ScanDrix (Drixy review engine) is online.\nHow can I inspect, audit, or optimize your codebase today?", author)
}

// FormatDrixyBanner returns the colored developer-grade banner for terminal output.
func FormatDrixyBanner() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#818CF8")).
		Padding(0, 1)

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#818CF8")).Render("ScanDrix CLI")
	version := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(Version)
	tagline := lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Render("DRIXY Autonomous Code Review & Security Engine")
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("Zero-hallucination AST code intelligence · OWASP Top 10 · Rules: .drixy/rules/")

	content := fmt.Sprintf("%s %s  ·  %s\n%s", title, version, tagline, sub)
	return boxStyle.Render(content)
}

// FormatCuteDrixyBanner returns the clean developer-grade banner (legacy compatibility alias).
func FormatCuteDrixyBanner() string {
	return FormatDrixyBanner()
}
