package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/auth/cliauth"
)

// TestCLISessionPersistsCreateReadComplete walks the device-code lifecycle
// against a real PostgreSQL database: create -> read back by device code ->
// complete with tokens -> read back again.
//
// AUDIT_REMEDIATION.md F-27. This exists because every earlier check of this
// path was satisfied by the in-memory fallback. On a single process the
// fallback made the flow look healthy while the durable write was failing on
// a NOT NULL violation, and the error was discarded.
//
// Skips unless SCANDRIX_E2E_RUNTIME_DSN points at a live database.
func TestCLISessionPersistsCreateReadComplete(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}

	client, err := NewClient(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	repo := NewRepository(client)
	ctx := context.Background()
	deviceCode := "lifecycle-probe-" + uuid.New().String()
	// Use the real generator rather than a hand-rolled code: the production
	// alphabet is uppercase-only, and CompleteCLISession upper-cases its
	// argument before matching. A mixed-case fixture would silently fail to
	// match and look like a persistence bug.
	userCode, err := cliauth.GenerateUnbiasedUserCode()
	if err != nil {
		t.Fatalf("GenerateUnbiasedUserCode: %v", err)
	}

	session := &cliauth.CLIDeviceSession{
		UUID:        uuid.New(),
		State:       "state-" + uuid.New().String(),
		DeviceCode:  deviceCode,
		UserCode:    userCode,
		RedirectURI: "http://localhost:1455/callback",
		Mode:        "cli",
		Status:      cliauth.StatusPending,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 1. Create must actually write. This is the assertion that fails when
	//    session_id is omitted from the INSERT.
	if err := repo.CreateCLISession(ctx, session); err != nil {
		t.Fatalf("CreateCLISession: %v", err)
	}

	// 2. The row must be visible on a fresh read path, not just in memory.
	got, err := repo.GetCLISessionByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetCLISessionByDeviceCode: %v", err)
	}
	if got == nil {
		t.Fatal("session was not readable back from the database")
	}
	if got.DeviceCode != deviceCode {
		t.Errorf("device_code = %q, want %q", got.DeviceCode, deviceCode)
	}
	if got.RedirectURI != session.RedirectURI {
		t.Errorf("redirect_uri = %q, want %q", got.RedirectURI, session.RedirectURI)
	}
	if got.Status != cliauth.StatusPending {
		t.Errorf("status = %q, want %q", got.Status, cliauth.StatusPending)
	}

	// 3. Complete must move it to completed and persist the tokens.
	userID := uuid.New()
	if err := repo.CompleteCLISession(ctx, userCode, "access-probe", "refresh-probe", userID, uuid.New(), "probe@example.com"); err != nil {
		t.Fatalf("CompleteCLISession: %v", err)
	}

	done, err := repo.GetCLISessionByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("read back after complete: %v", err)
	}
	if done.Status != cliauth.StatusCompleted {
		t.Errorf("status after complete = %q, want %q", done.Status, cliauth.StatusCompleted)
	}
	if done.AccessToken != "access-probe" {
		t.Errorf("access_token = %q, want %q", done.AccessToken, "access-probe")
	}
	if done.UserID == nil || *done.UserID != userID {
		t.Errorf("user_id not persisted correctly: %v", done.UserID)
	}
}

// TestGetCLISessionHandlesNullRedirect covers a session created without a
// redirect_uri. redirect_uri is nullable, and scanning it into a plain string
// fails on NULL, which made every such lookup error out.
//
// AUDIT_REMEDIATION.md F-27.
func TestGetCLISessionHandlesNullRedirect(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}

	client, err := NewClient(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	repo := NewRepository(client)
	ctx := context.Background()
	deviceCode := "nullredirect-probe-" + uuid.New().String()
	nullRedirectUserCode, err := cliauth.GenerateUnbiasedUserCode()
	if err != nil {
		t.Fatalf("GenerateUnbiasedUserCode: %v", err)
	}

	session := &cliauth.CLIDeviceSession{
		UUID:       uuid.New(),
		State:      "state-" + uuid.New().String(),
		DeviceCode: deviceCode,
		UserCode:   nullRedirectUserCode,
		Mode:       "cli",
		Status:     cliauth.StatusPending,
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := repo.CreateCLISession(ctx, session); err != nil {
		t.Fatalf("CreateCLISession: %v", err)
	}

	got, err := repo.GetCLISessionByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetCLISessionByDeviceCode with NULL redirect_uri: %v", err)
	}
	if got.RedirectURI != "" {
		t.Errorf("redirect_uri = %q, want empty", got.RedirectURI)
	}
}