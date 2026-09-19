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
	resendAPIEndpoint = "https://api.resend.com/events"
	resendTimeout     = 5 * time.Second
)

// ResendClient defines the interface for lifecycle transactional email events.
type ResendClient interface {
	IsEnabled() bool
	Send(ctx context.Context, event string, email string, payload map[string]any) error
}

// HTTPResendClient implements ResendClient via the Resend API.
type HTTPResendClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

// NewResendClient initializes Resend provider from environment variables.
func NewResendClient() *HTTPResendClient {
	apiKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	return &HTTPResendClient{
		apiKey:   apiKey,
		endpoint: resendAPIEndpoint,
		httpClient: &http.Client{
			Timeout: resendTimeout,
		},
	}
}

// NewResendClientWithConfig creates a Resend client with custom configuration.
func NewResendClientWithConfig(apiKey, endpoint string, httpClient *http.Client) *HTTPResendClient {
	if endpoint == "" {
		endpoint = resendAPIEndpoint
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: resendTimeout}
	}
	return &HTTPResendClient{
		apiKey:     apiKey,
		endpoint:   endpoint,
		httpClient: httpClient,
	}
}

func (r *HTTPResendClient) IsEnabled() bool {
	return r.apiKey != ""
}

// Send emits a lifecycle event to Resend for transactional customer communication.
func (r *HTTPResendClient) Send(ctx context.Context, event string, email string, payload map[string]any) error {
	if !r.IsEnabled() {
		return nil
	}

	body := map[string]any{
		"event":   event,
		"email":   email,
		"payload": payload,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal Resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create Resend request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Resend returned non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}
