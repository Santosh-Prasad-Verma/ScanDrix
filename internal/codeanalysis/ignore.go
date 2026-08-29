package codeanalysis

import (
	"path/filepath"
	"strings"
)

// IgnoreMatcher evaluates file paths against standard ignore rules and file patterns.
type IgnoreMatcher struct {
	patterns []string
}

var defaultIgnoredPatterns = []string{
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"go.sum",
	"Cargo.lock",
	"Gemfile.lock",
	"composer.lock",
	"vendor/*",
	"node_modules/*",
	"dist/*",
	"build/*",
	"*.min.js",
	"*.min.css",
	"*.map",
	".git/*",
	"*.pb.go",
	"*_generated.go",
}

// NewIgnoreMatcher initializes the file ignore engine with defaults and optional custom patterns.
func NewIgnoreMatcher(customPatterns []string) *IgnoreMatcher {
	all := make([]string, 0, len(defaultIgnoredPatterns)+len(customPatterns))
	all = append(all, defaultIgnoredPatterns...)
	all = append(all, customPatterns...)
	return &IgnoreMatcher{patterns: all}
}

// ShouldIgnore determines whether a given file path should be excluded from review.
func (m *IgnoreMatcher) ShouldIgnore(filePath string) bool {
	normalized := filepath.ToSlash(filePath)
	clean := strings.TrimPrefix(normalized, "./")
	base := filepath.Base(clean)

	for _, pattern := range m.patterns {
		// Exact base match
		if match, _ := filepath.Match(pattern, base); match {
			return true
		}
		// Full path glob match
		if match, _ := filepath.Match(pattern, clean); match {
			return true
		}
		// Directory prefix check
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "/*")
			if strings.HasPrefix(clean, prefix+"/") || clean == prefix {
				return true
			}
		}
	}

	return false
}
