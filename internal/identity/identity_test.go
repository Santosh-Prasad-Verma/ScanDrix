package identity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

func TestPermissionsEngineMatrix(t *testing.T) {
	engine := identity.NewPermissionsEngine()
	wsID := uuid.New()

	adminUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleAdmin,
	}

	maintainerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleMaintainer,
	}

	reviewerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleReviewer,
	}

	viewerUser := identity.UserProfile{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Role:        identity.RoleViewer,
	}

	// 1. Admin checks
	if !engine.Can(adminUser, identity.ActionDelete, identity.ResourceWorkspace) {
		t.Fatal("admin should be allowed to delete workspace")
	}
	if !engine.Can(adminUser, identity.ActionUpdate, identity.ResourceBYOK) {
		t.Fatal("admin should be allowed to update BYOK")
	}

	// 2. Maintainer checks
	if engine.Can(maintainerUser, identity.ActionDelete, identity.ResourceWorkspace) {
		t.Fatal("maintainer should NOT be allowed to delete workspace")
	}
	if engine.Can(maintainerUser, identity.ActionUpdate, identity.ResourceBYOK) {
		t.Fatal("maintainer should NOT be allowed to update BYOK")
	}
	if !engine.Can(maintainerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("maintainer should be allowed to trigger review")
	}

	// 3. Reviewer checks
	if !engine.Can(reviewerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("reviewer should be allowed to trigger review")
	}
	if engine.Can(reviewerUser, identity.ActionCreate, identity.ResourceRepo) {
		t.Fatal("reviewer should NOT be allowed to create repositories")
	}

	// 4. Viewer checks
	if engine.Can(viewerUser, identity.ActionTrigger, identity.ResourceReview) {
		t.Fatal("viewer should NOT be allowed to trigger reviews")
	}
	if !engine.Can(viewerUser, identity.ActionRead, identity.ResourceReview) {
		t.Fatal("viewer should be allowed to read reviews")
	}
}

func TestProfileService(t *testing.T) {
	svc := identity.NewProfileService()
	uid := uuid.New()
	wsID := uuid.New()

	profile := identity.UserProfile{
		ID:          uid,
		WorkspaceID: wsID,
		Email:       "engineer@scandrix.dev",
		DisplayName: "Lead Security Engineer",
		Role:        identity.RoleMaintainer,
	}

	// 1. Create Profile
	err := svc.CreateProfile(profile)
	if err != nil {
		t.Fatalf("failed creating profile: %v", err)
	}

	// 2. Prevent duplicate creation
	errDup := svc.CreateProfile(profile)
	if errDup == nil {
		t.Fatal("expected error creating duplicate profile ID")
	}

	// 3. Update Preferences
	err = svc.UpdatePreference(uid, "cli_theme", "dark-high-contrast")
	if err != nil {
		t.Fatalf("failed updating preference: %v", err)
	}

	// 4. Get Profile
	retrieved, err := svc.GetProfile(uid)
	if err != nil {
		t.Fatalf("failed retrieving profile: %v", err)
	}
	if retrieved.Preferences["cli_theme"] != "dark-high-contrast" {
		t.Fatalf("unexpected preference: %v", retrieved.Preferences)
	}
}

