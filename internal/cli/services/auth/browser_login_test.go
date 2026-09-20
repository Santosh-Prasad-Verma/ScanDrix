// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserLoginService_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.json")

	svc := NewBrowserLoginService("https://scandrix.dev")
	svc.SetTokenStorage(credPath)

	svc.SetOpenBrowser(func(targetURL string) error {
		// Asynchronously simulate user approving in browser
		go func() {
			time.Sleep(50 * time.Millisecond)
			u, err := url.Parse(targetURL)
			if err != nil {
				return
			}
			port := u.Query().Get("port")
			state := u.Query().Get("state")

			callbackURL := fmt.Sprintf("http://127.0.0.1:%s/callback?state=%s&token=scandrix_jwt_mock_token_123&email=dev@scandrix.dev", port, state)
			resp, err := http.Get(callbackURL)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	creds, err := svc.Login(ctx)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if creds.Token != "scandrix_jwt_mock_token_123" || creds.Email != "dev@scandrix.dev" {
		t.Errorf("unexpected credentials returned: %+v", creds)
	}

	// Verify saved credentials
	loaded, err := svc.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	if loaded.Token != creds.Token || loaded.Email != creds.Email {
		t.Errorf("loaded credentials do not match saved: %+v", loaded)
	}

	// Verify logout
	if err := svc.Logout(); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Errorf("credentials file still exists after logout")
	}
}

func TestBrowserLoginService_StateMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.json")

	svc := NewBrowserLoginService("https://scandrix.dev")
	svc.SetTokenStorage(credPath)

	svc.SetOpenBrowser(func(targetURL string) error {
		go func() {
			time.Sleep(50 * time.Millisecond)
			u, _ := url.Parse(targetURL)
			port := u.Query().Get("port")

			// Send wrong state
			callbackURL := fmt.Sprintf("http://127.0.0.1:%s/callback?state=wrong_state&token=xyz", port)
			resp, err := http.Get(callbackURL)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := svc.Login(ctx)
	if err == nil {
		t.Fatalf("expected error on state mismatch, got nil")
	}
}
