package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents an authenticated member in the platform.
type User struct {
	BaseEntity
	WorkspaceID   uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	Email         string     `json:"email" db:"email"`
	FullName      string     `json:"full_name" db:"full_name"`
	AvatarURL     *string    `json:"avatar_url,omitempty" db:"avatar_url"`
	IsActive      bool       `json:"is_active" db:"is_active"`
	IsSuperAdmin  bool       `json:"is_super_admin" db:"is_super_admin"`
	EmailVerified bool       `json:"email_verified" db:"email_verified"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}

// AccountProfile represents user profile preferences and personal dashboard settings.
type AccountProfile struct {
	TenantScopedEntity
	UserID                  uuid.UUID `json:"user_id" db:"user_id"`
	Role                    string    `json:"role" db:"role"`
	JobTitle                *string   `json:"job_title,omitempty" db:"job_title"`
	Bio                     *string   `json:"bio,omitempty" db:"bio"`
	ThemePreference         string    `json:"theme_preference" db:"theme_preference"`
	DigestFrequency         string    `json:"digest_frequency" db:"digest_frequency"`
	NotificationPreferences JSONBMap  `json:"notification_preferences" db:"notification_preferences"`
}

// ProfileModel alias for AccountProfile matching TypeORM entity naming.
type ProfileModel = AccountProfile

// ProfileConfig stores user-specific developer configurations and editor integrations.
type ProfileConfig struct {
	TenantScopedEntity
	UserID                 uuid.UUID `json:"user_id" db:"user_id"`
	PreferredModel         string    `json:"preferred_model" db:"preferred_model"`
	SyntaxHighlighterTheme string    `json:"syntax_highlighter_theme" db:"syntax_highlighter_theme"`
	Keybindings            string    `json:"keybindings" db:"keybindings"`
	ShowInlineDiffs        bool      `json:"show_inline_diffs" db:"show_inline_diffs"`
	AutoFoldPassingFiles   bool      `json:"auto_fold_passing_files" db:"auto_fold_passing_files"`
	Settings               JSONBMap  `json:"settings" db:"settings"`
}

// ProfileConfigModel alias.
type ProfileConfigModel = ProfileConfig

// Auth records password hashes, salts, and MFA secrets for local accounts.
type Auth struct {
	TenantScopedEntity
	UserID              uuid.UUID  `json:"user_id" db:"user_id"`
	PasswordHash        string     `json:"-" db:"password_hash"`
	Salt                string     `json:"-" db:"salt"`
	MFASecret           *string    `json:"-" db:"mfa_secret"`
	MFAEnabled          bool       `json:"mfa_enabled" db:"mfa_enabled"`
	FailedLoginAttempts int        `json:"failed_login_attempts" db:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty" db:"locked_until"`
	PasswordChangedAt   time.Time  `json:"password_changed_at" db:"password_changed_at"`
}

// AuthIntegration stores third-party OAuth provider identities (GitHub, GitLab, Bitbucket, Google).
type AuthIntegration struct {
	TenantScopedEntity
	UserID                uuid.UUID   `json:"user_id" db:"user_id"`
	Provider              string      `json:"provider" db:"provider"`
	ProviderAccountID     string      `json:"provider_account_id" db:"provider_account_id"`
	ProviderUsername      string      `json:"provider_username" db:"provider_username"`
	EncryptedAccessToken  string      `json:"-" db:"encrypted_access_token"`
	EncryptedRefreshToken *string     `json:"-" db:"encrypted_refresh_token"`
	TokenExpiresAt        *time.Time  `json:"token_expires_at,omitempty" db:"token_expires_at"`
	Scopes                StringSlice `json:"scopes" db:"scopes"`
	ProfileData           JSONBMap    `json:"profile_data" db:"profile_data"`
}

// Permissions stores RBAC role definitions and action matrix for team members.
type Permissions struct {
	TenantScopedEntity
	RoleName   string   `json:"role_name" db:"role_name"`
	Resource   string   `json:"resource" db:"resource"`
	Action     string   `json:"action" db:"action"`
	IsAllowed  bool     `json:"is_allowed" db:"is_allowed"`
	Conditions JSONBMap `json:"conditions" db:"conditions"`
}

// PermissionsModel alias.
type PermissionsModel = Permissions

// SSOConfig holds enterprise SAML 2.0 and OIDC configuration.
type SSOConfig struct {
	TenantScopedEntity
	OrganizationID       uuid.UUID `json:"organization_id" db:"organization_id"`
	ProviderType         string    `json:"provider_type" db:"provider_type"`
	IdpEntityID          string    `json:"idp_entity_id" db:"idp_entity_id"`
	IdpSSOURL            string    `json:"idp_sso_url" db:"idp_sso_url"`
	EncryptedCertificate string    `json:"-" db:"encrypted_certificate"`
	AttributeMapping     JSONBMap  `json:"attribute_mapping" db:"attribute_mapping"`
	AllowIdpInitiated    bool      `json:"allow_idp_initiated" db:"allow_idp_initiated"`
	EnforceSSO           bool      `json:"enforce_sso" db:"enforce_sso"`
	IsActive             bool      `json:"is_active" db:"is_active"`
}

// SSOConfigModel alias.
type SSOConfigModel = SSOConfig

// SSOTestSession records transient handshake payloads for testing SAML assertions before production lock.
type SSOTestSession struct {
	BaseEntity
	WorkspaceID     uuid.UUID `json:"workspace_id" db:"workspace_id"`
	SessionToken    string    `json:"session_token" db:"session_token"`
	RequestPayload  string    `json:"request_payload" db:"request_payload"`
	ResponsePayload *string   `json:"response_payload,omitempty" db:"response_payload"`
	Success         bool      `json:"success" db:"success"`
	ErrorMessage    *string   `json:"error_message,omitempty" db:"error_message"`
	ExpiresAt       time.Time `json:"expires_at" db:"expires_at"`
}

// SSOTestSessionModel alias.
type SSOTestSessionModel = SSOTestSession
