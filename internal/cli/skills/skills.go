// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundledSkill defines an embedded AI assistant skill definition.
type BundledSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Filename    string `json:"filename"`
	Content     string `json:"content"`
}

// BundledSkillsCatalog returns all embedded skills for Cursor, Claude Code, Codex, and AGY agents.
func BundledSkillsCatalog() []BundledSkill {
	return []BundledSkill{
		{
			Name:        "scandrix-review",
			Description: "Run local ScanDrix code review for workspace changes, gate commit/merge readiness, and auto-fix findings",
			Filename:    "scandrix-review.md",
			Content: `---
name: scandrix-review
description: Use when the user wants ScanDrix to review local changes, run scandrix review or --prompt-only, fix review findings, or check commit/push/merge readiness.
---

# ScanDrix Review
1. Run 'scandrix review --prompt-only' (or add --staged, --branch <name>, --commit <sha>).
2. For fast deterministic checks add '--fast'; for deep multi-critic LLM pass add '--heavy'.
3. To automatically apply suggested remediation diffs, run 'scandrix review --staged --fix'.
4. Make targeted changes to resolve findings before committing or pushing.
`,
		},
		{
			Name:        "scandrix-review-dev",
			Description: "Run local ScanDrix CLI against a local development API server (e.g. localhost:8080)",
			Filename:    "scandrix-review-dev.md",
			Content: `---
name: scandrix-review-dev
description: Use when the user explicitly asks to run ScanDrix CLI against a local development API server (e.g. localhost:8080, custom SCANDRIX_API_URL).
---

# ScanDrix Review (Dev Mode)
1. Ensure the local backend server is running on http://localhost:8080.
2. Run 'scandrix review --server http://localhost:8080 --prompt-only'.
`,
		},
		{
			Name:        "scandrix-business-rules-validation",
			Description: "Validate local diff changes against task requirements, acceptance criteria, or business rules",
			Filename:    "scandrix-business-rules-validation.md",
			Content: `---
name: scandrix-business-rules-validation
description: Use when the user wants ScanDrix to validate local diff changes against task requirements, acceptance criteria, or business rules via scandrix pr business-validation.
---

# ScanDrix Business Rules Validation
1. Choose local scope: default working tree diff, '--staged', '--branch <name>', or file list.
2. Run 'scandrix pr business-validation --staged --task-id <id>' (e.g. Jira/Linear key).
3. Review compliance score and address unmet acceptance criteria.
`,
		},
		{
			Name:        "scandrix-pr-suggestions-resolver",
			Description: "Fetch and automatically apply review suggestions for an existing remote pull request",
			Filename:    "scandrix-pr-suggestions-resolver.md",
			Content: `---
name: scandrix-pr-suggestions-resolver
description: Use when the user wants to fetch, triage, or implement ScanDrix suggestions for an existing remote pull request via scandrix pr suggestions.
---

# ScanDrix PR Suggestions Resolver
1. Run 'scandrix pr suggestions --pr-url <url>' (or '--pr-number <n>').
2. Triage suggestions against PR goals, prioritizing security and reliability.
3. Apply fixes incrementally, run test suites, and report outcomes.
`,
		},
		{
			Name:        "scandrix-centralized-config",
			Description: "Manage organization-wide centralized security and quality configurations",
			Filename:    "scandrix-centralized-config.md",
			Content: `---
name: scandrix-centralized-config
description: Use when the user wants to manage centralized configuration via scandrix config centralized commands.
---

# ScanDrix Centralized Configuration
1. Inspect status: 'scandrix config centralized status'.
2. Sync organization rules: 'scandrix config centralized sync'.
3. Track repositories: 'scandrix config remote add <owner/repo>'.
4. Disable centralized config: 'scandrix config centralized disable'.
`,
		},
		{
			Name:        "scandrix-rules",
			Description: "Create, update, view, and test custom ScanDrix rules",
			Filename:    "scandrix-rules.md",
			Content: `---
name: scandrix-rules
description: Use when the user wants to create, update, view, or sync ScanDrix organization rules via scandrix rules commands.
---

# ScanDrix Custom Rules
1. Initialize starter rules: 'scandrix rules init'.
2. Create custom rule: 'scandrix rules create "<Title>" --pattern "<Regex>" --severity HIGH'.
3. View active rules: 'scandrix rules list'.
4. Sync remote organization rules: 'scandrix rules sync'.
5. Validate YAML syntax: 'scandrix rules validate'.
`,
		},
		{
			Name:        "scandrix-trace",
			Description: "Session decision memory and architectural rationale recall",
			Filename:    "scandrix-trace.md",
			Content: `---
name: scandrix-trace
description: Use when about to edit files in an area you have not touched yet, or when the user asks why code is the way it is. Reads decisions already recorded via scandrix trace <paths>.
---

# ScanDrix Trace
1. Before modifying unfamiliar packages, run 'scandrix trace <path>' to recall architectural tradeoffs.
2. Pin critical decisions into future context: 'scandrix trace pin <id>'.
3. Prune outdated decisions: 'scandrix trace forget <id>'.
4. Launch local decision cockpit: 'scandrix trace ui'.
`,
		},
		{
			Name:        "hunk-review",
			Description: "Drive live Hunk terminal diff review sessions: navigate, reload, and comment",
			Filename:    "hunk-review.md",
			Content: `---
name: hunk-review
description: Interacts with live Hunk diff review sessions via CLI. Inspects review focus, navigates files and hunks, reloads session contents, and adds inline review comments.
---

# Hunk Review
1. Find live sessions: 'hunk session list --json'.
2. Inspect structure: 'hunk session review --repo . --json'.
3. Navigate diff focus: 'hunk session navigate --repo . --file <file> --hunk <n>'.
4. Add inline comment: 'hunk session comment add --repo . --file <file> --new-line <n> --summary "<note>"'.
`,
		},
	}
}

