package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserRepository defines persistent storage operations for User accounts.
type UserRepository interface {
	Find(ctx context.Context, filter map[string]any) ([]User, error)
	FindOne(ctx context.Context, filter map[string]any) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*User, error)
	Create(ctx context.Context, user User) (*User, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context, filter map[string]any) (int, error)
}

// AuthRepository defines persistence operations for user auth refresh tokens.
type AuthRepository interface {
	SaveRefreshToken(ctx context.Context, session AuthSession) (*AuthSession, error)
	FindRefreshToken(ctx context.Context, token string) (*AuthSession, error)
	UpdateRefreshToken(ctx context.Context, session AuthSession) (*AuthSession, error)
	DeactivateRefreshToken(ctx context.Context, userUUID uuid.UUID) error
}

// CliAuthSessionRepository defines persistence operations for CLI authentication flows.
type CliAuthSessionRepository interface {
	Create(ctx context.Context, session CliAuthSession) (*CliAuthSession, error)
	FindByState(ctx context.Context, state string) (*CliAuthSession, error)
	FindByDeviceCode(ctx context.Context, code string) (*CliAuthSession, error)
	FindByUserCode(ctx context.Context, code string) (*CliAuthSession, error)
	Complete(ctx context.Context, id uuid.UUID, tokens TokenResponse, userUUID uuid.UUID, userEmail string) (*CliAuthSession, error)
	MarkConsumed(ctx context.Context, id uuid.UUID) error
	MarkDenied(ctx context.Context, id uuid.UUID) error
	ExpirePending(ctx context.Context, now time.Time) (int, error)
}

// PermissionsRepository manages repository-scoped overrides for users.
type PermissionsRepository interface {
	Create(ctx context.Context, perms Permissions) (*Permissions, error)
	FindByUserUUID(ctx context.Context, userUUID uuid.UUID) (*Permissions, error)
	Update(ctx context.Context, id uuid.UUID, repoIDs []string) (*Permissions, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// ProfileRepository manages personalized account details.
type ProfileRepository interface {
	Create(ctx context.Context, profile UserProfile) (*UserProfile, error)
	FindByUserUUID(ctx context.Context, userUUID uuid.UUID) (*UserProfile, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) (*UserProfile, error)
	UpdateByUserID(ctx context.Context, userUUID uuid.UUID, profile UserProfile) error
	DeleteOne(ctx context.Context, id uuid.UUID) error
}

// ProfileConfigRepository manages individual profile preference items.
type ProfileConfigRepository interface {
	Find(ctx context.Context, profileUUID uuid.UUID) ([]ProfileConfig, error)
	FindOne(ctx context.Context, profileUUID uuid.UUID, key ProfileConfigKey) (*ProfileConfig, error)
	Create(ctx context.Context, config ProfileConfig) (*ProfileConfig, error)
	Update(ctx context.Context, id uuid.UUID, value any, status bool) (*ProfileConfig, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// PasswordService hashes and verifies user passwords.
type PasswordService interface {
	HashPassword(password string) (string, error)
	MatchPassword(enteredPassword, hashedPassword string) bool
}

// TokenService creates, parses, and validates authentication and verification JWTs.
type TokenService interface {
	CreateTokens(user User, teamRole *TeamMemberRole) (*TokenResponse, error)
	VerifyRefreshToken(token string) (uuid.UUID, error)
	CreateEmailToken(userUUID uuid.UUID, email string) (string, error)
	VerifyEmailToken(token string) (uuid.UUID, string, error)
	CreateForgotPassToken(userUUID uuid.UUID, email string) (string, error)
	VerifyForgotPassToken(token string) (uuid.UUID, string, error)
	CreateHelpdeskToken(userUUID uuid.UUID) (string, error)
}

// NotificationEmitter delivers event notifications to users and integrations.
type NotificationEmitter interface {
	Emit(ctx context.Context, event string, payload map[string]any, orgUUID *uuid.UUID, recipientUUID *uuid.UUID) error
}

// AuditLogEmitter records system audit log events for compliance.
type AuditLogEmitter interface {
	EmitAuditLog(ctx context.Context, event string, params map[string]any) error
}
