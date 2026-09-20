// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTokenFormat = errors.New("invalid CI token format")
	ErrTokenExpired       = errors.New("CI token has expired")
	ErrTokenSignature     = errors.New("invalid CI token HMAC signature")
	ErrTokenRevoked       = errors.New("CI token has been revoked")
	ErrScopeMismatch      = errors.New("CI token repository scope does not match target")
)

// CITokenClaims defines authorization data embedded within a CI token.
type CITokenClaims struct {
	TokenID        uuid.UUID `json:"token_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	RepositoryID   string    `json:"repository_id"`
	RepoFullName   string    `json:"repo_full_name"`
	Permissions    []string  `json:"permissions"`
	IssuedAt       int64     `json:"issued_at"`
	ExpiresAt      int64     `json:"expires_at"`
}

// CITokenService issues and validates cryptographically signed CI tokens.
type CITokenService struct {
	mu           sync.RWMutex
	secretKey    []byte
	revokedToken map[uuid.UUID]bool
}

// NewCITokenService constructs a token validation service.
func NewCITokenService(secretKey string) *CITokenService {
	if secretKey == "" {
		secretKey = "scandrix_ci_default_signing_key_secret_2026"
	}
	return &CITokenService{
		secretKey:    []byte(secretKey),
		revokedToken: make(map[uuid.UUID]bool),
	}
}

// IssueToken mints a new scoped CI token.
func (s *CITokenService) IssueToken(
	orgID uuid.UUID,
	repoID, repoFullName string,
	permissions []string,
	ttl time.Duration,
) (string, *CITokenClaims, error) {
	if ttl == 0 {
		ttl = 2 * time.Hour
	}

	now := time.Now().UTC()
	claims := &CITokenClaims{
		TokenID:        uuid.New(),
		OrganizationID: orgID,
		RepositoryID:   repoID,
		RepoFullName:   repoFullName,
		Permissions:    permissions,
		IssuedAt:       now.Unix(),
		ExpiresAt:      now.Add(ttl).Unix(),
	}

	rawClaims, err := json.Marshal(claims)
	if err != nil {
		return "", nil, err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(rawClaims)

	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(payloadB64))
	sigHex := hex.EncodeToString(mac.Sum(nil))

	tokenStr := fmt.Sprintf("scandrix_ci_%s.%s", payloadB64, sigHex)
	return tokenStr, claims, nil
}

// ValidateToken parses, authenticates, and checks expiration of a CI token.
func (s *CITokenService) ValidateToken(tokenStr string, expectedRepo string) (*CITokenClaims, error) {
	if !strings.HasPrefix(tokenStr, "scandrix_ci_") {
		return nil, ErrInvalidTokenFormat
	}

	trimmed := strings.TrimPrefix(tokenStr, "scandrix_ci_")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidTokenFormat
	}

	payloadB64, sigHex := parts[0], parts[1]

	// Verify HMAC-SHA256 signature
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(payloadB64))
	expectedSigHex := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sigHex), []byte(expectedSigHex)) {
		return nil, ErrTokenSignature
	}

	rawClaims, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("malformed base64 claims: %w", err)
	}

	var claims CITokenClaims
	if err := json.Unmarshal(rawClaims, &claims); err != nil {
		return nil, fmt.Errorf("malformed claims JSON: %w", err)
	}

	// Check revocation
	s.mu.RLock()
	revoked := s.revokedToken[claims.TokenID]
	s.mu.RUnlock()
	if revoked {
		return nil, ErrTokenRevoked
	}

	// Check expiration
	now := time.Now().Unix()
	if now > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	// Scope verification
	if expectedRepo != "" && claims.RepoFullName != "" && !strings.EqualFold(claims.RepoFullName, expectedRepo) {
		return nil, ErrScopeMismatch
	}

	return &claims, nil
}

// RevokeToken marks a token as revoked immediately.
func (s *CITokenService) RevokeToken(tokenID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedToken[tokenID] = true
}
