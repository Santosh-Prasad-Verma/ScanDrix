package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ReviewCommentPayload models an inline diff comment posted to a GitHub pull request review.
type ReviewCommentPayload struct {
	Path      string `json:"path"`
	Position  int    `json:"position,omitempty"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"` // "RIGHT" for additions
	Body      string `json:"body"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

// PullReviewSubmission models the GitHub API payload for submitting an overall review.
type PullReviewSubmission struct {
	CommitID string                 `json:"commit_id,omitempty"`
	Body     string                 `json:"body"`
	Event    string                 `json:"event"` // "COMMENT", "APPROVE", "REQUEST_CHANGES"
	Comments []ReviewCommentPayload `json:"comments,omitempty"`
}

// Client interacts with GitHub REST API v3 to retrieve diffs and publish automated reviews.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new GitHub API client instance.
func NewClient(token string) *Client {
	return &Client{
		baseURL: "https://api.github.com",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchPullRequestDiff retrieves the unified raw git diff for a specific pull request.
func (c *Client) FetchPullRequestDiff(ctx context.Context, owner, repo string, pullNumber int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, pullNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create pr diff request: %w", err)
	}

	// Request raw unified diff format from GitHub API
	req.Header.Set("Accept", "application/vnd.github.v3.diff")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed fetching diff (HTTP %d): %s", resp.StatusCode, string(body))
	}

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading diff response: %w", err)
	}

	return string(diffBytes), nil
}

// SubmitPullRequestReview publishes review findings as inline comments and summary verdict on GitHub.
func (c *Client) SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission PullReviewSubmission) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, pullNumber)

	payloadBytes, err := json.Marshal(submission)
	if err != nil {
		return fmt.Errorf("failed marshaling review submission: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed creating review post request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github api post review failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github post review rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// CheckRunAnnotation defines an annotation on a specific line of code.
type CheckRunAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"` // "notice", "warning", "failure"
	Message         string `json:"message"`
	Title           string `json:"title,omitempty"`
}

// CheckRunOutput defines the visual summary and annotations of a check run.
type CheckRunOutput struct {
	Title       string               `json:"title"`
	Summary     string               `json:"summary"`
	Text        string               `json:"text,omitempty"`
	Annotations []CheckRunAnnotation `json:"annotations,omitempty"`
}

// CreateCheckRunRequest models POST /repos/{owner}/{repo}/check-runs.
type CreateCheckRunRequest struct {
	Name       string          `json:"name"`
	HeadSHA    string          `json:"head_sha"`
	Status     string          `json:"status"` // "queued", "in_progress", "completed"
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	Output     *CheckRunOutput `json:"output,omitempty"`
}

// UpdateCheckRunRequest models PATCH /repos/{owner}/{repo}/check-runs/{check_run_id}.
type UpdateCheckRunRequest struct {
	Status      string          `json:"status,omitempty"`     // "completed"
	Conclusion  string          `json:"conclusion,omitempty"` // "success", "failure", "neutral", "action_required"
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Output      *CheckRunOutput `json:"output,omitempty"`
}

// CheckRunResponse models the response from check run operations.
type CheckRunResponse struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

// CreateCheckRun registers a check suite run on the pull request head commit.
func (c *Client) CreateCheckRun(ctx context.Context, owner, repo string, req CreateCheckRunRequest) (*CheckRunResponse, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/check-runs", c.baseURL, owner, repo)

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling create check run payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating check run request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("check run creation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github check run rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res CheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed parsing check run response: %w", err)
	}

	return &res, nil
}

// UpdateCheckRun updates status, conclusion, and findings annotations for a check run.
func (c *Client) UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, req UpdateCheckRunRequest) error {
	url := fmt.Sprintf("%s/repos/%s/%s/check-runs/%d", c.baseURL, owner, repo, checkRunID)

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed marshaling update check run payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed creating check run update request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("check run update failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github check run update rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateCommentReply posts an in-line answer to a specific review comment thread on GitHub.
func (c *Client) CreateCommentReply(ctx context.Context, owner, repo string, pullNumber int, commentID int64, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments/%d/replies", c.baseURL, owner, repo, pullNumber, commentID)

	payload := map[string]string{"body": body}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed marshaling comment reply: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed creating comment reply request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github comment reply request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github comment reply rejected (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GenerateAppJWT mints an RS256 JWT for GitHub App authentication using private key PEM bytes.
func GenerateAppJWT(appID string, privateKeyPEM []byte) (string, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return "", fmt.Errorf("failed decoding PEM block for GitHub App private key")
	}

	var privKey *rsa.PrivateKey
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		privKey = key
	} else if keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		var ok bool
		privKey, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("PKCS8 key is not an RSA private key")
		}
	} else {
		return "", fmt.Errorf("failed parsing private key as PKCS1 or PKCS8: %w", err)
	}

	now := time.Now().Unix()
	headerJSON := `{"alg":"RS256","typ":"JWT"}`
	payloadJSON := fmt.Sprintf(`{"iat":%d,"exp":%d,"iss":"%s"}`, now-60, now+600, appID)

	encodedHeader := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	signingInput := encodedHeader + "." + encodedPayload

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed signing GitHub App JWT: %w", err)
	}

	encodedSig := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + encodedSig, nil
}

// CreateInstallationToken exchanges a GitHub App JWT for a tenant installation access token.
func (c *Client) CreateInstallationToken(ctx context.Context, installationID int64, appJWT string) (string, error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.baseURL, installationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed creating installation token request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed requesting installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github installation token rejected (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed parsing installation token response: %w", err)
	}

	return result.Token, nil
}

