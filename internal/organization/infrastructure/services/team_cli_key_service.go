// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	clikey "github.com/scandrix/backend/internal/organization/domain/teamclikey"
	"golang.org/x/crypto/bcrypt"
)

// TeamCliKeyService implements clikey.ITeamCliKeyService.
type TeamCliKeyService struct {
	repo clikey.ITeamCliKeyRepository
}

// NewTeamCliKeyService creates a new TeamCliKeyService.
func NewTeamCliKeyService(repo clikey.ITeamCliKeyRepository) *TeamCliKeyService {
	return &TeamCliKeyService{repo: repo}
}

func (s *TeamCliKeyService) Create(ctx context.Context, entity *clikey.TeamCliKeyEntity) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Create(ctx, entity)
}

func (s *TeamCliKeyService) FindByKeyPrefix(ctx context.Context, prefix string) (*clikey.TeamCliKeyEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.FindByKeyPrefix(ctx, prefix)
}

func (s *TeamCliKeyService) ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*clikey.TeamCliKeyEntity, error) {
	if s.repo == nil {
		return nil, errors.New("repository uninitialized")
	}
	return s.repo.ListByWorkspace(ctx, wsID)
}

func (s *TeamCliKeyService) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.UpdateLastUsed(ctx, id)
}

func (s *TeamCliKeyService) UpdateConfig(ctx context.Context, wsID, keyID uuid.UUID, config []byte) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.UpdateConfig(ctx, wsID, keyID, config)
}

func (s *TeamCliKeyService) Revoke(ctx context.Context, wsID, keyID uuid.UUID) error {
	if s.repo == nil {
		return errors.New("repository uninitialized")
	}
	return s.repo.Revoke(ctx, wsID, keyID)
}

// GenerateKey generates a new scandrix_* prefixed API key, hashes it, and stores it.
func (s *TeamCliKeyService) GenerateKey(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, name string, config []byte, expiresAt *time.Time) (string, *clikey.TeamCliKeyEntity, error) {
	if wsID == uuid.Nil {
		return "", nil, errors.New("workspace ID is required")
	}
	if name == "" {
		return "", nil, errors.New("key name is required")
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random entropy: %w", err)
	}
	rawEntropy := base64.RawURLEncoding.EncodeToString(randomBytes)

	// Create prefix for fast index lookup (first 8 hex chars of sha256)
	hashSum := sha256.Sum256([]byte(rawEntropy))
	keyPrefix := hex.EncodeToString(hashSum[:])[:8]

	// Bcrypt hash for cryptographic storage
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(rawEntropy), 10)
	if err != nil {
		return "", nil, fmt.Errorf("failed to hash key: %w", err)
	}

	now := time.Now().UTC()
	entity := &clikey.TeamCliKeyEntity{
		UUID:        uuid.New(),
		WorkspaceID: wsID,
		TeamID:      teamID,
		Name:        name,
		KeyHash:     string(hashBytes),
		KeyPrefix:   keyPrefix,
		Active:      true,
		Config:      config,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.Create(ctx, entity); err != nil {
		return "", nil, err
	}

	rawKey := fmt.Sprintf("scandrix_%s", rawEntropy)
	return rawKey, entity, nil
}

// ValidateKey validates an incoming API key token.
func (s *TeamCliKeyService) ValidateKey(ctx context.Context, rawKey string) (*clikey.ValidateKeyResult, error) {
	token := strings.TrimSpace(rawKey)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "scandrix_")

	if token == "" {
		return nil, errors.New("invalid API key format")
	}

	hashSum := sha256.Sum256([]byte(token))
	prefix := hex.EncodeToString(hashSum[:])[:8]

	entity, err := s.FindByKeyPrefix(ctx, prefix)
	if err != nil || entity == nil {
		return nil, errors.New("invalid or unauthenticated API key")
	}

	if !entity.Active {
		return nil, errors.New("API key has been revoked")
	}

	if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
		return nil, errors.New("API key has expired")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(entity.KeyHash), []byte(token)); err != nil {
		return nil, errors.New("invalid key verification signature")
	}

	// Update last used timestamp
	_ = s.UpdateLastUsed(ctx, entity.UUID)

	return &clikey.ValidateKeyResult{
		KeyID:       entity.UUID,
		WorkspaceID: entity.WorkspaceID,
		TeamID:      entity.TeamID,
		Name:        entity.Name,
		Config:      entity.Config,
	}, nil
}
