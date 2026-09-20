// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberdomain

import (
	"time"

	"github.com/google/uuid"
)

// TeamMemberEntity represents a user's membership and integration profile in a team.
type TeamMemberEntity struct {
	UUID              uuid.UUID                      `json:"uuid"`
	WorkspaceID       uuid.UUID                      `json:"workspace_id"`
	TeamID            uuid.UUID                      `json:"team_id"`
	UserID            uuid.UUID                      `json:"user_id"`
	Email             string                         `json:"email"`
	Name              string                         `json:"name"`
	Role              TeamMemberRole                 `json:"role"`
	Avatar            string                         `json:"avatar"`
	Status            bool                           `json:"status"`
	CodeManagement    *CodeManagementMemberConfig    `json:"code_management,omitempty"`
	Communication     *CommunicationMemberConfig     `json:"communication,omitempty"`
	ProjectManagement *ProjectManagementMemberConfig `json:"project_management,omitempty"`
	CommunicationID   string                         `json:"communication_id,omitempty"`
	ReviewCount       int                            `json:"review_count"`
	JoinedAt          time.Time                      `json:"joined_at"`
}

// NewTeamMemberEntity creates a new TeamMemberEntity.
func NewTeamMemberEntity(wsID, teamID, userID uuid.UUID, email, name string, role TeamMemberRole) *TeamMemberEntity {
	if role == "" {
		role = RoleMember
	}
	return &TeamMemberEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		TeamID:      teamID,
		UserID:      userID,
		Email:       email,
		Name:        name,
		Role:        role,
		Status:      true,
		ReviewCount: 0,
		JoinedAt:    time.Now().UTC(),
	}
}
