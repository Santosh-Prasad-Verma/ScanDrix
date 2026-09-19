package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
)

// SCMPlatformType identifies the target Git hosting provider.
type SCMPlatformType string

const (
	PlatformGitHub      SCMPlatformType = "github"
	PlatformGitLab      SCMPlatformType = "gitlab"
	PlatformAzureDevOps SCMPlatformType = "azure_devops"
	PlatformBitbucket   SCMPlatformType = "bitbucket"
)



// RateLimitInfo tracks provider rate-limiting headers.
type RateLimitInfo struct {
	Remaining int
	Limit     int
	ResetAt   time.Time
	RetryAfter time.Duration
}

// PublishCommentPayload represents a unified comment payload ready for SCM dispatch.
type PublishCommentPayload struct {
	SuggestionID string
	FilePath     string
	StartLine    int
	EndLine      int
	Side         string // "RIGHT" or "LEFT"
	CommitSHA    string
	BaseSHA      string
	Body         string
	ReplyToID    string
	InReplyTo    int64
	Resolution   ThreadStatus
}

// PublishedThreadResult captures the outcome of publishing an inline discussion.
type PublishedThreadResult struct {
	SuggestionID string
	CommentID    string
	ThreadID     string
	Platform     SCMPlatformType
	URL          string
	IsResolved   bool
	PublishedAt  time.Time
}

// SCMThreadPublisherConfig provides credentials and policies for SCM communication.
type SCMThreadPublisherConfig struct {
	Platform       SCMPlatformType
	BaseURL        string
	Token          string
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	RateLimitSleep bool
	DryRun         bool
}

// SCMThreadPublisher coordinates batched, resilient posting of inline review comments.
type SCMThreadPublisher struct {
	cfg        SCMThreadPublisherConfig
	httpClient *http.Client
	mu         sync.Mutex
	postedKeys map[string]PublishedThreadResult
	rateLimits RateLimitInfo
}

// NewSCMThreadPublisher constructs a thread publisher with default retry limits.
func NewSCMThreadPublisher(cfg SCMThreadPublisherConfig, httpClient *http.Client) *SCMThreadPublisher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 4
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Second
	}

	return &SCMThreadPublisher{
		cfg:        cfg,
		httpClient: httpClient,
		postedKeys: make(map[string]PublishedThreadResult),
		rateLimits: RateLimitInfo{Remaining: 1000, Limit: 1000},
	}
}

