// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// TrackedRepository represents a connected repository in ScanDrix.
type TrackedRepository struct {
	ID            string `json:"id"`
	Namespace     string `json:"namespace"`
	Provider      string `json:"provider"`
	DefaultBranch string `json:"default_branch"`
	Name          string `json:"name,omitempty"`
	FullName      string `json:"full_name,omitempty"`
}

// UnmarshalJSON normalizes repository representations across provider endpoints.
func (t *TrackedRepository) UnmarshalJSON(data []byte) error {
	type Alias TrackedRepository
	var a struct {
		Alias
		FullName         string `json:"full_name"`
		Name             string `json:"name"`
		OrganizationName string `json:"organizationName"`
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*t = TrackedRepository(a.Alias)
	if t.Namespace == "" {
		if a.FullName != "" {
			t.Namespace = a.FullName
		} else if a.OrganizationName != "" && a.Name != "" {
			t.Namespace = a.OrganizationName + "/" + a.Name
		} else if a.Name != "" {
			t.Namespace = a.Name
		}
	}
	return nil
}

// RepositorySettings contains repo-specific review configuration.
type RepositorySettings struct {
	RepositoryID              string         `json:"repository_id"`
	Namespace                 string         `json:"namespace"`
	DefaultBranch             string         `json:"default_branch"`
	ReviewEnabled             bool           `json:"review_enabled"`
	AutoApproveEnabled        bool           `json:"auto_approve_enabled"`
	RequestChangesMinSeverity string         `json:"request_changes_min_severity,omitempty"`
	IgnoredPaths              []string       `json:"ignored_paths"`
	IgnoredFilePatterns       []string       `json:"ignored_file_patterns,omitempty"`
	BaseBranchPatterns        []string       `json:"base_branch_patterns,omitempty"`
	IgnoredTitlePatterns      []string       `json:"ignored_title_patterns,omitempty"`
	FocusAreas                []string       `json:"focus_areas,omitempty"`
	Reviewers                 []string       `json:"reviewers,omitempty"`
	CustomSettings            map[string]any `json:"custom_settings,omitempty"`
	Mode                      string         `json:"mode,omitempty"`
	PRURL                     string         `json:"pr_url,omitempty"`
	PRNumber                  int            `json:"pr_number,omitempty"`
	Message                   string         `json:"message,omitempty"`
}

// MarshalJSON provides dual camelCase and snake_case properties for compatibility.
func (s RepositorySettings) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"repositoryId":                 s.RepositoryID,
		"repository_id":                s.RepositoryID,
		"namespace":                    s.Namespace,
		"defaultBranch":                s.DefaultBranch,
		"default_branch":               s.DefaultBranch,
		"reviewEnabled":                s.ReviewEnabled,
		"review_enabled":               s.ReviewEnabled,
		"autoApproveEnabled":           s.AutoApproveEnabled,
		"auto_approve_enabled":          s.AutoApproveEnabled,
		"requestChangesMinSeverity":    s.RequestChangesMinSeverity,
		"request_changes_min_severity": s.RequestChangesMinSeverity,
		"ignoredPaths":                 s.IgnoredPaths,
		"ignored_paths":                s.IgnoredPaths,
		"ignoredFilePatterns":          s.IgnoredFilePatterns,
		"ignored_file_patterns":         s.IgnoredFilePatterns,
		"baseBranchPatterns":           s.BaseBranchPatterns,
		"base_branch_patterns":          s.BaseBranchPatterns,
		"ignoredTitlePatterns":         s.IgnoredTitlePatterns,
		"ignored_title_patterns":        s.IgnoredTitlePatterns,
		"focusAreas":                   s.FocusAreas,
		"focus_areas":                  s.FocusAreas,
		"reviewers":                    s.Reviewers,
		"customSettings":               s.CustomSettings,
		"custom_settings":              s.CustomSettings,
	})
}

