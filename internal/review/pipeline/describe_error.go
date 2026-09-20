// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package pipeline

import (
	"regexp"
	"strings"
)

// DescribedPipelineError turns a pipeline error into an actionable, single-sentence summary for users.
type DescribedPipelineError struct {
	Text       string `json:"text"`
	Classified bool   `json:"classified"`
}

// MaxRawLength is the longest raw message inlined before truncating.
const MaxRawLength = 160

var (
	whitespaceRegex      = regexp.MustCompile(`\s+`)
	sentenceBoundaryRegex = regexp.MustCompile(`^.*?[.!?](?:\s|$)`)
)

// ErrorClassifier classifies runtime errors into user-friendly explanations.
type ErrorClassifier interface {
	FriendlyMessage() string
	IsClassified() bool
}

// ClassifiedError attaches a friendly user-facing explanation at the throw site.
type ClassifiedError struct {
	Err             error
	Category        string
	FriendlyMessage string
}

func (c *ClassifiedError) Error() string {
	if c.Err != nil {
		return c.Err.Error()
	}
	return c.FriendlyMessage
}

// DescribePipelineError extracts an actionable user-facing message from a pipeline error.
// Only classifications attached at the throw site are used. Arbitrary errors are not
// re-classified to avoid misattributing provider vs platform failures.
func DescribePipelineError(err error, info *ReviewErrorInfo) DescribedPipelineError {
	if err == nil && info == nil {
		return DescribedPipelineError{
			Text:       "",
			Classified: false,
		}
	}

	// 1. Prefer throw-site structured classification from ReviewErrorInfo
	if info != nil && info.FriendlyMessage != "" {
		return DescribedPipelineError{
			Text:       strings.TrimSpace(info.FriendlyMessage),
			Classified: true,
		}
	}

	// 2. Check if error wraps a ClassifiedError
	if classified, ok := err.(*ClassifiedError); ok && classified.FriendlyMessage != "" {
		return DescribedPipelineError{
			Text:       strings.TrimSpace(classified.FriendlyMessage),
			Classified: true,
		}
	}

	// 3. Fall back to raw error message, collapsed to one line and capped
	rawMsg := ""
	if err != nil {
		rawMsg = err.Error()
	} else if info != nil {
		rawMsg = info.ProviderMessage
	}

	return DescribedPipelineError{
		Text:       ToOneLine(rawMsg, MaxRawLength),
		Classified: false,
	}
}

// ToOneLine collapses whitespace/newlines and caps length at sentence boundaries.
func ToOneLine(message string, maxLength int) string {
	if maxLength <= 0 {
		maxLength = MaxRawLength
	}

	flattened := whitespaceRegex.ReplaceAllString(message, " ")
	flattened = strings.TrimSpace(flattened)

	if len(flattened) <= maxLength {
		return flattened
	}

	// Prefer cutting at a sentence boundary so the result reads as a whole thought
	truncatedPrefix := flattened[:maxLength]
	sentenceMatch := sentenceBoundaryRegex.FindString(truncatedPrefix)
	if sentenceMatch != "" && len(strings.TrimSpace(sentenceMatch)) > 40 {
		return strings.TrimSpace(sentenceMatch)
	}

	return strings.TrimRight(truncatedPrefix, " .,;:!?") + "…"
}