// GenerateIdempotencyKey creates a stable hash preventing duplicate comment posting.
func (p *SCMThreadPublisher) GenerateIdempotencyKey(repoID string, prNumber int, payload PublishCommentPayload) string {
	raw := fmt.Sprintf("%s:%d:%s:%s:%d:%d:%s", repoID, prNumber, payload.CommitSHA, payload.FilePath, payload.StartLine, payload.EndLine, payload.Body)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// PublishBatch publishes a slice of suggestions to the target SCM using batch or parallel calls.
func (p *SCMThreadPublisher) PublishBatch(
	ctx context.Context,
	repoID string,
	prNumber int,
	payloads []PublishCommentPayload,
) ([]PublishedThreadResult, []error) {
	if len(payloads) == 0 {
		return nil, nil
	}

	if p.cfg.DryRun {
		var dryResults []PublishedThreadResult
		for _, pl := range payloads {
			dryResults = append(dryResults, PublishedThreadResult{
				SuggestionID: pl.SuggestionID,
				CommentID:    fmt.Sprintf("dry_run_comment_%s", pl.SuggestionID),
				ThreadID:     fmt.Sprintf("dry_run_thread_%s", pl.SuggestionID),
				Platform:     p.cfg.Platform,
				URL:          fmt.Sprintf("https://scandrix.dev/dry-run/%s/pull/%d#%s", repoID, prNumber, pl.SuggestionID),
				IsResolved:   false,
				PublishedAt:  time.Now().UTC(),
			})
		}
		return dryResults, nil
	}

	switch p.cfg.Platform {
	case PlatformGitHub:
		return p.publishGitHubBatch(ctx, repoID, prNumber, payloads)
	case PlatformGitLab:
		return p.publishGitLabBatch(ctx, repoID, prNumber, payloads)
	case PlatformAzureDevOps:
		return p.publishAzureDevOpsBatch(ctx, repoID, prNumber, payloads)
	case PlatformBitbucket:
		return p.publishBitbucketBatch(ctx, repoID, prNumber, payloads)
	default:
		return nil, []error{fmt.Errorf("unsupported SCM platform: %s", p.cfg.Platform)}
	}
}

// publishGitHubBatch handles GitHub Pull Request review comment batching via GraphQL or REST.
func (p *SCMThreadPublisher) publishGitHubBatch(
	ctx context.Context,
	repoID string,
	prNumber int,
	payloads []PublishCommentPayload,
) ([]PublishedThreadResult, []error) {
	var results []PublishedThreadResult
	var errs []error

	parts := strings.Split(repoID, "/")
	if len(parts) != 2 {
		return nil, []error{fmt.Errorf("invalid github repository format: expected 'owner/repo', got '%s'", repoID)}
	}
	owner, repo := parts[0], parts[1]

	for _, payload := range payloads {
		idemKey := p.GenerateIdempotencyKey(repoID, prNumber, payload)
		p.mu.Lock()
		if existing, ok := p.postedKeys[idemKey]; ok {
			p.mu.Unlock()
			results = append(results, existing)
			continue
		}
		p.mu.Unlock()

		res, err := p.executeWithRetry(ctx, func() (PublishedThreadResult, error) {
			return p.postGitHubReviewComment(ctx, owner, repo, prNumber, payload)
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to post github comment for suggestion %s: %w", payload.SuggestionID, err))
		} else {
			p.mu.Lock()
			p.postedKeys[idemKey] = res
			p.mu.Unlock()
			results = append(results, res)
		}
	}

	return results, errs
}

func (p *SCMThreadPublisher) postGitHubReviewComment(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	payload PublishCommentPayload,
) (PublishedThreadResult, error) {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", apiURL, owner, repo, prNumber)

	bodyMap := map[string]any{
		"body":      payload.Body,
		"commit_id": payload.CommitSHA,
		"path":      payload.FilePath,
		"line":      payload.EndLine,
	}

	if payload.StartLine > 0 && payload.StartLine < payload.EndLine {
		bodyMap["start_line"] = payload.StartLine
		bodyMap["start_side"] = "RIGHT"
		bodyMap["side"] = "RIGHT"
	}

	if payload.ReplyToID != "" {
		endpoint = fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments/%s/replies", apiURL, owner, repo, prNumber, payload.ReplyToID)
		bodyMap = map[string]any{
			"body": payload.Body,
		}
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return PublishedThreadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return PublishedThreadResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PublishedThreadResult{}, err
	}
	defer resp.Body.Close()

	p.extractRateLimitHeaders(resp.Header)

	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		return PublishedThreadResult{}, fmt.Errorf("github api error (status %d): %s", resp.StatusCode, string(respData))
	}

	var parsed struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
		NodeID  string `json:"node_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PublishedThreadResult{}, err
	}

	return PublishedThreadResult{
		SuggestionID: payload.SuggestionID,
		CommentID:    fmt.Sprintf("%d", parsed.ID),
		ThreadID:     parsed.NodeID,
		Platform:     PlatformGitHub,
		URL:          parsed.HTMLURL,
		IsResolved:   false,
		PublishedAt:  time.Now().UTC(),
	}, nil
}

// publishGitLabBatch creates merge request discussion threads on GitLab.
func (p *SCMThreadPublisher) publishGitLabBatch(
	ctx context.Context,
	repoID string,
	prNumber int,
	payloads []PublishCommentPayload,
) ([]PublishedThreadResult, []error) {
	var results []PublishedThreadResult
	var errs []error

	for _, payload := range payloads {
		idemKey := p.GenerateIdempotencyKey(repoID, prNumber, payload)
		p.mu.Lock()
		if existing, ok := p.postedKeys[idemKey]; ok {
			p.mu.Unlock()
			results = append(results, existing)
			continue
		}
		p.mu.Unlock()

		res, err := p.executeWithRetry(ctx, func() (PublishedThreadResult, error) {
			return p.postGitLabDiscussion(ctx, repoID, prNumber, payload)
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to post gitlab discussion for suggestion %s: %w", payload.SuggestionID, err))
		} else {
			p.mu.Lock()
			p.postedKeys[idemKey] = res
			p.mu.Unlock()
			results = append(results, res)
		}
	}

	return results, errs
}

func (p *SCMThreadPublisher) postGitLabDiscussion(
	ctx context.Context,
	projectID string,
	mrIID int,
	payload PublishCommentPayload,
) (PublishedThreadResult, error) {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	escapedProject := strings.ReplaceAll(projectID, "/", "%2F")
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions", apiURL, escapedProject, mrIID)

	position := map[string]any{
		"position_type": "text",
		"base_sha":      payload.BaseSHA,
		"start_sha":     payload.BaseSHA,
		"head_sha":      payload.CommitSHA,
		"new_path":      payload.FilePath,
		"old_path":      payload.FilePath,
		"new_line":      payload.EndLine,
	}
	if payload.StartLine > 0 && payload.StartLine < payload.EndLine {
		position["line_range"] = map[string]any{
			"start": map[string]any{"new_line": payload.StartLine, "type": "new"},
			"end":   map[string]any{"new_line": payload.EndLine, "type": "new"},
		}
	}

	bodyMap := map[string]any{
		"body":     payload.Body,
		"position": position,
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return PublishedThreadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return PublishedThreadResult{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PublishedThreadResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		return PublishedThreadResult{}, fmt.Errorf("gitlab api error (status %d): %s", resp.StatusCode, string(respData))
	}

	var parsed struct {
		ID    string `json:"id"`
		Notes []struct {
			ID   int64 `json:"id"`
			Note string `json:"body"`
		} `json:"notes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PublishedThreadResult{}, err
	}

	commentID := parsed.ID
	if len(parsed.Notes) > 0 {
		commentID = fmt.Sprintf("%d", parsed.Notes[0].ID)
	}

	return PublishedThreadResult{
		SuggestionID: payload.SuggestionID,
		CommentID:    commentID,
		ThreadID:     parsed.ID,
		Platform:     PlatformGitLab,
		URL:          fmt.Sprintf("https://gitlab.com/%s/-/merge_requests/%d#note_%s", projectID, mrIID, commentID),
		IsResolved:   false,
		PublishedAt:  time.Now().UTC(),
	}, nil
}

