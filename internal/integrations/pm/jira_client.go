package pm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JiraAdapter implements PMAdapter for Jira Cloud REST API v3.
type JiraAdapter struct {
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
}

// NewJiraAdapter creates an authenticated Jira API client.
func NewJiraAdapter(baseURL, email, apiToken string) *JiraAdapter {
	return &JiraAdapter{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (j *JiraAdapter) Platform() PMPlatform {
	return PlatformJira
}

func (j *JiraAdapter) CreateIssue(ctx context.Context, req IssueCreationRequest) (*PMIssue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue", j.baseURL)

	payload := map[string]any{
		"fields": map[string]any{
			"project": map[string]string{
				"key": req.ProjectKey,
			},
			"summary": req.Title,
			"description": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []map[string]any{
					{
						"type": "paragraph",
						"content": []map[string]any{
							{"type": "text", "text": req.Description},
						},
					},
				},
			},
			"issuetype": map[string]string{
				"name": req.IssueType,
			},
			"labels": []string{"scandrix-finding", strings.ToLower(string(req.Severity))},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	j.setHeaders(httpReq)

	resp, err := j.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira create issue returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &PMIssue{
		Key:       data.Key,
		ID:        data.ID,
		Platform:  PlatformJira,
		Title:     req.Title,
		Status:    "OPEN",
		URL:       fmt.Sprintf("%s/browse/%s", j.baseURL, data.Key),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (j *JiraAdapter) GetIssue(ctx context.Context, issueKey string) (*PMIssue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", j.baseURL, issueKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	j.setHeaders(req)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira get issue returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &PMIssue{
		Key:       data.Key,
		ID:        data.ID,
		Platform:  PlatformJira,
		Title:     data.Fields.Summary,
		Status:    data.Fields.Status.Name,
		URL:       fmt.Sprintf("%s/browse/%s", j.baseURL, data.Key),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (j *JiraAdapter) AddComment(ctx context.Context, issueKey, comment string) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", j.baseURL, issueKey)
	payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": comment},
					},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	j.setHeaders(req)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira add comment returned: %d", resp.StatusCode)
	}
	return nil
}

func (j *JiraAdapter) LinkPR(ctx context.Context, issueKey, prURL string) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/remotelink", j.baseURL, issueKey)
	payload := map[string]any{
		"object": map[string]string{
			"url":   prURL,
			"title": "Pull Request: ScanDrix Code Review",
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	j.setHeaders(req)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira link pr returned: %d", resp.StatusCode)
	}
	return nil
}

func (j *JiraAdapter) setHeaders(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(j.email + ":" + j.apiToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
