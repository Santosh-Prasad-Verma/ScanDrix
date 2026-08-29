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

// AzureBoardsAdapter implements PMAdapter for Azure DevOps Work Item Tracking REST API v7.1.
type AzureBoardsAdapter struct {
	baseURL      string
	organization string
	patToken     string
	httpClient   *http.Client
}

// NewAzureBoardsAdapter creates an authenticated Azure Boards client.
func NewAzureBoardsAdapter(baseURL, organization, patToken string) *AzureBoardsAdapter {
	if baseURL == "" {
		baseURL = "https://dev.azure.com"
	}
	return &AzureBoardsAdapter{
		baseURL:      strings.TrimRight(baseURL, "/"),
		organization: organization,
		patToken:     patToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (a *AzureBoardsAdapter) Platform() PMPlatform {
	return PlatformAzureBoards
}

func (a *AzureBoardsAdapter) CreateIssue(ctx context.Context, req IssueCreationRequest) (*PMIssue, error) {
	issueType := req.IssueType
	if issueType == "" {
		issueType = "Bug"
	}

	url := fmt.Sprintf("%s/%s/%s/_apis/wit/workitems/$%s?api-version=7.1-preview.3",
		a.baseURL, a.organization, req.ProjectKey, issueType)

	// Azure Boards uses RFC 6902 JSON Patch format
	patchOps := []map[string]any{
		{
			"op":    "add",
			"path":  "/fields/System.Title",
			"value": req.Title,
		},
		{
			"op":    "add",
			"path":  "/fields/System.Description",
			"value": req.Description,
		},
		{
			"op":    "add",
			"path":  "/fields/System.Tags",
			"value": fmt.Sprintf("scandrix; %s", strings.ToLower(string(req.Severity))),
		},
	}

	bodyBytes, _ := json.Marshal(patchOps)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	a.setHeaders(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("azure boards create work item returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     int    `json:"id"`
		URL    string `json:"url"`
		Fields struct {
			Title string `json:"System.Title"`
			State string `json:"System.State"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	workID := fmt.Sprintf("AB#%d", data.ID)
	webURL := fmt.Sprintf("%s/%s/%s/_workitems/edit/%d", a.baseURL, a.organization, req.ProjectKey, data.ID)

	return &PMIssue{
		Key:       workID,
		ID:        fmt.Sprintf("%d", data.ID),
		Platform:  PlatformAzureBoards,
		Title:     data.Fields.Title,
		Status:    data.Fields.State,
		URL:       webURL,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (a *AzureBoardsAdapter) GetIssue(ctx context.Context, issueKey string) (*PMIssue, error) {
	cleanID := strings.TrimPrefix(issueKey, "AB#")
	url := fmt.Sprintf("%s/%s/_apis/wit/workitems/%s?api-version=7.1-preview.3",
		a.baseURL, a.organization, cleanID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure boards get work item returned status: %d", resp.StatusCode)
	}

	var data struct {
		ID     int `json:"id"`
		Fields struct {
			Title string `json:"System.Title"`
			State string `json:"System.State"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &PMIssue{
		Key:       fmt.Sprintf("AB#%d", data.ID),
		ID:        fmt.Sprintf("%d", data.ID),
		Platform:  PlatformAzureBoards,
		Title:     data.Fields.Title,
		Status:    data.Fields.State,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (a *AzureBoardsAdapter) AddComment(ctx context.Context, issueKey, comment string) error {
	cleanID := strings.TrimPrefix(issueKey, "AB#")
	url := fmt.Sprintf("%s/%s/_apis/wit/workItems/%s/comments?api-version=7.1-preview.3",
		a.baseURL, a.organization, cleanID)

	payload := map[string]string{
		"text": comment,
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	a.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("azure boards add comment returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *AzureBoardsAdapter) LinkPR(ctx context.Context, issueKey, prURL string) error {
	cleanID := strings.TrimPrefix(issueKey, "AB#")
	url := fmt.Sprintf("%s/%s/_apis/wit/workitems/%s?api-version=7.1-preview.3",
		a.baseURL, a.organization, cleanID)

	patchOps := []map[string]any{
		{
			"op":   "add",
			"path": "/relations/-",
			"value": map[string]any{
				"rel": "Hyperlink",
				"url": prURL,
				"attributes": map[string]string{
					"comment": "ScanDrix Pull Request",
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(patchOps)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("azure boards link pr returned: %d", resp.StatusCode)
	}
	return nil
}

func (a *AzureBoardsAdapter) setHeaders(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(":" + a.patToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json-patch+json")
	req.Header.Set("Accept", "application/json")
}
