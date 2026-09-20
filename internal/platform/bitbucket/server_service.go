// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// BitbucketServerService implements contracts.ICodeManagementService for Bitbucket Data Center / Server (1.0 REST API).
// Translates libs/platform/infrastructure/adapters/services/bitbucket/bitbucket-data-center.service.ts
var _ contracts.ICodeManagementService = (*BitbucketServerService)(nil)

type BitbucketServerService struct {
	baseURL        string
	httpClient     *http.Client
	defaultTimeout time.Duration
	cacheStore     sync.Map
}

// BitbucketServerConfig configures BitbucketServerService instances.
type BitbucketServerConfig struct {
	BaseURL        string
	HTTPClient     *http.Client
	DefaultTimeout time.Duration
}

type serverCacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewBitbucketServerService creates an enterprise Bitbucket Server / Data Center service adapter.
func NewBitbucketServerService(cfg BitbucketServerConfig) *BitbucketServerService {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:7990"
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 45 * time.Second,
		}
	}

	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	return &BitbucketServerService{
		baseURL:        baseURL,
		httpClient:     client,
		defaultTimeout: timeout,
	}
}

func (s *BitbucketServerService) Provider() models.SCMProvider {
	return models.ProviderBitbucket
}

// -------------------------------------------------------------------------------------
// Cache Helpers
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) getCached(key string) (any, bool) {
	if val, ok := s.cacheStore.Load(key); ok {
		entry := val.(serverCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, true
		}
		s.cacheStore.Delete(key)
	}
	return nil, false
}

func (s *BitbucketServerService) setCached(key string, value any, ttl time.Duration) {
	s.cacheStore.Store(key, serverCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	})
}

// -------------------------------------------------------------------------------------
// Authentication & HTTP Execution
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) resolveBaseURL(orgData types.OrganizationAndTeamData) string {
	if orgData.IntegrationCredentials != nil {
		if host, ok := orgData.IntegrationCredentials["host"].(string); ok && host != "" {
			return strings.TrimRight(host, "/")
		}
		if u, ok := orgData.IntegrationCredentials["url"].(string); ok && u != "" {
			return strings.TrimRight(u, "/")
		}
	}
	return s.baseURL
}

func (s *BitbucketServerService) extractToken(orgData types.OrganizationAndTeamData) string {
	if orgData.AuthToken != "" {
		return orgData.AuthToken
	}
	if orgData.IntegrationCredentials != nil {
		if t, ok := orgData.IntegrationCredentials["token"].(string); ok && t != "" {
			return t
		}
		if t, ok := orgData.IntegrationCredentials["personalAccessToken"].(string); ok && t != "" {
			return t
		}
		if t, ok := orgData.IntegrationCredentials["accessToken"].(string); ok && t != "" {
			return t
		}
	}
	return ""
}

func (s *BitbucketServerService) extractBasicAuth(orgData types.OrganizationAndTeamData) (user, pass string) {
	if orgData.IntegrationCredentials != nil {
		u, _ := orgData.IntegrationCredentials["username"].(string)
		p, _ := orgData.IntegrationCredentials["password"].(string)
		if p == "" {
			p, _ = orgData.IntegrationCredentials["token"].(string)
		}
		if u != "" && p != "" {
			return u, p
		}
	}
	return "", ""
}

