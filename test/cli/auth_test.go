// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	"github.com/scandrix/backend/internal/cli/utils"
)

// AUTHENTICATION & DEVICE FLOW TEST SUITE

func setupTempHome(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	// These override any stored session token and take precedence over it
	// (see Service.getEnvAuthToken). A developer with a real key exported in
	// their shell or .env would otherwise silently break these tests, so the
	// suite clears them and asserts the real credential flow instead.
	clearAuthEnvOverrides(t)
	return tempDir
}

// clearAuthEnvOverrides removes the env-based auth overrides for the duration of
// the test. t.Setenv restores the previous values automatically.
func clearAuthEnvOverrides(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SCANDRIX_TOKEN",
		"SCANDRIX_ACCESS_TOKEN",
		"SCANDRIX_TEAM_KEY",
		"SCANDRIX_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

func TestAuthService_PasswordLogin(t *testing.T) {
	setupTempHome(t)

	// Mock backend server mimicking /api/v1/auth/login
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" && r.Method == http.MethodPost {
			var req api.LoginRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Email == "test@scandrix.dev" && req.Password == "correct-pass" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(api.AuthResponse{
					AccessToken:  "mock-access-token-123",
					RefreshToken: "mock-refresh-token-456",
					ExpiresIn:    3600,
					User: &utils.UserProfile{
						ID:    "usr-uuid-1",
						Email: "test@scandrix.dev",
						Role:  "admin",
					},
				})
				return
			}
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "", "")
	svc := auth.NewService(client)

	// Test 1: Successful Login
	user, err := svc.Login(context.Background(), "test@scandrix.dev", "correct-pass")
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if user.Email != "test@scandrix.dev" {
		t.Errorf("expected email test@scandrix.dev, got %s", user.Email)
	}

	// Verify credentials persisted to disk
	creds, err := svc.GetCredentials()
	if err != nil || creds == nil {
		t.Fatalf("failed retrieving saved credentials: %v", err)
	}
	if creds.AccessToken != "mock-access-token-123" {
		t.Errorf("expected token mock-access-token-123, got %s", creds.AccessToken)
	}

	// Test 2: Invalid Password
	_, err = svc.Login(context.Background(), "test@scandrix.dev", "wrong-pass")
	if err == nil {
		t.Fatalf("expected error on invalid password, got nil")
	}
}

func TestAuthService_DeviceAuthorizationFlow(t *testing.T) {
	setupTempHome(t)

	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/auth/cli/device/initiate":
			_ = json.NewEncoder(w).Encode(cliauth.DeviceLoginInitiateResult{
				DeviceCode:              "dev-code-xyz",
				UserCode:                "WDXZ-8899",
				VerificationURI:         "http://localhost/activate",
				VerificationURIComplete: "http://localhost/activate?code=WDXZ-8899",
				ExpiresIn:               600,
				Interval:                1,
			})
		case "/api/v1/auth/cli/device/poll":
			pollCount++
			if pollCount < 2 {
				// Pending first
				_ = json.NewEncoder(w).Encode(cliauth.DeviceLoginPollResult{
					Status: cliauth.StatusPending,
				})
			} else {
				// Completed on second poll
				_ = json.NewEncoder(w).Encode(cliauth.DeviceLoginPollResult{
					Status:       cliauth.StatusCompleted,
					AccessToken:  "jwt-device-access-token",
					RefreshToken: "jwt-device-refresh-token",
					UserEmail:    "developer@scandrix.dev",
				})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "", "")
	svc := auth.NewService(client)

	// Step 1: Initiate Device Flow
	initRes, err := svc.StartDeviceFlow(context.Background())
	if err != nil {
		t.Fatalf("failed initiating device flow: %v", err)
	}
	if initRes.UserCode != "WDXZ-8899" {
		t.Errorf("expected user code WDXZ-8899, got %s", initRes.UserCode)
	}

	// Step 2: Poll 1 (Pending)
	poll1, err := svc.PollDeviceFlow(context.Background(), initRes.DeviceCode)
	if err != nil {
		t.Fatalf("failed polling device: %v", err)
	}
	if poll1.Status != cliauth.StatusPending {
		t.Errorf("expected status pending, got %s", poll1.Status)
	}

	// Step 3: Poll 2 (Completed)
	poll2, err := svc.PollDeviceFlow(context.Background(), initRes.DeviceCode)
	if err != nil {
		t.Fatalf("failed polling device on second turn: %v", err)
	}
	if poll2.Status != cliauth.StatusCompleted {
		t.Errorf("expected status completed, got %s", poll2.Status)
	}

	// Step 4: Save Tokens
	profile, err := svc.SaveSessionTokens(poll2.AccessToken, poll2.RefreshToken, poll2.UserEmail, 3600)
	if err != nil {
		t.Fatalf("failed saving session tokens: %v", err)
	}
	if profile.Email != "developer@scandrix.dev" {
		t.Errorf("expected email developer@scandrix.dev, got %s", profile.Email)
	}

	// Verify valid token returned
	token, err := svc.GetValidToken(context.Background())
	if err != nil || token != "jwt-device-access-token" {
		t.Errorf("expected token jwt-device-access-token, got %s (err: %v)", token, err)
	}
}