func TestCASLAbilityFactoryAndScoping(t *testing.T) {
	permRepo := infrastructure.NewInMemoryPermissionsRepository()
	factory := application.NewPermissionsAbilityFactory(permRepo)

	ctx := context.Background()
	orgUUID := uuid.New()
	userUUID := uuid.New()

	// Assign specific repositories
	_, err := permRepo.Create(ctx, domain.Permissions{
		UUID:                  uuid.New(),
		UserUUID:              userUUID,
		AssignedRepositoryIDs: []string{"repo-alpha", "repo-beta"},
	})
	if err != nil {
		t.Fatalf("failed creating perms: %v", err)
	}

	repoAdminUser := domain.User{
		UUID:             userUUID,
		Role:             domain.RoleRepoAdmin,
		OrganizationUUID: &orgUUID,
	}

	ability, err := factory.CreateForUser(ctx, repoAdminUser, nil)
	if err != nil {
		t.Fatalf("failed creating ability: %v", err)
	}

	// RepoAdmin can read code review settings org-wide
	if !ability.Can(domain.ActionRead, domain.ResourceCodeReviewSettings) {
		t.Fatal("repo admin should be able to read code review settings org-wide")
	}

	// RepoAdmin can update code review settings ONLY on assigned repos
	if !ability.CanInRepo(domain.ActionUpdate, domain.ResourceCodeReviewSettings, "repo-alpha") {
		t.Fatal("repo admin should be able to update assigned repo-alpha")
	}
	if ability.CanInRepo(domain.ActionUpdate, domain.ResourceCodeReviewSettings, "repo-unassigned") {
		t.Fatal("repo admin should NOT be able to update unassigned repo")
	}

	// Check RoleUsesRepoAssignment
	if !domain.RoleUsesRepoAssignment(domain.RoleRepoAdmin) {
		t.Fatal("repo admin role should use repo assignment")
	}
	if domain.RoleUsesRepoAssignment(domain.RoleOwner) {
		t.Fatal("owner role should NOT use repo assignment")
	}

	// Permissions map building
	permMap := ability.BuildPermissionsMap()
	if len(permMap) == 0 {
		t.Fatal("expected non-empty permissions map")
	}
	if _, ok := permMap[domain.ResourceDrixyRules]; !ok {
		t.Fatal("expected drixy_rules in permissions map")
	}
}

type mockNotificationEmitter struct {
	emitted []string
}

func (m *mockNotificationEmitter) Emit(ctx context.Context, event string, payload map[string]any, orgUUID *uuid.UUID, recipientUUID *uuid.UUID) error {
	m.emitted = append(m.emitted, event)
	return nil
}

type mockAuditLogEmitter struct {
	events []string
}

func (m *mockAuditLogEmitter) EmitAuditLog(ctx context.Context, event string, params map[string]any) error {
	m.events = append(m.events, event)
	return nil
}

func TestAuthLifecycleAndTokenRotation(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	authRepo := infrastructure.NewInMemoryAuthRepository()
	profileRepo := infrastructure.NewInMemoryProfileRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	jwtService := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})
	notif := &mockNotificationEmitter{}

	signUpUC := application.NewSignUpUseCase(userRepo, profileRepo, pwService, jwtService, notif)
	loginUC := application.NewLoginUseCase(userRepo, authRepo, pwService, jwtService)
	refreshUC := application.NewRefreshTokenUseCase(authRepo, userRepo, jwtService)
	logoutUC := application.NewLogoutUseCase(authRepo)

	// 1. SignUp
	createdUser, err := signUpUC.Execute(ctx, application.SignUpInput{
		Email:       "alice@scandrix.dev",
		Password:    "SuperSecretPassword123!",
		Name:        "Alice Developer",
		PreVerified: true,
	})
	if err != nil {
		t.Fatalf("signup failed: %v", err)
	}
	if createdUser.Role != domain.RoleOwner {
		t.Fatalf("expected role owner for first org creator, got: %s", createdUser.Role)
	}

	// Duplicate signup prevention
	_, errDup := signUpUC.Execute(ctx, application.SignUpInput{
		Email:    "alice@scandrix.dev",
		Password: "AnotherPassword123!",
		Name:     "Alice Clone",
	})
	if errDup != application.ErrDuplicateEmail {
		t.Fatalf("expected duplicate email error, got: %v", errDup)
	}

	// 2. Login
	tokens, err := loginUC.Execute(ctx, "alice@scandrix.dev", "SuperSecretPassword123!")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("empty tokens returned")
	}

	// Bad password
	_, errBadPW := loginUC.Execute(ctx, "alice@scandrix.dev", "WrongPassword!")
	if errBadPW != application.ErrUnauthorized {
		t.Fatalf("expected unauthorized for bad password, got: %v", errBadPW)
	}

	// 3. Refresh Token Rotation
	newTokens, err := refreshUC.Execute(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("refresh token failed: %v", err)
	}
	if newTokens.AccessToken == "" || newTokens.RefreshToken == "" {
		t.Fatal("empty tokens returned from refresh")
	}

	// 4. Token Reuse Detection (Replay attack)
	_, errReplay := refreshUC.Execute(ctx, tokens.RefreshToken)
	if errReplay != application.ErrTokenAlreadyUsed {
		t.Fatalf("expected token already used error on replay, got: %v", errReplay)
	}

	// 5. Logout
	err = logoutUC.Execute(ctx, newTokens.RefreshToken)
	if err != nil {
		t.Fatalf("logout failed: %v", err)
	}
}

