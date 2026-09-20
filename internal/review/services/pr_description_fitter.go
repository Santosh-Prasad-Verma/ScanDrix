package services

import (
	"fmt"
	"strings"
)

// PlatformDescriptionLimits defines the exact platform enforced character limits for pull request descriptions.
var PlatformDescriptionLimits = map[SCMPlatformType]int{
	PlatformAzureDevOps: 4000,      // API enforced with InvalidArgumentValueException
	PlatformBitbucket:   32768,     // Official Atlassian limit for description and branch fields
	PlatformGitHub:      65536,     // HTTP 422 Body is too long (maximum is 65536 characters)
	PlatformGitLab:      1048576,   // 1 MiB default in application_setting.rb
	SCMPlatformType("forgejo"): 1048576, // 1 MiB sane application ceiling
}

const (
	TruncationNotice = "\n\n_…(truncated by ScanDrix to fit the platform description size limit)_\n"
	SummaryStartMarker = "<!-- scandrix-pr-summary:start -->"
	SummaryEndMarker   = "<!-- scandrix-pr-summary:end -->"
)

// PRDescriptionFitter ensures that generated PR summaries never exceed Git provider API limits.
type PRDescriptionFitter struct{}

// NewPRDescriptionFitter constructs a PR description fitter.
func NewPRDescriptionFitter() *PRDescriptionFitter {
	return &PRDescriptionFitter{}
}

// GetPlatformLimit returns the character limit for the given SCM platform.
func (f *PRDescriptionFitter) GetPlatformLimit(platform SCMPlatformType) int {
	if limit, ok := PlatformDescriptionLimits[platform]; ok {
		return limit
	}
	return 0 // No limit / unknown platform
}

// FitPRDescription slices or adjusts the description so it fits cleanly inside provider limits.
// Preserves the closing summary marker (<!-- scandrix-pr-summary:end -->) so subsequent runs
// can identify and update existing ScanDrix review summaries without duplicate generation.
func (f *PRDescriptionFitter) FitPRDescription(description string, platform SCMPlatformType) string {
	limit := f.GetPlatformLimit(platform)
	if limit <= 0 || len(description) <= limit {
		return description
	}

	if strings.HasSuffix(strings.TrimSpace(description), SummaryEndMarker) {
		budget := limit - len(TruncationNotice) - len(SummaryEndMarker)
		if budget <= 0 {
			// Pathological: the notice and marker alone exceed the platform ceiling
			return description[:limit]
		}

		trimmedContent := description[:budget]
		return trimmedContent + TruncationNotice + SummaryEndMarker
	}

	// If the description contains the summary block in the middle
	if idx := strings.Index(description, SummaryEndMarker); idx != -1 {
		prefix := description[:idx]
		budget := limit - len(TruncationNotice) - len(SummaryEndMarker)
		if len(prefix) > budget && budget > 0 {
			prefix = prefix[:budget]
		}
		return prefix + TruncationNotice + SummaryEndMarker
	}

	// Default hard slice with notice
	budget := limit - len(TruncationNotice)
	if budget <= 0 {
		return description[:limit]
	}
	return description[:budget] + TruncationNotice
}

// ExtractPreviousSummary extracts the previous ScanDrix summary from a PR description if present.
func (f *PRDescriptionFitter) ExtractPreviousSummary(description string) (string, string, bool) {
	startIdx := strings.Index(description, SummaryStartMarker)
	endIdx := strings.Index(description, SummaryEndMarker)

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		userPrefix := strings.TrimSpace(description[:startIdx])
		summaryContent := description[startIdx+len(SummaryStartMarker) : endIdx]
		return strings.TrimSpace(summaryContent), userPrefix, true
	}

	return "", description, false
}

// InjectSummary merges a newly generated ScanDrix summary into an existing PR body.
// Preserves author-written descriptions above the ScanDrix marker.
func (f *PRDescriptionFitter) InjectSummary(
	existingDescription string,
	newSummary string,
	platform SCMPlatformType,
) string {
	_, userPrefix, hasPrevious := f.ExtractPreviousSummary(existingDescription)

	var combined string
	if hasPrevious && userPrefix != "" {
		combined = fmt.Sprintf("%s\n\n%s\n%s\n%s", userPrefix, SummaryStartMarker, newSummary, SummaryEndMarker)
	} else if userPrefix != "" {
		combined = fmt.Sprintf("%s\n\n%s\n%s\n%s", strings.TrimSpace(existingDescription), SummaryStartMarker, newSummary, SummaryEndMarker)
	} else {
		combined = fmt.Sprintf("%s\n%s\n%s", SummaryStartMarker, newSummary, SummaryEndMarker)
	}

	return f.FitPRDescription(combined, platform)
}