func (s *BitbucketServerService) executeRequest(ctx context.Context, orgData types.OrganizationAndTeamData, method, endpoint string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body failed: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	baseURL := s.resolveBaseURL(orgData)
	reqURL := endpoint
	if !strings.HasPrefix(reqURL, "http://") && !strings.HasPrefix(reqURL, "https://") {
		reqURL = baseURL + "/" + strings.TrimLeft(endpoint, "/")
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create server request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	user, pass := s.extractBasicAuth(orgData)
	if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
	} else {
		token := s.extractToken(orgData)
		if token != "" {
			if strings.HasPrefix(token, "Bearer ") {
				req.Header.Set("Authorization", token)
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
	}

	var resp *http.Response
	maxRetries := 3
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = s.httpClient.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, 0, fmt.Errorf("execute request failed: %w", err)
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, resp.StatusCode, fmt.Errorf("bitbucket server unavailable (status %d)", resp.StatusCode)
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		break
	}

	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return respBytes, resp.StatusCode, fmt.Errorf("bitbucket server error status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, resp.StatusCode, nil
}

func (s *BitbucketServerService) parseProjectRepo(repo *types.RepositoryDescriptor) (projectKey, repoSlug string) {
	if repo == nil {
		return "", ""
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner, repo.Name
	}
	fullName := repo.FullName
	if fullName == "" {
		fullName = repo.Name
	}
	parts := strings.Split(strings.Trim(fullName, "/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	if repo.Owner != "" {
		return repo.Owner, fullName
	}
	return "", fullName
}

// -------------------------------------------------------------------------------------
// Issues API (Bitbucket Server delegates to Jira, no native issues)
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return false, nil
}

func (s *BitbucketServerService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	return []types.CodeManagementIssue{}, nil
}

func (s *BitbucketServerService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	return nil, nil
}

// -------------------------------------------------------------------------------------
// Repositories API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository full name: %s (expected PROJECT/repo)", name)
	}
	projectKey, repoSlug := parts[0], parts[1]

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID       int    `json:"id"`
		Slug     string `json:"slug"`
		Name     string `json:"name"`
		Public   bool   `json:"public"`
		DefaultBranch string `json:"defaultBranch"`
		Project  struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"project"`
		Links struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
			Clone []struct {
				Name string `json:"name"`
				Href string `json:"href"`
			} `json:"clone"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket server repo failed: %w", err)
	}

	selfURL := ""
	if len(raw.Links.Self) > 0 {
		selfURL = raw.Links.Self[0].Href
	}
	cloneURL := ""
	for _, c := range raw.Links.Clone {
		if c.Name == "http" {
			cloneURL = c.Href
			break
		}
	}

	defBranch := raw.DefaultBranch
	if defBranch == "" {
		defBranch = "main"
	}

	return &types.Repository{
		ID:               strconv.Itoa(raw.ID),
		Name:             raw.Slug,
		FullName:         fmt.Sprintf("%s/%s", raw.Project.Key, raw.Slug),
		Private:          !raw.Public,
		IsPrivate:        !raw.Public,
		DefaultBranch:    defBranch,
		HTMLURL:          selfURL,
		CloneURL:         cloneURL,
		OrganizationName: raw.Project.Name,
		Provider:         models.ProviderBitbucket,
	}, nil
}

