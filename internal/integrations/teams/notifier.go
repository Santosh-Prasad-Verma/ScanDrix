package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// Notifier sends Adaptive Cards to Microsoft Teams channels.
type Notifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewNotifier initializes the Teams webhook client.
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PostReviewSummary sends an Adaptive Card summarizing pull request review findings.
func (n *Notifier) PostReviewSummary(ctx context.Context, repoName string, prNumber int, prTitle string, findings []models.CodeFinding) error {
	if n.webhookURL == "" {
		return nil
	}

	themeColor := "22c55e" // Green
	if len(findings) > 0 {
		themeColor = "ef4444" // Red
	}

	payload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    fmt.Sprintf("Scandrix Review Completed for %s #%d", repoName, prNumber),
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("Scandrix Code Review - %s #%d", repoName, prNumber),
				"activitySubtitle": prTitle,
				"facts": []map[string]string{
					{"name": "Total Findings", "value": fmt.Sprintf("%d", len(findings))},
					{"name": "Status", "value": "Completed"},
				},
				"markdown": true,
			},
		},
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
		return fmt.Errorf("failed sending teams card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("teams webhook failed with status: %d", resp.StatusCode)
	}

	return nil
}
