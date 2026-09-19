// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
)

// CONSTANTS & SESSION TIMING

const TokenRefreshBufferMs = 5 * 60 * 1000 // 5 minutes

// Service provides concurrency-safe authentication, token rotation, and credential resolution.
type Service struct {
	apiClient   *api.Client
	cachedCreds *utils.StoredCredentials
	refreshMu   sync.Mutex
	credsMu     sync.RWMutex
}

var (
	defaultAuthService *Service
	serviceOnce        sync.Once
)

// INITIALIZATION & SINGLETON ACCESS

// DefaultService returns the singleton AuthService instance.
func DefaultService() *Service {
	serviceOnce.Do(func() {
		defaultAuthService = NewService(api.NewClient("", "", ""))
	})
	return defaultAuthService
}

// NewService creates a new AuthService bound to an API client.
func NewService(client *api.Client) *Service {
	return &Service{
		apiClient: client,
	}
}

// CREDENTIAL RESOLUTION & ENVIRONMENT OVERRIDES

// getEnvAuthToken checks environment variables for override tokens or team keys.
func (s *Service) getEnvAuthToken() string {
	if token := strings.TrimSpace(os.Getenv("SCANDRIX_TOKEN")); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("SCANDRIX_ACCESS_TOKEN")); token != "" {
		return token
	}
	if key := strings.TrimSpace(os.Getenv("SCANDRIX_TEAM_KEY")); key != "" {
		return key
	}
	if key := strings.TrimSpace(os.Getenv("SCANDRIX_API_KEY")); key != "" {
		return key
	}
	return ""
}

// IsAuthenticated returns true if any valid authentication mechanism is configured.
func (s *Service) IsAuthenticated() bool {
	if s.getEnvAuthToken() != "" {
		return true
	}

	creds, _ := s.GetCredentials()
	if creds != nil && creds.AccessToken != "" {
		return true
	}

	cfg := configcli.Load(".")
	return cfg.APIKey != "" || cfg.AccessToken != ""
}

// GetCredentials retrieves credentials from memory or ~/.scandrix/credentials.json.
func (s *Service) GetCredentials() (*utils.StoredCredentials, error) {
	s.credsMu.RLock()
	if s.cachedCreds != nil {
		defer s.credsMu.RUnlock()
		return s.cachedCreds, nil
	}
	s.credsMu.RUnlock()

	creds, err := utils.LoadCredentials()
	if err == nil && creds != nil {
		s.credsMu.Lock()
		s.cachedCreds = creds
		s.credsMu.Unlock()
		return creds, nil
	}

	return nil, err
}

// RFC 8252 LOOPBACK OAUTH AUTHORIZATION FLOW (Browser Login)

// LoginViaBrowser initiates an RFC 8252 loopback authorization flow with local callback server.
func (s *Service) LoginViaBrowser(ctx context.Context, onOpenURL func(string)) (*utils.UserProfile, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed binding local loopback listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	callbackChan := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(cliauth.LoopbackCallbackHTML))
		select {
		case callbackChan <- state:
		default:
		}
	})

	httpServer := &http.Server{Handler: mux}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	initRes, err := s.apiClient.InitLoopback(ctx, port)
	if err != nil {
		return nil, fmt.Errorf("loopback initiate failed: %w", err)
	}

	if onOpenURL != nil {
		onOpenURL(initRes.VerificationURI)
	}

	timeoutDur := time.Duration(initRes.ExpiresIn) * time.Second
	if timeoutDur <= 0 {
		timeoutDur = 10 * time.Minute
	}

	select {
	case state := <-callbackChan:
		if state != initRes.State {
			return nil, fmt.Errorf("authorization state mismatch (possible CSRF attempt)")
		}
	case <-time.After(timeoutDur):
		return nil, fmt.Errorf("authorization timed out after %v", timeoutDur)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Poll until tokens are available
	pollCtx, cancelPoll := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPoll()

	// Check immediately before ticker delay
	if pollResult, err := s.apiClient.PollLoopback(pollCtx, initRes.State); err == nil {
		if strings.EqualFold(string(pollResult.Status), string(cliauth.StatusCompleted)) && pollResult.AccessToken != "" {
			return s.SaveSessionTokens(pollResult.AccessToken, pollResult.RefreshToken, pollResult.UserEmail, int64(initRes.ExpiresIn))
		}
	}

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf("polling for authorization token timed out")
		case <-ticker.C:
			pollResult, err := s.apiClient.PollLoopback(pollCtx, initRes.State)
			if err != nil {
				continue
			}
			if strings.EqualFold(string(pollResult.Status), string(cliauth.StatusCompleted)) && pollResult.AccessToken != "" {
				return s.SaveSessionTokens(pollResult.AccessToken, pollResult.RefreshToken, pollResult.UserEmail, int64(initRes.ExpiresIn))
			}
			if strings.EqualFold(string(pollResult.Status), string(cliauth.StatusDenied)) {
				return nil, fmt.Errorf("authorization was denied by the user")
			}
			if strings.EqualFold(string(pollResult.Status), string(cliauth.StatusExpired)) {
				return nil, fmt.Errorf("loopback authorization session expired")
			}
		}
	}
}

