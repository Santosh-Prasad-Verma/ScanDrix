package grouping

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DefaultMaxFilesPerGroup = 10
	SmallChangeSetLabel     = "small change set"
)

// DiffItem represents one modified file in a pull request or patch.
type DiffItem struct {
	OldPath   string `json:"old_path"`
	NewPath   string `json:"new_path"`
	DiffText  string `json:"diff_text"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	IsDeleted bool   `json:"is_deleted"`
}

// DisplayPath returns the preferred relative path for logging and grouping.
func (d DiffItem) DisplayPath() string {
	if d.NewPath != "" {
		return d.NewPath
	}
	return d.OldPath
}

// FileGroup represents a set of semantically related diffs to be reviewed together.
type FileGroup struct {
	Label string     `json:"label"`
	Files []DiffItem `json:"files"`
}

// groupingResponse represents the LLM response mapping group labels to integer file indices.
type groupingResponse struct {
	Label string `json:"label"`
	Files []int  `json:"files"`
}

// BuildIndexedFileList formats the file change set into an indexed list for prompt injection.
// Using integer indices rather than full file paths reduces completion token usage by ~80%
// and prevents output truncation on large pull requests.
func BuildIndexedFileList(diffs []DiffItem) string {
	var sb strings.Builder
	for i, d := range diffs {
		sb.WriteString(fmt.Sprintf("[%d] %s (+%d, -%d)\n", i, d.DisplayPath(), d.Additions, d.Deletions))
	}
	return sb.String()
}

// ParseGroupingResponse parses the integer-indexed grouping response and maps it back to DiffItems.
func ParseGroupingResponse(content string, diffs []DiffItem) ([]FileGroup, error) {
	content = strings.TrimSpace(content)

	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		content = strings.Join(lines, "\n")
	}

	var resp []groupingResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse grouping JSON: %w", err)
	}

	seen := make([]bool, len(diffs))
	var groups []FileGroup

	for _, g := range resp {
		var gDiffs []DiffItem
		for _, idx := range g.Files {
			if idx < 0 || idx >= len(diffs) {
				continue
			}
			if seen[idx] {
				continue
			}
			seen[idx] = true
			gDiffs = append(gDiffs, diffs[idx])
		}
		if len(gDiffs) > 0 {
			groups = append(groups, FileGroup{Label: g.Label, Files: gDiffs})
		}
	}

	// Any unassigned files get their own single-file group to guarantee complete coverage
	for i, d := range diffs {
		if !seen[i] {
			groups = append(groups, FileGroup{Label: d.DisplayPath(), Files: []DiffItem{d}})
		}
	}

	return EnforceMaxFilesPerGroup(groups, DefaultMaxFilesPerGroup), nil
}

// EnforceMaxFilesPerGroup splits any group exceeding the file cap into smaller chunks.
func EnforceMaxFilesPerGroup(groups []FileGroup, maxFiles int) []FileGroup {
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFilesPerGroup
	}
	var result []FileGroup
	for _, g := range groups {
		if len(g.Files) <= maxFiles {
			result = append(result, g)
			continue
		}
		for i := 0; i < len(g.Files); i += maxFiles {
			end := i + maxFiles
			if end > len(g.Files) {
				end = len(g.Files)
			}
			result = append(result, FileGroup{
				Label: fmt.Sprintf("%s (part %d)", g.Label, (i/maxFiles)+1),
				Files: g.Files[i:end],
			})
		}
	}
	return result
}

// EnforceGroupTokenBudget ensures no group exceeds the maximum token budget.
func EnforceGroupTokenBudget(groups []FileGroup, tokenLimit int) []FileGroup {
	if tokenLimit <= 0 {
		return groups
	}
	var result []FileGroup
	for _, g := range groups {
		var currentChunk []DiffItem
		var currentTokens int64

		for _, item := range g.Files {
			itemTokens := EstimateDiffTokens(item.DiffText)
			if len(currentChunk) > 0 && currentTokens+itemTokens > int64(tokenLimit) {
				result = append(result, FileGroup{
					Label: g.Label,
					Files: currentChunk,
				})
				currentChunk = nil
				currentTokens = 0
			}
			currentChunk = append(currentChunk, item)
			currentTokens += itemTokens
		}
		if len(currentChunk) > 0 {
			result = append(result, FileGroup{
				Label: g.Label,
				Files: currentChunk,
			})
		}
	}
	return result
}
