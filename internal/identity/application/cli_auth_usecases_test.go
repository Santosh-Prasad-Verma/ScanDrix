package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/application"
	"github.com/scandrix/backend/internal/identity/domain"
	"github.com/scandrix/backend/internal/identity/infrastructure"
)

func TestInitiateCliLoginUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryCliAuthSessionRepository()
	uc := application.NewInitiateCliLoginUseCase(repo, "https://app.scandrix.dev")

	// 1. Port bounds validation
	_, err := uc.Execute(ctx, application.InitiateCliLoginInput{Port: 80})
	if err == nil {
		t.Fatalf("expected error for privileged port 80")
	}

	_, err = uc.Execute(ctx, application.InitiateCliLoginInput{Port: 70000})
	if err == nil {
		t.Fatalf("expected error for out of bounds port 70000")
	}

	// 2. Valid loopback initiation
	res, err := uc.Execute(ctx, application.InitiateCliLoginInput{Port: 8085, UserAgent: "ScanDrix-CLI/1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error initiating CLI login: %v", err)
	}

	if res.State == "" {
		t.Errorf("expected non-empty state")
	}
	if !strings.Contains(res.VerificationURI, "app.scandrix.dev/cli/authorize?state=") {
		t.Errorf("unexpected verification URI: %s", res.VerificationURI)
	}

	// Session stored in repo
	sess, err := repo.FindByState(ctx, res.State)
	if err != nil || sess == nil {
		t.Fatalf("expected session to be created in repo")
	}
	if sess.Status != domain.CliAuthStatusPending {
		t.Errorf("expected status pending, got: %s", sess.Status)
	}
	if sess.RedirectURI == nil || *sess.RedirectURI != "http://127.0.0.1:8085/callback" {
		t.Errorf("unexpected redirect URI: %v", sess.RedirectURI)
	}

	// 3. Loopback redirect validation helper
	if !application.IsLoopbackRedirect("http://127.0.0.1:8085/callback") {
		t.Errorf("expected 127.0.0.1 to be valid loopback")
	}
	if !application.IsLoopbackRedirect("http://localhost:3000/callback") {
		t.Errorf("expected localhost to be valid loopback")
	}
	if application.IsLoopbackRedirect("https://evil.com/callback") {
		t.Errorf("expected evil.com to be rejected")
	}
	if application.IsLoopbackRedirect("") {
		t.Errorf("expected empty string to be rejected")
	}
}

func TestInitiateCliDeviceLoginUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryCliAuthSessionRepository()
	uc := application.NewInitiateCliDeviceLoginUseCase(repo, "https://app.scandrix.dev")

	res, err := uc.Execute(ctx, application.InitiateCliDeviceLoginInput{UserAgent: "ScanDrix-CLI/1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeviceCode == "" {
		t.Errorf("expected device code")
	}
	if res.UserCode == "" || len(res.UserCode) != 9 || res.UserCode[4] != '-' {
		t.Errorf("expected user code format XXXX-XXXX, got: %s", res.UserCode)
	}
	if res.Interval != 5 {
		t.Errorf("expected poll interval 5s, got: %d", res.Interval)
	}
	if !strings.Contains(res.VerificationURI, "app.scandrix.dev/cli/authorize") {
		t.Errorf("unexpected verification URI: %s", res.VerificationURI)
	}
	if !strings.Contains(res.VerificationURIComplete, "code="+res.UserCode) {
		t.Errorf("unexpected complete verification URI: %s", res.VerificationURIComplete)
	}

	sess, err := repo.FindByUserCode(ctx, res.UserCode)
	if err != nil || sess == nil {
		t.Fatalf("expected session lookup by user code")
	}
	if sess.Mode != domain.CliAuthModeDevice {
		t.Errorf("expected mode device, got: %s", sess.Mode)
	}
}

func TestPollCliLoginUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryCliAuthSessionRepository()
	pollUC := application.NewPollCliLoginUseCase(repo)

	// 1. Not found for non-existent session
	res, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: "non-existent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "not_found" {
		t.Errorf("expected not_found, got: %s", res.Status)
	}

	// 2. Pending session
	now := time.Now().UTC()
	sessID := uuid.New()
	redirect := "http://127.0.0.1:8085/callback"
	sess := domain.CliAuthSession{
		UUID:        sessID,
		State:       "test-state-123",
		RedirectURI: &redirect,
		Status:      domain.CliAuthStatusPending,
		Mode:        domain.CliAuthModeLoopback,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, _ = repo.Create(ctx, sess)

	pollRes, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: "test-state-123"})
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}
	if pollRes.Status != domain.CliAuthStatusPending {
		t.Errorf("expected pending status, got: %s", pollRes.Status)
	}
	if pollRes.AccessToken != nil {
		t.Errorf("expected nil token for pending session")
	}

	// 3. Complete session
	uID := uuid.New()
	email := "developer@scandrix.dev"
	toks := domain.TokenResponse{
		AccessToken:  "jwt-access-token-abc",
		RefreshToken: "jwt-refresh-token-xyz",
	}
	_, err = repo.Complete(ctx, sessID, toks, uID, email)
	if err != nil {
		t.Fatalf("failed completing session: %v", err)
	}

	// 4. Poll completed session (first time -> completed)
	pollRes, err = pollUC.Execute(ctx, application.PollCliLoginInput{State: "test-state-123"})
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}
	if pollRes.Status != domain.CliAuthStatusCompleted {
		t.Errorf("expected completed status, got: %s", pollRes.Status)
	}
	if pollRes.AccessToken == nil || *pollRes.AccessToken != "jwt-access-token-abc" {
		t.Errorf("expected access token to be returned")
	}
	if pollRes.UserEmail == nil || *pollRes.UserEmail != email {
		t.Errorf("expected email %s, got: %v", email, pollRes.UserEmail)
	}

	// 5. Poll consumed session (second time -> consumed)
	pollRes2, err := pollUC.Execute(ctx, application.PollCliLoginInput{State: "test-state-123"})
	if err != nil {
		t.Fatalf("unexpected poll error on second call: %v", err)
	}
	if pollRes2.Status != domain.CliAuthStatusConsumed {
		t.Errorf("expected consumed status on repeat poll, got: %s", pollRes2.Status)
	}
}

func TestCompleteCliLoginUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryCliAuthSessionRepository()
	jwtService := infrastructure.NewJwtTokenService(domain.JWTConfig{
		Secret:        "test-secret-key-32-chars-long-abc",
		RefreshSecret: "test-refresh-secret-32-chars-long",
		ExpiresIn:     15 * time.Minute,
	})
	completeUC := application.NewCompleteCliLoginUseCase(repo, jwtService)

	now := time.Now().UTC()
	sessID := uuid.New()
	redirect := "http://127.0.0.1:9090/callback"
	sess := domain.CliAuthSession{
		UUID:        sessID,
		State:       "cli-state-456",
		RedirectURI: &redirect,
		Status:      domain.CliAuthStatusPending,
		Mode:        domain.CliAuthModeLoopback,
		ExpiresAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, _ = repo.Create(ctx, sess)

	userUUID := uuid.New()
	orgUUID := uuid.New()
	testUser := domain.User{
		UUID:             userUUID,
		Email:            "cli-coder@scandrix.dev",
		Name:             "CLI Coder",
		Role:             domain.RoleContributor,
		Status:           domain.UserStatusActive,
		OrganizationUUID: &orgUUID,
	}

	// 1. Complete successfully
	res, err := completeUC.Execute(ctx, application.CompleteCliLoginInput{
		State: "cli-state-456",
		User:  testUser,
	})
	if err != nil {
		t.Fatalf("unexpected complete error: %v", err)
	}
	if res.State != "cli-state-456" {
		t.Errorf("expected state cli-state-456, got: %s", res.State)
	}
	if res.RedirectURI == nil || *res.RedirectURI != redirect {
		t.Errorf("expected redirect URI %s, got: %v", redirect, res.RedirectURI)
	}

	// 2. Session should be completed in repo with tokens
	completedSess, err := repo.FindByState(ctx, "cli-state-456")
	if err != nil || completedSess == nil {
		t.Fatalf("expected completed session")
	}
	if completedSess.Status != domain.CliAuthStatusCompleted {
		t.Errorf("expected status completed, got: %s", completedSess.Status)
	}
	if completedSess.AccessToken == nil || *completedSess.AccessToken == "" {
		t.Errorf("expected generated access token")
	}
	if completedSess.RefreshToken == nil || *completedSess.RefreshToken == "" {
		t.Errorf("expected generated refresh token")
	}

	// 3. Completing again should fail because status is not pending
	_, err = completeUC.Execute(ctx, application.CompleteCliLoginInput{
		State: "cli-state-456",
		User:  testUser,
	})
	if err == nil {
		t.Fatalf("expected error completing already completed session")
	}
}

func TestGetCliLoginInfoUseCase(t *testing.T) {
	ctx := context.Background()
	repo := infrastructure.NewInMemoryCliAuthSessionRepository()
	infoUC := application.NewGetCliLoginInfoUseCase(repo)

	// Non-existent
	res, err := infoUC.Execute(ctx, "unknown-state", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Found {
		t.Errorf("expected Found to be false for unknown state")
	}

	// Existing device code session
	now := time.Now().UTC()
	deviceCode := "device-code-abc"
	userCode := "ABCD-EFGH"
	ua := "ScanDrix-CLI/1.0.0 (Linux)"
	sess := domain.CliAuthSession{
		UUID:       uuid.New(),
		State:      "device-state-789",
		DeviceCode: &deviceCode,
		UserCode:   &userCode,
		Status:     domain.CliAuthStatusPending,
		Mode:       domain.CliAuthModeDevice,
		UserAgent:  &ua,
		ExpiresAt:  now.Add(10 * time.Minute),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, _ = repo.Create(ctx, sess)

	info, err := infoUC.Execute(ctx, "", userCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Found {
		t.Fatalf("expected session to be found by user code")
	}
	if info.Mode != domain.CliAuthModeDevice {
		t.Errorf("expected mode device, got: %s", info.Mode)
	}
	if info.Status != domain.CliAuthStatusPending {
		t.Errorf("expected pending, got: %s", info.Status)
	}
	if info.UserAgent == nil || *info.UserAgent != ua {
		t.Errorf("expected user agent %s, got: %v", ua, info.UserAgent)
	}
}
