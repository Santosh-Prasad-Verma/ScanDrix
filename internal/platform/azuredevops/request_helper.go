package azuredevops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AzureReposRequestHelper handles low-level REST 7.0/7.1 communications with Azure DevOps Services and Azure DevOps Server.
// Translates libs/platform/infrastructure/adapters/services/azureRepos/azure-repos-request-helper.ts
type AzureReposRequestHelper struct {
	httpClient *http.Client
	defaultTimeout time.Duration
}

// NewAzureReposRequestHelper constructs an AzureReposRequestHelper.
func NewAzureReposRequestHelper(httpClient *http.Client, timeout time.Duration) *AzureReposRequestHelper {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 45 * time.Second,
		}
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &AzureReposRequestHelper{
		httpClient:     httpClient,
		defaultTimeout: timeout,
	}
}

// -------------------------------------------------------------------------------------
// Azure DevOps Data Types
// -------------------------------------------------------------------------------------

type AzureProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	State       string `json:"state,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

type AzureRepository struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	URL           string       `json:"url"`
	RemoteURL     string       `json:"remoteUrl"`
	DefaultBranch string       `json:"defaultBranch"`
	Size          int64        `json:"size"`
	Project       AzureProject `json:"project"`
	WebURL        string       `json:"webUrl"`
	IsFork        bool         `json:"isFork"`
	IsDisabled    bool         `json:"isDisabled"`
}

type AzureIdentityRef struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	URL         string `json:"url,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

type AzureReviewer struct {
	AzureIdentityRef
	Vote            int    `json:"vote"` // 10=approved, 5=approved with suggestions, 0=no vote, -5=waiting, -10=rejected
	HasDeclined     bool   `json:"hasDeclined"`
	IsRequired      bool   `json:"isRequired"`
	IsFlagged       bool   `json:"isFlagged"`
	ReviewerURL     string `json:"reviewerUrl,omitempty"`
}

type AzureGitRef struct {
	Name     string `json:"name"`
	ObjectID string `json:"objectId"`
}