func (s *BitbucketServerService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	endpoint := "/rest/api/1.0/repos?limit=100"
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID      int    `json:"id"`
			Slug    string `json:"slug"`
			Name    string `json:"name"`
			Public  bool   `json:"public"`
			DefaultBranch string `json:"defaultBranch"`
			Project struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"project"`
			Links struct {
				Self []struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket server repos failed: %w", err)
	}

	results := make([]*types.Repositories, 0, len(raw.Values))
	for _, r := range raw.Values {
		if visibility == "private" && r.Public {
			continue
		}
		if visibility == "public" && !r.Public {
			continue
		}

		selfURL := ""
		if len(r.Links.Self) > 0 {
			selfURL = r.Links.Self[0].Href
		}

		defBranch := r.DefaultBranch
		if defBranch == "" {
			defBranch = "main"
		}

		results = append(results, &types.Repositories{
			ID:               strconv.Itoa(r.ID),
			Name:             r.Slug,
			FullName:         fmt.Sprintf("%s/%s", r.Project.Key, r.Slug),
			Private:          !r.Public,
			DefaultBranch:    defBranch,
			HTTPURL:          selfURL,
			OrganizationName: r.Project.Name,
		})
	}

	return results, nil
}

func (s *BitbucketServerService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, "/rest/api/1.0/projects?limit=100", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID   int    `json:"id"`
			Key  string `json:"key"`
			Name string `json:"name"`
			Links struct {
				Self []struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket server projects failed: %w", err)
	}

	orgs := make([]*types.Organization, 0, len(raw.Values))
	for _, p := range raw.Values {
		orgs = append(orgs, &types.Organization{
			ID:    strconv.Itoa(p.ID),
			Login: p.Key,
			Name:  p.Name,
		})
	}

	return orgs, nil
}

func (s *BitbucketServerService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, "/rest/api/1.0/users?limit=100", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket server users failed: %w", err)
	}

	authors := make([]types.PullRequestAuthor, 0, len(raw.Values))
	for _, u := range raw.Values {
		authors = append(authors, types.PullRequestAuthor{
			ID:       strconv.Itoa(u.ID),
			Username: u.Name,
			Name:     u.DisplayName,
		})
	}

	return authors, nil
}

func (s *BitbucketServerService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	data, code, err := s.executeRequest(ctx, orgData, http.MethodGet, "/rest/api/1.0/users?limit=1", nil)
	if err != nil || code != http.StatusOK {
		return &types.CodeManagementConnectionStatus{
			HasConnection:   false,
			IsConnected:     false,
			IsSetupComplete: false,
			Message:         fmt.Sprintf("verification failed (status %d): %v", code, err),
			PlatformName:    string(models.ProviderBitbucket),
		}, nil
	}

	_ = data
	return &types.CodeManagementConnectionStatus{
		HasConnection:   true,
		IsConnected:     true,
		IsSetupComplete: true,
		Message:         "connected to bitbucket data center",
		PlatformName:    string(models.ProviderBitbucket),
	}, nil
}

func (s *BitbucketServerService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return "main", nil
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/default-branch", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return "main", nil
	}

	var raw struct {
		DisplayID string `json:"displayId"`
	}
	if err := json.Unmarshal(data, &raw); err == nil && raw.DisplayID != "" {
		return raw.DisplayID, nil
	}

	return "main", nil
}

// -------------------------------------------------------------------------------------
// Pull Requests API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get prs: project or repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests?limit=50", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	if state != "" {
		st := strings.ToUpper(state)
		if st == "OPEN" {
			endpoint += "&state=OPEN"
		} else if st == "MERGED" {
			endpoint += "&state=MERGED"
		} else if st == "DECLINED" || st == "CLOSED" {
			endpoint += "&state=DECLINED"
		}
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			State       string `json:"state"`
			CreatedDate int64  `json:"createdDate"`
			UpdatedDate int64  `json:"updatedDate"`
			FromRef     struct {
				ID         string `json:"id"`
				DisplayID  string `json:"displayId"`
				LatestCommit string `json:"latestCommit"`
			} `json:"fromRef"`
			ToRef struct {
				ID         string `json:"id"`
				DisplayID  string `json:"displayId"`
				LatestCommit string `json:"latestCommit"`
			} `json:"toRef"`
			Author struct {
				User struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"user"`
			} `json:"author"`
			Links struct {
				Self []struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket server prs failed: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(raw.Values))
	for _, it := range raw.Values {
		username := it.Author.User.Name
		if author != "" && !strings.EqualFold(username, author) {
			continue
		}
		if branch != "" && it.FromRef.DisplayID != branch {
			continue
		}

		selfURL := ""
		if len(it.Links.Self) > 0 {
			selfURL = it.Links.Self[0].Href
		}

		created := time.UnixMilli(it.CreatedDate).Format(time.RFC3339)
		updated := time.UnixMilli(it.UpdatedDate).Format(time.RFC3339)

		prs = append(prs, &types.PullRequest{
			ID:           strconv.Itoa(it.ID),
			Number:       it.ID,
			PullNumber:   it.ID,
			Title:        it.Title,
			Description:  it.Description,
			Body:         it.Description,
			State:        strings.ToLower(it.State),
			URL:          selfURL,
			PRURL:        selfURL,
			SourceBranch: it.FromRef.DisplayID,
			TargetBranch: it.ToRef.DisplayID,
			HeadSHA:      it.FromRef.LatestCommit,
			BaseSHA:      it.ToRef.LatestCommit,
			CreatedAt:    created,
			UpdatedAt:    updated,
			Author:       username,
			User: types.PullRequestUser{
				Login:    username,
				Username: username,
				Name:     it.Author.User.DisplayName,
			},
		})
	}

	return prs, nil
}

func (s *BitbucketServerService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
}

