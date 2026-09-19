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

// BitbucketCloudService implements contracts.ICodeManagementService for Bitbucket Cloud (2.0 API).
// Translates libs/platform/infrastructure/adapters/services/bitbucket/bitbucket-cloud.service.ts
var _ contracts.ICodeManagementService = (*BitbucketCloudService)(nil)

type BitbucketCloudService struct {
	baseURL        string
	httpClient     *http.Client
	defaultTimeout time.Duration
	cacheStore     sync.Map
}

// BitbucketCloudConfig configures the BitbucketCloudService adapter.
type BitbucketCloudConfig struct {
	BaseURL        string
	HTTPClient     *http.Client
	DefaultTimeout time.Duration
}

type cloudCacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewBitbucketCloudService creates a production Bitbucket Cloud service adapter.
func NewBitbucketCloudService(cfg BitbucketCloudConfig) *BitbucketCloudService {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
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

	return &BitbucketCloudService{
		baseURL:        baseURL,
		httpClient:     client,
		defaultTimeout: timeout,
	}
}

func (s *BitbucketCloudService) Provider() models.SCMProvider {
	return models.ProviderBitbucket
}

// -------------------------------------------------------------------------------------
// Cache Helpers
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) getCached(key string) (any, bool) {
	if val, ok := s.cacheStore.Load(key); ok {
		entry := val.(cloudCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, true
		}
		s.cacheStore.Delete(key)
	}
	return nil, false
}

func (s *BitbucketCloudService) setCached(key string, value any, ttl time.Duration) {
	s.cacheStore.Store(key, cloudCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	})
}

// -------------------------------------------------------------------------------------
// HTTP Request Execution & Authentication
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) extractToken(orgData types.OrganizationAndTeamData) string {
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
		if t, ok := orgData.IntegrationCredentials["oauthToken"].(string); ok && t != "" {
			return t
		}
	}
	return ""
}

func (s *BitbucketCloudService) extractBasicAuth(orgData types.OrganizationAndTeamData) (user, pass string) {
	if orgData.IntegrationCredentials != nil {
		u, _ := orgData.IntegrationCredentials["username"].(string)
		p, _ := orgData.IntegrationCredentials["appPassword"].(string)
		if p == "" {
			p, _ = orgData.IntegrationCredentials["password"].(string)
		}
		if u != "" && p != "" {
			return u, p
		}
	}
	return "", ""
}

