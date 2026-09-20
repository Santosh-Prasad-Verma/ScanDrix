// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// CharsPerToken provides heuristic estimation (1 token ≈ 4 characters).
	CharsPerToken = 4

	// PromptBudgetRatio caps estimated full prompt to 55% of the model context window.
	PromptBudgetRatio = 0.55

	// PromptStaticOverheadChars accounts for system prompt, tool schemas, and PR metadata.
	PromptStaticOverheadChars = 62000

	// PromptStaticOverheadCharsCompact accounts for trimmed system prompt in compact mode.
	PromptStaticOverheadCharsCompact = 48000
)

// LargePRAggressiveFilterPatterns drops low-signal test, style, and doc files during large reviews.
var LargePRAggressiveFilterPatterns = []string{
	"**/*.spec.*",
	"**/*.test.*",
	"**/test/**",
	"**/tests/**",
	"**/__tests__/**",
	"**/*.md",
	"**/*.css",
	"**/*.scss",
}

// ContextWindowTooSmallError is raised when model context window cannot fit base static overhead.
type ContextWindowTooSmallError struct {
	ContextWindowTokens int
	RequiredTokens      int
}

func (e *ContextWindowTooSmallError) Error() string {
	return fmt.Sprintf("model context window %d tokens is smaller than required static prompt overhead %d tokens",
		e.ContextWindowTokens, e.RequiredTokens)
}

// EstimateNonDiffOverheadTokens returns estimated token overhead for system prompt and schemas.
func EstimateNonDiffOverheadTokens(isCompact bool) int {
	chars := PromptStaticOverheadChars
	if isCompact {
		chars = PromptStaticOverheadCharsCompact
	}
	return chars / CharsPerToken
}

// AssertContextWindowFitsOverhead verifies that the context window accommodates static overhead with headroom.
func AssertContextWindowFitsOverhead(contextWindowTokens int, isCompact bool) error {
	overheadTokens := EstimateNonDiffOverheadTokens(isCompact)
	maxAllowedOverhead := int(float64(contextWindowTokens) * PromptBudgetRatio)
	if overheadTokens > maxAllowedOverhead {
		return &ContextWindowTooSmallError{
			ContextWindowTokens: contextWindowTokens,
			RequiredTokens:      overheadTokens,
		}
	}
	return nil
}

// ChangedFileReviewItem represents a file change evaluated for token budgeting and filtering.
type ChangedFileReviewItem struct {
	Filename  string
	Content   string
	Patch     string
	Additions int
	Deletions int
	Status    string
	IsBinary  bool
}

// MatchesGlob provides wildcard path matching supporting '**' recursive directory patterns.
func MatchesGlob(pattern, path string) bool {
	normPath := filepath.ToSlash(filepath.Clean(path))
	normPat := filepath.ToSlash(filepath.Clean(pattern))

	if normPat == "**" || normPat == "**/*" {
		return true
	}

	// Handle prefix **/
	if strings.HasPrefix(normPat, "**/") {
		subPat := normPat[3:]
		// Check if matches at root or in any directory
		if matched, _ := filepath.Match(subPat, normPath); matched {
			return true
		}
		parts := strings.Split(normPath, "/")
		for i := range parts {
			subPath := strings.Join(parts[i:], "/")
			if matched, _ := filepath.Match(subPat, subPath); matched {
				return true
			}
		}
	}

	matched, _ := filepath.Match(normPat, normPath)
	return matched
}

// ApplyLargePrAggressiveFilter drops low-signal files when non-deep mode or compact profile applies.
func ApplyLargePrAggressiveFilter(files []ChangedFileReviewItem, unconditionalFilter bool) []ChangedFileReviewItem {
	if !unconditionalFilter && len(files) < 20 {
		return files
	}

	filtered := make([]ChangedFileReviewItem, 0, len(files))
	for _, f := range files {
		skip := false
		for _, pat := range LargePRAggressiveFilterPatterns {
			if MatchesGlob(pat, f.Filename) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, f)
		}
	}

	// Never filter down to empty if original files were non-empty
	if len(filtered) == 0 && len(files) > 0 {
		return files
	}
	return filtered
}

// EstimateFileTokens calculates token cost for a single changed file hunk.
func EstimateFileTokens(f ChangedFileReviewItem) int {
	length := len(f.Patch)
	if length == 0 {
		length = len(f.Content)
	}
	// Add overhead for file header markdown formatting
	length += len(f.Filename) + 64
	return (length + CharsPerToken - 1) / CharsPerToken
}

// ChunkFilesByTokenBudget partitions files into discrete review batches respecting maxBudgetTokens.
func ChunkFilesByTokenBudget(files []ChangedFileReviewItem, maxBudgetTokens int) [][]ChangedFileReviewItem {
	if len(files) == 0 {
		return nil
	}
	if maxBudgetTokens <= 0 {
		return [][]ChangedFileReviewItem{files}
	}

	var chunks [][]ChangedFileReviewItem
	var currentChunk []ChangedFileReviewItem
	currentTokens := 0

	for _, f := range files {
		fileTokens := EstimateFileTokens(f)

		// If a single file exceeds budget, it gets its own dedicated chunk
		if fileTokens >= maxBudgetTokens {
			if len(currentChunk) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = nil
				currentTokens = 0
			}
			chunks = append(chunks, []ChangedFileReviewItem{f})
			continue
		}

		if currentTokens+fileTokens > maxBudgetTokens && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = []ChangedFileReviewItem{f}
			currentTokens = fileTokens
		} else {
			currentChunk = append(currentChunk, f)
			currentTokens += fileTokens
		}
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}