type AzurePullRequestModel struct {
	PullRequestID int               `json:"pullRequestId"`
	CodeReviewID  int               `json:"codeReviewId"`
	Status        string            `json:"status"` // "active", "completed", "abandoned"
	CreatedBy     AzureIdentityRef  `json:"createdBy"`
	CreationDate  time.Time         `json:"creationDate"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	SourceRefName string            `json:"sourceRefName"`
	TargetRefName string            `json:"targetRefName"`
	MergeStatus   string            `json:"mergeStatus"`
	MergeID       string            `json:"mergeId"`
	LastMergeSourceCommit *AzureGitRef `json:"lastMergeSourceCommit,omitempty"`
	LastMergeTargetCommit *AzureGitRef `json:"lastMergeTargetCommit,omitempty"`
	LastMergeCommit       *AzureGitRef `json:"lastMergeCommit,omitempty"`
	Reviewers     []AzureReviewer   `json:"reviewers"`
	URL           string            `json:"url"`
	IsDraft       bool              `json:"isDraft"`
	Repository    AzureRepository   `json:"repository"`
}

type AzureCommentPosition struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

type AzureThreadContext struct {
	FilePath       string                 `json:"filePath"`
	RightFileStart *AzureCommentPosition `json:"rightFileStart,omitempty"`
	RightFileEnd   *AzureCommentPosition `json:"rightFileEnd,omitempty"`
	LeftFileStart  *AzureCommentPosition `json:"leftFileStart,omitempty"`
	LeftFileEnd    *AzureCommentPosition `json:"leftFileEnd,omitempty"`
}

type AzureCommentModel struct {
	ID              int              `json:"id"`
	ParentCommentID int              `json:"parentCommentId"`
	Author          AzureIdentityRef `json:"author"`
	Content         string           `json:"content"`
	PublishedDate   time.Time        `json:"publishedDate"`
	LastUpdatedDate time.Time        `json:"lastUpdatedDate"`
	LastContentUpdatedDate time.Time `json:"lastContentUpdatedDate"`
	CommentType     int              `json:"commentType"` // 1=text, 2=codeChange, 3=system
	IsDeleted       bool             `json:"isDeleted"`
}

type AzureThreadModel struct {
	ID            int                  `json:"id"`
	PublishedDate time.Time            `json:"publishedDate"`
	LastUpdatedDate time.Time          `json:"lastUpdatedDate"`
	Comments      []AzureCommentModel  `json:"comments"`
	Status        string               `json:"status"` // "active", "fixed", "wontfix", "closed", "bydesign", "pending"
	ThreadContext *AzureThreadContext  `json:"threadContext,omitempty"`
	IsDeleted     bool                 `json:"isDeleted"`
}

type AzureCommitModel struct {
	CommitID string `json:"commitId"`
	Author   struct {
		Name  string    `json:"name"`
		Email string    `json:"email"`
		Date  time.Time `json:"date"`
	} `json:"author"`
	Committer struct {
		Name  string    `json:"name"`
		Email string    `json:"email"`
		Date  time.Time `json:"date"`
	} `json:"committer"`
	Comment string `json:"comment"`
	URL     string `json:"url"`
}

type AzureItemModel struct {
	ObjectID      string `json:"objectId"`
	GitObjectType string `json:"gitObjectType"`
	CommitID      string `json:"commitId"`
	Path          string `json:"path"`
	IsFolder      bool   `json:"isFolder"`
	Content       string `json:"content,omitempty"`
}

type AzureIterationModel struct {
	ID           int       `json:"id"`
	Description  string    `json:"description"`
	CreatedDate  time.Time `json:"createdDate"`
	SourceRefCommit struct {
		CommitID string `json:"commitId"`
	} `json:"sourceRefCommit"`
	TargetRefCommit struct {
		CommitID string `json:"commitId"`
	} `json:"targetRefCommit"`
}

type AzureIterationChangeModel struct {
	ChangeID int `json:"changeId"`
	Item     struct {
		ObjectID string `json:"objectId"`
		Path     string `json:"path"`
	} `json:"item"`
	ChangeType string `json:"changeType"` // "add", "edit", "delete"
}

// -------------------------------------------------------------------------------------
// HTTP Helpers
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) buildBaseURL(orgName string) string {
	clean := strings.TrimRight(orgName, "/")
	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
		return clean
	}
	return "https://dev.azure.com/" + clean
}

func (h *AzureReposRequestHelper) doRequest(ctx context.Context, orgName, token, method, apiPath string, body any) ([]byte, int, error) {
	baseURL := h.buildBaseURL(orgName)
	reqURL := apiPath
	if !strings.HasPrefix(reqURL, "http://") && !strings.HasPrefix(reqURL, "https://") {
		reqURL = baseURL + "/" + strings.TrimLeft(apiPath, "/")
	}

	// Ensure api-version parameter is present
	if !strings.Contains(reqURL, "api-version=") {
		if strings.Contains(reqURL, "?") {
			reqURL += "&api-version=7.1"
		} else {
			reqURL += "?api-version=7.1"
		}
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal azure request failed: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create azure request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Azure DevOps PAT authentication: Basic ":" + token
	if token != "" {
		basicAuth := base64.StdEncoding.EncodeToString([]byte(":" + token))
		req.Header.Set("Authorization", "Basic "+basicAuth)
	}

	var resp *http.Response
	maxRetries := 3
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = h.httpClient.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, 0, fmt.Errorf("azure devops request failed: %w", err)
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, resp.StatusCode, fmt.Errorf("azure devops rate limit/unavailable (%d)", resp.StatusCode)
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
		return nil, resp.StatusCode, fmt.Errorf("read azure response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return respBytes, resp.StatusCode, fmt.Errorf("azure devops api status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, resp.StatusCode, nil
}

// -------------------------------------------------------------------------------------
// Projects & Repositories
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) GetProjects(ctx context.Context, orgName, token string) ([]AzureProject, error) {
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, "/_apis/projects", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureProject `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure projects failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) GetRepositories(ctx context.Context, orgName, token, projectID string) ([]AzureRepository, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories", url.PathEscape(projectID))
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureRepository `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure repositories failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) GetRepository(ctx context.Context, orgName, token, projectID, repoID string) (*AzureRepository, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s", url.PathEscape(projectID), url.PathEscape(repoID))
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var repo AzureRepository
	if err := json.Unmarshal(data, &repo); err != nil {
		return nil, fmt.Errorf("decode azure repo failed: %w", err)
	}

	return &repo, nil
}

func (h *AzureReposRequestHelper) GetDefaultBranch(ctx context.Context, orgName, token, projectID, repoID string) (string, error) {
	repo, err := h.GetRepository(ctx, orgName, token, projectID, repoID)
	if err != nil {
		return "main", err
	}
	branch := strings.TrimPrefix(repo.DefaultBranch, "refs/heads/")
	if branch == "" {
		branch = "main"
	}
	return branch, nil
}

// -------------------------------------------------------------------------------------
// Pull Requests
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) GetPullRequestsByRepo(ctx context.Context, orgName, token, projectID, repoID, status, author, branch string) ([]AzurePullRequestModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests", url.PathEscape(projectID), url.PathEscape(repoID))
	q := url.Values{}
	if status != "" {
		q.Set("searchCriteria.status", status)
	}
	if author != "" {
		q.Set("searchCriteria.creatorId", author)
	}
	if branch != "" {
		if !strings.HasPrefix(branch, "refs/heads/") {
			branch = "refs/heads/" + branch
		}
		q.Set("searchCriteria.sourceRefName", branch)
	}

	if len(q) > 0 {
		apiPath += "?" + q.Encode()
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzurePullRequestModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure pull requests failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) GetPullRequestDetails(ctx context.Context, orgName, token, projectID, repoID string, prID int) (*AzurePullRequestModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%d", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var pr AzurePullRequestModel
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("decode azure pr details failed: %w", err)
	}

	return &pr, nil
}

func (h *AzureReposRequestHelper) CreatePullRequest(ctx context.Context, orgName, token, projectID, repoID, sourceBranch, targetBranch, title, description string) (*AzurePullRequestModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests", url.PathEscape(projectID), url.PathEscape(repoID))
	if !strings.HasPrefix(sourceBranch, "refs/heads/") {
		sourceBranch = "refs/heads/" + sourceBranch
	}
	if !strings.HasPrefix(targetBranch, "refs/heads/") {
		targetBranch = "refs/heads/" + targetBranch
	}

	body := map[string]any{
		"sourceRefName": sourceBranch,
		"targetRefName": targetBranch,
		"title":         title,
		"description":   description,
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, body)
	if err != nil {
		return nil, err
	}

	var pr AzurePullRequestModel
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("decode created azure pr failed: %w", err)
	}

	return &pr, nil
}

func (h *AzureReposRequestHelper) CompletePullRequest(ctx context.Context, orgName, token, projectID, repoID string, prID int, lastMergeSourceCommit string) (*AzurePullRequestModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%d", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	body := map[string]any{
		"status": "completed",
		"lastMergeSourceCommit": map[string]string{
			"commitId": lastMergeSourceCommit,
		},
		"completionOptions": map[string]any{
			"deleteSourceBranch": true,
			"mergeStrategy":      1, // NoFastForward
		},
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPatch, apiPath, body)
	if err != nil {
		return nil, err
	}

	var pr AzurePullRequestModel
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

func (h *AzureReposRequestHelper) UpdatePullRequestDescription(ctx context.Context, orgName, token, projectID, repoID string, prID int, description string) error {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullrequests/%d", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	body := map[string]any{
		"description": description,
	}

	_, _, err := h.doRequest(ctx, orgName, token, http.MethodPatch, apiPath, body)
	return err
}

// -------------------------------------------------------------------------------------
// Threads & Comments (Inline & General)
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) GetPullRequestComments(ctx context.Context, orgName, token, projectID, repoID string, prID int) ([]AzureThreadModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureThreadModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure pr threads failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) CreateReviewComment(ctx context.Context, orgName, token, projectID, repoID string, prID int, filePath string, startLine, line int, content string) (*AzureThreadModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	if line <= 0 {
		line = 1
	}
	if startLine <= 0 {
		startLine = line
	}

	// Normalise path format for Azure Repos
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	payload := map[string]any{
		"comments": []map[string]any{
			{
				"parentCommentId": 0,
				"content":         content,
				"commentType":     1, // Text
			},
		},
		"status": "active",
		"threadContext": map[string]any{
			"filePath": filePath,
			"rightFileStart": map[string]int{
				"line":   startLine,
				"offset": 1,
			},
			"rightFileEnd": map[string]int{
				"line":   line,
				"offset": 1,
			},
		},
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, payload)
	if err != nil {
		return nil, err
	}

	var thread AzureThreadModel
	if err := json.Unmarshal(data, &thread); err != nil {
		return nil, fmt.Errorf("decode created azure thread failed: %w", err)
	}

	return &thread, nil
}

func (h *AzureReposRequestHelper) CreateGeneralThread(ctx context.Context, orgName, token, projectID, repoID string, prID int, content string) (*AzureThreadModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	payload := map[string]any{
		"comments": []map[string]any{
			{
				"parentCommentId": 0,
				"content":         content,
				"commentType":     1,
			},
		},
		"status": "active",
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, payload)
	if err != nil {
		return nil, err
	}

	var thread AzureThreadModel
	if err := json.Unmarshal(data, &thread); err != nil {
		return nil, fmt.Errorf("decode general thread failed: %w", err)
	}

	return &thread, nil
}

func (h *AzureReposRequestHelper) AddCommentToThread(ctx context.Context, orgName, token, projectID, repoID string, prID, threadID, parentCommentID int, content string) (*AzureCommentModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments", url.PathEscape(projectID), url.PathEscape(repoID), prID, threadID)
	payload := map[string]any{
		"parentCommentId": parentCommentID,
		"content":         content,
		"commentType":     1,
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, payload)
	if err != nil {
		return nil, err
	}

	var comment AzureCommentModel
	if err := json.Unmarshal(data, &comment); err != nil {
		return nil, fmt.Errorf("decode added comment failed: %w", err)
	}

	return &comment, nil
}

func (h *AzureReposRequestHelper) UpdateCommentInThread(ctx context.Context, orgName, token, projectID, repoID string, prID, threadID, commentID int, content string) error {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments/%d", url.PathEscape(projectID), url.PathEscape(repoID), prID, threadID, commentID)
	payload := map[string]any{
		"content": content,
	}

	_, _, err := h.doRequest(ctx, orgName, token, http.MethodPatch, apiPath, payload)
	return err
}

func (h *AzureReposRequestHelper) UpdateThreadStatus(ctx context.Context, orgName, token, projectID, repoID string, prID, threadID int, status string) error {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d", url.PathEscape(projectID), url.PathEscape(repoID), prID, threadID)
	payload := map[string]any{
		"status": status, // "fixed", "closed", "active"
	}

	_, _, err := h.doRequest(ctx, orgName, token, http.MethodPatch, apiPath, payload)
	return err
}

// -------------------------------------------------------------------------------------
// Reviews & Votes
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) VotePullRequest(ctx context.Context, orgName, token, projectID, repoID string, prID int, reviewerID string, vote int) error {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers/%s", url.PathEscape(projectID), url.PathEscape(repoID), prID, url.PathEscape(reviewerID))
	payload := map[string]any{
		"vote": vote,
	}

	_, _, err := h.doRequest(ctx, orgName, token, http.MethodPut, apiPath, payload)
	return err
}

// -------------------------------------------------------------------------------------
// Iterations, Diff & Commits
// -------------------------------------------------------------------------------------

func (h *AzureReposRequestHelper) GetPullRequestIterations(ctx context.Context, orgName, token, projectID, repoID string, prID int) ([]AzureIterationModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/iterations", url.PathEscape(projectID), url.PathEscape(repoID), prID)
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureIterationModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure iterations failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) GetPullRequestIterationChanges(ctx context.Context, orgName, token, projectID, repoID string, prID, iterationID int) ([]AzureIterationChangeModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/iterations/%d/changes", url.PathEscape(projectID), url.PathEscape(repoID), prID, iterationID)
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		ChangeEntries []AzureIterationChangeModel `json:"changeEntries"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure iteration changes failed: %w", err)
	}

	return res.ChangeEntries, nil
}

