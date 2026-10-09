// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package forgejo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

// ForgejoService implements contracts.ICodeManagementService for Forgejo / Gitea API v1.
// Translates libs/platform/infrastructure/adapters/services/forgejo.service.ts
var _ contracts.ICodeManagementService = (*ForgejoService)(nil)

const (
	defaultEmptyRepoSeedPath    = "README.md"
	defaultEmptyRepoSeedContent = "# Initial Repository\n\nInitialized by ScanDrix.\n"
	defaultEmptyRepoSeedMessage = "Initial commit by ScanDrix"
	defaultReviewCommentMarker  = "<!-- drixy-codereview -->"
)

type ForgejoService struct {
	baseURL            string
	httpClient         *http.Client
	insecureClient     *http.Client
	checksService      *ForgejoChecksService
	defaultTimeout     time.Duration
	cacheStore         sync.Map
	insecureSkipVerify bool
	insecureHosts      sync.Map
}

// ForgejoServiceConfig configures ForgejoService instances.
type ForgejoServiceConfig struct {
	BaseURL            string
	HTTPClient         *http.Client
	ChecksService      *ForgejoChecksService
	DefaultTimeout     time.Duration
	InsecureSkipVerify bool
	CustomCACert       string
}

// NewForgejoService creates a production Forgejo/Gitea service adapter.
func NewForgejoService(cfg ForgejoServiceConfig) *ForgejoService {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://codeberg.org"
	}

	timeout := cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	// Dedicated self-hosted client with HTTPS certificate verification bypass for private enterprise instances
	insecureTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	insecureClient := &http.Client{
		Transport: insecureTransport,
		Timeout:   timeout,
	}

	checksSvc := cfg.ChecksService
	if checksSvc == nil {
		checksSvc = NewForgejoChecksService(baseURL, client, nil)
	}

	svc := &ForgejoService{
		baseURL:            baseURL,
		httpClient:         client,
		insecureClient:     insecureClient,
		checksService:      checksSvc,
		defaultTimeout:     timeout,
		insecureSkipVerify: cfg.InsecureSkipVerify,
	}

	return svc
}

func (s *ForgejoService) Provider() models.SCMProvider {
	return models.ProviderForgejo
}

// SetInsecureSkipVerify toggles TLS verification globally for all self-hosted Forgejo requests.
func (s *ForgejoService) SetInsecureSkipVerify(skip bool) {
	s.insecureSkipVerify = skip
}

// RegisterSelfHostedHost registers an on-premise instance host with TLS verification bypass.
func (s *ForgejoService) RegisterSelfHostedHost(baseURL string, skipTLS bool) {
	clean := strings.TrimRight(baseURL, "/")
	if clean != "" {
		s.insecureHosts.Store(clean, skipTLS)
	}
}

// -------------------------------------------------------------------------------------
// Credential and Path Resolution Helpers
// -------------------------------------------------------------------------------------

func (s *ForgejoService) extractCredentials(orgData types.OrganizationAndTeamData) (baseURL, token string) {
	baseURL = s.baseURL
	if host, ok := orgData.IntegrationCredentials["host"].(string); ok && host != "" {
		baseURL = strings.TrimRight(host, "/")
	} else if host, ok := orgData.IntegrationCredentials["baseURL"].(string); ok && host != "" {
		baseURL = strings.TrimRight(host, "/")
	}

	token = orgData.AuthToken
	if token == "" && orgData.IntegrationCredentials != nil {
		if t, ok := orgData.IntegrationCredentials["token"].(string); ok && t != "" {
			token = t
		} else if t, ok := orgData.IntegrationCredentials["accessToken"].(string); ok && t != "" {
			token = t
		}
	}

	// Enterprise self-hosted certificate bypass resolution
	if orgData.IntegrationCredentials != nil {
		if skip, ok := orgData.IntegrationCredentials["skipTLSVerify"].(bool); ok && skip {
			s.insecureHosts.Store(baseURL, true)
		} else if skipStr, ok := orgData.IntegrationCredentials["skipTLSVerify"].(string); ok && (skipStr == "true" || skipStr == "1") {
			s.insecureHosts.Store(baseURL, true)
		} else if skip, ok := orgData.IntegrationCredentials["insecureSkipVerify"].(bool); ok && skip {
			s.insecureHosts.Store(baseURL, true)
		} else if skip, ok := orgData.IntegrationCredentials["skip_tls_verify"].(bool); ok && skip {
			s.insecureHosts.Store(baseURL, true)
		}
	}

	return baseURL, token
}

