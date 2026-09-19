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

// AzureDevOpsRepo represents a Git repository in Azure DevOps.
type AzureDevOpsRepo struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	URL              string                 `json:"url"`
	State            string                 `json:"state,omitempty"`
	DefaultBranch    string                 `json:"defaultBranch,omitempty"`
	Size             int64                  `json:"size,omitempty"`
	RemoteURL        string                 `json:"remoteUrl,omitempty"`
	SSHURL           string                 `json:"sshUrl,omitempty"`
	WebURL           string                 `json:"webUrl,omitempty"`
	IsFork           bool                   `json:"isFork,omitempty"`
	ParentRepository *AzureDevOpsRepoRef    `json:"parentRepository,omitempty"`
	Project          *AzureDevOpsProjectRef `json:"project,omitempty"`
}

// AzureDevOpsRepoRef provides a minimal reference to a repository.
type AzureDevOpsRepoRef struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	URL     string                 `json:"url,omitempty"`
	Project *AzureDevOpsProjectRef `json:"project,omitempty"`
}

// AzureDevOpsProjectRef provides a project reference.
type AzureDevOpsProjectRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	State       string `json:"state,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

// CreateAzureRepoRequest holds parameters to create a new repository or fork.
type CreateAzureRepoRequest struct {
	Name             string                 `json:"name"`
	Project          *AzureDevOpsProjectRef `json:"project"`
	ParentRepository *AzureDevOpsRepoRef    `json:"parentRepository,omitempty"`
}

// AzureGitRefDetails represents extended Git reference details (branch or tag).
type AzureGitRefDetails struct {
	Name        string                  `json:"name"`
	ObjectID    string                  `json:"objectId"`
	Creator     *AzureDevOpsIdentityRef `json:"creator,omitempty"`
	URL         string                  `json:"url,omitempty"`
	IsLocked    bool                    `json:"isLocked,omitempty"`
	IsLockedBy  *AzureDevOpsIdentityRef `json:"isLockedBy,omitempty"`
	Statuses    []AzureGitRefStatus     `json:"statuses,omitempty"`
}

// AzureDevOpsIdentityRef represents a user identity in Azure DevOps.
type AzureDevOpsIdentityRef struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName,omitempty"`
	URL         string `json:"url,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// AzureGitRefStatus represents status check metadata on a ref.
type AzureGitRefStatus struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"targetUrl,omitempty"`
}

// AzureGitRefUpdate represents an update payload for a Git reference.
type AzureGitRefUpdate struct {
	Name        string `json:"name"`
	OldObjectID string `json:"oldObjectId"`
	NewObjectID string `json:"newObjectId"`
	IsLocked    bool   `json:"isLocked,omitempty"`
}

// AzureGitRefUpdateResult holds the outcome of a ref update.
type AzureGitRefUpdateResult struct {
	Name         string `json:"name"`
	OldObjectID  string `json:"oldObjectId"`
	NewObjectID  string `json:"newObjectId"`
	Success      bool   `json:"success"`
	UpdateStatus string `json:"updateStatus"`
}

// AzureGitItem represents a file or folder in a Git repository.
type AzureGitItem struct {
	ObjectID        string                       `json:"objectId"`
	GitObjectType   string                       `json:"gitObjectType"` // "blob", "tree"
	CommitID        string                       `json:"commitId"`
	Path            string                       `json:"path"`
	IsFolder        bool                         `json:"isFolder"`
	URL             string                       `json:"url,omitempty"`
	Content         string                       `json:"content,omitempty"`
	ContentMetadata *AzureGitItemContentMetadata `json:"contentMetadata,omitempty"`
}

