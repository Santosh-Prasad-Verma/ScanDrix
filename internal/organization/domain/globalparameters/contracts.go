// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package globalparams

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// IGlobalParametersRepository defines persistence for platform-wide parameters.
type IGlobalParametersRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*GlobalParametersEntity, error)
	FindByKey(ctx context.Context, key string) (*GlobalParametersEntity, error)
	FindUpdatedAtByKey(ctx context.Context, key string) (*time.Time, error)
	Create(ctx context.Context, entity *GlobalParametersEntity) error
	Update(ctx context.Context, entity *GlobalParametersEntity) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*GlobalParametersEntity, error)
}

// IGlobalParametersService defines business operations for platform-wide parameters.
type IGlobalParametersService interface {
	IGlobalParametersRepository
	CreateOrUpdateConfig(ctx context.Context, key string, val any, desc string) (*GlobalParametersEntity, error)
}
