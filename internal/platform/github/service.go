// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// GitHubService implements contracts.ICodeManagementService for GitHub Cloud and GitHub Enterprise Server.
// Translates libs/platform/infrastructure/adapters/services/github/github.service.ts
var _ contracts.ICodeManagementService = (*GitHubService)(nil)

type GitHubService struct {
	baseURL         string
	graphqlURL      string
	httpClient      *http.Client
	rateLimitGate   *GitHubRateLimitGateService
	etagStore       IETagStore
	checksService   *GithubChecksService
	tokenRotator    *AppTokenRotator
	defaultTimeout  time.Duration
	batchSize       int
	commentBadgeMap sync.Map
}

// GitHubServiceConfig configures GitHubService instances.
type GitHubServiceConfig struct {
	BaseURL        string
	GraphQLURL     string
	HTTPClient     *http.Client
	RateLimitGate  *GitHubRateLimitGateService
	ETagStore      IETagStore
	ChecksService  *GithubChecksService
	TokenRotator   *AppTokenRotator
	DefaultTimeout time.Duration
}

// NewGitHubService creates a production GitHub service adapter.
func NewGitHubService(cfg GitHubServiceConfig) *GitHubService {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	graphqlURL := cfg.GraphQLURL
	if graphqlURL == "" {
		if baseURL == "https://api.github.com" {
			graphqlURL = "https://api.github.com/graphql"
		} else {
			graphqlURL = baseURL + "/graphql"
		}
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 45 * time.Second,
		}
	}

	etagStore := cfg.ETagStore
	if etagStore == nil {
		etagStore = NewMemoryETagStore()
	}

	rateLimitGate := cfg.RateLimitGate
	if rateLimitGate == nil {
		rateLimitGate = NewGitHubRateLimitGateService(nil, nil)
	}

	checksService := cfg.ChecksService
	if checksService == nil {
		checksService = NewGithubChecksService(nil, nil)
	}

	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	return &GitHubService{
		baseURL:        baseURL,
		graphqlURL:     graphqlURL,
		httpClient:     client,
		rateLimitGate:  rateLimitGate,
		etagStore:      etagStore,
		checksService:  checksService,
		tokenRotator:   cfg.TokenRotator,
		defaultTimeout: timeout,
		batchSize:      50,
	}
}

// Provider returns GitHub SCM provider identifier.
func (s *GitHubService) Provider() models.SCMProvider {
	return models.ProviderGitHub
}

// -------------------------------------------------------------------------------------
// Authorization & HTTP Request Plumbing
// -------------------------------------------------------------------------------------

func (s *GitHubService) resolveToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	if orgData.IntegrationCredentials != nil {
		if token, ok := orgData.IntegrationCredentials["token"].(string); ok && token != "" {
			return token, nil
		}
		if pat, ok := orgData.IntegrationCredentials["personalAccessToken"].(string); ok && pat != "" {
			return pat, nil
		}
		if s.tokenRotator != nil {
			if appID, ok := orgData.IntegrationCredentials["appId"].(string); ok && appID != "" {
				if installID, ok := orgData.IntegrationCredentials["installationId"].(int64); ok && installID > 0 {
					return s.tokenRotator.GetInstallationToken(ctx, installID)
				}
			}
		}
	}
	return "", errors.New("github authorization credentials not found for organization")
}

func (s *GitHubService) resolveOwnerRepo(orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, string, error) {
	owner := ""
	name := ""
	if repo != nil {
		name = repo.Name
		owner = repo.Owner
	}
	if owner == "" && orgData.IntegrationCredentials != nil {
		if org, ok := orgData.IntegrationCredentials["org"].(string); ok && org != "" {
			owner = org
		} else if account, ok := orgData.IntegrationCredentials["account"].(string); ok && account != "" {
			owner = account
		}
	}
	if owner == "" && repo != nil && repo.FullName != "" {
		parts := strings.Split(repo.FullName, "/")
		if len(parts) == 2 {
			owner = parts[0]
			name = parts[1]
		}
	}
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("cannot resolve owner/name for repository (owner: %q, name: %q)", owner, name)
	}
	return owner, name, nil
}

