// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package clidevice

import (
	"context"

	"github.com/google/uuid"
)

// ICliDeviceRepository defines persistence for CLI hardware devices.
type ICliDeviceRepository interface {
	FindOne(ctx context.Context, wsID uuid.UUID, deviceID string) (*CliDeviceEntity, error)
	CountByWorkspaceID(ctx context.Context, wsID uuid.UUID) (int, error)
	Create(ctx context.Context, entity *CliDeviceEntity) error
	UpdateLastSeen(ctx context.Context, id uuid.UUID, userAgent string) error
	UpdateTokenHash(ctx context.Context, id uuid.UUID, tokenHash string, userAgent string) error
}

// ValidateDeviceParams holds registration and verification parameters for developer CLI devices.
type ValidateDeviceParams struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	DeviceID    string     `json:"device_id"`
	DeviceToken string     `json:"device_token,omitempty"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	UserAgent   string     `json:"user_agent,omitempty"`
}

// ICliDeviceService manages registration and seat/device quota checks.
type ICliDeviceService interface {
	ValidateOrRegisterDevice(ctx context.Context, wsID uuid.UUID, deviceID, deviceToken, userAgent string) (*DeviceValidationResult, error)
	ValidateOrRegisterDeviceWithUser(ctx context.Context, params ValidateDeviceParams) (*DeviceValidationResult, error)
}
