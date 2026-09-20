// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberusecases

import (
	"context"
	"errors"

	"github.com/google/uuid"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// CreateOrUpdateTeamMembersUseCase handles batch member additions, updates, and invites.
type CreateOrUpdateTeamMembersUseCase struct {
	memberService memberdomain.ITeamMembersService
	memberRepo    memberdomain.ITeamMembersRepository
}

// NewCreateOrUpdateTeamMembersUseCase instantiates the use case with service and repository dependencies.
func NewCreateOrUpdateTeamMembersUseCase(memberRepo memberdomain.ITeamMembersRepository) *CreateOrUpdateTeamMembersUseCase {
	var service memberdomain.ITeamMembersService
	if s, ok := memberRepo.(memberdomain.ITeamMembersService); ok {
		service = s
	}
	return &CreateOrUpdateTeamMembersUseCase{
		memberRepo:    memberRepo,
		memberService: service,
	}
}

// WithService allows injecting an explicit ITeamMembersService.
func (uc *CreateOrUpdateTeamMembersUseCase) WithService(service memberdomain.ITeamMembersService) *CreateOrUpdateTeamMembersUseCase {
	uc.memberService = service
	return uc
}

// Execute performs full batch creation or update of team members with identity integration mappings.
func (uc *CreateOrUpdateTeamMembersUseCase) Execute(ctx context.Context, wsID, teamID uuid.UUID, members []memberdomain.MemberItem, inviterEmail string) (*memberdomain.UpdateOrCreateMembersResponse, error) {
	if wsID == uuid.Nil || teamID == uuid.Nil {
		return nil, errors.New("workspace ID and team ID are required")
	}

	if uc.memberService != nil {
		return uc.memberService.UpdateOrCreateMembers(ctx, wsID, teamID, members, inviterEmail)
	}

	if uc.memberRepo == nil {
		return nil, errors.New("team members repository unavailable")
	}

	results := make([]memberdomain.InviteResult, 0, len(members))
	for _, m := range members {
		if m.Email == "" {
			continue
		}

		existing, _ := uc.memberRepo.FindOne(ctx, memberdomain.TeamMemberFilter{
			TeamID: &teamID,
			Email:  &m.Email,
		})

		if existing != nil {
			existing.Status = m.Active
			existing.Role = m.TeamRole
			if m.Name != "" {
				existing.Name = m.Name
			}
			if m.Avatar != "" {
				existing.Avatar = m.Avatar
			}
			if m.CommunicationID != "" {
				existing.CommunicationID = m.CommunicationID
			}
			if m.Communication != nil {
				existing.Communication = m.Communication
			}
			if m.CodeManagement != nil {
				existing.CodeManagement = m.CodeManagement
			}
			if m.ProjectManagement != nil {
				existing.ProjectManagement = m.ProjectManagement
			}

			_, err := uc.memberRepo.Create(ctx, existing)
			if err != nil {
				results = append(results, memberdomain.InviteResult{
					Email:   m.Email,
					Status:  memberdomain.StatusError,
					Message: err.Error(),
				})
			} else {
				results = append(results, memberdomain.InviteResult{
					Email:   m.Email,
					Status:  memberdomain.StatusInviteSent,
					UUID:    &existing.UUID,
					Message: "Member updated successfully",
				})
			}
		} else {
			userID := uuid.New()
			if m.UserID != nil && *m.UserID != uuid.Nil {
				userID = *m.UserID
			}
			name := m.Name
			if name == "" {
				name = m.Email
			}
			role := m.TeamRole
			if role == "" {
				role = memberdomain.RoleMember
			}

			newEntity := memberdomain.NewTeamMemberEntity(wsID, teamID, userID, m.Email, name, role)
			newEntity.Status = m.Active
			newEntity.Avatar = m.Avatar
			newEntity.CommunicationID = m.CommunicationID
			newEntity.Communication = m.Communication
			newEntity.CodeManagement = m.CodeManagement
			newEntity.ProjectManagement = m.ProjectManagement

			created, err := uc.memberRepo.Create(ctx, newEntity)
			if err != nil {
				results = append(results, memberdomain.InviteResult{
					Email:   m.Email,
					Status:  memberdomain.StatusError,
					Message: err.Error(),
				})
			} else {
				results = append(results, memberdomain.InviteResult{
					Email:   m.Email,
					Status:  memberdomain.StatusInviteSent,
					UUID:    &created.UUID,
					Message: "Invite sent successfully",
				})
			}
		}
	}

	return &memberdomain.UpdateOrCreateMembersResponse{
		Success: true,
		Results: results,
	}, nil
}

// ExecuteInvitations handles onboarding invitation slices for backward compatibility with older controller methods.
func (uc *CreateOrUpdateTeamMembersUseCase) ExecuteInvitations(ctx context.Context, wsID, teamID uuid.UUID, invites []memberdomain.MemberInvitation, inviterEmail string) ([]memberdomain.InviteResult, error) {
	members := make([]memberdomain.MemberItem, 0, len(invites))
	for _, inv := range invites {
		members = append(members, memberdomain.MemberItem{
			Email:    inv.Email,
			TeamRole: inv.Role,
			Active:   true,
		})
	}
	resp, err := uc.Execute(ctx, wsID, teamID, members, inviterEmail)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}
