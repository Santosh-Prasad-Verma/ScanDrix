package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// Notifier dispatches rich embed review summary cards to Discord webhooks.
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewNotifier initializes the Discord webhook notifier.
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetHTTPClient allows injecting a test HTTP client.
func (n *Notifier) SetHTTPClient(c *http.Client) {
	n.httpClient = c
}

// severityColor maps finding severity levels to Discord embed colors (hex as int).
func severityColor(criticalCount, highCount int) int {
	if criticalCount > 0 {
		return 0xED4245 // Red
	}
	if highCount > 0 {
		return 0xFEE75C // Yellow
	}
	return 0x57F287 // Green
}

// PostReviewSummary sends a code review summary as a Discord embed via webhook.
func (n *Notifier) PostReviewSummary(ctx context.Context, repoName string, prNumber int, prTitle string, findings []models.CodeFinding) error {
	if n.webhookURL == "" {
		return nil // No-op if not configured
	}

	criticalCount := 0
	highCount := 0
	for _, f := range findings {
		if f.Severity == models.SeverityCritical {
			criticalCount++
		} else if f.Severity == models.SeverityHigh {
			highCount++
		}
	}

	statusEmoji := "✅"
	verdict := "Review Passed — No High/Critical Defects"
	if criticalCount > 0 {
		statusEmoji = "🚨"
		verdict = fmt.Sprintf("Action Required: %d Critical Finding(s)", criticalCount)
	} else if highCount > 0 {
		statusEmoji = "⚠️"
		verdict = fmt.Sprintf("Warning: %d High Severity Finding(s)", highCount)
	}

	embed := map[string]any{
		"title":       fmt.Sprintf("%s ScanDrix Code Review Completed", statusEmoji),
		"description": verdict,
		"color":       severityColor(criticalCount, highCount),
		"fields": []map[string]any{
			{
				"name":   "Repository",
				"value":  fmt.Sprintf("`%s`", repoName),
				"inline": true,
			},
			{
				"name":   "Pull Request",
				"value":  fmt.Sprintf("#%d — %s", prNumber, prTitle),
				"inline": true,
			},
			{
				"name":   "Total Findings",
				"value":  fmt.Sprintf("%d", len(findings)),
				"inline": true,
			},
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"footer": map[string]string{
			"text": "ScanDrix Code Review Engine",
		},
	}

	// Add top findings as fields (max 5 to stay within Discord embed limits)
	maxFindings := 5
	if len(findings) < maxFindings {
		maxFindings = len(findings)
	}
	for i := 0; i < maxFindings; i++ {
		f := findings[i]
		severityBadge := "🟢"
		switch f.Severity {
		case models.SeverityCritical:
			severityBadge = "🔴"
		case models.SeverityHigh:
			severityBadge = "🟠"
		case models.SeverityMedium:
			severityBadge = "🟡"
		}

		fieldValue := fmt.Sprintf("%s **%s** — `%s:%d`", severityBadge, f.Severity, f.FilePath, f.StartLine)
		if f.Title != "" {
			fieldValue += "\n" + truncate(f.Title, 100)
		}

		embed["fields"] = append(embed["fields"].([]map[string]any), map[string]any{
			"name":   fmt.Sprintf("Finding #%d", i+1),
			"value":  fieldValue,
			"inline": false,
		})
	}

	payload := map[string]any{
		"content": nil, // Webhook with embed only
		"embeds":  []map[string]any{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed sending discord notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook error: status %d", resp.StatusCode)
	}

	return nil
}

// truncate limits a string to maxLen characters and appends "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
