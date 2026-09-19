package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AzureDevOpsAdvancedService provides extended capabilities for Azure DevOps REST 7.0/7.1 APIs,
// including PR iteration diffs, thread discussions, branch policies, work items, and reviewer votes.
type AzureDevOpsAdvancedService struct {
	client     *AzureReposRequestHelper
	httpClient *http.Client
	baseURL    string
}

// NewAzureDevOpsAdvancedService creates an instance of AzureDevOpsAdvancedService.
func NewAzureDevOpsAdvancedService(helper *AzureReposRequestHelper, httpClient *http.Client, baseURL ...string) *AzureDevOpsAdvancedService {
	url := "https://dev.azure.com"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	if helper == nil {
		helper = NewAzureReposRequestHelper(httpClient, 45*time.Second)
	}
	return &AzureDevOpsAdvancedService{
		client:     helper,
		httpClient: httpClient,
		baseURL:    url,
	}
}

// PullRequestIteration models an Azure DevOps pull request iteration.
type PullRequestIteration struct {
	ID          int       `json:"id"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	CreatedDate time.Time `json:"createdDate"`
	UpdatedDate time.Time `json:"updatedDate"`
	SourceRefCommit string `json:"sourceRefCommit"`
	TargetRefCommit string `json:"targetRefCommit"`
	CommonRefCommit string `json:"commonRefCommit"`
}

// PullRequestThread models an inline discussion thread in Azure DevOps.
type PullRequestThread struct {
	ID             int                  `json:"id"`
	Status         string               `json:"status"` // "active", "fixed", "wontFix", "closed", "byDesign"
	FilePath       string               `json:"filePath,omitempty"`
	Line           int                  `json:"line,omitempty"`
	StartLine      int                  `json:"startLine,omitempty"`
	Comments       []ThreadCommentEntry `json:"comments"`
	Properties     map[string]any       `json:"properties,omitempty"`
	IsDeleted      bool                 `json:"isDeleted"`
}

// ThreadCommentEntry models an individual comment inside a thread.
type ThreadCommentEntry struct {
	ID              int       `json:"id"`
	ParentCommentID int       `json:"parentCommentId"`
	Author          string    `json:"author"`
	Content         string    `json:"content"`
	PublishedDate   time.Time `json:"publishedDate"`
	CommentType     string    `json:"commentType"` // "text", "system", "codeChange"
}

// BranchPolicyEvaluation models the evaluation status of a branch policy.
type BranchPolicyEvaluation struct {
	EvaluationID string    `json:"evaluationId"`
	Status       string    `json:"status"` // "queued", "running", "approved", "rejected", "notApplicable"
	PolicyType   string    `json:"policyType"`
	DisplayName  string    `json:"displayName"`
	IsBlocking   bool      `json:"isBlocking"`
	StartedDate  time.Time `json:"startedDate"`
	CompletedDate time.Time `json:"completedDate,omitempty"`
}

// WorkItemLink models a work item linked to a pull request.
type WorkItemLink struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Type     string `json:"type,omitempty"`
	State    string `json:"state,omitempty"`
	Assigned string `json:"assignedTo,omitempty"`
}

// ReviewerVote models an assigned reviewer and their vote status.
type ReviewerVote struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	Vote        int    `json:"vote"` // 10: approved, 5: approved with suggestions, 0: no vote, -5: waiting, -10: rejected
	IsRequired  bool   `json:"isRequired"`
	HasDeclined bool   `json:"hasDeclined"`
}

// GetPullRequestIterations retrieves all iterations for a pull request.
func (s *AzureDevOpsAdvancedService) GetPullRequestIterations(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
) ([]PullRequestIteration, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/iterations?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var wrapper struct {
		Value []struct {
			ID          int    `json:"id"`
			Description string `json:"description"`
			Author      struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			CreatedDate     time.Time `json:"createdDate"`
			UpdatedDate     time.Time `json:"updatedDate"`
			SourceRefCommit struct {
				CommitID string `json:"commitId"`
			} `json:"sourceRefCommit"`
			TargetRefCommit struct {
				CommitID string `json:"commitId"`
			} `json:"targetRefCommit"`
			CommonRefCommit struct {
				CommitID string `json:"commitId"`
			} `json:"commonRefCommit"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	iterations := make([]PullRequestIteration, len(wrapper.Value))
	for i, v := range wrapper.Value {
		iterations[i] = PullRequestIteration{
			ID:              v.ID,
			Description:     v.Description,
			Author:          v.Author.DisplayName,
			CreatedDate:     v.CreatedDate,
			UpdatedDate:     v.UpdatedDate,
			SourceRefCommit: v.SourceRefCommit.CommitID,
			TargetRefCommit: v.TargetRefCommit.CommitID,
			CommonRefCommit: v.CommonRefCommit.CommitID,
		}
	}
	return iterations, nil
}