func (h *AzureReposRequestHelper) GetCommits(ctx context.Context, orgName, token, projectID, repoID, branch string) ([]AzureCommitModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/commits", url.PathEscape(projectID), url.PathEscape(repoID))
	if branch != "" {
		apiPath += "?searchCriteria.itemVersion.version=" + url.QueryEscape(branch)
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureCommitModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode azure commits failed: %w", err)
	}

	return res.Value, nil
}

func (h *AzureReposRequestHelper) GetFileContent(ctx context.Context, orgName, token, projectID, repoID, filePath, ref string) (string, error) {
	cleanPath := filePath
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/items?path=%s&includeContent=true", url.PathEscape(projectID), url.PathEscape(repoID), url.QueryEscape(cleanPath))
	if ref != "" {
		apiPath += "&versionDescriptor.version=" + url.QueryEscape(ref)
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return "", err
	}

	var res AzureItemModel
	if err := json.Unmarshal(data, &res); err == nil && res.Content != "" {
		return res.Content, nil
	}

	return string(data), nil
}

func (h *AzureReposRequestHelper) GetBatchItems(ctx context.Context, orgName, token, projectID, repoID string, paths []string, ref string) (map[string]string, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/itemsbatch", url.PathEscape(projectID), url.PathEscape(repoID))
	itemDescriptors := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		clean := p
		if !strings.HasPrefix(clean, "/") {
			clean = "/" + clean
		}
		desc := map[string]any{
			"path": clean,
		}
		if ref != "" {
			desc["version"] = ref
		}
		itemDescriptors = append(itemDescriptors, desc)
	}

	body := map[string]any{
		"itemDescriptors": itemDescriptors,
		"includeContent":  true,
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, body)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value [][]AzureItemModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, group := range res.Value {
		for _, item := range group {
			result[strings.TrimPrefix(item.Path, "/")] = item.Content
		}
	}

	return result, nil
}

