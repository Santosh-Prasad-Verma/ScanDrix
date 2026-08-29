package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// Notifier dispatches structured review summary cards to Slack.
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewNotifier initializes the Slack webhook notifier.
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PostReviewSummary formats findings and review metrics into Slack Block Kit cards.
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
	verdict := "Review Passed - No High/Critical Defects"
	if criticalCount > 0 {
		statusEmoji = "🚨"
		verdict = fmt.Sprintf("Action Required: %d Critical Finding(s)", criticalCount)
	} else if highCount > 0 {
		statusEmoji = "⚠️"
		verdict = fmt.Sprintf("Warning: %d High Severity Finding(s)", highCount)
	}

	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]string{
				"type": "plain_text",
				"text": fmt.Sprintf("%s Scandrix Code Review Completed", statusEmoji),
			},
		},
		{
			"type": "section",
			"fields": []map[string]string{
				{"type": "mrkdwn", "text": fmt.Sprintf("*Repository:*\n%s", repoName)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Pull Request:*\n#%d - %s", prNumber, prTitle)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Total Findings:*\n%d", len(findings))},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Verdict:*\n%s", verdict)},
			},
		},
	}

	payload := map[string]any{
		"text":   fmt.Sprintf("Scandrix Review finished for %s #%d: %s", repoName, prNumber, verdict),
		"blocks": blocks,
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
		return fmt.Errorf("failed sending slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook error: status %d", resp.StatusCode)
	}

	return nil
}