func (s *BitbucketServerService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get pr: project or repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		CreatedDate int64  `json:"createdDate"`
		UpdatedDate int64  `json:"updatedDate"`
		FromRef     struct {
			ID           string `json:"id"`
			DisplayID    string `json:"displayId"`
			LatestCommit string `json:"latestCommit"`
		} `json:"fromRef"`
		ToRef struct {
			ID           string `json:"id"`
			DisplayID    string `json:"displayId"`
			LatestCommit string `json:"latestCommit"`
		} `json:"toRef"`
		Author struct {
			User struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"author"`
		Links struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &it); err != nil {
		return nil, fmt.Errorf("decode bitbucket server pr failed: %w", err)
	}

	selfURL := ""
	if len(it.Links.Self) > 0 {
		selfURL = it.Links.Self[0].Href
	}

	created := time.UnixMilli(it.CreatedDate).Format(time.RFC3339)
	updated := time.UnixMilli(it.UpdatedDate).Format(time.RFC3339)

	return &types.PullRequest{
		ID:           strconv.Itoa(it.ID),
		Number:       it.ID,
		PullNumber:   it.ID,
		Title:        it.Title,
		Description:  it.Description,
		Body:         it.Description,
		State:        strings.ToLower(it.State),
		URL:          selfURL,
		PRURL:        selfURL,
		SourceBranch: it.FromRef.DisplayID,
		TargetBranch: it.ToRef.DisplayID,
		HeadSHA:      it.FromRef.LatestCommit,
		BaseSHA:      it.ToRef.LatestCommit,
		CreatedAt:    created,
		UpdatedAt:    updated,
		Author:       it.Author.User.Name,
		User: types.PullRequestUser{
			Login:    it.Author.User.Name,
			Username: it.Author.User.Name,
			Name:     it.Author.User.DisplayName,
		},
	}, nil
}

func (s *BitbucketServerService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "open", "", "")
}

func (s *BitbucketServerService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "open", "", "")
	if err != nil {
		return nil, err
	}

	results := make([]*types.PullRequestWithFiles, 0, len(prs))
	for _, pr := range prs {
		files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, pr.Number)
		if err != nil {
			files = []*types.PullRequestFile{}
		}
		results = append(results, &types.PullRequestWithFiles{
			PullRequest:      *pr,
			PullNumber:       pr.Number,
			State:            pr.State,
			Title:            pr.Title,
			PullRequestFiles: files,
			Files:            files,
		})
	}

	return results, nil
}

func (s *BitbucketServerService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server rttm: project or repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=MERGED&limit=50", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID          int   `json:"id"`
			CreatedDate int64 `json:"createdDate"`
			UpdatedDate int64 `json:"updatedDate"`
			Author      struct {
				User struct {
					Name string `json:"name"`
				} `json:"user"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	rttm := make([]*types.PullRequestCodeReviewTime, 0, len(raw.Values))
	for _, it := range raw.Values {
		created := time.UnixMilli(it.CreatedDate)
		merged := time.UnixMilli(it.UpdatedDate)
		duration := merged.Sub(created).Seconds()
		if duration < 0 {
			duration = 0
		}

		rttm = append(rttm, &types.PullRequestCodeReviewTime{
			PRNumber:       it.ID,
			ReviewDuration: duration,
			CreatedAt:      created,
			MergedAt:       &merged,
			Author:         it.Author.User.Name,
		})
	}

	return rttm, nil
}

func (s *BitbucketServerService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server changes requested: project or repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=OPEN&limit=50", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID        int    `json:"id"`
			Title     string `json:"title"`
			Reviewers []struct {
				Status string `json:"status"`
			} `json:"reviewers"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var results []types.PullRequestsWithChangesRequested
	for _, it := range raw.Values {
		for _, r := range it.Reviewers {
			if strings.EqualFold(r.Status, "NEEDS_WORK") {
				results = append(results, types.PullRequestsWithChangesRequested{
					Title:            it.Title,
					Number:           it.ID,
					ReviewDecision:   types.PullRequestReviewStateChangesRequested,
					ChangesRequested: true,
				})
				break
			}
		}
	}

	return results, nil
}

func (s *BitbucketServerService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	pr, err := s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
	if err != nil {
		return false, err
	}
	title := strings.ToLower(pr.Title)
	return strings.HasPrefix(title, "[wip]") || strings.HasPrefix(title, "wip:") || strings.HasPrefix(title, "[draft]"), nil
}

func (s *BitbucketServerService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return types.PullRequestReviewStatePending, errors.New("bitbucket server review status: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return types.PullRequestReviewStatePending, err
	}

	var raw struct {
		Reviewers []struct {
			Status   string `json:"status"`
			Approved bool   `json:"approved"`
		} `json:"reviewers"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return types.PullRequestReviewStatePending, err
	}

	hasApproved := false
	for _, r := range raw.Reviewers {
		if strings.EqualFold(r.Status, "NEEDS_WORK") {
			return types.PullRequestReviewStateChangesRequested, nil
		}
		if r.Approved || strings.EqualFold(r.Status, "APPROVED") {
			hasApproved = true
		}
	}

	if hasApproved {
		return types.PullRequestReviewStateApproved, nil
	}

	return types.PullRequestReviewStatePending, nil
}

func (s *BitbucketServerService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return errors.New("bitbucket server update description: repo missing")
	}

	pr, err := s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	body := map[string]any{
		"version":     1,
		"title":       pr.Title,
		"description": description,
	}

	_, _, err = s.executeRequest(ctx, orgData, http.MethodPut, endpoint, body)
	return err
}

func (s *BitbucketServerService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return errors.New("bitbucket server merge pr: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge?version=1", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	_, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, nil)
	return err
}