// -------------------------------------------------------------------------------------
// Organization Users (VSSPS API)
// -------------------------------------------------------------------------------------

// AzureUserEntitlement represents a user entitlement from VSSPS.
type AzureUserEntitlement struct {
	ID        string `json:"id"`
	User      struct {
		DisplayName   string `json:"displayName"`
		PrincipalName string `json:"principalName"`
		MailAddress   string `json:"mailAddress"`
		OriginID      string `json:"originId"`
		Origin        string `json:"origin"`
		Descriptor    string `json:"descriptor"`
	} `json:"user"`
}

// ListOrganizationUsers lists all users in the organization via VSSPS Member Entitlements API.
func (h *AzureReposRequestHelper) ListOrganizationUsers(ctx context.Context, orgName, token string) ([]AzureUserEntitlement, error) {
	// VSSPS uses a different base URL
	vsspsURL := fmt.Sprintf("https://vsaex.dev.azure.com/%s/_apis/userentitlements?api-version=7.1-preview.4", url.PathEscape(orgName))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vsspsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+token)))
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("VSSPS ListOrganizationUsers failed (HTTP %d): %s", resp.StatusCode, string(data))
	}

	var result struct {
		Members []AzureUserEntitlement `json:"members"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result.Members, nil
}

// -------------------------------------------------------------------------------------
// Service Hook Subscriptions (Webhook management)
// -------------------------------------------------------------------------------------

// AzureServiceHookSubscription represents an Azure DevOps service hook subscription.
type AzureServiceHookSubscription struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	PublisherInputs map[string]string `json:"publisherInputs"`
	ConsumerInputs  map[string]string `json:"consumerInputs"`
	EventType       string `json:"eventType"`
}

// ListServiceHookSubscriptions lists all service hook subscriptions for a project.
func (h *AzureReposRequestHelper) ListServiceHookSubscriptions(ctx context.Context, orgName, token, projectID string) ([]AzureServiceHookSubscription, error) {
	apiPath := fmt.Sprintf("/_apis/hooks/subscriptions?publisherId=tfs&publisherInputFilters[0].conditions[0].inputId=projectId&publisherInputFilters[0].conditions[0].inputValue=%s", url.QueryEscape(projectID))

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureServiceHookSubscription `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	return res.Value, nil
}

