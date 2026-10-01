package dtos

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// LoginRequest defines credentials for user authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest defines input for account creation.
type RegisterRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	DisplayName    string `json:"display_name"`
	WorkspaceName  string `json:"workspace_name"`
	WebsiteURL     string `json:"website_url,omitempty"`     // Anti-bot honeypot: must be empty
	TurnstileToken string `json:"turnstile_token,omitempty"` // Cloudflare Turnstile bot verification token
}

// AuthTokenResponse returns issued access tokens.
type AuthTokenResponse struct {
	AccessToken  string                `json:"access_token"`
	RefreshToken string                `json:"refresh_token,omitempty"`
	TokenType    string                `json:"token_type"`
	ExpiresIn    int64                 `json:"expires_in"`
	User         models.AccountProfile `json:"user"`
}

// CreateAPIKeyRequest defines input for generating team/CLI keys.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyResponse returns details of an issued API key.
type APIKeyResponse struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"key_prefix"`
	PlainKey  string     `json:"plain_key,omitempty"` // Only shown on creation
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Config carries the key's stored settings, including the capability flags
	// the dashboard toggles. It was previously dropped on read, so a key whose
	// permissions had been set always came back looking unconfigured.
	Config json.RawMessage `json:"config,omitempty"`
	// Active and LastUsedAt are real columns; without them the dashboard had to
	// guess, and showed every key as "never used".
	Active     bool       `json:"active"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// RefreshTokenRequest defines payload for rotating tokens.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest defines payload for invalidating refresh tokens.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// ForgotPasswordRequest requests a time-limited reset link/token.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest supplies the signed token and new credential.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// CompleteDeviceLoginRequest confirms device user code in the web dashboard.
type CompleteDeviceLoginRequest struct {
	UserCode string `json:"user_code"`
}

// OAuthCallbackRequest receives authorization code from SCM provider redirect.
type OAuthCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state,omitempty"`
}