func (s *BitbucketCloudService) executeRequest(ctx context.Context, orgData types.OrganizationAndTeamData, method, endpoint string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body failed: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := endpoint
	if !strings.HasPrefix(reqURL, "http://") && !strings.HasPrefix(reqURL, "https://") {
		reqURL = s.baseURL + "/" + strings.TrimLeft(endpoint, "/")
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request failed: %w", err)
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
				return nil, resp.StatusCode, fmt.Errorf("bitbucket cloud rate limited or unavailable (status %d)", resp.StatusCode)
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
		return respBytes, resp.StatusCode, fmt.Errorf("bitbucket cloud api error status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, resp.StatusCode, nil
}

func (s *BitbucketCloudService) parseRepoSlug(repo *types.RepositoryDescriptor) (workspace, repoSlug string) {
	if repo == nil {
		return "", ""
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner, repo.Name
	}
	slug := repo.FullName
	if slug == "" {
		slug = repo.Name
	}
	parts := strings.Split(strings.Trim(slug, "/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	if repo.Owner != "" {
		return repo.Owner, slug
	}
	return "", slug
}

// -------------------------------------------------------------------------------------
// Issues API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return true, nil
}

func (s *BitbucketCloudService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	repoDesc := &types.RepositoryDescriptor{
		Name:  params.Repository.Name,
		Owner: params.Repository.Owner,
	}
	ws, slug := s.parseRepoSlug(repoDesc)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud list issues: workspace or repository slug missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/issues?pagelen=50", url.PathEscape(ws), url.PathEscape(slug))
	if params.Filters != nil && params.Filters.State != "" {
		endpoint += fmt.Sprintf("&q=state%%3D%%22%s%%22", url.QueryEscape(params.Filters.State))
	}

	data, _, err := s.executeRequest(ctx, params.OrganizationAndTeamData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID        int    `json:"id"`
			Title     string `json:"title"`
			State     string `json:"state"`
			CreatedOn string `json:"created_on"`
			UpdatedOn string `json:"updated_on"`
			Content   struct {
				Raw string `json:"raw"`
			} `json:"content"`
			Reporter struct {
				DisplayName string `json:"display_name"`
				Nickname    string `json:"nickname"`
				AccountID   string `json:"account_id"`
			} `json:"reporter"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket issues failed: %w", err)
	}

	issues := make([]types.CodeManagementIssue, 0, len(raw.Values))
	for _, it := range raw.Values {
		body := it.Content.Raw
		username := it.Reporter.Nickname
		if username == "" {
			username = it.Reporter.DisplayName
		}
		issues = append(issues, types.CodeManagementIssue{
			ID:          strconv.Itoa(it.ID),
			Number:      it.ID,
			Title:       it.Title,
			Body:        &body,
			Description: body,
			State:       it.State,
			URL:         it.Links.HTML.Href,
			CreatedAt:   it.CreatedOn,
			UpdatedAt:   it.UpdatedOn,
			Author: &types.IssueAuthor{
				Username: username,
			},
			Platform: models.ProviderBitbucket,
		})
	}

	return issues, nil
}

func (s *BitbucketCloudService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	repoDesc := &types.RepositoryDescriptor{
		Name:  params.Repository.Name,
		Owner: params.Repository.Owner,
	}
	ws, slug := s.parseRepoSlug(repoDesc)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get issue: workspace or repository slug missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/issues/%d", url.PathEscape(ws), url.PathEscape(slug), params.IssueNumber)
	data, _, err := s.executeRequest(ctx, params.OrganizationAndTeamData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID        int    `json:"id"`
		Title     string `json:"title"`
		State     string `json:"state"`
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
		Content   struct {
			Raw string `json:"raw"`
		} `json:"content"`
		Reporter struct {
			DisplayName string `json:"display_name"`
			Nickname    string `json:"nickname"`
		} `json:"reporter"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &it); err != nil {
		return nil, fmt.Errorf("decode bitbucket issue failed: %w", err)
	}

	body := it.Content.Raw
	username := it.Reporter.Nickname
	if username == "" {
		username = it.Reporter.DisplayName
	}

	return &types.CodeManagementIssue{
		ID:          strconv.Itoa(it.ID),
		Number:      it.ID,
		Title:       it.Title,
		Body:        &body,
		Description: body,
		State:       it.State,
		URL:         it.Links.HTML.Href,
		CreatedAt:   it.CreatedOn,
		UpdatedAt:   it.UpdatedOn,
		Author: &types.IssueAuthor{
			Username: username,
		},
		Platform: models.ProviderBitbucket,
	}, nil
}

// -------------------------------------------------------------------------------------
// Repositories API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository full name: %s (expected workspace/repo)", name)
	}
	ws, slug := parts[0], parts[1]

	endpoint := fmt.Sprintf("/repositories/%s/%s", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		UUID      string `json:"uuid"`
		FullName  string `json:"full_name"`
		Name      string `json:"name"`
		IsPrivate bool   `json:"is_private"`
		MainBranch struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
			Clone []struct {
				Name string `json:"name"`
				Href string `json:"href"`
			} `json:"clone"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket repository failed: %w", err)
	}

	cloneURL := ""
	for _, c := range raw.Links.Clone {
		if c.Name == "https" {
			cloneURL = c.Href
			break
		}
	}

	return &types.Repository{
		ID:            raw.UUID,
		Name:          raw.Name,
		FullName:      raw.FullName,
		Private:       raw.IsPrivate,
		IsPrivate:     raw.IsPrivate,
		DefaultBranch: raw.MainBranch.Name,
		HTMLURL:       raw.Links.HTML.Href,
		CloneURL:      cloneURL,
		Provider:      models.ProviderBitbucket,
	}, nil
}

func (s *BitbucketCloudService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	var results []*types.Repositories
	endpoint := "/repositories?role=member&pagelen=100"

	for endpoint != "" {
		data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		var page struct {
			Next   string `json:"next"`
			Values []struct {
				UUID      string `json:"uuid"`
				FullName  string `json:"full_name"`
				Name      string `json:"name"`
				IsPrivate bool   `json:"is_private"`
				Language  string `json:"language"`
				MainBranch struct {
					Name string `json:"name"`
				} `json:"mainbranch"`
				Links struct {
					HTML struct {
						Href string `json:"href"`
					} `json:"html"`
				} `json:"links"`
			} `json:"values"`
		}

		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("decode bitbucket repositories page failed: %w", err)
		}

		for _, r := range page.Values {
			if visibility == "private" && !r.IsPrivate {
				continue
			}
			if visibility == "public" && r.IsPrivate {
				continue
			}
			if language != "" && !strings.EqualFold(r.Language, language) {
				continue
			}

			results = append(results, &types.Repositories{
				ID:            r.UUID,
				Name:          r.Name,
				FullName:      r.FullName,
				Private:       r.IsPrivate,
				DefaultBranch: r.MainBranch.Name,
				HTTPURL:       r.Links.HTML.Href,
				Language:      r.Language,
			})
		}

		endpoint = page.Next
	}

	return results, nil
}

func (s *BitbucketCloudService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, "/workspaces?pagelen=100", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			UUID string `json:"uuid"`
			Slug string `json:"slug"`
			Name string `json:"name"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
				Avatar struct {
					Href string `json:"href"`
				} `json:"avatar"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket workspaces failed: %w", err)
	}

	orgs := make([]*types.Organization, 0, len(raw.Values))
	for _, w := range raw.Values {
		orgs = append(orgs, &types.Organization{
			ID:        w.UUID,
			Login:     w.Slug,
			Name:      w.Name,
			AvatarURL: w.Links.Avatar.Href,
		})
	}

	return orgs, nil
}

func (s *BitbucketCloudService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, "/user/permissions/workspaces?pagelen=50", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			User struct {
				UUID        string `json:"uuid"`
				DisplayName string `json:"display_name"`
				Nickname    string `json:"nickname"`
				AccountID   string `json:"account_id"`
				Links       struct {
					Avatar struct {
						Href string `json:"href"`
					} `json:"avatar"`
				} `json:"links"`
			} `json:"user"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket members failed: %w", err)
	}

	authors := make([]types.PullRequestAuthor, 0, len(raw.Values))
	for _, it := range raw.Values {
		username := it.User.Nickname
		if username == "" {
			username = it.User.DisplayName
		}
		authors = append(authors, types.PullRequestAuthor{
			ID:       it.User.AccountID,
			Username: username,
			Name:     it.User.DisplayName,
		})
	}

	return authors, nil
}