// RFC 8628 DEVICE AUTHORIZATION FLOW (Browser Login)

// StartDeviceFlow initiates an RFC 8628 OAuth device login session.
func (s *Service) StartDeviceFlow(ctx context.Context) (*cliauth.DeviceLoginInitiateResult, error) {
	return s.apiClient.StartDeviceAuth(ctx)
}

// PollDeviceFlow polls for user consent on an initiated device session.
func (s *Service) PollDeviceFlow(ctx context.Context, deviceCode string) (*cliauth.DeviceLoginPollResult, error) {
	return s.apiClient.PollDeviceToken(ctx, deviceCode)
}

// SaveSessionTokens stores tokens received from device authorization or direct login.
func (s *Service) SaveSessionTokens(accessToken, refreshToken, email string, expiresInSeconds int64) (*utils.UserProfile, error) {
	if expiresInSeconds <= 0 {
		expiresInSeconds = 3600 // Default 1 hour
	}

	userProfile := &utils.UserProfile{
		Email: email,
	}

	// Extract claims from JWT if available
	if accessToken != "" {
		parts := strings.Split(accessToken, ".")
		if len(parts) >= 2 {
			if payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var claims struct {
					Sub   string `json:"sub"`
					WS    string `json:"ws"`
					Role  string `json:"role"`
					Email string `json:"email"`
				}
				if json.Unmarshal(payloadBytes, &claims) == nil {
					if claims.Email != "" && userProfile.Email == "" {
						userProfile.Email = claims.Email
					}
					userProfile.ID = claims.Sub
					userProfile.Role = claims.Role
					userProfile.Workspace = claims.WS
				}
			}
		}
	}

	creds := &utils.StoredCredentials{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().UnixMilli() + (expiresInSeconds * 1000),
		User:         userProfile,
	}

	if err := utils.SaveCredentials(creds); err != nil {
		return nil, fmt.Errorf("failed saving credentials: %w", err)
	}

	// Also sync global config file for legacy compatibility
	cfg := configcli.Load(".")
	cfg.AccessToken = accessToken
	cfg.RefreshToken = refreshToken
	cfg.UserEmail = userProfile.Email
	_ = configcli.SaveGlobal(cfg)

	s.credsMu.Lock()
	s.cachedCreds = creds
	s.credsMu.Unlock()

	s.apiClient.SetAuthToken(accessToken)
	return userProfile, nil
}

// PASSWORD AUTHENTICATION & DIRECT LOGIN

// Login authenticates with email/password and saves credentials securely.
func (s *Service) Login(ctx context.Context, email, password string) (*utils.UserProfile, error) {
	resp, err := s.apiClient.Login(ctx, email, password)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	user := resp.User
	if user == nil {
		user = &utils.UserProfile{Email: email}
	}

	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	return s.SaveSessionTokens(resp.AccessToken, resp.RefreshToken, user.Email, expiresIn)
}

