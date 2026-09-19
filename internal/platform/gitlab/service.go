// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// GitLabService implements contracts.ICodeManagementService for GitLab SaaS and GitLab Self-Managed.
// Translates libs/platform/infrastructure/adapters/services/gitlab.service.ts
var _ contracts.ICodeManagementService = (*GitLabService)(nil)

type GitLabService struct {
	baseURL        string
	httpClient     *http.Client
	cacheStore     sync.Map
	defaultTimeout time.Duration
}

// GitLabServiceConfig configures GitLabService instances.
type GitLabServiceConfig struct {
	BaseURL        string
	HTTPClient     *http.Client
	DefaultTimeout time.Duration
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewGitLabService creates an enterprise GitLab service adapter.
func NewGitLabService(cfg GitLabServiceConfig) *GitLabService {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
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

	return &GitLabService{
		baseURL:        baseURL,
		httpClient:     client,
		defaultTimeout: timeout,
	}
}

func (s *GitLabService) Provider() models.SCMProvider {
	return models.ProviderGitLab
}

// -------------------------------------------------------------------------------------
// Cache Helpers
// -------------------------------------------------------------------------------------

func (s *GitLabService) getCached(key string) (any, bool) {
	if val, ok := s.cacheStore.Load(key); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, true
		}
		s.cacheStore.Delete(key)
	}
	return nil, false
}

func (s *GitLabService) setCached(key string, value any, ttl time.Duration) {
	s.cacheStore.Store(key, cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	})
}

// -------------------------------------------------------------------------------------
// HTTP Request Dispatcher with Rate-Limiting and Retry
// -------------------------------------------------------------------------------------

func (s *GitLabService) resolveToken(orgData types.OrganizationAndTeamData) string {
	if orgData.AuthToken != "" {
		return orgData.AuthToken
	}
	if orgData.IntegrationCredentials != nil {
		if token, ok := orgData.IntegrationCredentials["token"].(string); ok && token != "" {
			return token
		}
		if pat, ok := orgData.IntegrationCredentials["personalAccessToken"].(string); ok && pat != "" {
			return pat
		}
		if oauth, ok := orgData.IntegrationCredentials["accessToken"].(string); ok && oauth != "" {
			return oauth
		}
	}
	return ""
}

func (s *GitLabService) resolveBaseURL(orgData types.OrganizationAndTeamData) string {
	if orgData.IntegrationCredentials != nil {
		if host, ok := orgData.IntegrationCredentials["host"].(string); ok && host != "" {
			host = strings.TrimRight(host, "/")
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "https://" + host
			}
			return host
		}
	}
	return s.baseURL
}

func (s *GitLabService) doRequest(ctx context.Context, orgData types.OrganizationAndTeamData, method, apiPath string, body any) (*http.Response, []byte, error) {
	baseURL := s.resolveBaseURL(orgData)
	token := s.resolveToken(orgData)

	fullURL := baseURL + "/api/v4" + apiPath
	var bodyReader io.Reader
	var jsonBytes []byte
	var err error

	if body != nil {
		jsonBytes, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal gitlab request payload: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	var resp *http.Response
	var respBody []byte

	for attempt := 0; attempt <= MaxRetryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to construct gitlab request: %w", err)
		}

		if token != "" {
			// GitLab supports both PRIVATE-TOKEN and Bearer (OAuth)
			if strings.HasPrefix(token, "glpat-") || !strings.Contains(token, ".") {
				req.Header.Set("PRIVATE-TOKEN", token)
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err = s.httpClient.Do(req)
		if err != nil {
			if attempt == MaxRetryAttempts {
				return nil, nil, fmt.Errorf("gitlab request %s %s failed after %d attempts: %w", method, apiPath, attempt, err)
			}
			time.Sleep(CalculateBackoff(attempt))
			if bodyReader != nil {
				bodyReader = bytes.NewReader(jsonBytes)
			}
			continue
		}

		respBody, err = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read gitlab response body: %w", err)
		}

		if IsGitLabRateLimitError(resp, nil) {
			if attempt == MaxRetryAttempts {
				return resp, respBody, ToGitLabRateLimitError(resp, errors.New("gitlab rate limit exhausted"), orgData)
			}
			retryDuration := ParseRetryAfter(resp.Header.Get("Retry-After"))
			if retryDuration > 5*time.Second {
				return resp, respBody, ToGitLabRateLimitError(resp, errors.New("gitlab rate limit hit with high retry-after"), orgData)
			}
			time.Sleep(retryDuration)
			if bodyReader != nil {
				bodyReader = bytes.NewReader(jsonBytes)
			}
			continue
		}

		if resp.StatusCode >= 500 && attempt < MaxRetryAttempts {
			time.Sleep(CalculateBackoff(attempt))
			if bodyReader != nil {
				bodyReader = bytes.NewReader(jsonBytes)
			}
			continue
		}

		break
	}

	return resp, respBody, nil
}

func (s *GitLabService) encodeProjectID(repo *types.RepositoryDescriptor) string {
	if repo == nil {
		return ""
	}
	if repo.ID != "" && repo.ID != "0" {
		return repo.ID
	}
	if repo.FullName != "" {
		return url.PathEscape(repo.FullName)
	}
	if repo.Owner != "" && repo.Name != "" {
		return url.PathEscape(repo.Owner + "/" + repo.Name)
	}
	return url.PathEscape(repo.Name)
}

// -------------------------------------------------------------------------------------
// Issues API
// -------------------------------------------------------------------------------------

