package types

import (
	"time"
)

// AuthMethod represents the method used to authenticate.
type AuthMethod string

const (
	AuthMethodBrowser  AuthMethod = "browser"
	AuthMethodDevice   AuthMethod = "device"
	AuthMethodTeamKey  AuthMethod = "team_key"
	AuthMethodToken    AuthMethod = "token"
	AuthMethodEnv      AuthMethod = "env"
)

// UserProfile holds the authenticated developer identity.
type UserProfile struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	AvatarURL      string    `json:"avatar_url,omitempty"`
	DefaultOrgID   string    `json:"default_org_id,omitempty"`
	DefaultOrgName string    `json:"default_org_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// WorkspaceContext holds tenant and permissions information.
type WorkspaceContext struct {
	WorkspaceID   string   `json:"workspace_id"`
	WorkspaceName string   `json:"workspace_name"`
	Role          string   `json:"role"` // owner, admin, member
	Permissions   []string `json:"permissions"`
	PlanTier      string   `json:"plan_tier"`
	SeatsAllocated int     `json:"seats_allocated"`
	BYOKEnabled   bool     `json:"byok_enabled"`
}

// SessionCredentials holds active access and refresh tokens.
type SessionCredentials struct {
	AccessToken   string            `json:"access_token"`
	RefreshToken  string            `json:"refresh_token,omitempty"`
	IDToken       string            `json:"id_token,omitempty"`
	ExpiresAt     time.Time         `json:"expires_at"`
	TokenType     string            `json:"token_type"` // Bearer
	AuthMethod    AuthMethod        `json:"auth_method"`
	User          *UserProfile      `json:"user,omitempty"`
	Workspaces    []WorkspaceContext `json:"workspaces,omitempty"`
	ActiveWorkspaceID string        `json:"active_workspace_id,omitempty"`
	TeamKey       string            `json:"team_key,omitempty"`
	APIBaseURL    string            `json:"api_base_url,omitempty"`
}

// IsExpired reports whether the session credentials have passed expiration.
func (c *SessionCredentials) IsExpired() bool {
	if c.AccessToken == "" {
		return true
	}
	if c.ExpiresAt.IsZero() {
		return false
	}
	// Buffer 30 seconds
	return time.Now().Add(30 * time.Second).After(c.ExpiresAt)
}

// DeviceCodeResponse holds the RFC 8628 device authorization payload.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// OAuthCallbackPayload holds redirect query parameters received on localhost.
type OAuthCallbackPayload struct {
	Code        string `json:"code"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	Description string `json:"error_description,omitempty"`
}

// TeamKeyValidationResult holds the verified team key context.
type TeamKeyValidationResult struct {
	Valid       bool     `json:"valid"`
	TeamID      string   `json:"team_id"`
	TeamName    string   `json:"team_name"`
	OrgID       string   `json:"org_id"`
	OrgName     string   `json:"org_name"`
	WorkspaceID string   `json:"workspace_id"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