func (s *GitHubService) executeRequest(ctx context.Context, token, method, apiPath string, body any, out any) error {
	fullURL := apiPath
	if !strings.HasPrefix(apiPath, "http://") && !strings.HasPrefix(apiPath, "https://") {
		fullURL = fmt.Sprintf("%s/%s", s.baseURL, strings.TrimLeft(apiPath, "/"))
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request payload: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ScanDrix-Platform/1.0")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github http request error (%s %s): %w", method, apiPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode == http.StatusNotModified {
		return nil
	}

	if resp.StatusCode >= 400 {
		rawErr, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api error (%s %s status %d): %s", method, apiPath, resp.StatusCode, string(rawErr))
	}

	if out != nil {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read github response body: %w", err)
		}
		if strOut, ok := out.(*string); ok {
			*strOut = string(raw)
			return nil
		}
		if bytesOut, ok := out.(*[]byte); ok {
			*bytesOut = raw
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("failed to unmarshal github response: %w (payload: %s)", err, string(raw))
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Repository Inspection & Manipulation
// -------------------------------------------------------------------------------------

func (s *GitHubService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner := ""
	if orgData.IntegrationCredentials != nil {
		if org, ok := orgData.IntegrationCredentials["org"].(string); ok && org != "" {
			owner = org
		}
	}
	if owner == "" {
		owner = name
	}

	apiPath := fmt.Sprintf("/repos/%s/%s", owner, name)
	if strings.Contains(name, "/") {
		apiPath = fmt.Sprintf("/repos/%s", name)
	}

	var data struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Private  bool   `json:"private"`
		Fork     bool   `json:"fork"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		Language      string `json:"language"`
		Archived      bool   `json:"archived"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &data); err != nil {
		return nil, err
	}

	return &types.Repository{
		ID:            strconv.FormatInt(data.ID, 10),
		Name:          data.Name,
		FullName:      data.FullName,
		Owner:         data.Owner.Login,
		Private:       data.Private,
		DefaultBranch: data.DefaultBranch,
		HTMLURL:       data.HTMLURL,
		CloneURL:      data.CloneURL,
		SSHURL:        data.SSHURL,
		Language:      data.Language,
		Archived:      data.Archived,
	}, nil
}

func (s *GitHubService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner := ""
	if orgData.IntegrationCredentials != nil {
		if org, ok := orgData.IntegrationCredentials["org"].(string); ok && org != "" {
			owner = org
		}
	}

	apiPath := "/user/repos?per_page=100&sort=updated"
	if owner != "" {
		apiPath = fmt.Sprintf("/orgs/%s/repos?per_page=100&sort=updated", owner)
	}

	var rawRepos []struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		Archived      bool   `json:"archived"`
		Language      string `json:"language"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawRepos); err != nil {
		return nil, err
	}

	results := make([]*types.Repositories, 0, len(rawRepos))
	for _, r := range rawRepos {
		if archived != nil && r.Archived != *archived {
			continue
		}
		if visibility == "private" && !r.Private {
			continue
		}
		if visibility == "public" && r.Private {
			continue
		}
		if language != "" && !strings.EqualFold(r.Language, language) {
			continue
		}

		results = append(results, &types.Repositories{
			ID:            strconv.FormatInt(r.ID, 10),
			Name:          r.Name,
			FullName:      r.FullName,
			Owner:         r.Owner.Login,
			Private:       r.Private,
			Archived:      r.Archived,
			DefaultBranch: r.DefaultBranch,
			Language:      r.Language,
		})
	}

	return results, nil
}

// -------------------------------------------------------------------------------------
// Pull Requests
// -------------------------------------------------------------------------------------

func (s *GitHubService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	if state == "" {
		state = "open"
	}

	query := url.Values{}
	query.Set("state", state)
	query.Set("per_page", "100")
	if branch != "" {
		query.Set("head", fmt.Sprintf("%s:%s", owner, branch))
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls?%s", owner, repoName, query.Encode())

	var rawPRs []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Draft  bool   `json:"draft"`
		User   struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		ClosedAt  *time.Time `json:"closed_at"`
		MergedAt  *time.Time `json:"merged_at"`
		HTMLURL   string     `json:"html_url"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawPRs); err != nil {
		return nil, err
	}

	results := make([]*types.PullRequest, 0, len(rawPRs))
	for _, r := range rawPRs {
		if author != "" && !strings.EqualFold(r.User.Login, author) {
			continue
		}

		var closedAtStr, mergedAtStr string
		if r.ClosedAt != nil {
			closedAtStr = r.ClosedAt.Format(time.RFC3339)
		}
		if r.MergedAt != nil {
			mergedAtStr = r.MergedAt.Format(time.RFC3339)
		}

		results = append(results, &types.PullRequest{
			Number:       r.Number,
			PullNumber:   r.Number,
			Title:        r.Title,
			Body:         r.Body,
			Description:  r.Body,
			State:        r.State,
			IsDraft:      r.Draft,
			Author:       r.User.Login,
			SourceBranch: r.Head.Ref,
			TargetBranch: r.Base.Ref,
			HeadSHA:      r.Head.SHA,
			BaseSHA:      r.Base.SHA,
			CreatedAt:    r.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
			ClosedAt:     closedAtStr,
			MergedAt:     mergedAtStr,
			URL:          r.HTMLURL,
			PRURL:        r.HTMLURL,
		})
	}

	return results, nil
}

// SearchPullRequestsByTitle searches pull requests in a repository using GitHub's Search API.
// Matches libs/platform/infrastructure/adapters/services/github/github.service.ts (searchPullRequestsByTitle).
func (s *GitHubService) SearchPullRequestsByTitle(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, title string, state string, author string, branch string) ([]*types.PullRequest, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	queryParts := []string{
		"is:pr",
		fmt.Sprintf("repo:%s/%s", owner, repoName),
	}

	if title != "" {
		queryParts = append(queryParts, fmt.Sprintf("%s in:title", title))
	}
	if state != "" && state != "all" {
		queryParts = append(queryParts, fmt.Sprintf("is:%s", state))
	}
	if author != "" {
		queryParts = append(queryParts, fmt.Sprintf("author:%s", author))
	}
	if branch != "" {
		queryParts = append(queryParts, fmt.Sprintf("base:%s", branch))
	}

	q := strings.Join(queryParts, " ")
	apiPath := fmt.Sprintf("/search/issues?q=%s&sort=created&order=desc&per_page=100", url.QueryEscape(q))

	var searchResp struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Number    int        `json:"number"`
			Title     string     `json:"title"`
			Body      string     `json:"body"`
			State     string     `json:"state"`
			Draft     bool       `json:"draft"`
			User      struct {
				Login string `json:"login"`
				ID    int64  `json:"id"`
			} `json:"user"`
			CreatedAt time.Time  `json:"created_at"`
			UpdatedAt time.Time  `json:"updated_at"`
			ClosedAt  *time.Time `json:"closed_at"`
			HTMLURL   string     `json:"html_url"`
			PullReq   *struct {
				URL string `json:"url"`
			} `json:"pull_request"`
		} `json:"items"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &searchResp); err != nil {
		return nil, err
	}

	results := make([]*types.PullRequest, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		if item.PullReq == nil {
			continue
		}

		var closedAtStr string
		if item.ClosedAt != nil {
			closedAtStr = item.ClosedAt.Format(time.RFC3339)
		}

		results = append(results, &types.PullRequest{
			Number:      item.Number,
			PullNumber:  item.Number,
			Title:       item.Title,
			Body:        item.Body,
			Description: item.Body,
			State:       item.State,
			IsDraft:     item.Draft,
			Author:      item.User.Login,
			CreatedAt:   item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   item.UpdatedAt.Format(time.RFC3339),
			ClosedAt:    closedAtStr,
			URL:         item.HTMLURL,
			PRURL:       item.HTMLURL,
		})
	}

	return results, nil
}

func (s *GitHubService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repoName, prNumber)

	var r struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Draft  bool   `json:"draft"`
		User   struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
		ClosedAt  *time.Time `json:"closed_at"`
		MergedAt  *time.Time `json:"merged_at"`
		HTMLURL   string     `json:"html_url"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &r); err != nil {
		return nil, err
	}

	var closedAtStr, mergedAtStr string
	if r.ClosedAt != nil {
		closedAtStr = r.ClosedAt.Format(time.RFC3339)
	}
	if r.MergedAt != nil {
		mergedAtStr = r.MergedAt.Format(time.RFC3339)
	}

	return &types.PullRequest{
		Number:       r.Number,
		PullNumber:   r.Number,
		Title:        r.Title,
		Body:         r.Body,
		Description:  r.Body,
		State:        r.State,
		IsDraft:      r.Draft,
		Author:       r.User.Login,
		SourceBranch: r.Head.Ref,
		TargetBranch: r.Base.Ref,
		HeadSHA:      r.Head.SHA,
		BaseSHA:      r.Base.SHA,
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
		ClosedAt:     closedAtStr,
		MergedAt:     mergedAtStr,
		URL:          r.HTMLURL,
		PRURL:        r.HTMLURL,
	}, nil
}

