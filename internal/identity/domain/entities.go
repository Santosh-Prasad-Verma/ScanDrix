package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents an account within ScanDrix.
type User struct {
	UUID             uuid.UUID         `json:"uuid"`
	Email            string            `json:"email"`
	Name             string            `json:"name"`
	Password         string            `json:"-"` // Hashed password, omitted from serialization
	Role             Role              `json:"role"`
	OrganizationUUID *uuid.UUID        `json:"organization_uuid,omitempty"`
	OrganizationName string            `json:"organization_name,omitempty"`
	TeamUUID         *uuid.UUID        `json:"team_uuid,omitempty"`
	Status           UserStatus        `json:"status"`
	IsAdmin          bool              `json:"is_admin"`
	AvatarURL        *string           `json:"avatar_url,omitempty"`
	TeamMembers      []TeamMember      `json:"team_members,omitempty"`
	Permissions      *Permissions      `json:"permissions,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// ActiveRule represents an instantiated authorization grant with resolved scopes.
type ActiveRule struct {
	Action               Action       `json:"action"`
	Resource             ResourceType `json:"resource"`
	Scope                PolicyScope  `json:"scope"`
	Global               bool         `json:"global"`
	OrganizationUUID     uuid.UUID    `json:"organization_uuid"`
	AllowedRepositoryIDs []string     `json:"allowed_repository_ids,omitempty"`
}

// AppAbility encapsulates the complete set of active permission grants for a user.
type AppAbility struct {
	UserUUID         uuid.UUID
	OrganizationUUID uuid.UUID
	Role             Role
	IsAdmin          bool
	Rules            []ActiveRule
	AssignedRepos    map[string]struct{}
}

// NewAppAbility creates a new AppAbility.
func NewAppAbility(isAdmin bool) *AppAbility {
	return &AppAbility{
		IsAdmin:       isAdmin,
		AssignedRepos: make(map[string]struct{}),
	}
}

// Allow adds an organization-wide permission rule.
func (a *AppAbility) Allow(action, resource string) {
	a.Rules = append(a.Rules, ActiveRule{
		Action:   Action(action),
		Resource: ResourceType(resource),
		Scope:    ScopeOrg,
	})
}

// AllowWithRepo adds a repository-scoped permission rule.
func (a *AppAbility) AllowWithRepo(action, resource, repoID string) {
	a.Rules = append(a.Rules, ActiveRule{
		Action:               Action(action),
		Resource:             ResourceType(resource),
		Scope:                ScopeRepo,
		AllowedRepositoryIDs: []string{repoID},
	})
}

// AssignRepo links an authorized repository ID to the ability.
func (a *AppAbility) AssignRepo(repoID string) {
	if a.AssignedRepos == nil {
		a.AssignedRepos = make(map[string]struct{})
	}
	a.AssignedRepos[repoID] = struct{}{}
}

// Can evaluates whether the user is authorized for an action on a resource.
func (a *AppAbility) Can(action string, resource string) bool {
	if a.IsAdmin {
		return true
	}
	act := Action(action)
	res := ResourceType(resource)
	for _, rule := range a.Rules {
		actionMatches := rule.Action == ActionManage || rule.Action == act || rule.Action == "*"
		resourceMatches := rule.Resource == ResourceAll || rule.Resource == res || rule.Resource == "*"
		if actionMatches && resourceMatches {
			return true
		}
	}
	return false
}

// CanAccessRepo checks if the repository ID is authorized for this ability.
func (a *AppAbility) CanAccessRepo(repoID string) bool {
	if a.IsAdmin {
		return true
	}
	if a.AssignedRepos == nil {
		return false
	}
	_, ok := a.AssignedRepos[repoID]
	return ok
}

// UserSafeView returns a copy of User with credentials removed for serialization.
func (u *User) UserSafeView() User {
	cpy := *u
	cpy.Password = ""
	return cpy
}

// TeamMember represents membership of a user in a team.
type TeamMember struct {
	UUID             uuid.UUID      `json:"uuid"`
	UserUUID         uuid.UUID      `json:"user_uuid"`
	OrganizationUUID uuid.UUID      `json:"organization_uuid"`
	TeamUUID         uuid.UUID      `json:"team_uuid"`
	TeamName         string         `json:"team_name,omitempty"`
	TeamRole         TeamMemberRole `json:"team_role"`
	Status           bool           `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// UserProfile represents personalized account metadata.
type UserProfile struct {
	UUID           uuid.UUID `json:"uuid"`
	UserUUID       uuid.UUID `json:"user_uuid"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone,omitempty"`
	Img            string    `json:"img,omitempty"`
	Position       string    `json:"position,omitempty"`
	Status         bool      `json:"status"`
	ReferralSource string    `json:"referral_source,omitempty"`
	PrimaryGoal    string    `json:"primary_goal,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ProfileConfig represents a customized profile setting.
type ProfileConfig struct {
	UUID        uuid.UUID        `json:"uuid"`
	ProfileUUID uuid.UUID        `json:"profile_uuid"`
	ConfigKey   ProfileConfigKey `json:"config_key"`
	ConfigValue any              `json:"config_value"`
	Status      bool             `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// Permissions represents repository-specific authorization overrides.
type Permissions struct {
	UUID                  uuid.UUID `json:"uuid"`
	UserUUID              uuid.UUID `json:"user_uuid"`
	AssignedRepositoryIDs []string  `json:"assigned_repository_ids"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// CliAuthSession tracks loopback or device-code authorization for the ScanDrix CLI.
type CliAuthSession struct {
	UUID         uuid.UUID             `json:"uuid"`
	SessionID    string                `json:"session_id,omitempty"`
	State        string                `json:"state"`
	DeviceCode   *string               `json:"device_code,omitempty"`
	UserCode     *string               `json:"user_code,omitempty"`
	RedirectURI  *string               `json:"redirect_uri,omitempty"`
	Mode         CliAuthSessionMode    `json:"mode"`
	Status       CliAuthSessionStatus  `json:"status"`
	AccessToken  *string               `json:"access_token,omitempty"`
	RefreshToken *string               `json:"refresh_token,omitempty"`
	UserUUID     *uuid.UUID            `json:"user_uuid,omitempty"`
	UserEmail    *string               `json:"user_email,omitempty"`
	UserAgent    *string               `json:"user_agent,omitempty"`
	ExpiresAt    time.Time             `json:"expires_at"`
	ConsumedAt   *time.Time            `json:"consumed_at,omitempty"`
	CompletedAt  *time.Time            `json:"completed_at,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// AuthSession represents an active session with an issued refresh token.
type AuthSession struct {
	UUID         uuid.UUID    `json:"uuid"`
	UserUUID     uuid.UUID    `json:"user_uuid"`
	RefreshToken string       `json:"refresh_token"`
	ExpiryDate   time.Time    `json:"expiry_date"`
	Used         bool         `json:"used"`
	AuthDetails  any          `json:"auth_details,omitempty"`
	AuthProvider AuthProvider `json:"auth_provider"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// TokenResponse contains freshly minted access and refresh tokens.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// JWTConfig holds cryptographic configuration for token generation and validation.
type JWTConfig struct {
	Secret             string        `json:"secret"`
	RefreshSecret      string        `json:"refresh_secret"`
	ExpiresIn          time.Duration `json:"expires_in"`
	RefreshExpiresIn   time.Duration `json:"refresh_expires_in"`
	HelpdeskPrivateKey string        `json:"helpdesk_private_key,omitempty"`
}
