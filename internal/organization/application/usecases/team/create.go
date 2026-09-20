// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package teamusecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/parameters"
	"github.com/scandrix/backend/internal/organization/domain/team"
)

// CreateTeamUseCase provisions a new team and initializes its required parameter records.
type CreateTeamUseCase struct {
	teamRepo  teamdomain.ITeamRepository
	paramRepo paramdomain.IParametersRepository
}

func NewCreateTeamUseCase(teamRepo teamdomain.ITeamRepository, paramRepo paramdomain.IParametersRepository) *CreateTeamUseCase {
	return &CreateTeamUseCase{
		teamRepo:  teamRepo,
		paramRepo: paramRepo,
	}
}

func (uc *CreateTeamUseCase) Execute(ctx context.Context, wsID uuid.UUID, name, description, autoAssignMode string) (*teamdomain.TeamEntity, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}
	if name == "" {
		return nil, errors.New("team name is required")
	}
	if uc.teamRepo == nil {
		return nil, errors.New("team repository unavailable")
	}

	// Check name conflict
	existingTeams, err := uc.teamRepo.Find(ctx, teamdomain.TeamFilter{
		WorkspaceID: &wsID,
		Name:        &name,
	})
	if err == nil && len(existingTeams) > 0 {
		return nil, fmt.Errorf("team with name %q already exists in workspace", name)
	}

	team := teamdomain.NewTeamEntity(wsID, name, description, autoAssignMode)
	createdTeam, err := uc.teamRepo.Create(ctx, team)
	if err != nil {
		return nil, err
	}

	// Bootstrap default platform_configs and code_review_config for the new team
	if uc.paramRepo != nil && createdTeam != nil {
		platformVal := paramdomain.PlatformConfigValue{
			FinishOnboard:                     false,
			FinishProjectManagementConnection: false,
			DrixyLearningStatus:               paramdomain.DrixyLearningStatusEnabled,
		}
		reviewVal := paramdomain.CodeReviewConfigValue{
			MaxFiles:               50,
			IgnoredPaths:           []string{"vendor/**", "node_modules/**"},
			IncludedPaths:          []string{"**/*"},
			AutomatedReviewLabels:  []string{"scandrix-review"},
			SeverityThreshold:      "medium",
			EnableInlineSuggestions: true,
			DrixyRulesEnabled:      true,
		}

		p1, _ := paramdomain.NewParametersEntity(wsID, &createdTeam.UUID, paramdomain.KeyPlatformConfigs, platformVal, "Initial team platform config")
		if p1 != nil {
			_, _ = uc.paramRepo.Create(ctx, p1)
		}
		p2, _ := paramdomain.NewParametersEntity(wsID, &createdTeam.UUID, paramdomain.KeyCodeReviewConfig, reviewVal, "Initial team code review config")
		if p2 != nil {
			_, _ = uc.paramRepo.Create(ctx, p2)
		}
	}

	return createdTeam, nil
}
