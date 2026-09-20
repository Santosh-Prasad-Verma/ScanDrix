// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// UserOrganizationChecker checks whether a user with given email belongs to a different organization.
type UserOrganizationChecker interface {
	CheckUserInOtherOrganization(ctx context.Context, email string, wsID uuid.UUID) (bool, *uuid.UUID, error)
}

// TeamMembersService implements memberdomain.ITeamMembersService for enterprise team member management.
type TeamMembersService struct {
	repo        memberdomain.ITeamMembersRepository
	userChecker UserOrganizationChecker
}

// NewTeamMembersService creates a new TeamMembersService.
func NewTeamMembersService(repo memberdomain.ITeamMembersRepository) *TeamMembersService {
	return &TeamMembersService{repo: repo}
}

// WithUserChecker attaches cross-organization conflict validation.
func (s *TeamMembersService) WithUserChecker(checker UserOrganizationChecker) *TeamMembersService {
	s.userChecker = checker
	return s
}

func (s *TeamMembersService) Find(ctx context.Context, filter memberdomain.TeamMemberFilter) ([]*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Find(ctx, filter)
}

func (s *TeamMembersService) FindOne(ctx context.Context, filter memberdomain.TeamMemberFilter) (*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindOne(ctx, filter)
}

func (s *TeamMembersService) FindByID(ctx context.Context, id uuid.UUID) (*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *TeamMembersService) FindManyByWorkspaceID(ctx context.Context, wsID uuid.UUID) ([]*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindManyByWorkspaceID(ctx, wsID)
}

func (s *TeamMembersService) FindManyByUserID(ctx context.Context, userID uuid.UUID) ([]*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindManyByUserID(ctx, userID)
}

func (s *TeamMembersService) FindMembersByCommunicationID(ctx context.Context, communicationID string) ([]*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindMembersByCommunicationID(ctx, communicationID)
}

func (s *TeamMembersService) CountByUser(ctx context.Context, userID uuid.UUID, teamMemberStatus *bool) (int, error) {
	if s.repo == nil {
		return 0, errors.New("repository uninitialized")
	}
	return s.repo.CountByUser(ctx, userID, teamMemberStatus)
}

func (s *TeamMembersService) CountTeamMembers(ctx context.Context, wsID, teamID uuid.UUID) (int, error) {
	if s.repo == nil {
		return 0, errors.New("repository uninitialized")
	}
	return s.repo.CountTeamMembers(ctx, wsID, teamID)
}

func (s *TeamMembersService) FindManyByOrganizationID(ctx context.Context, wsID uuid.UUID, teamStatus []string) ([]*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindManyByOrganizationID(ctx, wsID, teamStatus)
}

func (s *TeamMembersService) Create(ctx context.Context, entity *memberdomain.TeamMemberEntity) (*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *TeamMembersService) Update(ctx context.Context, filter memberdomain.TeamMemberFilter, data *memberdomain.TeamMemberEntity) (*memberdomain.TeamMemberEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, filter, data)
}

func (s *TeamMembersService) Delete(ctx context.Context, teamID, userID uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Delete(ctx, teamID, userID)
}

func (s *TeamMembersService) DeleteMembers(ctx context.Context, memberUUIDs []uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.DeleteMembers(ctx, memberUUIDs)
}

// FindTeamMembersFormatted produces formatted MemberItem list.
func (s *TeamMembersService) FindTeamMembersFormatted(ctx context.Context, wsID, teamID uuid.UUID, teamMembersStatus *bool) ([]memberdomain.MemberItem, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}

	filter := memberdomain.TeamMemberFilter{
		WorkspaceID: &wsID,
		Status:      teamMembersStatus,
	}
	if teamID != uuid.Nil {
		filter.TeamID = &teamID
	}

	entities, err := s.repo.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	formatted := make([]memberdomain.MemberItem, 0, len(entities))
	for _, m := range entities {
		item := memberdomain.MemberItem{
			UUID:              &m.UUID,
			Active:            m.Status,
			CommunicationID:   m.CommunicationID,
			TeamRole:          m.Role,
			Role:              string(m.Role),
			Avatar:            m.Avatar,
			Name:              m.Name,
			Communication:     m.Communication,
			CodeManagement:    m.CodeManagement,
			ProjectManagement: m.ProjectManagement,
			Email:             m.Email,
			UserID:            &m.UserID,
			UserExists:        m.UserID != uuid.Nil,
			UserStatus:        "ACTIVE",
		}
		formatted = append(formatted, item)
	}

	return formatted, nil
}