func (s *GitHubService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.GetPullRequest(ctx, orgData, repo, prNumber)
}

func (s *GitHubService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "open", "", "")
}

func (s *GitHubService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "open", "", "")
	if err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestWithFiles, 0, len(prs))
	for _, pr := range prs {
		files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, pr.Number)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch files for PR #%d: %w", pr.Number, err)
		}
		results = append(results, &types.PullRequestWithFiles{
			PullRequest: *pr,
			Files:       files,
		})
	}

	return results, nil
}

func (s *GitHubService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "all", "", "")
	if err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestCodeReviewTime, 0, len(prs))
	for _, pr := range prs {
		createdAt, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		var mergedAtPtr, closedAtPtr *time.Time
		var reviewTime float64

		if pr.MergedAt != "" {
			if m, err := time.Parse(time.RFC3339, pr.MergedAt); err == nil {
				mergedAtPtr = &m
				reviewTime = m.Sub(createdAt).Seconds()
			}
		} else if pr.ClosedAt != "" {
			if c, err := time.Parse(time.RFC3339, pr.ClosedAt); err == nil {
				closedAtPtr = &c
				reviewTime = c.Sub(createdAt).Seconds()
			}
		}

		results = append(results, &types.PullRequestCodeReviewTime{
			PRNumber:       pr.Number,
			ReviewDuration: reviewTime,
			CreatedAt:      createdAt,
			MergedAt:       mergedAtPtr,
			ClosedAt:       closedAtPtr,
			Author:         pr.Author,
		})
	}

	return results, nil
}

func (s *GitHubService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	pr, err := s.GetPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return false, err
	}
	return pr.IsDraft, nil
}

func (s *GitHubService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	pr, err := s.GetPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return "", err
	}
	return types.PullRequestReviewState(pr.State), nil
}

func (s *GitHubService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repoName, prNumber)
	body := map[string]string{
		"body": description,
	}

	return s.executeRequest(ctx, token, http.MethodPatch, apiPath, body, nil)
}

// -------------------------------------------------------------------------------------
// Diff & Files API
// -------------------------------------------------------------------------------------

func (s *GitHubService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	var allFiles []*types.PullRequestFile
	page := 1

	for {
		apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=100&page=%d", owner, repoName, prNumber, page)

		var rawFiles []struct {
			SHA         string `json:"sha"`
			Filename    string `json:"filename"`
			Status      string `json:"status"`
			Additions   int    `json:"additions"`
			Deletions   int    `json:"deletions"`
			Changes     int    `json:"changes"`
			BlobURL     string `json:"blob_url"`
			RawURL      string `json:"raw_url"`
			ContentsURL string `json:"contents_url"`
			Patch       string `json:"patch"`
		}

		if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawFiles); err != nil {
			return nil, err
		}

		if len(rawFiles) == 0 {
			break
		}

		for _, f := range rawFiles {
			allFiles = append(allFiles, &types.PullRequestFile{
				SHA:         f.SHA,
				Filename:    f.Filename,
				Status:      f.Status,
				Additions:   f.Additions,
				Deletions:   f.Deletions,
				Changes:     f.Changes,
				BlobURL:     f.BlobURL,
				RawURL:      f.RawURL,
				ContentsURL: f.ContentsURL,
				Patch:       f.Patch,
			})
		}

		if len(rawFiles) < 100 {
			break
		}
		page++
	}

	return allFiles, nil
}

func (s *GitHubService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repoName, baseSHA, headSHA)

	var comp struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &comp); err != nil {
		return nil, err
	}

	files := make([]string, len(comp.Files))
	for i, f := range comp.Files {
		files[i] = f.Filename
	}

	return files, nil
}