func (s *GitLabService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	repoDesc := &types.RepositoryDescriptor{
		Name:  params.Repository.Name,
		Owner: params.Repository.Owner,
	}
	projectID := s.encodeProjectID(repoDesc)
	if projectID == "" {
		return nil, errors.New("missing repository identification for ListIssues")
	}

	q := url.Values{}
	q.Set("per_page", "50")
	if params.Filters != nil && params.Filters.State != "" {
		q.Set("state", params.Filters.State)
	}

	apiPath := fmt.Sprintf("/projects/%s/issues?%s", projectID, q.Encode())
	resp, body, err := s.doRequest(ctx, params.OrganizationAndTeamData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab ListIssues failed status %d: %s", resp.StatusCode, string(body))
	}

	var issuesRaw []struct {
		ID          int      `json:"id"`
		IID         int      `json:"iid"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		State       string   `json:"state"`
		WebURL      string   `json:"web_url"`
		Labels      []string `json:"labels"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
		ClosedAt    string   `json:"closed_at"`
		Author      struct {
			Username  string `json:"username"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &issuesRaw); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab issues: %w", err)
	}

	issues := make([]types.CodeManagementIssue, len(issuesRaw))
	for i, r := range issuesRaw {
		desc := r.Description
		var closedAt *string
		if r.ClosedAt != "" {
			closedAt = &r.ClosedAt
		}
		issues[i] = types.CodeManagementIssue{
			ID:          strconv.Itoa(r.ID),
			Number:      r.IID,
			Title:       r.Title,
			Body:        &desc,
			Description: r.Description,
			State:       r.State,
			URL:         r.WebURL,
			Labels:      r.Labels,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			ClosedAt:    closedAt,
			Author: &types.IssueAuthor{
				Username: r.Author.Username,
			},
			Platform: models.ProviderGitLab,
		}
	}

	return issues, nil
}

func (s *GitLabService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	repoDesc := &types.RepositoryDescriptor{
		Name:  params.Repository.Name,
		Owner: params.Repository.Owner,
	}
	projectID := s.encodeProjectID(repoDesc)
	if projectID == "" {
		return nil, errors.New("missing repository identification for GetIssue")
	}

	apiPath := fmt.Sprintf("/projects/%s/issues/%d", projectID, params.IssueNumber)
	resp, body, err := s.doRequest(ctx, params.OrganizationAndTeamData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetIssue failed status %d: %s", resp.StatusCode, string(body))
	}

	var r struct {
		ID          int      `json:"id"`
		IID         int      `json:"iid"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		State       string   `json:"state"`
		WebURL      string   `json:"web_url"`
		Labels      []string `json:"labels"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
		ClosedAt    string   `json:"closed_at"`
		Author      struct {
			Username  string `json:"username"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab issue: %w", err)
	}

	desc := r.Description
	var closedAt *string
	if r.ClosedAt != "" {
		closedAt = &r.ClosedAt
	}
	return &types.CodeManagementIssue{
		ID:          strconv.Itoa(r.ID),
		Number:      r.IID,
		Title:       r.Title,
		Body:        &desc,
		Description: r.Description,
		State:       r.State,
		URL:         r.WebURL,
		Labels:      r.Labels,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		ClosedAt:    closedAt,
		Author: &types.IssueAuthor{
			Username: r.Author.Username,
		},
		Platform: models.ProviderGitLab,
	}, nil
}

func (s *GitLabService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return true, nil
}

// -------------------------------------------------------------------------------------
// Repositories & Search
// -------------------------------------------------------------------------------------

func (s *GitLabService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return nil, err
	}

	wanted := strings.TrimSpace(strings.ToLower(name))
	for _, r := range repos {
		fullName := strings.ToLower(r.FullName)
		repoName := strings.ToLower(r.Name)
		if repoName == wanted || fullName == wanted {
			return &types.Repository{
				ID:            r.ID,
				Name:          r.Name,
				FullName:      r.FullName,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				HTMLURL:       r.HTTPURL,
				CloneURL:      r.HTTPURL,
			}, nil
		}
	}

	return nil, nil
}

func (s *GitLabService) GetRepositories(ctx context.Context, orgData types.OrganizationAndTeamData, archived *bool, visibility, language string) ([]*types.Repositories, error) {
	q := url.Values{}
	q.Set("membership", "true")
	q.Set("per_page", "100")
	if archived != nil {
		q.Set("archived", strconv.FormatBool(*archived))
	}
	if visibility != "" {
		q.Set("visibility", strings.ToLower(visibility))
	}

	apiPath := "/projects?" + q.Encode()
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetRepositories failed status %d: %s", resp.StatusCode, string(body))
	}

	var projects []struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		NameWithNamespace string `json:"name_with_namespace"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
		Description       string `json:"description"`
		Visibility        string `json:"visibility"`
		WebURL            string `json:"web_url"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
		Archived          bool   `json:"archived"`
	}

	if err := json.Unmarshal(body, &projects); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab projects: %w", err)
	}

	result := make([]*types.Repositories, len(projects))
	for i, p := range projects {
		isPrivate := p.Visibility != "public"
		result[i] = &types.Repositories{
			ID:               strconv.Itoa(p.ID),
			Name:             p.Name,
			FullName:         p.PathWithNamespace,
			OrganizationName: path.Dir(p.PathWithNamespace),
			DefaultBranch:    p.DefaultBranch,
			Private:          isPrivate,
			HTTPURL:          p.WebURL,
		}
	}

	return result, nil
}

func (s *GitLabService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, "/groups?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetOrganizations failed status %d: %s", resp.StatusCode, string(body))
	}

	var groups []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Description string `json:"description"`
		WebURL      string `json:"web_url"`
		AvatarURL   string `json:"avatar_url"`
	}

	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab groups: %w", err)
	}

	orgs := make([]*types.Organization, len(groups))
	for i, g := range groups {
		orgs[i] = &types.Organization{
			ID:        strconv.Itoa(g.ID),
			Login:     g.Path,
			Name:      g.Name,
			AvatarURL: g.AvatarURL,
		}
	}

	return orgs, nil
}

func (s *GitLabService) GetDefaultBranch(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (string, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return "main", errors.New("missing repository ID for GetDefaultBranch")
	}

	apiPath := fmt.Sprintf("/projects/%s", projectID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return "main", err
	}
	if resp.StatusCode != http.StatusOK {
		return "main", fmt.Errorf("gitlab GetDefaultBranch failed status %d: %s", resp.StatusCode, string(body))
	}

	var p struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &p); err == nil && p.DefaultBranch != "" {
		return p.DefaultBranch, nil
	}

	return "main", nil
}

// -------------------------------------------------------------------------------------
// Pull Requests (Merge Requests)
// -------------------------------------------------------------------------------------

