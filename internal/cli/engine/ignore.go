package engine

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
)

var defaultIgnorePatterns = []string{
	// Directories
	"node_modules",
	"vendor",
	".git",
	"dist",
	"build",
	"out",
	".next",
	"coverage",
	".turbo",
	".cache",
	// Lockfiles
	"pnpm-lock.yaml",
	"package-lock.json",
	"yarn.lock",
	"go.sum",
	"Cargo.lock",
	"composer.lock",
	"poetry.lock",
	"Gemfile.lock",
	// Minified & bundled assets
	"*.min.js",
	"*.min.css",
	"*.bundle.js",
	"*.map",
	// Binaries & archives
	"*.wasm",
	"*.exe",
	"*.bin",
	"*.so",
	"*.dylib",
	"*.dll",
	"*.zip",
	"*.tar.gz",
	"*.png",
	"*.jpg",
	"*.jpeg",
	"*.gif",
	"*.svg",
	"*.ico",
	"*.pdf",
}

// IgnoreEngine checks whether file paths match default or custom .scandrixignore rules.
type IgnoreEngine struct {
	patterns []string
}

// NewIgnoreEngine loads default ignore patterns and custom patterns from .scandrixignore in workDir.
func NewIgnoreEngine(workDir string) *IgnoreEngine {
	patterns := append([]string{}, defaultIgnorePatterns...)

	if workDir == "" {
		workDir = "."
	}

	// Load custom patterns from .scandrixignore
	ignoreFile := filepath.Join(workDir, ".scandrixignore")
	if file, err := os.Open(ignoreFile); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
		_ = scanner.Err()
	}

	return &IgnoreEngine{patterns: patterns}
}

// ShouldIgnore returns true if the specified file path matches any ignore pattern.
func (ie *IgnoreEngine) ShouldIgnore(filePath string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(filePath))
	parts := strings.Split(cleanPath, "/")

	for _, pattern := range ie.patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		// Exact directory or filename match anywhere in path
		for _, part := range parts {
			if part == pattern {
				return true
			}
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}

		// Full path match or glob match
		if matched, _ := filepath.Match(pattern, cleanPath); matched {
			return true
		}
		if matched, _ := filepath.Match("*/"+pattern, cleanPath); matched {
			return true
		}
		if strings.HasPrefix(cleanPath, pattern+"/") || strings.HasSuffix(cleanPath, "/"+pattern) {
			return true
		}
	}

	return false
}

// FilterPatches excludes ignored files from a slice of FilePatch pointers.
func (ie *IgnoreEngine) FilterPatches(patches []*diff.FilePatch) []*diff.FilePatch {
	filtered := make([]*diff.FilePatch, 0, len(patches))
	for _, p := range patches {
		targetPath := p.NewPath
		if targetPath == "" {
			targetPath = p.OldPath
		}
		if !ie.ShouldIgnore(targetPath) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