func (s *GitHubService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, path, ref string) (*types.RepositoryFile, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repoName, strings.TrimPrefix(path, "/"))
	if ref != "" {
		apiPath += "?ref=" + url.QueryEscape(ref)
	}

	var data struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		SHA      string `json:"sha"`
		Size     int    `json:"size"`
		Type     string `json:"type"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &data); err != nil {
		return nil, err
	}

	var contentBytes []byte
	if data.Encoding == "base64" {
		cleaned := strings.ReplaceAll(data.Content, "\n", "")
		decoded, err := base64.StdEncoding.DecodeString(cleaned)
		if err == nil {
			contentBytes = decoded
		}
	} else {
		contentBytes = []byte(data.Content)
	}

	return &types.RepositoryFile{
		Path:     data.Path,
		Content:  string(contentBytes),
		SHA:      data.SHA,
		Size:     int64(data.Size),
		IsBinary: bytes.IndexByte(contentBytes, 0) != -1,
	}, nil
}

// GetRepositoryContentBatch retrieves multiple repository files in parallel using GraphQL batches of 50.
// Matches libs/platform/infrastructure/adapters/services/github/github.service.ts getRepositoryContentBatch
func (s *GitHubService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
	result := make(map[string]*types.RepositoryFile)
	if len(files) == 0 {
		return result, nil
	}

	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return nil, err
	}

	if ref == "" {
		ref, _ = s.GetDefaultBranch(ctx, orgData, &repo)
		if ref == "" {
			ref = "HEAD"
		}
	}

	// Chunk files into batches of 50
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		chunk := files[i:end]

		// Construct GraphQL query
		var queryBuf bytes.Buffer
		queryBuf.WriteString("query($owner: String!, $repo: String!) {\n")
		queryBuf.WriteString("  repository(owner: $owner, name: $repo) {\n")

		for idx, f := range chunk {
			expression := fmt.Sprintf("%s:%s", ref, strings.TrimPrefix(f, "/"))
			queryBuf.WriteString(fmt.Sprintf("    f%d: object(expression: %q) {\n", idx, expression))
			queryBuf.WriteString("      ... on Blob {\n")
			queryBuf.WriteString("        text\n")
			queryBuf.WriteString("        isBinary\n")
			queryBuf.WriteString("        byteSize\n")
			queryBuf.WriteString("        oid\n")
			queryBuf.WriteString("      }\n")
			queryBuf.WriteString("    }\n")
		}
		queryBuf.WriteString("  }\n")
		queryBuf.WriteString("}\n")

		gqlPayload := map[string]any{
			"query": queryBuf.String(),
			"variables": map[string]any{
				"owner": owner,
				"repo":  repoName,
			},
		}

		var gqlResp struct {
			Data struct {
				Repository map[string]*struct {
					Text     string `json:"text"`
					IsBinary bool   `json:"isBinary"`
					ByteSize int    `json:"byteSize"`
					OID      string `json:"oid"`
				} `json:"repository"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}

		if err := s.executeRequest(ctx, token, http.MethodPost, s.graphqlURL, gqlPayload, &gqlResp); err != nil {
			// Fallback: fetch sequentially via REST
			for _, f := range chunk {
				rf, err := s.GetRepositoryContentFile(ctx, orgData, &repo, f, ref)
				if err == nil && rf != nil {
					result[f] = rf
				}
			}
			continue
		}

		for idx, f := range chunk {
			alias := fmt.Sprintf("f%d", idx)
			blob := gqlResp.Data.Repository[alias]
			if blob != nil {
				result[f] = &types.RepositoryFile{
					Path:     f,
					Content:  blob.Text,
					SHA:      blob.OID,
					Size:     int64(blob.ByteSize),
					IsBinary: blob.IsBinary,
				}
			} else {
				// Fallback to REST for this file
				rf, err := s.GetRepositoryContentFile(ctx, orgData, &repo, f, ref)
				if err == nil && rf != nil {
					result[f] = rf
				}
			}
		}
	}

	return result, nil
}

func (s *GitHubService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	return s.GetRepositoryTreeByDirectory(ctx, orgData, repositoryID, "")
}

func (s *GitHubService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(repositoryID, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repositoryID %q; expected 'owner/name'", repositoryID)
	}
	owner, repoName := parts[0], parts[1]

	defaultBranch, err := s.GetDefaultBranch(ctx, orgData, &types.RepositoryDescriptor{Owner: owner, Name: repoName})
	if err != nil || defaultBranch == "" {
		defaultBranch = "HEAD"
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repoName, defaultBranch)

	var treeResp struct {
		SHA  string `json:"sha"`
		Tree []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size int    `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &treeResp); err != nil {
		return nil, err
	}

	cleanDir := strings.Trim(directoryPath, "/")
	results := make([]*types.TreeItem, 0, len(treeResp.Tree))

	for _, item := range treeResp.Tree {
		if cleanDir != "" {
			if !strings.HasPrefix(item.Path, cleanDir+"/") && item.Path != cleanDir {
				continue
			}
		}

		results = append(results, &types.TreeItem{
			Path: item.Path,
			Mode: item.Mode,
			Type: item.Type,
			SHA:  item.SHA,
			Size: int64(item.Size),
		})
	}

	return results, nil
}

func (s *GitHubService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	repoID := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
	tree, err := s.GetRepositoryTree(ctx, orgData, repoID)
	if err != nil {
		return nil, err
	}

	var candidatePaths []string
	for _, item := range tree {
		if item.Type != "blob" {
			continue
		}

		// Filter patterns
		matched := len(filePatterns) == 0
		for _, pat := range filePatterns {
			if ok, _ := path.Match(pat, path.Base(item.Path)); ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		excluded := false
		for _, pat := range excludePatterns {
			if ok, _ := path.Match(pat, path.Base(item.Path)); ok {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		candidatePaths = append(candidatePaths, item.Path)
		if maxFiles > 0 && len(candidatePaths) >= maxFiles {
			break
		}
	}

	batchResult, err := s.GetRepositoryContentBatch(ctx, orgData, repo, candidatePaths, branch)
	if err != nil {
		return nil, err
	}

	results := make([]*types.RepositoryFile, 0, len(batchResult))
	for _, rf := range batchResult {
		results = append(results, rf)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	return results, nil
}

func (s *GitHubService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	r, err := s.FindRepositoryByName(ctx, orgData, repo.Name)
	if err != nil {
		return "", err
	}
	if r.DefaultBranch != "" {
		return r.DefaultBranch, nil
	}
	return "main", nil
}

// -------------------------------------------------------------------------------------
// Commits
// -------------------------------------------------------------------------------------

func (s *GitHubService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("per_page", "100")
	if branch != "" {
		query.Set("sha", branch)
	}
	if author != "" {
		query.Set("author", author)
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/commits?%s", owner, repoName, query.Encode())

	var rawCommits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawCommits); err != nil {
		return nil, err
	}

	results := make([]*types.Commit, len(rawCommits))
	for i, c := range rawCommits {
		authorLogin := ""
		if c.Author != nil {
			authorLogin = c.Author.Login
		}
		results[i] = &types.Commit{
			SHA:     c.SHA,
			Message: c.Commit.Message,
			Author: types.GitActor{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
			},
			AuthorName:  c.Commit.Author.Name,
			AuthorEmail: c.Commit.Author.Email,
			AuthorLogin: authorLogin,
			Date:        c.Commit.Author.Date,
		}
	}

	return results, nil
}

func (s *GitHubService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?per_page=100", owner, repoName, prNumber)

	var rawCommits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawCommits); err != nil {
		return nil, err
	}

	results := make([]*types.Commit, len(rawCommits))
	for i, c := range rawCommits {
		authorLogin := ""
		if c.Author != nil {
			authorLogin = c.Author.Login
		}
		results[i] = &types.Commit{
			SHA:     c.SHA,
			Message: c.Commit.Message,
			Author: types.GitActor{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
			},
			AuthorName:  c.Commit.Author.Name,
			AuthorEmail: c.Commit.Author.Email,
			AuthorLogin: authorLogin,
			Date:        c.Commit.Author.Date,
		}
	}

	return results, nil
}

// -------------------------------------------------------------------------------------
// Reviews & Comments
// -------------------------------------------------------------------------------------

func (s *GitHubService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repoName, prNumber)

	payload := map[string]any{
		"body":      comment.Body,
		"commit_id": comment.CommitID,
		"path":      comment.Path,
		"line":      comment.Line,
		"side":      "RIGHT",
	}

	if comment.StartLine > 0 && comment.StartLine < comment.Line {
		payload["start_line"] = comment.StartLine
		payload["start_side"] = "RIGHT"
	}

	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		StartLine int       `json:"start_line"`
		CommitID  string    `json:"commit_id"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, &resp); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(resp.ID, 10),
		Body:      resp.Body,
		Path:      resp.Path,
		Line:      resp.Line,
		StartLine: resp.StartLine,
		CommitID:  resp.CommitID,
		Author: &types.PullRequestCommentAuthor{
			Username: resp.User.Login,
			Name:     resp.User.Login,
		},
		CreatedAt: resp.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *GitHubService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repoName, prNumber)
	payload := map[string]string{
		"body": body,
	}

	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, &resp); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:   strconv.FormatInt(resp.ID, 10),
		Body: resp.Body,
		Author: &types.PullRequestCommentAuthor{
			Username: resp.User.Login,
			Name:     resp.User.Login,
		},
		CreatedAt: resp.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *GitHubService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/comments/%s", owner, repoName, commentID)
	payload := map[string]string{
		"body": body,
	}

	return s.executeRequest(ctx, token, http.MethodPatch, apiPath, payload, nil)
}

