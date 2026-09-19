// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"time"

	"github.com/google/uuid"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// AddTeamMemberDTO represents a request to attach a member to a team.
type AddTeamMemberDTO struct {
	UserID uuid.UUID                   `json:"userId"`
	Email  string                      `json:"email"`
	Role   memberdomain.TeamMemberRole `json:"role"`
}

// InviteTeamMembersDTO represents a bulk onboarding invitation request.
type InviteTeamMembersDTO struct {
	Members []memberdomain.MemberInvitation `json:"members"`
}

// MemberItemDTO represents a member item for bulk updates and formatted listings.
type MemberItemDTO struct {
	UUID              *uuid.UUID                                  `json:"uuid,omitempty"`
	Active            bool                                        `json:"active"`
	CommunicationID   string                                      `json:"communicationId,omitempty"`
	TeamRole          memberdomain.TeamMemberRole                 `json:"teamRole"`
	Role              string                                      `json:"role,omitempty"`
	Avatar            string                                      `json:"avatar,omitempty"`
	Name              string                                      `json:"name,omitempty"`
	Communication     *memberdomain.CommunicationMemberConfig     `json:"communication,omitempty"`
	CodeManagement    *memberdomain.CodeManagementMemberConfig    `json:"codeManagement,omitempty"`
	ProjectManagement *memberdomain.ProjectManagementMemberConfig `json:"projectManagement,omitempty"`
	Email             string                                      `json:"email"`
	UserID            *uuid.UUID                                  `json:"userId,omitempty"`
	UserExists        bool                                        `json:"userExists,omitempty"`
	UserStatus        string                                      `json:"userStatus,omitempty"`
}

// UpdateOrCreateTeamMembersDTO represents payload for POST /team-members.
type UpdateOrCreateTeamMembersDTO struct {
	TeamID  string          `json:"teamId"`
	Members []MemberItemDTO `json:"members"`
}

// UpdateOrCreateTeamMembersResponseDTO represents member upsert response payload.
type UpdateOrCreateTeamMembersResponseDTO struct {
	Success bool                        `json:"success"`
	Results []memberdomain.InviteResult `json:"results"`
}

// TeamMembersListResponseDTO represents team members listing response.
type TeamMembersListResponseDTO struct {
	Members []MemberItemDTO `json:"members"`
}

// TeamMemberResponseDTO represents a team member profile.
type TeamMemberResponseDTO struct {
	ID                uuid.UUID                                   `json:"id"`
	TeamID            uuid.UUID                                   `json:"teamId"`
	UserID            uuid.UUID                                   `json:"userId"`
	Email             string                                      `json:"email"`
	Name              string                                      `json:"name"`
	Role              memberdomain.TeamMemberRole                 `json:"role"`
	Avatar            string                                      `json:"avatar"`
	CodeManagement    *memberdomain.CodeManagementMemberConfig    `json:"codeManagement,omitempty"`
	Communication     *memberdomain.CommunicationMemberConfig     `json:"communication,omitempty"`
	ProjectManagement *memberdomain.ProjectManagementMemberConfig `json:"projectManagement,omitempty"`
	CommunicationID   string                                      `json:"communicationId,omitempty"`
	ReviewCount       int                                         `json:"reviewCount"`
	JoinedAt          time.Time                                   `json:"joinedAt"`
}
