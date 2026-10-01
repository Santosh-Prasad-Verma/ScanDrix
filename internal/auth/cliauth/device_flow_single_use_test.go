package cliauth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestDeviceFlowConcurrentPollRedeemsOnce pins AUDIT_REMEDIATION.md F-27.
//
// The poll path used to be:
//
//	sess, _ := GetByDeviceCode(ctx, deviceCode)   // sees "completed"
//	_ = MarkConsumed(ctx, sess.UUID)              // error discarded
//	return sess.AccessToken, sess.RefreshToken    // tokens handed over
//
// The read and the write were separate steps, so two concurrent polls both
// observed StatusCompleted and both were given a full token pair. A one-time
// authorization could therefore be redeemed twice.
func TestDeviceFlowConcurrentPollRedeemsOnce(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	mgr := NewDeviceFlowManager(store, "https://app.example.test")

	const deviceCode = "device-code-race"
	const userCode = "USER-CODE"

	wsID := uuid.New()
	_ = store.CreateSession(ctx, &CLIDeviceSession{
		UUID:       uuid.New(),
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Mode:       "cli",
		Status:     StatusPending,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err := store.CompleteSession(ctx, userCode, "access-token-2", "refresh-token-2", wsID, uuid.New(), "user@example.test"); err != nil {
		t.Fatalf("complete session: %v", err)
	}

	// Two pollers race for the same one-time code.
	type outcome struct {
		gotToken bool
		err      error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := mgr.PollDeviceLogin(ctx, deviceCode)
			results <- outcome{
				gotToken: res != nil && res.AccessToken != "",
				err:      err,
			}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winners, losers := 0, 0
	for r := range results {
		if r.gotToken {
			winners++
		} else {
			losers++
		}
	}

	if winners != 1 {
		t.Fatalf("exactly one poll must receive the tokens, got %d (losers: %d)", winners, losers)
	}
	if losers != 1 {
		t.Fatalf("the losing poll must receive no tokens, got %d", losers)
	}
}

// TestDeviceFlowSequentialPollIsSingleUse proves the same property without a
// race, which is the shape an attacker can trigger without concurrency.
func TestDeviceFlowSequentialPollIsSingleUse(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	mgr := NewDeviceFlowManager(store, "https://app.example.test")

	const deviceCode = "device-code-seq"
	const userCode = "SEQ-CODE"

	wsID := uuid.New()
	_ = store.CreateSession(ctx, &CLIDeviceSession{
		UUID:       uuid.New(),
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Mode:       "cli",
		Status:     StatusPending,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err := store.CompleteSession(ctx, userCode, "access-token-2", "refresh-token-2", wsID, uuid.New(), "user@example.test"); err != nil {
		t.Fatalf("complete session: %v", err)
	}

	first, err := mgr.PollDeviceLogin(ctx, deviceCode)
	if err != nil {
		t.Fatalf("first poll should succeed: %v", err)
	}
	if first.AccessToken != "access-token-2" || first.RefreshToken != "refresh-token-2" {
		t.Fatalf("first poll must return the tokens, got %+v", first)
	}

	second, err := mgr.PollDeviceLogin(ctx, deviceCode)
	if err == nil {
		t.Fatalf("second poll must be refused, got status %q with token %q",
			second.Status, second.AccessToken)
	}
	if second != nil && second.AccessToken != "" {
		t.Fatal("the second poll must never return tokens")
	}
}

// TestConsumeAndGetSessionIsAtomicAtTheStore covers the store contract itself,
// independent of the manager, including the pending case.
func TestConsumeAndGetSessionIsAtomicAtTheStore(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	pending := &CLIDeviceSession{
		UUID:       uuid.New(),
		DeviceCode: "pending-code",
		Mode:       "cli",
		Status:     StatusPending,
		ExpiresAt:  time.Now().UTC().Add(time.Minute),
	}
	_ = store.CreateSession(ctx, pending)

	// A pending session must be reported as pending, never claimed.
	if _, err := store.ConsumeAndGetSession(ctx, "pending-code"); err != ErrSessionPending {
		t.Fatalf("expected ErrSessionPending for a pending session, got %v", err)
	}

	// Unknown device code.
	if _, err := store.ConsumeAndGetSession(ctx, "nope"); err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}

	completed := &CLIDeviceSession{
		UUID:         uuid.New(),
		DeviceCode:   "done-code",
		Mode:         "cli",
		Status:       StatusCompleted,
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Now().UTC().Add(time.Minute),
	}
	_ = store.CreateSession(ctx, completed)

	claimed, err := store.ConsumeAndGetSession(ctx, "done-code")
	if err != nil {
		t.Fatalf("first claim should succeed: %v", err)
	}
	if claimed.AccessToken != "a" {
		t.Fatalf("claim must return the tokens, got %q", claimed.AccessToken)
	}
	// The returned copy must not alias stored state.
	claimed.AccessToken = "mutated"
	if again, err := store.ConsumeAndGetSession(ctx, "done-code"); again != nil || err != ErrSessionConsumed {
		t.Fatalf("second claim must be refused, got session=%v err=%v", again, err)
	}

	if _, err := store.ConsumeAndGetSession(ctx, "done-code"); err != ErrSessionConsumed {
		t.Fatalf("expected ErrSessionConsumed on replay, got %v", err)
	}
}
