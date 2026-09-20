// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package sso_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/sso"
)

func TestSSOTestSession_Lifecycle(t *testing.T) {
	samlHandler := sso.NewSAMLHandler()
	workbench := sso.NewSSOTestSessionWorkbench(samlHandler, nil)
	ctx := context.Background()
	wsID := uuid.New()

	// 1. Create test session
	domains := []string{"acme.corp", "subsidiary.acme.corp"}
	session, err := workbench.CreateSession(ctx, wsID, sso.ProviderTypeSAML2, "https://idp.acme.corp/sso", domains, "admin@acme.corp")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.Status != sso.TestSessionStatusPending {
		t.Fatalf("Expected status PENDING, got %s", session.Status)
	}
	if session.ConfigFingerprint == "" {
		t.Fatal("Expected non-empty config fingerprint")
	}

	// 2. Fetch active session
	retrieved, err := workbench.GetSession(ctx, session.SessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.SessionID != session.SessionID {
		t.Fatalf("Session ID mismatch: %s vs %s", retrieved.SessionID, session.SessionID)
	}

	// 3. Mark Failed explicitly
	failed, err := workbench.MarkFailed(session.SessionID, sso.ErrCodeSignatureFailed, "Signature verification failed: expired certificate")
	if err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}
	if failed.Status != sso.TestSessionStatusFailed {
		t.Fatalf("Expected status FAILED, got %s", failed.Status)
	}
	if failed.FailureCode != sso.ErrCodeSignatureFailed {
		t.Fatalf("Expected failure code %s, got %s", sso.ErrCodeSignatureFailed, failed.FailureCode)
	}
}

func TestSSOTestSession_MissingOrExpired(t *testing.T) {
	workbench := sso.NewSSOTestSessionWorkbench(nil, nil)
	ctx := context.Background()

	// Non-existent session
	if _, err := workbench.GetSession(ctx, "non-existent-id"); !errors.Is(err, sso.ErrTestSessionNotFound) {
		t.Fatalf("Expected ErrTestSessionNotFound, got %v", err)
	}
}

func TestSSOTestSession_CleanupExpired(t *testing.T) {
	workbench := sso.NewSSOTestSessionWorkbench(nil, nil)
	ctx := context.Background()
	wsID := uuid.New()

	session, err := workbench.CreateSession(ctx, wsID, sso.ProviderTypeSAML2, "https://test.com", []string{"test.com"}, "admin@test.com")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Artificially expire the session
	session.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)

	purged := workbench.CleanupExpired()
	if purged != 1 {
		t.Fatalf("Expected 1 purged session, got %d", purged)
	}

	// Confirm retrieval fails
	if _, err := workbench.GetSession(ctx, session.SessionID); !errors.Is(err, sso.ErrTestSessionNotFound) {
		t.Fatalf("Expected ErrTestSessionNotFound after cleanup, got %v", err)
	}
}

func TestSSOTestSession_Concurrency(t *testing.T) {
	workbench := sso.NewSSOTestSessionWorkbench(nil, nil)
	ctx := context.Background()
	wsID := uuid.New()

	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sess, err := workbench.CreateSession(ctx, wsID, sso.ProviderTypeSAML2, "https://idp.org/sso", []string{"idp.org"}, "admin@idp.org")
			if err != nil {
				t.Errorf("Worker %d create session failed: %v", id, err)
				return
			}
			_, _ = workbench.GetSession(ctx, sess.SessionID)
			_, _ = workbench.MarkFailed(sess.SessionID, sso.ErrCodeInvalidAssertion, "test error")
		}(i)
	}

	wg.Wait()
}
