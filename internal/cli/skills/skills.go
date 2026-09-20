// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		{
			Name:        "scandrix-security-core",
			Description: "Real-time enterprise security guard, secret scanner, and zero-tolerance vulnerability enforcer",
			Filename:    "scandrix-security-core.md",
			Content: `---
name: scandrix-security-core
description: Real-time enterprise security guard, secret scanner, and zero-tolerance vulnerability enforcer.
---

# ScanDrix Security Core Guard
1. Zero Secret Exposure: Never commit credentials; runtime env injection only.
2. Universal Injection Prevention: Parameterized queries only; no shell string evaluation.
3. Fail-Closed Authorization: Default deny on missing context, errors, or timeouts.
4. Client Hygiene: Redact internal stack traces; generic client errors.
`,
		},
		{
			Name:        "scandrix-sec-crypto",
			Description: "Cryptographic standards, secure key management, and data encryption policies",
			Filename:    "scandrix-sec-crypto.md",
			Content: `---
name: scandrix-sec-crypto
description: Cryptographic standards, secure key management, and data encryption policies.
---

# ScanDrix Crypto Standards
1. Approved Primitives: AES-256-GCM, ChaCha20-Poly1305, Ed25519, Argon2id.
2. Disallow: MD5, SHA-1, DES, RC4, raw SHA-256 for passwords.
3. CSPRNG: Always use crypto/rand; never seedable PRNGs for tokens/IVs.
4. Nonce/IV: Never reuse an IV with the same key.
`,
		},
		{
			Name:        "scandrix-sec-infra",
			Description: "Container hardening, Dockerfiles, Kubernetes, cloud posture, and network exposure",
			Filename:    "scandrix-sec-infra.md",
			Content: `---
name: scandrix-sec-infra
description: Container hardening, Dockerfiles, Kubernetes, cloud posture, and network exposure.
---

# ScanDrix Infrastructure & Cloud Hardening
1. Non-Root Execution: Always specify USER nonroot:nonroot or numeric UID.
2. Base Images: Minimal distroless or Alpine base.
3. Read-Only Root Filesystem: readOnlyRootFilesystem: true in k8s security context.
4. Security Headers: Enforce CSP, HSTS, X-Content-Type-Options: nosniff.
`,
		},
		{
			Name:        "scandrix-sec-input-web",
			Description: "Input sanitization, schema enforcement, XSS, CSRF, and file upload protection",
			Filename:    "scandrix-sec-input-web.md",
			Content: `---
name: scandrix-sec-input-web
description: Input sanitization, schema enforcement, XSS, CSRF, and file upload protection.
---

# ScanDrix Web Security & Input Validation
1. Schemas: Validate all incoming payloads against schemas (Zod, Pydantic, JSON Schema).
2. XSS: Context-aware output encoding; no dangerouslySetInnerHTML.
3. File Uploads: Magic byte verification (libmagic); UUID filenames outside web root.
4. Cookies: HttpOnly; Secure; SameSite=Strict (SameSite=Lax for OAuth/SSO redirects).
`,
		},
		{
			Name:        "scandrix-sec-supplychain",
			Description: "Dependency management, SBOM verification, and pipeline integrity",
			Filename:    "scandrix-sec-supplychain.md",
			Content: `---
name: scandrix-sec-supplychain
description: Dependency management, SBOM verification, and pipeline integrity.
---

# ScanDrix Supply Chain Guard
1. Deterministic Builds: Commit lockfiles (go.sum, package-lock.json, pnpm-lock.yaml).
2. Auditing: Mandate govulncheck, npm audit, trivy fs.
3. Minimal Surface: Prefer standard library over external dependencies.
4. Typosquatting: Check package namespaces and verification status.
`,
		},
	}
}

// ManagedSkillsManifest is the manifest filename tracking ScanDrix-managed skills.
const ManagedSkillsManifest = ".scandrix-managed-skills.json"

const legacyBusinessRulesName = "business-rules-validation"

// TargetType defines whether the agent uses command markdown files or skill directories.
type TargetType string

const (
	TargetTypeSkill   TargetType = "skill"
	TargetTypeCommand TargetType = "command"
)

// TargetScope defines whether the target is in the project workspace or user home.
type TargetScope string