// publishAzureDevOpsBatch creates pull request discussion threads on Azure DevOps.
func (p *SCMThreadPublisher) publishAzureDevOpsBatch(
	ctx context.Context,
	repoID string,
	prNumber int,
	payloads []PublishCommentPayload,
) ([]PublishedThreadResult, []error) {
	var results []PublishedThreadResult
	var errs []error

	for _, payload := range payloads {
		idemKey := p.GenerateIdempotencyKey(repoID, prNumber, payload)
		p.mu.Lock()
		if existing, ok := p.postedKeys[idemKey]; ok {
			p.mu.Unlock()
			results = append(results, existing)
			continue
		}
		p.mu.Unlock()

		res, err := p.executeWithRetry(ctx, func() (PublishedThreadResult, error) {
			return p.postAzureDevOpsThread(ctx, repoID, prNumber, payload)
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to post azure devops thread for suggestion %s: %w", payload.SuggestionID, err))
		} else {
			p.mu.Lock()
			p.postedKeys[idemKey] = res
			p.mu.Unlock()
			results = append(results, res)
		}
	}

	return results, errs
}

func (p *SCMThreadPublisher) postAzureDevOpsThread(
	ctx context.Context,
	repoID string,
	prNumber int,
	payload PublishCommentPayload,
) (PublishedThreadResult, error) {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://dev.azure.com"
	}

	// Azure DevOps thread creation endpoint
	endpoint := fmt.Sprintf("%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.0", apiURL, repoID, prNumber)

	startLine := payload.EndLine
	if payload.StartLine > 0 {
		startLine = payload.StartLine
	}

	statusNum := 1 // active
	if payload.Resolution == ThreadStatusFixed {
		statusNum = 2 // fixed
	} else if payload.Resolution == ThreadStatusWontFix {
		statusNum = 3 // wontfix
	} else if payload.Resolution == ThreadStatusClosed {
		statusNum = 4 // closed
	}

	bodyMap := map[string]any{
		"comments": []map[string]any{
			{
				"parentCommentId": 0,
				"content":         payload.Body,
				"commentType":     1, // text
			},
		},
		"status": statusNum,
		"threadContext": map[string]any{
			"filePath": "/" + strings.TrimPrefix(payload.FilePath, "/"),
			"rightFileStart": map[string]any{
				"line":   startLine,
				"offset": 1,
			},
			"rightFileEnd": map[string]any{
				"line":   payload.EndLine,
				"offset": 1,
			},
		},
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return PublishedThreadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return PublishedThreadResult{}, err
	}
	req.Header.Set("Authorization", "Basic "+p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PublishedThreadResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		return PublishedThreadResult{}, fmt.Errorf("azure devops api error (status %d): %s", resp.StatusCode, string(respData))
	}

	var parsed struct {
		ID       int `json:"id"`
		Comments []struct {
			ID int `json:"id"`
		} `json:"comments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PublishedThreadResult{}, err
	}

	commentID := fmt.Sprintf("%d", parsed.ID)
	if len(parsed.Comments) > 0 {
		commentID = fmt.Sprintf("%d", parsed.Comments[0].ID)
	}

	return PublishedThreadResult{
		SuggestionID: payload.SuggestionID,
		CommentID:    commentID,
		ThreadID:     fmt.Sprintf("%d", parsed.ID),
		Platform:     PlatformAzureDevOps,
		URL:          fmt.Sprintf("%s/_git/%s/pullrequest/%d?_a=overview&discussionId=%d", apiURL, repoID, prNumber, parsed.ID),
		IsResolved:   statusNum == 2,
		PublishedAt:  time.Now().UTC(),
	}, nil
}

// publishBitbucketBatch posts comments to Bitbucket pull requests.
func (p *SCMThreadPublisher) publishBitbucketBatch(
	ctx context.Context,
	repoID string,
	prNumber int,
	payloads []PublishCommentPayload,
) ([]PublishedThreadResult, []error) {
	var results []PublishedThreadResult
	var errs []error

	for _, payload := range payloads {
		idemKey := p.GenerateIdempotencyKey(repoID, prNumber, payload)
		p.mu.Lock()
		if existing, ok := p.postedKeys[idemKey]; ok {
			p.mu.Unlock()
			results = append(results, existing)
			continue
		}
		p.mu.Unlock()

		res, err := p.executeWithRetry(ctx, func() (PublishedThreadResult, error) {
			return p.postBitbucketComment(ctx, repoID, prNumber, payload)
		})

		if err != nil {
			errs = append(errs, fmt.Errorf("failed to post bitbucket comment for suggestion %s: %w", payload.SuggestionID, err))
		} else {
			p.mu.Lock()
			p.postedKeys[idemKey] = res
			p.mu.Unlock()
			results = append(results, res)
		}
	}

	return results, errs
}

func (p *SCMThreadPublisher) postBitbucketComment(
	ctx context.Context,
	repoID string,
	prNumber int,
	payload PublishCommentPayload,
) (PublishedThreadResult, error) {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://api.bitbucket.org/2.0"
	}

	// repoID format: workspace/repo-slug
	endpoint := fmt.Sprintf("%s/repositories/%s/pullrequests/%d/comments", apiURL, repoID, prNumber)

	bodyMap := map[string]any{
		"content": map[string]any{
			"raw": payload.Body,
		},
		"inline": map[string]any{
			"to":   payload.EndLine,
			"path": payload.FilePath,
		},
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return PublishedThreadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return PublishedThreadResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PublishedThreadResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		return PublishedThreadResult{}, fmt.Errorf("bitbucket api error (status %d): %s", resp.StatusCode, string(respData))
	}

	var parsed struct {
		ID    int `json:"id"`
		Links struct {
			HTML struct {
				HRef string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PublishedThreadResult{}, err
	}

	return PublishedThreadResult{
		SuggestionID: payload.SuggestionID,
		CommentID:    fmt.Sprintf("%d", parsed.ID),
		ThreadID:     fmt.Sprintf("%d", parsed.ID),
		Platform:     PlatformBitbucket,
		URL:          parsed.Links.HTML.HRef,
		IsResolved:   false,
		PublishedAt:  time.Now().UTC(),
	}, nil
}

// ResolveThread updates an existing SCM thread to mark it as resolved or fixed.
func (p *SCMThreadPublisher) ResolveThread(ctx context.Context, repoID string, prNumber int, threadID string) error {
	if p.cfg.DryRun {
		return nil
	}

	switch p.cfg.Platform {
	case PlatformGitHub:
		return p.resolveGitHubThreadGraphQL(ctx, threadID)
	case PlatformGitLab:
		return p.resolveGitLabDiscussion(ctx, repoID, prNumber, threadID)
	case PlatformAzureDevOps:
		return p.updateAzureDevOpsThreadStatus(ctx, repoID, prNumber, threadID, 2) // 2 = Fixed
	default:
		return nil
	}
}

// MinimizeComment hides an outdated or duplicate comment on GitHub.
func (p *SCMThreadPublisher) MinimizeComment(ctx context.Context, commentNodeID string, reason MinimizationReason) error {
	if p.cfg.DryRun || p.cfg.Platform != PlatformGitHub {
		return nil
	}

	query := `mutation MinimizeComment($input: MinimizeCommentInput!) {
		minimizeComment(input: $input) {
			minimizedComment {
				isMinimized
				minimizedReason
			}
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"subjectId": commentNodeID,
			"classifier": string(reason),
		},
	}

	payloadMap := map[string]any{
		"query":     query,
		"variables": vars,
	}

	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return err
	}

	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://api.github.com/graphql"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("minimizeComment failed (status %d): %s", resp.StatusCode, string(data))
	}

	return nil
}