// DeleteServiceHookSubscription deletes a specific service hook subscription.
func (h *AzureReposRequestHelper) DeleteServiceHookSubscription(ctx context.Context, orgName, token, subscriptionID string) error {
	apiPath := fmt.Sprintf("/_apis/hooks/subscriptions/%s", url.PathEscape(subscriptionID))
	_, _, err := h.doRequest(ctx, orgName, token, http.MethodDelete, apiPath, nil)
	return err
}

// -------------------------------------------------------------------------------------
// Authenticated User Identity
// -------------------------------------------------------------------------------------

// GetAuthenticatedUserID returns the ID of the authenticated user (via the connection data endpoint).
func (h *AzureReposRequestHelper) GetAuthenticatedUserID(ctx context.Context, orgName, token string) (string, error) {
	apiPath := "/_apis/connectionData"
	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return "", err
	}

	var result struct {
		AuthenticatedUser struct {
			ID string `json:"id"`
		} `json:"authenticatedUser"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	return result.AuthenticatedUser.ID, nil
}

// -------------------------------------------------------------------------------------
// Pull Request Reviewers
// -------------------------------------------------------------------------------------

// GetPullRequestReviewers lists all reviewers for a pull request.
func (h *AzureReposRequestHelper) GetPullRequestReviewers(ctx context.Context, orgName, token, projectID, repoID string, prID int) ([]AzureReviewer, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers",
		url.PathEscape(projectID), url.PathEscape(repoID), prID)

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureReviewer `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	return res.Value, nil
}

