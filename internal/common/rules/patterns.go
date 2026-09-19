package rules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/scandrix/backend/internal/common/glob"
)

var RuleFilePatterns = []string{
	// Cursor
	".cursorrules",
	".cursor/rules/**/*.mdc",

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

	// Generic / internal
	".rules/**/*",
	".drixy/rules/**",
	".scandrix/rules/**",
	"rules/**/*.md",
	"docs/coding-standards/**/*",
}

var RuleFileDiscoveryPatterns = func() []string {
	var pats []string
	pats = append(pats, RuleFilePatterns...)
	for _, p := range RuleFilePatterns {
		pats = append(pats, "**/" + p)
	}
	return pats
}()

func IsIDERuleSource(sourcePath string) bool {
	if sourcePath == "" {
		return false
	}
	return glob.IsFileMatchingGlobCaseInsensitive(sourcePath, RuleFileDiscoveryPatterns)
}

var IDERuleDirMarkers = func() []string {
	markerMap := make(map[string]bool)
	reWildcard := regexp.MustCompile(`[*?[]`)
	for _, pat := range RuleFilePatterns {
		idx := reWildcard.FindStringIndex(pat)
		fixedPrefix := pat
		if idx != nil {
			fixedPrefix = pat[:idx[0]]
		}
		var dir string
		if strings.HasSuffix(fixedPrefix, "/") {
			dir = strings.TrimSuffix(fixedPrefix, "/")
		} else {
			dir = filepath.ToSlash(filepath.Dir(fixedPrefix))
		}
		if dir != "" && dir != "." {
			markerMap[dir] = true
		}
	}
	var markers []string
	for m := range markerMap {
		markers = append(markers, m)
	}
	sort.Slice(markers, func(i, j int) bool {
		return len(markers[i]) > len(markers[j])
	})
	return markers
}()

func ExtractRepoSubdirFromIDESource(sourceFilePath string) *string {
	if sourceFilePath == "" {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(sourceFilePath))
	if dir == "" || dir == "." {
		return nil
	}

	for _, marker := range IDERuleDirMarkers {
		if dir == marker {
			return nil
		}
		if strings.HasSuffix(dir, "/"+marker) {
			stripped := dir[:len(dir)-len(marker)-1]
			if stripped != "" {
				return &stripped
			}
			return nil
		}
	}

	return &dir
}

func SplitRulePathGlobs(rulePath string) []string {
	var globs []string
	var current strings.Builder
	braceDepth := 0
	bracketDepth := 0
	parenDepth := 0
	escaped := false

	for i := 0; i < len(rulePath); i++ {
		char := rulePath[i]
		if escaped {
			escaped = false
			current.WriteByte(char)
			continue
		}
		if char == '\\' {
			escaped = true
			current.WriteByte(char)
			continue
		}

		if char == '{' && bracketDepth == 0 {
			braceDepth++
		} else if char == '}' && braceDepth > 0 && bracketDepth == 0 {
			braceDepth--
		} else if char == '[' && bracketDepth == 0 {
			bracketDepth++
		} else if char == ']' && bracketDepth > 0 {
			bracketDepth--
		} else if char == '(' && bracketDepth == 0 {
			parenDepth++
		} else if char == ')' && parenDepth > 0 && bracketDepth == 0 {
			parenDepth--
		}

		if char == ',' && braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
			str := strings.TrimSpace(current.String())
			if str != "" {
				globs = append(globs, str)
			}
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}

	str := strings.TrimSpace(current.String())
	if str != "" {
		globs = append(globs, str)
	}

	return globs
}

