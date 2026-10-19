package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// InteractionPayload models Slack's block_actions payload.
type InteractionPayload struct {
	Type        string           `json:"type"`
	ResponseURL string           `json:"response_url"`
	TriggerID   string           `json:"trigger_id"`
	User        SlackUser        `json:"user"`
	Channel     SlackChannel     `json:"channel"`
	Team        SlackTeam        `json:"team"`
	Actions     []SlackActionItem `json:"actions"`
}

type SlackUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type SlackChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SlackTeam struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type SlackActionItem struct {
	ActionID string `json:"action_id"`
	BlockID  string `json:"block_id"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

// ParseInteractionPayload extracts the InteractionPayload from either a form payload or raw JSON body.
func ParseInteractionPayload(r *http.Request, rawBody []byte) (*InteractionPayload, error) {
	var payloadStr string

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		vals, err := url.ParseQuery(string(rawBody))
		if err == nil && vals.Get("payload") != "" {
			payloadStr = vals.Get("payload")
		}
	}

	if payloadStr == "" {
		bodyStr := string(rawBody)
		if strings.HasPrefix(bodyStr, "payload=") {
			trimmed := strings.TrimPrefix(bodyStr, "payload=")
			if unescaped, err := url.QueryUnescape(trimmed); err == nil {
				payloadStr = unescaped
			} else {
				payloadStr = trimmed
			}
		} else {
			payloadStr = bodyStr
		}
	}

	var payload InteractionPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, fmt.Errorf("failed to decode slack interaction payload: %w", err)
	}

	return &payload, nil
}

// RespondToSlackURL sends an asynchronous update or ephemeral message back to Slack via response_url.
func RespondToSlackURL(ctx context.Context, responseURL string, message map[string]any) error {
	if responseURL == "" {
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal response message: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responseURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create response request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed posting to slack response_url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack response_url returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