func (p *SCMThreadPublisher) resolveGitHubThreadGraphQL(ctx context.Context, threadNodeID string) error {
	query := `mutation ResolveReviewThread($input: ResolveReviewThreadInput!) {
		resolveReviewThread(input: $input) {
			thread {
				isResolved
			}
		}
	}`

	vars := map[string]any{
		"input": map[string]any{
			"threadId": threadNodeID,
		},
	}

	payloadMap := map[string]any{
		"query":     query,
		"variables": vars,
	}

	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return err
	}

	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://api.github.com/graphql"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resolveReviewThread failed (status %d): %s", resp.StatusCode, string(data))
	}

	return nil
}

func (p *SCMThreadPublisher) resolveGitLabDiscussion(ctx context.Context, projectID string, mrIID int, discussionID string) error {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	escapedProject := strings.ReplaceAll(projectID, "/", "%2F")
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions/%s?resolved=true", apiURL, escapedProject, mrIID, discussionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", p.cfg.Token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resolve gitlab discussion failed (status %d): %s", resp.StatusCode, string(data))
	}
	return nil
}

func (p *SCMThreadPublisher) updateAzureDevOpsThreadStatus(ctx context.Context, repoID string, prNumber int, threadID string, status int) error {
	apiURL := p.cfg.BaseURL
	if apiURL == "" {
		apiURL = "https://dev.azure.com"
	}
	endpoint := fmt.Sprintf("%s/_apis/git/repositories/%s/pullRequests/%d/threads/%s?api-version=7.0", apiURL, repoID, prNumber, threadID)

	bodyMap := map[string]any{
		"status": status,
	}
	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+p.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update azure devops thread status failed (status %d): %s", resp.StatusCode, string(data))
	}
	return nil
}