// AzureGitItemContentMetadata holds metadata about file content.
type AzureGitItemContentMetadata struct {
	ContentType string `json:"contentType,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Extension   string `json:"extension,omitempty"`
	FileName    string `json:"fileName,omitempty"`
}

// AzureGitBlob represents raw blob content and metadata.
type AzureGitBlob struct {
	ObjectID string `json:"objectId"`
	Size     int64  `json:"size"`
	Content  string `json:"content"`
	URL      string `json:"url,omitempty"`
}

// AzureGitTree represents a directory tree in Git.
type AzureGitTree struct {
	ObjectID    string              `json:"objectId"`
	URL         string              `json:"url,omitempty"`
	TreeEntries []AzureGitTreeEntry `json:"treeEntries"`
}

// AzureGitTreeEntry represents an entry inside a Git tree.
type AzureGitTreeEntry struct {
	Mode          string `json:"mode"`
	GitObjectType string `json:"gitObjectType"`
	ObjectID      string `json:"objectId"`
	RelativePath  string `json:"relativePath"`
	Size          int64  `json:"size,omitempty"`
	URL           string `json:"url,omitempty"`
}

// AzureGitPush represents a Git push operation.
type AzureGitPush struct {
	PushID     int64                   `json:"pushId"`
	Date       time.Time               `json:"date"`
	PushedBy   *AzureDevOpsIdentityRef `json:"pushedBy,omitempty"`
	RefUpdates []AzureGitRefUpdate     `json:"refUpdates"`
	Commits    []AzureGitCommitInfo    `json:"commits"`
}

// AzureGitCommitInfo holds commit details.
type AzureGitCommitInfo struct {
	CommitID     string          `json:"commitId"`
	Author       *AzureGitAuthor `json:"author,omitempty"`
	Committer    *AzureGitAuthor `json:"committer,omitempty"`
	Comment      string          `json:"comment"`
	ChangeCounts map[string]int  `json:"changeCounts,omitempty"`
	URL          string          `json:"url,omitempty"`
}

// AzureGitAuthor represents author/committer information.
type AzureGitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// CreateAzureGitPushRequest contains data to push changes into Azure DevOps repository.
type CreateAzureGitPushRequest struct {
	RefUpdates []AzureGitRefUpdate   `json:"refUpdates"`
	Commits    []CreateAzureGitCommit `json:"commits"`
}

// CreateAzureGitCommit contains commit message and change entries for a push.
type CreateAzureGitCommit struct {
	Comment string            `json:"comment"`
	Changes []AzureGitChange  `json:"changes"`
}

// AzureGitChange represents a change operation on a file.
type AzureGitChange struct {
	ChangeType  string                  `json:"changeType"` // "add", "edit", "delete"
	Item        *AzureGitItemDescriptor `json:"item"`
	NewContent  *AzureGitItemContent    `json:"newContent,omitempty"`
}

// AzureGitItemDescriptor points to the target path.
type AzureGitItemDescriptor struct {
	Path string `json:"path"`
}

// AzureGitItemContent specifies the new content.
type AzureGitItemContent struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"` // "rawtext", "base64encoded"
}

// AzureDevOpsRepositoriesAndRefsService handles Azure DevOps Git repositories, refs, trees, and pushes.
type AzureDevOpsRepositoriesAndRefsService struct {
	baseURL    string
	pat        string
	httpClient *http.Client
}

// NewAzureDevOpsRepositoriesAndRefsService creates a new instance of the service.
func NewAzureDevOpsRepositoriesAndRefsService(baseURL, pat string, client *http.Client) *AzureDevOpsRepositoriesAndRefsService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://dev.azure.com"
	}
	return &AzureDevOpsRepositoriesAndRefsService{
		baseURL:    strings.TrimRight(baseURL, "/"),
		pat:        pat,
		httpClient: client,
	}
}

func (s *AzureDevOpsRepositoriesAndRefsService) applyAuth(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(":" + s.pat))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// GetRepository retrieves a Git repository by ID or name.
func (s *AzureDevOpsRepositoriesAndRefsService) GetRepository(ctx context.Context, organization, project, repositoryID string) (*AzureDevOpsRepo, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get repository failed (status %d): %s", resp.StatusCode, string(body))
	}

	var repo AzureDevOpsRepo
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

// ListRepositories retrieves all repositories in a project or organization.
func (s *AzureDevOpsRepositoriesAndRefsService) ListRepositories(ctx context.Context, organization, project string, includeHidden, includeAll bool) ([]AzureDevOpsRepo, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories?includeHidden=%t&includeAll=%t&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), includeHidden, includeAll)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list repositories failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Value []AzureDevOpsRepo `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Value, nil
}

// CreateRepository creates a new Git repository.
func (s *AzureDevOpsRepositoriesAndRefsService) CreateRepository(ctx context.Context, organization, project string, createReq CreateAzureRepoRequest) (*AzureDevOpsRepo, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project))

	bodyBytes, err := json.Marshal(createReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create repository failed (status %d): %s", resp.StatusCode, string(body))
	}

	var repo AzureDevOpsRepo
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

// ForkRepository creates a fork of an existing repository into a target repository name.
func (s *AzureDevOpsRepositoriesAndRefsService) ForkRepository(ctx context.Context, organization, project, sourceRepoID, targetRepoName string) (*AzureDevOpsRepo, error) {
	createReq := CreateAzureRepoRequest{
		Name: targetRepoName,
		Project: &AzureDevOpsProjectRef{
			Name: project,
		},
		ParentRepository: &AzureDevOpsRepoRef{
			ID: sourceRepoID,
		},
	}
	return s.CreateRepository(ctx, organization, project, createReq)
}

// ListForks retrieves all forks created from a specific repository.
func (s *AzureDevOpsRepositoriesAndRefsService) ListForks(ctx context.Context, organization, project, repositoryID string) ([]AzureDevOpsRepoRef, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/forks?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list forks failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Value []AzureDevOpsRepoRef `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Value, nil
}

