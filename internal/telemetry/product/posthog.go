// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultPostHogHost = "https://us.i.posthog.com"
	posthogTimeout     = 5 * time.Second
)

// FeatureEvaluationContext provides additional group targeting for flag checks.
type FeatureEvaluationContext struct {
	Groups map[string]string `json:"groups,omitempty"`
}

// PostHogClient defines the product analytics contract.
type PostHogClient interface {
	IsEnabled() bool
	Capture(ctx context.Context, distinctID string, event string, properties map[string]any, groups map[string]string) error
	Identify(ctx context.Context, distinctID string, properties map[string]any) error
	GroupIdentify(ctx context.Context, groupType string, groupKey string, properties map[string]any) error
	IsFeatureEnabled(ctx context.Context, featureName string, identifier string, orgTeamData any, evalCtx *FeatureEvaluationContext) (bool, error)
}

// HTTPPostHogClient implements PostHogClient via the PostHog REST API.
type HTTPPostHogClient struct {
	apiKey     string
	host       string
	httpClient *http.Client
}

// NewPostHogClient initializes a new PostHog analytics provider from environment variables.
func NewPostHogClient() *HTTPPostHogClient {
	apiKey := strings.TrimSpace(os.Getenv("POSTHOG_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("API_POSTHOG_KEY"))
	}
	host := strings.TrimSpace(os.Getenv("POSTHOG_HOST"))
	if host == "" {
		host = defaultPostHogHost
	}
	host = strings.TrimRight(host, "/")

	return &HTTPPostHogClient{
		apiKey: apiKey,
		host:   host,
		httpClient: &http.Client{
			Timeout: posthogTimeout,
		},
	}
}

// NewPostHogClientWithConfig creates a client with explicit configuration.
func NewPostHogClientWithConfig(apiKey, host string, httpClient *http.Client) *HTTPPostHogClient {
	if host == "" {
		host = defaultPostHogHost
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: posthogTimeout}
	}
	return &HTTPPostHogClient{
		apiKey:     apiKey,
		host:       strings.TrimRight(host, "/"),
		httpClient: httpClient,
	}
}

func (p *HTTPPostHogClient) IsEnabled() bool {
	return p.apiKey != ""
}

// Capture emits a discrete analytics event with properties and group linkages.
func (p *HTTPPostHogClient) Capture(ctx context.Context, distinctID string, event string, properties map[string]any, groups map[string]string) error {
	if !p.IsEnabled() {
		return nil
	}

	props := make(map[string]any)
	for k, v := range properties {
		props[k] = v
	}

	if len(groups) > 0 {
		groupMap := make(map[string]string)
		for k, v := range groups {
			if strings.TrimSpace(v) != "" {
				groupMap[k] = v
			}
		}
		if len(groupMap) > 0 {
			props["$groups"] = groupMap
		}
	}

	payload := map[string]any{
		"api_key":     p.apiKey,
		"event":       event,
		"distinct_id": distinctID,
		"properties":  props,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	return p.sendPost(ctx, "/capture", payload)
}

// Identify associates distinct properties with an identified user.
func (p *HTTPPostHogClient) Identify(ctx context.Context, distinctID string, properties map[string]any) error {
	if !p.IsEnabled() {
		return nil
	}

	payload := map[string]any{
		"api_key":     p.apiKey,
		"event":       "$identify",
		"distinct_id": distinctID,
		"properties": map[string]any{
			"$set": properties,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	return p.sendPost(ctx, "/capture", payload)
}

// GroupIdentify associates metadata with a parent group (organization, team, repository).
func (p *HTTPPostHogClient) GroupIdentify(ctx context.Context, groupType string, groupKey string, properties map[string]any) error {
	if !p.IsEnabled() {
		return nil
	}

	payload := map[string]any{
		"api_key":     p.apiKey,
		"event":       "$groupidentify",
		"distinct_id": fmt.Sprintf("%s_%s", groupType, groupKey),
		"properties": map[string]any{
			"$group_type": groupType,
			"$group_key":  groupKey,
			"$group_set":  properties,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	return p.sendPost(ctx, "/capture", payload)
}

// IsFeatureEnabled queries the PostHog feature flags API for flag evaluation.
func (p *HTTPPostHogClient) IsFeatureEnabled(ctx context.Context, featureName string, identifier string, orgTeamData any, evalCtx *FeatureEvaluationContext) (bool, error) {
	if !p.IsEnabled() {
		return true, nil
	}

	groups := make(map[string]string)
	if orgData, ok := orgTeamData.(map[string]any); ok {
		if orgID, ok := orgData["organizationId"].(string); ok && orgID != "" {
			groups["organization"] = orgID
		}
	} else if orgIDGetter, ok := orgTeamData.(interface{ GetOrganizationID() string }); ok {
		if id := orgIDGetter.GetOrganizationID(); id != "" {
			groups["organization"] = id
		}
	}
	if evalCtx != nil && evalCtx.Groups != nil {
		for k, v := range evalCtx.Groups {
			if strings.TrimSpace(v) != "" {
				groups[k] = v
			}
		}
	}

	payload := map[string]any{
		"api_key":     p.apiKey,
		"distinct_id": identifier,
	}
	if len(groups) > 0 {
		payload["groups"] = groups
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal PostHog decide payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/decide?v=3", p.host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("failed to create PostHog decide request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("PostHog decide request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("PostHog decide returned non-2xx status: %d", resp.StatusCode)
	}

	var decideResp struct {
		FeatureFlags map[string]any `json:"featureFlags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decideResp); err != nil {
		return false, fmt.Errorf("failed to decode PostHog decide response: %w", err)
	}

	val, exists := decideResp.FeatureFlags[featureName]
	if !exists {
		return false, nil
	}
	if b, ok := val.(bool); ok {
		return b, nil
	}
	if s, ok := val.(string); ok {
		return s == "true", nil
	}
	return false, nil
}

func (p *HTTPPostHogClient) sendPost(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal PostHog payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s%s", p.host, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create PostHog request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PostHog request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PostHog returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}
