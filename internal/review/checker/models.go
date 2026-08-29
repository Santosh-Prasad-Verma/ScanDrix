package checker

import (
	"time"

	"github.com/google/uuid"
)

// SuggestionStatus tracks whether a developer incorporated an AI recommendation.
type SuggestionStatus string

const (
	StatusPending        SuggestionStatus = "PENDING"
	StatusAcceptedExact  SuggestionStatus = "ACCEPTED_EXACT"
	StatusAcceptedManual SuggestionStatus = "ACCEPTED_MANUAL"
	StatusRejected       SuggestionStatus = "REJECTED"
	StatusIgnored        SuggestionStatus = "IGNORED"
)

// SuggestionVerification details the resolution check on a specific finding.
type SuggestionVerification struct {
	FindingID           uuid.UUID        `json:"finding_id"`
	WorkspaceID         uuid.UUID        `json:"workspace_id"`
	PullRequestNumber   int              `json:"pull_request_number"`
	Status              SuggestionStatus `json:"status"`
	OriginalCode        string           `json:"original_code"`
	SuggestedCode       string           `json:"suggested_code"`
	CommittedCode       string           `json:"committed_code"`
	ResolutionCommitSHA string           `json:"resolution_commit_sha"`
	SimilarityScore     float64          `json:"similarity_score"` // 0.0 to 1.0
	StillVulnerable     bool             `json:"still_vulnerable"`
	VerifiedAt          time.Time        `json:"verified_at"`
}

// AdoptionMetrics aggregates developer AI acceptance velocity.
type AdoptionMetrics struct {
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	TotalSuggestions    int       `json:"total_suggestions"`
	AcceptedExactCount  int       `json:"accepted_exact_count"`
	AcceptedManualCount int       `json:"accepted_manual_count"`
	RejectedCount       int       `json:"rejected_count"`
	PendingCount        int       `json:"pending_count"`
	AdoptionRate        float64   `json:"adoption_rate"` // (Exact + Manual) / Total
}