// -------------------------------------------------------------------------------------
// Repository Items (Tree)
// -------------------------------------------------------------------------------------

// GetItems lists repository items (files and folders) with optional recursion.
func (h *AzureReposRequestHelper) GetItems(ctx context.Context, orgName, token, projectID, repoID string, scopePath string, recursionLevel string, ref string) ([]AzureItemModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/items?recursionLevel=%s&includeContentMetadata=true",
		url.PathEscape(projectID), url.PathEscape(repoID), url.QueryEscape(recursionLevel))

	if scopePath != "" {
		apiPath += "&scopePath=" + url.QueryEscape(scopePath)
	}
	if ref != "" {
		apiPath += "&versionDescriptor.version=" + url.QueryEscape(ref) + "&versionDescriptor.versionType=branch"
	}

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureItemModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	return res.Value, nil
}

// -------------------------------------------------------------------------------------
// Language Statistics
// -------------------------------------------------------------------------------------

// AzureLanguageBreakdown represents a single language entry from project language stats.
type AzureLanguageBreakdown struct {
	Name               string  `json:"name"`
	LanguagePercentage float64 `json:"languagePercentage"`
}

// GetLanguageStats retrieves repository language breakdown via the project analysis API.
func (h *AzureReposRequestHelper) GetLanguageStats(ctx context.Context, orgName, token, projectID string) ([]AzureLanguageBreakdown, error) {
	apiPath := fmt.Sprintf("/%s/_apis/projectanalysis/languagemetrics", url.PathEscape(projectID))

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		LanguageBreakdown []AzureLanguageBreakdown `json:"languageBreakdown"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	return res.LanguageBreakdown, nil
}

// -------------------------------------------------------------------------------------
// Pull Request Commits
// -------------------------------------------------------------------------------------

// GetPullRequestCommits retrieves all commits associated with a pull request.
func (h *AzureReposRequestHelper) GetPullRequestCommits(ctx context.Context, orgName, token, projectID, repoID string, prID int) ([]AzureCommitModel, error) {
	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pullRequests/%d/commits",
		url.PathEscape(projectID), url.PathEscape(repoID), prID)

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Value []AzureCommitModel `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}

	return res.Value, nil
}