func (s *GitHubService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments/%s/replies", owner, repoName, prNumber, parentCommentID)
	payload := map[string]string{
		"body": body,
	}

	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		CommitID  string    `json:"commit_id"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, &resp); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(resp.ID, 10),
		Body:      resp.Body,
		Path:      resp.Path,
		Line:      resp.Line,
		CommitID:  resp.CommitID,
		Author: &types.PullRequestCommentAuthor{
			Username: resp.User.Login,
			Name:     resp.User.Login,
		},
		CreatedAt: resp.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/comments/%s", owner, repoName, commentID)
	payload := map[string]string{
		"body": body,
	}

	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodPatch, apiPath, payload, &resp); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(resp.ID, 10),
		Body:      resp.Body,
		Path:      resp.Path,
		Line:      resp.Line,
		Author: &types.PullRequestCommentAuthor{
			Username: resp.User.Login,
			Name:     resp.User.Login,
		},
		CreatedAt: resp.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/comments/%s", owner, repoName, commentID)

	var resp struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		CommitID  string    `json:"commit_id"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &resp); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(resp.ID, 10),
		Body:      resp.Body,
		Path:      resp.Path,
		Line:      resp.Line,
		CommitID:  resp.CommitID,
		Author: &types.PullRequestCommentAuthor{
			Username: resp.User.Login,
			Name:     resp.User.Login,
		},
		CreatedAt: resp.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=100", owner, repoName, prNumber)

	var rawComments []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		CommitID  string    `json:"commit_id"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawComments); err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestReviewComment, len(rawComments))
	for i, c := range rawComments {
		results[i] = &types.PullRequestReviewComment{
			ID:       strconv.FormatInt(c.ID, 10),
			Body:     c.Body,
			Path:     c.Path,
			Line:     c.Line,
			CommitID: c.CommitID,
			Author: &types.PullRequestCommentAuthor{
				Username: c.User.Login,
				Name:     c.User.Login,
			},
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		}
	}

	return results, nil
}

func (s *GitHubService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	reviewComments, err := s.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}

	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", owner, repoName, prNumber)

	var issueComments []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &issueComments); err != nil {
		return nil, err
	}

	for _, c := range issueComments {
		reviewComments = append(reviewComments, &types.PullRequestReviewComment{
			ID:   strconv.FormatInt(c.ID, 10),
			Body: c.Body,
			Author: &types.PullRequestCommentAuthor{
				Username: c.User.Login,
				Name:     c.User.Login,
			},
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		})
	}

	sort.Slice(reviewComments, func(i, j int) bool {
		return reviewComments[i].CreatedAt < reviewComments[j].CreatedAt
	})

	return reviewComments, nil
}

func (s *GitHubService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	return s.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
}

func (s *GitHubService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	// GitHub GraphQL mutation to resolve review thread
	mutation := `
		mutation($threadId: ID!) {
			resolveReviewThread(input: {threadId: $threadId}) {
				thread {
					isResolved
				}
			}
		}
	`

	payload := map[string]any{
		"query": mutation,
		"variables": map[string]any{
			"threadId": threadID,
		},
	}

	return s.executeRequest(ctx, token, http.MethodPost, s.graphqlURL, payload, nil)
}

func (s *GitHubService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	if reason == "" {
		reason = "OUTDATED"
	}

	mutation := `
		mutation($subjectId: ID!, $classifier: ReportedContentClassifiers!) {
			minimizeComment(input: {subjectId: $subjectId, classifier: $classifier}) {
				minimizedComment {
					isMinimized
				}
			}
		}
	`

	payload := map[string]any{
		"query": mutation,
		"variables": map[string]any{
			"subjectId":  commentID,
			"classifier": strings.ToUpper(reason),
		},
	}

	return s.executeRequest(ctx, token, http.MethodPost, s.graphqlURL, payload, nil)
}

func (s *GitHubService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	payload := map[string]string{
		"event": "APPROVE",
		"body":  message,
	}

	return s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, nil)
}

func (s *GitHubService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	payload := map[string]string{
		"event": "REQUEST_CHANGES",
		"body":  message,
	}

	return s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, nil)
}

func (s *GitHubService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return err
	}

	if method == "" {
		method = "squash"
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repoName, prNumber)
	payload := map[string]string{
		"merge_method": method,
	}

	return s.executeRequest(ctx, token, http.MethodPut, apiPath, payload, nil)
}

func (s *GitHubService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)

	var rawReviews []struct {
		State string `json:"state"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawReviews); err != nil {
		return nil, err
	}

	valid := make([]string, 0, len(rawReviews))
	for _, r := range rawReviews {
		if r.State != "DISMISSED" {
			valid = append(valid, r.State)
		}
	}

	return valid, nil
}