func (s *GitLabService) GetPullRequests(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, state, author, branch string) ([]*types.PullRequest, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetPullRequests")
	}

	q := url.Values{}
	q.Set("per_page", "50")
	if state != "" {
		switch strings.ToLower(state) {
		case "open", "opened":
			q.Set("state", "opened")
		case "closed":
			q.Set("state", "closed")
		case "merged":
			q.Set("state", "merged")
		case "all":
			q.Set("state", "all")
		default:
			q.Set("state", state)
		}
	}
	if branch != "" {
		q.Set("source_branch", branch)
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests?%s", projectID, q.Encode())
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetPullRequests failed status %d: %s", resp.StatusCode, string(body))
	}

	var mrs []struct {
		ID           int    `json:"id"`
		IID          int    `json:"iid"`
		ProjectID    int    `json:"project_id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		State        string `json:"state"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		MergedAt     string `json:"merged_at"`
		ClosedAt     string `json:"closed_at"`
		TargetBranch string `json:"target_branch"`
		SourceBranch string `json:"source_branch"`
		SHA          string `json:"sha"`
		WebURL       string `json:"web_url"`
		Draft        bool   `json:"draft"`
		WorkInProgress bool `json:"work_in_progress"`
		DiffRefs     struct {
			BaseSHA  string `json:"base_sha"`
			HeadSHA  string `json:"head_sha"`
			StartSHA string `json:"start_sha"`
		} `json:"diff_refs"`
		Author struct {
			ID        int    `json:"id"`
			Username  string `json:"username"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &mrs); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab merge requests: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(mrs))
	for _, mr := range mrs {
		if author != "" && !strings.EqualFold(mr.Author.Username, author) {
			continue
		}

		headSHA := mr.DiffRefs.HeadSHA
		if headSHA == "" {
			headSHA = mr.SHA
		}

		isDraft := mr.Draft || mr.WorkInProgress ||
			strings.HasPrefix(strings.ToLower(mr.Title), "draft:") ||
			strings.HasPrefix(strings.ToLower(mr.Title), "wip:")

		prs = append(prs, &types.PullRequest{
			ID:             strconv.Itoa(mr.ID),
			Number:         mr.IID,
			PullNumber:     mr.IID,
			Title:          mr.Title,
			Body:           mr.Description,
			Description:    mr.Description,
			Message:        mr.Title,
			State:          mr.State,
			CreatedAt:      mr.CreatedAt,
			UpdatedAt:      mr.UpdatedAt,
			MergedAt:       mr.MergedAt,
			ClosedAt:       mr.ClosedAt,
			SourceBranch:   mr.SourceBranch,
			TargetBranch:   mr.TargetBranch,
			SourceRefName:  mr.SourceBranch,
			TargetRefName:  mr.TargetBranch,
			HeadSHA:        headSHA,
			BaseSHA:        mr.DiffRefs.BaseSHA,
			PRURL:          mr.WebURL,
			URL:            mr.WebURL,
			OrganizationID: orgData.OrganizationID,
			RepositoryID:   strconv.Itoa(mr.ProjectID),
			IsDraft:        isDraft,
			User: types.PullRequestUser{
				ID:        strconv.Itoa(mr.Author.ID),
				Login:     mr.Author.Username,
				Username:  mr.Author.Username,
				Name:      mr.Author.Name,
				AvatarURL: mr.Author.AvatarURL,
			},
			Head: types.PullRequestBranchReference{
				Ref: mr.SourceBranch,
				SHA: headSHA,
				Repo: types.PullRequestRepoReference{
					ID: strconv.Itoa(mr.ProjectID),
				},
			},
			Base: types.PullRequestBranchReference{
				Ref: mr.TargetBranch,
				SHA: mr.DiffRefs.BaseSHA,
				Repo: types.PullRequestRepoReference{
					ID: strconv.Itoa(mr.ProjectID),
				},
			},
		})
	}

	return prs, nil
}

func (s *GitLabService) GetPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetPullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetPullRequest failed status %d: %s", resp.StatusCode, string(body))
	}

	var mr struct {
		ID             int    `json:"id"`
		IID            int    `json:"iid"`
		ProjectID      int    `json:"project_id"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		State          string `json:"state"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
		MergedAt       string `json:"merged_at"`
		ClosedAt       string `json:"closed_at"`
		TargetBranch   string `json:"target_branch"`
		SourceBranch   string `json:"source_branch"`
		SHA            string `json:"sha"`
		WebURL         string `json:"web_url"`
		Draft          bool   `json:"draft"`
		WorkInProgress bool   `json:"work_in_progress"`
		DiffRefs       struct {
			BaseSHA  string `json:"base_sha"`
			HeadSHA  string `json:"head_sha"`
			StartSHA string `json:"start_sha"`
		} `json:"diff_refs"`
		Author struct {
			ID        int    `json:"id"`
			Username  string `json:"username"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab merge request: %w", err)
	}

	headSHA := mr.DiffRefs.HeadSHA
	if headSHA == "" {
		headSHA = mr.SHA
	}

	isDraft := mr.Draft || mr.WorkInProgress ||
		strings.HasPrefix(strings.ToLower(mr.Title), "draft:") ||
		strings.HasPrefix(strings.ToLower(mr.Title), "wip:")

	return &types.PullRequest{
		ID:             strconv.Itoa(mr.ID),
		Number:         mr.IID,
		PullNumber:     mr.IID,
		Title:          mr.Title,
		Body:           mr.Description,
		Description:    mr.Description,
		Message:        mr.Title,
		State:          mr.State,
		CreatedAt:      mr.CreatedAt,
		UpdatedAt:      mr.UpdatedAt,
		MergedAt:       mr.MergedAt,
		ClosedAt:       mr.ClosedAt,
		SourceBranch:   mr.SourceBranch,
		TargetBranch:   mr.TargetBranch,
		SourceRefName:  mr.SourceBranch,
		TargetRefName:  mr.TargetBranch,
		HeadSHA:        headSHA,
		BaseSHA:        mr.DiffRefs.BaseSHA,
		PRURL:          mr.WebURL,
		URL:            mr.WebURL,
		OrganizationID: orgData.OrganizationID,
		RepositoryID:   strconv.Itoa(mr.ProjectID),
		IsDraft:        isDraft,
		User: types.PullRequestUser{
			ID:        strconv.Itoa(mr.Author.ID),
			Login:     mr.Author.Username,
			Username:  mr.Author.Username,
			Name:      mr.Author.Name,
			AvatarURL: mr.Author.AvatarURL,
		},
		Head: types.PullRequestBranchReference{
			Ref: mr.SourceBranch,
			SHA: headSHA,
			Repo: types.PullRequestRepoReference{
				ID: strconv.Itoa(mr.ProjectID),
			},
		},
		Base: types.PullRequestBranchReference{
			Ref: mr.TargetBranch,
			SHA: mr.DiffRefs.BaseSHA,
			Repo: types.PullRequestRepoReference{
				ID: strconv.Itoa(mr.ProjectID),
			},
		},
	}, nil
}

func (s *GitLabService) GetPullRequestByNumber(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (*types.PullRequest, error) {
	return s.GetPullRequest(ctx, orgData, repo, prNumber)
}

func (s *GitLabService) GetPullRequestsByRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "opened", "", "")
}

func (s *GitLabService) GetPullRequestsWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestWithFiles, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "opened", "", "")
	if err != nil {
		return nil, err
	}

	result := make([]*types.PullRequestWithFiles, len(prs))
	for i, pr := range prs {
		files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, pr.Number)
		if err != nil {
			files = []*types.PullRequestFile{}
		}
		result[i] = &types.PullRequestWithFiles{
			PullRequest: *pr,
			Files:       files,
		}
	}

	return result, nil
}

func (s *GitLabService) GetPullRequestsForRTTM(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]*types.PullRequestCodeReviewTime, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "merged", "", "")
	if err != nil {
		return nil, err
	}

	rttm := make([]*types.PullRequestCodeReviewTime, 0, len(prs))
	for _, pr := range prs {
		if pr.CreatedAt != "" && pr.MergedAt != "" {
			cTime, err1 := time.Parse(time.RFC3339, pr.CreatedAt)
			mTime, err2 := time.Parse(time.RFC3339, pr.MergedAt)
			if err1 == nil && err2 == nil {
				durationSeconds := mTime.Sub(cTime).Seconds()
				rttm = append(rttm, &types.PullRequestCodeReviewTime{
					PRNumber:       pr.Number,
					ReviewDuration: durationSeconds,
					CreatedAt:      cTime,
					MergedAt:       &mTime,
					Author:         pr.User.Username,
				})
			}
		}
	}

	return rttm, nil
}

func (s *GitLabService) IsDraftPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (bool, error) {
	pr, err := s.GetPullRequest(ctx, orgData, repo, prNumber)
	if err != nil {
		return false, err
	}
	if pr == nil {
		return false, nil
	}
	return pr.IsDraft, nil
}

// -------------------------------------------------------------------------------------
// Branch Management & Pull Request Creation with Files
// -------------------------------------------------------------------------------------

