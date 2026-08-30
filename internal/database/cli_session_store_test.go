package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/database"
)

func TestPostgresCLISessionStoreAdapter(t *testing.T) {
	ctx := context.Background()
	repo := database.NewRepository(nil)
	store := database.NewPostgresCLISessionStore(repo)

	// Verify interface conformance
	var _ cliauth.SessionStore = store

	session := &cliauth.CLIDeviceSession{
		UUID:        uuid.New(),
		State:       "test-state",
		DeviceCode:  "dc-12345",
		UserCode:    "ABCD-EFGH",
		Status:      cliauth.StatusPending,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 1. Create session (nil client guard returns nil)
	err := store.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("expected nil error on nil db client, got %v", err)
	}

	// 2. GetByDeviceCode (nil client guard returns error)
	_, err = store.GetByDeviceCode(ctx, "dc-12345")
	if err == nil {
		t.Fatal("expected error on nil client for GetByDeviceCode")
	}

	// 3. GetByUserCode (nil client guard returns error)
	_, err = store.GetByUserCode(ctx, "ABCD-EFGH")
	if err == nil {
		t.Fatal("expected error on nil client for GetByUserCode")
	}

	// 4. CompleteSession (nil client guard returns nil)
	err = store.CompleteSession(ctx, "ABCD-EFGH", "access-token", "refresh-token", uuid.New(), "user@example.com")
	if err != nil {
		t.Fatalf("expected nil error on nil db client, got %v", err)
	}

	// 5. MarkConsumed (nil client guard returns nil)
	err = store.MarkConsumed(ctx, session.UUID)
	if err != nil {
		t.Fatalf("expected nil error on nil db client, got %v", err)
	}
}
