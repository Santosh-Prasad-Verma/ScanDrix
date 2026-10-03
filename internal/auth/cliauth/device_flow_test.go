package cliauth_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/pkg/models"
)

func TestUnbiasedUserCodeGeneration(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := cliauth.GenerateUnbiasedUserCode()
		if err != nil {
			t.Fatalf("unexpected error generating code: %v", err)
		}

		// Verify format XXXX-XXXX
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("invalid code format: %s", code)
		}

		// Ensure no excluded ambiguous characters (0, O, 1, I)
		for _, ch := range strings.ReplaceAll(code, "-", "") {
			if strings.ContainsRune("0O1I", ch) {
				t.Fatalf("code contains ambiguous char: %c in %s", ch, code)
			}
		}

		if seen[code] {
			t.Fatalf("duplicate code generated: %s", code)
		}
		seen[code] = true
	}
}

func TestCLIDeviceFlowLifecycle(t *testing.T) {
	ctx := context.Background()
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	// 1. Initiate device login
	initRes, err := manager.InitiateDeviceLogin(ctx, "ScanDrix-CLI/v1.0")
	if err != nil {
		t.Fatalf("failed initiating device login: %v", err)
	}

	if initRes.DeviceCode == "" || initRes.UserCode == "" {
		t.Fatalf("missing device or user code: %+v", initRes)
	}
	if !strings.Contains(initRes.VerificationURIComplete, initRes.UserCode) {
		t.Fatalf("verification URI does not contain user code: %s", initRes.VerificationURIComplete)
	}

	// 2. Poll while pending
	pollRes, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if pollRes.Status != cliauth.StatusPending {
		t.Fatalf("expected pending status, got %s", pollRes.Status)
	}
	if pollRes.AccessToken != "" {
		t.Fatal("pending poll should not contain access token")
	}

	// 3. Complete device authorization in browser
	userProfile := &models.AccountProfile{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
		Email:       "alice@example.com",
		DisplayName: "Alice Smith",
		Role:        models.RoleAdmin,
	}

	fakeAccess := "fake_access_jwt_12345"
	fakeRefresh := "fake_refresh_token_67890"

	err = manager.CompleteDeviceLogin(ctx, initRes.UserCode, fakeAccess, fakeRefresh, userProfile)
	if err != nil {
		t.Fatalf("failed to complete device login: %v", err)
	}

	// 4. Poll after completion -> yields tokens and marks consumed
	pollResCompleted, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if err != nil {
		t.Fatalf("poll completed failed: %v", err)
	}
	if pollResCompleted.Status != cliauth.StatusCompleted {
		t.Fatalf("expected completed status, got %s", pollResCompleted.Status)
	}
	if pollResCompleted.AccessToken != fakeAccess || pollResCompleted.RefreshToken != fakeRefresh {
		t.Fatalf("tokens mismatch: %+v", pollResCompleted)
	}
	if pollResCompleted.UserEmail != "alice@example.com" {
		t.Fatalf("email mismatch: %s", pollResCompleted.UserEmail)
	}

	// 5. Re-poll a consumed session -> replay defence.
	//
	// This used to assert only that no tokens came back, and accepted a bare
	// "consumed" status with a nil error. The status alone is a weak signal for
	// the caller: RFC 8628 3.5 expects an error once a grant has been issued,
	// and "consumed" is exactly the condition that tells a user their code may
	// have been captured by someone else. The poll now returns
	// ErrSessionConsumed, and the test pins that (AUDIT_REMEDIATION.md F-27).
	pollResConsumed, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if !errors.Is(err, cliauth.ErrSessionConsumed) {
		t.Fatalf("expected ErrSessionConsumed on replay, got %v", err)
	}
	if pollResConsumed == nil || pollResConsumed.Status != cliauth.StatusConsumed {
		t.Fatalf("expected consumed status on replay, got %+v", pollResConsumed)
	}
	if pollResConsumed.AccessToken != "" || pollResConsumed.RefreshToken != "" {
		t.Fatal("consumed session must not yield tokens on subsequent polls")
	}
}

func TestCLIDeviceFlowExpiration(t *testing.T) {
	ctx := context.Background()
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	initRes, err := manager.InitiateDeviceLogin(ctx, "ScanDrix-CLI/v1.0")
	if err != nil {
		t.Fatalf("failed initiating: %v", err)
	}

	// Manually expire session
	sess, err := store.GetByDeviceCode(ctx, initRes.DeviceCode)
	if err != nil {
		t.Fatalf("failed getting session: %v", err)
	}
	sess.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	_ = store.CreateSession(ctx, sess)

	// Poll should report expired
	pollRes, _ := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if pollRes.Status != cliauth.StatusExpired {
		t.Fatalf("expected expired status, got %s", pollRes.Status)
	}
}