// -------------------------------------------------------------------------------------
// Branch Existence Check
// -------------------------------------------------------------------------------------

// BranchExists checks if a branch exists in a repository.
func (h *AzureReposRequestHelper) BranchExists(ctx context.Context, orgName, token, projectID, repoID, branchName string) (bool, error) {
	refName := branchName
	if !strings.HasPrefix(refName, "refs/heads/") {
		refName = "refs/heads/" + refName
	}

	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/refs?filter=%s",
		url.PathEscape(projectID), url.PathEscape(repoID), url.QueryEscape(refName))

	data, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, apiPath, nil)
	if err != nil {
		return false, err
	}

	var res struct {
		Value []AzureGitRef `json:"value"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return false, err
	}

	return len(res.Value) > 0, nil
}

// -------------------------------------------------------------------------------------
// Upload Files to Branch (Push API)
// -------------------------------------------------------------------------------------

// AzurePushChange represents a single file change for a push operation.
type AzurePushChange struct {
	ChangeType string `json:"changeType"` // "add", "edit", "delete"
	FilePath   string `json:"filePath"`
	Content    string `json:"content,omitempty"`
}

// UploadFilesToBranch pushes file changes to a branch via the Git Push API.
func (h *AzureReposRequestHelper) UploadFilesToBranch(ctx context.Context, orgName, token, projectID, repoID, branchName, baseBranch, commitMessage string, author *struct{ Name, Email string }, changes []AzurePushChange) error {
	// Resolve the old object ID from the base branch
	oldObjectID := "0000000000000000000000000000000000000000" // initial push
	baseRefName := baseBranch
	if !strings.HasPrefix(baseRefName, "refs/heads/") {
		baseRefName = "refs/heads/" + baseRefName
	}

	targetRefName := branchName
	if !strings.HasPrefix(targetRefName, "refs/heads/") {
		targetRefName = "refs/heads/" + targetRefName
	}

	// Try to get the base branch ref
	refsPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/refs?filter=%s",
		url.PathEscape(projectID), url.PathEscape(repoID), url.QueryEscape(baseRefName))
	refData, _, err := h.doRequest(ctx, orgName, token, http.MethodGet, refsPath, nil)
	if err == nil {
		var refs struct {
			Value []struct {
				ObjectID string `json:"objectId"`
			} `json:"value"`
		}
		if json.Unmarshal(refData, &refs) == nil && len(refs.Value) > 0 {
			oldObjectID = refs.Value[0].ObjectID
		}
	}

	// Build the push changes
	pushChanges := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		change := map[string]any{
			"changeType": c.ChangeType,
			"item":       map[string]string{"path": c.FilePath},
		}
		if c.ChangeType != "delete" && c.Content != "" {
			change["newContent"] = map[string]any{
				"content":     c.Content,
				"contentType": "rawtext",
			}
		}
		pushChanges = append(pushChanges, change)
	}

	commit := map[string]any{
		"comment": commitMessage,
		"changes": pushChanges,
	}
	if author != nil {
		commit["author"] = map[string]string{
			"name":  author.Name,
			"email": author.Email,
		}
	}

	pushBody := map[string]any{
		"refUpdates": []map[string]string{
			{
				"name":        targetRefName,
				"oldObjectId": oldObjectID,
			},
		},
		"commits": []any{commit},
	}

	apiPath := fmt.Sprintf("/%s/_apis/git/repositories/%s/pushes",
		url.PathEscape(projectID), url.PathEscape(repoID))

	_, _, err = h.doRequest(ctx, orgName, token, http.MethodPost, apiPath, pushBody)
	return err
}