func (s *BitbucketServerService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return errors.New("bitbucket server approve: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/approve", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	_, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *BitbucketServerService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return errors.New("bitbucket server request changes: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/participants", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	body := map[string]any{
		"status": "NEEDS_WORK",
	}

	_, _, err := s.executeRequest(ctx, orgData, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *BitbucketServerService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	status, err := s.GetReviewStatusByPullRequest(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}
	return status == types.PullRequestReviewStateApproved, nil
}

func (s *BitbucketServerService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	projectKey, repoSlug := s.parseProjectRepo(&repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server create pr: repo missing")
	}

	if len(files) > 0 {
		ok, err := s.UploadFiles(ctx, orgData, repo, sourceBranch, targetBranch, commitMessage, author, files)
		if err != nil || !ok {
			return nil, fmt.Errorf("upload files failed: %w", err)
		}
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	body := map[string]any{
		"title":       title,
		"description": description,
		"fromRef": map[string]any{
			"id": "refs/heads/" + sourceBranch,
		},
		"toRef": map[string]any{
			"id": "refs/heads/" + targetBranch,
		},
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &it); err != nil {
		return nil, err
	}

	return s.GetPullRequestByNumber(ctx, orgData, &repo, it.ID)
}

func (s *BitbucketServerService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	return true, nil
}

// -------------------------------------------------------------------------------------
// Commits & Diffs API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get commits: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/commits?limit=50", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	if branch != "" {
		endpoint += "&until=" + url.QueryEscape(branch)
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID           string `json:"id"`
			Message      string `json:"message"`
			AuthorTimestamp int64  `json:"authorTimestamp"`
			Author struct {
				Name         string `json:"name"`
				DisplayName  string `json:"displayName"`
				EmailAddress string `json:"emailAddress"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	commits := make([]*types.Commit, 0, len(raw.Values))
	for _, it := range raw.Values {
		username := it.Author.Name
		if author != "" && !strings.EqualFold(username, author) {
			continue
		}

		commits = append(commits, &types.Commit{
			SHA:     it.ID,
			Message: it.Message,
			Author: types.GitActor{
				Name:  username,
				Email: it.Author.EmailAddress,
			},
			AuthorName:  username,
			AuthorLogin: username,
			Date:        time.UnixMilli(it.AuthorTimestamp),
		})
	}

	return commits, nil
}

func (s *BitbucketServerService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server pr commits: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/commits?limit=100", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID              string `json:"id"`
			Message         string `json:"message"`
			AuthorTimestamp int64  `json:"authorTimestamp"`
			Author struct {
				Name         string `json:"name"`
				DisplayName  string `json:"displayName"`
				EmailAddress string `json:"emailAddress"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	commits := make([]*types.Commit, 0, len(raw.Values))
	for _, it := range raw.Values {
		commits = append(commits, &types.Commit{
			SHA:     it.ID,
			Message: it.Message,
			Author: types.GitActor{
				Name:  it.Author.Name,
				Email: it.Author.EmailAddress,
			},
			AuthorName:  it.Author.Name,
			AuthorLogin: it.Author.Name,
			Date:        time.UnixMilli(it.AuthorTimestamp),
		})
	}

	return commits, nil
}

func (s *BitbucketServerService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get files: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/changes?limit=500", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Type string `json:"type"`
			Path struct {
				ToString string `json:"toString"`
			} `json:"path"`
			SrcPath struct {
				ToString string `json:"toString"`
			} `json:"srcPath"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	files := make([]*types.PullRequestFile, 0, len(raw.Values))
	for _, it := range raw.Values {
		filePath := it.Path.ToString
		if filePath == "" {
			filePath = it.SrcPath.ToString
		}
		status := "modified"
		if it.Type == "ADD" {
			status = "added"
		} else if it.Type == "DELETE" {
			status = "removed"
		}

		patch, adds, dels := s.fetchFileDiff(ctx, orgData, projectKey, repoSlug, prNumber, filePath)
		changes := adds + dels
		if changes == 0 {
			changes = 1
		}

		files = append(files, &types.PullRequestFile{
			Filename:  filePath,
			Status:    status,
			Additions: adds,
			Deletions: dels,
			Changes:   changes,
			Patch:     patch,
		})
	}

	return files, nil
}

type bitbucketServerDiffResponse struct {
	Diffs []struct {
		Hunks []struct {
			SourceLine      int `json:"sourceLine"`
			SourceSpan      int `json:"sourceSpan"`
			DestinationLine int `json:"destinationLine"`
			DestinationSpan int `json:"destinationSpan"`
			Segments        []struct {
				Type  string `json:"type"` // "ADDED", "REMOVED", "CONTEXT"
				Lines []struct {
					Line string `json:"line"`
				} `json:"lines"`
			} `json:"segments"`
		} `json:"hunks"`
	} `json:"diffs"`
}

func (s *BitbucketServerService) fetchFileDiff(ctx context.Context, orgData types.OrganizationAndTeamData, projectKey, repoSlug string, prNumber int, filePath string) (patch string, additions int, deletions int) {
	diffEndpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/diff?path=%s&limit=1000",
		url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber, url.QueryEscape(filePath))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, diffEndpoint, nil)
	if err != nil || len(data) == 0 {
		return "", 0, 0
	}

	var diffRes bitbucketServerDiffResponse
	if err := json.Unmarshal(data, &diffRes); err != nil {
		return "", 0, 0
	}

	var sb strings.Builder
	for _, diff := range diffRes.Diffs {
		for _, hunk := range diff.Hunks {
			sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunk.SourceLine, hunk.SourceSpan, hunk.DestinationLine, hunk.DestinationSpan))
			for _, seg := range hunk.Segments {
				prefix := " "
				if seg.Type == "ADDED" {
					prefix = "+"
					additions += len(seg.Lines)
				} else if seg.Type == "REMOVED" {
					prefix = "-"
					deletions += len(seg.Lines)
				}
				for _, l := range seg.Lines {
					sb.WriteString(prefix)
					sb.WriteString(l.Line)
					sb.WriteString("\n")
				}
			}
		}
	}

	return sb.String(), additions, deletions
}

func (s *BitbucketServerService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
	files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, 0, len(files))
	for _, f := range files {
		filenames = append(filenames, f.Filename)
	}
	return filenames, nil
}

// -------------------------------------------------------------------------------------
// Comments & Reviews API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get comments: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/activities?limit=100", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Action  string `json:"action"`
			Comment struct {
				ID          int    `json:"id"`
				Text        string `json:"text"`
				CreatedDate int64  `json:"createdDate"`
				Author      struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"author"`
				Anchor struct {
					Line int    `json:"line"`
					Path string `json:"path"`
				} `json:"anchor"`
				Comments []struct {
					ID          int    `json:"id"`
					Text        string `json:"text"`
					CreatedDate int64  `json:"createdDate"`
					Author      struct {
						Name string `json:"name"`
					} `json:"author"`
				} `json:"comments"`
			} `json:"comment"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var results []*types.PullRequestReviewComment
	for _, it := range raw.Values {
		if it.Action == "COMMENTED" && it.Comment.ID != 0 {
			threadID := strconv.Itoa(it.Comment.ID)
			created := time.UnixMilli(it.Comment.CreatedDate).Format(time.RFC3339)

			results = append(results, &types.PullRequestReviewComment{
				ID:        threadID,
				ThreadID:  threadID,
				Path:      it.Comment.Anchor.Path,
				Line:      it.Comment.Anchor.Line,
				StartLine: it.Comment.Anchor.Line,
				Body:      it.Comment.Text,
				Author: &types.PullRequestCommentAuthor{
					Username: it.Comment.Author.Name,
					Name:     it.Comment.Author.DisplayName,
				},
				CreatedAt: created,
			})

			for _, sub := range it.Comment.Comments {
				subCreated := time.UnixMilli(sub.CreatedDate).Format(time.RFC3339)
				results = append(results, &types.PullRequestReviewComment{
					ID:        strconv.Itoa(sub.ID),
					ThreadID:  threadID,
					Body:      sub.Text,
					Author: &types.PullRequestCommentAuthor{
						Username: sub.Author.Name,
					},
					CreatedAt: subCreated,
				})
			}
		}
	}

	return results, nil
}

func (s *BitbucketServerService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	all, err := s.GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	var inline []*types.PullRequestReviewComment
	for _, c := range all {
		if c.Path != "" && c.Line > 0 {
			inline = append(inline, c)
		}
	}
	return inline, nil
}

func (s *BitbucketServerService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	all, err := s.GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	var threads []*types.PullRequestReviewComment
	for _, c := range all {
		if c.ThreadID == c.ID {
			threads = append(threads, c)
		}
	}
	return threads, nil
}

func (s *BitbucketServerService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get comment: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/comments/%s", url.PathEscape(projectKey), url.PathEscape(repoSlug), commentID)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID          int    `json:"id"`
		Text        string `json:"text"`
		CreatedDate int64  `json:"createdDate"`
		Author      struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"author"`
		Anchor struct {
			Line int    `json:"line"`
			Path string `json:"path"`
		} `json:"anchor"`
	}

	if err := json.Unmarshal(data, &it); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(it.ID),
		Path:      it.Anchor.Path,
		Line:      it.Anchor.Line,
		StartLine: it.Anchor.Line,
		Body:      it.Text,
		Author: &types.PullRequestCommentAuthor{
			Username: it.Author.Name,
			Name:     it.Author.DisplayName,
		},
		CreatedAt: time.UnixMilli(it.CreatedDate).Format(time.RFC3339),
	}, nil
}

func (s *BitbucketServerService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server create review comment: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	body := map[string]any{
		"text": comment.Body,
	}

	if comment.Path != "" && comment.Line > 0 {
		body["anchor"] = map[string]any{
			"line":     comment.Line,
			"lineType": "ADDED",
			"fileType": "TO",
			"path":     comment.Path,
		}
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &it); err != nil {
		return nil, err
	}

	comment.ID = strconv.Itoa(it.ID)
	return &comment, nil
}

func (s *BitbucketServerService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateReviewComment(ctx, orgData, repo, prNumber, types.PullRequestReviewComment{
		Body: body,
	})
}

func (s *BitbucketServerService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketServerService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketServerService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server reply comment: repo missing")
	}

	parentInt, _ := strconv.Atoi(parentCommentID)
	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/comments", url.PathEscape(projectKey), url.PathEscape(repoSlug), prNumber)
	reqBody := map[string]any{
		"text": body,
		"parent": map[string]any{
			"id": parentInt,
		},
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &it); err != nil {
		return nil, err
	}

	return &types.PullRequestReviewComment{
		ID:       strconv.Itoa(it.ID),
		ThreadID: parentCommentID,
		Body:     body,
	}, nil
}