// TargetDirectory represents a local agent skills folder.
type TargetDirectory struct {
	Label   string
	Path    string
	BaseDir string
}

// SkillSyncResult aggregates file creation and update metrics.
type SkillSyncResult struct {
	CreatedCount   int      `json:"created_count"`
	UpdatedCount   int      `json:"updated_count"`
	UnchangedCount int      `json:"unchanged_count"`
	RemovedCount   int      `json:"removed_count"`
	Targets        []string `json:"targets"`
}

// ListBundledSkills returns the names of all embedded skills.
func ListBundledSkills() []string {
	catalog := BundledSkillsCatalog()
	names := make([]string, 0, len(catalog))
	for _, s := range catalog {
		names = append(names, fmt.Sprintf("%s — %s", s.Name, s.Description))
	}
	return names
}

// GeneratePromptXML formats all bundled skills into an XML payload for system prompts.
func GeneratePromptXML() string {
	catalog := BundledSkillsCatalog()
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range catalog {
		b.WriteString(fmt.Sprintf("  <skill name=\"%s\">\n", s.Name))
		b.WriteString(fmt.Sprintf("    <description>%s</description>\n", s.Description))
		b.WriteString(fmt.Sprintf("    <file>%s</file>\n", s.Filename))
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// GeneratePromptJSON formats all bundled skills as a JSON array.
func GeneratePromptJSON() string {
	catalog := BundledSkillsCatalog()
	data, _ := json.MarshalIndent(catalog, "", "  ")
	return string(data)
}

// DetectTargetDirectories identifies agent configuration directories in the workspace.
func DetectTargetDirectories(workDir string) []TargetDirectory {
	if workDir == "" {
		workDir = "."
	}

	targets := make([]TargetDirectory, 0)

	// 1. Cursor rules directory (.cursor/rules/)
	targets = append(targets, TargetDirectory{
		Label:   "Cursor Rules",
		Path:    filepath.Join(workDir, ".cursor", "rules"),
		BaseDir: filepath.Join(workDir, ".cursor"),
	})

	// 2. Claude Code agent directory (.claude/)
	targets = append(targets, TargetDirectory{
		Label:   "Claude Code",
		Path:    filepath.Join(workDir, ".claude"),
		BaseDir: filepath.Join(workDir, ".claude"),
	})

	// 3. AGY / Antigravity agent skills (.agents/skills/)
	targets = append(targets, TargetDirectory{
		Label:   "AGY Skills",
		Path:    filepath.Join(workDir, ".agents", "skills"),
		BaseDir: filepath.Join(workDir, ".agents"),
	})

	return targets
}

// Install synchronizes bundled skills into detected agent directories.
func Install(workDir string, dryRun bool) (*SkillSyncResult, error) {
	targets := DetectTargetDirectories(workDir)
	catalog := BundledSkillsCatalog()

	res := &SkillSyncResult{
		Targets: make([]string, 0),
	}

	for _, target := range targets {
		res.Targets = append(res.Targets, target.Label)
		if !dryRun {
			_ = os.MkdirAll(target.Path, 0755)
		}

		for _, skill := range catalog {
			destFile := filepath.Join(target.Path, skill.Filename)
			existing, err := os.ReadFile(destFile)
			if err != nil {
				// File does not exist -> Create
				res.CreatedCount++
				if !dryRun {
					_ = os.WriteFile(destFile, []byte(skill.Content), 0644)
				}
			} else if string(existing) == skill.Content {
				res.UnchangedCount++
			} else {
				res.UpdatedCount++
				if !dryRun {
					_ = os.WriteFile(destFile, []byte(skill.Content), 0644)
				}
			}
		}
	}

	return res, nil
}

// Uninstall removes managed skills from detected agent directories.
func Uninstall(workDir string, dryRun bool) (*SkillSyncResult, error) {
	targets := DetectTargetDirectories(workDir)
	catalog := BundledSkillsCatalog()

	res := &SkillSyncResult{
		Targets: make([]string, 0),
	}

	for _, target := range targets {
		res.Targets = append(res.Targets, target.Label)
		for _, skill := range catalog {
			destFile := filepath.Join(target.Path, skill.Filename)
			if _, err := os.Stat(destFile); err == nil {
				res.RemovedCount++
				if !dryRun {
					_ = os.Remove(destFile)
				}
			}
		}
	}

	return res, nil
}
