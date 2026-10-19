package teams

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// BuildInteractiveTeamsCard builds a Microsoft Teams Adaptive Card v1.5 with interactive action buttons.
func BuildInteractiveTeamsCard(
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

	accentColor := "Good" // Green
	statusEmoji := "✅"
	verdict := "Review Passed — No High/Critical Defects"
	if criticalCount > 0 {
		accentColor = "Attention" // Red
		statusEmoji = "🚨"
		verdict = fmt.Sprintf("Action Required: %d Critical Finding(s)", criticalCount)
	} else if highCount > 0 {
		accentColor = "Warning" // Amber
		statusEmoji = "⚠️"
		verdict = fmt.Sprintf("Warning: %d High Severity Finding(s)", highCount)
	}

	facts := []map[string]string{
		{"title": "Repository", "value": repoName},
		{"title": "Pull Request", "value": fmt.Sprintf("#%d — %s", prNumber, prTitle)},
		{"title": "Total Findings", "value": fmt.Sprintf("%d", len(findings))},
		{"title": "Verdict", "value": verdict},
	}

	body := []map[string]any{
		{
			"type":   "TextBlock",
			"size":   "Medium",
			"weight": "Bolder",
			"color":  accentColor,
			"text":   fmt.Sprintf("%s ScanDrix Code Review Completed", statusEmoji),
		},
		{
			"type":  "FactSet",
			"facts": facts,
		},
	}

	if len(findings) > 0 {
		var listText string
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
			listText += fmt.Sprintf("- %s **[%s]** %s (`%s:%d`)\n", icon, f.Severity, f.Title, f.FilePath, f.StartLine)
		}
		body = append(body, map[string]any{
			"type": "TextBlock",
			"text": "**Key Findings:**\n" + listText,
			"wrap": true,
		})
	}

	actions := []map[string]any{
		{
			"type":  "Action.Execute",
			"title": "✅ Approve PR",
			"verb":  "approve_pr",
			"data": map[string]any{
				"action":       "approve_pr",
				"repo":         repoName,
				"pr_number":    prNumber,
				"workspace_id": workspaceID,
			},
		},
		{
			"type":  "Action.Execute",
			"title": "🛑 Request Changes",
			"verb":  "request_changes",
			"data": map[string]any{
				"action":       "request_changes",
				"repo":         repoName,
				"pr_number":    prNumber,
				"workspace_id": workspaceID,
			},
		},
	}

	if dashboardURL != "" {
		actions = append(actions, map[string]any{
			"type":  "Action.OpenUrl",
			"title": "🔍 View in ScanDrix",
			"url":   dashboardURL,
		})
	}

	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.5",
					"body":    body,
					"actions": actions,
				},
			},
		},
	}
}

// BuildResolutionCard returns an updated Adaptive Card confirming action execution.
func BuildResolutionCard(
	repoName string,
	prNumber int,
	actionName string,
	actorUsername string,
) map[string]any {
	bannerEmoji := "✅"
	actionText := "APPROVED"
	accentColor := "Good"
	if actionName == "request_changes" {
		bannerEmoji = "🛑"
		actionText = "CHANGES REQUESTED"
		accentColor = "Attention"
	}

	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.5",
					"body": []map[string]any{
						{
							"type":   "TextBlock",
							"size":   "Medium",
							"weight": "Bolder",
							"color":  accentColor,
							"text":   fmt.Sprintf("%s Pull Request #%d %s", bannerEmoji, prNumber, actionText),
						},
						{
							"type":  "FactSet",
							"facts": []map[string]string{
								{"title": "Action", "value": actionText},
								{"title": "Repository", "value": repoName},
								{"title": "Executed By", "value": actorUsername},
								{"title": "Timestamp", "value": time.Now().UTC().Format(time.RFC1123)},
							},
						},
					},
				},
			},
		},
	}
}
