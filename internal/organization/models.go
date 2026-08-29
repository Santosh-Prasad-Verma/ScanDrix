package organization

import (
	"time"

	"github.com/google/uuid"
)

// WorkspaceTenant represents an isolated enterprise organization.
type WorkspaceTenant struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"` // starter, growth, enterprise
	MaxSeats  int       `json:"max_seats"`
	UsedSeats int       `json:"used_seats"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Team represents a group of developers within a workspace.
type Team struct {
	ID             uuid.UUID   `json:"id"`
	WorkspaceID    uuid.UUID   `json:"workspace_id"`
	Name           string      `json:"name"`
	RepositoryIDs  []uuid.UUID `json:"repository_ids"`
	AutoAssignMode string      `json:"auto_assign_mode"` // round_robin, least_busy, none
	CreatedAt      time.Time   `json:"created_at"`
}

// TeamMember links a user to a team with a specific role.
type TeamMember struct {
	ID          uuid.UUID `json:"id"`
	TeamID      uuid.UUID `json:"team_id"`
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"` // lead, member
	ReviewCount int       `json:"review_count"`
	JoinedAt    time.Time `json:"joined_at"`
}

// MemberInvitation represents a pending invite to join a workspace.
type MemberInvitation struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	InviteToken string    `json:"invite_token"`
	Accepted    bool      `json:"accepted"`
	ExpiresAt   time.Time `json:"expires_at"`
}
