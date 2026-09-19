package azuredevops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AzureDevOpsWorkItemsService manages Azure Boards work items linked to code reviews and PRs.
type AzureDevOpsWorkItemsService struct {
	baseURL    string
	httpClient *http.Client
}

// NewAzureDevOpsWorkItemsService creates an instance of AzureDevOpsWorkItemsService.
func NewAzureDevOpsWorkItemsService(httpClient *http.Client, baseURL ...string) *AzureDevOpsWorkItemsService {
	url := "https://dev.azure.com"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &AzureDevOpsWorkItemsService{
		baseURL:    url,
		httpClient: httpClient,
	}
}

// WorkItemDetail models detailed properties of an Azure Boards work item.
type WorkItemDetail struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	WorkItemType string        `json:"workItemType"` // "Bug", "Task", "User Story", "Issue"
	State       string         `json:"state"`        // "New", "Active", "Resolved", "Closed"
	AssignedTo  string         `json:"assignedTo,omitempty"`
	CreatedBy   string         `json:"createdBy"`
	CreatedDate time.Time      `json:"createdDate"`
	URL         string         `json:"url"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// WorkItemUpdatePatch models a JSON Patch operation on a work item.
type WorkItemUpdatePatch struct {
	Op    string `json:"op"` // "add", "replace", "remove", "test"
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// GetWorkItem retrieves a single work item by ID.
func (s *AzureDevOpsWorkItemsService) GetWorkItem(
	ctx context.Context,
	token, org, project string,
	workItemID int,
) (*WorkItemDetail, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/wit/workitems/%d?api-version=7.1-preview.3",
		s.baseURL, org, project, workItemID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		ID     int            `json:"id"`
		URL    string         `json:"url"`
		Fields map[string]any `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode work item: %w", err)
	}

	detail := &WorkItemDetail{
		ID:     raw.ID,
		URL:    raw.URL,
		Fields: raw.Fields,
	}
	if title, ok := raw.Fields["System.Title"].(string); ok {
		detail.Title = title
	}
	if wType, ok := raw.Fields["System.WorkItemType"].(string); ok {
		detail.WorkItemType = wType
	}
	if state, ok := raw.Fields["System.State"].(string); ok {
		detail.State = state
	}
	if assigned, ok := raw.Fields["System.AssignedTo"].(map[string]any); ok {
		detail.AssignedTo, _ = assigned["displayName"].(string)
	}
	if createdBy, ok := raw.Fields["System.CreatedBy"].(map[string]any); ok {
		detail.CreatedBy, _ = createdBy["displayName"].(string)
	}
	if createdDateStr, ok := raw.Fields["System.CreatedDate"].(string); ok {
		detail.CreatedDate, _ = time.Parse(time.RFC3339, createdDateStr)
	}
	return detail, nil
}

// UpdateWorkItem updates work item fields using JSON Patch.
func (s *AzureDevOpsWorkItemsService) UpdateWorkItem(
	ctx context.Context,
	token, org, project string,
	workItemID int,
	patches []WorkItemUpdatePatch,
) (*WorkItemDetail, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/wit/workitems/%d?api-version=7.1-preview.3",
		s.baseURL, org, project, workItemID)

	bodyBytes, err := json.Marshal(patches)
	if err != nil {
		return nil, fmt.Errorf("marshal patch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json-patch+json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		ID     int            `json:"id"`
		URL    string         `json:"url"`
		Fields map[string]any `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode work item: %w", err)
	}

	detail := &WorkItemDetail{
		ID:     raw.ID,
		URL:    raw.URL,
		Fields: raw.Fields,
	}
	if title, ok := raw.Fields["System.Title"].(string); ok {
		detail.Title = title
	}
	if state, ok := raw.Fields["System.State"].(string); ok {
		detail.State = state
	}
	return detail, nil
}

// ExtractWorkItemIDs parses commit messages or PR descriptions for `#123` work item references.
func ExtractWorkItemIDs(text string) []int {
	var ids []int
	words := strings.Fields(text)
	for _, w := range words {
		cleaned := strings.TrimLeft(w, "([{<\"'")
		if strings.HasPrefix(cleaned, "#") {
			numStr := strings.TrimPrefix(cleaned, "#")
			numStr = strings.TrimRight(numStr, ",.:;!?)]}>\"'")
			if id, err := strconv.Atoi(numStr); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
