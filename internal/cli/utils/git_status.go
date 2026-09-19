// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"strings"
)

// FileStatusEntry represents a file and its change status.
type FileStatusEntry struct {
	File   string `json:"file"`
	Status string `json:"status"` // "added", "modified", "deleted", "renamed"
}

// ParseGitStatus maps a git porcelain status character to a standardized status string.
func ParseGitStatus(statusChar string) string {
	if len(statusChar) == 0 {
		return "modified"
	}
	char := strings.ToUpper(string(statusChar[0]))
	switch char {
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "M":
		return "modified"
	default:
		return "modified"
	}
}

// ParseGitNameStatusOutput converts raw output from 'git diff --name-status' into FileStatusEntry records.
func ParseGitNameStatusOutput(nameStatus string) []FileStatusEntry {
	var files []FileStatusEntry

	lines := strings.Split(nameStatus, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		parts := strings.Split(trimmed, "\t")
		if len(parts) < 2 {
			continue
		}

		statusChar := parts[0]
		fileName := parts[1]
		if strings.HasPrefix(statusChar, "R") || strings.HasPrefix(statusChar, "C") {
			fileName = parts[len(parts)-1]
		}

		if fileName == "" {
			continue
		}

		files = append(files, FileStatusEntry{
			File:   fileName,
			Status: ParseGitStatus(statusChar),
		})
	}

	return files
}

// CreateFileSelectionFromNameStatus parses --name-status output into a file list and status lookup map.
func CreateFileSelectionFromNameStatus(nameStatus string) (filesToRead []string, statusMap map[string]string) {
	entries := ParseGitNameStatusOutput(nameStatus)
	filesToRead = make([]string, 0, len(entries))
	statusMap = make(map[string]string, len(entries))

	for _, entry := range entries {
		filesToRead = append(filesToRead, entry.File)
		statusMap[entry.File] = entry.Status
	}

	return filesToRead, statusMap
}
