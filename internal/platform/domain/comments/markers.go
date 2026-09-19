// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package comments

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	DrixyCodeReviewCompletedMarker        = "## Code Review Completed! 🔥"
	DrixyCodeReviewCompletedMarkerEncoded = "## Code Review Completed! ud83dudd25" // Azure encoded emoji
	DrixyCriticalIssueCommentMarker       = "# Found critical issues please"
	DrixyStartCommandMarker               = "@drixy start"
	DefaultBotUsername                    = "drixy"
	MaxReviewDirectiveLength              = 500
)

var (
	drixyReviewCommandPattern      = regexp.MustCompile(`(?i)^\s*@drixy\s+(start-review|review)(?:\s|$)`)
	drixyReviewMarkerPattern       = regexp.MustCompile(`(?i)<!--\s*drixy-codereview\s*-->`)
	drixyForceReviewCommandPattern = regexp.MustCompile(`(?i)^\s*@drixy\s+(start-review|review)\s+--?force\b`)
	drixyHeavyReviewCommandPattern = regexp.MustCompile(`(?i)^\s*@drixy\s+(?:start-review|review)\b[ \t]+(?:[^\n]*\s)?--?heavy\b`)
	drixyReviewCommandHeadPattern  = regexp.MustCompile(`(?i)^\s*@drixy\s+(?:start-review|review)\b[ \t]*(?:(?:--?force|--?heavy)\b[ \t]*)*`)
	exactMarkers                   = []string{
		DrixyCodeReviewCompletedMarker,
		DrixyCodeReviewCompletedMarkerEncoded,
		DrixyCriticalIssueCommentMarker,
	}
	patternMarkers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)/@?drixy\s+(start(-review)?|review)\b|start-review`),
	}
)

// HasDrixyMarker checks whether a comment contains any ScanDrix completion/status markers.
func HasDrixyMarker(text string) bool {
	if text == "" {
		return false
	}
	for _, marker := range exactMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	for _, pattern := range patternMarkers {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// HasReviewMarker checks if comment includes the internal drixy-codereview HTML tag.
func HasReviewMarker(text string) bool {
	if text == "" {
		return false
	}
	return drixyReviewMarkerPattern.MatchString(text)
}

// IsReviewCommand checks if comment issues a review trigger (@drixy review / @drixy start-review).
func IsReviewCommand(text, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && botUsername != DefaultBotUsername {
		customPattern := regexp.MustCompile(`(?i)^\s*@` + regexp.QuoteMeta(botUsername) + `\s+(start-review|review)(?:\s|$)`)
		return customPattern.MatchString(text)
	}
	return false
}

// IsForceReviewCommand checks if review command carries `--force` flag.
func IsForceReviewCommand(text, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyForceReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && botUsername != DefaultBotUsername {
		customPattern := regexp.MustCompile(`(?i)^\s*@` + regexp.QuoteMeta(botUsername) + `\s+(start-review|review)\s+--?force\b`)
		return customPattern.MatchString(text)
	}
	return false
}

// IsHeavyReviewCommand checks if review command carries `--heavy` flag for deeper analysis.
func IsHeavyReviewCommand(text, botUsername string) bool {
	if text == "" {
		return false
	}
	if drixyHeavyReviewCommandPattern.MatchString(text) {
		return true
	}
	if botUsername != "" && botUsername != DefaultBotUsername {
		customPattern := regexp.MustCompile(`(?i)^\s*@` + regexp.QuoteMeta(botUsername) + `\s+(?:start-review|review)\b[ \t]+(?:[^\n]*\s)?--?heavy\b`)
		return customPattern.MatchString(text)
	}
	return false
}

// IsDrixyMentionNonReview checks if comment mentions @drixy for Git chat (not a review command).
func IsDrixyMentionNonReview(text, botUsername string) bool {
	if text == "" {
		return false
	}
	bot := DefaultBotUsername
	if botUsername != "" {
		bot = botUsername
	}
	mentionPattern := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(bot) + `\b`)
	if !mentionPattern.MatchString(text) {
		return false
	}
	return !IsReviewCommand(text, bot)
}

// SanitizeReviewDirective sanitizes free-text directives to prevent prompt breakout.
func SanitizeReviewDirective(raw string) string {
	var sb strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f || r == '<' || r == '>' {
			sb.WriteRune(' ')
		} else {
			sb.WriteRune(r)
		}
	}
	// collapse whitespace
	fields := strings.Fields(sb.String())
	return strings.Join(fields, " ")
}

// NormalizeReviewDirective limits length and applies sanitization.
func NormalizeReviewDirective(raw string) string {
	if raw == "" {
		return ""
	}
	clean := SanitizeReviewDirective(raw)
	if len(clean) > MaxReviewDirectiveLength {
		clean = clean[:MaxReviewDirectiveLength]
	}
	return strings.TrimSpace(clean)
}

// ParseReviewDirective extracts steering instructions after `@drixy review`.
func ParseReviewDirective(text, botUsername string) string {
	if text == "" || !IsReviewCommand(text, botUsername) {
		return ""
	}

	var headLoc []int
	if drixyReviewCommandHeadPattern.MatchString(text) {
		headLoc = drixyReviewCommandHeadPattern.FindStringIndex(text)
	} else if botUsername != "" && botUsername != DefaultBotUsername {
		customHead := regexp.MustCompile(`(?i)^\s*@` + regexp.QuoteMeta(botUsername) + `\s+(?:start-review|review)\b[ \t]*(?:(?:--?force|--?heavy)\b[ \t]*)*`)
		headLoc = customHead.FindStringIndex(text)
	}

	if len(headLoc) < 2 {
		return ""
	}

	remainder := text[headLoc[1]:]
	lines := strings.Split(remainder, "\n")
	firstLine := strings.TrimSpace(lines[0])

	// Trim surrounding quotes
	firstLine = strings.Trim(firstLine, `"'` + "`")

	// Strip any trailing flags
	flagRegex := regexp.MustCompile(`(?i)\s*--?(?:heavy|force)\b`)
	cleaned := flagRegex.ReplaceAllString(firstLine, "")

	return NormalizeReviewDirective(cleaned)
}

// CleanAuthor removes markdown / control characters from author usernames.
func CleanAuthor(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) && !unicode.IsControl(r) {
			return r
		}
		return -1
	}, name)
}