func TestAuthService_TeamAPIKey(t *testing.T) {
	setupTempHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/cli/validate-key" {
			key := r.Header.Get("X-Team-Key")
			w.Header().Set("Content-Type", "application/json")
			if key == "scandrix_live_valid_key_999" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"valid": true,
					"team": map[string]string{
						"id":   "team-1",
						"name": "Acme Security Team",
					},
					"organization": map[string]string{
						"id":   "org-1",
						"name": "Acme Corp",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"valid": false,
				"error": "team key is invalid or revoked",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "", "")
	svc := auth.NewService(client)

	// Valid Team Key
	orgName, err := svc.SetTeamKey(context.Background(), "scandrix_live_valid_key_999")
	if err != nil {
		t.Fatalf("failed setting valid team key: %v", err)
	}
	if orgName != "Acme Security Team" {
		t.Errorf("expected organization Acme Security Team, got %s", orgName)
	}

	// Invalid Team Key
	_, err = svc.SetTeamKey(context.Background(), "invalid_key")
	if err == nil {
		t.Fatalf("expected error for invalid team key, got nil")
	}
}

func TestAuthService_AutomaticTokenRotation(t *testing.T) {
	setupTempHome(t)

	refreshHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" && r.Method == http.MethodPost {
			refreshHit = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.AuthResponse{
				AccessToken:  "refreshed-access-token-999",
				RefreshToken: "refreshed-refresh-token-999",
				ExpiresIn:    7200,
				User: &utils.UserProfile{
					Email: "auto@scandrix.dev",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "", "")
	svc := auth.NewService(client)

	// Save expired credential
	expiredCreds := &utils.StoredCredentials{
		AccessToken:  "old-expired-token",
		RefreshToken: "valid-refresh-token",
		ExpiresAt:    time.Now().UnixMilli() - 10000, // already expired
		User:         &utils.UserProfile{Email: "auto@scandrix.dev"},
	}
	_ = utils.SaveCredentials(expiredCreds)

	// Calling GetValidToken should trigger automatic refresh
	token, err := svc.GetValidToken(context.Background())
	if err != nil {
		t.Fatalf("GetValidToken returned error: %v", err)
	}
	if !refreshHit {
		t.Fatalf("expected /api/v1/auth/refresh endpoint to be called")
	}
	if token != "refreshed-access-token-999" {
		t.Errorf("expected refreshed token, got %s", token)
	}
}

func TestAuthService_Logout(t *testing.T) {
	setupTempHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "", "")
	svc := auth.NewService(client)

	// Setup stored creds
	_ = utils.SaveCredentials(&utils.StoredCredentials{
		AccessToken: "active-token",
	})

	if !svc.IsAuthenticated() {
		t.Fatalf("expected authenticated state before logout")
	}

	if err := svc.Logout(context.Background()); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	if svc.IsAuthenticated() {
		t.Fatalf("expected unauthenticated state after logout")
	}
}

func TestAuth_CredentialsFilePermissions(t *testing.T) {
	setupTempHome(t)

	creds := &utils.StoredCredentials{
		AccessToken:  "secret-token-value",
		RefreshToken: "secret-refresh-value",
		ExpiresAt:    time.Now().UnixMilli() + 3600000,
	}

	if err := utils.SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	path := utils.CredentialsPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed on credentials file: %v", err)
	}

	// File must have 0600 permissions
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected 0600 file permissions on %s, got %#o", path, perm)
	}

	// Directory must have 0700 permissions
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat failed on credentials directory: %v", err)
	}
	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf("expected 0700 directory permissions on %s, got %#o", filepath.Dir(path), dirPerm)
	}
}