func (s *GitHubService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "open", "", "")
	if err != nil {
		return nil, err
	}

	var results []types.PullRequestsWithChangesRequested
	for _, pr := range prs {
		reviews, err := s.GetListOfValidReviews(ctx, orgData, repo, pr.Number)
		if err != nil {
			continue
		}

		hasChangesRequested := false
		for _, r := range reviews {
			if r == "CHANGES_REQUESTED" {
				hasChangesRequested = true
				break
			}
		}

		if hasChangesRequested {
			results = append(results, types.PullRequestsWithChangesRequested{
				PullRequest:      *pr,
				ChangesRequested: true,
			})
		}
	}

	return results, nil
}

func (s *GitHubService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	reviews, err := s.GetListOfValidReviews(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}

	approvedCount := 0
	for _, r := range reviews {
		if r == "CHANGES_REQUESTED" {
			return false, nil
		}
		if r == "APPROVED" {
			approvedCount++
		}
	}

	return approvedCount > 0, nil
}

// -------------------------------------------------------------------------------------
// Reactions
// -------------------------------------------------------------------------------------

func (s *GitHubService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)

	var rawReactions []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawReactions); err != nil {
		return nil, err
	}

	reactionCount := make(map[string]int)
	for _, r := range rawReactions {
		reactionCount[r.Content]++
	}

	result := make([]types.ReactionsInComments, 0, len(reactionCount))
	for reaction, count := range reactionCount {
		result = append(result, types.ReactionsInComments{
			Reaction: reaction,
			Count:    count,
		})
	}

	return result, nil
}

func (s *GitHubService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)
	payload := map[string]string{
		"content": reaction,
	}

	return s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, nil)
}

func (s *GitHubService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repoName, commentID)
	payload := map[string]string{
		"content": reaction,
	}

	return s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, nil)
}

func (s *GitHubService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)

	var rawReactions []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawReactions); err != nil {
		return err
	}

	filterMap := make(map[string]bool)
	for _, r := range reactions {
		filterMap[r] = true
	}

	for _, rx := range rawReactions {
		if filterMap[rx.Content] {
			delPath := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions/%d", owner, repoName, prNumber, rx.ID)
			_ = s.executeRequest(ctx, token, http.MethodDelete, delPath, nil, nil)
		}
	}

	return nil
}

func (s *GitHubService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repoName, commentID)

	var rawReactions []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawReactions); err != nil {
		return err
	}

	filterMap := make(map[string]bool)
	for _, r := range reactions {
		filterMap[r] = true
	}

	for _, rx := range rawReactions {
		if filterMap[rx.Content] {
			delPath := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions/%d", owner, repoName, commentID, rx.ID)
			_ = s.executeRequest(ctx, token, http.MethodDelete, delPath, nil, nil)
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Users, Members & Organizations
// -------------------------------------------------------------------------------------

func (s *GitHubService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	prs, err := s.GetPullRequests(ctx, orgData, nil, "all", "", "")
	if err != nil {
		return nil, err
	}

	authorMap := make(map[string]types.PullRequestAuthor)
	for _, pr := range prs {
		if _, exists := authorMap[pr.Author]; !exists {
			isBot := false
			if determineBots {
				isBot = strings.HasSuffix(strings.ToLower(pr.Author), "[bot]") || strings.Contains(strings.ToLower(pr.Author), "bot")
			}
			authorMap[pr.Author] = types.PullRequestAuthor{
				Username: pr.Author,
				IsBot:    isBot,
			}
		}
	}

	results := make([]types.PullRequestAuthor, 0, len(authorMap))
	for _, a := range authorMap {
		results = append(results, a)
	}

	return results, nil
}

func (s *GitHubService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner := ""
	if orgData.IntegrationCredentials != nil {
		if org, ok := orgData.IntegrationCredentials["org"].(string); ok && org != "" {
			owner = org
		}
	}

	if owner == "" {
		return nil, errors.New("organization name required for listing members")
	}

	apiPath := fmt.Sprintf("/orgs/%s/members?per_page=100", owner)

	var members []struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &members); err != nil {
		return nil, err
	}

	results := make([]types.PullRequestAuthor, len(members))
	for i, m := range members {
		results[i] = types.PullRequestAuthor{
			Username: m.Login,
			IsBot:    m.Type == "Bot",
		}
	}

	return results, nil
}

func (s *GitHubService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/users/%s", username)

	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &u); err != nil {
		return nil, err
	}

	return &types.PullRequestUser{
		ID:        strconv.FormatInt(u.ID, 10),
		Username:  u.Login,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		IsBot:     u.Type == "Bot",
	}, nil
}

func (s *GitHubService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	results := make(map[string]*types.PullRequestUser)
	for _, u := range usernames {
		user, err := s.GetUserByUsername(ctx, orgData, u)
		if err == nil && user != nil {
			results[u] = user
		}
	}
	return results, nil
}

func (s *GitHubService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	if userName != "" {
		user, err := s.GetUserByUsername(ctx, orgData, userName)
		if err == nil && user != nil {
			return user, nil
		}
	}
	return nil, fmt.Errorf("user not found for email %q or name %q", email, userName)
}

func (s *GitHubService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/user/%s", userID)

	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &u); err != nil {
		return nil, err
	}

	return &types.PullRequestUser{
		ID:        strconv.FormatInt(u.ID, 10),
		Username:  u.Login,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		IsBot:     u.Type == "Bot",
	}, nil
}

func (s *GitHubService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	apiPath := "/user"

	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &u); err != nil {
		return nil, err
	}

	return &types.PullRequestUser{
		ID:        strconv.FormatInt(u.ID, 10),
		Username:  u.Login,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		IsBot:     u.Type == "Bot",
	}, nil
}

func (s *GitHubService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	apiPath := "/user/orgs?per_page=100"

	var orgs []struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &orgs); err != nil {
		return nil, err
	}

	results := make([]*types.Organization, len(orgs))
	for i, o := range orgs {
		results[i] = &types.Organization{
			ID:   strconv.FormatInt(o.ID, 10),
			Name: o.Login,
		}
	}

	return results, nil
}

