package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// CreateTeamRequest defines parameters for organizing developers into functional teams.
type CreateTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TeamResponse returns team summary information.
type TeamResponse struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// AddTeamMemberRequest invites or binds a developer to a team.
type AddTeamMemberRequest struct {
	Email string          `json:"email"`
	Role  models.UserRole `json:"role"`
}

// TeamMemberResponse details an individual member's status on a team.
type TeamMemberResponse struct {
	ID          uuid.UUID       `json:"id"`
	TeamID      uuid.UUID       `json:"team_id"`
	UserID      uuid.UUID       `json:"user_id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        models.UserRole `json:"role"`
	JoinedAt    time.Time       `json:"joined_at"`
}
