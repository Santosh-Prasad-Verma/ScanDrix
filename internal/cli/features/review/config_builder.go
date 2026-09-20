package review

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

const (
	MaxFiles        = 500
	MaxDiffChars    = 500000  // 500K characters, matches backend validation
	MaxContentChars = 2000000 // 2M characters
)

// ReviewPayloadConfig models review steering and inlining configuration.
type ReviewPayloadConfig struct {
	RulesOnly bool                `json:"rulesOnly,omitempty"`
	Fast      bool                `json:"fast,omitempty"`
	Focus     string              `json:"focus,omitempty"`
	Heavy     bool                `json:"heavy,omitempty"`
	Files     []types.FileContent `json:"files,omitempty"`
}

// FileContentReader is a callback function that fetches full file contents from git.
type FileContentReader func(files []string, staged bool, commit, branch string) ([]types.FileContent, error)

// BuildConfigOptions configures review payload construction.
type BuildConfigOptions struct {
	RulesOnly bool
	Fast      bool
	Focus     string
	Heavy     bool
	Quiet     bool
	Files     []string
	Staged    bool
	Commit    string
	Branch    string
	Reader    FileContentReader
}

// BuildReviewPayloadConfig constructs the remote review request payload with smart inlining.
func BuildReviewPayloadConfig(opts BuildConfigOptions) (*ReviewPayloadConfig, error) {
	cfg := &ReviewPayloadConfig{
		RulesOnly: opts.RulesOnly,
		Fast:      opts.Fast,
		Focus:     opts.Focus,
		Heavy:     opts.Heavy,
	}

	// Skip inlining file contents when:
	// - fast mode: remote sandbox has limited step budget
	// - branch / commit mode: remote worker clones the committed tree directly
	skipInlining := opts.Fast || opts.Branch != "" || opts.Commit != ""
	if skipInlining || opts.Reader == nil {
		return cfg, nil
	}

	rawFiles, err := opts.Reader(opts.Files, opts.Staged, opts.Commit, opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("failed to read full file contents: %w", err)
	}

	cfg.Files = FilterReviewFiles(rawFiles, opts.Quiet)
	return cfg, nil
}

// FilterReviewFiles validates per-file character size limits and clips excess file lists.
func FilterReviewFiles(files []types.FileContent, quiet bool) []types.FileContent {
	var filtered []types.FileContent
	var skipped []string

	for _, f := range files {
		diffLen := len(f.Diff)
		contentLen := len(f.Content)

		if diffLen > MaxDiffChars {
			skipped = append(skipped, fmt.Sprintf("  - %s (diff: %d chars, max: %d)", f.Path, diffLen, MaxDiffChars))
			continue
		}

		if contentLen > MaxContentChars {
			skipped = append(skipped, fmt.Sprintf("  - %s (content: %d chars, max: %d)", f.Path, contentLen, MaxContentChars))
			continue
		}

		filtered = append(filtered, f)
	}

	if !quiet && len(skipped) > 0 {
		fmt.Printf("⚠ Skipped %d file(s) exceeding size limits:\n%s\n", len(skipped), strings.Join(skipped, "\n"))
	}

	if len(filtered) > MaxFiles {
		if !quiet {
			fmt.Printf("⚠ Too many files (%d), sending first %d\n", len(filtered), MaxFiles)
		}
		filtered = filtered[:MaxFiles]
	}

	return filtered
}