// UpdateOrCreateMembers implements bulk upsert and onboarding.
func (s *TeamMembersService) UpdateOrCreateMembers(ctx context.Context, wsID, teamID uuid.UUID, members []memberdomain.MemberItem, inviterEmail string) (*memberdomain.UpdateOrCreateMembersResponse, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}

	results := make([]memberdomain.InviteResult, 0, len(members))

	// Fetch existing members in this workspace to resolve user IDs and prevent duplicates
	orgMembers, _ := s.repo.FindManyByWorkspaceID(ctx, wsID)
	orgMemberEmailMap := make(map[string]*memberdomain.TeamMemberEntity)
	for _, om := range orgMembers {
		emailKey := strings.ToLower(om.Email)
		if emailKey != "" {
			orgMemberEmailMap[emailKey] = om
		}
	}

	for _, member := range members {
		email := strings.TrimSpace(member.Email)
		if email == "" {
			continue
		}
		emailKey := strings.ToLower(email)

		// Check if user is already registered in another organization
		if s.userChecker != nil {
			inOther, otherUserID, _ := s.userChecker.CheckUserInOtherOrganization(ctx, email, wsID)
			if inOther {
				results = append(results, memberdomain.InviteResult{
					Email:   email,
					Status:  memberdomain.StatusUserAlreadyRegisteredInOtherOrg,
					UUID:    otherUserID,
					Message: "User already registered in another organization",
				})
				continue
			}
		}

		// Check if user already exists in this team
		existingTeamMember, _ := s.repo.FindOne(ctx, memberdomain.TeamMemberFilter{
			TeamID: &teamID,
			Email:  &email,
		})

		var userID uuid.UUID
		if member.UserID != nil && *member.UserID != uuid.Nil {
			userID = *member.UserID
		} else if existingTeamMember != nil && existingTeamMember.UserID != uuid.Nil {
			userID = existingTeamMember.UserID
		} else if orgMatch, exists := orgMemberEmailMap[emailKey]; exists && orgMatch.UserID != uuid.Nil {
			userID = orgMatch.UserID
		} else {
			userID = uuid.New()
		}

		name := member.Name
		if name == "" {
			name = strings.Split(email, "@")[0]
		}

		teamRole := member.TeamRole
		if teamRole == "" {
			teamRole = memberdomain.RoleMember
		}

		if existingTeamMember != nil {
			// Update existing membership
			existingTeamMember.Name = name
			existingTeamMember.Status = member.Active
			existingTeamMember.Role = teamRole
			if member.Avatar != "" {
				existingTeamMember.Avatar = member.Avatar
			}
			if member.CommunicationID != "" {
				existingTeamMember.CommunicationID = member.CommunicationID
			}
			if member.Communication != nil {
				existingTeamMember.Communication = member.Communication
			}
			if member.CodeManagement != nil {
				existingTeamMember.CodeManagement = member.CodeManagement
			}
			if member.ProjectManagement != nil {
				existingTeamMember.ProjectManagement = member.ProjectManagement
			}

			_, err := s.repo.Create(ctx, existingTeamMember)
			if err != nil {
				results = append(results, memberdomain.InviteResult{
					Email:   email,
					Status:  memberdomain.StatusError,
					Message: err.Error(),
				})
			} else {
				results = append(results, memberdomain.InviteResult{
					Email:   email,
					Status:  memberdomain.StatusAlreadyMember,
					UUID:    &existingTeamMember.UUID,
					Message: "Team member updated successfully",
				})
			}
		} else {
			// Create new membership
			newMember := memberdomain.NewTeamMemberEntity(wsID, teamID, userID, email, name, teamRole)
			newMember.Status = member.Active
			newMember.Avatar = member.Avatar
			newMember.CommunicationID = member.CommunicationID
			newMember.Communication = member.Communication
			newMember.CodeManagement = member.CodeManagement
			newMember.ProjectManagement = member.ProjectManagement

			created, err := s.repo.Create(ctx, newMember)
			if err != nil {
				results = append(results, memberdomain.InviteResult{
					Email:   email,
					Status:  memberdomain.StatusError,
					Message: err.Error(),
				})
			} else {
				results = append(results, memberdomain.InviteResult{
					Email:   email,
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

// InviteMembers processes invitations, creating or updating membership profiles.
func (s *TeamMembersService) InviteMembers(ctx context.Context, wsID, teamID uuid.UUID, invites []memberdomain.MemberInvitation, inviterEmail string) ([]memberdomain.InviteResult, error) {
	members := make([]memberdomain.MemberItem, 0, len(invites))
	for _, inv := range invites {
		members = append(members, memberdomain.MemberItem{
			Email:    inv.Email,
			TeamRole: inv.Role,
			Active:   true,
		})
	}

	resp, err := s.UpdateOrCreateMembers(ctx, wsID, teamID, members, inviterEmail)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}