func PathMatchesIDERuleDir(candidatePath string) bool {
	if candidatePath == "" {
		return false
	}
	globs := SplitRulePathGlobs(candidatePath)
	if len(globs) == 0 {
		return false
	}

	rePrefix := regexp.MustCompile(`[*?[{(]`)
	reTrailingMod := regexp.MustCompile(`[@!+]$`)

	for _, g := range globs {
		idx := rePrefix.FindStringIndex(g)
		fixedPrefix := g
		if idx != nil {
			fixedPrefix = g[:idx[0]]
		}
		fixedPrefix = reTrailingMod.ReplaceAllString(fixedPrefix, "")
		var dir string
		if strings.HasSuffix(fixedPrefix, "/") {
			dir = strings.TrimSuffix(fixedPrefix, "/")
		} else {
			dir = filepath.ToSlash(filepath.Dir(fixedPrefix))
		}
		if dir == "" || dir == "." {
			continue
		}
		for _, marker := range IDERuleDirMarkers {
			if dir == marker || strings.HasSuffix(dir, "/"+marker) {
				return true
			}
		}
	}
	return false
}

type ValidatedRulePathReason string

const (
	ReasonAcceptedAsIs    ValidatedRulePathReason = "accepted-as-is"
	ReasonAcceptedScoped  ValidatedRulePathReason = "accepted-scoped"
	ReasonRejectedIdePath ValidatedRulePathReason = "rejected-ide-path"
	ReasonRejectedEmpty   ValidatedRulePathReason = "rejected-empty"
)

type ValidatedRulePath struct {
	Path            string                  `json:"path"`
	Reason          ValidatedRulePathReason `json:"reason"`
	OriginalLlmPath *string                 `json:"originalLlmPath,omitempty"`
}

func ValidateAndScopeIDERulePath(llmPath, sourceFilePath, pathSource string) ValidatedRulePath {
	reGlob := regexp.MustCompile(`[*?[]`)
	sourceLooksLikeGlob := reGlob.MatchString(sourceFilePath)
	var subdir *string
	if !sourceLooksLikeGlob {
		subdir = ExtractRepoSubdirFromIDESource(sourceFilePath)
	}

	subdirGlob := "**/*"
	if subdir != nil && *subdir != "" {
		subdirGlob = *subdir + "/**/*"
	}

	isEmpty := strings.TrimSpace(llmPath) == "" || strings.TrimSpace(llmPath) == sourceFilePath
	if isEmpty {
		return ValidatedRulePath{
			Path:            subdirGlob,
			Reason:          ReasonRejectedEmpty,
			OriginalLlmPath: &llmPath,
		}
	}

	if PathMatchesIDERuleDir(llmPath) {
		return ValidatedRulePath{
			Path:            subdirGlob,
			Reason:          ReasonRejectedIdePath,
			OriginalLlmPath: &llmPath,
		}
	}

	if llmPath == "**/*" && pathSource != "declared" {
		if subdir != nil && *subdir != "" {
			return ValidatedRulePath{
				Path:            subdirGlob,
				Reason:          ReasonAcceptedScoped,
				OriginalLlmPath: &llmPath,
			}
		}
	}

	return ValidatedRulePath{
		Path:            llmPath,
		Reason:          ReasonAcceptedAsIs,
		OriginalLlmPath: &llmPath,
	}
}

func FileMatchesRulePath(filePath, pattern string) bool {
	if filePath == pattern {
		return true
	}
	globs := SplitRulePathGlobs(pattern)
	for _, g := range globs {
		if filePath == g {
			return true
		}
		if strings.HasSuffix(g, "/") && strings.HasPrefix(filePath, g) {
			return true
		}
		if glob.IsFileMatchingGlob(filePath, []string{g}) {
			return true
		}
	}
	return false
}

func MatchFileAgainstRule(filePath, patternsStr string) bool {
	globs := SplitRulePathGlobs(patternsStr)
	matched := false
	for _, g := range globs {
		if strings.HasPrefix(g, "!") {
			neg := strings.TrimPrefix(g, "!")
			if glob.IsFileMatchingGlob(filePath, []string{neg}) {
				return false
			}
		} else {
			if glob.IsFileMatchingGlob(filePath, []string{g}) {
				matched = true
			}
		}
	}
	return matched
}