func TestCLIDeviceFlowSlowDown(t *testing.T) {
	ctx := context.Background()
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	initRes, err := manager.InitiateDeviceLogin(ctx, "ScanDrix-CLI/v1.0")
	if err != nil {
		t.Fatalf("failed initiating: %v", err)
	}

	// First poll should succeed with pending
	res1, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if err != nil {
		t.Fatalf("unexpected error on first poll: %v", err)
	}
	if res1.Status != cliauth.StatusPending {
		t.Fatalf("expected pending, got %s", res1.Status)
	}

	// Second poll immediately after (<5s) should return slow_down per RFC 8628 §3.5
	res2, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if err != cliauth.ErrSlowDown {
		t.Fatalf("expected ErrSlowDown, got: %v", err)
	}
	if res2.Status != "slow_down" {
		t.Fatalf("expected status slow_down, got %s", res2.Status)
	}
	if res2.Interval <= initRes.Interval {
		t.Fatalf("expected interval to increase, got %d (initial %d)", res2.Interval, initRes.Interval)
	}
}

func TestCLIDeviceFlowBruteForceProtection(t *testing.T) {
	ctx := context.Background()
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	fakeProfile := &models.AccountProfile{
		ID:    uuid.New(),
		Email: "test@example.com",
	}

	userCode := "TEST-CODE"
	// Attempt 5 invalid complete attempts
	for i := 1; i <= 5; i++ {
		err := manager.CompleteDeviceLogin(ctx, userCode, "access", "refresh", fakeProfile)
		if err != cliauth.ErrSessionNotFound {
			t.Fatalf("attempt %d: expected ErrSessionNotFound, got %v", i, err)
		}
	}

	// 6th attempt should be blocked with ErrTooManyAttempts
	err := manager.CompleteDeviceLogin(ctx, userCode, "access", "refresh", fakeProfile)
	if err != cliauth.ErrTooManyAttempts {
		t.Fatalf("expected ErrTooManyAttempts, got: %v", err)
	}
}

func TestCLIDeviceFlowBruteForceDifferentCodes(t *testing.T) {
	ctx := context.Background()
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	fakeProfile := &models.AccountProfile{
		ID:    uuid.New(),
		Email: "attacker@example.com",
	}

	// Try 5 different codes - should still lock out by user identity
	for i := 1; i <= 5; i++ {
		code := fmt.Sprintf("CODE-%04d", i)
		err := manager.CompleteDeviceLogin(ctx, code, "access", "refresh", fakeProfile)
		if err != cliauth.ErrSessionNotFound {
			t.Fatalf("attempt %d: expected ErrSessionNotFound, got %v", i, err)
		}
	}

	// 6th attempt with yet another code must be blocked by user lockout
	err := manager.CompleteDeviceLogin(ctx, "CODE-9999", "access", "refresh", fakeProfile)
	if err != cliauth.ErrTooManyAttempts {
		t.Fatalf("expected ErrTooManyAttempts when scanning different codes, got: %v", err)
	}
}

func TestCLIDeviceFlowBruteForceClientIPLockout(t *testing.T) {
	store := cliauth.NewInMemorySessionStore()
	manager := cliauth.NewDeviceFlowManager(store, "https://app.scandrix.dev")

	ctx := cliauth.WithClientIP(context.Background(), "198.51.100.24")
	// Try 5 different codes with anonymous/changing profiles from the same client IP
	for i := 1; i <= 5; i++ {
		profile := &models.AccountProfile{
			ID:    uuid.New(),
			Email: fmt.Sprintf("random-%d@example.com", i),
		}
		code := fmt.Sprintf("SCAN-%04d", i)
		err := manager.CompleteDeviceLogin(ctx, code, "access", "refresh", profile)
		if err != cliauth.ErrSessionNotFound {
			t.Fatalf("attempt %d: expected ErrSessionNotFound, got %v", i, err)
		}
	}

	// 6th attempt from same client IP must be locked out even with a new profile and code
	newProfile := &models.AccountProfile{ID: uuid.New(), Email: "fresh@example.com"}
	err := manager.CompleteDeviceLogin(ctx, "SCAN-9999", "access", "refresh", newProfile)
	if err != cliauth.ErrTooManyAttempts {
		t.Fatalf("expected ErrTooManyAttempts on client IP lockout, got %v", err)
	}
}