func (s *BitbucketCloudService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	data, code, err := s.executeRequest(ctx, orgData, http.MethodGet, "/user", nil)
	if err != nil || code != http.StatusOK {
		return &types.CodeManagementConnectionStatus{
			HasConnection:   false,
			IsConnected:     false,
			IsSetupComplete: false,
			Message:         fmt.Sprintf("verification failed (status %d): %v", code, err),
			PlatformName:    string(models.ProviderBitbucket),
		}, nil
	}

	var user struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	}
	_ = json.Unmarshal(data, &user)

	name := user.Nickname
	if name == "" {
		name = user.DisplayName
	}

	return &types.CodeManagementConnectionStatus{
		HasConnection:   true,
		IsConnected:     true,
		IsSetupComplete: true,
		Message:         "connected as " + name,
		PlatformName:    string(models.ProviderBitbucket),
	}, nil
}

func (s *BitbucketCloudService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return "main", nil
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return "main", nil
	}

	var raw struct {
		MainBranch struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := json.Unmarshal(data, &raw); err == nil && raw.MainBranch.Name != "" {
		return raw.MainBranch.Name, nil
	}

	return "main", nil
}

// -------------------------------------------------------------------------------------
// Pull Requests API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get pull requests: workspace or repository slug missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests?pagelen=50", url.PathEscape(ws), url.PathEscape(slug))
	var qParts []string

	if state != "" {
		st := strings.ToUpper(state)
		if st == "OPEN" {
			qParts = append(qParts, "state=\"OPEN\"")
		} else if st == "MERGED" {
			qParts = append(qParts, "state=\"MERGED\"")
		} else if st == "DECLINED" || st == "CLOSED" {
			qParts = append(qParts, "(state=\"DECLINED\" OR state=\"SUPERSEDED\")")
		}
	}

	if branch != "" {
		qParts = append(qParts, fmt.Sprintf("source.branch.name=\"%s\"", branch))
	}

	if len(qParts) > 0 {
		endpoint += "&q=" + url.QueryEscape(strings.Join(qParts, " AND "))
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
			CreatedOn   string `json:"created_on"`
			UpdatedOn   string `json:"updated_on"`
			Author      struct {
				DisplayName string `json:"display_name"`
				Nickname    string `json:"nickname"`
				AccountID   string `json:"account_id"`
				Links       struct {
					Avatar struct {
						Href string `json:"href"`
					} `json:"avatar"`
				} `json:"links"`
			} `json:"author"`
			Source struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"source"`
			Destination struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"destination"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode bitbucket pull requests failed: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(raw.Values))
	for _, it := range raw.Values {
		username := it.Author.Nickname
		if username == "" {
			username = it.Author.DisplayName
		}
		if author != "" && !strings.EqualFold(username, author) {
			continue
		}

		prs = append(prs, &types.PullRequest{
			ID:          strconv.Itoa(it.ID),
			Number:      it.ID,
			PullNumber:  it.ID,
			Title:       it.Title,
			Description: it.Description,
			Body:        it.Description,
			State:       strings.ToLower(it.State),
			URL:         it.Links.HTML.Href,
			PRURL:       it.Links.HTML.Href,
			SourceBranch: it.Source.Branch.Name,
			TargetBranch: it.Destination.Branch.Name,
			HeadSHA:     it.Source.Commit.Hash,
			BaseSHA:     it.Destination.Commit.Hash,
			CreatedAt:   it.CreatedOn,
			UpdatedAt:   it.UpdatedOn,
			Author:      username,
			User: types.PullRequestUser{
				ID:        it.Author.AccountID,
				Login:     username,
				Username:  username,
				Name:      it.Author.DisplayName,
				AvatarURL: it.Author.Links.Avatar.Href,
			},
		})
	}

	return prs, nil
}

func (s *BitbucketCloudService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
}

func (s *BitbucketCloudService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get pull request: workspace or repository slug missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		CreatedOn   string `json:"created_on"`
		UpdatedOn   string `json:"updated_on"`
		Author      struct {
			DisplayName string `json:"display_name"`
			Nickname    string `json:"nickname"`
			AccountID   string `json:"account_id"`
			Links       struct {
				Avatar struct {
					Href string `json:"href"`
				} `json:"avatar"`
			} `json:"links"`
		} `json:"author"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"destination"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &it); err != nil {
		return nil, fmt.Errorf("decode bitbucket pull request failed: %w", err)
	}

	username := it.Author.Nickname
	if username == "" {
		username = it.Author.DisplayName
	}

	return &types.PullRequest{
		ID:          strconv.Itoa(it.ID),
		Number:      it.ID,
		PullNumber:  it.ID,
		Title:       it.Title,
		Description: it.Description,
		Body:        it.Description,
		State:       strings.ToLower(it.State),
		URL:         it.Links.HTML.Href,
		PRURL:       it.Links.HTML.Href,
		SourceBranch: it.Source.Branch.Name,
		TargetBranch: it.Destination.Branch.Name,
		HeadSHA:     it.Source.Commit.Hash,
		BaseSHA:     it.Destination.Commit.Hash,
		CreatedAt:   it.CreatedOn,
		UpdatedAt:   it.UpdatedOn,
		Author:      username,
		User: types.PullRequestUser{
			ID:        it.Author.AccountID,
			Login:     username,
			Username:  username,
			Name:      it.Author.DisplayName,
			AvatarURL: it.Author.Links.Avatar.Href,
		},
	}, nil
}