// executeWithRetry wraps network calls with exponential backoff and rate limit jitter.
func (p *SCMThreadPublisher) executeWithRetry(
	ctx context.Context,
	op func() (PublishedThreadResult, error),
) (PublishedThreadResult, error) {
	var lastErr error
	backoff := p.cfg.InitialBackoff

	for attempt := 0; attempt <= p.cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return PublishedThreadResult{}, ctx.Err()
		}

		// Wait if rate limit was signaled
		p.mu.Lock()
		if p.rateLimits.Remaining == 0 && p.rateLimits.ResetAt.After(time.Now()) {
			waitDur := time.Until(p.rateLimits.ResetAt)
			p.mu.Unlock()
			if waitDur > 0 && waitDur < 2*time.Minute && p.cfg.RateLimitSleep {
				select {
				case <-time.After(waitDur):
				case <-ctx.Done():
					return PublishedThreadResult{}, ctx.Err()
				}
			} else {
				return PublishedThreadResult{}, errors.New("scm rate limit exhausted; aborting request")
			}
		} else {
			p.mu.Unlock()
		}

		res, err := op()
		if err == nil {
			return res, nil
		}

		lastErr = err

		// Inspect if error is transient / retryable
		errStr := err.Error()
		isRetryable := strings.Contains(errStr, "status 429") ||
			strings.Contains(errStr, "status 502") ||
			strings.Contains(errStr, "status 503") ||
			strings.Contains(errStr, "status 504") ||
			strings.Contains(errStr, "secondary rate limit") ||
			strings.Contains(errStr, "connection reset")

		if !isRetryable || attempt == p.cfg.MaxRetries {
			break
		}

		// Calculate jittered backoff
		jitter := time.Duration(rand.Float64() * float64(backoff) * 0.3)
		sleepDur := backoff + jitter
		if sleepDur > p.cfg.MaxBackoff {
			sleepDur = p.cfg.MaxBackoff
		}

		select {
		case <-time.After(sleepDur):
		case <-ctx.Done():
			return PublishedThreadResult{}, ctx.Err()
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(p.cfg.MaxBackoff)))
	}

	return PublishedThreadResult{}, lastErr
}

