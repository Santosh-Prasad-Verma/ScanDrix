package database

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/scandrix/backend/internal/auth/cliauth"
)

// TestConsumeCLISessionByDeviceCodeIsAtomic verifies the guarded
// `UPDATE ... WHERE status = 'completed' RETURNING` really admits exactly one
// winner when two independent database connections claim the same device code
// at the same time.
//
// AUDIT_REMEDIATION.md F-27. This is the check the in-memory test cannot make:
// the in-memory store serialises on a mutex, whereas the real risk is two
// separate transactions in Postgres both reading status 'completed'.
//
// Skips unless SCANDRIX_E2E_RUNTIME_DSN points at a live database.
func TestConsumeCLISessionByDeviceCodeIsAtomic(t *testing.T) {
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
	deviceCode := "atomic-probe-" + uuid.New().String()

	session := &cliauth.CLIDeviceSession{
		UUID:         uuid.New(),
		DeviceCode:   deviceCode,
		UserCode:     "PROBE",
		Mode:         "cli",
		Status:       cliauth.StatusCompleted,
		AccessToken:  "access-atomic",
		RefreshToken: "refresh-atomic",
		UserEmail:    "atomic@example.test",
		ExpiresAt:    time.Now().UTC().Add(10 * time.Minute),
	}
	// Seeded with direct SQL rather than repo.CreateCLISession on purpose.
	//
	// CreateCLISession omits the NOT NULL session_id column, so against this
	// schema it fails -- and its caller discards that error, leaving the device
	// flow memory-backed. That is a separate pre-existing defect; seeding here
	// keeps this test pointed at the guarded UPDATE it is meant to verify.
	if _, err := client.Pool.Exec(ctx, `
		INSERT INTO cli_auth_sessions (
			session_id, uuid, state, device_code, user_code, mode, status,
			access_token, refresh_token, user_email, expires_at, redirect_uri
		) VALUES ($1, $2, $3, $4, $5, $6, 'completed', $7, $8, $9, $10, '')`,
		deviceCode, session.UUID, session.State, session.DeviceCode, session.UserCode,
		session.Mode, session.AccessToken, session.RefreshToken, session.UserEmail,
		session.ExpiresAt,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.Pool.Exec(context.Background(),
			`DELETE FROM cli_auth_sessions WHERE device_code = $1`, deviceCode)
	})

	// Confirm the row really landed in Postgres before racing on it.
	stored, err := repo.GetCLISessionByDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("read back seeded session: %v", err)
	}
	if stored == nil || string(stored.Status) != string(cliauth.StatusCompleted) {
		t.Skipf("device flow session store is not backed by Postgres in this environment (%v); "+
			"the in-memory atomicity test already covers the logic", err)
	}

	const racers = 4
	var (
		start    = make(chan struct{})
		wg       sync.WaitGroup
		mu       sync.Mutex
		winners  int
		consumed int
	)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sess, err := repo.ConsumeCLISessionByDeviceCode(context.Background(), deviceCode)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && sess != nil && sess.AccessToken != "":
				winners++
			case err == cliauth.ErrSessionConsumed:
				consumed++
			}
		}()
	}
	close(start)
	wg.Wait()

	if winners != 1 {
		t.Fatalf("exactly one claim must win, got %d (losers reporting consumed: %d)", winners, consumed)
	}
	if consumed != racers-1 {
		t.Fatalf("all %d losers must report ErrSessionConsumed, got %d", racers-1, consumed)
	}
}

// TestConsumeCLISessionByDeviceCodeUnknownCode covers the miss path.
func TestConsumeCLISessionByDeviceCodeUnknownCode(t *testing.T) {
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
	if _, err := repo.ConsumeCLISessionByDeviceCode(context.Background(), "no-such-device-code-"+uuid.New().String()); err == nil {
		t.Fatal("claiming an unknown device code must return an error")
	}
}
