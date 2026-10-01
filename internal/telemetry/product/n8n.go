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
	n8nTimeout = 5 * time.Second
)

// N8nClient defines the contract for lifecycle webhook forwarding.
type N8nClient interface {
	IsEnabled() bool
	Notify(ctx context.Context, eventID string, props map[string]any) error
}

// HTTPN8nClient implements N8nClient via webhook delivery with retries.
type HTTPN8nClient struct {
	webhookURL string
	httpClient *http.Client
}

// NewN8nClient initializes the n8n provider from environment variables.
func NewN8nClient() *HTTPN8nClient {
	url := strings.TrimSpace(os.Getenv("N8N_WEBHOOK_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("API_SIGNUP_NOTIFICATION_WEBHOOK"))
	}
	return &HTTPN8nClient{
		webhookURL: url,
		httpClient: &http.Client{
			Timeout: n8nTimeout,
		},
	}
}

// NewN8nClientWithConfig creates an n8n client with custom configuration.
func NewN8nClientWithConfig(webhookURL string, httpClient *http.Client) *HTTPN8nClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: n8nTimeout}
	}
	return &HTTPN8nClient{
		webhookURL: webhookURL,
		httpClient: httpClient,
	}
}

func (n *HTTPN8nClient) IsEnabled() bool {
	return n.webhookURL != ""
}

// Notify delivers a product event to the n8n webhook with up to 2 attempts.
func (n *HTTPN8nClient) Notify(ctx context.Context, eventID string, props map[string]any) error {
	if !n.IsEnabled() {
		return nil
	}

	payload := map[string]any{
		"eventId":   eventID,
		"props":     props,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal n8n webhook payload: %w", err)
	}

	const maxAttempts = 2
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to create n8n request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := n.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("n8n webhook returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		if attempt < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	return lastErr
}
