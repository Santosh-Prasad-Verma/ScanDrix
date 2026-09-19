// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"strings"
)

// DiffChangesSummary counts lines added and removed in a diff.
type DiffChangesSummary struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// CountDiffChanges parses a raw git diff text and returns counts of added and deleted lines.
func CountDiffChanges(diff string) DiffChangesSummary {
	var summary DiffChangesSummary

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			summary.Additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			summary.Deletions++
		}
	}

	return summary
}