func (s *GitLabService) CreatePullRequestWithFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, sourceBranch, targetBranch, title, description, commitMessage string, author *types.GitActor, files []types.PullRequestFileChange) (*types.PullRequest, error) {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for CreatePullRequestWithFiles")
	}

	// 1. Ensure source branch exists or create it from targetBranch
	branchAPI := fmt.Sprintf("/projects/%s/repository/branches/%s", projectID, url.PathEscape(sourceBranch))
	resp, _, err := s.doRequest(ctx, orgData, http.MethodGet, branchAPI, nil)
	if err == nil && resp.StatusCode == http.StatusNotFound {
		// Create source branch
		createBranchPayload := map[string]string{
			"branch": sourceBranch,
			"ref":    targetBranch,
		}
		createBranchAPI := fmt.Sprintf("/projects/%s/repository/branches", projectID)
		bResp, bBody, bErr := s.doRequest(ctx, orgData, http.MethodPost, createBranchAPI, createBranchPayload)
		if bErr != nil {
			return nil, fmt.Errorf("failed to create gitlab source branch %s: %w", sourceBranch, bErr)
		}
		if bResp.StatusCode != http.StatusCreated && bResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to create branch %s status %d: %s", sourceBranch, bResp.StatusCode, string(bBody))
		}
	}

	// 2. Commit files to sourceBranch
	if len(files) > 0 {
		ok, err := s.UploadFiles(ctx, orgData, repo, sourceBranch, targetBranch, commitMessage, author, files)
		if err != nil || !ok {
			return nil, fmt.Errorf("failed to commit files to branch %s: %w", sourceBranch, err)
		}
	}

	// 3. Create Merge Request
	mrPayload := map[string]any{
		"source_branch": sourceBranch,
		"target_branch": targetBranch,
		"title":         title,
		"description":   description,
	}
	createMRAPI := fmt.Sprintf("/projects/%s/merge_requests", projectID)
	mrResp, mrBody, mrErr := s.doRequest(ctx, orgData, http.MethodPost, createMRAPI, mrPayload)
	if mrErr != nil {
		return nil, fmt.Errorf("failed to create gitlab merge request: %w", mrErr)
	}
	if mrResp.StatusCode != http.StatusCreated && mrResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create merge request status %d: %s", mrResp.StatusCode, string(mrBody))
	}

	var createdMR struct {
		IID int `json:"iid"`
	}
	if err := json.Unmarshal(mrBody, &createdMR); err != nil {
		return nil, fmt.Errorf("failed to parse created MR response: %w", err)
	}

	return s.GetPullRequest(ctx, orgData, &repo, createdMR.IID)
}

func (s *GitLabService) UploadFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branchName, baseBranch, message string, author *types.GitActor, files []types.PullRequestFileChange) (bool, error) {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return false, errors.New("missing repository ID for UploadFiles")
	}

	actions := make([]map[string]any, 0, len(files))
	for _, f := range files {
		action := "create"
		if strings.EqualFold(f.Operation, "delete") {
			action = "delete"
		} else {
			// Check if file exists to choose create vs update
			filePathAPI := fmt.Sprintf("/projects/%s/repository/files/%s?ref=%s", projectID, url.PathEscape(f.Path), url.QueryEscape(branchName))
			checkResp, _, _ := s.doRequest(ctx, orgData, http.MethodHead, filePathAPI, nil)
			if checkResp != nil && checkResp.StatusCode == http.StatusOK {
				action = "update"
			}
		}

		item := map[string]any{
			"action":    action,
			"file_path": f.Path,
		}
		if action != "delete" {
			item["content"] = f.Content
		}
		actions = append(actions, item)
	}

	commitPayload := map[string]any{
		"branch":         branchName,
		"commit_message": message,
		"actions":        actions,
	}
	if author != nil {
		if author.Name != "" {
			commitPayload["author_name"] = author.Name
		}
		if author.Email != "" {
			commitPayload["author_email"] = author.Email
		}
	}

	apiPath := fmt.Sprintf("/projects/%s/repository/commits", projectID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, commitPayload)
	if err != nil {
		return false, fmt.Errorf("gitlab commit creation failed: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("gitlab commit creation status %d: %s", resp.StatusCode, string(body))
	}

	return true, nil
}

// -------------------------------------------------------------------------------------
// Diff, Commits & File Contents
// -------------------------------------------------------------------------------------

func (s *GitLabService) GetFilesByPullRequestId(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestFile, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetFilesByPullRequestId")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/diffs?per_page=100", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetFilesByPullRequestId failed status %d: %s", resp.StatusCode, string(body))
	}

	var diffs []struct {
		OldPath     string `json:"old_path"`
		NewPath     string `json:"new_path"`
		AMode       string `json:"a_mode"`
		BMode       string `json:"b_mode"`
		NewFile     bool   `json:"new_file"`
		RenamedFile bool   `json:"renamed_file"`
		DeletedFile bool   `json:"deleted_file"`
		Diff        string `json:"diff"`
	}

	if err := json.Unmarshal(body, &diffs); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab diffs: %w", err)
	}

	files := make([]*types.PullRequestFile, len(diffs))
	for i, d := range diffs {
		status := "modified"
		if d.NewFile {
			status = "added"
		} else if d.DeletedFile {
			status = "deleted"
		} else if d.RenamedFile {
			status = "renamed"
		}

		filename := d.NewPath
		if filename == "" {
			filename = d.OldPath
		}

		adds := 0
		deletes := 0
		if d.Diff != "" {
			lines := strings.Split(d.Diff, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
					adds++
				} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
					deletes++
				}
			}
		}

		files[i] = &types.PullRequestFile{
			Filename:  filename,
			Status:    status,
			Patch:     d.Diff,
			Additions: adds,
			Deletions: deletes,
			Changes:   adds + deletes,
		}
	}

	return files, nil
}

func (s *GitLabService) GetChangedFilesSinceLastCommit(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, baseSHA, headSHA string) ([]string, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetChangedFilesSinceLastCommit")
	}

	apiPath := fmt.Sprintf("/projects/%s/repository/compare?from=%s&to=%s", projectID, url.QueryEscape(baseSHA), url.QueryEscape(headSHA))
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab compare failed status %d: %s", resp.StatusCode, string(body))
	}

	var comparison struct {
		Diffs []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
		} `json:"diffs"`
	}

	if err := json.Unmarshal(body, &comparison); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab comparison: %w", err)
	}

	paths := make([]string, 0, len(comparison.Diffs))
	for _, d := range comparison.Diffs {
		if d.NewPath != "" {
			paths = append(paths, d.NewPath)
		} else if d.OldPath != "" {
			paths = append(paths, d.OldPath)
		}
	}

	return paths, nil
}

func (s *GitLabService) GetCommits(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, branch, author string) ([]*types.Commit, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetCommits")
	}

	q := url.Values{}
	q.Set("per_page", "50")
	if branch != "" {
		q.Set("ref_name", branch)
	}

	apiPath := fmt.Sprintf("/projects/%s/repository/commits?%s", projectID, q.Encode())
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetCommits failed status %d: %s", resp.StatusCode, string(body))
	}

	var commitsRaw []struct {
		ID             string `json:"id"`
		ShortID        string `json:"short_id"`
		Title          string `json:"title"`
		Message        string `json:"message"`
		AuthorName     string `json:"author_name"`
		AuthorEmail    string `json:"author_email"`
		AuthoredDate   string `json:"authored_date"`
		CommitterName  string `json:"committer_name"`
		CommitterEmail string `json:"committer_email"`
		CommittedDate  string `json:"committed_date"`
		WebURL         string `json:"web_url"`
	}

	if err := json.Unmarshal(body, &commitsRaw); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab commits: %w", err)
	}

	commits := make([]*types.Commit, 0, len(commitsRaw))
	for _, c := range commitsRaw {
		if author != "" && !strings.EqualFold(c.AuthorName, author) && !strings.EqualFold(c.AuthorEmail, author) {
			continue
		}
		date, _ := time.Parse(time.RFC3339, c.AuthoredDate)
		commits = append(commits, &types.Commit{
			SHA:         c.ID,
			Message:     c.Message,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			Date:        date,
			URL:         c.WebURL,
		})
	}

	return commits, nil
}