// UnmarshalJSON normalizes both camelCase and snake_case server payloads.
func (s *RepositorySettings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	getBool := func(camel, snake string) bool {
		if v, ok := raw[camel]; ok {
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				return b
			}
		}
		if v, ok := raw[snake]; ok {
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				return b
			}
		}
		return false
	}

	getString := func(camel, snake string) string {
		if v, ok := raw[camel]; ok {
			var str string
			if err := json.Unmarshal(v, &str); err == nil {
				return str
			}
		}
		if v, ok := raw[snake]; ok {
			var str string
			if err := json.Unmarshal(v, &str); err == nil {
				return str
			}
		}
		return ""
	}

	getStringSlice := func(camel, snake string) []string {
		if v, ok := raw[camel]; ok {
			var sl []string
			if err := json.Unmarshal(v, &sl); err == nil {
				return sl
			}
		}
		if v, ok := raw[snake]; ok {
			var sl []string
			if err := json.Unmarshal(v, &sl); err == nil {
				return sl
			}
		}
		return nil
	}

	s.RepositoryID = getString("repositoryId", "repository_id")
	s.Namespace = getString("namespace", "namespace")
	s.DefaultBranch = getString("defaultBranch", "default_branch")
	s.ReviewEnabled = getBool("reviewEnabled", "review_enabled")
	s.AutoApproveEnabled = getBool("autoApproveEnabled", "auto_approve_enabled")
	s.RequestChangesMinSeverity = getString("requestChangesMinSeverity", "request_changes_min_severity")
	s.IgnoredPaths = getStringSlice("ignoredPaths", "ignored_paths")
	s.IgnoredFilePatterns = getStringSlice("ignoredFilePatterns", "ignored_file_patterns")
	s.BaseBranchPatterns = getStringSlice("baseBranchPatterns", "base_branch_patterns")
	s.IgnoredTitlePatterns = getStringSlice("ignoredTitlePatterns", "ignored_title_patterns")
	s.FocusAreas = getStringSlice("focusAreas", "focus_areas")
	s.Reviewers = getStringSlice("reviewers", "reviewers")
	s.Mode = getString("mode", "mode")
	s.PRURL = getString("prUrl", "pr_url")
	s.Message = getString("message", "message")
	if v, ok := raw["prNumber"]; ok {
		_ = json.Unmarshal(v, &s.PRNumber)
	} else if v, ok := raw["pr_number"]; ok {
		_ = json.Unmarshal(v, &s.PRNumber)
	}

	if v, ok := raw["customSettings"]; ok {
		_ = json.Unmarshal(v, &s.CustomSettings)
	} else if v, ok := raw["custom_settings"]; ok {
		_ = json.Unmarshal(v, &s.CustomSettings)
	}

	return nil
}

// CentralizedConfigRepository models repository metadata returned by centralized config endpoints.
type CentralizedConfigRepository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CentralizedConfigStatus models status of organization centralized config.
type CentralizedConfigStatus struct {
	Enabled            bool                         `json:"enabled"`
	Repository         *CentralizedConfigRepository `json:"repository,omitempty"`
	SelectedRepository *TrackedRepository           `json:"selected_repository,omitempty"`
	SyncMode           string                       `json:"sync_mode,omitempty"`
	TargetRepositories []string                     `json:"target_repositories,omitempty"`
	LastSyncedAt       string                       `json:"last_synced_at,omitempty"`
}

// CentralizedActionResponse represents the result of init, sync, or disable.
type CentralizedActionResponse struct {
	Success    bool               `json:"success"`
	Message    string             `json:"message"`
	Repository *TrackedRepository `json:"repository,omitempty"`
	PRURL      string             `json:"pr_url,omitempty"`
	PRUrlCamel string             `json:"prUrl,omitempty"`
}

// GetPRURL returns the pull request URL if present across formats.
func (c *CentralizedActionResponse) GetPRURL() string {
	if c.PRURL != "" {
		return c.PRURL
	}
	return c.PRUrlCamel
}

