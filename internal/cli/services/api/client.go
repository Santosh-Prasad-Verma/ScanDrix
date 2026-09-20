// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/utils"
)

// HTTP CLIENT RESILIENCE CONSTANTS & DEFAULTS

const (
	DefaultServerURL      = "http://localhost:8080"
	DefaultRequestTimeout = 60 * time.Minute
	MaxRetries            = 3
	InitialBackoff        = 500 * time.Millisecond
)

// Client handles authenticated, resilient HTTP communication with the ScanDrix API gateway.
type Client struct {
	serverURL  string
	authToken  string
	teamKey    string
	deviceID   string
	httpClient *http.Client
}

// CLIENT FACTORY & URL VALIDATION

// NewClient creates a new API client using loaded config and credentials.
func NewClient(serverURLOverride, tokenOverride, teamKeyOverride string) *Client {
	cfg := configcli.Load(".")

	serverURL := DefaultServerURL
	if serverURLOverride != "" {
		serverURL = serverURLOverride
	} else if cfg.ServerURL != "" {
		serverURL = cfg.ServerURL
	}

	// Validate API URL (HTTPS enforced unless localhost)
	serverURL = validateServerURL(serverURL)

	timeout := DefaultRequestTimeout
	if cfg.TimeoutMinutes > 0 {
		timeout = time.Duration(cfg.TimeoutMinutes) * time.Minute
	}
	if envTimeout := os.Getenv("SCANDRIX_REQUEST_TIMEOUT_MIN"); envTimeout != "" {
		if t, err := strconv.Atoi(envTimeout); err == nil && t > 0 {
			timeout = time.Duration(t) * time.Minute
		}
	}

	authToken := tokenOverride
	if authToken == "" {
		authToken = cfg.AccessToken
	}
	if authToken == "" {
		if creds, err := utils.LoadCredentials(); err == nil && creds != nil {
			authToken = creds.AccessToken
		}
	}

	teamKey := teamKeyOverride
	if teamKey == "" {
		teamKey = cfg.APIKey
	}

	return &Client{
		serverURL:  strings.TrimRight(serverURL, "/"),
		authToken:  authToken,
		teamKey:    teamKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func validateServerURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		utils.Warn("Invalid API URL format: %s. Falling back to default: %s", rawURL, DefaultServerURL)
		return DefaultServerURL
	}

	isLocal := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !isLocal {
		utils.Warn("Security Warning: Remote ScanDrix API URL must use HTTPS. Falling back to default: %s", DefaultServerURL)
		return DefaultServerURL
	}

	return rawURL
}

// ACCESSORS & MUTATORS

// ServerURL returns the configured base URL.
func (c *Client) ServerURL() string {
	return c.serverURL
}

// SetAuthToken updates the client's bearer token.
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
}

// SetTeamKey updates the client's team key.
func (c *Client) SetTeamKey(key string) {
	c.teamKey = key
}

// HTTP DISPATCH ENGINE (Backoff, Headers, Retry Handling)

// Do performs an HTTP request with exponential backoff and authentication headers.
func (c *Client) Do(ctx context.Context, method, path string, body any, result any) error {
	_, respBytes, err := c.DoRaw(ctx, method, path, nil, body)
	if err != nil {
		return err
	}

	if result != nil && len(respBytes) > 0 {
		// First check if response is wrapped in NestJS {"data": ...} envelope
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if errEnv := json.Unmarshal(respBytes, &env); errEnv == nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if errData := json.Unmarshal(env.Data, result); errData == nil {
				return nil
			}
		}

		if err := json.Unmarshal(respBytes, result); err != nil {
			return fmt.Errorf("failed decoding response JSON: %w (raw: %s)", err, string(respBytes))
		}
	}

	return nil
}

// DoRaw performs an HTTP request with headers, backoff, and returns the raw HTTP status and bytes.
func (c *Client) DoRaw(ctx context.Context, method, path string, extraHeaders map[string]string, body any) (int, []byte, error) {
	fullURL := c.serverURL + path
	if !strings.HasPrefix(path, "/") {
		fullURL = c.serverURL + "/" + path
	}

	var reqBytes []byte
	if body != nil {
		var err error
		reqBytes, err = json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("failed encoding JSON payload: %w", err)
		}
	}

	var lastErr error
	backoff := InitialBackoff

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
			backoff *= 2
		}

		var bodyReader io.Reader
		if reqBytes != nil {
			bodyReader = bytes.NewReader(reqBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return 0, nil, fmt.Errorf("failed creating HTTP request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("ScanDrix-CLI/%s", utils.CLIVersion))

		// Apply custom extra headers
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}

		// Apply authentication
		if c.teamKey != "" {
			req.Header.Set("X-Team-Key", c.teamKey)
			req.Header.Set("X-API-Key", c.teamKey)
			if !strings.HasPrefix(c.teamKey, "Bearer ") {
				req.Header.Set("Authorization", "Bearer "+c.teamKey)
			}
		} else if c.authToken != "" {
			token := c.authToken
			if !strings.HasPrefix(token, "Bearer ") {
				token = "Bearer " + token
			}
			req.Header.Set("Authorization", token)
		}

		if c.deviceID != "" {
			req.Header.Set("X-Device-Id", c.deviceID)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			utils.Debug("HTTP request attempt %d failed: %v", attempt+1, err)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed reading response body: %w", err)
			continue
		}

		// Handle retryable status codes (429, 502, 503, 504)
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout {
			lastErr = fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBytes))
			utils.Debug("Retryable status %d encountered, retrying...", resp.StatusCode)
			continue
		}

		// Handle error status codes
		if resp.StatusCode >= 400 {
			// Automatic 401 fallback: if bearer session expired/unauthorized and a team key is configured, retry with team key
			if resp.StatusCode == http.StatusUnauthorized && c.authToken != "" && c.teamKey == "" && !strings.HasPrefix(c.authToken, "scandrix_") {
				cfg := configcli.Load(".")
				if cfg.APIKey != "" {
					utils.Debug("Bearer token unauthorized (401), falling back to configured team key")
					c.teamKey = cfg.APIKey
					c.authToken = ""
					continue
				}
				if creds, _ := utils.LoadCredentials(); creds != nil && creds.TeamKey != "" {
					utils.Debug("Bearer token unauthorized (401), falling back to credentials team key")
					c.teamKey = creds.TeamKey
					c.authToken = ""
					continue
				}
			}

			var errResp struct {
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			_ = json.Unmarshal(respBytes, &errResp)
			errMsg := errResp.Message
			if errMsg == "" {
				errMsg = errResp.Error
			}
			if errMsg == "" {
				errMsg = string(respBytes)
			}
			return resp.StatusCode, respBytes, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errMsg)
		}

		return resp.StatusCode, respBytes, nil
	}

	return 0, nil, fmt.Errorf("request to %s failed after %d attempts: %w", fullURL, MaxRetries+1, lastErr)
}
