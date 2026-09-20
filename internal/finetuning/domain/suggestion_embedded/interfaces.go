// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package suggestionembedded

// OrganizationRef represents a lightweight organization reference.
type OrganizationRef struct {
	UUID string `json:"uuid"`
}

// SuggestionEmbedded defines the data structure for vectorized code review suggestions.
type SuggestionEmbedded struct {
	UUID               string           `json:"uuid,omitempty"`
	SuggestionID       string           `json:"suggestion_id"`
	SuggestionEmbed    []float64        `json:"suggestion_embed"`
	PullRequestNumber  int              `json:"pull_request_number"`
	RepositoryID       string           `json:"repository_id"`
	RepositoryFullName string           `json:"repository_full_name"`
	Organization       *OrganizationRef `json:"organization,omitempty"`
	Label              string           `json:"label"`
	Severity           string           `json:"severity"`
	FeedbackType       string           `json:"feedback_type"`
	ImprovedCode       string           `json:"improved_code"`
	SuggestionContent  string           `json:"suggestion_content"`
	OneSentenceSummary string           `json:"one_sentence_summary,omitempty"`
	Language           string           `json:"language"`
}

// SuggestionEmbeddedFeedbacks summarizes reaction metrics.
type SuggestionEmbeddedFeedbacks struct {
	PositiveFeedbacks int `json:"positive_feedbacks"`
	NegativeFeedbacks int `json:"negative_feedbacks"`
	Total             int `json:"total"`
}

// LanguageCount records feedback tallies per programming language.
type LanguageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// LanguageFeedbackGroup groups feedback counts by language.
type LanguageFeedbackGroup struct {
	Language []LanguageCount `json:"language"`
	Total    int             `json:"total"`
}

// SuggestionEmbeddedFeedbacksWithLanguage provides feedback metrics partitioned by language.
type SuggestionEmbeddedFeedbacksWithLanguage struct {
	PositiveFeedbacks LanguageFeedbackGroup `json:"positive_feedbacks"`
	NegativeFeedbacks LanguageFeedbackGroup `json:"negative_feedbacks"`
	Total             int                   `json:"total"`
}

// PullRequestRefToEmbed represents pull request metadata for suggestion embedding.
type PullRequestRefToEmbed struct {
	ID         string `json:"id,omitempty"`
	Number     int    `json:"number"`
	Repository struct {
		ID       string `json:"id"`
		FullName string `json:"fullName"`
	} `json:"repository"`
}

// SuggestionToEmbed represents an un-embedded or partially embedded suggestion.
type SuggestionToEmbed struct {
	ID                   string                `json:"id"`
	SuggestionContent    string                `json:"suggestionContent"`
	OneSentenceSummary   string                `json:"oneSentenceSummary"`
	Label                string                `json:"label"`
	Severity             string                `json:"severity"`
	FeedbackType         string                `json:"feedbackType"`
	ImprovedCode         string                `json:"improvedCode,omitempty"`
	Language             string                `json:"language,omitempty"`
	ImplementationStatus string                `json:"implementationStatus,omitempty"`
	SuggestionEmbed      []float64             `json:"suggestionEmbed,omitempty"`
	OrganizationID       string                `json:"organizationId"`
	PullRequest          PullRequestRefToEmbed `json:"pullRequest"`
}