func (s *GitLabService) GetCommitsForPullRequestForCodeReview(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.Commit, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetCommitsForPullRequestForCodeReview")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/commits?per_page=100", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab MR commits failed status %d: %s", resp.StatusCode, string(body))
	}

	var commitsRaw []struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Message        string `json:"message"`
		AuthorName     string `json:"author_name"`
		AuthorEmail    string `json:"author_email"`
		AuthoredDate   string `json:"authored_date"`
		WebURL         string `json:"web_url"`
	}

	if err := json.Unmarshal(body, &commitsRaw); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab MR commits: %w", err)
	}

	commits := make([]*types.Commit, len(commitsRaw))
	for i, c := range commitsRaw {
		date, _ := time.Parse(time.RFC3339, c.AuthoredDate)
		commits[i] = &types.Commit{
			SHA:         c.ID,
			Message:     c.Message,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			Date:        date,
			URL:         c.WebURL,
		}
	}

	return commits, nil
}

func (s *GitLabService) GetRepositoryContentFile(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, filePath, ref string) (*types.RepositoryFile, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetRepositoryContentFile")
	}

	if ref == "" {
		ref = "main"
	}

	encodedPath := url.PathEscape(filePath)
	apiPath := fmt.Sprintf("/projects/%s/repository/files/%s/raw?ref=%s", projectID, encodedPath, url.QueryEscape(ref))
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetRepositoryContentFile failed status %d: %s", resp.StatusCode, string(body))
	}

	return &types.RepositoryFile{
		Path:    filePath,
		Content: string(body),
		SHA:     ref,
		Size:    int64(len(body)),
	}, nil
}

func (s *GitLabService) GetRepositoryContentBatch(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, files []string, ref string) (map[string]*types.RepositoryFile, error) {
	result := make(map[string]*types.RepositoryFile)
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 8) // Concurrency limit

	for _, f := range files {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			content, err := s.GetRepositoryContentFile(ctx, orgData, &repo, path, ref)
			if err == nil && content != nil {
				mu.Lock()
				result[path] = content
				mu.Unlock()
			}
		}(f)
	}

	wg.Wait()
	return result, nil
}

func (s *GitLabService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	encodedID := url.PathEscape(repositoryID)
	apiPath := fmt.Sprintf("/projects/%s/repository/tree?recursive=true&per_page=100", encodedID)

	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetRepositoryTree failed status %d: %s", resp.StatusCode, string(body))
	}

	var items []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
		Mode string `json:"mode"`
	}

	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab tree items: %w", err)
	}

	tree := make([]*types.TreeItem, len(items))
	for i, item := range items {
		treeType := "file"
		if item.Type == "tree" {
			treeType = "directory"
		}
		tree[i] = &types.TreeItem{
			Path: item.Path,
			Mode: item.Mode,
			Type: treeType,
			SHA:  item.ID,
		}
	}

	return tree, nil
}

func (s *GitLabService) GetRepositoryTreeByDirectory(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID, directoryPath string) ([]*types.TreeItem, error) {
	encodedID := url.PathEscape(repositoryID)
	q := url.Values{}
	q.Set("recursive", "false")
	q.Set("per_page", "100")
	if directoryPath != "" {
		q.Set("path", directoryPath)
	}

	apiPath := fmt.Sprintf("/projects/%s/repository/tree?%s", encodedID, q.Encode())
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetRepositoryTreeByDirectory failed status %d: %s", resp.StatusCode, string(body))
	}

	var items []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
		Mode string `json:"mode"`
	}

	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab directory items: %w", err)
	}

	tree := make([]*types.TreeItem, len(items))
	for i, item := range items {
		treeType := "file"
		if item.Type == "tree" {
			treeType = "directory"
		}
		tree[i] = &types.TreeItem{
			Path: item.Path,
			Mode: item.Mode,
			Type: treeType,
			SHA:  item.ID,
		}
	}

	return tree, nil
}

