package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

var jiraKeyRegex = regexp.MustCompile(`([A-Z][A-Z0-9]+-\d+)`)

// Client manages communication with Jira REST API v3.
type Client struct {
	baseURL    string
	username   string
	apiToken   string
	httpClient *http.Client
}

// NewClient initializes the Jira integration client.
func NewClient(baseURL, username, apiToken string) *Client {
	return &Client{
		baseURL:    baseURL,
		username:   username,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ExtractIssueKeys parses Jira ticket identifiers from PR titles or branch names.
func ExtractIssueKeys(text string) []string {
	matches := jiraKeyRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	unique := make(map[string]bool)
	res := make([]string, 0, len(matches))
	for _, m := range matches {
		if !unique[m] {
			unique[m] = true
			res = append(res, m)
		}
	}
	return res
}

// CreateFindingIssue generates a Jira task from a discovered code finding.
func (c *Client) CreateFindingIssue(ctx context.Context, projectKey string, finding models.CodeFinding) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("jira baseURL not configured")
	}

	payload := map[string]any{
		"fields": map[string]any{
			"project": map[string]string{"key": projectKey},
			"summary": fmt.Sprintf("[%s] %s in %s", finding.Severity, finding.Title, finding.FilePath),
			"description": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]string{
								"type": "text",
								"text": fmt.Sprintf("File: %s (Line %d)\n\nDescription:\n%s\n\nRemediation:\n%s",
									finding.FilePath, finding.StartLine, finding.Description, finding.Remediation),
							},
						},
					},
				},
			},
			"issuetype": map[string]string{"name": "Bug"},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rest/api/3/issue", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(c.username, c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed executing jira issue request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("jira issue creation failed with status: %d", resp.StatusCode)
	}

	var created struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", err
	}

	return created.Key, nil
}