// ListGitRefs retrieves branches or tags from a repository.
func (s *AzureDevOpsRepositoriesAndRefsService) ListGitRefs(ctx context.Context, organization, project, repositoryID, filter string) ([]AzureGitRefDetails, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID))
	if filter != "" {
		endpoint += "&filter=" + url.QueryEscape(filter)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list git refs failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Value []AzureGitRefDetails `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Value, nil
}

// GetGitRef retrieves a specific Git reference by name (e.g. "heads/main").
func (s *AzureDevOpsRepositoriesAndRefsService) GetGitRef(ctx context.Context, organization, project, repositoryID, refName string) (*AzureGitRefDetails, error) {
	refs, err := s.ListGitRefs(ctx, organization, project, repositoryID, refName)
	if err != nil {
		return nil, err
	}
	for _, r := range refs {
		if r.Name == refName || r.Name == "refs/"+refName {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("git ref not found: %s", refName)
}

// UpdateGitRefs updates or creates references in bulk.
func (s *AzureDevOpsRepositoriesAndRefsService) UpdateGitRefs(ctx context.Context, organization, project, repositoryID string, updates []AzureGitRefUpdate) ([]AzureGitRefUpdateResult, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID))

	bodyBytes, err := json.Marshal(updates)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update git refs failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Value []AzureGitRefUpdateResult `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Value, nil
}

// LockGitRef locks a branch to prevent updates.
func (s *AzureDevOpsRepositoriesAndRefsService) LockGitRef(ctx context.Context, organization, project, repositoryID, refName string) (*AzureGitRefDetails, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?filter=%s&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.QueryEscape(refName))

	payload := map[string]bool{"isLocked": true}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lock git ref failed (status %d): %s", resp.StatusCode, string(body))
	}

	var ref AzureGitRefDetails
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// UnlockGitRef unlocks a locked branch.
func (s *AzureDevOpsRepositoriesAndRefsService) UnlockGitRef(ctx context.Context, organization, project, repositoryID, refName string) (*AzureGitRefDetails, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/refs?filter=%s&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.QueryEscape(refName))

	payload := map[string]bool{"isLocked": false}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unlock git ref failed (status %d): %s", resp.StatusCode, string(body))
	}

	var ref AzureGitRefDetails
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// GetGitItem retrieves metadata and content of a file or folder at a specific version.
func (s *AzureDevOpsRepositoriesAndRefsService) GetGitItem(ctx context.Context, organization, project, repositoryID, path, version, versionType string) (*AzureGitItem, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/items?path=%s&includeContent=true&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.QueryEscape(path))
	if version != "" {
		endpoint += "&versionDescriptor.version=" + url.QueryEscape(version)
		if versionType != "" {
			endpoint += "&versionDescriptor.versionType=" + url.QueryEscape(versionType)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get git item failed (status %d): %s", resp.StatusCode, string(body))
	}

	var item AzureGitItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}
	return &item, nil
}

// GetGitBlob retrieves raw blob content for a given SHA.
func (s *AzureDevOpsRepositoriesAndRefsService) GetGitBlob(ctx context.Context, organization, project, repositoryID, sha string) (*AzureGitBlob, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/blobs/%s?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.PathEscape(sha))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get git blob failed (status %d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &AzureGitBlob{
		ObjectID: sha,
		Size:     int64(len(data)),
		Content:  string(data),
	}, nil
}

// GetGitTree retrieves a directory tree structure.
func (s *AzureDevOpsRepositoriesAndRefsService) GetGitTree(ctx context.Context, organization, project, repositoryID, sha string, recursive bool) (*AzureGitTree, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/trees/%s?recursive=%t&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.PathEscape(sha), recursive)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get git tree failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tree AzureGitTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, err
	}
	return &tree, nil
}

// CreateGitPush executes a Git push to create commits and update refs.
func (s *AzureDevOpsRepositoriesAndRefsService) CreateGitPush(ctx context.Context, organization, project, repositoryID string, pushReq CreateAzureGitPushRequest) (*AzureGitPush, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pushes?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID))

	bodyBytes, err := json.Marshal(pushReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create git push failed (status %d): %s", resp.StatusCode, string(body))
	}

	var push AzureGitPush
	if err := json.NewDecoder(resp.Body).Decode(&push); err != nil {
		return nil, err
	}
	return &push, nil
}

// GetGitCommit retrieves a single commit by ID.
func (s *AzureDevOpsRepositoriesAndRefsService) GetGitCommit(ctx context.Context, organization, project, repositoryID, commitID string) (*AzureGitCommitInfo, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/commits/%s?api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), url.PathEscape(commitID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get git commit failed (status %d): %s", resp.StatusCode, string(body))
	}

	var commit AzureGitCommitInfo
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		return nil, err
	}
	return &commit, nil
}

// ListGitCommits retrieves commits in a repository with optional filtering.
func (s *AzureDevOpsRepositoriesAndRefsService) ListGitCommits(ctx context.Context, organization, project, repositoryID string, top, skip int) ([]AzureGitCommitInfo, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/commits?$top=%d&$skip=%d&api-version=7.1-preview.1",
		s.baseURL, url.PathEscape(organization), url.PathEscape(project), url.PathEscape(repositoryID), top, skip)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.applyAuth(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list git commits failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Value []AzureGitCommitInfo `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Value, nil
}