func (s *ForgejoService) parseOwnerAndRepo(repo *types.RepositoryDescriptor) (owner, repoName string) {
	if repo == nil {
		return "", ""
	}
	if repo.FullName != "" {
		parts := strings.Split(strings.Trim(repo.FullName, "/"), "/")
		if len(parts) >= 2 {
			return parts[0], parts[1]
		}
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner, repo.Name
	}
	parts := strings.Split(strings.Trim(repo.Name, "/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", repo.Name
}

func (s *ForgejoService) doRequest(
	ctx context.Context,
	baseURL, token, method, endpoint string,
	body any,
) ([]byte, int, error) {
	fullURL := baseURL
	if !strings.HasPrefix(endpoint, "http") {
		if !strings.HasPrefix(endpoint, "/api/v1") {
			fullURL = baseURL + "/api/v1" + endpoint
		} else {
			fullURL = baseURL + endpoint
		}
	} else {
		fullURL = endpoint
	}

	var reqBody io.Reader
	if body != nil {
		switch v := body.(type) {
		case []byte:
			reqBody = bytes.NewReader(v)
		case string:
			reqBody = strings.NewReader(v)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, 0, fmt.Errorf("marshal request body failed: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request failed: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := s.httpClient
	if s.insecureSkipVerify || os.Getenv("FORGEJO_INSECURE_SKIP_VERIFY") == "true" || os.Getenv("FORGEJO_SKIP_VERIFY") == "true" {
		client = s.insecureClient
	} else if skip, ok := s.insecureHosts.Load(strings.TrimRight(baseURL, "/")); ok && skip == true {
		client = s.insecureClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body failed: %w", err)
	}

	return respBytes, resp.StatusCode, nil
}

// -------------------------------------------------------------------------------------
// Issues API
// -------------------------------------------------------------------------------------

func (s *ForgejoService) ListIssues(ctx context.Context, params types.ListIssuesParams) ([]types.CodeManagementIssue, error) {
	baseURL, token := s.extractCredentials(params.OrganizationAndTeamData)
	owner := params.Repository.Owner
	repo := params.Repository.Name
	if owner == "" || repo == "" {
		return nil, errors.New("repository owner and name required")
	}

	state := "open"
	if params.Filters != nil && params.Filters.State != "" {
		state = string(params.Filters.State)
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=%s&type=issues&limit=50", owner, repo, state)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return []types.CodeManagementIssue{}, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo list issues returned status %d: %s", statusCode, string(respBytes))
	}

	var rawIssues []struct {
		ID        int64  `json:"id"`
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		ClosedAt  string `json:"closed_at"`
		User      struct {
			ID       int64  `json:"id"`
			Login    string `json:"login"`
			FullName string `json:"full_name"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		PullRequest any `json:"pull_request"`
	}

	if err := json.Unmarshal(respBytes, &rawIssues); err != nil {
		return nil, fmt.Errorf("decode issues response: %w", err)
	}

	var issues []types.CodeManagementIssue
	for _, raw := range rawIssues {
		if raw.PullRequest != nil {
			continue // filter out pull requests represented as issues
		}
		var labels []string
		for _, l := range raw.Labels {
			if l.Name != "" {
				labels = append(labels, l.Name)
			}
		}
		var assignees []string
		for _, a := range raw.Assignees {
			if a.Login != "" {
				assignees = append(assignees, a.Login)
			}
		}

		var authorUser *types.IssueAuthor
		if raw.User.Login != "" {
			userIDStr := strconv.FormatInt(raw.User.ID, 10)
			authorUser = &types.IssueAuthor{
				Username: raw.User.Login,
				ID:       &userIDStr,
			}
		}

		closedAtStr := raw.ClosedAt
		issues = append(issues, types.CodeManagementIssue{
			ID:        strconv.FormatInt(raw.ID, 10),
			Number:    raw.Number,
			Title:     raw.Title,
			Body:      &raw.Body,
			State:     raw.State,
			URL:       raw.HTMLURL,
			Labels:    labels,
			Assignees: assignees,
			Author:    authorUser,
			CreatedAt: raw.CreatedAt,
			UpdatedAt: raw.UpdatedAt,
			ClosedAt:  &closedAtStr,
			Platform:  models.ProviderForgejo,
		})
	}

	return issues, nil
}

func (s *ForgejoService) GetIssue(ctx context.Context, params types.GetIssueParams) (*types.CodeManagementIssue, error) {
	baseURL, token := s.extractCredentials(params.OrganizationAndTeamData)
	owner := params.Repository.Owner
	repo := params.Repository.Name
	if owner == "" || repo == "" {
		return nil, errors.New("repository owner and name required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, params.IssueNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get issue returned status %d: %s", statusCode, string(respBytes))
	}

	var raw struct {
		ID        int64  `json:"id"`
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		HTMLURL   string `json:"html_url"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		ClosedAt  string `json:"closed_at"`
		User      struct {
			ID       int64  `json:"id"`
			Login    string `json:"login"`
			FullName string `json:"full_name"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		PullRequest any `json:"pull_request"`
	}

	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode issue response: %w", err)
	}
	if raw.PullRequest != nil {
		return nil, nil // is a PR
	}

	var labels []string
	for _, l := range raw.Labels {
		if l.Name != "" {
			labels = append(labels, l.Name)
		}
	}
	var assignees []string
	for _, a := range raw.Assignees {
		if a.Login != "" {
			assignees = append(assignees, a.Login)
		}
	}

	var authorUser *types.IssueAuthor
	if raw.User.Login != "" {
		userIDStr := strconv.FormatInt(raw.User.ID, 10)
		authorUser = &types.IssueAuthor{
			Username: raw.User.Login,
			ID:       &userIDStr,
		}
	}

	closedAtStr := raw.ClosedAt
	return &types.CodeManagementIssue{
		ID:        strconv.FormatInt(raw.ID, 10),
		Number:    raw.Number,
		Title:     raw.Title,
		Body:      &raw.Body,
		State:     raw.State,
		URL:       raw.HTMLURL,
		Labels:    labels,
		Assignees: assignees,
		Author:    authorUser,
		CreatedAt: raw.CreatedAt,
		UpdatedAt: raw.UpdatedAt,
		ClosedAt:  &closedAtStr,
		Platform:  models.ProviderForgejo,
	}, nil
}

func (s *ForgejoService) SupportsIssues(ctx context.Context, orgData types.OrganizationAndTeamData) (bool, error) {
	return true, nil
}

// -------------------------------------------------------------------------------------
// Repository Inspection & Manipulation
// -------------------------------------------------------------------------------------

func (s *ForgejoService) FindRepositoryByName(ctx context.Context, orgData types.OrganizationAndTeamData, name string) (*types.Repository, error) {
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return nil, err
	}

	wanted := strings.ToLower(strings.TrimSpace(name))
	for _, r := range repos {
		fullName := strings.ToLower(r.FullName)
		if fullName == "" {
			fullName = strings.ToLower(r.OrganizationName + "/" + r.Name)
		}
		if strings.ToLower(r.Name) == wanted || fullName == wanted {
			return &types.Repository{
				ID:            r.ID,
				Name:          r.Name,
				FullName:      fullName,
				DefaultBranch: r.DefaultBranch,
				CloneURL:      r.HTTPURL,
				Provider:      models.ProviderForgejo,
			}, nil
		}
	}

	return nil, nil
}

func (s *ForgejoService) CreatePullRequestWithFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	sourceBranch, targetBranch, title, description, commitMessage string,
	author *types.GitActor,
	files []types.PullRequestFileChange,
) (*types.PullRequest, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("invalid repository identifier")
	}

	if targetBranch == "" {
		defBranch, err := s.GetDefaultBranch(ctx, orgData, &repo)
		if err == nil && defBranch != "" {
			targetBranch = defBranch
		} else {
			targetBranch = "main"
		}
	}

	if sourceBranch == "" {
		sourceBranch = fmt.Sprintf("scandrix-review-%d", time.Now().Unix())
	}
	if title == "" {
		title = "ScanDrix Automated Review Fixes"
	}
	if commitMessage == "" {
		commitMessage = "Apply fixes suggested by ScanDrix AI review"
	}

	// Seed repository if empty
	_ = s.ensureBaseBranchExists(ctx, baseURL, token, owner, repoName, targetBranch, author)

	// Upload files
	ok, err := s.UploadFiles(ctx, orgData, repo, sourceBranch, targetBranch, commitMessage, author, files)
	if err != nil || !ok {
		return nil, fmt.Errorf("upload files to branch %s failed: %w", sourceBranch, err)
	}

	// Create Pull Request
	payload := map[string]any{
		"base":  targetBranch,
		"head":  sourceBranch,
		"title": title,
		"body":  description,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls", owner, repoName)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo create pull request failed (status %d): %s", statusCode, string(respBytes))
	}

	var prData struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBytes, &prData); err != nil {
		return nil, fmt.Errorf("decode create pr response: %w", err)
	}

	return &types.PullRequest{
		ID:         strconv.FormatInt(prData.ID, 10),
		Number:     prData.Number,
		PullNumber: prData.Number,
		Title:      prData.Title,
		PRURL:      prData.HTMLURL,
		State:      "open",
	}, nil
}

func (s *ForgejoService) UploadFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	branchName, baseBranch, message string,
	author *types.GitActor,
	files []types.PullRequestFileChange,
) (bool, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return false, errors.New("invalid repository identifier")
	}

	if baseBranch == "" {
		baseBranch = "main"
	}
	if branchName == "" {
		branchName = baseBranch
	}
	if message == "" {
		message = "Update files via ScanDrix"
	}

	branchExists := s.checkBranchExists(ctx, baseURL, token, owner, repoName, branchName)
	refBranch := baseBranch
	if branchExists {
		refBranch = branchName
	}

	type changeFileItem struct {
		Operation string `json:"operation"`
		Path      string `json:"path"`
		Content   string `json:"content,omitempty"`
	}

	var changes []changeFileItem
	for _, f := range files {
		op := f.Operation
		if op == "" {
			op = "upsert"
		}
		cleanPath := strings.TrimPrefix(f.Path, "/")
		fileExists := s.checkFileExists(ctx, baseURL, token, owner, repoName, refBranch, cleanPath)

		if op == "delete" {
			if fileExists {
				changes = append(changes, changeFileItem{
					Operation: "delete",
					Path:      cleanPath,
				})
			}
			continue
		}

		opType := "create"
		if fileExists {
			opType = "update"
		}

		changes = append(changes, changeFileItem{
			Operation: opType,
			Path:      cleanPath,
			Content:   base64.StdEncoding.EncodeToString([]byte(f.Content)),
		})
	}

	if len(changes) == 0 {
		return true, nil
	}

	authorIdentity := map[string]string{
		"name":  "ScanDrix Review Bot",
		"email": "drixy@scandrix.dev",
	}
	if author != nil && author.Name != "" {
		authorIdentity["name"] = author.Name
		if author.Email != "" {
			authorIdentity["email"] = author.Email
		}
	}

	payload := map[string]any{
		"files":     changes,
		"message":   message,
		"branch":    refBranch,
		"author":    authorIdentity,
		"committer": authorIdentity,
	}
	if !branchExists && branchName != baseBranch {
		payload["new_branch"] = branchName
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/contents", owner, repoName)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return false, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return false, fmt.Errorf("forgejo change-files returned status %d: %s", statusCode, string(respBytes))
	}

	return true, nil
}

func (s *ForgejoService) checkBranchExists(ctx context.Context, baseURL, token, owner, repo, branch string) bool {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches/%s", owner, repo, url.PathEscape(branch))
	_, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	return err == nil && statusCode == http.StatusOK
}

func (s *ForgejoService) checkFileExists(ctx context.Context, baseURL, token, owner, repo, branch, filePath string) bool {
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, filePath, url.QueryEscape(branch))
	_, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	return err == nil && statusCode == http.StatusOK
}

func (s *ForgejoService) ensureBaseBranchExists(
	ctx context.Context,
	baseURL, token, owner, repo, baseBranch string,
	author *types.GitActor,
) error {
	if s.checkBranchExists(ctx, baseURL, token, owner, repo, baseBranch) {
		return nil
	}

	authorIdentity := map[string]string{
		"name":  "ScanDrix Review Bot",
		"email": "drixy@scandrix.dev",
	}
	if author != nil && author.Name != "" {
		authorIdentity["name"] = author.Name
		if author.Email != "" {
			authorIdentity["email"] = author.Email
		}
	}

	payload := map[string]any{
		"files": []map[string]any{
			{
				"operation": "create",
				"path":      defaultEmptyRepoSeedPath,
				"content":   base64.StdEncoding.EncodeToString([]byte(defaultEmptyRepoSeedContent)),
			},
		},
		"message":   defaultEmptyRepoSeedMessage,
		"branch":    baseBranch,
		"author":    authorIdentity,
		"committer": authorIdentity,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/contents", owner, repo)
	_, _, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	return err
}

// -------------------------------------------------------------------------------------
// Pull Requests
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetPullRequests(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	state, author, branch string,
) ([]*types.PullRequest, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	queryState := "all"
	if state != "" && state != "all" {
		queryState = state
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&limit=50", owner, repoName, queryState)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return []*types.PullRequest{}, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo list pull requests returned status %d: %s", statusCode, string(respBytes))
	}

	var rawPRs []forgejoPullRequestJSON
	if err := json.Unmarshal(respBytes, &rawPRs); err != nil {
		return nil, fmt.Errorf("decode prs response: %w", err)
	}

	var result []*types.PullRequest
	for _, raw := range rawPRs {
		pr := s.transformPullRequest(raw, owner, repoName, orgData)
		if author != "" && pr.User.Login != author && pr.User.Name != author {
			continue
		}
		if branch != "" && pr.SourceRefName != branch && pr.TargetRefName != branch {
			continue
		}
		result = append(result, pr)
	}

	return result, nil
}

func (s *ForgejoService) GetPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (*types.PullRequest, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get pull request returned status %d: %s", statusCode, string(respBytes))
	}

	var raw forgejoPullRequestJSON
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode pr response: %w", err)
	}

	return s.transformPullRequest(raw, owner, repoName, orgData), nil
}

func (s *ForgejoService) GetPullRequestByNumber(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (*types.PullRequest, error) {
	return s.GetPullRequest(ctx, orgData, repo, prNumber)
}

func (s *ForgejoService) GetPullRequestsByRepository(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
) ([]*types.PullRequest, error) {
	return s.GetPullRequests(ctx, orgData, &repo, "open", "", "")
}

type forgejoPullRequestJSON struct {
	ID        int64     `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Merged    bool      `json:"merged"`
	Draft     bool      `json:"draft"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ClosedAt  *time.Time`json:"closed_at"`
	MergedAt  *time.Time`json:"merged_at"`
	User      struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		FullName string `json:"full_name"`
	} `json:"user"`
	Head struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			ID            int64  `json:"id"`
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			ID            int64  `json:"id"`
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repo"`
	} `json:"base"`
}

func (s *ForgejoService) transformPullRequest(
	raw forgejoPullRequestJSON,
	owner, repoName string,
	orgData types.OrganizationAndTeamData,
) *types.PullRequest {
	state := "open"
	if raw.Merged {
		state = "merged"
	} else if raw.State == "closed" {
		state = "closed"
	}

	fullName := fmt.Sprintf("%s/%s", owner, repoName)
	if raw.Base.Repo.FullName != "" {
		fullName = raw.Base.Repo.FullName
	}

	isDraft := raw.Draft
	titleLower := strings.ToLower(raw.Title)
	if strings.HasPrefix(titleLower, "wip:") || strings.HasPrefix(titleLower, "[wip]") ||
		strings.HasPrefix(titleLower, "draft:") || strings.HasPrefix(titleLower, "[draft]") {
		isDraft = true
	}

	userName := raw.User.FullName
	if userName == "" {
		userName = raw.User.Login
	}

	closedAtStr := ""
	if raw.ClosedAt != nil {
		closedAtStr = raw.ClosedAt.Format(time.RFC3339)
	}
	mergedAtStr := ""
	if raw.MergedAt != nil {
		mergedAtStr = raw.MergedAt.Format(time.RFC3339)
	}

	return &types.PullRequest{
		ID:             strconv.FormatInt(raw.ID, 10),
		Number:         raw.Number,
		PullNumber:     raw.Number,
		OrganizationID: orgData.OrganizationID,
		Title:          raw.Title,
		Body:           raw.Body,
		State:          state,
		PRURL:          raw.HTMLURL,
		Repository:     fullName,
		RepositoryID:   strconv.FormatInt(raw.Base.Repo.ID, 10),
		SourceRefName:  raw.Head.Ref,
		TargetRefName:  raw.Base.Ref,
		CreatedAt:      raw.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      raw.UpdatedAt.Format(time.RFC3339),
		ClosedAt:       closedAtStr,
		MergedAt:       mergedAtStr,
		IsDraft:        isDraft,
		User: types.PullRequestUser{
			ID:    strconv.FormatInt(raw.User.ID, 10),
			Login: raw.User.Login,
			Name:  userName,
		},
		Head: types.PullRequestBranchReference{
			Ref: raw.Head.Ref,
			SHA: raw.Head.SHA,
			Repo: types.PullRequestRepoReference{
				ID:            strconv.FormatInt(raw.Head.Repo.ID, 10),
				Name:          raw.Head.Repo.Name,
				FullName:      raw.Head.Repo.FullName,
				DefaultBranch: raw.Head.Repo.DefaultBranch,
			},
		},
		Base: types.PullRequestBranchReference{
			Ref: raw.Base.Ref,
			SHA: raw.Base.SHA,
			Repo: types.PullRequestRepoReference{
				ID:            strconv.FormatInt(raw.Base.Repo.ID, 10),
				Name:          raw.Base.Repo.Name,
				FullName:      fullName,
				DefaultBranch: raw.Base.Repo.DefaultBranch,
			},
		},
	}
}

// -------------------------------------------------------------------------------------
// Repositories & Organizations
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetRepositories(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	archived *bool,
	visibility, language string,
) ([]*types.Repositories, error) {
	baseURL, token := s.extractCredentials(orgData)
	var repos []*types.Repositories
	seenIDs := make(map[string]bool)

	// 1. Current user repos
	userEndpoint := "/user/repos?limit=50"
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, userEndpoint, nil)
	if err == nil && statusCode == http.StatusOK {
		var rawUserRepos []forgejoRepoJSON
		if err := json.Unmarshal(respBytes, &rawUserRepos); err == nil {
			for _, r := range rawUserRepos {
				idStr := strconv.FormatInt(r.ID, 10)
				if !seenIDs[idStr] {
					seenIDs[idStr] = true
					repos = append(repos, s.transformRepository(r))
				}
			}
		}
	}

	// 2. Organization repos
	orgsEndpoint := "/user/orgs?limit=50"
	respBytes, statusCode, err = s.doRequest(ctx, baseURL, token, http.MethodGet, orgsEndpoint, nil)
	if err == nil && statusCode == http.StatusOK {
		var orgs []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(respBytes, &orgs); err == nil {
			for _, org := range orgs {
				if org.Name == "" {
					continue
				}
				orgReposEndpoint := fmt.Sprintf("/orgs/%s/repos?limit=50", org.Name)
				rBytes, rStatus, rErr := s.doRequest(ctx, baseURL, token, http.MethodGet, orgReposEndpoint, nil)
				if rErr == nil && rStatus == http.StatusOK {
					var rawOrgRepos []forgejoRepoJSON
					if err := json.Unmarshal(rBytes, &rawOrgRepos); err == nil {
						for _, r := range rawOrgRepos {
							idStr := strconv.FormatInt(r.ID, 10)
							if !seenIDs[idStr] {
								seenIDs[idStr] = true
								repos = append(repos, s.transformRepository(r))
							}
						}
					}
				}
			}
		}
	}

	// Filtering
	var filtered []*types.Repositories
	for _, r := range repos {
		if visibility != "" && r.Visibility != visibility {
			continue
		}
		if language != "" && !strings.EqualFold(r.Language, language) {
			continue
		}
		filtered = append(filtered, r)
	}

	return filtered, nil
}

type forgejoRepoJSON struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	CloneURL      string    `json:"clone_url"`
	HTMLURL       string    `json:"html_url"`
	AvatarURL     string    `json:"avatar_url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	Archived      bool      `json:"archived"`
	UpdatedAt     time.Time `json:"updated_at"`
	Owner         struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

func (s *ForgejoService) transformRepository(raw forgejoRepoJSON) *types.Repositories {
	visibility := "public"
	if raw.Private {
		visibility = "private"
	}
	avatar := raw.AvatarURL
	if avatar == "" {
		avatar = raw.Owner.AvatarURL
	}

	return &types.Repositories{
		ID:               strconv.FormatInt(raw.ID, 10),
		Name:             raw.Name,
		FullName:         raw.FullName,
		HTTPURL:          raw.CloneURL,
		AvatarURL:        avatar,
		OrganizationName: raw.Owner.Login,
		Visibility:       visibility,
		DefaultBranch:    raw.DefaultBranch,
		LastActivityAt:   raw.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *ForgejoService) GetOrganizations(ctx context.Context, orgData types.OrganizationAndTeamData) ([]*types.Organization, error) {
	baseURL, token := s.extractCredentials(orgData)
	endpoint := "/user/orgs?limit=50"
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo list orgs returned status %d: %s", statusCode, string(respBytes))
	}

	var rawOrgs []struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		UserName  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(respBytes, &rawOrgs); err != nil {
		return nil, fmt.Errorf("decode orgs: %w", err)
	}

	var orgs []*types.Organization
	for _, o := range rawOrgs {
		name := o.Name
		if name == "" {
			name = o.UserName
		}
		orgs = append(orgs, &types.Organization{
			ID:        strconv.FormatInt(o.ID, 10),
			Name:      name,
			AvatarURL: o.AvatarURL,
		})
	}
	return orgs, nil
}

func (s *ForgejoService) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	baseURL, token := s.extractCredentials(orgData)
	orgs, _ := s.GetOrganizations(ctx, orgData)

	var members []types.PullRequestAuthor
	seen := make(map[string]bool)

	for _, org := range orgs {
		endpoint := fmt.Sprintf("/orgs/%s/members?limit=50", org.Name)
		respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
		if err == nil && statusCode == http.StatusOK {
			var rawUsers []struct {
				ID       int64  `json:"id"`
				Login    string `json:"login"`
				FullName string `json:"full_name"`
			}
			if err := json.Unmarshal(respBytes, &rawUsers); err == nil {
				for _, u := range rawUsers {
					idStr := strconv.FormatInt(u.ID, 10)
					if !seen[idStr] {
						seen[idStr] = true
						name := u.FullName
						if name == "" {
							name = u.Login
						}
						members = append(members, types.PullRequestAuthor{
							ID:   idStr,
							Name: name,
						})
					}
				}
			}
		}
	}

	// Fallback to current user if no org members
	if len(members) == 0 {
		user, err := s.GetCurrentUser(ctx, orgData)
		if err == nil && user != nil {
			members = append(members, types.PullRequestAuthor{
				ID:   user.ID,
				Name: user.Name,
			})
		}
	}

	return members, nil
}

func (s *ForgejoService) VerifyConnection(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.CodeManagementConnectionStatus, error) {
	user, err := s.GetCurrentUser(ctx, orgData)
	if err != nil || user == nil {
		return &types.CodeManagementConnectionStatus{
			HasConnection:   false,
			IsConnected:     false,
			IsSetupComplete: false,
			Message:         "Failed to authenticate with Forgejo",
			PlatformName:    string(models.ProviderForgejo),
		}, err
	}

	return &types.CodeManagementConnectionStatus{
		HasConnection:   true,
		IsConnected:     true,
		IsSetupComplete: true,
		Message:         fmt.Sprintf("Authenticated as %s (%s)", user.Name, user.Login),
		PlatformName:    string(models.ProviderForgejo),
	}, nil
}

// -------------------------------------------------------------------------------------
// Pull Request Files & Diffs
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetFilesByPullRequestId(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestFile, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	filesEndpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?limit=100", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, filesEndpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get pr files returned status %d: %s", statusCode, string(respBytes))
	}

	var rawFiles []struct {
		Filename         string `json:"filename"`
		Status           string `json:"status"`
		Additions        int    `json:"additions"`
		Deletions        int    `json:"deletions"`
		Changes          int    `json:"changes"`
		PreviousFilename string `json:"previous_filename"`
	}
	if err := json.Unmarshal(respBytes, &rawFiles); err != nil {
		return nil, fmt.Errorf("decode pr files: %w", err)
	}

	// Fetch unified diff to associate patches per file
	diffEndpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d.diff", owner, repoName, prNumber)
	diffBytes, diffStatus, _ := s.doRequest(ctx, baseURL, token, http.MethodGet, diffEndpoint, nil)
	patchMap := make(map[string]string)
	if diffStatus == http.StatusOK && len(diffBytes) > 0 {
		patchMap = s.parseUnifiedDiff(string(diffBytes))
	}

	var files []*types.PullRequestFile
	for _, f := range rawFiles {
		patch := patchMap[f.Filename]
		files = append(files, &types.PullRequestFile{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Changes:   f.Changes,
			Patch:     patch,
		})
	}

	return files, nil
}

func (s *ForgejoService) parseUnifiedDiff(diffContent string) map[string]string {
	patchMap := make(map[string]string)
	if diffContent == "" {
		return patchMap
	}

	// Split by file diff headers
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
		// Format: a/path b/path
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

func (s *ForgejoService) GetChangedFilesSinceLastCommit(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	baseSHA, headSHA string,
) ([]string, error) {
	files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, prNumber)
	if err != nil {
		return nil, err
	}
	var filenames []string
	for _, f := range files {
		filenames = append(filenames, f.Filename)
	}
	return filenames, nil
}

func (s *ForgejoService) GetPullRequestsWithFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]*types.PullRequestWithFiles, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "open", "", "")
	if err != nil {
		return nil, err
	}

	var result []*types.PullRequestWithFiles
	for _, pr := range prs {
		files, err := s.GetFilesByPullRequestId(ctx, orgData, repo, pr.Number)
		if err != nil {
			continue
		}

		result = append(result, &types.PullRequestWithFiles{
			PullRequest:      *pr,
			ID:               pr.Number,
			PullNumber:       pr.Number,
			State:            pr.State,
			Title:            pr.Title,
			Repository:       pr.Repository,
			PullRequestFiles: files,
			Files:            files,
		})
	}

	return result, nil
}

func (s *ForgejoService) GetPullRequestsForRTTM(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]*types.PullRequestCodeReviewTime, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "closed", "", "")
	if err != nil {
		return nil, err
	}

	var reviewTimes []*types.PullRequestCodeReviewTime
	for _, pr := range prs {
		created, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		var closed *time.Time
		if pr.ClosedAt != "" {
			if t, err := time.Parse(time.RFC3339, pr.ClosedAt); err == nil {
				closed = &t
			}
		}
		reviewTimes = append(reviewTimes, &types.PullRequestCodeReviewTime{
			PRNumber:  pr.Number,
			CreatedAt: created,
			ClosedAt:  closed,
			Author:    pr.User.Login,
		})
	}
	return reviewTimes, nil
}

// -------------------------------------------------------------------------------------
// Reviews, Comments, and Discussions
// -------------------------------------------------------------------------------------

func (s *ForgejoService) CreateReviewComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	comment types.PullRequestReviewComment,
) (*types.PullRequestReviewComment, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	line := comment.Line
	if line <= 0 && comment.StartLine > 0 {
		line = comment.StartLine
	}
	if line <= 0 {
		line = 1
	}

	body := comment.Body
	if !strings.Contains(body, defaultReviewCommentMarker) {
		body = s.FormatReviewCommentBody(body, "", true, true)
	}

	payload := map[string]any{
		"body":  "",
		"event": "COMMENT",
		"comments": []map[string]any{
			{
				"path":         comment.Path,
				"body":         body,
				"new_position": line,
			},
		},
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo create review comment failed (status %d): %s", statusCode, string(respBytes))
	}

	var rawReview struct {
		ID       int64 `json:"id"`
		Comments []struct {
			ID        int64     `json:"id"`
			Body      string    `json:"body"`
			Path      string    `json:"path"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(respBytes, &rawReview); err != nil {
		return nil, fmt.Errorf("decode review comment response: %w", err)
	}

	commentID := strconv.FormatInt(rawReview.ID, 10)
	createdAt := time.Now().Format(time.RFC3339)
	if len(rawReview.Comments) > 0 {
		commentID = strconv.FormatInt(rawReview.Comments[0].ID, 10)
		createdAt = rawReview.Comments[0].CreatedAt.Format(time.RFC3339)
	}

	return &types.PullRequestReviewComment{
		ID:        commentID,
		ThreadID:  strconv.FormatInt(rawReview.ID, 10),
		Path:      comment.Path,
		Line:      line,
		Body:      body,
		CreatedAt: createdAt,
	}, nil
}

func (s *ForgejoService) CreateCommentInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	return s.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *ForgejoService) CreateIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	payload := map[string]string{
		"body": body,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo create issue comment returned status %d: %s", statusCode, string(respBytes))
	}

	var rawComment struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(respBytes, &rawComment); err != nil {
		return nil, fmt.Errorf("decode issue comment: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(rawComment.ID, 10),
		ThreadID:  strconv.FormatInt(rawComment.ID, 10),
		Body:      rawComment.Body,
		CreatedAt: rawComment.CreatedAt.Format(time.RFC3339),
		UpdatedAt: rawComment.UpdatedAt.Format(time.RFC3339),
	}, nil
}

func (s *ForgejoService) CreateSingleIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	body string,
) (*types.PullRequestReviewComment, error) {
	return s.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *ForgejoService) UpdateIssueComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	commentID string,
	body string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	payload := map[string]string{
		"body": body,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/comments/%s", owner, repoName, commentID)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPatch, endpoint, payload)
	if err != nil {
		return err
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("forgejo update comment returned status %d: %s", statusCode, string(respBytes))
	}
	return nil
}

func (s *ForgejoService) MinimizeComment(ctx context.Context, orgData types.OrganizationAndTeamData, commentID string, reason string) error {
	// Forgejo API does not support minimizing comments yet - return nil as documented in ScanDrix platform specs
	return nil
}

func (s *ForgejoService) GetPullRequestReviewComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	commentID string,
) (*types.PullRequestReviewComment, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/comments/%s", owner, repoName, commentID)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get comment returned status %d: %s", statusCode, string(respBytes))
	}

	var raw struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode comment: %w", err)
	}

	return &types.PullRequestReviewComment{
		ID:        strconv.FormatInt(raw.ID, 10),
		ThreadID:  strconv.FormatInt(raw.ID, 10),
		Body:      raw.Body,
		CreatedAt: raw.CreatedAt.Format(time.RFC3339),
		UpdatedAt: raw.UpdatedAt.Format(time.RFC3339),
		Author: &types.PullRequestCommentAuthor{
			Username: raw.User.Login,
		},
	}, nil
}

