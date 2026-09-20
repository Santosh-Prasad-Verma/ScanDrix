// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/utils"
)

// 1. DATA CONTRACTS & DTOs

// LoginRequest defines credentials for password authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse defines tokens returned by auth endpoints.
type AuthResponse struct {
	AccessToken  string             `json:"access_token"`
	RefreshToken string             `json:"refresh_token,omitempty"`
	ExpiresIn    int64              `json:"expires_in"` // Seconds
	User         *utils.UserProfile `json:"user,omitempty"`
}

// RefreshRequest defines payload to rotate access token.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// WhoamiResponse describes the authenticated user and active workspace.
type WhoamiResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Role        string `json:"role,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Workspace   string `json:"workspace_name,omitempty"`
	AuthType    string `json:"auth_type,omitempty"`
}

// 2. AUTHENTICATION SERVICE CALLS (Password, Refresh, Logout, Whoami)

// Login authenticates with email and password against /api/v1/auth/login.
func (c *Client) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	req := LoginRequest{Email: email, Password: password}
	var resp AuthResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/login", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Refresh obtains a new access token using a valid refresh token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	req := RefreshRequest{RefreshToken: refreshToken}
	var resp AuthResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/refresh", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Logout clears the session on the backend.
func (c *Client) Logout(ctx context.Context) error {
	return c.Do(ctx, http.MethodPost, "/api/v1/auth/logout", nil, nil)
}

// Whoami retrieves the current user profile from /api/v1/auth/me.
func (c *Client) Whoami(ctx context.Context) (*WhoamiResponse, error) {
	var resp WhoamiResponse
	if err := c.Do(ctx, http.MethodGet, "/api/v1/auth/me", nil, &resp); err != nil {
		return nil, err
	}
	if resp.Name == "" && resp.DisplayName != "" {
		resp.Name = resp.DisplayName
	}
	return &resp, nil
}

// 3. TEAM API KEY VALIDATION & CI TOKENS

// GenerateCIToken creates an automation token for CI/CD pipelines.
func (c *Client) GenerateCIToken(ctx context.Context) (string, error) {
	var resp struct {
		Token string `json:"token"`
	}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/cli-keys", nil, &resp); err != nil {
		return "", err
	}
	return resp.Token, nil
}

// VerifyTeamKey checks if a team API key is valid via /cli/validate-key.
func (c *Client) VerifyTeamKey(ctx context.Context, key string) (bool, string, error) {
	oldKey := c.teamKey
	c.teamKey = key
	defer func() { c.teamKey = oldKey }()

	var resp struct {
		Valid        bool   `json:"valid"`
		Error        string `json:"error,omitempty"`
		Team         struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
		Organization struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organization"`
	}

	if err := c.Do(ctx, http.MethodPost, "/cli/validate-key", nil, &resp); err != nil {
		if errGet := c.Do(ctx, http.MethodGet, "/cli/validate-key", nil, &resp); errGet != nil {
			// Fallback to /api/v1/cli/validate-key
			if errLegacy := c.Do(ctx, http.MethodPost, "/api/v1/cli/validate-key", nil, &resp); errLegacy != nil {
				if errLegacyGet := c.Do(ctx, http.MethodGet, "/api/v1/cli/validate-key", nil, &resp); errLegacyGet != nil {
					return false, "", err
				}
			}
		}
	}

	if !resp.Valid && resp.Error != "" {
		return false, "", fmt.Errorf("%s", resp.Error)
	}

	name := resp.Team.Name
	if name == "" {
		name = resp.Organization.Name
	}
	return resp.Valid, name, nil
}

// 4. RFC 8628 CLI DEVICE AUTHORIZATION (Browser Login)

// StartDeviceAuth initiates the RFC 8628 device authorization flow via /cli/auth/device-init.
func (c *Client) StartDeviceAuth(ctx context.Context) (*cliauth.DeviceLoginInitiateResult, error) {
	var resp cliauth.DeviceLoginInitiateResult
	if err := c.Do(ctx, http.MethodPost, "/cli/auth/device-init", map[string]string{}, &resp); err != nil {
		// Fallback to /api/v1/auth/cli/device/initiate
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/auth/cli/device/initiate", map[string]string{}, &resp); errFallback != nil {
			if errLegacy := c.Do(ctx, http.MethodPost, "/api/v1/cli/device/initiate", map[string]string{}, &resp); errLegacy != nil {
				return nil, err
			}
		}
	}
	return &resp, nil
}

// PollDeviceToken polls the device login session via /cli/auth/login-poll until approved or expired.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (*cliauth.DeviceLoginPollResult, error) {
	endpoint := fmt.Sprintf("/cli/auth/login-poll?device_code=%s", deviceCode)
	var resp cliauth.DeviceLoginPollResult
	if err := c.Do(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		fallbackEndpoint := fmt.Sprintf("/api/v1/auth/cli/device/poll?device_code=%s", deviceCode)
		if errFallback := c.Do(ctx, http.MethodGet, fallbackEndpoint, nil, &resp); errFallback != nil {
			legacyEndpoint := fmt.Sprintf("/api/v1/cli/device/poll?device_code=%s", deviceCode)
			if errLegacy := c.Do(ctx, http.MethodGet, legacyEndpoint, nil, &resp); errLegacy != nil {
				return nil, err
			}
		}
	}
	return &resp, nil
}

// 5. RFC 8252 LOOPBACK BROWSER AUTHORIZATION

// InitLoopback initiates an RFC 8252 loopback authorization session on localhost port.
func (c *Client) InitLoopback(ctx context.Context, port int) (*cliauth.LoopbackInitiateResult, error) {
	req := map[string]int{"port": port}
	var resp cliauth.LoopbackInitiateResult
	if err := c.Do(ctx, http.MethodPost, "/cli/auth/login-init", req, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/cli/auth/login-init", req, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// PollLoopback checks if the loopback session has been authorized by the user.
func (c *Client) PollLoopback(ctx context.Context, state string) (*cliauth.LoopbackPollResult, error) {
	endpoint := fmt.Sprintf("/cli/auth/login-poll?state=%s", state)
	var resp cliauth.LoopbackPollResult
	if err := c.Do(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		fallbackEndpoint := fmt.Sprintf("/api/v1/cli/auth/login-poll?state=%s", state)
		if errFallback := c.Do(ctx, http.MethodGet, fallbackEndpoint, nil, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}
