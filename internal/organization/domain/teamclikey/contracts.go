// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package clikey

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ITeamCliKeyRepository defines persistence for CLI API keys.
type ITeamCliKeyRepository interface {
	Create(ctx context.Context, entity *TeamCliKeyEntity) error
	FindByKeyPrefix(ctx context.Context, prefix string) (*TeamCliKeyEntity, error)
	ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*TeamCliKeyEntity, error)
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	UpdateConfig(ctx context.Context, wsID, keyID uuid.UUID, config []byte) error
	Revoke(ctx context.Context, wsID, keyID uuid.UUID) error
}

// ITeamCliKeyService defines the business operations for team CLI keys.
type ITeamCliKeyService interface {
	ITeamCliKeyRepository
	GenerateKey(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, name string, config []byte, expiresAt *time.Time) (rawKey string, entity *TeamCliKeyEntity, err error)
	ValidateKey(ctx context.Context, rawKey string) (*ValidateKeyResult, error)
}
