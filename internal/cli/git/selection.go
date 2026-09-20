package git

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// GitFileReadOptions configures target refs or staging modes for git reading.
type GitFileReadOptions struct {
	Staged bool   `json:"staged"`
	Commit string `json:"commit"`
	Branch string `json:"branch"`
}

// DiffReadPlan specifies how diff commands should be invoked for a given target.
type DiffReadPlan struct {
	Mode         string   `json:"mode"` // "single-diff" or "working-tree-diff"
	Args         []string `json:"args,omitempty"`
	StagedArgs   []string `json:"staged_args,omitempty"`
	UnstagedArgs []string `json:"unstaged_args,omitempty"`
}

// ContentReadPlan specifies how file contents should be read (git-show or filesystem).
type ContentReadPlan struct {
	Mode string   `json:"mode"` // "git-show" or "fs"
	Args []string `json:"args,omitempty"`
	Path string   `json:"path,omitempty"`
}

// FileSelection encapsulates targeted files and their status mappings.
type FileSelection struct {
	FilesToRead   []string          `json:"files_to_read"`
	FileStatusMap map[string]string `json:"file_status_map"`
}

// CreateFileSelectionFromPaths creates a selection from an explicit list of file paths.
func CreateFileSelectionFromPaths(files []string) FileSelection {
	statusMap := make(map[string]string, len(files))
	for _, f := range files {
		statusMap[f] = "modified"
	}
	return FileSelection{
		FilesToRead:   files,
		FileStatusMap: statusMap,
	}
}

// CreateFileSelectionFromNameStatus builds a selection from git diff --name-status output.
func CreateFileSelectionFromNameStatus(nameStatus string) FileSelection {
	return FileSelection{
		FilesToRead:   ListFilesFromNameStatus(nameStatus),
		FileStatusMap: BuildFileStatusMap(nameStatus),
	}
}

// CreateFileSelectionFromModifiedFiles creates a selection from existing FileDiff structs.
func CreateFileSelectionFromModifiedFiles(files []types.FileDiff) FileSelection {
	paths := make([]string, len(files))
	statusMap := make(map[string]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
		statusMap[f.Path] = f.Status
	}
	return FileSelection{
		FilesToRead:   paths,
		FileStatusMap: statusMap,
	}
}

// BuildFileDiffReadPlan constructs the exact git diff command arguments.
func BuildFileDiffReadPlan(filePath string, options *GitFileReadOptions) DiffReadPlan {
	if options != nil && options.Branch != "" {
		return DiffReadPlan{
			Mode: "single-diff",
			Args: []string{options.Branch + "...HEAD", "--", filePath},
		}
	}

	if options != nil && options.Commit != "" {
		return DiffReadPlan{
			Mode: "single-diff",
			Args: []string{options.Commit + "^", options.Commit, "--", filePath},
		}
	}

	if options != nil && options.Staged {
		return DiffReadPlan{
			Mode: "single-diff",
			Args: []string{"--cached", "--", filePath},
		}
	}

	return DiffReadPlan{
		Mode:         "working-tree-diff",
		StagedArgs:   []string{"--cached", "--", filePath},
		UnstagedArgs: []string{"--", filePath},
	}
}

// BuildFileContentReadPlan determines whether to read from git object db or filesystem.
func BuildFileContentReadPlan(filePath string, options *GitFileReadOptions) ContentReadPlan {
	if options != nil && options.Commit != "" {
		return ContentReadPlan{
			Mode: "git-show",
			Args: []string{options.Commit + ":" + filePath},
		}
	}

	if options != nil && options.Branch != "" {
		return ContentReadPlan{
			Mode: "git-show",
			Args: []string{"HEAD:" + filePath},
		}
	}

	return ContentReadPlan{
		Mode: "fs",
		Path: filePath,
	}
}

// DefaultExcludedPatterns returns a list of patterns ignored during code reviews.
var DefaultExcludedPatterns = []string{
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"go.sum",
	"Cargo.lock",
	"composer.lock",
	"Gemfile.lock",
	"poetry.lock",
	"vendor/**",
	"node_modules/**",
	"dist/**",
	"build/**",
	".git/**",
	".scandrix/**",
	"*.min.js",
	"*.min.css",
	"*.map",
	"*.png",
	"*.jpg",
	"*.jpeg",
	"*.gif",
	"*.svg",
	"*.ico",
	"*.woff",
	"*.woff2",
	"*.ttf",
	"*.eot",
	"*.mp4",
	"*.webm",
	"*.pdf",
	"*.zip",
	"*.tar.gz",
}

// LoadIgnorePatterns loads custom ignore patterns from .scandrixignore if it exists.
func LoadIgnorePatterns(repoRoot string) []string {
	patterns := append([]string{}, DefaultExcludedPatterns...)

	ignoreFile := filepath.Join(repoRoot, ".scandrixignore")
	f, err := os.Open(ignoreFile)
	if err != nil {
		return patterns
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns
}

// ShouldIgnoreFile checks whether a given file path matches any exclusion pattern.
func ShouldIgnoreFile(relPath string, patterns []string) bool {
	normalized := filepath.ToSlash(relPath)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)
		// Exact match or suffix match for extensions
		if pattern == normalized {
			return true
		}
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(normalized, strings.TrimPrefix(pattern, "*")) {
			return true
		}
		// Directory wildcard: vendor/** matches vendor/foo.go
		if strings.HasSuffix(pattern, "/**") {
			dirPrefix := strings.TrimSuffix(pattern, "/**") + "/"
			if strings.HasPrefix(normalized, dirPrefix) || normalized == strings.TrimSuffix(pattern, "/**") {
				return true
			}
		}
		// Match glob
		if matched, err := filepath.Match(pattern, normalized); err == nil && matched {
			return true
		}
	}
	return false
}

// FilterReviewTargets filters a list of files against ignore patterns and deleted statuses.
func FilterReviewTargets(files []string, statusMap map[string]string, patterns []string) []string {
	filtered := make([]string, 0, len(files))
	for _, f := range files {
		if statusMap != nil && statusMap[f] == "deleted" {
			continue
		}
		if ShouldIgnoreFile(f, patterns) {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}