func (s *GitHubService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, repo)
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/languages", owner, repoName)
	languages := make(map[string]int)

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &languages); err != nil {
		return nil, err
	}

	return languages, nil
}

// -------------------------------------------------------------------------------------
// Files Upload & Pull Request Creation
// -------------------------------------------------------------------------------------

func (s *GitHubService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return nil, err
	}

	if targetBranch == "" {
		targetBranch, _ = s.GetDefaultBranch(ctx, orgData, &repo)
	}

	// 1. Upload files to source branch
	ok, err := s.UploadFiles(ctx, orgData, repo, sourceBranch, targetBranch, commitMessage, author, files)
	if err != nil || !ok {
		return nil, fmt.Errorf("failed to upload files to branch %q: %w", sourceBranch, err)
	}

	// 2. Create Pull Request
	apiPath := fmt.Sprintf("/repos/%s/%s/pulls", owner, repoName)
	payload := map[string]any{
		"title": title,
		"body":  description,
		"head":  sourceBranch,
		"base":  targetBranch,
	}

	var pr struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		HTMLURL string `json:"html_url"`
		Head   struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"base"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := s.executeRequest(ctx, token, http.MethodPost, apiPath, payload, &pr); err != nil {
		return nil, err
	}

	return &types.PullRequest{
		Number:       pr.Number,
		PullNumber:   pr.Number,
		Title:        pr.Title,
		Body:         pr.Body,
		Description:  pr.Body,
		State:        pr.State,
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		HeadSHA:      pr.Head.SHA,
		BaseSHA:      pr.Base.SHA,
		URL:          pr.HTMLURL,
		PRURL:        pr.HTMLURL,
		CreatedAt:    pr.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *GitHubService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return false, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return false, err
	}

	if baseBranch == "" {
		baseBranch, _ = s.GetDefaultBranch(ctx, orgData, &repo)
	}

	// 1. Get base branch SHA
	refPath := fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repoName, baseBranch)
	var refData struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := s.executeRequest(ctx, token, http.MethodGet, refPath, nil, &refData); err != nil {
		return false, fmt.Errorf("failed to get base ref %s: %w", baseBranch, err)
	}
	baseSHA := refData.Object.SHA

	// 2. Create blobs
	type treeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	var treeEntries []treeEntry

	for _, f := range files {
		blobPath := fmt.Sprintf("/repos/%s/%s/git/blobs", owner, repoName)
		blobPayload := map[string]string{
			"content":  base64.StdEncoding.EncodeToString([]byte(f.Content)),
			"encoding": "base64",
		}
		var blobResp struct {
			SHA string `json:"sha"`
		}
		if err := s.executeRequest(ctx, token, http.MethodPost, blobPath, blobPayload, &blobResp); err != nil {
			return false, fmt.Errorf("failed to create blob for %s: %w", f.Path, err)
		}

		treeEntries = append(treeEntries, treeEntry{
			Path: strings.TrimPrefix(f.Path, "/"),
			Mode: "100644",
			Type: "blob",
			SHA:  blobResp.SHA,
		})
	}

	// 3. Create tree
	treePath := fmt.Sprintf("/repos/%s/%s/git/trees", owner, repoName)
	treePayload := map[string]any{
		"base_tree": baseSHA,
		"tree":      treeEntries,
	}
	var newTreeResp struct {
		SHA string `json:"sha"`
	}
	if err := s.executeRequest(ctx, token, http.MethodPost, treePath, treePayload, &newTreeResp); err != nil {
		return false, fmt.Errorf("failed to create git tree: %w", err)
	}

	// 4. Create commit
	commitPath := fmt.Sprintf("/repos/%s/%s/git/commits", owner, repoName)
	commitPayload := map[string]any{
		"message": message,
		"tree":    newTreeResp.SHA,
		"parents": []string{baseSHA},
	}
	var newCommitResp struct {
		SHA string `json:"sha"`
	}
	if err := s.executeRequest(ctx, token, http.MethodPost, commitPath, commitPayload, &newCommitResp); err != nil {
		return false, fmt.Errorf("failed to create commit: %w", err)
	}

	// 5. Create or update branch reference
	branchRefPath := fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", owner, repoName, branchName)
	updatePayload := map[string]any{
		"sha":   newCommitResp.SHA,
		"force": true,
	}
	if err := s.executeRequest(ctx, token, http.MethodPatch, branchRefPath, updatePayload, nil); err != nil {
		// Try creating the ref if it doesn't exist
		createRefPath := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repoName)
		createPayload := map[string]any{
			"ref": "refs/heads/" + branchName,
			"sha": newCommitResp.SHA,
		}
		if err2 := s.executeRequest(ctx, token, http.MethodPost, createRefPath, createPayload, nil); err2 != nil {
			return false, fmt.Errorf("failed to create branch ref %s: %w", branchName, err2)
		}
	}

	return true, nil
}

// -------------------------------------------------------------------------------------
// Verification, Clone Params & Webhooks
// -------------------------------------------------------------------------------------

func (s *GitHubService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return &types.CodeManagementConnectionStatus{
			IsConnected: false,
			Message:     err.Error(),
		}, nil
	}

	var u struct {
		Login string `json:"login"`
	}
	if err := s.executeRequest(ctx, token, http.MethodGet, "/user", nil, &u); err != nil {
		return &types.CodeManagementConnectionStatus{
			IsConnected: false,
			Message:     err.Error(),
		}, nil
	}

	return &types.CodeManagementConnectionStatus{
		IsConnected: true,
		Message:     fmt.Sprintf("Authenticated as %s", u.Login),
	}, nil
}

func (s *GitHubService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	return s.resolveToken(ctx, orgData)
}

func (s *GitHubService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := s.resolveOwnerRepo(orgData, &repo)
	if err != nil {
		return nil, err
	}

	cleanHost := strings.TrimPrefix(s.baseURL, "https://")
	cleanHost = strings.TrimPrefix(cleanHost, "http://")
	if cleanHost == "api.github.com" {
		cleanHost = "github.com"
	}

	authenticatedURL := fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", token, cleanHost, owner, repoName)

	return &types.GitCloneParams{
		URL:      authenticatedURL,
		Token:    token,
		Username: "x-access-token",
	}, nil
}

