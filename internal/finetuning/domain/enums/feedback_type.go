// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package enums

// FeedbackType categorizes developer reactions to review suggestions.
type FeedbackType string

const (
	FeedbackTypePositiveReaction      FeedbackType = "positiveReaction"
	FeedbackTypeNegativeReaction      FeedbackType = "negativeReaction"
	FeedbackTypeSuggestionImplemented FeedbackType = "suggestionImplemented"
	FeedbackTypeNeutral               FeedbackType = "neutral"

	// Shorthand aliases
	PositiveReaction      = FeedbackTypePositiveReaction
	NegativeReaction      = FeedbackTypeNegativeReaction
	SuggestionImplemented = FeedbackTypeSuggestionImplemented
	Neutral               = FeedbackTypeNeutral
)
