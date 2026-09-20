package azuredevops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// AzureVote represents a reviewer vote on an Azure DevOps pull request.
type AzureVote int

const (
	VoteApproved               AzureVote = 10
	VoteApprovedWithSuggestion AzureVote = 5
	VoteNoVote                 AzureVote = 0
	VoteWaitingForAuthor       AzureVote = -5
	VoteRejected               AzureVote = -10
)

// ServiceHookSubscription represents an Azure DevOps service hook webhook subscription.
type ServiceHookSubscription struct {
	ID               string            `json:"id,omitempty"`
	URL              string            `json:"url,omitempty"`
	Status           string            `json:"status,omitempty"`
	PublisherID      string            `json:"publisherId"`
	EventType        string            `json:"eventType"`
	ResourceVersion  string            `json:"resourceVersion,omitempty"`
	ConsumerID       string            `json:"consumerId"`
	ConsumerActionID string            `json:"consumerActionId"`
	PublisherInputs  map[string]string `json:"publisherInputs"`
	ConsumerInputs   map[string]string `json:"consumerInputs"`
	CreatedDate      string            `json:"createdDate,omitempty"`
	ModifiedDate     string            `json:"modifiedDate,omitempty"`
}

// ReviewerVoteRequest represents a payload to submit or update a reviewer's vote on a PR.
type ReviewerVoteRequest struct {
	Vote       AzureVote `json:"vote"`
	IsRequired bool      `json:"isRequired,omitempty"`
	IsFlagged  bool      `json:"isFlagged,omitempty"`
}

// ReviewerVoteResponse represents the reviewer details and recorded vote.
type ReviewerVoteResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	UniqueName  string    `json:"uniqueName"`
	Vote        AzureVote `json:"vote"`
	IsRequired  bool      `json:"isRequired"`
	IsFlagged   bool      `json:"isFlagged"`
	URL         string    `json:"url"`
}

// PRThreadStatusUpdate represents a status update to an existing PR discussion thread.
type PRThreadStatusUpdate struct {
	Status string `json:"status"` // "active", "fixed", "wontFix", "closed", "byDesign", "pending"
}

// ThreadCommentRequest represents a comment to add or update in a PR thread.
type ThreadCommentRequest struct {
	Content     string `json:"content"`
	CommentType int    `json:"commentType,omitempty"` // 1 = text, 2 = codeChange, 3 = system
}

// ListServiceHookSubscriptions retrieves all configured service hook subscriptions in the organization.
func (s *AzureDevOpsAdvancedService) ListServiceHookSubscriptions(ctx context.Context, token, org string, publisherID, eventType string) ([]ServiceHookSubscription, error) {
	endpoint := fmt.Sprintf("%s/%s/_apis/hooks/subscriptions?api-version=7.1", s.baseURL, org)
	if publisherID != "" {
		endpoint += "&publisherId=" + url.QueryEscape(publisherID)
	}
	if eventType != "" {
		endpoint += "&eventType=" + url.QueryEscape(eventType)
	}

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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Value []ServiceHookSubscription `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return payload.Value, nil
}

// CreateServiceHookSubscription creates a new webhook service hook subscription.
func (s *AzureDevOpsAdvancedService) CreateServiceHookSubscription(ctx context.Context, token, org string, sub ServiceHookSubscription) (*ServiceHookSubscription, error) {
	endpoint := fmt.Sprintf("%s/%s/_apis/hooks/subscriptions?api-version=7.1", s.baseURL, org)
	if sub.PublisherID == "" {
		sub.PublisherID = "tfs"
	}
	if sub.ConsumerID == "" {
		sub.ConsumerID = "webHooks"
	}
	if sub.ConsumerActionID == "" {
		sub.ConsumerActionID = "httpRequest"
	}
	if sub.ResourceVersion == "" {
		sub.ResourceVersion = "1.0"
	}

	bodyBytes, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var created ServiceHookSubscription
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &created, nil
}

// DeleteServiceHookSubscription deletes a service hook subscription by ID.
func (s *AzureDevOpsAdvancedService) DeleteServiceHookSubscription(ctx context.Context, token, org string, subscriptionID string) error {
	if subscriptionID == "" {
		return fmt.Errorf("subscriptionID is required")
	}

	endpoint := fmt.Sprintf("%s/%s/_apis/hooks/subscriptions/%s?api-version=7.1", s.baseURL, org, url.PathEscape(subscriptionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SetReviewerVoteWithDetails records or updates a reviewer vote on a pull request with full details.
func (s *AzureDevOpsAdvancedService) SetReviewerVoteWithDetails(ctx context.Context, token, org, project, repoID string, prID int, reviewerID string, voteReq ReviewerVoteRequest) (*ReviewerVoteResponse, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers/%s?api-version=7.1",
		s.baseURL, org, project, repoID, prID, url.PathEscape(reviewerID))

	bodyBytes, err := json.Marshal(voteReq)
	if err != nil {
		return nil, fmt.Errorf("marshal vote payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var result ReviewerVoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// ListReviewers retrieves all reviewers and their current votes for a pull request.
func (s *AzureDevOpsAdvancedService) ListReviewers(ctx context.Context, token, org, project, repoID string, prID int) ([]ReviewerVoteResponse, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers?api-version=7.1",
		s.baseURL, org, project, repoID, prID)

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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Value []ReviewerVoteResponse `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return payload.Value, nil
}

// UpdateThreadStatusExtended updates the resolution status of a PR discussion thread (e.g. "fixed", "closed", "active").
func (s *AzureDevOpsAdvancedService) UpdateThreadStatusExtended(ctx context.Context, token, org, project, repoID string, prID, threadID int, status string) error {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d?api-version=7.1",
		s.baseURL, org, project, repoID, prID, threadID)

	payload := PRThreadStatusUpdate{Status: status}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal status update: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// AddCommentToThread appends a new comment reply to an existing discussion thread.
func (s *AzureDevOpsAdvancedService) AddCommentToThread(ctx context.Context, token, org, project, repoID string, prID, threadID int, content string) (*ThreadCommentEntry, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=7.1",
		s.baseURL, org, project, repoID, prID, threadID)

	payload := ThreadCommentRequest{
		Content:     content,
		CommentType: 1, // Text
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal comment payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		ID            int       `json:"id"`
		Content       string    `json:"content"`
		PublishedDate time.Time `json:"publishedDate"`
		CommentType   string    `json:"commentType"`
		Author        struct {
			DisplayName string `json:"displayName"`
		} `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode comment response: %w", err)
	}

	return &ThreadCommentEntry{
		ID:            raw.ID,
		Content:       raw.Content,
		PublishedDate: raw.PublishedDate,
		CommentType:   raw.CommentType,
		Author:        raw.Author.DisplayName,
	}, nil
}