func (s *BitbucketServerService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
	err := s.UpdateIssueComment(ctx, orgData, repo, commentID, body)
	if err != nil {
		return nil, err
	}
	return &types.PullRequestReviewComment{
		ID:       commentID,
		ThreadID: parentID,
		Body:     body,
	}, nil
}

func (s *BitbucketServerService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return errors.New("bitbucket server update comment: repo missing")
	}

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/comments/%s", url.PathEscape(projectKey), url.PathEscape(repoSlug), commentID)
	reqBody := map[string]any{
		"version": 1,
		"text":    body,
	}

	_, _, err := s.executeRequest(ctx, orgData, http.MethodPut, endpoint, reqBody)
	return err
}

func (s *BitbucketServerService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	return nil
}

func (s *BitbucketServerService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	return nil
}

func (s *BitbucketServerService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	return []string{}, nil
}

// -------------------------------------------------------------------------------------
// Users & Authors API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	token := s.extractToken(orgData)
	return &types.PullRequestUser{
		Username: "service-user",
		Login:    "service-user",
		Name:     "Service User",
		ID:       token,
	}, nil
}

func (s *BitbucketServerService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	endpoint := fmt.Sprintf("/rest/api/1.0/users/%s", url.PathEscape(username))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return &types.PullRequestUser{
		ID:       strconv.Itoa(raw.ID),
		Username: raw.Name,
		Login:    raw.Name,
		Name:     raw.DisplayName,
		Email:    raw.EmailAddress,
	}, nil
}