func (s *ForgejoService) CreateResponseToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	parentCommentID, body string,
) (*types.PullRequestReviewComment, error) {
	// Post as issue comment or review reply
	return s.CreateIssueComment(ctx, orgData, repo, prNumber, body)
}

func (s *ForgejoService) UpdateDescriptionInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	description string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	payload := map[string]string{
		"body": description,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPatch, endpoint, payload)
	if err != nil {
		return err
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("forgejo update pr description returned status %d: %s", statusCode, string(respBytes))
	}
	return nil
}

func (s *ForgejoService) UpdateResponseToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	parentID, commentID, body string,
) (*types.PullRequestReviewComment, error) {
	if err := s.UpdateIssueComment(ctx, orgData, repo, commentID, body); err != nil {
		return nil, err
	}
	return s.GetPullRequestReviewComment(ctx, orgData, repo, commentID)
}

func (s *ForgejoService) GetAllCommentsInPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?limit=100", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get pr comments returned status %d: %s", statusCode, string(respBytes))
	}

	var rawComments []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(respBytes, &rawComments); err != nil {
		return nil, fmt.Errorf("decode comments: %w", err)
	}

	var comments []*types.PullRequestReviewComment
	for _, c := range rawComments {
		comments = append(comments, &types.PullRequestReviewComment{
			ID:        strconv.FormatInt(c.ID, 10),
			ThreadID:  strconv.FormatInt(c.ID, 10),
			Body:      c.Body,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
			UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
			Author: &types.PullRequestCommentAuthor{
				Username: c.User.Login,
			},
		})
	}
	return comments, nil
}

