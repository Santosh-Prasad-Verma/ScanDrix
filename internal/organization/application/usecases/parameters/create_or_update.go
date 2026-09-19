// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramusecases

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/parameters"
)

// CreateOrUpdateParametersUseCase persists workspace and team-scoped review configurations.
type CreateOrUpdateParametersUseCase struct {
	repo paramdomain.IParametersRepository
}

func NewCreateOrUpdateParametersUseCase(repo paramdomain.IParametersRepository) *CreateOrUpdateParametersUseCase {
	return &CreateOrUpdateParametersUseCase{repo: repo}
}

func (uc *CreateOrUpdateParametersUseCase) Execute(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any, desc string) (*paramdomain.ParametersEntity, error) {
	if wsID == uuid.Nil {
		return nil, errors.New("workspace ID is required")
	}
	if uc.repo == nil {
		return nil, errors.New("repository unavailable")
	}

	entity, err := paramdomain.NewParametersEntity(wsID, teamID, key, val, desc)
	if err != nil {
		return nil, err
	}

	existing, _ := uc.repo.FindByKey(ctx, wsID, teamID, key)
	if existing == nil {
		return uc.repo.Create(ctx, entity)
	}

	return uc.repo.Update(ctx, paramdomain.ParametersFilter{
		WorkspaceID: &wsID,
		TeamID:      teamID,
		ConfigKey:   &key,
	}, entity)
}