func (p *SCMThreadPublisher) extractRateLimitHeaders(header http.Header) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if rem := header.Get("X-RateLimit-Remaining"); rem != "" {
		var r int
		if _, err := fmt.Sscanf(rem, "%d", &r); err == nil {
			p.rateLimits.Remaining = r
		}
	}
	if lim := header.Get("X-RateLimit-Limit"); lim != "" {
		var l int
		if _, err := fmt.Sscanf(lim, "%d", &l); err == nil {
			p.rateLimits.Limit = l
		}
	}
	if reset := header.Get("X-RateLimit-Reset"); reset != "" {
		var sec int64
		if _, err := fmt.Sscanf(reset, "%d", &sec); err == nil {
			p.rateLimits.ResetAt = time.Unix(sec, 0)
		}
	}
	if retryAfter := header.Get("Retry-After"); retryAfter != "" {
		var sec int
		if _, err := fmt.Sscanf(retryAfter, "%d", &sec); err == nil {
			p.rateLimits.RetryAfter = time.Duration(sec) * time.Second
			p.rateLimits.ResetAt = time.Now().Add(p.rateLimits.RetryAfter)
		}
	}
}

// ConvertCodeSuggestionsToPublishPayloads transforms domain suggestions into SCM publish payloads.
func ConvertCodeSuggestionsToPublishPayloads(
	suggestions []*domain.CodeSuggestion,
	commitSHA, baseSHA string,
	formatter func(*domain.CodeSuggestion) string,
) []PublishCommentPayload {
	var payloads []PublishCommentPayload

	for _, s := range suggestions {
		if s == nil {
			continue
		}

		body := s.GetExplanation()
		if formatter != nil {
			body = formatter(s)
		}

		startL := s.GetStartLine()
		endL := s.GetEndLine()
		if endL < startL {
			endL = startL
		}

		payloads = append(payloads, PublishCommentPayload{
			SuggestionID: s.ID.String(),
			FilePath:     s.GetFilePath(),
			StartLine:    startL,
			EndLine:      endL,
			Side:         "RIGHT",
			CommitSHA:    commitSHA,
			BaseSHA:      baseSHA,
			Body:         body,
			Resolution:   ThreadStatusActive,
		})
	}

	return payloads
}