func (s *ForgejoService) GetPullRequestReviewComments(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?limit=50", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get reviews returned status %d: %s", statusCode, string(respBytes))
	}

	var rawReviews []struct {
		ID       int64  `json:"id"`
		State    string `json:"state"`
		Comments []struct {
			ID        int64     `json:"id"`
			Body      string    `json:"body"`
			Path      string    `json:"path"`
			Line      int       `json:"line"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(respBytes, &rawReviews); err != nil {
		return nil, fmt.Errorf("decode reviews: %w", err)
	}

	var comments []*types.PullRequestReviewComment
	for _, r := range rawReviews {
		// Fetch review comments if not inlined
		rComments := r.Comments
		if len(rComments) == 0 {
			reviewCommentsURL := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews/%d/comments", owner, repoName, prNumber, r.ID)
			rcBytes, rcStatus, rcErr := s.doRequest(ctx, baseURL, token, http.MethodGet, reviewCommentsURL, nil)
			if rcErr == nil && rcStatus == http.StatusOK {
				_ = json.Unmarshal(rcBytes, &rComments)
			}
		}

		for _, c := range rComments {
			comments = append(comments, &types.PullRequestReviewComment{
				ID:        strconv.FormatInt(c.ID, 10),
				ThreadID:  strconv.FormatInt(r.ID, 10),
				Path:      c.Path,
				Line:      c.Line,
				Body:      c.Body,
				CreatedAt: c.CreatedAt.Format(time.RFC3339),
				UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
				Author: &types.PullRequestCommentAuthor{
					Username: c.User.Login,
				},
			})
		}
	}

	return comments, nil
}

func (s *ForgejoService) GetPullRequestReviewThreads(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.PullRequestReviewComment, error) {
	return s.GetPullRequestReviewComments(ctx, orgData, repo, prNumber)
}

func (s *ForgejoService) MarkReviewCommentAsResolved(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	threadID string,
) error {
	// Not supported in current Forgejo API versions
	return nil
}

// -------------------------------------------------------------------------------------
// PR Review Decision & Actions
// -------------------------------------------------------------------------------------

func (s *ForgejoService) ApprovePullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	message string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	payload := map[string]string{
		"event": "APPROVED",
		"body":  message,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return err
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("forgejo approve PR failed (status %d): %s", statusCode, string(respBytes))
	}
	return nil
}

func (s *ForgejoService) RequestChangesPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	message string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	body := message
	if body == "" {
		body = "# Found critical issues please review the requested changes\n\nAutomated review by ScanDrix."
	}

	payload := map[string]string{
		"event": "REQUEST_CHANGES",
		"body":  body,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return err
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("forgejo request changes failed (status %d): %s", statusCode, string(respBytes))
	}
	return nil
}

func (s *ForgejoService) MergePullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
	method string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	doType := "merge"
	switch method {
	case "squash":
		doType = "squash"
	case "rebase":
		doType = "rebase"
	}

	payload := map[string]any{
		"Do":                        doType,
		"delete_branch_after_merge": true,
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return err
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("forgejo merge PR failed (status %d): %s", statusCode, string(respBytes))
	}
	return nil
}

func (s *ForgejoService) GetListOfValidReviews(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]string, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get reviews returned status %d: %s", statusCode, string(respBytes))
	}

	var rawReviews []struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(respBytes, &rawReviews); err != nil {
		return nil, fmt.Errorf("decode reviews: %w", err)
	}

	var states []string
	for _, r := range rawReviews {
		if r.State != "" {
			states = append(states, r.State)
		}
	}
	return states, nil
}

func (s *ForgejoService) GetPullRequestsWithChangesRequested(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) ([]types.PullRequestsWithChangesRequested, error) {
	prs, err := s.GetPullRequests(ctx, orgData, repo, "open", "", "")
	if err != nil {
		return nil, err
	}

	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)

	var result []types.PullRequestsWithChangesRequested
	for _, pr := range prs {
		endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, pr.Number)
		respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
		if err != nil || statusCode != http.StatusOK {
			continue
		}

		var reviews []struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(respBytes, &reviews); err == nil && len(reviews) > 0 {
			latest := reviews[len(reviews)-1]
			if latest.State == "REQUEST_CHANGES" {
				result = append(result, types.PullRequestsWithChangesRequested{
					Title:          pr.Title,
					Number:         pr.Number,
					ReviewDecision: types.PullRequestReviewStateChangesRequested,
				})
			}
		}
	}

	return result, nil
}

func (s *ForgejoService) GetReviewStatusByPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (types.PullRequestReviewState, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return types.PullRequestReviewStatePending, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return types.PullRequestReviewStatePending, nil
	}

	var reviews []struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(respBytes, &reviews); err != nil || len(reviews) == 0 {
		return types.PullRequestReviewStatePending, nil
	}

	latest := reviews[len(reviews)-1]
	switch latest.State {
	case "APPROVED":
		return types.PullRequestReviewStateApproved, nil
	case "REQUEST_CHANGES":
		return types.PullRequestReviewStateChangesRequested, nil
	default:
		return types.PullRequestReviewStatePending, nil
	}
}

func (s *ForgejoService) CheckIfPullRequestShouldBeApproved(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	prNumber int,
	repo types.RepositoryDescriptor,
) (bool, error) {
	status, err := s.GetReviewStatusByPullRequest(ctx, orgData, &repo, prNumber)
	if err != nil {
		return false, err
	}
	return status == types.PullRequestReviewStateApproved, nil
}

func (s *ForgejoService) IsDraftPullRequest(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) (bool, error) {
	pr, err := s.GetPullRequest(ctx, orgData, repo, prNumber)
	if err != nil || pr == nil {
		return false, err
	}
	return pr.IsDraft, nil
}

// -------------------------------------------------------------------------------------
// Commits & Content
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetCommits(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	branch, author string,
) ([]*types.Commit, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/commits?limit=50", owner, repoName)
	if branch != "" {
		endpoint += fmt.Sprintf("&sha=%s", url.QueryEscape(branch))
	}

	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get commits returned status %d: %s", statusCode, string(respBytes))
	}

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
	}
	if err := json.Unmarshal(respBytes, &rawCommits); err != nil {
		return nil, fmt.Errorf("decode commits: %w", err)
	}

	var commits []*types.Commit
	for _, c := range rawCommits {
		if author != "" && c.Commit.Author.Name != author && c.Commit.Author.Email != author {
			continue
		}
		commits = append(commits, &types.Commit{
			SHA:         c.SHA,
			Message:     c.Commit.Message,
			AuthorName:  c.Commit.Author.Name,
			AuthorLogin: c.Commit.Author.Name,
			Date:        c.Commit.Author.Date,
			Author: types.GitActor{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
			},
		})
	}
	return commits, nil
}

func (s *ForgejoService) GetCommitsForPullRequestForCodeReview(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]*types.Commit, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?limit=100", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get pr commits returned status %d: %s", statusCode, string(respBytes))
	}

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
	}
	if err := json.Unmarshal(respBytes, &rawCommits); err != nil {
		return nil, fmt.Errorf("decode pr commits: %w", err)
	}

	var commits []*types.Commit
	for _, c := range rawCommits {
		commits = append(commits, &types.Commit{
			SHA:         c.SHA,
			Message:     c.Commit.Message,
			AuthorName:  c.Commit.Author.Name,
			AuthorLogin: c.Commit.Author.Name,
			Date:        c.Commit.Author.Date,
			Author: types.GitActor{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
			},
		})
	}

	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Date.Before(commits[j].Date)
	})

	return commits, nil
}

func (s *ForgejoService) GetRepositoryContentFile(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	filePath, ref string,
) (*types.RepositoryFile, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	cleanPath := strings.TrimPrefix(filePath, "/")
	endpoint := fmt.Sprintf("/repos/%s/%s/raw/%s", owner, repoName, cleanPath)
	if ref != "" {
		endpoint += fmt.Sprintf("?ref=%s", url.QueryEscape(ref))
	}

	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get raw content returned status %d: %s", statusCode, string(respBytes))
	}

	return &types.RepositoryFile{
		Path:    cleanPath,
		Content: string(respBytes),
		Size:    int64(len(respBytes)),
	}, nil
}

func (s *ForgejoService) GetRepositoryContentBatch(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	files []string,
	ref string,
) (map[string]*types.RepositoryFile, error) {
	result := make(map[string]*types.RepositoryFile)
	for _, f := range files {
		rf, err := s.GetRepositoryContentFile(ctx, orgData, &repo, f, ref)
		if err == nil && rf != nil {
			result[f] = rf
		}
	}
	return result, nil
}

func (s *ForgejoService) GetRepositoryAllFiles(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	branch string,
	filePatterns, excludePatterns []string,
	maxFiles int,
) ([]*types.RepositoryFile, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return nil, errors.New("repository descriptor required")
	}

	if branch == "" {
		branch = "main"
	}
	if maxFiles <= 0 {
		maxFiles = 200
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repoName, url.PathEscape(branch))
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get tree returned status %d: %s", statusCode, string(respBytes))
	}

	var rawTree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(respBytes, &rawTree); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}

	var files []*types.RepositoryFile
	for _, item := range rawTree.Tree {
		if item.Type != "blob" {
			continue
		}
		if len(files) >= maxFiles {
			break
		}

		matched := true
		if len(filePatterns) > 0 {
			matched = false
			for _, pat := range filePatterns {
				if ok, _ := filepath.Match(pat, item.Path); ok || strings.HasSuffix(item.Path, pat) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}

		excluded := false
		for _, ex := range excludePatterns {
			if ok, _ := filepath.Match(ex, item.Path); ok || strings.Contains(item.Path, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		contentFile, err := s.GetRepositoryContentFile(ctx, orgData, &repo, item.Path, branch)
		if err == nil && contentFile != nil {
			files = append(files, contentFile)
		}
	}

	return files, nil
}

func (s *ForgejoService) GetDefaultBranch(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) (string, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return "main", nil
	}

	endpoint := fmt.Sprintf("/repos/%s/%s", owner, repoName)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return "main", nil
	}

	var raw struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(respBytes, &raw); err == nil && raw.DefaultBranch != "" {
		return raw.DefaultBranch, nil
	}

	return "main", nil
}

func (s *ForgejoService) GetLanguageRepository(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
) (map[string]int, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return map[string]int{}, nil
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/languages", owner, repoName)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return map[string]int{}, nil
	}

	var langs map[string]int
	_ = json.Unmarshal(respBytes, &langs)
	if langs == nil {
		langs = make(map[string]int)
	}
	return langs, nil
}

func (s *ForgejoService) GetCloneParams(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
) (*types.GitCloneParams, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	fullName := repo.FullName
	if fullName == "" {
		fullName = fmt.Sprintf("%s/%s", owner, repoName)
	}

	cloneURL := fmt.Sprintf("%s/%s.git", baseURL, fullName)
	defaultBranch, _ := s.GetDefaultBranch(ctx, orgData, &repo)

	return &types.GitCloneParams{
		URL:            cloneURL,
		Provider:       models.ProviderForgejo,
		OrganizationID: orgData.OrganizationID,
		RepositoryID:   repo.ID,
		RepositoryName: repoName,
		Branch:         defaultBranch,
		Auth: &types.GitCloneAuth{
			Type:     types.AuthModePAT,
			Username: "oauth2",
			Token:    token,
		},
	}, nil
}

// -------------------------------------------------------------------------------------
// Users & Authentication
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetCurrentUser(ctx context.Context, orgData types.OrganizationAndTeamData) (*types.PullRequestUser, error) {
	baseURL, token := s.extractCredentials(orgData)
	endpoint := "/user"
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get current user returned status %d: %s", statusCode, string(respBytes))
	}

	var raw struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	name := raw.FullName
	if name == "" {
		name = raw.Login
	}

	return &types.PullRequestUser{
		ID:    strconv.FormatInt(raw.ID, 10),
		Login: raw.Login,
		Name:  name,
		Email: raw.Email,
	}, nil
}

func (s *ForgejoService) GetUserByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, username string) (*types.PullRequestUser, error) {
	baseURL, token := s.extractCredentials(orgData)
	endpoint := fmt.Sprintf("/users/%s", url.PathEscape(username))
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("forgejo get user returned status %d: %s", statusCode, string(respBytes))
	}

	var raw struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	name := raw.FullName
	if name == "" {
		name = raw.Login
	}

	return &types.PullRequestUser{
		ID:    strconv.FormatInt(raw.ID, 10),
		Login: raw.Login,
		Name:  name,
		Email: raw.Email,
	}, nil
}

func (s *ForgejoService) GetUsersByUsername(ctx context.Context, orgData types.OrganizationAndTeamData, usernames []string) (map[string]*types.PullRequestUser, error) {
	result := make(map[string]*types.PullRequestUser)
	for _, u := range usernames {
		user, err := s.GetUserByUsername(ctx, orgData, u)
		if err == nil && user != nil {
			result[u] = user
		}
	}
	return result, nil
}

func (s *ForgejoService) GetUserByEmailOrName(ctx context.Context, orgData types.OrganizationAndTeamData, email, userName string) (*types.PullRequestUser, error) {
	query := userName
	if query == "" {
		query = email
	}
	if query == "" {
		return nil, errors.New("email or username required")
	}

	baseURL, token := s.extractCredentials(orgData)
	endpoint := fmt.Sprintf("/users/search?q=%s&limit=5", url.QueryEscape(query))
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return nil, err
	}

	var searchResp struct {
		Data []struct {
			ID       int64  `json:"id"`
			Login    string `json:"login"`
			FullName string `json:"full_name"`
			Email    string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &searchResp); err == nil && len(searchResp.Data) > 0 {
		first := searchResp.Data[0]
		name := first.FullName
		if name == "" {
			name = first.Login
		}
		return &types.PullRequestUser{
			ID:    strconv.FormatInt(first.ID, 10),
			Login: first.Login,
			Name:  name,
			Email: first.Email,
		}, nil
	}

	return nil, nil
}

func (s *ForgejoService) GetUserByID(ctx context.Context, orgData types.OrganizationAndTeamData, userID string) (*types.PullRequestUser, error) {
	baseURL, token := s.extractCredentials(orgData)
	endpoint := fmt.Sprintf("/users/%s", userID)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return nil, err
	}

	var raw struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, err
	}

	name := raw.FullName
	if name == "" {
		name = raw.Login
	}

	return &types.PullRequestUser{
		ID:    strconv.FormatInt(raw.ID, 10),
		Login: raw.Login,
		Name:  name,
		Email: raw.Email,
	}, nil
}

func (s *ForgejoService) GetPullRequestAuthors(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	determineBots bool,
) ([]types.PullRequestAuthor, error) {
	members, err := s.GetListMembers(ctx, orgData)
	if err != nil {
		return []types.PullRequestAuthor{}, err
	}
	return members, nil
}

func (s *ForgejoService) GetAuthenticationOAuthToken(ctx context.Context, orgData types.OrganizationAndTeamData) (string, error) {
	_, token := s.extractCredentials(orgData)
	if token == "" {
		return "", errors.New("no auth token configured")
	}
	return token, nil
}

// -------------------------------------------------------------------------------------
// Reactions
// -------------------------------------------------------------------------------------

func (s *ForgejoService) CountReactions(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo *types.RepositoryDescriptor,
	prNumber int,
) ([]types.ReactionsInComments, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(repo)
	if owner == "" || repoName == "" {
		return []types.ReactionsInComments{}, nil
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return []types.ReactionsInComments{}, nil
	}

	var rawReactions []struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(respBytes, &rawReactions)

	counts := make(map[string]int)
	for _, r := range rawReactions {
		if r.Content != "" {
			counts[r.Content]++
		}
	}

	var result []types.ReactionsInComments
	for r, count := range counts {
		result = append(result, types.ReactionsInComments{
			Reaction: r,
			Count:    count,
		})
	}
	return result, nil
}

func (s *ForgejoService) AddReactionToPR(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	reaction string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	payload := map[string]string{"content": reaction}
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)
	_, _, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	return err
}

func (s *ForgejoService) AddReactionToComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	commentID int64,
	reaction string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	payload := map[string]string{"content": reaction}
	endpoint := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repoName, commentID)
	_, _, err := s.doRequest(ctx, baseURL, token, http.MethodPost, endpoint, payload)
	return err
}

func (s *ForgejoService) RemoveReactionsFromPR(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	reactions []string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	for _, reaction := range reactions {
		payload := map[string]string{"content": reaction}
		endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", owner, repoName, prNumber)
		_, _, _ = s.doRequest(ctx, baseURL, token, http.MethodDelete, endpoint, payload)
	}
	return nil
}

func (s *ForgejoService) RemoveReactionsFromComment(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	commentID int64,
	reactions []string,
) error {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := s.parseOwnerAndRepo(&repo)
	if owner == "" || repoName == "" {
		return errors.New("repository descriptor required")
	}

	for _, reaction := range reactions {
		payload := map[string]string{"content": reaction}
		endpoint := fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", owner, repoName, commentID)
		_, _, _ = s.doRequest(ctx, baseURL, token, http.MethodDelete, endpoint, payload)
	}
	return nil
}

// -------------------------------------------------------------------------------------
// Webhooks
// -------------------------------------------------------------------------------------

func (s *ForgejoService) DeleteWebhook(ctx context.Context, orgData types.OrganizationAndTeamData) error {
	baseURL, token := s.extractCredentials(orgData)
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return err
	}

	webhookURL := "https://scandrix.dev/api/webhooks/forgejo"

	for _, repo := range repos {
		owner := repo.OrganizationName
		repoName := repo.Name
		endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repoName)
		respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
		if err != nil || statusCode != http.StatusOK {
			continue
		}

		var hooks []struct {
			ID     int64 `json:"id"`
			Config struct {
				URL string `json:"url"`
			} `json:"config"`
		}
		if err := json.Unmarshal(respBytes, &hooks); err == nil {
			for _, h := range hooks {
				if strings.Contains(h.Config.URL, "scandrix") || h.Config.URL == webhookURL {
					delEndpoint := fmt.Sprintf("/repos/%s/%s/hooks/%d", owner, repoName, h.ID)
					_, _, _ = s.doRequest(ctx, baseURL, token, http.MethodDelete, delEndpoint, nil)
				}
			}
		}
	}
	return nil
}

func (s *ForgejoService) IsWebhookActive(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) (bool, error) {
	baseURL, token := s.extractCredentials(orgData)
	repos, err := s.GetRepositories(ctx, orgData, nil, "", "")
	if err != nil {
		return false, err
	}

	for _, repo := range repos {
		if repo.ID != repositoryID && repo.Name != repositoryID {
			continue
		}
		owner := repo.OrganizationName
		repoName := repo.Name
		endpoint := fmt.Sprintf("/repos/%s/%s/hooks", owner, repoName)
		respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
		if err != nil || statusCode != http.StatusOK {
			continue
		}

		var hooks []struct {
			Active bool `json:"active"`
			Config struct {
				URL string `json:"url"`
			} `json:"config"`
		}
		if err := json.Unmarshal(respBytes, &hooks); err == nil {
			for _, h := range hooks {
				if strings.Contains(h.Config.URL, "scandrix") && h.Active {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// -------------------------------------------------------------------------------------
// Tree Inspection & Formatting
// -------------------------------------------------------------------------------------

func (s *ForgejoService) GetRepositoryTree(ctx context.Context, orgData types.OrganizationAndTeamData, repositoryID string) ([]*types.TreeItem, error) {
	return s.GetRepositoryTreeByDirectory(ctx, orgData, repositoryID, "")
}

func (s *ForgejoService) GetRepositoryTreeByDirectory(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repositoryID, directoryPath string,
) ([]*types.TreeItem, error) {
	baseURL, token := s.extractCredentials(orgData)
	owner, repoName := "", repositoryID
	if parts := strings.Split(repositoryID, "/"); len(parts) >= 2 {
		owner = parts[0]
		repoName = parts[1]
	}

	cleanDir := strings.Trim(directoryPath, "/")
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repoName, cleanDir)
	respBytes, statusCode, err := s.doRequest(ctx, baseURL, token, http.MethodGet, endpoint, nil)
	if err != nil || statusCode != http.StatusOK {
		return []*types.TreeItem{}, err
	}

	var entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(respBytes, &entries); err != nil {
		return []*types.TreeItem{}, err
	}

	var items []*types.TreeItem
	for _, e := range entries {
		itemType := "blob"
		if e.Type == "dir" {
			itemType = "tree"
		}
		items = append(items, &types.TreeItem{
			Path: e.Path,
			Mode: "100644",
			Type: itemType,
			Size: e.Size,
		})
	}
	return items, nil
}

// -------------------------------------------------------------------------------------
// Review Comment Formatting
// -------------------------------------------------------------------------------------

func (s *ForgejoService) FormatReviewCommentBody(suggestion any, repoLanguage string, includeHeader, includeFooter bool) string {
	var sb strings.Builder

	if includeHeader {
		sb.WriteString(defaultReviewCommentMarker)
		sb.WriteString("\n### 🛡️ ScanDrix AI Review Suggestion\n\n")
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

func (s *ForgejoService) ResolveMrAuthorFromWebhookPayload(ctx context.Context, orgData types.OrganizationAndTeamData, payload any) (*types.PullRequestUser, error) {
	if m, ok := payload.(map[string]any); ok {
		if pr, ok := m["pull_request"].(map[string]any); ok {
			if u, ok := pr["user"].(map[string]any); ok {
				var idStr string
				if id, ok := u["id"].(float64); ok {
					idStr = strconv.FormatInt(int64(id), 10)
				}
				login, _ := u["login"].(string)
				fullName, _ := u["full_name"].(string)
				avatar, _ := u["avatar_url"].(string)
				return &types.PullRequestUser{
					ID:        idStr,
					Login:     login,
					Username:  login,
					Name:      fullName,
					AvatarURL: avatar,
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *ForgejoService) GetRecentRepositoryComments(ctx context.Context, orgData types.OrganizationAndTeamData, repo types.RepositoryDescriptor, limit int) ([]*types.PullRequestReviewComment, error) {
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

