package pm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LinearAdapter implements PMAdapter for Linear's GraphQL API.
type LinearAdapter struct {
	endpoint   string
	apiToken   string
	httpClient *http.Client
}

// NewLinearAdapter creates an authenticated Linear GraphQL client.
func NewLinearAdapter(apiToken string) *LinearAdapter {
	return &LinearAdapter{
		endpoint: "https://api.linear.app/graphql",
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (l *LinearAdapter) Platform() PMPlatform {
	return PlatformLinear
}

func (l *LinearAdapter) CreateIssue(ctx context.Context, req IssueCreationRequest) (*PMIssue, error) {
	mutation := `
		mutation CreateIssue($teamId: String!, $title: String!, $description: String!) {
			issueCreate(input: { teamId: $teamId, title: $title, description: $description }) {
				success
				issue {
					id
					identifier
					title
					url
					state {
						name
					}
				}
			}
		}
	`

	payload := map[string]any{
		"query": mutation,
		"variables": map[string]any{
			"teamId":      req.ProjectKey,
			"title":       req.Title,
			"description": req.Description,
		},
	}

	respBody, err := l.doGraphQL(ctx, payload)
	if err != nil {
		return nil, err
	}

	var data struct {
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					URL        string `json:"url"`
					State      struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, err
	}

	issue := data.Data.IssueCreate.Issue
	return &PMIssue{
		Key:       issue.Identifier,
		ID:        issue.ID,
		Platform:  PlatformLinear,
		Title:     issue.Title,
		Status:    issue.State.Name,
		URL:       issue.URL,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (l *LinearAdapter) GetIssue(ctx context.Context, issueKey string) (*PMIssue, error) {
	query := `
		query GetIssue($id: String!) {
			issue(id: $id) {
				id
				identifier
				title
				url
				state {
					name
				}
			}
		}
	`

	payload := map[string]any{
		"query": query,
		"variables": map[string]any{
			"id": issueKey,
		},
	}

	respBody, err := l.doGraphQL(ctx, payload)
	if err != nil {
		return nil, err
	}

	var data struct {
		Data struct {
			Issue struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				URL        string `json:"url"`
				State      struct {
					Name string `json:"name"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, err
	}

	issue := data.Data.Issue
	return &PMIssue{
		Key:       issue.Identifier,
		ID:        issue.ID,
		Platform:  PlatformLinear,
		Title:     issue.Title,
		Status:    issue.State.Name,
		URL:       issue.URL,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (l *LinearAdapter) AddComment(ctx context.Context, issueKey, comment string) error {
	mutation := `
		mutation AddComment($issueId: String!, $body: String!) {
			commentCreate(input: { issueId: $issueId, body: $body }) {
				success
			}
		}
	`

	payload := map[string]any{
		"query": mutation,
		"variables": map[string]any{
			"issueId": issueKey,
			"body":    comment,
		},
	}

	_, err := l.doGraphQL(ctx, payload)
	return err
}

func (l *LinearAdapter) LinkPR(ctx context.Context, issueKey, prURL string) error {
	mutation := `
		mutation LinkPR($issueId: String!, $url: String!, $title: String!) {
			attachmentCreate(input: { issueId: $issueId, url: $url, title: $title }) {
				success
			}
		}
	`

	payload := map[string]any{
		"query": mutation,
		"variables": map[string]any{
			"issueId": issueKey,
			"url":     prURL,
			"title":   "Pull Request: ScanDrix Code Review",
		},
	}

	_, err := l.doGraphQL(ctx, payload)
	return err
}

func (l *LinearAdapter) doGraphQL(ctx context.Context, payload map[string]any) ([]byte, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", l.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("linear graphql returned status: %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}
