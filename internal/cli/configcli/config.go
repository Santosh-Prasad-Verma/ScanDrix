// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package configcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CLIConfig encapsulates hierarchical configuration loaded from environments,
// global user directories, and repository-level settings files.
type CLIConfig struct {
	ServerURL        string `json:"server_url"`
	BillingURL       string `json:"billing_url,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	UserEmail        string `json:"user_email,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	DefaultFormat    string `json:"default_format,omitempty"`
	FailOnSeverity   string `json:"fail_on_severity,omitempty"`
	AutoReviewStaged bool   `json:"auto_review_staged,omitempty"`
	TimeoutMinutes   int    `json:"timeout_minutes,omitempty"`
	Verbose          bool   `json:"verbose,omitempty"`
	Quiet            bool   `json:"quiet,omitempty"`
}

// GlobalConfigPath returns the absolute path to ~/.scandrix/config.json.
func GlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix", "config.json")
}

// RepoConfigPath returns the absolute path to .scandrix/config.json within workDir.
func RepoConfigPath(workDir string) string {
	if workDir == "" {
		workDir = "."
	}
	return filepath.Join(workDir, ".scandrix", "config.json")
}

// Load merges global, repo-level, and environment configurations hierarchically.
// Priority: Environment Variables > Repo Config > Global Config > Safe Defaults.
func Load(workDir string) *CLIConfig {
	cfg := &CLIConfig{
		ServerURL:      "http://localhost:8080",
		BillingURL:     "https://scandrix.dev/pricing",
		DefaultFormat:  "terminal",
		FailOnSeverity: "HIGH",
		TimeoutMinutes: 60,
	}

	// 1. Load global user configuration (~/.scandrix/config.json)
	if data, err := os.ReadFile(GlobalConfigPath()); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// 2. Load repository-level configuration (.scandrix/config.json)
	if data, err := os.ReadFile(RepoConfigPath(workDir)); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// 3. Apply Environment Variable overrides
	if envURL := os.Getenv("SCANDRIX_SERVER_URL"); envURL != "" {
		cfg.ServerURL = strings.TrimRight(envURL, "/")
	}
	if envBilling := os.Getenv("SCANDRIX_BILLING_URL"); envBilling != "" {
		cfg.BillingURL = envBilling
	}
	if envKey := os.Getenv("SCANDRIX_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	}
	if envToken := os.Getenv("SCANDRIX_ACCESS_TOKEN"); envToken != "" {
		cfg.AccessToken = envToken
	}
	if envRefresh := os.Getenv("SCANDRIX_REFRESH_TOKEN"); envRefresh != "" {
		cfg.RefreshToken = envRefresh
	}
	if envWS := os.Getenv("SCANDRIX_WORKSPACE_ID"); envWS != "" {
		cfg.WorkspaceID = envWS
	}
	if envFmt := os.Getenv("SCANDRIX_FORMAT"); envFmt != "" {
		cfg.DefaultFormat = strings.ToLower(envFmt)
	}
	if envFail := os.Getenv("SCANDRIX_FAIL_ON_SEVERITY"); envFail != "" {
		cfg.FailOnSeverity = strings.ToUpper(envFail)
	}
	if envTimeout := os.Getenv("SCANDRIX_REQUEST_TIMEOUT_MIN"); envTimeout != "" {
		if t, err := strconv.Atoi(envTimeout); err == nil && t > 0 {
			cfg.TimeoutMinutes = t
		}
	}
	if envVerbose := os.Getenv("SCANDRIX_VERBOSE"); envVerbose == "1" || strings.ToLower(envVerbose) == "true" {
		cfg.Verbose = true
	}
	if envQuiet := os.Getenv("SCANDRIX_QUIET"); envQuiet == "1" || strings.ToLower(envQuiet) == "true" {
		cfg.Quiet = true
	}

	return cfg
}

// SaveGlobal persists credentials and settings to ~/.scandrix/config.json.
func SaveGlobal(cfg *CLIConfig) error {
	path := GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// SaveRepo persists repo-specific settings to .scandrix/config.json.
func SaveRepo(workDir string, cfg *CLIConfig) error {
	path := RepoConfigPath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ----------------------------------------------------------------------------
// Remote Repository Management Models & API Operations
// ----------------------------------------------------------------------------

// TrackRepositoryRequest defines payload to track a repository in ScanDrix.
type TrackRepositoryRequest struct {
	Namespace string `json:"namespace"`
	Provider  string `json:"provider"`
	Branch    string `json:"default_branch,omitempty"`
}

// TrackRepositoryResponse returns repository metadata from the server.
type TrackRepositoryResponse struct {
	ID            string    `json:"id"`
	Namespace     string    `json:"namespace"`
	Provider      string    `json:"provider"`
	DefaultBranch string    `json:"default_branch"`
	CreatedAt     time.Time `json:"created_at"`
}

// CentralizedConfigStatus models status of centralized repository rules.
type CentralizedConfigStatus struct {
	Enabled       bool     `json:"enabled"`
	SelectedRepo  string   `json:"selected_repository,omitempty"`
	SyncMode      string   `json:"sync_mode,omitempty"`
	TargetRepos   []string `json:"target_repositories,omitempty"`
	LastSyncedAt  string   `json:"last_synced_at,omitempty"`
}

// CentralizedConfigActionResponse models response from centralized actions.
type CentralizedConfigActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	PRURL   string `json:"pr_url,omitempty"`
}

// APIClient provides authenticated HTTP communication with the ScanDrix API gateway.
type APIClient struct {
	serverURL  string
	authToken  string
	apiKey     string
	httpClient *http.Client
}

// NewAPIClient creates a configured APIClient instance.
func NewAPIClient(cfg *CLIConfig) *APIClient {
	timeout := time.Duration(cfg.TimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 60 * time.Minute
	}
	return &APIClient{
		serverURL:  cfg.ServerURL,
		authToken:  cfg.AccessToken,
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// DoRequest performs an authenticated HTTP request with JSON payload handling.
func (c *APIClient) DoRequest(ctx context.Context, method, endpoint string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.serverURL + endpoint
	if !strings.HasPrefix(endpoint, "/") {
		url = c.serverURL + "/" + endpoint
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ScanDrix-CLI/v1.2.0")

	// Apply authentication headers: Bearer Token or Team Key
	if c.apiKey != "" {
		req.Header.Set("X-Team-Key", c.apiKey)
	} else if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection to ScanDrix API (%s) failed: %w", c.serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed decoding response: %w", err)
		}
	}

	return nil
}

// GetCentralizedConfigStatus fetches current organization centralized config state.
func (c *APIClient) GetCentralizedConfigStatus(ctx context.Context) (*CentralizedConfigStatus, error) {
	var status CentralizedConfigStatus
	if err := c.DoRequest(ctx, http.MethodGet, "/api/v1/config/centralized/status", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// InitCentralizedConfig initializes centralized repository rules.
func (c *APIClient) InitCentralizedConfig(ctx context.Context, repoID, syncOption string) (*CentralizedConfigActionResponse, error) {
	payload := map[string]string{
		"repository_id": repoID,
		"sync_option":   syncOption,
	}
	var res CentralizedConfigActionResponse
	if err := c.DoRequest(ctx, http.MethodPost, "/api/v1/config/centralized/init", payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// SyncCentralizedConfig triggers an immediate sync of organization rules.
func (c *APIClient) SyncCentralizedConfig(ctx context.Context) (*CentralizedConfigActionResponse, error) {
	var res CentralizedConfigActionResponse
	if err := c.DoRequest(ctx, http.MethodPost, "/api/v1/config/centralized/sync", nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DisableCentralizedConfig disables centralized configuration sync.
func (c *APIClient) DisableCentralizedConfig(ctx context.Context) (*CentralizedConfigActionResponse, error) {
	var res CentralizedConfigActionResponse
	if err := c.DoRequest(ctx, http.MethodPost, "/api/v1/config/centralized/disable", nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// TrackRepository registers a repository with the ScanDrix platform.
func (c *APIClient) TrackRepository(ctx context.Context, namespace, provider string) (*TrackRepositoryResponse, error) {
	payload := TrackRepositoryRequest{
		Namespace: namespace,
		Provider:  provider,
	}
	var res TrackRepositoryResponse
	if err := c.DoRequest(ctx, http.MethodPost, "/api/v1/repositories", payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ListTrackedRepositories lists all tracked repositories for the active workspace.
func (c *APIClient) ListTrackedRepositories(ctx context.Context) ([]TrackRepositoryResponse, error) {
	var res []TrackRepositoryResponse
	if err := c.DoRequest(ctx, http.MethodGet, "/api/v1/repositories", nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}
