package cliauth_test

import (
	"context"
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

	// 5. Re-poll consumed session -> replay defense, status consumed with NO tokens
	pollResConsumed, err := manager.PollDeviceLogin(ctx, initRes.DeviceCode)
	if err != nil {
		t.Fatalf("poll consumed failed: %v", err)
	}
	if pollResConsumed.Status != cliauth.StatusConsumed {
		t.Fatalf("expected consumed status on replay, got %s", pollResConsumed.Status)
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
