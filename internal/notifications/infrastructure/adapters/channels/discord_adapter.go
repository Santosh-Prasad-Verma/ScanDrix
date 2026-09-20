package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// DiscordChannelAdapter formats and delivers rich embed notifications to Discord webhooks.
type DiscordChannelAdapter struct {
	httpClient *http.Client
	defaultURL string
}

// NewDiscordChannelAdapter creates a Discord channel adapter.
func NewDiscordChannelAdapter(defaultURL string) *DiscordChannelAdapter {
	return &DiscordChannelAdapter{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		defaultURL: defaultURL,
	}
}

// Channel returns enums.ChannelDiscord.
func (a *DiscordChannelAdapter) Channel() enums.Channel {
	return enums.ChannelDiscord
}

// Deliver formats an Embed payload and posts to the designated Discord webhook.
func (a *DiscordChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	webhookURL := a.defaultURL
	if custom, ok := deliveryCtx.Metadata["discordWebhookUrl"].(string); ok && custom != "" {
		webhookURL = custom
	}
	if webhookURL == "" {
		return fmt.Errorf("no discord webhook URL provided")
	}

	color := 0x3b82f6 // Blue default
	switch deliveryCtx.Criticality {
	case enums.CriticalityCritical:
		color = 0xef4444 // Red
	case enums.CriticalitySystem, enums.CriticalityTransactional:
		color = 0xf59e0b // Amber
	}

	embed := map[string]interface{}{
		"title":       deliveryCtx.Title,
		"description": deliveryCtx.Body,
		"color":       color,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"footer": map[string]interface{}{
			"text": "ScanDrix Code Review",
		},
		"fields": []map[string]interface{}{
			{
				"name":   "Category",
				"value":  deliveryCtx.Category,
				"inline": true,
			},
			{
				"name":   "Criticality",
				"value":  string(deliveryCtx.Criticality),
				"inline": true,
			},
		},
	}

	if deliveryCtx.CtaURL != "" {
		embed["url"] = deliveryCtx.CtaURL
	}

	payload := map[string]interface{}{
		"username": "ScanDrix",
		"embeds":   []map[string]interface{}{embed},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
