package slack

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// BlockActionValue defines the structured JSON payload stored inside Slack button values.
type BlockActionValue struct {
	Action      string `json:"action"`
	Repo        string `json:"repo"`
	PRNumber    int    `json:"pr_number"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	FindingID   string `json:"finding_id,omitempty"`
}

// BuildInteractiveReviewBlocks generates a rich Block Kit payload with interactive action buttons.
func BuildInteractiveReviewBlocks(
	repoName string,
	prNumber int,
	prTitle string,
	dashboardURL string,
	findings []models.CodeFinding,
	workspaceID string,
) map[string]any {
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

	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]any{
				"type":  "plain_text",
				"text":  fmt.Sprintf("%s ScanDrix Code Review Completed", statusEmoji),
				"emoji": true,
			},
		},
		{
			"type": "section",
			"fields": []map[string]any{
				{"type": "mrkdwn", "text": fmt.Sprintf("*Repository:*\n`%s`", repoName)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Pull Request:*\n#%d — %s", prNumber, prTitle)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Total Findings:*\n%d", len(findings))},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Verdict:*\n*%s*", verdict)},
			},
		},
	}

	// Add top findings summary (up to 3 items)
	if len(findings) > 0 {
		var findingsList string
		limit := 3
		if len(findings) < limit {
			limit = len(findings)
		}
		for i := 0; i < limit; i++ {
			f := findings[i]
			icon := "⚠️"
			if f.Severity == models.SeverityCritical {
				icon = "🚨"
			}
			findingsList += fmt.Sprintf("• %s *[%s]* %s (`%s:%d`)\n", icon, f.Severity, f.Title, f.FilePath, f.StartLine)
		}
		if len(findings) > limit {
			findingsList += fmt.Sprintf("_...and %d more finding(s) in ScanDrix Studio._\n", len(findings)-limit)
		}

		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Key Security & Quality Findings:*\n%s", findingsList),
			},
		})
	}

	// Interactive Action Buttons Block
	approveVal, _ := json.Marshal(BlockActionValue{Action: "approve_pr", Repo: repoName, PRNumber: prNumber, WorkspaceID: workspaceID})
	requestChangesVal, _ := json.Marshal(BlockActionValue{Action: "request_changes", Repo: repoName, PRNumber: prNumber, WorkspaceID: workspaceID})

	elements := []map[string]any{
		{
			"type":     "button",
			"text":     map[string]any{"type": "plain_text", "text": "✅ Approve PR", "emoji": true},
			"style":    "primary",
			"action_id": "approve_pr",
			"value":    string(approveVal),
		},
		{
			"type":     "button",
			"text":     map[string]any{"type": "plain_text", "text": "🛑 Request Changes", "emoji": true},
			"style":    "danger",
			"action_id": "request_changes",
			"value":    string(requestChangesVal),
		},
	}

	if dashboardURL != "" {
		elements = append(elements, map[string]any{
			"type": "button",
			"text": map[string]any{"type": "plain_text", "text": "🔍 View in ScanDrix", "emoji": true},
			"url":  dashboardURL,
		})
	}

	blocks = append(blocks, map[string]any{
		"type":     "actions",
		"block_id": "pr_review_actions",
		"elements": elements,
	})

	blocks = append(blocks, map[string]any{
		"type": "context",
		"elements": []map[string]any{
			{
				"type": "mrkdwn",
				"text": fmt.Sprintf("ScanDrix AI Engine · Evaluated at %s", time.Now().UTC().Format("15:04:05 MST")),
			},
		},
	})

	return map[string]any{
		"text":   fmt.Sprintf("ScanDrix Review: %s #%d — %s", repoName, prNumber, verdict),
		"blocks": blocks,
	}
}

// BuildResolutionResponse generates an in-place updated message confirming the action taken.
func BuildResolutionResponse(
	repoName string,
	prNumber int,
	actionName string,
	actorUsername string,
) map[string]any {
	bannerEmoji := "✅"
	actionText := "APPROVED"
	if actionName == "request_changes" {
		bannerEmoji = "🛑"
		actionText = "CHANGES REQUESTED"
	} else if actionName == "resolve_finding" {
		bannerEmoji = "🎯"
		actionText = "RESOLVED FINDING"
	}

	return map[string]any{
		"replace_original": true,
		"text":             fmt.Sprintf("%s PR #%d was %s by @%s via ScanDrix Bot.", bannerEmoji, prNumber, actionText, actorUsername),
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]any{
					"type":  "plain_text",
					"text":  fmt.Sprintf("%s Pull Request #%d %s", bannerEmoji, prNumber, actionText),
					"emoji": true,
				},
			},
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Action:* `%s`\n*Repository:* `%s`\n*Executed by:* `@%s`\n*Timestamp:* `%s`", actionText, repoName, actorUsername, time.Now().UTC().Format(time.RFC1123)),
				},
			},
			{
				"type": "context",
				"elements": []map[string]any{
					{
						"type": "mrkdwn",
						"text": "Confirmed and recorded in ScanDrix Outbox Relay.",
					},
				},
			},
		},
	}
}
