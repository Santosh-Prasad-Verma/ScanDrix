package codeowners

import (
	"bufio"
	"path/filepath"
	"strings"
)

// Rule represents a single CODEOWNERS line: a glob pattern and its designated owners.
type Rule struct {
	Pattern string   // e.g. "*.go", "internal/auth/**", "/docs/"
	Owners  []string // e.g. ["@backend-team", "@alice", "ops@scandrix.dev"]
}

// File represents a parsed CODEOWNERS file.
type File struct {
	Rules []Rule
}

// Parse reads raw CODEOWNERS content and returns structured rules.
// Supports GitHub/GitLab CODEOWNERS format: each line is "<pattern> <owner1> <owner2> ..."
// Lines starting with '#' are comments; blank lines are ignored.
// Rules are stored in declaration order — last matching rule wins (per GitHub semantics).
func Parse(content string) *File {
	f := &File{
		Rules: make([]Rule, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and blank lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue // Pattern without owners is invalid
		}

		rule := Rule{
			Pattern: fields[0],
			Owners:  fields[1:],
		}
		f.Rules = append(f.Rules, rule)
	}

	return f
}

// Match returns the owners responsible for a given file path.
// Per GitHub semantics, the LAST matching rule wins.
func (f *File) Match(filePath string) []string {
	if len(f.Rules) == 0 {
		return nil
	}

	var matchedOwners []string
	for _, rule := range f.Rules {
		if matchPattern(rule.Pattern, filePath) {
			matchedOwners = rule.Owners
		}
	}

	return matchedOwners
}

// MatchAll returns a deduplicated set of all owners across a list of changed file paths.
func (f *File) MatchAll(changedFiles []string) []string {
	ownerSet := make(map[string]struct{})

	for _, fp := range changedFiles {
		owners := f.Match(fp)
		for _, o := range owners {
			ownerSet[o] = struct{}{}
		}
	}

	result := make([]string, 0, len(ownerSet))
	for o := range ownerSet {
		result = append(result, o)
	}
	return result
}

// matchPattern checks if a file path matches a CODEOWNERS glob pattern.
func matchPattern(pattern, filePath string) bool {
	// Normalize: strip leading "/" for comparison
	cleanPattern := strings.TrimPrefix(pattern, "/")
	cleanPath := strings.TrimPrefix(filePath, "/")

	// Handle directory patterns ending with "/"
	if strings.HasSuffix(cleanPattern, "/") {
		dir := strings.TrimSuffix(cleanPattern, "/")
		return strings.HasPrefix(cleanPath, dir+"/") || cleanPath == dir
	}

	// Handle "**" recursive glob: e.g. "internal/auth/**"
	if strings.Contains(cleanPattern, "**") {
		prefix := strings.TrimSuffix(cleanPattern, "/**")
		prefix = strings.TrimSuffix(prefix, "**")
		return strings.HasPrefix(cleanPath, prefix)
	}

	// Handle extension globs: e.g. "*.go", "*.ts"
	if strings.HasPrefix(cleanPattern, "*") {
		return matchGlob(cleanPattern, filepath.Base(cleanPath))
	}

	// Handle path globs with filepath.Match
	matched, err := filepath.Match(cleanPattern, cleanPath)
	if err == nil && matched {
		return true
	}

	// Exact path match or prefix match for directories
	if cleanPath == cleanPattern || strings.HasPrefix(cleanPath, cleanPattern+"/") {
		return true
	}

	return false
}

// matchGlob wraps filepath.Match with safe error handling.
func matchGlob(pattern, name string) bool {
	matched, err := filepath.Match(pattern, name)
	return err == nil && matched
}