func (s *GitLabService) GetRepositoryAllFiles(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, branch string, filePatterns, excludePatterns []string, maxFiles int) ([]*types.RepositoryFile, error) {
	tree, err := s.GetRepositoryTree(ctx, orgData, repo.ID)
	if err != nil {
		return nil, err
	}

	if maxFiles <= 0 {
		maxFiles = 100
	}

	var matchedPaths []string
	for _, item := range tree {
		if item.Type == "directory" {
			continue
		}
		if len(excludePatterns) > 0 {
			excluded := false
			for _, pat := range excludePatterns {
				if strings.Contains(item.Path, pat) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}
		if len(filePatterns) > 0 {
			matched := false
			for _, pat := range filePatterns {
				if strings.HasSuffix(item.Path, pat) || strings.Contains(item.Path, pat) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		matchedPaths = append(matchedPaths, item.Path)
		if len(matchedPaths) >= maxFiles {
			break
		}
	}

	filesMap, err := s.GetRepositoryContentBatch(ctx, orgData, repo, matchedPaths, branch)
	if err != nil {
		return nil, err
	}

	result := make([]*types.RepositoryFile, 0, len(filesMap))
	for _, f := range filesMap {
		result = append(result, f)
	}

	return result, nil
}

func (s *GitLabService) GetLanguageRepository(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) (map[string]int, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetLanguageRepository")
	}

	apiPath := fmt.Sprintf("/projects/%s/languages", projectID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab languages failed status %d: %s", resp.StatusCode, string(body))
	}

	var rawLangs map[string]float64
	if err := json.Unmarshal(body, &rawLangs); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab languages: %w", err)
	}

	langs := make(map[string]int, len(rawLangs))
	for lang, pct := range rawLangs {
		langs[lang] = int(pct * 100)
	}

	return langs, nil
}

// -------------------------------------------------------------------------------------
// Discussions & Comments (Code Review Inline Comments)
// -------------------------------------------------------------------------------------

func (s *GitLabService) CreateReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, comment types.PullRequestReviewComment) (*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for CreateReviewComment")
	}

	// 1. Fetch versions to acquire diff refs (base_commit_sha, start_commit_sha, head_commit_sha)
	versionsAPI := fmt.Sprintf("/projects/%s/merge_requests/%d/versions", projectID, prNumber)
	_, vBody, vErr := s.doRequest(ctx, orgData, http.MethodGet, versionsAPI, nil)
	if vErr != nil {
		return nil, fmt.Errorf("failed to fetch gitlab MR versions: %w", vErr)
	}

	var versions []struct {
		ID             int    `json:"id"`
		HeadCommitSHA  string `json:"head_commit_sha"`
		BaseCommitSHA  string `json:"base_commit_sha"`
		StartCommitSHA string `json:"start_commit_sha"`
	}
	_ = json.Unmarshal(vBody, &versions)

	var baseSHA, startSHA, headSHA string
	if len(versions) > 0 {
		baseSHA = versions[0].BaseCommitSHA
		startSHA = versions[0].StartCommitSHA
		headSHA = versions[0].HeadCommitSHA
	}
	if headSHA == "" {
		headSHA = comment.CommitID
	}

	newLine := comment.Line
	if comment.StartLine > 0 {
		newLine = comment.StartLine
	}

	position := map[string]any{
		"position_type": "text",
		"base_sha":      baseSHA,
		"start_sha":     startSHA,
		"head_sha":      headSHA,
		"new_path":      comment.Path,
		"new_line":      newLine,
	}
	if comment.StartLine > 0 && comment.Line > comment.StartLine {
		position["old_path"] = comment.Path
		position["old_line"] = comment.StartLine
	}

	discussionPayload := map[string]any{
		"body":     comment.Body,
		"position": position,
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, discussionPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to create gitlab discussion: %w", err)
	}

	// If position failed (e.g. line mismatch), fall back to creating a standard MR note
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, fmt.Sprintf("**[%s:%d]** %s", comment.Path, comment.Line, comment.Body))
	}

	var disc struct {
		ID    string `json:"id"`
		Notes []struct {
			ID        int    `json:"id"`
			Body      string `json:"body"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Author    struct {
				ID       int    `json:"id"`
				Username string `json:"username"`
			} `json:"author"`
		} `json:"notes"`
	}

	if err := json.Unmarshal(body, &disc); err != nil || len(disc.Notes) == 0 {
		return &types.PullRequestReviewComment{
			Body: comment.Body,
			Path: comment.Path,
			Line: comment.Line,
		}, nil
	}

	firstNote := disc.Notes[0]
	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(firstNote.ID),
		ThreadID:  disc.ID,
		Body:      firstNote.Body,
		Path:      comment.Path,
		Line:      comment.Line,
		CreatedAt: firstNote.CreatedAt,
		UpdatedAt: firstNote.UpdatedAt,
		Author: &types.PullRequestCommentAuthor{
			Username: firstNote.Author.Username,
			Name:     firstNote.Author.Username,
		},
	}, nil
}

func (s *GitLabService) CreateCommentInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for CreateCommentInPullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes", projectID, prNumber)
	resp, respBody, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab CreateCommentInPullRequest failed status %d: %s", resp.StatusCode, string(respBody))
	}

	var note struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.Unmarshal(respBody, &note); err != nil {
		return nil, fmt.Errorf("failed to decode note response: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(note.ID),
		Body:      note.Body,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
		Author: &types.PullRequestCommentAuthor{
			Username: note.Author.Username,
			Name:     note.Author.Username,
		},
	}, nil
}

func (s *GitLabService) CreateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *GitLabService) CreateSingleIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, body string) (*types.PullRequestReviewComment, error) {
	return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
}

func (s *GitLabService) UpdateIssueComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID, body string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for UpdateIssueComment")
	}

	// GitLab note update endpoint: PUT /projects/:id/merge_requests/:mr_iid/notes/:note_id
	// Or PUT /projects/:id/notes/:note_id
	apiPath := fmt.Sprintf("/projects/%s/notes/%s", projectID, commentID)
	resp, respBody, err := s.doRequest(ctx, orgData, http.MethodPut, apiPath, map[string]string{"body": body})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab UpdateIssueComment failed status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *GitLabService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID, reason string) error {
	// GitLab does not have a native "minimize comment" API like GitHub;
	// We wrap the comment in a collapsible <details> tag with reason.
	wrappedBody := fmt.Sprintf("<details>\n<summary><em>Marked as %s</em></summary>\n\n*(Content minimized)*\n</details>", reason)
	return s.UpdateIssueComment(ctx, orgData, nil, commentID, wrappedBody)
}

func (s *GitLabService) GetPullRequestReviewComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, commentID string) (*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetPullRequestReviewComment")
	}

	apiPath := fmt.Sprintf("/projects/%s/notes/%s", projectID, commentID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetPullRequestReviewComment failed status %d: %s", resp.StatusCode, string(body))
	}

	var note struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &note); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab note: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(note.ID),
		Body:      note.Body,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
		Author: &types.PullRequestCommentAuthor{
			Username: note.Author.Username,
			Name:     note.Author.Username,
		},
	}, nil
}

func (s *GitLabService) CreateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentCommentID, body string) (*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for CreateResponseToComment")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s/notes", projectID, prNumber, parentCommentID)
	resp, respBody, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// Fall back to top-level note
		return s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, body)
	}

	var note struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
	}

	if err := json.Unmarshal(respBody, &note); err != nil {
		return nil, fmt.Errorf("failed to decode note response: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(note.ID),
		ThreadID:  parentCommentID,
		Body:      note.Body,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
		Author: &types.PullRequestCommentAuthor{
			Username: note.Author.Username,
			Name:     note.Author.Username,
		},
	}, nil
}

func (s *GitLabService) UpdateResponseToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, parentID, commentID, body string) (*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for UpdateResponseToComment")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s/notes/%s", projectID, prNumber, parentID, commentID)
	resp, respBody, err := s.doRequest(ctx, orgData, http.MethodPut, apiPath, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab UpdateResponseToComment failed status %d: %s", resp.StatusCode, string(respBody))
	}

	var note struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
	}

	if err := json.Unmarshal(respBody, &note); err != nil {
		return nil, fmt.Errorf("failed to decode updated note response: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.Itoa(note.ID),
		ThreadID:  parentID,
		Body:      note.Body,
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
		Author: &types.PullRequestCommentAuthor{
			Username: note.Author.Username,
			Name:     note.Author.Username,
		},
	}, nil
}

func (s *GitLabService) MarkReviewCommentAsResolved(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, threadID string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for MarkReviewCommentAsResolved")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/discussions/%s?resolved=true", projectID, threadID)
	resp, respBody, err := s.doRequest(ctx, orgData, http.MethodPut, apiPath, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("gitlab MarkReviewCommentAsResolved failed status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *GitLabService) GetAllCommentsInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetAllCommentsInPullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes?per_page=100", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetAllCommentsInPullRequest failed status %d: %s", resp.StatusCode, string(body))
	}

	var notes []struct {
		ID        int    `json:"id"`
		Body      string `json:"body"`
		System    bool   `json:"system"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Author    struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
		} `json:"author"`
	}

	if err := json.Unmarshal(body, &notes); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab notes: %w", err)
	}

	comments := make([]*types.PullRequestReviewComment, 0, len(notes))
	for _, n := range notes {
		if n.System {
			continue // filter out system audit notes
		}
		comments = append(comments, &types.PullRequestReviewComment{
			ID:        strconv.Itoa(n.ID),
			Body:      n.Body,
			CreatedAt: n.CreatedAt,
			UpdatedAt: n.UpdatedAt,
			Author: &types.PullRequestCommentAuthor{
				Username: n.Author.Username,
				Name:     n.Author.Username,
			},
		})
	}

	return comments, nil
}