// TEAM API KEY CONFIGURATION

// SetTeamKey persists a team API key to ~/.scandrix/config.json.
func (s *Service) SetTeamKey(ctx context.Context, key string) (string, error) {
	valid, org, err := s.apiClient.VerifyTeamKey(ctx, key)
	if err != nil || !valid {
		return "", fmt.Errorf("invalid team API key: %v", err)
	}

	cfg := configcli.Load(".")
	cfg.APIKey = key
	if err := configcli.SaveGlobal(cfg); err != nil {
		return "", fmt.Errorf("failed saving team key to config: %w", err)
	}

	s.apiClient.SetTeamKey(key)
	return org, nil
}

// LOGOUT & SESSION TERMINATION

// Logout removes stored credentials and clears in-memory state.
func (s *Service) Logout(ctx context.Context) error {
	_ = s.apiClient.Logout(ctx)
	_ = utils.ClearCredentials()

	cfg := configcli.Load(".")
	cfg.AccessToken = ""
	cfg.RefreshToken = ""
	cfg.APIKey = ""
	cfg.UserEmail = ""
	_ = configcli.SaveGlobal(cfg)

	s.credsMu.Lock()
	s.cachedCreds = nil
	s.credsMu.Unlock()

	return nil
}

// TOKEN VALIDATION & AUTO-ROTATION

// GetValidToken returns a valid bearer token or team key, automatically refreshing expired tokens.
func (s *Service) GetValidToken(ctx context.Context) (string, error) {
	// 1. Environment variable override
	if envToken := s.getEnvAuthToken(); envToken != "" {
		return envToken, nil
	}

	// 2. Stored credentials with proactive refresh
	creds, _ := s.GetCredentials()
	if creds != nil && creds.AccessToken != "" {
		nowMs := time.Now().UnixMilli()
		isExpiredOrClose := creds.ExpiresAt > 0 && nowMs > (creds.ExpiresAt-TokenRefreshBufferMs)

		if isExpiredOrClose && creds.RefreshToken != "" {
			return s.refreshTokenSingleFlight(ctx, creds.RefreshToken)
		}

		return creds.AccessToken, nil
	}

	// 3. Team key in global config
	cfg := configcli.Load(".")
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}
	if cfg.AccessToken != "" {
		return cfg.AccessToken, nil
	}

	return "", utils.NewCommandError(utils.ErrCodeAuthRequired, "Not authenticated. Run: scandrix auth login or scandrix auth team-key --key <your-key>", 1, nil)
}

// refreshTokenSingleFlight ensures only one refresh request runs concurrently.
func (s *Service) refreshTokenSingleFlight(ctx context.Context, refreshToken string) (string, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	// Re-check credentials in case another goroutine just refreshed them
	s.credsMu.RLock()
	currentCreds := s.cachedCreds
	s.credsMu.RUnlock()

	nowMs := time.Now().UnixMilli()
	if currentCreds != nil && currentCreds.ExpiresAt > (nowMs+TokenRefreshBufferMs) {
		return currentCreds.AccessToken, nil
	}

	resp, err := s.apiClient.Refresh(ctx, refreshToken)
	if err != nil {
		_ = utils.ClearCredentials()
		s.credsMu.Lock()
		s.cachedCreds = nil
		s.credsMu.Unlock()

		cfg := configcli.Load(".")
		if cfg.APIKey != "" {
			return cfg.APIKey, nil
		}

		return "", utils.NewCommandError(utils.ErrCodeAuthRequired, "Session expired. Run: scandrix auth login", 1, nil)
	}

	newCreds := &utils.StoredCredentials{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + (resp.ExpiresIn * 1000),
		User:         resp.User,
	}

	_ = utils.SaveCredentials(newCreds)
	s.credsMu.Lock()
	s.cachedCreds = newCreds
	s.credsMu.Unlock()

	s.apiClient.SetAuthToken(resp.AccessToken)
	return resp.AccessToken, nil
}
