package clitokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

const (
	// TokenCacheTTL limits in-memory token retention before checking DB for cross-pod revocations.
	TokenCacheTTL = 60 * time.Second
)

var (
	ErrTokenNotFound = errors.New("cli token not found")
	ErrTokenRevoked  = errors.New("cli token has been revoked")
	ErrTokenExpired  = errors.New("cli token has expired")
	ErrScopeMissing  = errors.New("cli token lacks required scope")
)

// TokenRepository abstracts durable persistence for CLI API keys in PostgreSQL.
type TokenRepository interface {
	SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.TeamCLIKey, error)
	TouchAPIKeyUsage(ctx context.Context, keyID uuid.UUID) error
	RevokeAPIKey(ctx context.Context, workspaceID, keyID uuid.UUID) error
	ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*models.TeamCLIKey, error)
}

// TokenService manages the lifecycle of team and workspace CLI API keys.
type TokenService struct {
	mu     sync.RWMutex
	tokens map[string]*TeamCLIToken // in-memory fast-lookup cache: key: tokenHash
	repo   TokenRepository
}

// NewTokenService initializes the token manager, optionally attaching a PostgreSQL repository.
func NewTokenService(repo ...TokenRepository) *TokenService {
	s := &TokenService{
		tokens: make(map[string]*TeamCLIToken),
	}
	if len(repo) > 0 && repo[0] != nil {
		s.repo = repo[0]
	}
	return s
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
		CachedAt:    now,
	}

	s.mu.Lock()
	s.tokens[tokenHash] = record
	s.mu.Unlock()

	// Persist to PostgreSQL repository if attached
	if s.repo != nil {
		prefix := hexEntropy
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		if err := s.repo.SaveAPIKey(ctx, record.ID, record.WorkspaceID, record.Name, record.TokenHash, prefix, record.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed persisting CLI token to database: %w", err)
		}
	}

	return &MintTokenResponse{
		Token:  plaintextToken,
		Record: record,
	}, nil
}

// ValidateToken verifies a plaintext token against stored hashes and checks expiry/revocation.
// DB lookups run without holding the cache lock to eliminate concurrency bottlenecks under load.
func (s *TokenService) ValidateToken(ctx context.Context, plaintext string, requiredScope TokenScope) (*TeamCLIToken, error) {
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	// 1. Fast path: check in-memory cache with TTL validation
	s.mu.RLock()
	cachedRecord, exists := s.tokens[tokenHash]
	isCacheFresh := exists && time.Since(cachedRecord.CachedAt) < TokenCacheTTL
	s.mu.RUnlock()

	var record *TeamCLIToken

	if isCacheFresh {
		record = cachedRecord
	} else if s.repo != nil {
		// 2. Query repository outside of mutex lock to prevent blocking concurrent requests
		dbKey, err := s.repo.GetAPIKeyByHash(ctx, tokenHash)
		if err == nil && dbKey != nil {
			if !dbKey.Active {
				s.mu.Lock()
				delete(s.tokens, tokenHash)
				s.mu.Unlock()
				return nil, ErrTokenRevoked
			}
			if dbKey.ExpiresAt != nil && time.Now().UTC().After(*dbKey.ExpiresAt) {
				s.mu.Lock()
				delete(s.tokens, tokenHash)
				s.mu.Unlock()
				return nil, ErrTokenExpired
			}

			// Parse scopes from dbKey.Config (least privilege default: non-admin)
			var scopes []TokenScope
			if len(dbKey.Config) > 0 {
				var cfgObj struct {
					Scopes []TokenScope `json:"scopes"`
				}
				if err := json.Unmarshal(dbKey.Config, &cfgObj); err == nil && len(cfgObj.Scopes) > 0 {
					scopes = cfgObj.Scopes
				}
			}
			if len(scopes) == 0 {
				// Default to standard non-admin capabilities per Principle of Least Privilege
				scopes = []TokenScope{ScopeReviewRead, ScopeReviewWrite, ScopeRulesSync}
			}

			var wsID uuid.UUID
			if dbKey.WorkspaceID != nil {
				wsID = *dbKey.WorkspaceID
			}
			var teamID uuid.UUID
			if dbKey.TeamID != nil {
				teamID = *dbKey.TeamID
			}

			record = &TeamCLIToken{
				ID:          dbKey.ID,
				WorkspaceID: wsID,
				TeamID:      teamID,
				Name:        dbKey.Name,
				TokenHash:   dbKey.KeyHash,
				MaskedToken: dbKey.KeyPrefix + "...",
				Scopes:      scopes,
				ExpiresAt:   dbKey.ExpiresAt,
				LastUsedAt:  dbKey.LastUsedAt,
				CreatedAt:   dbKey.CreatedAt,
				CachedAt:    time.Now().UTC(),
			}

			s.mu.Lock()
			s.tokens[tokenHash] = record
			s.mu.Unlock()
		}
	} else if exists {
		// In-memory without database attached
		record = cachedRecord
	}

	if record == nil {
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
	s.mu.Lock()
	record.LastUsedAt = &now
	record.UsageCount++
	s.mu.Unlock()

	if s.repo != nil {
		_ = s.repo.TouchAPIKeyUsage(ctx, record.ID)
	}

	return record, nil
}

// RevokeToken marks an active token as revoked in memory and in the database repository.
func (s *TokenService) RevokeToken(ctx context.Context, tokenID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var wsID uuid.UUID
	found := false
	for _, t := range s.tokens {
		if t.ID == tokenID {
			t.IsRevoked = true
			wsID = t.WorkspaceID
			found = true
			break
		}
	}

	if s.repo != nil {
		if err := s.repo.RevokeAPIKey(ctx, wsID, tokenID); err != nil {
			return fmt.Errorf("failed revoking token in database: %w", err)
		}
		return nil
	}

	if !found {
		return ErrTokenNotFound
	}
	return nil
}

// ListTokens returns all active and revoked tokens for a team or workspace.
func (s *TokenService) ListTokens(ctx context.Context, wsID uuid.UUID) []*TeamCLIToken {
	if s.repo != nil {
		dbKeys, err := s.repo.ListAPIKeys(ctx, wsID)
		if err == nil && len(dbKeys) > 0 {
			var result []*TeamCLIToken
			for _, k := range dbKeys {
				var tID uuid.UUID
				if k.TeamID != nil {
					tID = *k.TeamID
				}
				result = append(result, &TeamCLIToken{
					ID:          k.ID,
					WorkspaceID: wsID,
					TeamID:      tID,
					Name:        k.Name,
					TokenHash:   k.KeyHash,
					MaskedToken: k.KeyPrefix + "...",
					Scopes:      []TokenScope{ScopeAdmin, ScopeReviewRead, ScopeReviewWrite, ScopeRulesSync},
					ExpiresAt:   k.ExpiresAt,
					LastUsedAt:  k.LastUsedAt,
					IsRevoked:   !k.Active,
					CreatedAt:   k.CreatedAt,
				})
			}
			return result
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TeamCLIToken
	for _, t := range s.tokens {
		if t.WorkspaceID == wsID {
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
