package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

type mockNotificationEmitter struct {
	emittedEvents   []string
	emittedPayloads []map[string]any
	shouldFail      bool
}

func (m *mockNotificationEmitter) Emit(ctx context.Context, event string, payload map[string]any, orgUUID *uuid.UUID, recipientUUID *uuid.UUID) error {
	if m.shouldFail {
		return errors.New("outbox down")
	}
	m.emittedEvents = append(m.emittedEvents, event)
	m.emittedPayloads = append(m.emittedPayloads, payload)
	return nil
}

type mockAuditLogEmitter struct {
	events []string
	params []map[string]any
}

func (m *mockAuditLogEmitter) EmitAuditLog(ctx context.Context, event string, params map[string]any) error {
	m.events = append(m.events, event)
	m.params = append(m.params, params)
	return nil
}

func TestSignUpUseCase_OrgJoiningStatusMatrix(t *testing.T) {
	// Matrix: (CLOUD vs SELF-HOSTED) × (self-claim vs IdP-verified).
	// The owner-create branch (no organizationId) is unconditionally ACTIVE and
	// exercised separately below to lock that contract in.

	setup := func() (*application.SignUpUseCase, domain.UserRepository) {
		userRepo := infrastructure.NewInMemoryUserRepository()
		profileRepo := infrastructure.NewInMemoryProfileRepository()
		pwService := infrastructure.NewBcryptPasswordService(10)
		jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
			Secret:        "test-secret-key-32-chars-long-abc",
			RefreshSecret: "test-refresh-secret-32-chars-long",
		})
		if ctorErr != nil {
			t.Fatalf("jwtService construction failed: %v", ctorErr)
		}
		notif := &mockNotificationEmitter{}
		uc := application.NewSignUpUseCase(userRepo, profileRepo, pwService, jwtService, notif)
		return uc, userRepo
	}

	ctx := context.Background()
	orgID := uuid.New()

	t.Run("with organizationId (auto-join / SSO path)", func(t *testing.T) {
		t.Run("cloud + self-claim -> PENDING (must confirm email)", func(t *testing.T) {
			uc, _ := setup()
			uc.SetCloudMode(true)

			user, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "sso-user1@scandrix.dev",
				Name:           "SSO User 1",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    false,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Status != domain.UserStatusPending {
				t.Fatalf("expected PENDING status, got %s", user.Status)
			}
			if user.Role != domain.RoleContributor {
				t.Fatalf("expected CONTRIBUTOR role, got %s", user.Role)
			}
		})

		t.Run("self-hosted + self-claim -> ACTIVE (no email infra)", func(t *testing.T) {
			uc, _ := setup()
			uc.SetCloudMode(false)

			user, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "sso-user2@scandrix.dev",
				Name:           "SSO User 2",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    false,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Status != domain.UserStatusActive {
				t.Fatalf("expected ACTIVE status, got %s", user.Status)
			}
		})

		t.Run("cloud + preVerified (SSO) -> ACTIVE (IdP attested)", func(t *testing.T) {
			uc, _ := setup()
			uc.SetCloudMode(true)

			user, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "sso-user3@scandrix.dev",
				Name:           "SSO User 3",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Status != domain.UserStatusActive {
				t.Fatalf("expected ACTIVE status, got %s", user.Status)
			}
		})

		t.Run("self-hosted + preVerified (SSO) -> ACTIVE", func(t *testing.T) {
			uc, _ := setup()
			uc.SetCloudMode(false)

			user, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "sso-user4@scandrix.dev",
				Name:           "SSO User 4",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Status != domain.UserStatusActive {
				t.Fatalf("expected ACTIVE status, got %s", user.Status)
			}
		})
	})

	t.Run("without organizationId (owner self-signup)", func(t *testing.T) {
		t.Run("always ACTIVE owner regardless of cloud mode", func(t *testing.T) {
			uc, _ := setup()
			uc.SetCloudMode(true)

			user, err := uc.Execute(ctx, application.SignUpInput{
				Email:    "owner@scandrix.dev",
				Name:     "Owner User",
				Password: "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Status != domain.UserStatusActive {
				t.Fatalf("expected ACTIVE status for owner, got %s", user.Status)
			}
			if user.Role != domain.RoleOwner {
				t.Fatalf("expected OWNER role, got %s", user.Role)
			}
		})
	})

	t.Run("team_member membership status (P3 regression)", func(t *testing.T) {
		// The team_member.status flag controls whether the member shows up
		// in the Workspace members list. It must be active for owners and for
		// trusted provisioning (SSO/preVerified), and must stay inactive for a
		// plain non-owner join — regardless of cloud mode. Guards the
		// status: isOwner || !!options?.preVerified rule against regression.

		t.Run("SSO provisioning (preVerified, non-owner) -> membership ACTIVE — even in cloud", func(t *testing.T) {
			uc, userRepo := setup()
			uc.SetCloudMode(true)

			created, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "sso@scandrix.dev",
				Name:           "SSO Provisioned",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fullUser, _ := userRepo.FindByUUID(ctx, created.UUID)
			if len(fullUser.TeamMembers) == 0 {
				t.Fatal("expected team member record")
			}
			if !fullUser.TeamMembers[0].Status {
				t.Fatal("expected team member status to be true for preVerified SSO user")
			}
		})

		t.Run("plain non-owner join (no preVerified) -> membership INACTIVE — unchanged", func(t *testing.T) {
			uc, userRepo := setup()
			uc.SetCloudMode(true)

			created, err := uc.Execute(ctx, application.SignUpInput{
				Email:          "invitee@scandrix.dev",
				Name:           "Invitee",
				Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
				OrganizationID: &orgID,
				PreVerified:    false,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fullUser, _ := userRepo.FindByUUID(ctx, created.UUID)
			if len(fullUser.TeamMembers) == 0 {
				t.Fatal("expected team member record")
			}
			if fullUser.TeamMembers[0].Status {
				t.Fatal("expected team member status to be false for plain non-owner join")
			}
		})

		t.Run("owner self-signup -> membership ACTIVE", func(t *testing.T) {
			uc, userRepo := setup()
			uc.SetCloudMode(true)

			created, err := uc.Execute(ctx, application.SignUpInput{
				Email:    "owner-reg@scandrix.dev",
				Name:     "Owner Reg",
				Password: "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fullUser, _ := userRepo.FindByUUID(ctx, created.UUID)
			if len(fullUser.TeamMembers) == 0 {
				t.Fatal("expected team member record")
			}
			if !fullUser.TeamMembers[0].Status {
				t.Fatal("expected team member status to be true for owner")
			}
		})
	})
}

func TestOAuthLoginUseCase_Provisioning(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	authRepo := infrastructure.NewInMemoryAuthRepository()
	profileRepo := infrastructure.NewInMemoryProfileRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	notif := &mockNotificationEmitter{}

	signUpUC := application.NewSignUpUseCase(userRepo, profileRepo, pwService, jwtService, notif)
	oauthUC := application.NewOAuthLoginUseCase(userRepo, authRepo, jwtService, signUpUC)

	// First time OAuth login provisions user
	tokens, err := oauthUC.Execute(ctx, "GitHub Dev", "github-dev@scandrix.dev", "refresh-tok-123", domain.AuthProviderGitHub)
	if err != nil {
		t.Fatalf("oauth login failed: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}

	// Verify user exists and is active
	user, err := userRepo.FindByEmail(ctx, "github-dev@scandrix.dev")
	if err != nil || user == nil {
		t.Fatal("expected user to be created")
	}
	if user.Status != domain.UserStatusActive {
		t.Fatalf("expected active status, got: %s", user.Status)
	}

	// Second time login with same email reuses account
	tokens2, err := oauthUC.Execute(ctx, "GitHub Dev", "github-dev@scandrix.dev", "refresh-tok-456", domain.AuthProviderGitHub)
	if err != nil {
		t.Fatalf("second oauth login failed: %v", err)
	}
	if tokens2.AccessToken == "" {
		t.Fatal("expected access token on subsequent login")
	}
}

func TestPasswordResetAndEmailConfirmationFlow(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	profileRepo := infrastructure.NewInMemoryProfileRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	notif := &mockNotificationEmitter{}

	signUpUC := application.NewSignUpUseCase(userRepo, profileRepo, pwService, jwtService, notif)
	forgotPWUC := application.NewForgotPasswordUseCase(userRepo, jwtService, notif)
	resetPWUC := application.NewResetPasswordUseCase(userRepo, pwService, jwtService)
	confirmEmailUC := application.NewConfirmEmailUseCase(userRepo, jwtService)

	orgID := uuid.New()
	user, err := signUpUC.Execute(ctx, application.SignUpInput{
		Email:          "pending-user@scandrix.dev",
		Name:           "Pending User",
		Password:       "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8",
		OrganizationID: &orgID,
		PreVerified:    false,
	})
	if err != nil {
		t.Fatalf("signup failed: %v", err)
	}
	if user.Status != domain.UserStatusPending {
		t.Fatalf("expected pending status, got %s", user.Status)
	}

	// 1. Confirm Email
	emailToken, err := jwtService.CreateEmailToken(user.UUID, user.Email)
	if err != nil {
		t.Fatalf("failed creating email token: %v", err)
	}
	err = confirmEmailUC.Execute(ctx, emailToken)
	if err != nil {
		t.Fatalf("confirm email failed: %v", err)
	}
	confirmedUser, _ := userRepo.FindByUUID(ctx, user.UUID)
	if confirmedUser.Status != domain.UserStatusActive {
		t.Fatalf("expected active status after confirmation, got: %s", confirmedUser.Status)
	}

	// 2. Forgot Password
	err = forgotPWUC.Execute(ctx, "pending-user@scandrix.dev")
	if err != nil {
		t.Fatalf("forgot password failed: %v", err)
	}
	if len(notif.emittedEvents) == 0 {
		t.Fatal("expected notification event emitted")
	}

	// 3. Reset Password
	resetToken, err := jwtService.CreateForgotPassToken(user.UUID, user.Email)
	if err != nil {
		t.Fatalf("failed creating reset token: %v", err)
	}
	err = resetPWUC.Execute(ctx, resetToken, "BrandZq7-Kv4-Mn9-Tb2-Xc6-Rp8")
	if err != nil {
		t.Fatalf("reset password failed: %v", err)
	}

	// Verify updated password
	updated, _ := userRepo.FindByUUID(ctx, user.UUID)
	if !pwService.MatchPassword("BrandZq7-Kv4-Mn9-Tb2-Xc6-Rp8", updated.Password) {
		t.Fatal("expected new password to match")
	}
}

func TestConfirmEmailUseCase_EdgeCases(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	uc := application.NewConfirmEmailUseCase(userRepo, jwtService)

	// 1. Invalid token
	err := uc.Execute(ctx, "invalid-garbage-token")
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}

	// 2. Token for non-existent user
	nonExistentUUID := uuid.New()
	token, err := jwtService.CreateEmailToken(nonExistentUUID, "ghost@scandrix.dev")
	if err != nil {
		t.Fatalf("failed creating token: %v", err)
	}
	err = uc.Execute(ctx, token)
	if !errors.Is(err, application.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	// 3. User already active -> no-op success
	user := domain.User{
		UUID:   uuid.New(),
		Email:  "active@scandrix.dev",
		Status: domain.UserStatusActive,
	}
	_, _ = userRepo.Create(ctx, user)
	activeToken, _ := jwtService.CreateEmailToken(user.UUID, user.Email)
	err = uc.Execute(ctx, activeToken)
	if err != nil {
		t.Fatalf("expected nil error for already active user, got: %v", err)
	}
}

func TestResendEmailUseCase(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	notif := &mockNotificationEmitter{}
	uc := application.NewResendEmailUseCase(userRepo, jwtService, notif)

	// 1. Non-existent user
	err := uc.Execute(ctx, "nonexistent@scandrix.dev")
	if !errors.Is(err, application.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	// 2. Already active user is a no-op
	activeUser := domain.User{
		UUID:   uuid.New(),
		Email:  "active@scandrix.dev",
		Status: domain.UserStatusActive,
	}
	_, _ = userRepo.Create(ctx, activeUser)
	err = uc.Execute(ctx, "active@scandrix.dev")
	if err != nil {
		t.Fatalf("expected nil error for active user, got: %v", err)
	}
	if len(notif.emittedEvents) != 0 {
		t.Errorf("expected no event emitted for active user")
	}

	// 3. Pending user receives confirmation email
	pendingUser := domain.User{
		UUID:   uuid.New(),
		Email:  "pending@scandrix.dev",
		Status: domain.UserStatusPending,
	}
	_, _ = userRepo.Create(ctx, pendingUser)
	err = uc.Execute(ctx, "pending@scandrix.dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notif.emittedEvents) != 1 || notif.emittedEvents[0] != "AUTH_CONFIRM_EMAIL" {
		t.Errorf("expected AUTH_CONFIRM_EMAIL event, got: %v", notif.emittedEvents)
	}
}

func TestForgotPasswordUseCase_EdgeCases(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	notif := &mockNotificationEmitter{}
	uc := application.NewForgotPasswordUseCase(userRepo, jwtService, notif)

	// 1. User not found
	err := uc.Execute(ctx, "unknown@scandrix.dev")
	if !errors.Is(err, application.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	// 2. Existing user
	user := domain.User{
		UUID:             uuid.New(),
		Email:            "dev@scandrix.dev",
		OrganizationName: "ScanDrix Inc",
		Status:           domain.UserStatusActive,
	}
	_, _ = userRepo.Create(ctx, user)
	err = uc.Execute(ctx, "dev@scandrix.dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notif.emittedEvents) != 1 || notif.emittedEvents[0] != "AUTH_FORGOT_PASSWORD" {
		t.Errorf("expected AUTH_FORGOT_PASSWORD event, got: %v", notif.emittedEvents)
	}
}

func TestResetPasswordUseCase_EdgeCases(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	jwtService, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtService construction failed: %v", ctorErr)
	}
	uc := application.NewResetPasswordUseCase(userRepo, pwService, jwtService)

	// 1. Invalid token
	err := uc.Execute(ctx, "invalid-token", "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8")
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}

	// 2. Token for non-existent user
	token, _ := jwtService.CreateForgotPassToken(uuid.New(), "ghost@scandrix.dev")
	err = uc.Execute(ctx, token, "Zq7-Kv4-Mn9-Tb2-Xc6-Rp8")
	if !errors.Is(err, application.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestCreateHelpdeskTokenUseCase(t *testing.T) {
	ctx := context.Background()

	// 1. Fallback to HMAC when no RSA key configured
	jwtServiceHMAC, ctorErr := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	if ctorErr != nil {
		t.Fatalf("jwtServiceHMAC construction failed: %v", ctorErr)
	}
	ucHMAC := application.NewCreateHelpdeskTokenUseCase(jwtServiceHMAC)

	token, err := ucHMAC.Execute(ctx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error with HMAC fallback: %v", err)
	}
	if token == "" {
		t.Errorf("expected non-empty helpdesk token")
	}
}