func (s *GitHubService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return err
	}

	owner := ""
	if orgData.IntegrationCredentials != nil {
		if org, ok := orgData.IntegrationCredentials["org"].(string); ok {
			owner = org
		}
	}

	if owner == "" {
		return errors.New("organization required for webhook deletion")
	}

	apiPath := fmt.Sprintf("/orgs/%s/hooks", owner)

	var hooks []struct {
		ID     int64 `json:"id"`
		Config struct {
			URL string `json:"url"`
		} `json:"config"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &hooks); err != nil {
		return err
	}

	for _, h := range hooks {
		if strings.Contains(h.Config.URL, "scandrix") {
			delPath := fmt.Sprintf("/orgs/%s/hooks/%d", owner, h.ID)
			_ = s.executeRequest(ctx, token, http.MethodDelete, delPath, nil, nil)
		}
	}

	return nil
}

func (s *GitHubService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return false, err
	}

	apiPath := fmt.Sprintf("/repos/%s/hooks", repositoryID)

	var hooks []struct {
		Active bool `json:"active"`
		Config struct {
			URL string `json:"url"`
		} `json:"config"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &hooks); err != nil {
		return false, err
	}

	for _, h := range hooks {
		if strings.Contains(h.Config.URL, "scandrix") && h.Active {
			return true, nil
		}
	}

	return false, nil
}

// -------------------------------------------------------------------------------------
// Issues API
// -------------------------------------------------------------------------------------

func (s *GitHubService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return true, nil
}

func (s *GitHubService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	token, err := s.resolveToken(ctx, params.OrganizationAndTeamData)
	if err != nil {
		return nil, err
	}

	state := "open"
	if params.Filters != nil && params.Filters.State != "" {
		state = params.Filters.State
	}

	owner := params.Repository.Owner
	repoName := params.Repository.Name
	if owner == "" {
		owner, repoName, _ = s.resolveOwnerRepo(params.OrganizationAndTeamData, nil)
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=100", owner, repoName, state)

	var rawIssues []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		HTMLURL   string    `json:"html_url"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawIssues); err != nil {
		return nil, err
	}

	results := make([]types.CodeManagementIssue, 0, len(rawIssues))
	for _, issue := range rawIssues {
		if issue.PullRequest != nil {
			continue // Skip pull requests returned as issues
		}
		bodyStr := issue.Body
		results = append(results, types.CodeManagementIssue{
			Number:      issue.Number,
			Title:       issue.Title,
			Body:        &bodyStr,
			Description: issue.Body,
			State:       issue.State,
			URL:         issue.HTMLURL,
			CreatedAt:   issue.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   issue.UpdatedAt.Format(time.RFC3339),
			Platform:    models.ProviderGitHub,
		})
	}

	return results, nil
}

func (s *GitHubService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	token, err := s.resolveToken(ctx, params.OrganizationAndTeamData)
	if err != nil {
		return nil, err
	}

	owner := params.Repository.Owner
	repoName := params.Repository.Name
	if owner == "" {
		owner, repoName, _ = s.resolveOwnerRepo(params.OrganizationAndTeamData, nil)
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repoName, params.IssueNumber)

	var issue struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		HTMLURL   string    `json:"html_url"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &issue); err != nil {
		return nil, err
	}

	bodyStr := issue.Body
	return &types.CodeManagementIssue{
		Number:      issue.Number,
		Title:       issue.Title,
		Body:        &bodyStr,
		Description: issue.Body,
		State:       issue.State,
		URL:         issue.HTMLURL,
		CreatedAt:   issue.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   issue.UpdatedAt.Format(time.RFC3339),
		Platform:    models.ProviderGitHub,
	}, nil
}

// -------------------------------------------------------------------------------------
// Review Comment Formatting
// -------------------------------------------------------------------------------------

func (s *GitHubService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
	var sb strings.Builder

	if includeHeader {
		sb.WriteString("<!-- drixy-codereview -->\n")
		sb.WriteString("### 🛡️ ScanDrix AI Review Suggestion\n\n")
	}

	if text, ok := suggestion.(string); ok {
		sb.WriteString(text)
	} else if data, err := json.MarshalIndent(suggestion, "", "  "); err == nil {
		sb.WriteString("```json\n")
		sb.WriteString(string(data))
		sb.WriteString("\n```")
	}

	if includeFooter {
		sb.WriteString("\n\n---\n*Automated review by [ScanDrix](https://scandrix.dev)*")
	}

	return sb.String()
}

func (s *GitHubService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	if m, ok := payload.(map[string]any); ok {
		if pr, ok := m["pull_request"].(map[string]any); ok {
			if u, ok := pr["user"].(map[string]any); ok {
				var idStr string
				if id, ok := u["id"].(float64); ok {
					idStr = strconv.FormatInt(int64(id), 10)
				}
				login, _ := u["login"].(string)
				avatar, _ := u["avatar_url"].(string)
				return &types.PullRequestUser{
					ID:        idStr,
					Login:     login,
					Username:  login,
					AvatarURL: avatar,
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *GitHubService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	if limit <= 0 {
		limit = 100
	}
	token, err := s.resolveToken(ctx, orgData)
	if err != nil {
		return nil, err
	}

	owner, repoName, _ := s.resolveOwnerRepo(orgData, &repo)
	apiPath := fmt.Sprintf("/repos/%s/%s/pulls/comments?sort=created&direction=desc&per_page=%d", owner, repoName, limit)

	var rawComments []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}

	if err := s.executeRequest(ctx, token, http.MethodGet, apiPath, nil, &rawComments); err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestReviewComment, len(rawComments))
	for i, rc := range rawComments {
		results[i] = &types.PullRequestReviewComment{
			ID:   strconv.FormatInt(rc.ID, 10),
			Body: rc.Body,
			Path: rc.Path,
			Author: &types.PullRequestCommentAuthor{
				Username: rc.User.Login,
				Name:     rc.User.Login,
			},
			CreatedAt: rc.CreatedAt.Format(time.RFC3339),
		}
	}

	return results, nil
}

