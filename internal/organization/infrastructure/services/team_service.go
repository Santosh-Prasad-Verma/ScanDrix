// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	teamdomain "github.com/scandrix/backend/internal/organization/domain/team"
)

// TeamService implements teamdomain.ITeamService.
type TeamService struct {
	repo teamdomain.ITeamRepository
}

// NewTeamService creates a new TeamService.
func NewTeamService(repo teamdomain.ITeamRepository) *TeamService {
	return &TeamService{repo: repo}
}

func (s *TeamService) Find(ctx context.Context, filter teamdomain.TeamFilter) ([]*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Find(ctx, filter)
}

func (s *TeamService) FindOne(ctx context.Context, filter teamdomain.TeamFilter) (*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindOne(ctx, filter)
}

func (s *TeamService) FindByID(ctx context.Context, id uuid.UUID) (*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *TeamService) FindByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByWorkspaceID(ctx, workspaceID)
}

func (s *TeamService) GetTeamsByUserID(ctx context.Context, userID, workspaceID uuid.UUID) ([]*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.GetTeamsByUserID(ctx, userID, workspaceID)
}

func (s *TeamService) FindFirstCreatedTeam(ctx context.Context, workspaceID uuid.UUID) (*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindFirstCreatedTeam(ctx, workspaceID)
}

func (s *TeamService) Create(ctx context.Context, entity *teamdomain.TeamEntity) (*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *TeamService) Update(ctx context.Context, filter teamdomain.TeamFilter, data *teamdomain.TeamEntity) (*teamdomain.TeamEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, filter, data)
}

func (s *TeamService) Delete(ctx context.Context, id uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Delete(ctx, id)
}

func (s *TeamService) ListWithIntegrations(ctx context.Context, workspaceID uuid.UUID) ([]*teamdomain.TeamWithIntegrations, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.ListWithIntegrations(ctx, workspaceID)
}

// CreateTeam provisions a new team within a workspace.
func (s *TeamService) CreateTeam(ctx context.Context, wsID uuid.UUID, name, description string) (*teamdomain.TeamEntity, error) {
	if wsID == uuid.Nil || name == "" {
		return nil, errors.New("workspace ID and name are required")
	}
	entity := teamdomain.NewTeamEntity(wsID, name, description, "round_robin")
	return s.Create(ctx, entity)
}
