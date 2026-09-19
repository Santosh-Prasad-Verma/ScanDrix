// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	clidevicedomain "github.com/scandrix/backend/internal/organization/domain/clidevice"
)

// CliDeviceService implements clidevicedomain.ICliDeviceService.
type CliDeviceService struct {
	repo        clidevicedomain.ICliDeviceRepository
	deviceLimit int
}

// NewCliDeviceService creates a new CliDeviceService.
func NewCliDeviceService(repo clidevicedomain.ICliDeviceRepository) *CliDeviceService {
	limit := 0
	if limStr := os.Getenv("CLI_DEVICE_LIMIT"); limStr != "" {
		if parsed, err := strconv.Atoi(limStr); err == nil {
			limit = parsed
		}
	}
	return &CliDeviceService{
		repo:        repo,
		deviceLimit: limit,
	}
}

// ValidateOrRegisterDevice enforces hardware device quotas, updates device heartbeat, or issues/refreshes device tokens.
func (s *CliDeviceService) ValidateOrRegisterDevice(ctx context.Context, wsID uuid.UUID, deviceID, deviceToken, userAgent string) (*clidevicedomain.DeviceValidationResult, error) {
	return s.ValidateOrRegisterDeviceWithUser(ctx, clidevicedomain.ValidateDeviceParams{
		WorkspaceID: wsID,
		DeviceID:    deviceID,
		DeviceToken: deviceToken,
		UserAgent:   userAgent,
	})
}

// ValidateOrRegisterDeviceWithUser binds hardware devices to user accounts and recovers gracefully from concurrent registration races.
func (s *CliDeviceService) ValidateOrRegisterDeviceWithUser(ctx context.Context, params clidevicedomain.ValidateDeviceParams) (*clidevicedomain.DeviceValidationResult, error) {
	wsID := params.WorkspaceID
	deviceID := params.DeviceID
	deviceToken := params.DeviceToken
	userAgent := params.UserAgent

	if wsID == uuid.Nil || deviceID == "" {
		return nil, errors.New("workspace ID and device ID are required")
	}
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}

	existing, err := s.repo.FindOne(ctx, wsID, deviceID)
	if err != nil {
		return nil, err
	}

	// Known device
	if existing != nil {
		if deviceToken != "" {
			hashBytes := sha256.Sum256([]byte(deviceToken))
			tokenHash := hex.EncodeToString(hashBytes[:])

			if tokenHash == existing.DeviceTokenHash {
				_ = s.repo.UpdateLastSeen(ctx, existing.UUID, userAgent)
				return &clidevicedomain.DeviceValidationResult{}, nil
			}
		}

		// Re-issue token for registered device
		newToken, newTokenHash, err := generateDeviceToken()
		if err != nil {
			return nil, err
		}

		if err := s.repo.UpdateTokenHash(ctx, existing.UUID, newTokenHash, userAgent); err != nil {
			return nil, fmt.Errorf("failed to refresh device token: %w", err)
		}

		return &clidevicedomain.DeviceValidationResult{DeviceToken: newToken}, nil
	}

	// New device registration - enforce limit if configured
	if s.deviceLimit > 0 {
		count, err := s.repo.CountByWorkspaceID(ctx, wsID)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect device quota: %w", err)
		}
		if count >= s.deviceLimit {
			return nil, fmt.Errorf("CLI device limit reached for workspace (%d/%d)", count, s.deviceLimit)
		}
	}

	newToken, newTokenHash, err := generateDeviceToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entity := &clidevicedomain.CliDeviceEntity{
		UUID:            uuid.New(),
		WorkspaceID:     wsID,
		UserID:          params.UserID,
		DeviceID:        deviceID,
		DeviceTokenHash: newTokenHash,
		UserAgent:       userAgent,
		LastSeenAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.Create(ctx, entity); err != nil {
		// Race condition: another concurrent request registered this device
		raced, findErr := s.repo.FindOne(ctx, wsID, deviceID)
		if findErr == nil && raced != nil {
			reissueToken, reissueHash, tokErr := generateDeviceToken()
			if tokErr == nil {
				if updateErr := s.repo.UpdateTokenHash(ctx, raced.UUID, reissueHash, userAgent); updateErr == nil {
					return &clidevicedomain.DeviceValidationResult{DeviceToken: reissueToken}, nil
				}
			}
		}
		return nil, fmt.Errorf("failed to register device: %w", err)
	}

	return &clidevicedomain.DeviceValidationResult{DeviceToken: newToken}, nil
}

func generateDeviceToken() (rawToken, tokenHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(bytes)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}