func (s *BitbucketCloudService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "open", "", "")
}

func (s *BitbucketCloudService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
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

func (s *BitbucketCloudService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get prs for rttm: workspace or repository slug missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests?state=MERGED&pagelen=50", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID        int    `json:"id"`
			CreatedOn string `json:"created_on"`
			UpdatedOn string `json:"updated_on"`
			Author    struct {
				Nickname string `json:"nickname"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	rttm := make([]*types.PullRequestCodeReviewTime, 0, len(raw.Values))
	for _, it := range raw.Values {
		created, _ := time.Parse(time.RFC3339, it.CreatedOn)
		merged, _ := time.Parse(time.RFC3339, it.UpdatedOn)
		duration := merged.Sub(created).Seconds()
		if duration < 0 {
			duration = 0
		}

		rttm = append(rttm, &types.PullRequestCodeReviewTime{
			PRNumber:       it.ID,
			ReviewDuration: duration,
			CreatedAt:      created,
			MergedAt:       &merged,
			Author:         it.Author.Nickname,
		})
	}

	return rttm, nil
}

func (s *BitbucketCloudService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get changes requested: workspace or repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests?state=OPEN&pagelen=50", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			ID     int    `json:"id"`
			Title  string `json:"title"`
			Participants []struct {
				Role     string `json:"role"`
				Approved bool   `json:"approved"`
				State    string `json:"state"`
				User     struct {
					Nickname string `json:"nickname"`
				} `json:"user"`
			} `json:"participants"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var results []types.PullRequestsWithChangesRequested
	for _, it := range raw.Values {
		for _, p := range it.Participants {
			if strings.EqualFold(p.State, "changes_requested") {
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

func (s *BitbucketCloudService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	pr, err := s.GetPullRequestByNumber(ctx, orgData, repo, prNumber)
	if err != nil {
		return false, err
	}
	title := strings.ToLower(pr.Title)
	return strings.HasPrefix(title, "[wip]") || strings.HasPrefix(title, "wip:") || strings.HasPrefix(title, "[draft]"), nil
}

func (s *BitbucketCloudService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return types.PullRequestReviewStatePending, errors.New("bitbucket cloud get review status: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return types.PullRequestReviewStatePending, err
	}

	var raw struct {
		Participants []struct {
			Role     string `json:"role"`
			Approved bool   `json:"approved"`
			State    string `json:"state"`
		} `json:"participants"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return types.PullRequestReviewStatePending, err
	}

	hasApproval := false
	for _, p := range raw.Participants {
		if strings.EqualFold(p.State, "changes_requested") {
			return types.PullRequestReviewStateChangesRequested, nil
		}
		if p.Approved {
			hasApproval = true
		}
	}

	if hasApproval {
		return types.PullRequestReviewStateApproved, nil
	}

	return types.PullRequestReviewStatePending, nil
}

func (s *BitbucketCloudService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return errors.New("bitbucket cloud update description: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	body := map[string]any{
		"description": description,
	}

	_, _, err := s.executeRequest(ctx, orgData, http.MethodPut, endpoint, body)
	return err
}

func (s *BitbucketCloudService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return errors.New("bitbucket cloud merge pull request: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/merge", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	body := map[string]any{
		"merge_strategy": "merge_commit",
	}
	if method == "squash" {
		body["merge_strategy"] = "squash"
	} else if method == "fast_forward" {
		body["merge_strategy"] = "fast_forward"
	}

	_, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, body)
	return err
}

func (s *BitbucketCloudService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return errors.New("bitbucket cloud approve pull request: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/approve", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	_, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *BitbucketCloudService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return errors.New("bitbucket cloud request changes: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/request-changes", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	_, _, err := s.executeRequest(ctx, orgData, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *BitbucketCloudService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	status, err := s.GetReviewStatusByPullRequest(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}
	return status == types.PullRequestReviewStateApproved, nil
}

func (s *BitbucketCloudService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	ws, slug := s.parseRepoSlug(&repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud create pr with files: repo missing")
	}

	if len(files) > 0 {
		ok, err := s.UploadFiles(ctx, orgData, repo, sourceBranch, targetBranch, commitMessage, author, files)
		if err != nil || !ok {
			return nil, fmt.Errorf("upload files failed: %w", err)
		}
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests", url.PathEscape(ws), url.PathEscape(slug))
	body := map[string]any{
		"title":       title,
		"description": description,
		"source": map[string]any{
			"branch": map[string]any{
				"name": sourceBranch,
			},
		},
		"destination": map[string]any{
			"branch": map[string]any{
				"name": targetBranch,
			},
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

func (s *BitbucketCloudService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	ws, slug := s.parseRepoSlug(&repo)
	if ws == "" || slug == "" {
		return false, errors.New("bitbucket cloud upload files: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/src", url.PathEscape(ws), url.PathEscape(slug))
	form := url.Values{}
	form.Set("message", message)
	form.Set("branch", branchName)
	if baseBranch != "" {
		form.Set("parents", baseBranch)
	}
	if author != nil && author.Name != "" {
		form.Set("author", fmt.Sprintf("%s <%s>", author.Name, author.Email))
	}

	for _, f := range files {
		form.Set(f.Path, f.Content)
	}

	reqURL := s.baseURL + "/" + strings.TrimLeft(endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := s.extractToken(orgData)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

// -------------------------------------------------------------------------------------
// Commits & Diffs API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get commits: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/commits", url.PathEscape(ws), url.PathEscape(slug))
	if branch != "" {
		endpoint += "/" + url.PathEscape(branch)
	}
	endpoint += "?pagelen=50"

	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Hash    string `json:"hash"`
			Message string `json:"message"`
			Date    string `json:"date"`
			Author  struct {
				Raw  string `json:"raw"`
				User struct {
					Nickname    string `json:"nickname"`
					DisplayName string `json:"display_name"`
				} `json:"user"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	commits := make([]*types.Commit, 0, len(raw.Values))
	for _, it := range raw.Values {
		username := it.Author.User.Nickname
		if username == "" {
			username = it.Author.User.DisplayName
		}
		if author != "" && !strings.EqualFold(username, author) {
			continue
		}

		commitDate, _ := time.Parse(time.RFC3339, it.Date)
		commits = append(commits, &types.Commit{
			SHA:     it.Hash,
			Message: it.Message,
			Author: types.GitActor{
				Name: username,
			},
			AuthorName:  username,
			AuthorLogin: username,
			Date:        commitDate,
		})
	}

	return commits, nil
}

func (s *BitbucketCloudService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get pr commits: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/commits?pagelen=100", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Hash    string `json:"hash"`
			Message string `json:"message"`
			Date    string `json:"date"`
			Author  struct {
				User struct {
					Nickname    string `json:"nickname"`
					DisplayName string `json:"display_name"`
				} `json:"user"`
			} `json:"author"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	commits := make([]*types.Commit, 0, len(raw.Values))
	for _, it := range raw.Values {
		name := it.Author.User.Nickname
		if name == "" {
			name = it.Author.User.DisplayName
		}
		commitDate, _ := time.Parse(time.RFC3339, it.Date)
		commits = append(commits, &types.Commit{
			SHA:     it.Hash,
			Message: it.Message,
			Author: types.GitActor{
				Name: name,
			},
			AuthorName:  name,
			AuthorLogin: name,
			Date:        commitDate,
		})
	}

	return commits, nil
}

func (s *BitbucketCloudService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get files: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/diffstat?pagelen=500", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Status string `json:"status"`
			LinesAdded int `json:"lines_added"`
			LinesRemoved int `json:"lines_removed"`
			Old struct {
				Path string `json:"path"`
			} `json:"old"`
			New struct {
				Path string `json:"path"`
			} `json:"new"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Attempt to fetch full PR diff to populate patches
	diffEndpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/diff", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	diffData, _, diffErr := s.executeRequest(ctx, orgData, http.MethodGet, diffEndpoint, nil)
	var patchMap map[string]string
	if diffErr == nil && len(diffData) > 0 {
		patchMap = s.parseUnifiedDiff(string(diffData))
	}

	files := make([]*types.PullRequestFile, 0, len(raw.Values))
	for _, it := range raw.Values {
		filePath := it.New.Path
		if filePath == "" {
			filePath = it.Old.Path
		}
		status := "modified"
		if it.Status == "added" {
			status = "added"
		} else if it.Status == "removed" {
			status = "removed"
		}

		patch := ""
		if patchMap != nil {
			if p, ok := patchMap[filePath]; ok {
				patch = p
			}
		}

		files = append(files, &types.PullRequestFile{
			Filename:  filePath,
			Status:    status,
			Additions: it.LinesAdded,
			Deletions: it.LinesRemoved,
			Changes:   it.LinesAdded + it.LinesRemoved,
			Patch:     patch,
		})
	}

	return files, nil
}

func (s *BitbucketCloudService) parseUnifiedDiff(diffContent string) map[string]string {
	patchMap := make(map[string]string)
	if diffContent == "" {
		return patchMap
	}

	sections := strings.Split(diffContent, "diff --git ")
	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		lines := strings.Split(section, "\n")
		if len(lines) == 0 {
			continue
		}

		header := lines[0]
		parts := strings.Split(header, " ")
		if len(parts) < 2 {
			continue
		}
		bPath := strings.TrimPrefix(parts[1], "b/")

		idx := strings.Index(section, "@@")
		if idx == -1 {
			patchMap[bPath] = ""
			continue
		}
		patchMap[bPath] = strings.TrimSpace(section[idx:])
	}

	return patchMap
}

func (s *BitbucketCloudService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
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

func (s *BitbucketCloudService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get all comments: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments?pagelen=100", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	var allComments []*types.PullRequestReviewComment

	for endpoint != "" {
		data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		var page struct {
			Next   string `json:"next"`
			Values []struct {
				ID        int    `json:"id"`
				CreatedOn string `json:"created_on"`
				UpdatedOn string `json:"updated_on"`
				Content   struct {
					Raw string `json:"raw"`
				} `json:"content"`
				User struct {
					Nickname    string `json:"nickname"`
					DisplayName string `json:"display_name"`
					AccountID   string `json:"account_id"`
				} `json:"user"`
				Parent struct {
					ID int `json:"id"`
				} `json:"parent"`
				Inline struct {
					Path string `json:"path"`
					To   *int   `json:"to"`
					From *int   `json:"from"`
				} `json:"inline"`
			} `json:"values"`
		}

		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}

		for _, it := range page.Values {
			line := 0
			if it.Inline.To != nil {
				line = *it.Inline.To
			} else if it.Inline.From != nil {
				line = *it.Inline.From
			}

			username := it.User.Nickname
			if username == "" {
				username = it.User.DisplayName
			}

			threadID := strconv.Itoa(it.ID)
			if it.Parent.ID != 0 {
				threadID = strconv.Itoa(it.Parent.ID)
			}

			allComments = append(allComments, &types.PullRequestReviewComment{
				ID:        strconv.Itoa(it.ID),
				ThreadID:  threadID,
				Path:      it.Inline.Path,
				Line:      line,
				StartLine: line,
				Body:      it.Content.Raw,
				Author: &types.PullRequestCommentAuthor{
					ID:       it.User.AccountID,
					Username: username,
					Name:     it.User.DisplayName,
				},
				CreatedAt: it.CreatedOn,
				UpdatedAt: it.UpdatedOn,
			})
		}

		endpoint = page.Next
	}

	return allComments, nil
}

func (s *BitbucketCloudService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	all, err := s.GetAllCommentsInPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	var inlineOnly []*types.PullRequestReviewComment
	for _, c := range all {
		if c.Path != "" && c.Line > 0 {
			inlineOnly = append(inlineOnly, c)
		}
	}
	return inlineOnly, nil
}

func (s *BitbucketCloudService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
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

func (s *BitbucketCloudService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get review comment: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/comments/%s", url.PathEscape(ws), url.PathEscape(slug), commentID)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var it struct {
		ID        int    `json:"id"`
		CreatedOn string `json:"created_on"`
		Content   struct {
			Raw string `json:"raw"`
		} `json:"content"`
		User struct {
			Nickname    string `json:"nickname"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
		Inline struct {
			Path string `json:"path"`
			To   *int   `json:"to"`
		} `json:"inline"`
	}

	if err := json.Unmarshal(data, &it); err != nil {
		return nil, err
	}

	line := 0
	if it.Inline.To != nil {
		line = *it.Inline.To
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(it.ID),
		Path:      it.Inline.Path,
		Line:      line,
		StartLine: line,
		Body:      it.Content.Raw,
		Author: &types.PullRequestCommentAuthor{
			Username: it.User.Nickname,
			Name:     it.User.DisplayName,
		},
		CreatedAt: it.CreatedOn,
	}, nil
}

func (s *BitbucketCloudService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud create review comment: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	body := map[string]any{
		"content": map[string]any{
			"raw": comment.Body,
		},
	}

	if comment.Path != "" && comment.Line > 0 {
		body["inline"] = map[string]any{
			"path": comment.Path,
			"to":   comment.Line,
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

func (s *BitbucketCloudService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateReviewComment(ctx, orgData, repo, prNumber, types.PullRequestReviewComment{
		Body: body,
	})
}

func (s *BitbucketCloudService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketCloudService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *BitbucketCloudService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud reply comment: repo missing")
	}

	parentInt, _ := strconv.Atoi(parentCommentID)
	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", url.PathEscape(ws), url.PathEscape(slug), prNumber)
	reqBody := map[string]any{
		"content": map[string]any{
			"raw": body,
		},
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

func (s *BitbucketCloudService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
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

func (s *BitbucketCloudService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string, body string) error {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return errors.New("bitbucket cloud update comment: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/comments/%s", url.PathEscape(ws), url.PathEscape(slug), commentID)
	reqBody := map[string]any{
		"content": map[string]any{
			"raw": body,
		},
	}

	_, _, err := s.executeRequest(ctx, orgData, http.MethodPut, endpoint, reqBody)
	return err
}

func (s *BitbucketCloudService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	return nil
}

func (s *BitbucketCloudService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	// Bitbucket Cloud doesn't have a native "resolve comment" API like GitHub/GitLab.
	// The closest equivalent is to update the comment status or leave it as-is.
	// The TS SDK uses bitbucketAPI.pullrequests.resolveComment which is only available
	// in the Bitbucket SDK wrapper. Via REST we cannot resolve comments directly.
	// This is a known limitation - return nil to indicate no-op.
	return nil
}

func (s *BitbucketCloudService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	ws, slug := s.parseRepoSlug(repo)

	endpoint := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d",
		url.PathEscape(ws), url.PathEscape(slug), prNumber)

	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Participants []struct {
			Role     string `json:"role"`
			Approved bool   `json:"approved"`
			User     struct {
				UUID        string `json:"uuid"`
				DisplayName string `json:"display_name"`
			} `json:"user"`
		} `json:"participants"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var validIDs []string
	for _, p := range raw.Participants {
		if p.Approved && p.Role == "REVIEWER" {
			validIDs = append(validIDs, p.User.UUID)
		}
	}

	return validIDs, nil
}

// -------------------------------------------------------------------------------------
// Users & Authors API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, "/user", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		UUID        string `json:"uuid"`
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
		AccountID   string `json:"account_id"`
		Links       struct {
			Avatar struct {
				Href string `json:"href"`
			} `json:"avatar"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	name := raw.Nickname
	if name == "" {
		name = raw.DisplayName
	}

	return &types.PullRequestUser{
		ID:        raw.AccountID,
		Username:  name,
		Login:     name,
		Name:      raw.DisplayName,
		AvatarURL: raw.Links.Avatar.Href,
	}, nil
}

func (s *BitbucketCloudService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	endpoint := fmt.Sprintf("/users/%s", url.PathEscape(username))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
		AccountID   string `json:"account_id"`
		Links       struct {
			Avatar struct {
				Href string `json:"href"`
			} `json:"avatar"`
		} `json:"links"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	name := raw.Nickname
	if name == "" {
		name = raw.DisplayName
	}

	return &types.PullRequestUser{
		ID:        raw.AccountID,
		Username:  name,
		Login:     name,
		Name:      raw.DisplayName,
		AvatarURL: raw.Links.Avatar.Href,
	}, nil
}

func (s *BitbucketCloudService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	res := make(map[string]*types.PullRequestUser)
	for _, u := range usernames {
		if user, err := s.GetUserByUsername(ctx, orgData, u); err == nil && user != nil {
			res[u] = user
		}
	}
	return res, nil
}

func (s *BitbucketCloudService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	if userName != "" {
		return s.GetUserByUsername(ctx, orgData, userName)
	}
	return nil, errors.New("user not found by email in bitbucket cloud")
}

func (s *BitbucketCloudService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	return s.GetUserByUsername(ctx, orgData, userID)
}

func (s *BitbucketCloudService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	return s.GetListMembers(ctx, orgData)
}

// -------------------------------------------------------------------------------------
// Content, Batch Files, Trees & Languages
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, filePath, ref string) (*types.RepositoryFile, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get content: repo missing")
	}

	if ref == "" {
		ref = "HEAD"
	}

	cleanPath := strings.TrimLeft(filePath, "/")
	endpoint := fmt.Sprintf("/repositories/%s/%s/src/%s/%s", url.PathEscape(ws), url.PathEscape(slug), url.PathEscape(ref), cleanPath)
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

func (s *BitbucketCloudService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
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

func (s *BitbucketCloudService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	ws, slug := s.parseRepoSlug(repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get language: repo missing")
	}

	endpoint := fmt.Sprintf("/repositories/%s/%s", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(data, &raw); err == nil && raw.Language != "" {
		return map[string]int{raw.Language: 100}, nil
	}

	return map[string]int{}, nil
}

func (s *BitbucketCloudService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid bitbucket repositoryID: %s", repositoryID)
	}
	ws, slug := parts[0], parts[1]

	endpoint := fmt.Sprintf("/repositories/%s/%s/src/HEAD/?pagelen=100&format=meta", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	tree := make([]*types.TreeItem, 0, len(raw.Values))
	for _, it := range raw.Values {
		treeType := "blob"
		if it.Type == "commit_directory" {
			treeType = "tree"
		}
		tree = append(tree, &types.TreeItem{
			Path: it.Path,
			Type: treeType,
			Size: it.Size,
		})
	}

	return tree, nil
}

func (s *BitbucketCloudService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid bitbucket repositoryID: %s", repositoryID)
	}
	ws, slug := parts[0], parts[1]

	cleanDir := strings.Trim(directoryPath, "/")
	endpoint := fmt.Sprintf("/repositories/%s/%s/src/HEAD/%s/?pagelen=100&format=meta", url.PathEscape(ws), url.PathEscape(slug), cleanDir)
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Values []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	tree := make([]*types.TreeItem, 0, len(raw.Values))
	for _, it := range raw.Values {
		treeType := "blob"
		if it.Type == "commit_directory" {
			treeType = "tree"
		}
		tree = append(tree, &types.TreeItem{
			Path: it.Path,
			Type: treeType,
			Size: it.Size,
		})
	}

	return tree, nil
}

func (s *BitbucketCloudService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	ws, slug := s.parseRepoSlug(&repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get all files: repo missing")
	}

	if branch == "" {
		branch = "HEAD"
	}

	tree, err := s.GetRepositoryTree(ctx, orgData, ws+"/"+slug)
	if err != nil {
		return nil, err
	}

	var matchedPaths []string
	for _, it := range tree {
		if it.Type == "blob" {
			matchedPaths = append(matchedPaths, it.Path)
			if maxFiles > 0 && len(matchedPaths) >= maxFiles {
				break
			}
		}
	}

	batch, err := s.GetRepositoryContentBatch(ctx, orgData, repo, matchedPaths, branch)
	if err != nil {
		return nil, err
	}

	var result []*types.RepositoryFile
	for _, f := range batch {
		result = append(result, f)
	}

	return result, nil
}

func (s *BitbucketCloudService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	ws, slug := s.parseRepoSlug(&repo)
	if ws == "" || slug == "" {
		return nil, errors.New("bitbucket cloud get clone params: repo missing")
	}

	token := s.extractToken(orgData)
	cloneURL := fmt.Sprintf("https://x-token-auth:%s@bitbucket.org/%s/%s.git", token, ws, slug)

	return &types.GitCloneParams{
		URL:      cloneURL,
		Branch:   "main",
		Token:    token,
		Provider: models.ProviderBitbucket,
		Auth: &types.GitCloneAuth{
			Type:  types.AuthModePAT,
			Token: token,
		},
	}, nil
}

func (s *BitbucketCloudService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	token := s.extractToken(orgData)
	if token != "" {
		return token, nil
	}
	return "", errors.New("oauth token not found")
}

// -------------------------------------------------------------------------------------
// Webhooks API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return false, fmt.Errorf("invalid repositoryID: %s", repositoryID)
	}
	ws, slug := parts[0], parts[1]

	endpoint := fmt.Sprintf("/repositories/%s/%s/hooks", url.PathEscape(ws), url.PathEscape(slug))
	data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}

	var raw struct {
		Values []struct {
			Active bool   `json:"active"`
			URL    string `json:"url"`
		} `json:"values"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return false, err
	}

	for _, h := range raw.Values {
		if h.Active && strings.Contains(h.URL, "scandrix.dev") {
			return true, nil
		}
	}

	return len(raw.Values) > 0, nil
}

func (s *BitbucketCloudService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	// List all accessible repositories and clean up ScanDrix webhooks
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return fmt.Errorf("failed to list repos for webhook cleanup: %w", err)
	}

	for _, repo := range repos {
		parts := strings.Split(repo.FullName, "/")
		if len(parts) < 2 {
			continue
		}
		ws, slug := parts[0], parts[1]

		endpoint := fmt.Sprintf("/repositories/%s/%s/hooks?pagelen=50",
			url.PathEscape(ws), url.PathEscape(slug))

		data, _, err := s.executeRequest(ctx, orgData, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}

		var raw struct {
			Values []struct {
				UUID string `json:"uuid"`
				URL  string `json:"url"`
			} `json:"values"`
		}

		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}

		for _, hook := range raw.Values {
			if strings.Contains(hook.URL, "scandrix") {
				delEndpoint := fmt.Sprintf("/repositories/%s/%s/hooks/%s",
					url.PathEscape(ws), url.PathEscape(slug), url.PathEscape(hook.UUID))
				_, _, _ = s.executeRequest(ctx, orgData, http.MethodDelete, delEndpoint, nil)
			}
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Reactions API
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	return []types.ReactionsInComments{}, nil
}

func (s *BitbucketCloudService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	return nil
}

func (s *BitbucketCloudService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	return nil
}

func (s *BitbucketCloudService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	return nil
}

func (s *BitbucketCloudService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	return nil
}

// -------------------------------------------------------------------------------------
// Comment Formatting
// -------------------------------------------------------------------------------------

func (s *BitbucketCloudService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
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

func (s *BitbucketCloudService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	if m, ok := payload.(map[string]any); ok {
		if pr, ok := m["pullrequest"].(map[string]any); ok {
			if author, ok := pr["author"].(map[string]any); ok {
				uuidStr, _ := author["uuid"].(string)
				nickname, _ := author["nickname"].(string)
				displayName, _ := author["display_name"].(string)
				if nickname == "" {
					nickname = displayName
				}
				var avatar string
				if links, ok := author["links"].(map[string]any); ok {
					if av, ok := links["avatar"].(map[string]any); ok {
						avatar, _ = av["href"].(string)
					}
				}
				return &types.PullRequestUser{
					ID:        uuidStr,
					Login:     nickname,
					Username:  nickname,
					Name:      displayName,
					AvatarURL: avatar,
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *BitbucketCloudService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
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

