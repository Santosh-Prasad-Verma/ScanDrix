package domain

import (
	"context"

	"github.com/google/uuid"
)

// AuditLogRepository contract for enterprise compliance tracking.
type AuditLogRepository interface {
	Record(ctx context.Context, log *AuditLogRecord) error
	Query(ctx context.Context, wsID uuid.UUID, targetResource string, pq PaginationQuery) (*PaginatedResult[*AuditLogRecord], error)
}

// OrganizationAndTeamData mirrors ScanDrix OrganizationAndTeamData structure.
type OrganizationAndTeamData struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	TeamID         uuid.UUID `json:"teamId"`
}

// ChangedFileSummary mirrors PR file summary.
type ChangedFileSummary struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Status           string `json:"status"` // "added", "modified", "deleted"
}

// PullRequestClosedEvent mirrors ScanDrix PullRequestClosedEvent.
type PullRequestClosedEvent struct {
	OrganizationAndTeamData OrganizationAndTeamData `json:"organizationAndTeamData"`
	Repository              struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"fullName,omitempty"`
	} `json:"repository"`
	PullRequestNumber int                  `json:"pullRequestNumber"`
	Files             []ChangedFileSummary `json:"files,omitempty"`
	Merged            bool                 `json:"merged"`
}
