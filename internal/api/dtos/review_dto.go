package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// TriggerReviewRequest initiates a manual or webhook pull request review.
type TriggerReviewRequest struct {
	RepositoryID uuid.UUID `json:"repository_id"`
	PullNumber   int       `json:"pull_number"`
	HeadSHA      string    `json:"head_sha"`
	BaseSHA      string    `json:"base_sha"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	RawDiff      string    `json:"raw_diff,omitempty"`
}

// ReviewSummaryResponse models the high-level review status and metrics.
type ReviewSummaryResponse struct {
	ReviewID      uuid.UUID            `json:"review_id"`
	RepositoryID  uuid.UUID            `json:"repository_id"`
	PullNumber    int                  `json:"pull_number"`
	Title         string               `json:"title"`
	Status        models.ReviewState   `json:"status"`
	Verdict       string               `json:"verdict"`
	FindingsCount int                  `json:"findings_count"`
	CriticalCount int                  `json:"critical_count"`
	HighCount     int                  `json:"high_count"`
	MediumCount   int                  `json:"medium_count"`
	LowCount      int                  `json:"low_count"`
	CreatedAt     time.Time            `json:"created_at"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
	Findings      []models.CodeFinding `json:"findings,omitempty"`
}

// DismissFindingRequest records a developer dismissing a specific finding.
type DismissFindingRequest struct {
	Reason string `json:"reason"`
}