// GetPullRequestThreads retrieves all discussion threads for a pull request.
func (s *AzureDevOpsAdvancedService) GetPullRequestThreads(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
) ([]PullRequestThread, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(body))
	}

	var wrapper struct {
		Value []struct {
			ID            int    `json:"id"`
			Status        string `json:"status"`
			IsDeleted     bool   `json:"isDeleted"`
			ThreadContext *struct {
				FilePath  string `json:"filePath"`
				RightFile *struct {
					Line int `json:"line"`
				} `json:"rightFile"`
				LeftFile *struct {
					Line int `json:"line"`
				} `json:"leftFile"`
			} `json:"threadContext"`
			Comments []struct {
				ID              int    `json:"id"`
				ParentCommentID int    `json:"parentCommentId"`
				Content         string `json:"content"`
				CommentType     string `json:"commentType"`
				PublishedDate   time.Time `json:"publishedDate"`
				Author          struct {
					DisplayName string `json:"displayName"`
				} `json:"author"`
			} `json:"comments"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	threads := make([]PullRequestThread, len(wrapper.Value))
	for i, v := range wrapper.Value {
		th := PullRequestThread{
			ID:        v.ID,
			Status:    v.Status,
			IsDeleted: v.IsDeleted,
		}
		if v.ThreadContext != nil {
			th.FilePath = v.ThreadContext.FilePath
			if v.ThreadContext.RightFile != nil {
				th.Line = v.ThreadContext.RightFile.Line
			} else if v.ThreadContext.LeftFile != nil {
				th.Line = v.ThreadContext.LeftFile.Line
			}
		}
		th.Comments = make([]ThreadCommentEntry, len(v.Comments))
		for j, c := range v.Comments {
			th.Comments[j] = ThreadCommentEntry{
				ID:              c.ID,
				ParentCommentID: c.ParentCommentID,
				Author:          c.Author.DisplayName,
				Content:         c.Content,
				PublishedDate:   c.PublishedDate,
				CommentType:     c.CommentType,
			}
		}
		threads[i] = th
	}
	return threads, nil
}

// CreateThreadComment appends a comment to an existing thread or starts a new thread.
func (s *AzureDevOpsAdvancedService) CreateThreadComment(
	ctx context.Context,
	token, org, project, repoID string,
	prID, threadID int,
	content string,
) (*ThreadCommentEntry, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID, threadID)

	payload := map[string]any{
		"content":     content,
		"commentType": "text",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID              int       `json:"id"`
		ParentCommentID int       `json:"parentCommentId"`
		Content         string    `json:"content"`
		CommentType     string    `json:"commentType"`
		PublishedDate   time.Time `json:"publishedDate"`
		Author          struct {
			DisplayName string `json:"displayName"`
		} `json:"author"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode comment: %w", err)
	}

	return &ThreadCommentEntry{
		ID:              res.ID,
		ParentCommentID: res.ParentCommentID,
		Author:          res.Author.DisplayName,
		Content:         res.Content,
		PublishedDate:   res.PublishedDate,
		CommentType:     res.CommentType,
	}, nil
}

