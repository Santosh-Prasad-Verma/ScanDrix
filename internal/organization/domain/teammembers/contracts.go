// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberdomain

import (
	"context"

	"github.com/google/uuid"
)

// TeamMemberFilter allows querying team members.
type TeamMemberFilter struct {
	UUID        *uuid.UUID      `json:"uuid,omitempty"`
	WorkspaceID *uuid.UUID      `json:"workspace_id,omitempty"`
	TeamID      *uuid.UUID      `json:"team_id,omitempty"`
	UserID      *uuid.UUID      `json:"user_id,omitempty"`
	Email       *string         `json:"email,omitempty"`
	Role        *TeamMemberRole `json:"role,omitempty"`
	Status      *bool           `json:"status,omitempty"`
}

// ITeamMembersRepository defines data access for team members.
type ITeamMembersRepository interface {
	Find(ctx context.Context, filter TeamMemberFilter) ([]*TeamMemberEntity, error)
	FindOne(ctx context.Context, filter TeamMemberFilter) (*TeamMemberEntity, error)
	FindByID(ctx context.Context, wsID, id uuid.UUID) (*TeamMemberEntity, error)
	FindManyByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*TeamMemberEntity, error)
	FindManyByUserID(ctx context.Context, wsID, userID uuid.UUID) ([]*TeamMemberEntity, error)
	FindMembersByCommunicationID(ctx context.Context, wsID uuid.UUID, communicationID string) ([]*TeamMemberEntity, error)
	CountByUser(ctx context.Context, wsID, userID uuid.UUID, teamMemberStatus *bool) (int, error)
	CountTeamMembers(ctx context.Context, wsID, teamID uuid.UUID) (int, error)
	FindManyByOrganizationID(ctx context.Context, wsID uuid.UUID, teamStatus []string) ([]*TeamMemberEntity, error)
	Create(ctx context.Context, entity *TeamMemberEntity) (*TeamMemberEntity, error)
	Update(ctx context.Context, filter TeamMemberFilter, data *TeamMemberEntity) (*TeamMemberEntity, error)
	Delete(ctx context.Context, wsID, teamID, userID uuid.UUID) error
	DeleteMembers(ctx context.Context, wsID uuid.UUID, memberUUIDs []uuid.UUID) error
}

// ITeamMembersService defines business operations for team members.
type ITeamMembersService interface {
	ITeamMembersRepository
	InviteMembers(ctx context.Context, wsID, teamID uuid.UUID, invites []MemberInvitation, inviterEmail string) ([]InviteResult, error)
	FindTeamMembersFormatted(ctx context.Context, wsID, teamID uuid.UUID, teamMembersStatus *bool) ([]MemberItem, error)
	UpdateOrCreateMembers(ctx context.Context, wsID, teamID uuid.UUID, members []MemberItem, inviterEmail string) (*UpdateOrCreateMembersResponse, error)
}
