package git

import (
	"bufio"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// NameStatusEntry represents a single parsed entry from git diff --name-status.
type NameStatusEntry struct {
	File    string `json:"file"`
	OldFile string `json:"old_file,omitempty"`
	Status  string `json:"status"` // added, modified, deleted, renamed
}

// PorcelainStatusEntry represents an entry parsed from git status --porcelain.
type PorcelainStatusEntry struct {
	IndexStatus string `json:"index_status"` // Staged status character (X)
	WorkStatus  string `json:"work_status"`  // Worktree status character (Y)
	File        string `json:"file"`
	OldFile     string `json:"old_file,omitempty"`
	Status      string `json:"status"` // mapped status
	IsStaged    bool   `json:"is_staged"`
	IsUnstaged  bool   `json:"is_unstaged"`
	IsUntracked bool   `json:"is_untracked"`
}

// ParseGitStatus maps a git status character to standard status string.
func ParseGitStatus(statusChar string) string {
	if len(statusChar) == 0 {
		return "modified"
	}
	char := strings.ToUpper(string(statusChar[0]))
	switch char {
	case "A", "?":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "U":
		return "unmerged"
	default:
		return "modified"
	}
}

// ParseGitNameStatusOutput parses the tab-delimited output of git diff --name-status.
func ParseGitNameStatusOutput(nameStatus string) []NameStatusEntry {
	var entries []NameStatusEntry
	scanner := bufio.NewScanner(strings.NewReader(nameStatus))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		statusToken := parts[0]
		status := ParseGitStatus(statusToken)

		if strings.HasPrefix(statusToken, "R") || strings.HasPrefix(statusToken, "C") {
			// Format: R100\told_file\tnew_file
			oldFile := parts[1]
			newFile := parts[len(parts)-1]
			entries = append(entries, NameStatusEntry{
				File:    newFile,
				OldFile: oldFile,
				Status:  status,
			})
		} else {
			// Format: M\tfile or A\tfile or D\tfile
			entries = append(entries, NameStatusEntry{
				File:   parts[1],
				Status: status,
			})
		}
	}

	return entries
}

// ListFilesFromNameStatus extracts slice of file names from git diff --name-status output.
func ListFilesFromNameStatus(nameStatus string) []string {
	entries := ParseGitNameStatusOutput(nameStatus)
	files := make([]string, len(entries))
	for i, entry := range entries {
		files[i] = entry.File
	}
	return files
}

// BuildFileStatusMap returns a lookup map of file path to normalized status.
func BuildFileStatusMap(nameStatus string) map[string]string {
	entries := ParseGitNameStatusOutput(nameStatus)
	m := make(map[string]string, len(entries))
	for _, entry := range entries {
		m[entry.File] = entry.Status
	}
	return m
}

// ParsePorcelainStatus parses git status --porcelain output into structured entries.
func ParsePorcelainStatus(output string) []PorcelainStatusEntry {
	var results []PorcelainStatusEntry
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}

		indexChar := string(line[0])
		workChar := string(line[1])
		payload := strings.TrimSpace(line[2:])

		entry := PorcelainStatusEntry{
			IndexStatus: indexChar,
			WorkStatus:  workChar,
		}

		if indexChar == "?" && workChar == "?" {
			entry.IsUntracked = true
			entry.Status = "added"
			entry.File = payload
		} else {
			if indexChar != " " && indexChar != "?" {
				entry.IsStaged = true
			}
			if workChar != " " && workChar != "?" {
				entry.IsUnstaged = true
			}

			// Handle rename syntax: "R  orig -> target" or "RM orig -> target"
			if strings.Contains(payload, " -> ") {
				parts := strings.Split(payload, " -> ")
				entry.OldFile = strings.Trim(parts[0], "\"")
				entry.File = strings.Trim(parts[1], "\"")
				entry.Status = "renamed"
			} else {
				entry.File = strings.Trim(payload, "\"")
				if indexChar == "D" || workChar == "D" {
					entry.Status = "deleted"
				} else if indexChar == "A" {
					entry.Status = "added"
				} else {
					entry.Status = "modified"
				}
			}
		}

		results = append(results, entry)
	}

	return results
}

// SummarizePorcelainToDiffs converts porcelain entries to types.FileDiff entries.
func SummarizePorcelainToDiffs(entries []PorcelainStatusEntry) []types.FileDiff {
	diffs := make([]types.FileDiff, 0, len(entries))
	for _, e := range entries {
		diffs = append(diffs, types.FileDiff{
			Path:    e.File,
			OldPath: e.OldFile,
			Status:  e.Status,
		})
	}
	return diffs
}