func TestCliAuthFlows(t *testing.T) {
	ctx := context.Background()
	sessionRepo := infrastructure.NewInMemoryCliAuthSessionRepository()
	userRepo := infrastructure.NewInMemoryUserRepository()
	pwService := infrastructure.NewBcryptPasswordService(10)
	jwtService := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
	})

	initLoopbackUC := application.NewInitiateCliLoginUseCase(sessionRepo, "https://app.scandrix.dev")
	initDeviceUC := application.NewInitiateCliDeviceLoginUseCase(sessionRepo, "https://app.scandrix.dev")
	pollUC := application.NewPollCliLoginUseCase(sessionRepo)
	completeUC := application.NewCompleteCliLoginUseCase(sessionRepo, jwtService)
	infoUC := application.NewGetCliLoginInfoUseCase(sessionRepo)

	// Create test user
	hashedPW, _ := pwService.HashPassword("cli-pass-123")
	user, err := userRepo.Create(ctx, domain.User{
		UUID:     uuid.New(),
		Email:    "dev@scandrix.dev",
		Password: hashedPW,
		Role:     domain.RoleOwner,
		Status:   domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("failed creating user: %v", err)
	}

	// 1. Loopback Flow
	loopbackRes, err := initLoopbackUC.Execute(ctx, application.InitiateCliLoginInput{
		Port:      8085,
		UserAgent: "ScanDrix-CLI/1.0.0",
	})
	if err != nil {
		t.Fatalf("initiate loopback failed: %v", err)
	}
	if !strings.Contains(loopbackRes.VerificationURI, "https://app.scandrix.dev/cli/authorize?state=") {
		t.Fatalf("unexpected verification URI: %s", loopbackRes.VerificationURI)
	}

	// Check info safe read
	info, err := infoUC.Execute(ctx, loopbackRes.State, "")
	if err != nil || !info.Found || info.Mode != domain.CliAuthModeLoopback {
		t.Fatalf("info lookup failed: %+v", info)
	}

	// Poll while pending
	pollPending, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: loopbackRes.State})
	if err != nil || pollPending.Status != domain.CliAuthStatusPending {
		t.Fatalf("expected pending status, got: %+v", pollPending)
	}

	// Complete authorization
	completeRes, err := completeUC.Execute(ctx, application.CompleteCliLoginInput{
		State: loopbackRes.State,
		User:  *user,
	})
	if err != nil {
		t.Fatalf("complete CLI auth failed: %v", err)
	}
	if completeRes.RedirectURI == nil || *completeRes.RedirectURI != "http://127.0.0.1:8085/callback" {
		t.Fatalf("unexpected redirect URI: %v", completeRes.RedirectURI)
	}

	// Poll completed
	pollDone, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: loopbackRes.State})
	if err != nil || pollDone.Status != domain.CliAuthStatusCompleted {
		t.Fatalf("expected completed status, got: %+v", pollDone)
	}
	if pollDone.AccessToken == nil || *pollDone.AccessToken == "" {
		t.Fatal("expected access token in completed poll")
	}

	// Poll after consumed should return consumed with no token (replay protection)
	pollConsumed, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: loopbackRes.State})
	if err != nil || pollConsumed.Status != domain.CliAuthStatusConsumed {
		t.Fatalf("expected consumed status, got: %+v", pollConsumed)
	}
	if pollConsumed.AccessToken != nil {
		t.Fatal("consumed session must not return access token")
	}

	// 2. Device Flow
	deviceRes, err := initDeviceUC.Execute(ctx, application.InitiateCliDeviceLoginInput{
		UserAgent: "ScanDrix-CLI/1.0.0",
	})
	if err != nil {
		t.Fatalf("initiate device login failed: %v", err)
	}
	// Check user code format: XXXX-XXXX (8 chars plus hyphen)
	if len(deviceRes.UserCode) != 9 || !strings.Contains(deviceRes.UserCode, "-") {
		t.Fatalf("unexpected user code format: %s", deviceRes.UserCode)
	}

	// Complete device authorization using userCode
	_, err = completeUC.Execute(ctx, application.CompleteCliLoginInput{
		UserCode: deviceRes.UserCode,
		User:     *user,
	})
	if err != nil {
		t.Fatalf("complete device auth failed: %v", err)
	}

	// Poll by deviceCode
	pollDevice, err := pollUC.Execute(ctx, application.PollCliLoginInput{DeviceCode: deviceRes.DeviceCode})
	if err != nil || pollDevice.Status != domain.CliAuthStatusCompleted {
		t.Fatalf("expected completed device poll, got: %+v", pollDevice)
	}
}

