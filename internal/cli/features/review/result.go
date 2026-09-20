package review

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

var severityOrder = map[string]int{
	"info":     0,
	"warning":  1,
	"error":    2,
	"critical": 3,
}

// ShouldUseInteractiveReview checks if the CLI should launch the terminal TUI for findings navigation.
func ShouldUseInteractiveReview(isAgent bool, interactive bool, output string, format string) bool {
	if isAgent {
		return false
	}
	if interactive {
		return true
	}
	return output == "" && (format == "" || format == "terminal")
}

// ShouldUseHunkViewer evaluates whether the review should open the hunk split/unified diff TUI.
func ShouldUseHunkViewer(isAgent, interactive, noHunk bool, output, format string, ttyOut, scopeSupported, platformSupported bool) bool {
	if isAgent || noHunk || interactive {
		return false
	}
	if output != "" {
		return false
	}
	if format != "" && format != "terminal" {
		return false
	}
	if !ttyOut || !platformSupported || !scopeSupported {
		return false
	}
	return true
}

// IsHunkPlatformSupported checks whether the current operating system supports the hunk binary.
func IsHunkPlatformSupported() bool {
	return runtime.GOOS != "windows"
}

// ShouldFailReview determines if review issues violate the --fail-on threshold.
func ShouldFailReview(result *types.ReviewResult, failOn string) bool {
	if failOn == "" || result == nil {
		return false
	}

	threshold, ok := severityOrder[strings.ToLower(strings.TrimSpace(failOn))]
	if !ok {
		return false
	}

	for _, issue := range result.Issues {
		sev := strings.ToLower(string(issue.Severity))
		if order, exists := severityOrder[sev]; exists && order >= threshold {
			return true
		}
	}

	return false
}

// FormatFailOnExitMessage produces the standard error exit message when issues exceed threshold.
func FormatFailOnExitMessage(result *types.ReviewResult, failOn string) string {
	if failOn == "" || result == nil {
		return ""
	}

	threshold, ok := severityOrder[strings.ToLower(strings.TrimSpace(failOn))]
	if !ok {
		return ""
	}

	blockingCount := 0
	for _, issue := range result.Issues {
		sev := strings.ToLower(string(issue.Severity))
		if order, exists := severityOrder[sev]; exists && order >= threshold {
			blockingCount++
		}
	}

	if blockingCount == 0 {
		return ""
	}

	issueLabel := "issue"
	verbPhrase := "meets or exceeds"
	if blockingCount > 1 {
		issueLabel = "issues"
		verbPhrase = "meet or exceed"
	}

	return fmt.Sprintf("Exiting with code 1 because %d %s %s `--fail-on %s`.", blockingCount, issueLabel, verbPhrase, failOn)
}

// FormatTrialCompletionMessage provides user-facing rate-limit or trial information.
func FormatTrialCompletionMessage(used, limit int) string {
	if limit > 0 {
		return fmt.Sprintf("Review complete! (Trial: %d/%d reviews today)", used, limit)
	}
	return "Review complete! (Active workspace)"
}