// TrackRepository registers a new repository with ScanDrix.
func (c *Client) TrackRepository(ctx context.Context, namespace, provider, branch string) (*TrackedRepository, error) {
	// First check available repositories if team key is active
	var available []TrackedRepository
	_ = c.Do(ctx, http.MethodGet, "/cli/config/repositories/available", nil, &available)
	for _, repo := range available {
		if repo.Namespace == namespace || repo.FullName == namespace || repo.Name == namespace || strings.HasSuffix(repo.Namespace, "/"+namespace) {
			payload := map[string]any{"repositoryIds": []string{repo.ID}}
			var addResp map[string]any
			if err := c.Do(ctx, http.MethodPost, "/cli/config/repositories", payload, &addResp); err == nil {
				return &repo, nil
			}
		}
	}

	payload := map[string]string{
		"namespace":      namespace,
		"provider":       provider,
		"default_branch": branch,
	}
	var resp TrackedRepository
	if err := c.Do(ctx, http.MethodPost, "/api/v1/repositories", payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListRepositories retrieves all repositories tracked by the active workspace.
func (c *Client) ListRepositories(ctx context.Context) ([]TrackedRepository, error) {
	var repos []TrackedRepository
	if err := c.Do(ctx, http.MethodGet, "/cli/config/repositories/selected", nil, &repos); err != nil {
		if errFallback := c.Do(ctx, http.MethodGet, "/api/v1/repositories", nil, &repos); errFallback != nil {
			return nil, err
		}
	}
	return repos, nil
}

// GetRepositorySettings retrieves configuration for a specific repository.
func (c *Client) GetRepositorySettings(ctx context.Context, repository string) (*RepositorySettings, error) {
	var settings RepositorySettings
	if err := c.Do(ctx, http.MethodGet, "/cli/config/repositories/"+repository+"/settings", nil, &settings); err != nil {
		if errFallback := c.Do(ctx, http.MethodGet, "/api/v1/repositories/"+repository+"/settings", nil, &settings); errFallback != nil {
			return nil, err
		}
	}
	return &settings, nil
}

// UpdateRepositorySettings updates configuration for a repository.
func (c *Client) UpdateRepositorySettings(ctx context.Context, repository string, settings RepositorySettings) (*RepositorySettings, error) {
	var resp RepositorySettings
	if err := c.Do(ctx, http.MethodPatch, "/cli/config/repositories/"+repository+"/settings", settings, &resp); err != nil {
		if errPut := c.Do(ctx, http.MethodPut, "/cli/config/repositories/"+repository+"/settings", settings, &resp); errPut != nil {
			if errFallback := c.Do(ctx, http.MethodPut, "/api/v1/repositories/"+repository+"/settings", settings, &resp); errFallback != nil {
				return nil, err
			}
		}
	}
	return &resp, nil
}

// AddRepositoryPattern appends a glob pattern to a repository configuration field.
func (c *Client) AddRepositoryPattern(ctx context.Context, repository, field, pattern string) (*RepositorySettings, error) {
	current, err := c.GetRepositorySettings(ctx, repository)
	if err != nil {
		current = &RepositorySettings{Namespace: repository}
	}

	normField := field
	switch normField {
	case "ignore-files", "ignored_paths", "ignore", "ignore-file":
		for _, p := range current.IgnoredFilePatterns {
			if p == pattern {
				return current, nil
			}
		}
		current.IgnoredFilePatterns = append(current.IgnoredFilePatterns, pattern)
		current.IgnoredPaths = append(current.IgnoredPaths, pattern)
	case "base-branches", "base-branch", "branches":
		for _, p := range current.BaseBranchPatterns {
			if p == pattern {
				return current, nil
			}
		}
		current.BaseBranchPatterns = append(current.BaseBranchPatterns, pattern)
	case "ignore-titles", "ignore-title", "titles":
		for _, p := range current.IgnoredTitlePatterns {
			if p == pattern {
				return current, nil
			}
		}
		current.IgnoredTitlePatterns = append(current.IgnoredTitlePatterns, pattern)
	case "review-files", "focus_areas", "focus":
		for _, p := range current.FocusAreas {
			if p == pattern {
				return current, nil
			}
		}
		current.FocusAreas = append(current.FocusAreas, pattern)
	default:
		if current.CustomSettings == nil {
			current.CustomSettings = make(map[string]any)
		}
		var list []string
		if existing, ok := current.CustomSettings[field].([]string); ok {
			list = existing
		}
		list = append(list, pattern)
		current.CustomSettings[field] = list
	}

	return c.UpdateRepositorySettings(ctx, repository, *current)
}

// RemoveRepositoryPattern removes a glob pattern from a repository configuration field.
func (c *Client) RemoveRepositoryPattern(ctx context.Context, repository, field, pattern string) (*RepositorySettings, error) {
	current, err := c.GetRepositorySettings(ctx, repository)
	if err != nil {
		return nil, err
	}

	normField := field
	switch normField {
	case "ignore-files", "ignored_paths", "ignore", "ignore-file":
		var updated []string
		for _, p := range current.IgnoredFilePatterns {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		current.IgnoredFilePatterns = updated

		var updatedPaths []string
		for _, p := range current.IgnoredPaths {
			if p != pattern {
				updatedPaths = append(updatedPaths, p)
			}
		}
		current.IgnoredPaths = updatedPaths
	case "base-branches", "base-branch", "branches":
		var updated []string
		for _, p := range current.BaseBranchPatterns {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		current.BaseBranchPatterns = updated
	case "ignore-titles", "ignore-title", "titles":
		var updated []string
		for _, p := range current.IgnoredTitlePatterns {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		current.IgnoredTitlePatterns = updated
	case "review-files", "focus_areas", "focus":
		var updated []string
		for _, p := range current.FocusAreas {
			if p != pattern {
				updated = append(updated, p)
			}
		}
		current.FocusAreas = updated
	default:
		if current.CustomSettings != nil {
			if existing, ok := current.CustomSettings[field].([]string); ok {
				var updated []string
				for _, p := range existing {
					if p != pattern {
						updated = append(updated, p)
					}
				}
				current.CustomSettings[field] = updated
			}
		}
	}

	return c.UpdateRepositorySettings(ctx, repository, *current)
}

// GetCentralizedStatus queries the current status of centralized configuration.
func (c *Client) GetCentralizedStatus(ctx context.Context) (*CentralizedConfigStatus, error) {
	var status CentralizedConfigStatus
	if err := c.Do(ctx, http.MethodGet, "/cli/config/centralized/status", nil, &status); err != nil {
		if errFallback := c.Do(ctx, http.MethodGet, "/api/v1/config/centralized/status", nil, &status); errFallback != nil {
			return nil, err
		}
	}
	return &status, nil
}

// InitCentralizedConfig enables centralized config with a chosen repository.
func (c *Client) InitCentralizedConfig(ctx context.Context, repository, syncOption string) (*CentralizedActionResponse, error) {
	payload := map[string]string{
		"repositoryId": repository,
		"repository":   repository,
		"syncOption":   syncOption,
		"sync_option":  syncOption,
	}
	var resp CentralizedActionResponse
	if err := c.Do(ctx, http.MethodPost, "/cli/config/centralized/init", payload, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/config/centralized/init", payload, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// SyncCentralizedConfig propagates centralized configuration to tracked repos.
func (c *Client) SyncCentralizedConfig(ctx context.Context) (*CentralizedActionResponse, error) {
	var resp CentralizedActionResponse
	if err := c.Do(ctx, http.MethodPost, "/cli/config/centralized/sync", nil, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/config/centralized/sync", nil, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// DisableCentralizedConfig deactivates centralized configuration.
func (c *Client) DisableCentralizedConfig(ctx context.Context) (*CentralizedActionResponse, error) {
	var resp CentralizedActionResponse
	if err := c.Do(ctx, http.MethodPost, "/cli/config/centralized/disable", nil, &resp); err != nil {
		if errFallback := c.Do(ctx, http.MethodPost, "/api/v1/config/centralized/disable", nil, &resp); errFallback != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// DownloadCentralizedZip downloads the centralized configuration zip package.
func (c *Client) DownloadCentralizedZip(ctx context.Context, outputPath string) error {
	downloadURL := c.serverURL + "/cli/config/centralized/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	if c.teamKey != "" {
		req.Header.Set("X-Team-Key", c.teamKey)
	} else if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		// Fallback to legacy path
		if resp != nil {
			resp.Body.Close()
		}
		fallbackURL := c.serverURL + "/api/v1/config/centralized/download"
		reqFallback, errF := http.NewRequestWithContext(ctx, http.MethodGet, fallbackURL, nil)
		if errF == nil {
			if c.teamKey != "" {
				reqFallback.Header.Set("X-Team-Key", c.teamKey)
			} else if c.authToken != "" {
				reqFallback.Header.Set("Authorization", "Bearer "+c.authToken)
			}
			resp, err = c.httpClient.Do(reqFallback)
		}
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (status %d): %s", resp.StatusCode, string(body))
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}

