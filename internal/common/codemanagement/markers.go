// Package codemanagement provides comment markers, review directive parsing, severity shields, and badge utilities.
package codemanagement

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// DrixyCodeReviewCompletedMarker marks review completion in comments.
	DrixyCodeReviewCompletedMarker = "## Code Review Completed! 🔥"
	// DrixyCodeReviewCompletedMarkerEncoded represents Azure encoded emoji.
	DrixyCodeReviewCompletedMarkerEncoded = "## Code Review Completed! ud83dudd25"
	// DrixyCriticalIssueCommentMarker indicates critical issues were detected.
	DrixyCriticalIssueCommentMarker = "# Found critical issues please"
	// DrixyStartCommandMarker represents manual start command.
	DrixyStartCommandMarker = "@drixy start"
	// DefaultBotUsername is the primary bot identity.
	DefaultBotUsername = "drixy"
	// MaxReviewDirectiveLength caps free-text directive length to prevent prompt bloat.
	MaxReviewDirectiveLength = 500
)
var (
	drixyReviewCommandPattern    = regexp.MustCompile(`(?i)^\s*@drixy\s+(start-review|review)(?:\s|$)`)
	drixyReviewMarkerPattern     = regexp.MustCompile(`(?i)<!--\s*drixy-codereview\s*-->`)
	drixyMentionPattern          = regexp.MustCompile(`(?i)^\s*@drixy\b`)
	drixyForceReviewCommandPattern = regexp.MustCompile(`(?i)^\s*@drixy\s+(start-review|review)\s+--?force\b`)
	drixyHeavyReviewCommandPattern = regexp.MustCompile(`(?i)^\s*@drixy\s+(?:start-review|review)\b[ \t]+(?:[^\n]*\s)?--?heavy\b`)
	drixyReviewCommandHeadPattern = regexp.MustCompile(`(?i)^\s*@drixy\s+(?:start-review|review)\b[ \t]*(?:(?:--?force|--?heavy)\b[ \t]*)*`)
	controlCharsRegex = regexp.MustCompile(`[\x00-\x1f\x7f<>]`)
	whitespaceRegex   = regexp.MustCompile(`\s+`)
	flagsStripRegex   = regexp.MustCompile(`(?i)\s*--?(?:heavy|force)\b`)
	quotesStripRegex  = regexp.MustCompile(`^["']+|["']+$`)
)

// HasDrixyMarker returns true if the comment text contains any standard Drixy review marker.
func HasDrixyMarker(text string) bool {
	if text == "" {
		return false
	}
	if strings.Contains(text, DrixyCodeReviewCompletedMarker) ||
		strings.Contains(text, DrixyCodeReviewCompletedMarkerEncoded) ||
		strings.Contains(text, DrixyCriticalIssueCommentMarker) {
		return true
	}
	pattern := regexp.MustCompile(`(?i)@?drixy\s+(start(-review)?|review)\b|start-review`)
	return pattern.MatchString(text)
}

// HasReviewMarker returns true if the HTML comment marker <!-- drixy-codereview --> is present.
func HasReviewMarker(text string) bool {
	if text == "" {
		return false
	}
	return drixyReviewMarkerPattern.MatchString(text)
}

// IsReviewCommand checks if comment text triggers a code review (@drixy review or @drixy start-review).
func IsReviewCommand(text string, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && !strings.EqualFold(botUsername, DefaultBotUsername) {
		customPattern := regexp.MustCompile(fmt.Sprintf(`(?i)^\s*@%s\s+(start-review|review)(?:\s|$)`, regexp.QuoteMeta(botUsername)))
		return customPattern.MatchString(text)
	}
	return false
}

// IsForceReviewCommand returns true if the review command carries the --force flag.
func IsForceReviewCommand(text string, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyForceReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && !strings.EqualFold(botUsername, DefaultBotUsername) {
		customPattern := regexp.MustCompile(fmt.Sprintf(`(?i)^\s*@%s\s+(start-review|review)\s+--?force\b`, regexp.QuoteMeta(botUsername)))
		return customPattern.MatchString(text)
	}
	return false
}

// IsHeavyReviewCommand returns true if the review command carries the --heavy flag.
func IsHeavyReviewCommand(text string, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyHeavyReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && !strings.EqualFold(botUsername, DefaultBotUsername) {
		customPattern := regexp.MustCompile(fmt.Sprintf(`(?i)^\s*@%s\s+(?:start-review|review)\b[ \t]+(?:[^\n]*\s)?--?heavy\b`, regexp.QuoteMeta(botUsername)))
		return customPattern.MatchString(text)
	}
	return false
}

// IsDrixyMentionNonReview returns true if comment mentions @drixy without a review command.
func IsDrixyMentionNonReview(text string, botUsername string) bool {
	if text == "" {
		return false
	}
	if IsReviewCommand(text, botUsername) {
		return false
	}
	if drixyMentionPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && !strings.EqualFold(botUsername, DefaultBotUsername) {
		customPattern := regexp.MustCompile(fmt.Sprintf(`(?i)^\s*@%s\b`, regexp.QuoteMeta(botUsername)))
		return customPattern.MatchString(text)
	}
	return false
}

// NormalizeReviewDirective sanitizes and length-caps an extracted steering directive string.
func NormalizeReviewDirective(raw string) string {
	if raw == "" {
		return ""
	}
	clean := controlCharsRegex.ReplaceAllString(raw, " ")
	clean = whitespaceRegex.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	if len(clean) > MaxReviewDirectiveLength {
		clean = clean[:MaxReviewDirectiveLength]
	}
	return clean
}

// ParseReviewDirective extracts any free-text steering directive appended to a review command.
func ParseReviewDirective(text string, botUsername string) string {
	if text == "" || !IsReviewCommand(text, botUsername) {
		return ""
	}
	headLoc := drixyReviewCommandHeadPattern.FindStringIndex(text)
	if headLoc == nil && botUsername != "" {
		customHead := regexp.MustCompile(fmt.Sprintf(`(?i)^\s*@%s\s+(?:start-review|review)\b[ \t]*(?:(?:--?force|--?heavy)\b[ \t]*)*`, regexp.QuoteMeta(botUsername)))
		headLoc = customHead.FindStringIndex(text)
	}
	if headLoc == nil {
		return ""
	}

	remaining := text[headLoc[1]:]
	lines := strings.Split(remaining, "\n")
	firstLine := strings.TrimSpace(lines[0])
	firstLine = quotesStripRegex.ReplaceAllString(firstLine, "")
	firstLine = flagsStripRegex.ReplaceAllString(firstLine, "")

	return NormalizeReviewDirective(firstLine)
}
