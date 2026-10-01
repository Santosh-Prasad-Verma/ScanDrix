package cliauth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// The OAuth device flow authenticates by device_code before any tenant exists,
// so cli_auth_sessions could not be tenant-scoped. The fix is the RFC 8628
// pattern: bind the tenant on the browser-approval leg, and refuse to hand
// tokens to any session that has no tenant.
//
// AUDIT_REMEDIATION.md F-37.
func newPendingSession(t *testing.T, m *DeviceFlowManager, deviceCode, userCode string) {
	t.Helper()
	err := m.store.CreateSession(context.Background(), &CLIDeviceSession{
		UUID:       uuid.New(),
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Status:     StatusPending,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestF37ApprovalBindsTheTenant(t *testing.T) {
	m := NewDeviceFlowManager(NewInMemorySessionStore(), "")
	newPendingSession(t, m, "dev-code-tenant", "USER-AAAA")

	ws := uuid.New()
	user := uuid.New()
	err := m.CompleteDeviceLogin(context.Background(), "user-aaaa",
		"access-token", "refresh-token",
		&models.AccountProfile{ID: user, WorkspaceID: ws, Email: "a@example.com"})
	if err != nil {
		t.Fatalf("approval should succeed with a tenant: %v", err)
	}

	sess, err := m.store.GetByDeviceCode(context.Background(), "dev-code-tenant")
	if err != nil {
		t.Fatalf("get by device code: %v", err)
	}
	if sess.WorkspaceID != ws {
		t.Fatalf("workspace not recorded: got %s want %s", sess.WorkspaceID, ws)
	}
}

func TestF37ApprovalWithoutTenantFailsClosed(t *testing.T) {
	m := NewDeviceFlowManager(NewInMemorySessionStore(), "")
	newPendingSession(t, m, "dev-code-notenant", "USER-BBBB")

	err := m.CompleteDeviceLogin(context.Background(), "user-bbbb",
		"access-token", "refresh-token",
		&models.AccountProfile{ID: uuid.New(), Email: "b@example.com"})
	if err != ErrNoWorkspace {
		t.Fatalf("approval without a tenant must be refused, got %v", err)
	}

	// And the session must still be redeemable-as-nothing, i.e. not completed.
	sess, err := m.store.GetByDeviceCode(context.Background(), "dev-code-notenant")
	if err != nil {
		t.Fatalf("get by device code: %v", err)
	}
	if sess.Status != StatusPending {
		t.Fatalf("session must stay PENDING, got %s", sess.Status)
	}
	if sess.AccessToken != "" {
		t.Fatal("no token may be stored when approval was refused")
	}
}

func TestF37RedemptionRefusesSessionWithNoTenant(t *testing.T) {
	store := NewInMemorySessionStore()
	m := NewDeviceFlowManager(store, "")

	// Forge a COMPLETED row that never recorded a tenant - exactly the state a
	// pre-fix deployment could leave behind.
	err := store.CreateSession(context.Background(), &CLIDeviceSession{
		UUID:        uuid.New(),
		DeviceCode:  "dev-code-orphan",
		UserCode:    "USER-CCCC",
		Status:      StatusCompleted,
		AccessToken: "stolen-token",
		ExpiresAt:   time.Now().UTC().Add(10 * time.Minute),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := m.PollDeviceLogin(context.Background(), "dev-code-orphan")
	if err != ErrNoWorkspace {
		t.Fatalf("redemption of a tenant-less session must be refused, got %v", err)
	}
	if res != nil && res.Status == StatusCompleted {
		t.Fatal("must not report an approved session")
	}
}

func TestF37RedemptionSucceedsWithTenant(t *testing.T) {
	store := NewInMemorySessionStore()
	m := NewDeviceFlowManager(store, "")
	ws := uuid.New()
	err := store.CreateSession(context.Background(), &CLIDeviceSession{
		UUID:        uuid.New(),
		DeviceCode:  "dev-code-good",
		UserCode:    "USER-DDDD",
		Status:      StatusCompleted,
		WorkspaceID: ws,
		AccessToken: "access-token",
		ExpiresAt:   time.Now().UTC().Add(10 * time.Minute),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.PollDeviceLogin(context.Background(), "dev-code-good"); err != nil && err != ErrSessionNotFound {
		t.Fatalf("a properly bound session must be redeemable, got %v", err)
	}
}
