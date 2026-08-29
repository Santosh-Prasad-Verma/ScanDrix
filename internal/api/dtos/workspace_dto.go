package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// CreateWorkspaceRequest creates a new tenant workspace.
type CreateWorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// WorkspaceResponse returns workspace details.
type WorkspaceResponse struct {
	ID        uuid.UUID            `json:"id"`
	Name      string               `json:"name"`
	Slug      string               `json:"slug"`
	Status    models.TenantStatus  `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
}

// CockpitMetricsResponse provides executive security dashboard stats.
type CockpitMetricsResponse struct {
	TotalReviews       int     `json:"total_reviews"`
	TotalFindings      int     `json:"total_findings"`
	CriticalFindings   int     `json:"critical_findings"`
	HighFindings       int     `json:"high_findings"`
	PassRatePercentage float64 `json:"pass_rate_percentage"`
	ActiveRepositories int     `json:"active_repositories"`
	TotalDevelopers    int     `json:"total_developers"`
}
