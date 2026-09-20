// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package clidevice

import (
	"time"

	"github.com/google/uuid"
)

// CliDeviceEntity tracks developer machines authorized to run reviews.
type CliDeviceEntity struct {
	UUID            uuid.UUID  `json:"uuid"`
	WorkspaceID     uuid.UUID  `json:"workspace_id"`
	UserID          *uuid.UUID `json:"user_id,omitempty"`
	DeviceID        string     `json:"device_id"`
	DeviceTokenHash string     `json:"device_token_hash"`
	UserAgent       string     `json:"user_agent"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// DeviceValidationResult returns a re-issued device token if needed.
type DeviceValidationResult struct {
	DeviceToken string `json:"device_token,omitempty"`
}
