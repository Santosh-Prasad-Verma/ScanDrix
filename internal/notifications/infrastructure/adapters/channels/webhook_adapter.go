package channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// WebhookChannelAdapter signs and dispatches raw outbound webhooks to customer endpoints.
type WebhookChannelAdapter struct {
	httpClient    *http.Client
	defaultURL    string
	defaultSecret string
}

// NewWebhookChannelAdapter creates an outbound signed webhook adapter.
func NewWebhookChannelAdapter(defaultURL, defaultSecret string) *WebhookChannelAdapter {
	return &WebhookChannelAdapter{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		defaultURL:    defaultURL,
		defaultSecret: defaultSecret,
	}
}

// Channel returns enums.ChannelWebhook.
func (a *WebhookChannelAdapter) Channel() enums.Channel {
	return enums.ChannelWebhook
}

// Deliver signs the notification event with HMAC-SHA256 and transmits via HTTP POST.
func (a *WebhookChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	endpointURL := a.defaultURL
	if custom, ok := deliveryCtx.Metadata["webhookUrl"].(string); ok && custom != "" {
		endpointURL = custom
	}
	if endpointURL == "" {
		return fmt.Errorf("no target webhook URL provided")
	}

	secret := a.defaultSecret
	if customSecret, ok := deliveryCtx.Metadata["webhookSecret"].(string); ok && customSecret != "" {
		secret = customSecret
	}

	now := time.Now().UTC()
	timestampStr := strconv.FormatInt(now.Unix(), 10)

	payload := map[string]interface{}{
		"id":             deliveryCtx.DeliveryID,
		"event":          deliveryCtx.Event,
		"organizationId": deliveryCtx.OrganizationID,
		"criticality":    deliveryCtx.Criticality,
		"category":       deliveryCtx.Category,
		"title":          deliveryCtx.Title,
		"body":           deliveryCtx.Body,
		"ctaUrl":         deliveryCtx.CtaURL,
		"data":           deliveryCtx.Metadata,
		"correlationId":  deliveryCtx.CorrelationID,
		"timestamp":      now.Format(time.RFC3339),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal outbound webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ScanDrix-Webhooks/1.0")
	req.Header.Set("X-ScanDrix-Event", string(deliveryCtx.Event))
	req.Header.Set("X-ScanDrix-Delivery", deliveryCtx.DeliveryID)
	req.Header.Set("X-ScanDrix-Timestamp", timestampStr)

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestampStr + "."))
		mac.Write(bodyBytes)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-ScanDrix-Signature", "sha256="+signature)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("outbound webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook endpoint responded with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