const (
	TargetScopeProject TargetScope = "project"
	TargetScopeUser    TargetScope = "user"
)

// TargetDefinition defines a discovery pattern for agent tools.
type TargetDefinition struct {
	Label              string
	Scope              TargetScope
	Type               TargetType
	ActivationSegments []string
	BaseSegments       []string
}

// DefaultTargetDefinitions enumerates all agent integration targets.
var DefaultTargetDefinitions = []TargetDefinition{
	{Label: "Codex project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".codex"}, BaseSegments: []string{".codex", "skills"}},
	{Label: "Codex user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".codex"}, BaseSegments: []string{".codex", "skills"}},
	{Label: "Claude project commands", Scope: TargetScopeProject, Type: TargetTypeCommand, ActivationSegments: []string{".claude"}, BaseSegments: []string{".claude", "commands"}},
	{Label: "Claude user commands", Scope: TargetScopeUser, Type: TargetTypeCommand, ActivationSegments: []string{".claude"}, BaseSegments: []string{".claude", "commands"}},
	{Label: "Claude config commands", Scope: TargetScopeUser, Type: TargetTypeCommand, ActivationSegments: []string{".config", "claude"}, BaseSegments: []string{".config", "claude", "commands"}},
	{Label: "Cursor project commands", Scope: TargetScopeProject, Type: TargetTypeCommand, ActivationSegments: []string{".cursor"}, BaseSegments: []string{".cursor", "commands"}},
	{Label: "Cursor user commands", Scope: TargetScopeUser, Type: TargetTypeCommand, ActivationSegments: []string{".cursor"}, BaseSegments: []string{".cursor", "commands"}},
	{Label: "Cursor project rules", Scope: TargetScopeProject, Type: TargetTypeCommand, ActivationSegments: []string{".cursor"}, BaseSegments: []string{".cursor", "rules"}},
	{Label: "Agents project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".agents"}, BaseSegments: []string{".agents", "skills"}},
	{Label: "Agents user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".agents"}, BaseSegments: []string{".agents", "skills"}},
	{Label: "Agents user skills (legacy config path)", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".config", "agents"}, BaseSegments: []string{".config", "agents", "skills"}},
	{Label: "OpenCode project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".opencode"}, BaseSegments: []string{".opencode", "skill"}},
	{Label: "OpenCode user commands", Scope: TargetScopeUser, Type: TargetTypeCommand, ActivationSegments: []string{".config", "opencode"}, BaseSegments: []string{".config", "opencode", "command"}},
	{Label: "AiderDesk project commands", Scope: TargetScopeProject, Type: TargetTypeCommand, ActivationSegments: []string{".aider-desk"}, BaseSegments: []string{".aider-desk", "commands"}},
	{Label: "AiderDesk user commands", Scope: TargetScopeUser, Type: TargetTypeCommand, ActivationSegments: []string{".aider-desk"}, BaseSegments: []string{".aider-desk", "commands"}},
	{Label: "Kilo Code project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".kilocode"}, BaseSegments: []string{".kilocode", "skills"}},
	{Label: "Kilo Code user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".kilocode"}, BaseSegments: []string{".kilocode", "skills"}},
	{Label: "Roo Code project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".roo"}, BaseSegments: []string{".roo", "skills"}},
	{Label: "Roo Code user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".roo"}, BaseSegments: []string{".roo", "skills"}},
	{Label: "Goose project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".goose"}, BaseSegments: []string{".goose", "skills"}},
	{Label: "Goose user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".config", "goose"}, BaseSegments: []string{".config", "goose", "skills"}},
	{Label: "Antigravity project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".agent"}, BaseSegments: []string{".agent", "skills"}},
	{Label: "Antigravity user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".gemini", "antigravity"}, BaseSegments: []string{".gemini", "antigravity", "skills"}},
	{Label: "Droid project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".factory"}, BaseSegments: []string{".factory", "skills"}},
	{Label: "Droid user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".factory"}, BaseSegments: []string{".factory", "skills"}},
	{Label: "Windsurf project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".windsurf"}, BaseSegments: []string{".windsurf", "skills"}},
	{Label: "Windsurf user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".codeium", "windsurf"}, BaseSegments: []string{".codeium", "windsurf", "skills"}},
	{Label: "Gemini project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".gemini"}, BaseSegments: []string{".gemini", "skills"}},
	{Label: "Kiro project skills", Scope: TargetScopeProject, Type: TargetTypeSkill, ActivationSegments: []string{".kiro"}, BaseSegments: []string{".kiro", "skills"}},
	{Label: "Kiro user skills", Scope: TargetScopeUser, Type: TargetTypeSkill, ActivationSegments: []string{".kiro"}, BaseSegments: []string{".kiro", "skills"}},
}

// TargetDirectory represents a resolved local agent target directory.
type TargetDirectory struct {
	Label          string      `json:"label"`
	Scope          TargetScope `json:"scope"`
	Type           TargetType  `json:"type"`
	ActivationPath string      `json:"activation_path"`
	Path           string      `json:"path"` // Alias for BaseDir
	BaseDir        string      `json:"base_dir"`
	IsActive       bool        `json:"is_active"`
}

// TargetSyncResult details sync operations for an individual target.
type TargetSyncResult struct {
	Target         TargetDirectory `json:"target"`
	Synced         bool            `json:"synced"`
	Created        int             `json:"created"`
	Updated        int             `json:"updated"`
	Unchanged      int             `json:"unchanged"`
	RemovedManaged int             `json:"removed_managed"`
	RemovedLegacy  int             `json:"removed_legacy"`
	Reason         string          `json:"reason,omitempty"`
}

// SkillSyncResult aggregates file creation and update metrics across all targets.
type SkillSyncResult struct {
	Results        []TargetSyncResult `json:"results"`
	ScannedTargets int                `json:"scanned_targets"`
	SyncedTargets  int                `json:"synced_targets"`
	SkippedTargets int                `json:"skipped_targets"`
	CreatedCount   int                `json:"created_count"`
	UpdatedCount   int                `json:"updated_count"`
	UnchangedCount int                `json:"unchanged_count"`
	RemovedCount   int                `json:"removed_count"`
	RemovedManaged int                `json:"removed_managed"`
	RemovedLegacy  int                `json:"removed_legacy"`
	Targets        []string           `json:"targets"`
}

// SkillCheckDetail reports the state of an individual skill in a target.
type SkillCheckDetail struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "up-to-date", "outdated", "missing"
	Path   string `json:"path"`
}

// SkillTargetStatus gives check status for an individual agent target.
type SkillTargetStatus struct {
	Target    TargetDirectory    `json:"target"`
	Status    string             `json:"status"` // "up-to-date", "outdated", "missing", "uninitialized"
	Installed int                `json:"installed"`
	UpToDate  int                `json:"up_to_date"`
	Outdated  int                `json:"outdated"`
	Missing   int                `json:"missing"`
	Stale     int                `json:"stale"`
	Skills    []SkillCheckDetail `json:"skills"`
}

// SkillCheckReport is returned by Check auditing all targets.
type SkillCheckReport struct {
	ScannedTargets int                 `json:"scanned_targets"`
	ActiveTargets  int                 `json:"active_targets"`
	HealthyTargets int                 `json:"healthy_targets"`
	Targets        []SkillTargetStatus `json:"targets"`
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

// BuildSkillSyncTargets constructs all potential target directories from cwd and homeDir.
func BuildSkillSyncTargets(cwd, homeDir string) []TargetDirectory {
	if cwd == "" {
		cwd = "."
	}
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	targets := make([]TargetDirectory, 0, len(DefaultTargetDefinitions))
	for _, def := range DefaultTargetDefinitions {
		root := cwd
		if def.Scope == TargetScopeUser {
			if homeDir == "" {
				continue
			}
			root = homeDir
		}

		actPath := filepath.Join(append([]string{root}, def.ActivationSegments...)...)
		baseDir := filepath.Join(append([]string{root}, def.BaseSegments...)...)

		targets = append(targets, TargetDirectory{
			Label:          def.Label,
			Scope:          def.Scope,
			Type:           def.Type,
			ActivationPath: actPath,
			Path:           baseDir,
			BaseDir:        baseDir,
		})
	}
	return targets
}

// DetectTargetDirectories returns project targets for the given workDir.
func DetectTargetDirectories(workDir string) []TargetDirectory {
	return BuildSkillSyncTargets(workDir, "")
}

// ResolveSkillPath returns the filesystem destination for a skill within a target.
func ResolveSkillPath(target TargetDirectory, skill BundledSkill) string {
	if target.Type == TargetTypeCommand {
		return filepath.Join(target.BaseDir, skill.Filename)
	}
	return filepath.Join(target.BaseDir, skill.Name, "SKILL.md")
}

// ResolveSkillEntryDir returns the path to the root entry of a skill in a target.
func ResolveSkillEntryDir(target TargetDirectory, skillName string) string {
	if target.Type == TargetTypeCommand {
		return filepath.Join(target.BaseDir, skillName+".md")
	}
	return filepath.Join(target.BaseDir, skillName)
}

// ResolveManagedManifestPath returns the path to the managed skills manifest for a target.
func ResolveManagedManifestPath(target TargetDirectory) string {
	return filepath.Join(target.BaseDir, ManagedSkillsManifest)
}

// ReadManagedSkillNames reads the list of managed skill names from the manifest.
func ReadManagedSkillNames(target TargetDirectory) ([]string, error) {
	manifestPath := ResolveManagedManifestPath(target)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, err
	}
	return names, nil
}

// WriteManagedSkillNames writes the list of managed skill names to the manifest.
func WriteManagedSkillNames(target TargetDirectory, skillNames []string, dryRun bool) error {
	if dryRun {
		return nil
	}

	sort.Strings(skillNames)
	manifestPath := ResolveManagedManifestPath(target)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(skillNames, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(manifestPath, data, 0644)
}

func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func isDirectory(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// Sync synchronizes bundled skills into all detected agent directories.
func Sync(workDir, homeDir string, dryRun, installMode bool) (*SkillSyncResult, error) {
	targets := BuildSkillSyncTargets(workDir, homeDir)
	catalog := BundledSkillsCatalog()

	currentSkillNames := make([]string, 0, len(catalog))
	for _, sk := range catalog {
		currentSkillNames = append(currentSkillNames, sk.Name)
	}
	currentSkillMap := make(map[string]BundledSkill, len(catalog))
	for _, sk := range catalog {
		currentSkillMap[sk.Name] = sk
	}

	res := &SkillSyncResult{
		Results: make([]TargetSyncResult, 0, len(targets)),
		Targets: make([]string, 0),
	}

	for _, target := range targets {
		hasBaseDir := isDirectory(target.BaseDir)

		if installMode && !hasBaseDir {
			// If install mode and activation root exists (or project target with empty root), create baseDir
			hasAct := isDirectory(target.ActivationPath)
			if !hasAct && target.Scope == TargetScopeProject {
				// Project target: allow install
				hasAct = true
			}
			if !hasAct {
				res.Results = append(res.Results, TargetSyncResult{
					Target: target,
					Synced: false,
					Reason: "Agent root directory not found.",
				})
				continue
			}

			if !dryRun {
				_ = os.MkdirAll(target.BaseDir, 0755)
			}
			hasBaseDir = true
		}

		if !hasBaseDir {
			res.Results = append(res.Results, TargetSyncResult{
				Target: target,
				Synced: false,
				Reason: "Target directory not found.",
			})
			continue
		}

		target.IsActive = true
		targetRes := TargetSyncResult{
			Target: target,
			Synced: true,
		}

		previouslyManaged, _ := ReadManagedSkillNames(target)
		prevSet := make(map[string]bool, len(previouslyManaged))
		for _, name := range previouslyManaged {
			prevSet[name] = true
		}

		// 1. Sync catalog skills
		for _, skill := range catalog {
			destFile := ResolveSkillPath(target, skill)
			existing, err := os.ReadFile(destFile)
			if err != nil {
				targetRes.Created++
				if !dryRun {
					_ = os.MkdirAll(filepath.Dir(destFile), 0755)
					_ = os.WriteFile(destFile, []byte(skill.Content), 0644)
				}
			} else if string(existing) == skill.Content {
				targetRes.Unchanged++
			} else {
				targetRes.Updated++
				if !dryRun {
					_ = os.MkdirAll(filepath.Dir(destFile), 0755)
					_ = os.WriteFile(destFile, []byte(skill.Content), 0644)
				}
			}
		}

		// 2. Remove stale managed skills
		for _, prevName := range previouslyManaged {
			if _, exists := currentSkillMap[prevName]; !exists {
				entryPath := ResolveSkillEntryDir(target, prevName)
				if _, err := os.Stat(entryPath); err == nil {
					targetRes.RemovedManaged++
					if !dryRun {
						_ = os.RemoveAll(entryPath)
					}
				}
			}
		}

		// 3. Remove legacy entries
		legacyPath := filepath.Join(target.BaseDir, legacyBusinessRulesName)
		if target.Type == TargetTypeCommand {
			legacyPath = filepath.Join(target.BaseDir, legacyBusinessRulesName+".md")
		}
		if _, err := os.Stat(legacyPath); err == nil {
			targetRes.RemovedLegacy++
			if !dryRun {
				_ = os.RemoveAll(legacyPath)
			}
		}

		// 4. Update managed manifest
		_ = WriteManagedSkillNames(target, currentSkillNames, dryRun)

		res.Results = append(res.Results, targetRes)
		res.Targets = append(res.Targets, target.Label)
		res.CreatedCount += targetRes.Created
		res.UpdatedCount += targetRes.Updated
		res.UnchangedCount += targetRes.Unchanged
		res.RemovedManaged += targetRes.RemovedManaged
		res.RemovedLegacy += targetRes.RemovedLegacy
		res.RemovedCount += targetRes.RemovedManaged + targetRes.RemovedLegacy
		res.SyncedTargets++
	}

	res.ScannedTargets = len(targets)
	res.SkippedTargets = res.ScannedTargets - res.SyncedTargets
	return res, nil
}

// UninstallTargets removes all managed skills and manifests from detected agent directories.
func UninstallTargets(workDir, homeDir string, dryRun bool) (*SkillSyncResult, error) {
	targets := BuildSkillSyncTargets(workDir, homeDir)
	catalog := BundledSkillsCatalog()

	res := &SkillSyncResult{
		Results: make([]TargetSyncResult, 0, len(targets)),
		Targets: make([]string, 0),
	}

	for _, target := range targets {
		if !isDirectory(target.BaseDir) {
			continue
		}

		targetRes := TargetSyncResult{
			Target: target,
			Synced: true,
		}

		managedNames, _ := ReadManagedSkillNames(target)
		seen := make(map[string]bool)
		for _, name := range managedNames {
			seen[name] = true
		}
		for _, sk := range catalog {
			seen[sk.Name] = true
		}

		for skillName := range seen {
			entryPath := ResolveSkillEntryDir(target, skillName)
			if _, err := os.Stat(entryPath); err == nil {
				targetRes.RemovedManaged++
				if !dryRun {
					_ = os.RemoveAll(entryPath)
				}
			}
		}

		// Remove legacy file
		legacyPath := filepath.Join(target.BaseDir, legacyBusinessRulesName)
		if target.Type == TargetTypeCommand {
			legacyPath = filepath.Join(target.BaseDir, legacyBusinessRulesName+".md")
		}
		if _, err := os.Stat(legacyPath); err == nil {
			targetRes.RemovedLegacy++
			if !dryRun {
				_ = os.RemoveAll(legacyPath)
			}
		}

		// Remove manifest file
		manifestPath := ResolveManagedManifestPath(target)
		if _, err := os.Stat(manifestPath); err == nil {
			if !dryRun {
				_ = os.Remove(manifestPath)
			}
		}

		res.Results = append(res.Results, targetRes)
		res.Targets = append(res.Targets, target.Label)
		res.RemovedManaged += targetRes.RemovedManaged
		res.RemovedLegacy += targetRes.RemovedLegacy
		res.RemovedCount += targetRes.RemovedManaged + targetRes.RemovedLegacy
		res.SyncedTargets++
	}

	res.ScannedTargets = len(targets)
	res.SkippedTargets = res.ScannedTargets - res.SyncedTargets
	return res, nil
}

// Check audits all detected agent targets against the bundled skills catalog.
func Check(workDir, homeDir string) (*SkillCheckReport, error) {
	targets := BuildSkillSyncTargets(workDir, homeDir)
	catalog := BundledSkillsCatalog()

	currentSkillMap := make(map[string]BundledSkill, len(catalog))
	for _, sk := range catalog {
		currentSkillMap[sk.Name] = sk
	}

	report := &SkillCheckReport{
		ScannedTargets: len(targets),
		Targets:        make([]SkillTargetStatus, 0, len(targets)),
	}

	for _, target := range targets {
		if !isDirectory(target.BaseDir) {
			continue
		}

		target.IsActive = true
		report.ActiveTargets++

		managedNames, _ := ReadManagedSkillNames(target)
		staleCount := 0
		for _, m := range managedNames {
			if _, ok := currentSkillMap[m]; !ok {
				staleCount++
			}
		}

		status := SkillTargetStatus{
			Target:  target,
			Stale:   staleCount,
			Skills:  make([]SkillCheckDetail, 0, len(catalog)),
		}

		for _, sk := range catalog {
			destFile := ResolveSkillPath(target, sk)
			detail := SkillCheckDetail{
				Name: sk.Name,
				Path: destFile,
			}

			content, err := os.ReadFile(destFile)
			if err != nil {
				detail.Status = "missing"
				status.Missing++
			} else if string(content) == sk.Content {
				detail.Status = "up-to-date"
				status.UpToDate++
				status.Installed++
			} else {
				detail.Status = "outdated"
				status.Outdated++
				status.Installed++
			}
			status.Skills = append(status.Skills, detail)
		}

		if status.Missing == 0 && status.Outdated == 0 && status.Stale == 0 {
			status.Status = "up-to-date"
			report.HealthyTargets++
		} else if status.Outdated > 0 {
			status.Status = "outdated"
		} else {
			status.Status = "missing"
		}

		report.Targets = append(report.Targets, status)
	}

	return report, nil
}

// Install synchronizes bundled skills into detected agent directories.
func Install(workDir string, dryRun bool) (*SkillSyncResult, error) {
	home, _ := os.UserHomeDir()
	return Sync(workDir, home, dryRun, true)
}

// Uninstall removes managed skills from detected agent directories.
func Uninstall(workDir string, dryRun bool) (*SkillSyncResult, error) {
	home, _ := os.UserHomeDir()
	return UninstallTargets(workDir, home, dryRun)
}

// FormatSkillsPrompt formats bundled skills as XML, JSON, or Markdown for LLM prompt context injection.
func FormatSkillsPrompt(catalog []BundledSkill, format string, selected []string) (string, error) {
	var targetSkills []BundledSkill

	if len(selected) > 0 {
		catalogMap := make(map[string]BundledSkill)
		for _, s := range catalog {
			catalogMap[strings.ToLower(s.Name)] = s
			catalogMap[strings.ToLower(s.Filename)] = s
			catalogMap[strings.ToLower(strings.TrimSuffix(s.Filename, ".md"))] = s
		}

		for _, name := range selected {
			key := strings.ToLower(strings.TrimSpace(name))
			if s, ok := catalogMap[key]; ok {
				targetSkills = append(targetSkills, s)
			} else {
				return "", fmt.Errorf("skill %q not found in bundled catalog", name)
			}
		}
	} else {
		targetSkills = make([]BundledSkill, len(catalog))
		copy(targetSkills, catalog)
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		data, err := json.MarshalIndent(targetSkills, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed marshaling skills to JSON: %w", err)
		}
		return string(data), nil

	case "xml":
		var b strings.Builder
		b.WriteString("<skills>\n")
		for _, s := range targetSkills {
			b.WriteString(fmt.Sprintf("  <skill name=%q description=%q>\n", s.Name, s.Description))
			b.WriteString("    <instructions><![CDATA[\n")
			b.WriteString(s.Content)
			if !strings.HasSuffix(s.Content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("    ]]></instructions>\n")
			b.WriteString("  </skill>\n")
		}
		b.WriteString("</skills>")
		return b.String(), nil

	default: // markdown / text
		var b strings.Builder
		b.WriteString("# ScanDrix Assistant Skills\n\n")
		for i, s := range targetSkills {
			b.WriteString(fmt.Sprintf("## %d. %s\n", i+1, s.Name))
			b.WriteString(fmt.Sprintf("> %s\n\n", s.Description))
			b.WriteString(s.Content)
			if !strings.HasSuffix(s.Content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n---\n\n")
		}
		return strings.TrimSpace(b.String()), nil
	}
}

