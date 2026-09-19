// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: file_patterns.go
// ═══════════════════════════════════════════════════════════════

package utils

import (
	"path"
	"path/filepath"
	"strings"
)

// RuleFilePatterns lists the recognized IDE and repo rule files.
var RuleFilePatterns = []string{
	// Cursor
	".cursorrules",
	".cursor/rules/**/*.mdc",
	".cursor/rules/**/*.md",

	// GitHub Copilot
	".github/copilot-instructions.md",
	".github/instructions/**/*.instructions.md",

	// Agentic
	"AGENTS.md",
	".agents.md",
	".agent.md",
	".agents/rules/**",

	// Claude
	"CLAUDE.md",
	".claude/settings.json",

	// Windsurf
	".windsurfrules",

	// Sourcegraph Cody
	".sourcegraph/**/*.rule.md",

	// OpenCode
	".opencode.json",

	// Aider
	".aider.conf.yml",
	".aiderignore",

	// ScanDrix / generic repo rules
	".drixy/rules/**",
	".drixy/rules.yaml",
	".drixy/rules.yml",
	".drixy/rules.json",
	".rules/**/*",
	"rules/**/*.md",
	"docs/coding-standards/**/*",
}

// RuleFileDiscoveryPatterns expands root patterns to also discover nested per-directory rule files in monorepos.
var RuleFileDiscoveryPatterns = func() []string {
	res := make([]string, 0, len(RuleFilePatterns)*2)
	res = append(res, RuleFilePatterns...)
	for _, p := range RuleFilePatterns {
		res = append(res, "**/" + p)
	}
	return res
}()

// IsIdeRuleSource checks whether the given path corresponds to a known IDE rule source file.
func IsIdeRuleSource(sourcePath string) bool {
	if sourcePath == "" {
		return false
	}
	clean := filepath.ToSlash(sourcePath)
	lower := strings.ToLower(clean)

	for _, p := range RuleFilePatterns {
		if strings.HasSuffix(lower, strings.ToLower(p)) || strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
		if matched, _ := filepath.Match(p, clean); matched {
			return true
		}
		if matched, _ := filepath.Match(p, filepath.Base(clean)); matched {
			return true
		}
	}
	return false
}

// SplitRulePathGlobs splits a comma-joined rule path into individual glob patterns, respecting brace expansion.
func SplitRulePathGlobs(rulePath string) []string {
	var globs []string
	var current strings.Builder
	braceDepth := 0
	bracketDepth := 0
	parenDepth := 0
	escaped := false

	for _, r := range rulePath {
		if escaped {
			escaped = false
			current.WriteRune(r)
			continue
		}
		if r == '\\' {
			escaped = true
			current.WriteRune(r)
			continue
		}

		if r == '{' && bracketDepth == 0 {
			braceDepth++
		} else if r == '}' && braceDepth > 0 && bracketDepth == 0 {
			braceDepth--
		} else if r == '[' && bracketDepth == 0 {
			bracketDepth++
		} else if r == ']' && bracketDepth > 0 {
			bracketDepth--
		} else if r == '(' && bracketDepth == 0 {
			parenDepth++
		} else if r == ')' && parenDepth > 0 && bracketDepth == 0 {
			parenDepth--
		}

		if r == ',' && braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
			trimmed := strings.TrimSpace(current.String())
			if trimmed != "" {
				globs = append(globs, trimmed)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}

	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		globs = append(globs, trimmed)
	}
	return globs
}

// ExtractRepoSubdirFromIdeSource returns the repository sub-directory prefix for a nested rule file.
func ExtractRepoSubdirFromIdeSource(sourceFilePath string) string {
	if sourceFilePath == "" {
		return ""
	}
	clean := filepath.ToSlash(sourceFilePath)
	dir := path.Dir(clean)
	if dir == "." || dir == "/" || dir == "" {
		return ""
	}

	markers := []string{
		".cursor/rules",
		".cursor",
		".github/instructions",
		".github",
		".agents/rules",
		".agents",
		".drixy/rules",
		".drixy",
		".rules",
		"rules",
		"docs/coding-standards",
	}

	for _, marker := range markers {
		if dir == marker {
			return ""
		}
		if strings.HasSuffix(dir, "/"+marker) {
			trimmed := strings.TrimSuffix(dir, "/"+marker)
			return strings.TrimPrefix(trimmed, "/")
		}
	}

	return dir
}

// ValidateAndScopeIdeRulePath ensures rule path globs target actual code rather than the rule file itself.
func ValidateAndScopeIdeRulePath(llmPath, sourceFilePath string) string {
	subdir := ExtractRepoSubdirFromIdeSource(sourceFilePath)
	subdirGlob := "**/*"
	if subdir != "" {
		subdirGlob = subdir + "/**/*"
	}

	trimmed := strings.TrimSpace(llmPath)
	if trimmed == "" || trimmed == sourceFilePath {
		return subdirGlob
	}

	// If the path targets the rule definitions directory, redirect to source subdir
	if strings.Contains(trimmed, ".cursor") || strings.Contains(trimmed, ".drixy") || strings.Contains(trimmed, ".agents") {
		return subdirGlob
	}

	if trimmed == "**/*" && subdir != "" {
		return subdirGlob
	}

	return trimmed
}

// FileMatchesRulePath tests whether a given repository file path matches a rule glob pattern.
func FileMatchesRulePath(filePath, pattern string) bool {
	cleanFile := filepath.ToSlash(filePath)
	globs := SplitRulePathGlobs(pattern)
	if len(globs) == 0 {
		return true
	}

	for _, g := range globs {
		g = filepath.ToSlash(strings.TrimSpace(g))
		if g == "" || g == "**/*" || g == "*" {
			return true
		}
		if g == cleanFile {
			return true
		}
		if strings.HasSuffix(g, "/") && strings.HasPrefix(cleanFile, g) {
			return true
		}
		if matched, err := filepath.Match(g, cleanFile); err == nil && matched {
			return true
		}
		if matched, err := filepath.Match(g, filepath.Base(cleanFile)); err == nil && matched {
			return true
		}
	}
	return false
}
