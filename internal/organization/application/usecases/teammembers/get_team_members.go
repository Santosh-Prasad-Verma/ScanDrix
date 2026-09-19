// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberusecases

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	memberdomain "github.com/scandrix/backend/internal/organization/domain/teammembers"
)

// GetTeamMembersUseCase retrieves formatted, active team members sorted by email.
type GetTeamMembersUseCase struct {
	memberRepo    memberdomain.ITeamMembersRepository
	memberService memberdomain.ITeamMembersService
}

// NewGetTeamMembersUseCase instantiates a new GetTeamMembersUseCase.
func NewGetTeamMembersUseCase(memberRepo memberdomain.ITeamMembersRepository) *GetTeamMembersUseCase {
	var service memberdomain.ITeamMembersService
	if s, ok := memberRepo.(memberdomain.ITeamMembersService); ok {
		service = s
	}
	return &GetTeamMembersUseCase{
		memberRepo:    memberRepo,
		memberService: service,
	}
}

// WithService sets the team members service.
func (uc *GetTeamMembersUseCase) WithService(service memberdomain.ITeamMembersService) *GetTeamMembersUseCase {
	uc.memberService = service
	return uc
}

// Execute fetches and returns sorted members for the given workspace and optional team.
func (uc *GetTeamMembersUseCase) Execute(ctx context.Context, wsID, teamID uuid.UUID, activeOnly bool) ([]*memberdomain.TeamMemberEntity, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}
	if uc.memberRepo == nil {
		return nil, errors.New("team members repository unavailable")
	}

	filter := memberdomain.TeamMemberFilter{
		WorkspaceID: &wsID,
	}
	if teamID != uuid.Nil {
		filter.TeamID = &teamID
	}
	if activeOnly {
		activeStatus := true
		filter.Status = &activeStatus
	}

	members, err := uc.memberRepo.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Sort members by email alphabetically (empty emails at the end)
	sort.Slice(members, func(i, j int) bool {
		emailA := members[i].Email
		emailB := members[j].Email
		if emailA != "" && emailB == "" {
			return true
		}
		if emailA == "" && emailB != "" {
			return false
		}
		return strings.ToLower(emailA) < strings.ToLower(emailB)
	})

	return members, nil
}

// ExecuteFormatted fetches and returns sorted MemberItem DTO list.
func (uc *GetTeamMembersUseCase) ExecuteFormatted(ctx context.Context, wsID, teamID uuid.UUID) ([]memberdomain.MemberItem, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}

	if uc.memberService != nil {
		activeStatus := true
		items, err := uc.memberService.FindTeamMembersFormatted(ctx, wsID, teamID, &activeStatus)
		if err == nil {
			sort.Slice(items, func(i, j int) bool {
				emailA := items[i].Email
				emailB := items[j].Email
				if emailA != "" && emailB == "" {
					return true
				}
				if emailA == "" && emailB != "" {
					return false
				}
				return strings.ToLower(emailA) < strings.ToLower(emailB)
			})
			return items, nil
		}
	}

	// Fallback via repository
	entities, err := uc.Execute(ctx, wsID, teamID, true)
	if err != nil {
		return nil, err
	}

	items := make([]memberdomain.MemberItem, 0, len(entities))
	for _, m := range entities {
		items = append(items, memberdomain.MemberItem{
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
		})
	}
	return items, nil
}
