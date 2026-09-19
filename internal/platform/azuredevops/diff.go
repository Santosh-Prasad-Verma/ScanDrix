package azuredevops

import (
	"fmt"
	"strings"
)

// GenerateTwoFilesPatch generates a standard unified git diff between original and modified content.
// Matches libs/platform/infrastructure/adapters/services/azureRepos/azureRepos.service.ts (_generateFileDiffForAzure).
func GenerateTwoFilesPatch(oldPath, newPath, oldContent, newContent, baseSHA, targetSHA string) (string, int, int) {
	if oldContent == "" && newContent == "" {
		return "", 0, 0
	}

	if oldContent == "" {
		// Pure addition
		lines := strings.Split(newContent, "\n")
		// Remove trailing empty line if string ended with newline
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		additions := len(lines)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", newPath))
		sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", additions))
		for _, l := range lines {
			sb.WriteString("+")
			sb.WriteString(l)
			sb.WriteString("\n")
		}
		return sb.String(), additions, 0
	}

	if newContent == "" {
		// Pure deletion
		lines := strings.Split(oldContent, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		deletions := len(lines)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("--- a/%s\n+++ /dev/null\n", oldPath))
		sb.WriteString(fmt.Sprintf("@@ -1,%d +0,0 @@\n", deletions))
		for _, l := range lines {
			sb.WriteString("-")
			sb.WriteString(l)
			sb.WriteString("\n")
		}
		return sb.String(), 0, deletions
	}

	// Content modified - perform line-by-line diff using LCS
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	n := len(oldLines)
	m := len(newLines)

	// Compute LCS table
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if oldLines[i] == newLines[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i+1][j+1] = dp[i+1][j]
			} else {
				dp[i+1][j+1] = dp[i][j+1]
			}
		}
	}

	type diffLine struct {
		op   byte // ' ', '+', '-'
		text string
	}

	var edits []diffLine
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			edits = append(edits, diffLine{op: ' ', text: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			edits = append(edits, diffLine{op: '+', text: newLines[j-1]})
			j--
		} else if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			edits = append(edits, diffLine{op: '-', text: oldLines[i-1]})
			i--
		}
	}

	// Reverse edits to get forward order
	for k := 0; k < len(edits)/2; k++ {
		opp := len(edits) - 1 - k
		edits[k], edits[opp] = edits[opp], edits[k]
	}

	additions := 0
	deletions := 0
	for _, e := range edits {
		if e.op == '+' {
			additions++
		} else if e.op == '-' {
			deletions++
		}
	}

	if additions == 0 && deletions == 0 {
		return "", 0, 0
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", oldPath, newPath))
	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", n, m))
	for _, e := range edits {
		sb.WriteByte(e.op)
		sb.WriteString(e.text)
		sb.WriteString("\n")
	}

	return sb.String(), additions, deletions
}
