package interfaces

import (
	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/finetuning/domain/enums"
	"github.com/scandrix/backend/internal/finetuning/domain/suggestion_embedded"
	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

// DrixyFineTuningPullRequestRepository holds repository metadata for fine tuning.
type DrixyFineTuningPullRequestRepository struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

// DrixyFineTuningPullRequest holds pull request context for fine tuning.
type DrixyFineTuningPullRequest struct {
	ID         string                               `json:"id"`
	Number     int                                  `json:"number"`
	Repository DrixyFineTuningPullRequestRepository `json:"repository"`
}

// DrixyFineTuning represents a full fine-tuning record.
type DrixyFineTuning struct {
	UUID              uuid.UUID                  `json:"uuid"`
	SuggestionID      string                     `json:"suggestionId"`
	SuggestionContent string                     `json:"suggestionContent"`
	ImprovedCode      string                     `json:"improvedCode"`
	Severity          string                     `json:"severity"`
	Label             string                     `json:"label"`
	FeedbackType      enums.FeedbackType         `json:"feedbackType"`
	PullRequest       DrixyFineTuningPullRequest `json:"pullRequest"`
	OrganizationID    string                     `json:"organizationId"`
}

// EmbeddingMetadata holds model metadata for embeddings.
type EmbeddingMetadata struct {
	Model string `json:"model"`
}

// EmbeddingResult represents the result of embedding a suggestion.
type EmbeddingResult struct {
	SuggestionID string            `json:"suggestionId"`
	Embedding    []float64         `json:"embedding"`
	Metadata     EmbeddingMetadata `json:"metadata"`
}

// ClusterAnalysis represents aggregate feedback counts for a cluster.
type ClusterAnalysis struct {
	Total             int `json:"total"`
	PositiveReactions int `json:"positiveReactions"`
	NegativeReactions int `json:"negativeReactions"`
	Implemented       int `json:"implemented"`
	Neutral           int `json:"neutral"`
}

// ClusterizedSuggestion represents a suggestion assigned to a cluster.
type ClusterizedSuggestion struct {
	Cluster            int                                    `json:"cluster"`
	FineTuningDecision enums.FineTuningDecision               `json:"fineTuningDecision,omitempty"`
	OriginalSuggestion suggestionembedded.SuggestionEmbedded `json:"originalSuggestion"`
	Language           string                                 `json:"language"`
}

// ClusterDistribution maps cluster IDs to their feedback distribution analysis.
type ClusterDistribution map[int]ClusterAnalysis

// FineTuningAnalysisResult represents the categorized outcome of fine tuning analysis.
type FineTuningAnalysisResult struct {
	KeepedSuggestions    []*reviewdomain.CodeSuggestion `json:"keepedSuggestions"`
	DiscardedSuggestions []*reviewdomain.CodeSuggestion `json:"discardedSuggestions"`
}
