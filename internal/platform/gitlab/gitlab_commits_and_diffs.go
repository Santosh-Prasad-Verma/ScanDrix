package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

// MRChangeCount summarizes line and file alterations in a merge request.
type MRChangeCount struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Changes   int `json:"changes"`
	Files     int `json:"files"`
}

// RTTMMergeRequest holds turnaround and metrics for real-time team review tracking.
type RTTMMergeRequest struct {
	ID           int      `json:"id"`
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	Author       string   `json:"author"`
	State        string   `json:"state"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	MergedAt     string   `json:"merged_at,omitempty"`
	Reviewers    []string `json:"reviewers"`
	Draft        bool     `json:"draft"`
	WebURL       string   `json:"web_url"`
	LineChanges  int      `json:"line_changes"`
	FilesChanged int      `json:"files_changed"`
}

// CommitFileAction represents an individual file action within a multi-file commit.
type CommitFileAction struct {
	Action   string `json:"action"` // "create", "delete", "move", "update", "chmod"
	FilePath string `json:"file_path"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"` // "text" or "base64"
}

// MultiFileCommitRequest holds parameters for creating a multi-file commit on GitLab.
type MultiFileCommitRequest struct {
	Branch        string             `json:"branch"`
	CommitMessage string             `json:"commit_message"`
	StartBranch   string             `json:"start_branch,omitempty"`
	Actions       []CommitFileAction `json:"actions"`
	AuthorEmail   string             `json:"author_email,omitempty"`
	AuthorName    string             `json:"author_name,omitempty"`
}

// CommitDiffSummary represents a single file diff entry from a comparison.
type CommitDiffSummary struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
	Diff        string `json:"diff"`
}

// CommitsAndDiffsService provides merge request change counting, RTTM analytics, and multi-file commits.
type CommitsAndDiffsService struct {
	client *Client
}

// NewCommitsAndDiffsService creates a new CommitsAndDiffsService.
func NewCommitsAndDiffsService(client *Client) *CommitsAndDiffsService {
	return &CommitsAndDiffsService{client: client}
}

// CountChangesInMergeRequest counts the added, deleted, and modified lines in a merge request.
func (s *CommitsAndDiffsService) CountChangesInMergeRequest(ctx context.Context, projectID, mrIID int) (*MRChangeCount, error) {
	endpoint := fmt.Sprintf("/api/v4/projects/%d/merge_requests/%d/changes", projectID, mrIID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d for MR changes", resp.StatusCode)
	}

	var payload struct {
		Changes []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
			Diff    string `json:"diff"`
		} `json:"changes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	summary := &MRChangeCount{
		Files: len(payload.Changes),
	}

	for _, ch := range payload.Changes {
		lines := strings.Split(ch.Diff, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				summary.Additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				summary.Deletions++
			}
		}
	}
	summary.Changes = summary.Additions + summary.Deletions

	return summary, nil
}

// GetPullRequestsForRTTM retrieves merge requests for team turnaround metrics and review volume analysis.
func (s *CommitsAndDiffsService) GetPullRequestsForRTTM(ctx context.Context, projectID int, state string, limit int) ([]RTTMMergeRequest, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if state == "" {
		state = "all"
	}

	endpoint := fmt.Sprintf("/api/v4/projects/%d/merge_requests?state=%s&per_page=%d&order_by=updated_at&sort=desc",
		projectID, url.QueryEscape(state), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d for RTTM MRs", resp.StatusCode)
	}

	var mrs []struct {
		ID        int    `json:"id"`
		IID       int    `json:"iid"`
		Title     string `json:"title"`
		State     string `json:"state"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		MergedAt  string `json:"merged_at"`
		Draft     bool   `json:"draft"`
		WebURL    string `json:"web_url"`
		Author    struct {
			Username string `json:"username"`
		} `json:"author"`
		Reviewers []struct {
			Username string `json:"username"`
		} `json:"reviewers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, err
	}

	results := make([]RTTMMergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		revs := make([]string, 0, len(mr.Reviewers))
		for _, r := range mr.Reviewers {
			revs = append(revs, r.Username)
		}

		results = append(results, RTTMMergeRequest{
			ID:        mr.ID,
			IID:       mr.IID,
			Title:     mr.Title,
			Author:    mr.Author.Username,
			State:     mr.State,
			CreatedAt: mr.CreatedAt,
			UpdatedAt: mr.UpdatedAt,
			MergedAt:  mr.MergedAt,
			Reviewers: revs,
			Draft:     mr.Draft,
			WebURL:    mr.WebURL,
		})
	}

	return results, nil
}

// GetChangedFilesSinceLastCommit compares two commit SHAs and returns changed file summaries.
func (s *CommitsAndDiffsService) GetChangedFilesSinceLastCommit(ctx context.Context, projectID int, fromSHA, toSHA string) ([]CommitDiffSummary, error) {
	if fromSHA == "" || toSHA == "" {
		return nil, fmt.Errorf("both fromSHA and toSHA are required")
	}

	endpoint := fmt.Sprintf("/api/v4/projects/%d/repository/compare?from=%s&to=%s",
		projectID, url.QueryEscape(fromSHA), url.QueryEscape(toSHA))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d for commit compare", resp.StatusCode)
	}

	var compareResp struct {
		Diffs []CommitDiffSummary `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&compareResp); err != nil {
		return nil, err
	}

	return compareResp.Diffs, nil
}