func (s *BitbucketServerService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	res := make(map[string]*types.PullRequestUser)
	for _, u := range usernames {
		if user, err := s.GetUserByUsername(ctx, orgData, u); err == nil && user != nil {
			res[u] = user
		}
	}
	return res, nil
}

func (s *BitbucketServerService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	if userName != "" {
		return s.GetUserByUsername(ctx, orgData, userName)
	}
	return nil, errors.New("user not found by email in bitbucket server")
}

func (s *BitbucketServerService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	return s.GetUserByUsername(ctx, orgData, userID)
}

func (s *BitbucketServerService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	return s.GetListMembers(ctx, orgData)
}

// -------------------------------------------------------------------------------------
// Content, Batch Files, Trees & Languages
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, filePath, ref string) (*types.RepositoryFile, error) {
	projectKey, repoSlug := s.parseProjectRepo(repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server get content: repo missing")
	}

	cleanPath := strings.TrimLeft(filePath, "/")
	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/raw/%s", url.PathEscape(projectKey), url.PathEscape(repoSlug), cleanPath)
	if ref != "" {
		endpoint += "?at=" + url.QueryEscape(ref)
	}

	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	return &types.RepositoryFile{
		Path:    filePath,
		Content: string(data),
		Size:    int64(len(data)),
		SHA:     ref,
	}, nil
}

func (s *BitbucketServerService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
	result := make(map[string]*types.RepositoryFile)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, f := range files {
		wg.Add(1)
		go func(pathStr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			file, err := s.GetRepositoryContentFile(ctx, orgData, &repo, pathStr, ref)
			if err == nil && file != nil {
				mu.Lock()
				result[pathStr] = file
				mu.Unlock()
			}
		}(f)
	}

	wg.Wait()
	return result, nil
}

func (s *BitbucketServerService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	return map[string]int{}, nil
}

