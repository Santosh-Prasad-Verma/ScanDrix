// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
)

type cacheEntry struct {
	entity    *paramdomain.ParametersEntity
	expiresAt time.Time
}

// ParametersService implements paramdomain.IParametersService.
type ParametersService struct {
	repo     paramdomain.IParametersRepository
	cacheMu  sync.RWMutex
	cache    map[string]cacheEntry
	cacheTTL time.Duration
}

// NewParametersService creates a new ParametersService.
func NewParametersService(repo paramdomain.IParametersRepository) *ParametersService {
	return &ParametersService{
		repo:     repo,
		cache:    make(map[string]cacheEntry),
		cacheTTL: 5 * time.Minute,
	}
}

func cacheKey(wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) string {
	tID := "nil"
	if teamID != nil {
		tID = teamID.String()
	}
	return fmt.Sprintf("%s:%s:%s", wsID.String(), tID, string(key))
}

func (s *ParametersService) Find(ctx context.Context, filter paramdomain.ParametersFilter) ([]*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Find(ctx, filter)
}

func (s *ParametersService) FindOne(ctx context.Context, filter paramdomain.ParametersFilter) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindOne(ctx, filter)
}

func (s *ParametersService) FindByID(ctx context.Context, id uuid.UUID) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByID(ctx, id)
}

func (s *ParametersService) FindByKey(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKey(ctx, wsID, teamID, key)
}

func (s *ParametersService) Create(ctx context.Context, entity *paramdomain.ParametersEntity) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	res, err := s.repo.Create(ctx, entity)
	if err == nil && res != nil {
		s.cacheMu.Lock()
		delete(s.cache, cacheKey(res.WorkspaceID, res.TeamID, res.ConfigKey))
		s.cacheMu.Unlock()
	}
	return res, err
}

func (s *ParametersService) Update(ctx context.Context, filter paramdomain.ParametersFilter, data *paramdomain.ParametersEntity) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.Update(ctx, filter, data)
}

func (s *ParametersService) Delete(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	err := s.repo.Delete(ctx, wsID, teamID, key)
	if err == nil {
		s.cacheMu.Lock()
		delete(s.cache, cacheKey(wsID, teamID, key))
		s.cacheMu.Unlock()
	}
	return err
}

func (s *ParametersService) DeleteByTeamID(ctx context.Context, teamID uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.DeleteByTeamID(ctx, teamID)
}

// FindByKeyCached returns cached parameter entity or queries database with TTL refresh.
func (s *ParametersService) FindByKeyCached(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) (*paramdomain.ParametersEntity, error) {
	k := cacheKey(wsID, teamID, key)
	s.cacheMu.RLock()
	entry, exists := s.cache[k]
	s.cacheMu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		return entry.entity, nil
	}

	entity, err := s.FindByKey(ctx, wsID, teamID, key)
	if err != nil {
		return nil, err
	}

	if entity != nil {
		s.cacheMu.Lock()
		s.cache[k] = cacheEntry{
			entity:    entity,
			expiresAt: time.Now().Add(s.cacheTTL),
		}
		s.cacheMu.Unlock()
	}
	return entity, nil
}

// CreateOrUpdateConfig creates or updates configuration for a given key.
func (s *ParametersService) CreateOrUpdateConfig(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any, desc string) (*paramdomain.ParametersEntity, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}

	existing, err := s.FindByKey(ctx, wsID, teamID, key)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		entity, err := paramdomain.NewParametersEntity(wsID, teamID, key, val, desc)
		if err != nil {
			return nil, err
		}
		return s.Create(ctx, entity)
	}

	existing.ConfigValue = bytes
	if desc != "" {
		existing.Description = desc
	}
	return s.Create(ctx, existing)
}

// CreateNewActiveVersion deactivates older active rows and creates a brand-new versioned row.
func (s *ParametersService) CreateNewActiveVersion(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any, nextVersion int) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	res, err := s.repo.CreateNewActiveVersion(ctx, wsID, teamID, key, val, nextVersion)
	if err == nil && res != nil {
		s.cacheMu.Lock()
		delete(s.cache, cacheKey(res.WorkspaceID, res.TeamID, res.ConfigKey))
		s.cacheMu.Unlock()
	}
	return res, err
}

// CreateActiveVersionIfAbsent ensures an initial active parameter version exists.
func (s *ParametersService) CreateActiveVersionIfAbsent(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any) (*paramdomain.ParametersEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	res, err := s.repo.CreateActiveVersionIfAbsent(ctx, wsID, teamID, key, val)
	if err == nil && res != nil {
		s.cacheMu.Lock()
		delete(s.cache, cacheKey(res.WorkspaceID, res.TeamID, res.ConfigKey))
		s.cacheMu.Unlock()
	}
	return res, err
}
