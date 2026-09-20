package clitokens

import (
	"time"

	"github.com/google/uuid"
)

// TokenScope limits API capabilities for a minted CLI key.
type TokenScope string

const (
	ScopeReviewRead  TokenScope = "review:read"
	ScopeReviewWrite TokenScope = "review:write"
	ScopeRulesSync   TokenScope = "rules:sync"
	ScopeAdmin       TokenScope = "admin:all"
)

// TeamCLIToken stores metadata and one-way cryptographic hash of a team CLI key.
type TeamCLIToken struct {
	ID          uuid.UUID    `json:"id"`
	WorkspaceID uuid.UUID    `json:"workspace_id"`
	TeamID      uuid.UUID    `json:"team_id"`
	Name        string       `json:"name"`
	TokenHash   string       `json:"-"` // SHA-256 hex digest, never exposed in JSON
	MaskedToken string       `json:"masked_token"`
	Scopes      []TokenScope `json:"scopes"`
	ExpiresAt   *time.Time   `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time   `json:"last_used_at,omitempty"`
	UsageCount  int64        `json:"usage_count"`
	IsRevoked   bool         `json:"is_revoked"`
	CreatedBy   uuid.UUID    `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
	CachedAt    time.Time    `json:"-"`
}

// MintTokenRequest specifies the parameters for creating a new team or personal CLI key.
type MintTokenRequest struct {
	WorkspaceID uuid.UUID     `json:"workspace_id"`
	TeamID      uuid.UUID     `json:"team_id"`
	Name        string        `json:"name"`
	Scopes      []TokenScope  `json:"scopes"`
	TTL         time.Duration `json:"ttl,omitempty"` // 0 = never expires
	CreatedBy   uuid.UUID     `json:"created_by"`
}

// MintTokenResponse delivers the plaintext key once upon generation.
type MintTokenResponse struct {
	Token  string        `json:"token"` // Shown ONLY once!
	Record *TeamCLIToken `json:"record"`
}