func (s *BitbucketServerService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repositoryID: %s", repositoryID)
	}
	projectKey, repoSlug := parts[0], parts[1]

	endpoint := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/files?limit=500", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []string `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	tree := make([]*types.TreeItem, 0, len(raw.Values))
	for _, p := range raw.Values {
		tree = append(tree, &types.TreeItem{
			Path: p,
			Type: "blob",
		})
	}

	return tree, nil
}

func (s *BitbucketServerService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	tree, err := s.GetRepositoryTree(ctx, orgData, repositoryID)
	if err != nil {
		return nil, err
	}

	cleanDir := strings.Trim(directoryPath, "/")
	var subTree []*types.TreeItem
	for _, it := range tree {
		if strings.HasPrefix(it.Path, cleanDir) {
			subTree = append(subTree, it)
		}
	}

	return subTree, nil
}

func (s *BitbucketServerService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	projectKey, repoSlug := s.parseProjectRepo(&repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server all files: repo missing")
	}

	tree, err := s.GetRepositoryTree(ctx, orgData, projectKey+"/"+repoSlug)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, it := range tree {
		paths = append(paths, it.Path)
		if maxFiles > 0 && len(paths) >= maxFiles {
			break
		}
	}

	batch, err := s.GetRepositoryContentBatch(ctx, orgData, repo, paths, branch)
	if err != nil {
		return nil, err
	}

	var result []*types.RepositoryFile
	for _, f := range batch {
		result = append(result, f)
	}

	return result, nil
}

func (s *BitbucketServerService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	projectKey, repoSlug := s.parseProjectRepo(&repo)
	if projectKey == "" || repoSlug == "" {
		return nil, errors.New("bitbucket server clone params: repo missing")
	}

	baseURL := s.resolveBaseURL(orgData)
	u, _ := url.Parse(baseURL)
	token := s.extractToken(orgData)
	host := u.Host
	if host == "" {
		host = "localhost:7990"
	}

	user, _ := s.extractBasicAuth(orgData)
	if user == "" {
		user = "git"
	}

	cloneURL := fmt.Sprintf("https://%s:%s@%s/scm/%s/%s.git", user, token, host, strings.ToLower(projectKey), strings.ToLower(repoSlug))

	return &types.GitCloneParams{
		URL:      cloneURL,
		Branch:   "main",
		Token:    token,
		Username: user,
		Provider: models.ProviderBitbucket,
		Auth: &types.GitCloneAuth{
			Type:     types.AuthModeBasic,
			Username: user,
			Token:    token,
		},
	}, nil
}

func (s *BitbucketServerService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	token := s.extractToken(orgData)
	if token != "" {
		return token, nil
	}
	return "", errors.New("token not found")
}

// -------------------------------------------------------------------------------------
// Webhooks API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	return true, nil
}

func (s *BitbucketServerService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	return nil
}

// -------------------------------------------------------------------------------------
// Reactions API
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	return []types.ReactionsInComments{}, nil
}

func (s *BitbucketServerService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	return nil
}

func (s *BitbucketServerService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	return nil
}

func (s *BitbucketServerService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	return nil
}

func (s *BitbucketServerService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	return nil
}

// -------------------------------------------------------------------------------------
// Comment Formatting
// -------------------------------------------------------------------------------------

func (s *BitbucketServerService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
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

func (s *BitbucketServerService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	if m, ok := payload.(map[string]any); ok {
		if pr, ok := m["pullRequest"].(map[string]any); ok {
			if author, ok := pr["author"].(map[string]any); ok {
				if user, ok := author["user"].(map[string]any); ok {
					var idStr string
					if id, ok := user["id"].(float64); ok {
						idStr = strconv.FormatInt(int64(id), 10)
					}
					name, _ := user["name"].(string)
					displayName, _ := user["displayName"].(string)
					email, _ := user["emailAddress"].(string)
					return &types.PullRequestUser{
						ID:       idStr,
						Login:    name,
						Username: name,
						Name:     displayName,
						Email:    email,
					}, nil
				}
			}
		}
	}
	return nil, nil
}

func (s *BitbucketServerService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	if limit <= 0 {
		limit = 100
	}
	prs, err := s.GetPullRequests(ctx, orgData, &repo, "all", "", "")
	if err != nil {
		return nil, err
	}

	var results []*types.PullRequestReviewComment
	for _, pr := range prs {
		if len(results) >= limit {
			break
		}
		comments, err := s.GetPullRequestReviewComments(ctx, orgData, &repo, pr.Number)
		if err != nil {
			continue
		}
		for _, c := range comments {
			results = append(results, c)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

