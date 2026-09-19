package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// Common error definitions for authentication operations.
var (
	ErrDuplicateEmail    = errors.New("a user with this email already exists")
	ErrDuplicateOrg      = errors.New("an organization with this name already exists")
	ErrUnauthorized      = errors.New("unauthorized credentials")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidToken      = errors.New("token is invalid or has expired")
	ErrTokenAlreadyUsed  = errors.New("refresh token has already been consumed")
)

// SignUpInput contains arguments for user registration.
type SignUpInput struct {
	Email          string     `json:"email"`
	Password       string     `json:"password"`
	Name           string     `json:"name"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	PreVerified    bool       `json:"pre_verified,omitempty"`
	CloudMode      *bool      `json:"cloud_mode,omitempty"`
}

// SignUpUseCase registers a new account, provisions organization context, and initializes profile.
type SignUpUseCase struct {
	userRepo        domain.UserRepository
	profileRepo     domain.ProfileRepository
	passwordService domain.PasswordService
	tokenService    domain.TokenService
	notifEmitter    domain.NotificationEmitter
	cloudMode       bool
}

func NewSignUpUseCase(
	userRepo domain.UserRepository,
	profileRepo domain.ProfileRepository,
	pwService domain.PasswordService,
	tokenService domain.TokenService,
	notifEmitter domain.NotificationEmitter,
) *SignUpUseCase {
	return &SignUpUseCase{
		userRepo:        userRepo,
		profileRepo:     profileRepo,
		passwordService: pwService,
		tokenService:    tokenService,
		notifEmitter:    notifEmitter,
		cloudMode:       true,
	}
}

// SetCloudMode configures whether the use case operates in cloud mode or self-hosted mode.
func (uc *SignUpUseCase) SetCloudMode(cloud bool) {
	uc.cloudMode = cloud
}

func (uc *SignUpUseCase) Execute(ctx context.Context, input SignUpInput) (*domain.User, error) {
	existing, _ := uc.userRepo.FindByEmail(ctx, input.Email)
	if existing != nil {
		return nil, ErrDuplicateEmail
	}

	hashedPassword, err := uc.passwordService.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("failed hashing password: %w", err)
	}

	now := time.Now().UTC()
	userUUID := uuid.New()

	var role domain.Role
	var status domain.UserStatus
	var orgUUID uuid.UUID
	var orgName string

	isCloud := uc.cloudMode
	if input.CloudMode != nil {
		isCloud = *input.CloudMode
	}

	isOwner := input.OrganizationID == nil || *input.OrganizationID == uuid.Nil
	if !isOwner {
		role = domain.RoleContributor
		orgUUID = *input.OrganizationID
		orgName = "Organization"
		if !isCloud || input.PreVerified {
			status = domain.UserStatusActive
		} else {
			status = domain.UserStatusPending
		}
	} else {
		// New organization creator becomes owner and active immediately regardless of cloud mode
		role = domain.RoleOwner
		status = domain.UserStatusActive
		orgUUID = uuid.New()
		orgName = fmt.Sprintf("%s Organization", input.Name)
	}

	teamUUID := uuid.New()
	membershipStatus := isOwner || input.PreVerified
	teamMember := domain.TeamMember{
		UUID:             uuid.New(),
		UserUUID:         userUUID,
		OrganizationUUID: orgUUID,
		TeamUUID:         teamUUID,
		TeamName:         fmt.Sprintf("%s Team", input.Name),
		TeamRole:         domain.TeamMemberRoleLeader,
		Status:           membershipStatus,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if !isOwner {
		teamMember.TeamRole = domain.TeamMemberRoleMember
	}

	user := domain.User{
		UUID:             userUUID,
		Email:            input.Email,
		Password:         hashedPassword,
		Role:             role,
		OrganizationUUID: &orgUUID,
		OrganizationName: orgName,
		Status:           status,
		TeamMembers:      []domain.TeamMember{teamMember},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	createdUser, err := uc.userRepo.Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed creating user record: %w", err)
	}

	// Initialize user profile
	profile := domain.UserProfile{
		UUID:      uuid.New(),
		UserUUID:  userUUID,
		Name:      input.Name,
		Status:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := uc.profileRepo.Create(ctx, profile); err != nil {
		// Log and continue, profile can be recovered
	}

	if uc.notifEmitter != nil && status == domain.UserStatusPending {
		emailToken, _ := uc.tokenService.CreateEmailToken(userUUID, input.Email)
		_ = uc.notifEmitter.Emit(ctx, "AUTH_CONFIRM_EMAIL", map[string]any{
			"email": input.Email,
			"token": emailToken,
			"name":  input.Name,
		}, &orgUUID, &userUUID)
	}

	safe := createdUser.UserSafeView()
	return &safe, nil
}

// LoginUseCase authenticates email and password credentials, returning bearer tokens.
type LoginUseCase struct {
	userRepo        domain.UserRepository
	authRepo        domain.AuthRepository
	passwordService domain.PasswordService
	tokenService    domain.TokenService
}

func NewLoginUseCase(
	userRepo domain.UserRepository,
	authRepo domain.AuthRepository,
	pwService domain.PasswordService,
	tokenService domain.TokenService,
) *LoginUseCase {
	return &LoginUseCase{
		userRepo:        userRepo,
		authRepo:        authRepo,
		passwordService: pwService,
		tokenService:    tokenService,
	}
}

func (uc *LoginUseCase) Execute(ctx context.Context, email, password string) (*domain.TokenResponse, error) {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		return nil, ErrUnauthorized
	}

	if !uc.passwordService.MatchPassword(password, user.Password) {
		return nil, ErrUnauthorized
	}

	var teamRole *domain.TeamMemberRole
	if len(user.TeamMembers) > 0 {
		teamRole = &user.TeamMembers[0].TeamRole
	}

	tokens, err := uc.tokenService.CreateTokens(*user, teamRole)
	if err != nil {
		return nil, fmt.Errorf("failed generating tokens: %w", err)
	}

	now := time.Now().UTC()
	session := domain.AuthSession{
		UUID:         uuid.New(),
		UserUUID:     user.UUID,
		RefreshToken: tokens.RefreshToken,
		ExpiryDate:   now.Add(30 * 24 * time.Hour),
		Used:         false,
		AuthProvider: domain.AuthProviderCredentials,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, _ = uc.authRepo.SaveRefreshToken(ctx, session)

	return tokens, nil
}

// OAuthLoginUseCase processes third-party single-sign-on authorizations.
type OAuthLoginUseCase struct {
	userRepo     domain.UserRepository
	authRepo     domain.AuthRepository
	tokenService domain.TokenService
	signUpUC     *SignUpUseCase
}

func NewOAuthLoginUseCase(
	userRepo domain.UserRepository,
	authRepo domain.AuthRepository,
	tokenService domain.TokenService,
	signUpUC *SignUpUseCase,
) *OAuthLoginUseCase {
	return &OAuthLoginUseCase{
		userRepo:     userRepo,
		authRepo:     authRepo,
		tokenService: tokenService,
		signUpUC:     signUpUC,
	}
}

func (uc *OAuthLoginUseCase) Execute(
	ctx context.Context,
	name string,
	email string,
	providerRefreshToken string,
	provider domain.AuthProvider,
) (*domain.TokenResponse, error) {
	user, _ := uc.userRepo.FindByEmail(ctx, email)
	if user == nil {
		// Auto-provision new SSO user with secure random password
		buf := make([]byte, 24)
		_, _ = rand.Read(buf)
		randomPW := base64.RawURLEncoding.EncodeToString(buf)

		created, err := uc.signUpUC.Execute(ctx, SignUpInput{
			Email:       email,
			Password:    randomPW,
			Name:        name,
			PreVerified: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed auto-provisioning SSO account: %w", err)
		}
		user = created
	}

	var teamRole *domain.TeamMemberRole
	if len(user.TeamMembers) > 0 {
		teamRole = &user.TeamMembers[0].TeamRole
	}

	tokens, err := uc.tokenService.CreateTokens(*user, teamRole)
	if err != nil {
		return nil, fmt.Errorf("failed generating tokens: %w", err)
	}

	now := time.Now().UTC()
	session := domain.AuthSession{
		UUID:         uuid.New(),
		UserUUID:     user.UUID,
		RefreshToken: tokens.RefreshToken,
		ExpiryDate:   now.Add(30 * 24 * time.Hour),
		Used:         false,
		AuthDetails: map[string]any{
			"provider_refresh_token": providerRefreshToken,
		},
		AuthProvider: provider,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, _ = uc.authRepo.SaveRefreshToken(ctx, session)

	return tokens, nil
}

// RefreshTokenUseCase rotates refresh tokens with replay detection.
type RefreshTokenUseCase struct {
	authRepo     domain.AuthRepository
	userRepo     domain.UserRepository
	tokenService domain.TokenService
}

func NewRefreshTokenUseCase(
	authRepo domain.AuthRepository,
	userRepo domain.UserRepository,
	tokenService domain.TokenService,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		authRepo:     authRepo,
		userRepo:     userRepo,
		tokenService: tokenService,
	}
}

func (uc *RefreshTokenUseCase) Execute(ctx context.Context, oldRefreshToken string) (*domain.TokenResponse, error) {
	userUUID, err := uc.tokenService.VerifyRefreshToken(oldRefreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	storedSession, err := uc.authRepo.FindRefreshToken(ctx, oldRefreshToken)
	if err != nil || storedSession == nil {
		return nil, ErrInvalidToken
	}

	if storedSession.Used {
		// Potential token compromise / replay attack: invalidate all user sessions
		_ = uc.authRepo.DeactivateRefreshToken(ctx, userUUID)
		return nil, ErrTokenAlreadyUsed
	}

	if time.Now().UTC().After(storedSession.ExpiryDate) {
		return nil, ErrInvalidToken
	}

	user, err := uc.userRepo.FindByUUID(ctx, userUUID)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}

	var teamRole *domain.TeamMemberRole
	if len(user.TeamMembers) > 0 {
		teamRole = &user.TeamMembers[0].TeamRole
	}

	newTokens, err := uc.tokenService.CreateTokens(*user, teamRole)
	if err != nil {
		return nil, fmt.Errorf("failed generating new tokens: %w", err)
	}

	// Mark old session consumed
	storedSession.Used = true
	storedSession.UpdatedAt = time.Now().UTC()
	_, _ = uc.authRepo.UpdateRefreshToken(ctx, *storedSession)

	// Persist new session
	now := time.Now().UTC()
	newSession := domain.AuthSession{
		UUID:         uuid.New(),
		UserUUID:     user.UUID,
		RefreshToken: newTokens.RefreshToken,
		ExpiryDate:   now.Add(30 * 24 * time.Hour),
		Used:         false,
		AuthProvider: storedSession.AuthProvider,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, _ = uc.authRepo.SaveRefreshToken(ctx, newSession)

	return newTokens, nil
}

// LogoutUseCase invalidates an active refresh token.
type LogoutUseCase struct {
	authRepo domain.AuthRepository
}

func NewLogoutUseCase(authRepo domain.AuthRepository) *LogoutUseCase {
	return &LogoutUseCase{authRepo: authRepo}
}

func (uc *LogoutUseCase) Execute(ctx context.Context, refreshToken string) error {
	session, err := uc.authRepo.FindRefreshToken(ctx, refreshToken)
	if err != nil || session == nil {
		return nil // Graceful no-op if session already gone
	}

	session.Used = true
	session.UpdatedAt = time.Now().UTC()
	_, err = uc.authRepo.UpdateRefreshToken(ctx, *session)
	return err
}

// ConfirmEmailUseCase validates email verification tokens and marks users active.
type ConfirmEmailUseCase struct {
	userRepo     domain.UserRepository
	tokenService domain.TokenService
}

func NewConfirmEmailUseCase(userRepo domain.UserRepository, tokenService domain.TokenService) *ConfirmEmailUseCase {
	return &ConfirmEmailUseCase{userRepo: userRepo, tokenService: tokenService}
}

func (uc *ConfirmEmailUseCase) Execute(ctx context.Context, token string) error {
	userUUID, email, err := uc.tokenService.VerifyEmailToken(token)
	if err != nil {
		return ErrInvalidToken
	}

	user, err := uc.userRepo.FindByUUID(ctx, userUUID)
	if err != nil || user == nil || user.Email != email {
		return ErrUserNotFound
	}

	if user.Status == domain.UserStatusActive {
		return nil
	}

	_, err = uc.userRepo.Update(ctx, user.UUID, map[string]any{
		"status": string(domain.UserStatusActive),
	})
	return err
}

// ResendEmailUseCase issues a fresh confirmation token.
type ResendEmailUseCase struct {
	userRepo     domain.UserRepository
	tokenService domain.TokenService
	notifEmitter domain.NotificationEmitter
}

func NewResendEmailUseCase(
	userRepo domain.UserRepository,
	tokenService domain.TokenService,
	emitter domain.NotificationEmitter,
) *ResendEmailUseCase {
	return &ResendEmailUseCase{
		userRepo:     userRepo,
		tokenService: tokenService,
		notifEmitter: emitter,
	}
}

func (uc *ResendEmailUseCase) Execute(ctx context.Context, email string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		return ErrUserNotFound
	}

	if user.Status == domain.UserStatusActive {
		return nil
	}

	token, err := uc.tokenService.CreateEmailToken(user.UUID, user.Email)
	if err != nil {
		return err
	}

	if uc.notifEmitter != nil {
		_ = uc.notifEmitter.Emit(ctx, "AUTH_CONFIRM_EMAIL", map[string]any{
			"email": user.Email,
			"token": token,
		}, user.OrganizationUUID, &user.UUID)
	}
	return nil
}

// ForgotPasswordUseCase issues a password reset link token and notifies the user.
type ForgotPasswordUseCase struct {
	userRepo     domain.UserRepository
	tokenService domain.TokenService
	notifEmitter domain.NotificationEmitter
}

func NewForgotPasswordUseCase(
	userRepo domain.UserRepository,
	tokenService domain.TokenService,
	emitter domain.NotificationEmitter,
) *ForgotPasswordUseCase {
	return &ForgotPasswordUseCase{
		userRepo:     userRepo,
		tokenService: tokenService,
		notifEmitter: emitter,
	}
}

func (uc *ForgotPasswordUseCase) Execute(ctx context.Context, email string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		return ErrUserNotFound
	}

	token, err := uc.tokenService.CreateForgotPassToken(user.UUID, user.Email)
	if err != nil {
		return fmt.Errorf("failed creating password reset token: %w", err)
	}

	if uc.notifEmitter != nil {
		_ = uc.notifEmitter.Emit(ctx, "AUTH_FORGOT_PASSWORD", map[string]any{
			"email": user.Email,
			"name":  user.OrganizationName,
			"token": token,
		}, user.OrganizationUUID, &user.UUID)
	}

	return nil
}

// ResetPasswordUseCase applies a new password using a verified reset token.
type ResetPasswordUseCase struct {
	userRepo        domain.UserRepository
	passwordService domain.PasswordService
	tokenService    domain.TokenService
}

func NewResetPasswordUseCase(
	userRepo domain.UserRepository,
	pwService domain.PasswordService,
	tokenService domain.TokenService,
) *ResetPasswordUseCase {
	return &ResetPasswordUseCase{
		userRepo:        userRepo,
		passwordService: pwService,
		tokenService:    tokenService,
	}
}

func (uc *ResetPasswordUseCase) Execute(ctx context.Context, token, newPassword string) error {
	userUUID, email, err := uc.tokenService.VerifyForgotPassToken(token)
	if err != nil {
		return ErrInvalidToken
	}

	user, err := uc.userRepo.FindByUUID(ctx, userUUID)
	if err != nil || user == nil || user.Email != email {
		return ErrUserNotFound
	}

	hashedPassword, err := uc.passwordService.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed hashing password: %w", err)
	}

	_, err = uc.userRepo.Update(ctx, user.UUID, map[string]any{
		"password": hashedPassword,
	})
	return err
}

// CreateHelpdeskTokenUseCase generates short-lived RS256 JWT tokens for customer support desks.
type CreateHelpdeskTokenUseCase struct {
	tokenService domain.TokenService
}

func NewCreateHelpdeskTokenUseCase(tokenService domain.TokenService) *CreateHelpdeskTokenUseCase {
	return &CreateHelpdeskTokenUseCase{tokenService: tokenService}
}

func (uc *CreateHelpdeskTokenUseCase) Execute(ctx context.Context, userUUID uuid.UUID) (string, error) {
	return uc.tokenService.CreateHelpdeskToken(userUUID)
}
