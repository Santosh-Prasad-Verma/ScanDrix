package clitokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTokenNotFound = errors.New("cli token not found")
	ErrTokenRevoked  = errors.New("cli token has been revoked")
	ErrTokenExpired  = errors.New("cli token has expired")
	ErrScopeMissing  = errors.New("cli token lacks required scope")
)

// TokenService manages the lifecycle of team and workspace CLI API keys.
type TokenService struct {
	mu     sync.RWMutex
	tokens map[string]*TeamCLIToken // key: tokenHash
}

// NewTokenService initializes the token manager.
func NewTokenService() *TokenService {
	return &TokenService{
		tokens: make(map[string]*TeamCLIToken),
	}
}

// MintToken generates a cryptographically secure API key and persists its SHA-256 digest.
func (s *TokenService) MintToken(ctx context.Context, req MintTokenRequest) (*MintTokenResponse, error) {
	// 1. Generate 32 bytes of secure entropy
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("failed generating token entropy: %w", err)
	}

	hexEntropy := hex.EncodeToString(entropy)
	plaintextToken := fmt.Sprintf("scandrix_team_%s", hexEntropy)

	// 2. Compute SHA-256 digest for zero-knowledge storage
	hash := sha256.Sum256([]byte(plaintextToken))
	tokenHash := hex.EncodeToString(hash[:])

	// 3. Create masked token for audit display
	masked := fmt.Sprintf("scandrix_team_%s...%s", hexEntropy[:6], hexEntropy[len(hexEntropy)-4:])

	now := time.Now().UTC()
	var expiresAt *time.Time
	if req.TTL > 0 {
		exp := now.Add(req.TTL)
		expiresAt = &exp
	}

	record := &TeamCLIToken{
		ID:          uuid.New(),
		WorkspaceID: req.WorkspaceID,
		TeamID:      req.TeamID,
		Name:        req.Name,
		TokenHash:   tokenHash,
		MaskedToken: masked,
		Scopes:      req.Scopes,
		ExpiresAt:   expiresAt,
		UsageCount:  0,
		IsRevoked:   false,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   now,
	}

	s.mu.Lock()
	s.tokens[tokenHash] = record
	s.mu.Unlock()

	return &MintTokenResponse{
		Token:  plaintextToken,
		Record: record,
	}, nil
}

// ValidateToken verifies a plaintext token against stored hashes and checks expiry/revocation.
func (s *TokenService) ValidateToken(ctx context.Context, plaintext string, requiredScope TokenScope) (*TeamCLIToken, error) {
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.tokens[tokenHash]
	if !exists {
		return nil, ErrTokenNotFound
	}

	if record.IsRevoked {
		return nil, ErrTokenRevoked
	}

	if record.ExpiresAt != nil && time.Now().UTC().After(*record.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Verify required scope
	if requiredScope != "" && !hasScope(record.Scopes, requiredScope) {
		return nil, ErrScopeMissing
	}

	// Update telemetry
	now := time.Now().UTC()
	record.LastUsedAt = &now
	record.UsageCount++

	return record, nil
}

// RevokeToken marks an active token as revoked.
func (s *TokenService) RevokeToken(ctx context.Context, tokenID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range s.tokens {
		if t.ID == tokenID {
			t.IsRevoked = true
			return nil
		}
	}
	return ErrTokenNotFound
}

// ListTokens returns all active and revoked tokens for a team or workspace.
func (s *TokenService) ListTokens(ctx context.Context, wsID uuid.UUID) []*TeamCLIToken {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TeamCLIToken
	for _, t := range s.tokens {
		if t.WorkspaceID == wsID {
			// Return shallow copy
			copyItem := *t
			result = append(result, &copyItem)
		}
	}
	return result
}

func hasScope(scopes []TokenScope, required TokenScope) bool {
	for _, s := range scopes {
		if s == ScopeAdmin || s == required {
			return true
		}
	}
	return false
}