func (s *GitLabService) GetPullRequestReviewComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetPullRequestReviewComments")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions?per_page=100", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetPullRequestReviewComments failed status %d: %s", resp.StatusCode, string(body))
	}

	var discussions []struct {
		ID    string `json:"id"`
		Notes []struct {
			ID        int    `json:"id"`
			Body      string `json:"body"`
			System    bool   `json:"system"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Position  struct {
				NewPath string `json:"new_path"`
				NewLine int    `json:"new_line"`
			} `json:"position"`
			Author struct {
				Username string `json:"username"`
			} `json:"author"`
		} `json:"notes"`
	}

	if err := json.Unmarshal(body, &discussions); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab discussions: %w", err)
	}

	comments := make([]*types.PullRequestReviewComment, 0)
	for _, d := range discussions {
		for _, n := range d.Notes {
			if n.System {
				continue
			}
			comments = append(comments, &types.PullRequestReviewComment{
				ID:        strconv.Itoa(n.ID),
				ThreadID:  d.ID,
				Body:      n.Body,
				Path:      n.Position.NewPath,
				Line:      n.Position.NewLine,
				CreatedAt: n.CreatedAt,
				UpdatedAt: n.UpdatedAt,
				Author: &types.PullRequestCommentAuthor{
					Username: n.Author.Username,
					Name:     n.Author.Username,
				},
			})
		}
	}

	return comments, nil
}

func (s *GitLabService) GetPullRequestReviewThreads(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]*types.PullRequestReviewComment, error) {
	return s.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
}

func (s *GitLabService) UpdateDescriptionInPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, description string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for UpdateDescriptionInPullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d", projectID, prNumber)
	payload := map[string]string{"description": description}
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPut, apiPath, payload)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab UpdateDescriptionInPullRequest failed status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Approvals & Merge
// -------------------------------------------------------------------------------------

func (s *GitLabService) MergePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, method string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for MergePullRequest")
	}

	payload := map[string]any{
		"should_remove_source_branch": true,
	}
	if strings.EqualFold(method, "squash") {
		payload["squash"] = true
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/merge", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPut, apiPath, payload)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab MergePullRequest failed status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *GitLabService) ApprovePullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for ApprovePullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/approve", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, nil)
	if err != nil {
		return err
	}
	// 401/409 means already approved, treat as idempotent success
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("gitlab ApprovePullRequest failed status %d: %s", resp.StatusCode, string(body))
	}

	if message != "" {
		_, _ = s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
	}

	return nil
}

func (s *GitLabService) RequestChangesPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int, message string) error {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return errors.New("missing repository ID for RequestChangesPullRequest")
	}

	// In GitLab, unapproving retracts approval.
	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/unapprove", projectID, prNumber)
	_, _, _ = s.doRequest(ctx, orgData, http.MethodPost, apiPath, nil)

	if message != "" {
		_, err := s.CreateCommentInPullRequest(ctx, orgData, repo, prNumber, message)
		return err
	}

	return nil
}

func (s *GitLabService) GetReviewStatusByPullRequest(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) (types.PullRequestReviewState, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return types.PullRequestReviewStateCommented, errors.New("missing repository ID for GetReviewStatusByPullRequest")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/approval_state", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return types.PullRequestReviewStateCommented, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.PullRequestReviewStateCommented, nil
	}

	var state struct {
		Rules []struct {
			Approved bool `json:"approved"`
		} `json:"rules"`
	}

	if err := json.Unmarshal(body, &state); err == nil && len(state.Rules) > 0 {
		allApproved := true
		for _, r := range state.Rules {
			if !r.Approved {
				allApproved = false
				break
			}
		}
		if allApproved {
			return types.PullRequestReviewStateApproved, nil
		}
	}

	return types.PullRequestReviewStateCommented, nil
}

func (s *GitLabService) CheckIfPullRequestShouldBeApproved(ctx context.Context, orgData types.OrganizationAndTeamData, prNumber int, repo types.RepositoryDescriptor) (bool, error) {
	status, err := s.GetReviewStatusByPullRequest(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}
	return status == types.PullRequestReviewStateApproved, nil
}

func (s *GitLabService) GetListOfValidReviews(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]string, error) {
	projectID := s.encodeProjectID(repo)
	if projectID == "" {
		return nil, errors.New("missing repository ID for GetListOfValidReviews")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/approvals", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab approvals list failed status %d: %s", resp.StatusCode, string(body))
	}

	var approvals struct {
		ApprovedBy []struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"approved_by"`
	}

	if err := json.Unmarshal(body, &approvals); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab approvals: %w", err)
	}

	reviewers := make([]string, len(approvals.ApprovedBy))
	for i, a := range approvals.ApprovedBy {
		reviewers[i] = a.User.Username
	}

	return reviewers, nil
}

func (s *GitLabService) GetPullRequestsWithChangesRequested(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor) ([]types.PullRequestsWithChangesRequested, error) {
	// In GitLab CE/EE without approvals, return empty
	return []types.PullRequestsWithChangesRequested{}, nil
}

// -------------------------------------------------------------------------------------
// Users, Members & Authentication
// -------------------------------------------------------------------------------------

func (s *GitLabService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	user, err := s.GetCurrentUser(ctx, orgData)
	if err != nil || user == nil {
		return &types.CodeManagementConnectionStatus{
			HasConnection: false,
			IsConnected:   false,
			Message:       "Failed to authenticate with GitLab",
		}, nil
	}

	return &types.CodeManagementConnectionStatus{
		HasConnection: true,
		IsConnected:   true,
		Message:       "Successfully connected to GitLab as @" + user.Username,
	}, nil
}

func (s *GitLabService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, "/user", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetCurrentUser failed status %d: %s", resp.StatusCode, string(body))
	}

	var u struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		Bot       bool   `json:"bot"`
	}

	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab user: %w", err)
	}

	return &types.PullRequestUser{
		ID:        strconv.Itoa(u.ID),
		Login:     u.Username,
		Username:  u.Username,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		IsBot:     u.Bot,
	}, nil
}

func (s *GitLabService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	cacheKey := "gitlab-user-" + strings.ToLower(username)
	if cached, ok := s.getCached(cacheKey); ok {
		return cached.(*types.PullRequestUser), nil
	}

	apiPath := fmt.Sprintf("/users?username=%s", url.QueryEscape(username))
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetUserByUsername failed status %d: %s", resp.StatusCode, string(body))
	}

	var users []struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Bot       bool   `json:"bot"`
	}

	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}
	if len(users) == 0 {
		return nil, nil
	}

	u := users[0]
	user := &types.PullRequestUser{
		ID:        strconv.Itoa(u.ID),
		Login:     u.Username,
		Username:  u.Username,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		IsBot:     u.Bot,
	}

	s.setCached(cacheKey, user, 30*time.Minute)
	return user, nil
}

func (s *GitLabService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	result := make(map[string]*types.PullRequestUser)
	for _, username := range usernames {
		u, err := s.GetUserByUsername(ctx, orgData, username)
		if err == nil && u != nil {
			result[username] = u
		}
	}
	return result, nil
}

func (s *GitLabService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	query := email
	if query == "" {
		query = userName
	}
	if query == "" {
		return nil, errors.New("empty email and username for GetUserByEmailOrName")
	}

	apiPath := fmt.Sprintf("/users?search=%s", url.QueryEscape(query))
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab user search failed status %d: %s", resp.StatusCode, string(body))
	}

	var users []struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.Unmarshal(body, &users); err != nil || len(users) == 0 {
		return nil, nil
	}

	return &types.PullRequestUser{
		ID:        strconv.Itoa(users[0].ID),
		Login:     users[0].Username,
		Username:  users[0].Username,
		Name:      users[0].Name,
		AvatarURL: users[0].AvatarURL,
	}, nil
}

func (s *GitLabService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	apiPath := fmt.Sprintf("/users/%s", url.PathEscape(userID))
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab GetUserByID failed status %d: %s", resp.StatusCode, string(body))
	}

	var u struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}

	return &types.PullRequestUser{
		ID:        strconv.Itoa(u.ID),
		Login:     u.Username,
		Username:  u.Username,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}, nil
}

func (s *GitLabService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, "/users?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab members failed status %d: %s", resp.StatusCode, string(body))
	}

	var users []struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Bot       bool   `json:"bot"`
	}

	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to decode members: %w", err)
	}

	authors := make([]types.PullRequestAuthor, len(users))
	for i, u := range users {
		authors[i] = types.PullRequestAuthor{
			ID:       strconv.Itoa(u.ID),
			Username: u.Username,
			Name:     u.Name,
			IsBot:    u.Bot,
		}
	}

	return authors, nil
}