func TestUserManagementAndProfileConfigs(t *testing.T) {
	ctx := context.Background()
	userRepo := infrastructure.NewInMemoryUserRepository()
	profileRepo := infrastructure.NewInMemoryProfileRepository()
	configRepo := infrastructure.NewInMemoryProfileConfigRepository()
	audit := &mockAuditLogEmitter{}
	notif := &mockNotificationEmitter{}

	orgUUID := uuid.New()
	adminUUID := uuid.New()

	// Create admin
	_, _ = userRepo.Create(ctx, domain.User{
		UUID:             adminUUID,
		Email:            "admin@scandrix.dev",
		Role:             domain.RoleOwner,
		OrganizationUUID: &orgUUID,
		OrganizationName: "ScanDrix Corp",
		Status:           domain.UserStatusActive,
	})

	// Create target user
	targetUUID := uuid.New()
	_, _ = userRepo.Create(ctx, domain.User{
		UUID:             targetUUID,
		Email:            "developer@scandrix.dev",
		Role:             domain.RoleContributor,
		OrganizationUUID: &orgUUID,
		OrganizationName: "ScanDrix Corp",
		Status:           domain.UserStatusActive,
	})

	// 1. UpdateAnotherUserUseCase (Role promotion)
	updateAnotherUC := application.NewUpdateAnotherUserUseCase(userRepo, audit, notif)
	newRole := domain.RoleRepoAdmin
	updatedUser, err := updateAnotherUC.Execute(ctx, application.UpdateAnotherUserInput{
		ActingUserUUID:   adminUUID,
		ActingEmail:      "admin@scandrix.dev",
		TargetUserUUID:   targetUUID,
		OrganizationUUID: orgUUID,
		NewRole:          &newRole,
	})
	if err != nil {
		t.Fatalf("update another user failed: %v", err)
	}
	if updatedUser.Role != domain.RoleRepoAdmin {
		t.Fatalf("expected new role repo_admin, got: %s", updatedUser.Role)
	}
	if len(audit.events) == 0 || audit.events[0] != "USER_ROLE_CHANGE" {
		t.Fatalf("expected audit event USER_ROLE_CHANGE, got: %v", audit.events)
	}
	if len(notif.emitted) == 0 || notif.emitted[0] != "ORG_ROLE_CHANGED" {
		t.Fatalf("expected notification ORG_ROLE_CHANGED, got: %v", notif.emitted)
	}

	// 2. ProfileConfigService
	profileUUID := uuid.New()
	_ = profileRepo.UpdateByUserID(ctx, targetUUID, domain.UserProfile{
		UUID:     profileUUID,
		UserUUID: targetUUID,
		Name:     "Test Developer",
		Status:   true,
	})

	configSvc := application.NewProfileConfigService(configRepo)
	cfg, err := configSvc.SetConfig(ctx, profileUUID, domain.ProfileConfigUserNotifications, map[string]any{
		"email": true,
		"slack": false,
	})
	if err != nil {
		t.Fatalf("set config failed: %v", err)
	}
	if cfg.ConfigKey != domain.ProfileConfigUserNotifications {
		t.Fatalf("unexpected key: %s", cfg.ConfigKey)
	}

	retrievedCfg, err := configSvc.GetConfig(ctx, profileUUID, domain.ProfileConfigUserNotifications)
	if err != nil || retrievedCfg == nil {
		t.Fatalf("get config failed: %v", err)
	}

	configsList, err := configSvc.ListConfigs(ctx, profileUUID)
	if err != nil || len(configsList) != 1 {
		t.Fatalf("expected 1 config, got: %d", len(configsList))
	}
}