// UpdateThreadStatus sets the resolution status of a thread ("active", "fixed", "wontFix", "closed", "byDesign").
func (s *AzureDevOpsAdvancedService) UpdateThreadStatus(
	ctx context.Context,
	token, org, project, repoID string,
	prID, threadID int,
	status string,
) error {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID, threadID)

	payload := map[string]string{
		"status": status,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetPolicyEvaluations fetches all branch policy evaluations for a pull request.
func (s *AzureDevOpsAdvancedService) GetPolicyEvaluations(
	ctx context.Context,
	token, org, project string,
	artifactID string,
) ([]BranchPolicyEvaluation, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/evaluations?artifactId=%s&api-version=7.1-preview.1",
		s.baseURL, org, project, artifactID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []struct {
			EvaluationID string `json:"evaluationId"`
			Status       string `json:"status"`
			Configuration struct {
				Type struct {
					DisplayName string `json:"displayName"`
				} `json:"type"`
				IsBlocking bool `json:"isBlocking"`
			} `json:"configuration"`
			StartedDate   time.Time `json:"startedDate"`
			CompletedDate time.Time `json:"completedDate"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode evaluations: %w", err)
	}

	evals := make([]BranchPolicyEvaluation, len(wrapper.Value))
	for i, v := range wrapper.Value {
		evals[i] = BranchPolicyEvaluation{
			EvaluationID:  v.EvaluationID,
			Status:        v.Status,
			PolicyType:    v.Configuration.Type.DisplayName,
			DisplayName:   v.Configuration.Type.DisplayName,
			IsBlocking:    v.Configuration.IsBlocking,
			StartedDate:   v.StartedDate,
			CompletedDate: v.CompletedDate,
		}
	}
	return evals, nil
}

// GetPullRequestWorkItems retrieves all work items linked to a pull request.
func (s *AzureDevOpsAdvancedService) GetPullRequestWorkItems(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
) ([]WorkItemLink, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/workitems?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode workitems: %w", err)
	}

	items := make([]WorkItemLink, len(wrapper.Value))
	for i, v := range wrapper.Value {
		items[i] = WorkItemLink{
			ID:  v.ID,
			URL: v.URL,
		}
	}
	return items, nil
}

// SetReviewerVote casts or updates a vote on an Azure DevOps pull request.
// Vote values: 10 (Approved), 5 (Approved with suggestions), 0 (No vote), -5 (Waiting for author), -10 (Rejected).
func (s *AzureDevOpsAdvancedService) SetReviewerVote(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
	reviewerID string,
	vote int,
) (*ReviewerVote, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers/%s?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID, reviewerID)

	payload := map[string]int{
		"vote": vote,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal vote: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops api error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		UniqueName  string `json:"uniqueName"`
		Vote        int    `json:"vote"`
		IsRequired  bool   `json:"isRequired"`
		HasDeclined bool   `json:"hasDeclined"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode vote response: %w", err)
	}

	return &ReviewerVote{
		ID:          res.ID,
		DisplayName: res.DisplayName,
		UniqueName:  res.UniqueName,
		Vote:        res.Vote,
		IsRequired:  res.IsRequired,
		HasDeclined: res.HasDeclined,
	}, nil
}

// CalculateDiffLinePosition finds the right-file or left-file line index for Azure DevOps inline threads.
func (s *AzureDevOpsAdvancedService) CalculateDiffLinePosition(
	diffText string,
	targetPath string,
	targetLine int,
) (rightLine, leftLine int, err error) {
	lines := strings.Split(diffText, "\n")
	inTargetFile := false
	currentRight := 0
	currentLeft := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			inTargetFile = strings.Contains(line, "b/"+targetPath) || strings.Contains(line, targetPath)
			continue
		}

		if !inTargetFile {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			// @@ -oldStart,oldLen +newStart,newLen @@
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				if strings.HasPrefix(parts[1], "-") {
					oldInfo := strings.TrimPrefix(parts[1], "-")
					oldParts := strings.Split(oldInfo, ",")
					if parsed, err := strconv.Atoi(oldParts[0]); err == nil {
						currentLeft = parsed - 1
					}
				}
				if strings.HasPrefix(parts[2], "+") {
					newInfo := strings.TrimPrefix(parts[2], "+")
					newParts := strings.Split(newInfo, ",")
					if parsed, err := strconv.Atoi(newParts[0]); err == nil {
						currentRight = parsed - 1
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			currentRight++
			if currentRight == targetLine {
				return currentRight, 0, nil
			}
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			currentLeft++
		} else {
			currentRight++
			currentLeft++
			if currentRight == targetLine {
				return currentRight, currentLeft, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("line %d not found in diff for %s", targetLine, targetPath)
}
