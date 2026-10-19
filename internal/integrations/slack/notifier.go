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
	return n.PostInteractiveReviewSummary(ctx, repoName, prNumber, prTitle, "", findings, "")
}

// PostInteractiveReviewSummary formats findings into an interactive Slack Block Kit card with action buttons.
func (n *Notifier) PostInteractiveReviewSummary(
	ctx context.Context,
	repoName string,
	prNumber int,
	prTitle string,
	dashboardURL string,
	findings []models.CodeFinding,
	workspaceID string,
) error {
	if n.webhookURL == "" {
		return nil // No-op if not configured
	}

	payload := BuildInteractiveReviewBlocks(repoName, prNumber, prTitle, dashboardURL, findings, workspaceID)

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
