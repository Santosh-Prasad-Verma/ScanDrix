package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// Client coordinates with Linear GraphQL API.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient initializes the Linear API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// CreateIssue creates a Linear issue from a finding.
func (c *Client) CreateIssue(ctx context.Context, teamID string, finding models.CodeFinding) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("linear api key not configured")
	}

	mutation := `mutation IssueCreate($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue {
				id
				identifier
				url
			}
		}
	}`

	title := fmt.Sprintf("[%s] %s", finding.Severity, finding.Title)
	description := fmt.Sprintf("**File:** `%s:%d`\n\n**Severity:** %s\n\n### Description\n%s\n\n### Remediation\n```\n%s\n```",
		finding.FilePath, finding.StartLine, finding.Severity, finding.Description, finding.Remediation,
	)

	priority := 3 // Normal
	if finding.Severity == models.SeverityCritical {
		priority = 1 // Urgent
	} else if finding.Severity == models.SeverityHigh {
		priority = 2 // High
	}

	variables := map[string]any{
		"input": map[string]any{
			"teamId":      teamID,
			"title":       title,
			"description": description,
			"priority":    priority,
		},
	}

	requestPayload := map[string]any{
		"query":     mutation,
		"variables": variables,
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("linear graphql request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linear api failed with status: %d", resp.StatusCode)
	}

	var resData struct {
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					Identifier string `json:"identifier"`
					URL        string `json:"url"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return "", err
	}

	if !resData.Data.IssueCreate.Success {
		return "", fmt.Errorf("linear failed to create issue")
	}

	return resData.Data.IssueCreate.Issue.Identifier, nil
}