func (s *GitLabService) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	return s.GetListMembers(ctx, orgData)
}

func (s *GitLabService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	token := s.resolveToken(orgData)
	if token == "" {
		return "", errors.New("no authentication token configured for GitLab")
	}
	return token, nil
}

func (s *GitLabService) GetCloneParams(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor) (*types.GitCloneParams, error) {
	token := s.resolveToken(orgData)
	if token == "" {
		return nil, errors.New("gitlab authentication details not found")
	}

	baseURL := s.resolveBaseURL(orgData)
	projectPath := repo.FullName
	if projectPath == "" {
		projectPath = repo.Name
	}

	fullURL := fmt.Sprintf("%s/%s.git", baseURL, strings.TrimPrefix(projectPath, "/"))

	return &types.GitCloneParams{
		OrganizationID: orgData.OrganizationID,
		RepositoryID:   repo.ID,
		RepositoryName: repo.Name,
		URL:            fullURL,
		Provider:       models.ProviderGitLab,
		Branch:         "main",
		Auth: &types.GitCloneAuth{
			Type:  types.AuthModePAT,
			Token: token,
		},
	}, nil
}

// -------------------------------------------------------------------------------------
// Award Emojis (Reactions)
// -------------------------------------------------------------------------------------

func (s *GitLabService) CountReactions(ctx context.Context, orgData types.OrganizationAndTeamData, repo *types.RepositoryDescriptor, prNumber int) ([]types.ReactionsInComments, error) {
	comments, err := s.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}

	projectID := s.encodeProjectID(repo)
	result := make([]types.ReactionsInComments, 0, len(comments))

	for _, c := range comments {
		if c.ID == "" {
			continue
		}
		apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%s/award_emoji", projectID, prNumber, c.ID)
		resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var awards []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(body, &awards)

		thumbsUp := 0
		thumbsDown := 0
		for _, a := range awards {
			if strings.HasPrefix(a.Name, "thumbsup") || a.Name == "+1" {
				thumbsUp++
			} else if strings.HasPrefix(a.Name, "thumbsdown") || a.Name == "-1" {
				thumbsDown++
			}
		}

		var r types.ReactionsInComments
		r.Reaction = "+1"
		r.Count = thumbsUp
		r.Reactions.ThumbsUp = thumbsUp
		r.Reactions.ThumbsDown = thumbsDown
		r.Comment.ID = c.ID
		result = append(result, r)
	}

	return result, nil
}

func (s *GitLabService) AddReactionToPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reaction string) error {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return errors.New("missing repository ID for AddReactionToPR")
	}

	emojiName := "thumbsup"
	if reaction == "-1" || strings.Contains(strings.ToLower(reaction), "down") {
		emojiName = "thumbsdown"
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/award_emoji", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, map[string]string{"name": emojiName})
	if err != nil {
		return err
	}
	// 404/409 means already awarded
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("gitlab AddReactionToPR failed status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *GitLabService) AddReactionToComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reaction string) error {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return errors.New("missing repository ID for AddReactionToComment")
	}

	emojiName := "thumbsup"
	if reaction == "-1" || strings.Contains(strings.ToLower(reaction), "down") {
		emojiName = "thumbsdown"
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%d/award_emoji", projectID, prNumber, commentID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodPost, apiPath, map[string]string{"name": emojiName})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("gitlab AddReactionToComment failed status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *GitLabService) RemoveReactionsFromPR(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, reactions []string) error {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return errors.New("missing repository ID for RemoveReactionsFromPR")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/award_emoji", projectID, prNumber)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}

	var awards []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(body, &awards)

	for _, a := range awards {
		for _, target := range reactions {
			if strings.EqualFold(a.Name, target) || (target == "+1" && strings.HasPrefix(a.Name, "thumbsup")) {
				delPath := fmt.Sprintf("/projects/%s/merge_requests/%d/award_emoji/%d", projectID, prNumber, a.ID)
				_, _, _ = s.doRequest(ctx, orgData, http.MethodDelete, delPath, nil)
			}
		}
	}

	return nil
}

func (s *GitLabService) RemoveReactionsFromComment(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, prNumber int, commentID int64, reactions []string) error {
	projectID := s.encodeProjectID(&repo)
	if projectID == "" {
		return errors.New("missing repository ID for RemoveReactionsFromComment")
	}

	apiPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%d/award_emoji", projectID, prNumber, commentID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}

	var awards []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(body, &awards)

	for _, a := range awards {
		for _, target := range reactions {
			if strings.EqualFold(a.Name, target) || (target == "+1" && strings.HasPrefix(a.Name, "thumbsup")) {
				delPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%d/award_emoji/%d", projectID, prNumber, commentID, a.ID)
				_, _, _ = s.doRequest(ctx, orgData, http.MethodDelete, delPath, nil)
			}
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------
// Webhooks Lifecycle
// -------------------------------------------------------------------------------------

func (s *GitLabService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	// List all accessible repositories
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return fmt.Errorf("failed to list repos for webhook cleanup: %w", err)
	}

	for _, repo := range repos {
		encodedID := url.PathEscape(repo.ID)
		apiPath := fmt.Sprintf("/projects/%s/hooks", encodedID)

		resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var hooks []struct {
			ID  int    `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &hooks); err != nil {
			continue
		}

		for _, hook := range hooks {
			if strings.Contains(hook.URL, "scandrix") {
				delPath := fmt.Sprintf("/projects/%s/hooks/%d", encodedID, hook.ID)
				_, _, _ = s.doRequest(ctx, orgData, http.MethodDelete, delPath, nil)
			}
		}
	}

	return nil
}

func (s *GitLabService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	encodedID := url.PathEscape(repositoryID)
	apiPath := fmt.Sprintf("/projects/%s/hooks", encodedID)
	resp, body, err := s.doRequest(ctx, orgData, http.MethodGet, apiPath, nil)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var hooks []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &hooks); err == nil && len(hooks) > 0 {
		return true, nil
	}

	return false, nil
}

// -------------------------------------------------------------------------------------
// Comment Formatting
// -------------------------------------------------------------------------------------

func (s *GitLabService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
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

func (s *GitLabService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	var authorID int64
	switch p := payload.(type) {
	case map[string]any:
		if mr, ok := p["merge_request"].(map[string]any); ok {
			if id, ok := mr["author_id"].(float64); ok {
				authorID = int64(id)
			}
		}
		if authorID == 0 {
			if oa, ok := p["object_attributes"].(map[string]any); ok {
				if id, ok := oa["author_id"].(float64); ok {
					authorID = int64(id)
				}
			}
		}
	}

	if authorID == 0 || orgData.OrganizationID == "" {
		return nil, nil
	}

	cacheKey := fmt.Sprintf("gitlab-mr-author-%s-%d", orgData.OrganizationID, authorID)
	if cached, ok := s.getCached(cacheKey); ok {
		return cached.(*types.PullRequestUser), nil
	}

	author, err := s.GetUserByID(ctx, orgData, strconv.FormatInt(authorID, 10))
	if err != nil {
		return nil, err
	}

	if author != nil {
		s.setCached(cacheKey, author, 30*time.Minute)
	}

	return author, nil
}

func (s *GitLabService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
	if limit <= 0 {
		limit = 100
	}
	// Fetch recent PRs for repo and collect review comments up to limit
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

