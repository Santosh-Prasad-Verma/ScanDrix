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

// SlackChannelAdapter formats and delivers rich Block Kit notifications to Slack webhooks.
type SlackChannelAdapter struct {
	httpClient *http.Client
	defaultURL string
}

// NewSlackChannelAdapter creates a Slack adapter instance.
func NewSlackChannelAdapter(defaultURL string) *SlackChannelAdapter {
	return &SlackChannelAdapter{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		defaultURL: defaultURL,
	}
}

// Channel returns enums.ChannelSlack.
func (a *SlackChannelAdapter) Channel() enums.Channel {
	return enums.ChannelSlack
}

// Deliver formats Block Kit JSON and posts to the designated Slack incoming webhook.
func (a *SlackChannelAdapter) Deliver(ctx context.Context, deliveryCtx contracts.DeliveryContext) error {
	webhookURL := a.defaultURL
	if custom, ok := deliveryCtx.Metadata["slackWebhookUrl"].(string); ok && custom != "" {
		webhookURL = custom
	}
	if webhookURL == "" {
		return fmt.Errorf("no slack webhook URL provided")
	}

	blocks := []map[string]interface{}{
		{
			"type": "header",
			"text": map[string]interface{}{
				"type":  "plain_text",
				"text":  deliveryCtx.Title,
				"emoji": true,
			},
		},
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": deliveryCtx.Body,
			},
		},
		{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Category:* %s  |  *Criticality:* %s  |  *ScanDrix Code Review*", deliveryCtx.Category, deliveryCtx.Criticality),
				},
			},
		},
	}

	if deliveryCtx.CtaURL != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type": "button",
					"text": map[string]interface{}{
						"type":  "plain_text",
						"text":  "View in ScanDrix",
						"emoji": true,
					},
					"url":   deliveryCtx.CtaURL,
					"style": "primary",
				},
			},
		})
	}

	payload := map[string]interface{}{
		"text":   deliveryCtx.Title + ": " + deliveryCtx.Body,
		"blocks": blocks,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
