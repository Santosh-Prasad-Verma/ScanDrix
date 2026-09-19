// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package paramusecases

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/organization/domain/parameters"
)

type cacheEntry struct {
	entity    *paramdomain.ParametersEntity
	expiresAt time.Time
}

// FindByKeyParametersUseCase reads parameter configs with short-lived TTL in-memory caching.
type FindByKeyParametersUseCase struct {
	repo       paramdomain.IParametersRepository
	cache      map[string]cacheEntry
	mu         sync.RWMutex
	defaultTTL time.Duration
}

func NewFindByKeyParametersUseCase(repo paramdomain.IParametersRepository) *FindByKeyParametersUseCase {
	return &FindByKeyParametersUseCase{
		repo:       repo,
		cache:      make(map[string]cacheEntry),
		defaultTTL: 60 * time.Second,
	}
}

func (uc *FindByKeyParametersUseCase) getCacheKey(wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) string {
	tID := "no-team"
	if teamID != nil {
		tID = teamID.String()
	}
	return fmt.Sprintf("%s:%s:%s", key, wsID.String(), tID)
}

func (uc *FindByKeyParametersUseCase) Execute(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) (*paramdomain.ParametersEntity, error) {
	cacheKey := uc.getCacheKey(wsID, teamID, key)

	uc.mu.RLock()
	entry, found := uc.cache[cacheKey]
	uc.mu.RUnlock()

	if found && time.Now().Before(entry.expiresAt) {
		return entry.entity, nil
	}

	if uc.repo == nil {
		return nil, nil
	}

	entity, err := uc.repo.FindByKey(ctx, wsID, teamID, key)
	if err != nil || entity == nil {
		return nil, err
	}

	uc.mu.Lock()
	uc.cache[cacheKey] = cacheEntry{
		entity:    entity,
		expiresAt: time.Now().Add(uc.defaultTTL),
	}
	uc.mu.Unlock()

	return entity, nil
}
