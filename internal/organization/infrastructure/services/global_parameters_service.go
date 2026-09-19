// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	globalparamdomain "github.com/scandrix/backend/internal/organization/domain/globalparameters"
)

// GlobalParametersService implements globalparamdomain.IGlobalParametersService.
type GlobalParametersService struct {
	repo globalparamdomain.IGlobalParametersRepository
}

// NewGlobalParametersService creates a new GlobalParametersService.
func NewGlobalParametersService(repo globalparamdomain.IGlobalParametersRepository) *GlobalParametersService {
	return &GlobalParametersService{repo: repo}
}

func (s *GlobalParametersService) FindByID(ctx context.Context, id uuid.UUID) (*globalparamdomain.GlobalParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *GlobalParametersService) FindByKey(ctx context.Context, key string) (*globalparamdomain.GlobalParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKey(ctx, key)
}

func (s *GlobalParametersService) FindUpdatedAtByKey(ctx context.Context, key string) (*time.Time, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindUpdatedAtByKey(ctx, key)
}

func (s *GlobalParametersService) Delete(ctx context.Context, id uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Delete(ctx, id)
}

func (s *GlobalParametersService) Create(ctx context.Context, entity *globalparamdomain.GlobalParametersEntity) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *GlobalParametersService) Update(ctx context.Context, entity *globalparamdomain.GlobalParametersEntity) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, entity)
}

func (s *GlobalParametersService) List(ctx context.Context) ([]*globalparamdomain.GlobalParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.List(ctx)
}

// CreateOrUpdateConfig creates or updates system-wide parameters.
func (s *GlobalParametersService) CreateOrUpdateConfig(ctx context.Context, key string, val any, desc string) (*globalparamdomain.GlobalParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}

	existing, err := s.FindByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		entity, err := globalparamdomain.NewGlobalParametersEntity(key, val, desc)
		if err != nil {
			return nil, err
		}
		if err := s.Create(ctx, entity); err != nil {
			return nil, err
		}
		return entity, nil
	}

	existing.ConfigValue = bytes
	if desc != "" {
		existing.Description = desc
	}
	if err := s.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}
