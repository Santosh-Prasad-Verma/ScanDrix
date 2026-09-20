// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	"github.com/scandrix/backend/internal/cli/utils"
)

func TestAuthServiceResolution(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	client := api.NewClient("http://localhost:8080", "", "")
	svc := auth.NewService(client)

	// Initially not authenticated
	os.Unsetenv("SCANDRIX_TOKEN")
	os.Unsetenv("SCANDRIX_ACCESS_TOKEN")
	os.Unsetenv("SCANDRIX_TEAM_KEY")
	os.Unsetenv("SCANDRIX_API_KEY")

	if svc.IsAuthenticated() {
		t.Fatalf("expected unauthenticated initially")
	}

	// Authenticated via environment variable
	os.Setenv("SCANDRIX_API_KEY", "scandrix_team_testkey")
	if !svc.IsAuthenticated() {
		t.Fatalf("expected authenticated via SCANDRIX_API_KEY")
	}
	os.Unsetenv("SCANDRIX_API_KEY")

	// Authenticated via stored credentials
	creds := &utils.StoredCredentials{
		AccessToken: "jwt-test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour).UnixMilli(),
		User: &utils.UserProfile{
			Email: "tester@scandrix.dev",
		},
	}
	if err := utils.SaveCredentials(creds); err != nil {
		t.Fatalf("save credentials error: %v", err)
	}

	svc2 := auth.NewService(client)
	if !svc2.IsAuthenticated() {
		t.Fatalf("expected authenticated via stored credentials")
	}

	loadedCreds, err := svc2.GetCredentials()
	if err != nil || loadedCreds.AccessToken != "jwt-test-token" {
		t.Fatalf("failed retrieving credentials: %v", err)
	}

	// Logout
	if err := svc2.Logout(context.Background()); err != nil {
		t.Fatalf("logout error: %v", err)
	}

	svc3 := auth.NewService(client)
	if svc3.IsAuthenticated() {
		t.Fatalf("expected unauthenticated after logout")
	}
}

func TestLoginViaBrowser(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/auth/login-init":
			var req struct {
				Port int `json:"port"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state":            "state-xyz-123",
				"verification_uri": fmt.Sprintf("http://127.0.0.1:%d/callback?state=state-xyz-123", req.Port),
				"expires_in":       10,
			})
		case "/cli/auth/login-poll":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":       cliauth.StatusCompleted,
				"access_token": "jwt-loopback-approved",
				"user_email":   "developer@scandrix.io",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	client := api.NewClient(mockServer.URL, "", "")
	svc := auth.NewService(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := svc.LoginViaBrowser(ctx, func(verifyURL string) {
		// Simulate the browser opening and hitting the loopback callback endpoint
		go func() {
			time.Sleep(50 * time.Millisecond)
			resp, err := http.Get(verifyURL)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	})

	if err != nil {
		t.Fatalf("LoginViaBrowser failed: %v", err)
	}

	if user == nil || user.Email != "developer@scandrix.io" {
		t.Fatalf("unexpected user profile: %+v", user)
	}

	creds, err := svc.GetCredentials()
	if err != nil || creds.AccessToken != "jwt-loopback-approved" {
		t.Fatalf("unexpected stored credentials: %+v (err: %v)", creds, err)
	}
}