// UploadFilesCommit submits a multi-file batch commit to a GitLab branch.
func (s *CommitsAndDiffsService) UploadFilesCommit(ctx context.Context, projectID int, payload MultiFileCommitRequest) (string, error) {
	if payload.Branch == "" {
		return "", fmt.Errorf("branch name is required")
	}
	if len(payload.Actions) == 0 {
		return "", fmt.Errorf("at least one commit action is required")
	}

	endpoint := fmt.Sprintf("/api/v4/projects/%d/repository/commits", projectID)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.client.baseURL+endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API returned status %d for multi-file commit", resp.StatusCode)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// CreateMergeRequestWithFiles creates a new branch, commits the specified files, and opens a merge request.
func (s *CommitsAndDiffsService) CreateMergeRequestWithFiles(ctx context.Context, projectID int, targetBranch, newBranch, title, desc string, actions []CommitFileAction) (*types.PullRequest, error) {
	// 1. Commit files to new branch starting from targetBranch
	commitReq := MultiFileCommitRequest{
		Branch:        newBranch,
		StartBranch:   targetBranch,
		CommitMessage: fmt.Sprintf("chore: %s", title),
		Actions:       actions,
	}

	commitSHA, err := s.UploadFilesCommit(ctx, projectID, commitReq)
	if err != nil {
		return nil, fmt.Errorf("failed to commit files: %w", err)
	}

	// 2. Open merge request
	mrEndpoint := fmt.Sprintf("/api/v4/projects/%d/merge_requests", projectID)
	mrBody, _ := json.Marshal(map[string]string{
		"source_branch": newBranch,
		"target_branch": targetBranch,
		"title":         title,
		"description":   desc,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.client.baseURL+mrEndpoint, bytes.NewReader(mrBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d for MR creation", resp.StatusCode)
	}

	var mr struct {
		ID     int    `json:"id"`
		IID    int    `json:"iid"`
		Title  string `json:"title"`
		State  string `json:"state"`
		WebURL string `json:"web_url"`
		Author struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, err
	}

	return &types.PullRequest{
		ID:           strconv.Itoa(mr.ID),
		Number:       mr.IID,
		PullNumber:   mr.IID,
		Title:        mr.Title,
		State:        mr.State,
		Author:       mr.Author.Username,
		SourceBranch: newBranch,
		TargetBranch: targetBranch,
		HeadSHA:      commitSHA,
		URL:          mr.WebURL,
		PRURL:        mr.WebURL,
	}, nil
}
